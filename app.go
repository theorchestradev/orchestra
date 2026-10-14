package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/theorchestradev/orchestra/internal/appearance"
	"github.com/theorchestradev/orchestra/internal/core"
	"github.com/theorchestradev/orchestra/internal/telemetry"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"log/slog"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type App struct {
	ctx             context.Context
	log             *slog.Logger
	store           *core.Store
	updater         *core.Updater
	telemetry       *telemetry.Client
	telemetryMu     sync.Mutex
	startupError    error
	frontendReady   atomic.Bool
	quitConfirmed   atomic.Bool
	usageMu         sync.Mutex
	usageCache      map[string]usageReading // By profile ID; "" is Default.
	openRouterMu    sync.Mutex
	openRouterCache map[string]openRouterReading // By profile ID; "" is Default.
}

// statePath returns the workspace file, which ORCHESTRA_STATE_FILE overrides for QA.
func statePath() string {
	if path := os.Getenv("ORCHESTRA_STATE_FILE"); path != "" {
		return path
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".orchestra", "state.json")
}
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	path := statePath()
	if os.Getenv("ORCHESTRA_STATE_FILE") == "" {
		home, _ := os.UserHomeDir()
		from := filepath.Join(home, "Library", "Application Support", "Orchestra Rewrite")
		_, existed := os.Stat(path)
		// Earlier versions kept the workspace here. A custom state file never migrates.
		if a.startupError = core.MigrateState(from, filepath.Dir(path)); a.startupError != nil {
			a.log.Error("workspace migration failed", "error", a.startupError)
			return
		}
		if _, err := os.Stat(path); err == nil && existed != nil {
			a.log.Info("workspace migrated", "from", from)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		a.startupError = err
		return
	}
	a.store, a.startupError = core.NewStore(path, exe, func(event string, data any) { runtime.EventsEmit(ctx, event, data) }, a.log)
	if a.startupError != nil {
		a.log.Error("workspace open failed", "error", a.startupError)
	}
	if a.store != nil {
		store := a.store
		a.updater = core.NewUpdater(core.UpdaterOptions{
			Current:     version,
			Executable:  exe,
			Source:      core.GitHubReleases,
			Download:    core.GitHubDownload,
			Preferences: func() core.Preferences { return store.Snapshot().Preferences },
			Emit:        func(s core.UpdateStatus) { runtime.EventsEmit(ctx, "update", s) },
			Log:         a.log,
		})
		go a.updater.Run(ctx)
	}
	// A custom state file keeps worktrees beside it so QA runs stay isolated.
	if a.store != nil && os.Getenv("ORCHESTRA_STATE_FILE") == "" {
		home, _ := os.UserHomeDir()
		a.store.SetWorktreesDir(filepath.Join(home, ".orchestra", "worktrees"))
	}
	if a.store != nil {
		a.telemetry = telemetry.New(buildTelemetryKey, buildTelemetryHost, version, a.log)
		p := a.store.Snapshot().Preferences
		a.telemetry.SetConsent(p.TelemetryEnabled, p.TelemetryInstallID)
		a.store.SetTracker(a.telemetry.Track)
		a.trackAppOpened(p)
	}
}
func (a *App) shutdown(context.Context) {
	if a.store != nil {
		a.store.Shutdown()
	}
	// Sessions have stopped, so a staged update can replace the app now. The event is
	// tracked before telemetry flushes and only with the user's consent.
	if a.updater != nil {
		installed, err := a.updater.ApplyStaged()
		if err != nil {
			a.log.Error("update install failed", "error", err)
		}
		if installed != "" && a.telemetry != nil {
			a.telemetry.Track("update_installed", map[string]any{"from_version": version, "to_version": installed})
		}
	}
	if a.telemetry != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		a.telemetry.Close(ctx)
	}
}

func (a *App) TelemetryStatus() telemetry.Status {
	if a.telemetry == nil {
		return telemetry.Status{Reason: "Orchestra is starting."}
	}
	return a.telemetry.Status()
}

// Track is the only frontend analytics entry point and uses the same validator as Store.
func (a *App) Track(event string, props map[string]any) {
	if a.telemetry != nil {
		a.telemetry.Track(event, props)
	}
}
func (a *App) trackAppOpened(p core.Preferences) {
	kind := "built-in"
	if strings.HasPrefix(p.Theme, "custom:") {
		kind = "custom"
	}
	arch := goruntime.GOARCH
	if arch != "amd64" && arch != "arm64" {
		arch = "other"
	}
	a.Track("app_opened", map[string]any{"version": version, "os": goruntime.GOOS, "arch": arch, "theme_kind": kind, "arc_mode": p.ArcMode})
}
func (a *App) SetTelemetryConsent(enabled bool) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	a.telemetryMu.Lock()
	defer a.telemetryMu.Unlock()
	if enabled && (a.telemetry == nil || !a.telemetry.Status().Available) {
		return core.State{}, fmt.Errorf("Usage sharing is unavailable in this environment.")
	}
	// Revoke before persistence: a failed disk write must not allow another send.
	if !enabled && a.telemetry != nil {
		a.telemetry.SetConsent(false, "")
	}
	previous := a.store.Snapshot().Preferences.TelemetryEnabled
	state, err := a.store.SetTelemetryConsent(enabled)
	if err == nil && a.telemetry != nil {
		a.telemetry.SetConsent(enabled, state.Preferences.TelemetryInstallID)
		if enabled && !previous {
			a.trackAppOpened(state.Preferences)
		}
	}
	return state, err
}
func (a *App) ready() error {
	if a.startupError != nil {
		return a.startupError
	}
	if a.store == nil {
		return fmt.Errorf("Orchestra is starting.")
	}
	return nil
}
func (a *App) Workspace() (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	return a.store.Snapshot(), nil
}
func (a *App) ChooseFolder() (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}

	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Project Folder", CanCreateDirectories: false})
}

type usageReading struct {
	usage   []core.AgentUsage
	checked time.Time
}

// AgentUsage reads the Codex and Claude limits of a profile's accounts; "" is Default.
func (a *App) AgentUsage(refresh bool, profileID string) ([]core.AgentUsage, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	profile, err := a.store.Profile(profileID)
	if err != nil {
		return nil, err
	}
	a.usageMu.Lock()
	defer a.usageMu.Unlock()
	if a.usageCache == nil {
		a.usageCache = map[string]usageReading{}
	}
	cached, ok := a.usageCache[profileID]
	if !ok || time.Since(cached.checked) >= 5*time.Minute || (refresh && time.Since(cached.checked) >= 15*time.Second) {
		cached = usageReading{core.ReadAgentUsage(a.ctx, profile), time.Now()}
		a.usageCache[profileID] = cached
	}
	return cached.usage, nil
}

// QA workspaces use a separate Keychain service instead of the user's key. Each profile
// keeps its own management key; "" is Default, which keeps the original service.
func openRouterKeyService(profileID string) string {
	service := "com.orchestra.openrouter"
	if path := os.Getenv("ORCHESTRA_STATE_FILE"); path != "" {
		service += fmt.Sprintf(".workspace-%x", sha256.Sum256([]byte(path)))
	}
	if profileID != "" {
		service += ".profile-" + profileID
	}
	return service
}

type openRouterReading struct {
	account core.OpenRouterAccount
	checked time.Time
}

// openRouterProfile rejects an unknown profile, so no Keychain item is made for it.
func (a *App) openRouterProfile(profileID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	_, err := a.store.Profile(profileID)
	return err
}

// OpenRouterUsage reads the account of a profile's management key; "" is Default.
func (a *App) OpenRouterUsage(refresh bool, profileID string) (core.OpenRouterAccount, error) {
	if err := a.openRouterProfile(profileID); err != nil {
		return core.OpenRouterAccount{}, err
	}
	a.openRouterMu.Lock()
	defer a.openRouterMu.Unlock()
	if cached, ok := a.openRouterCache[profileID]; ok && time.Since(cached.checked) < 5*time.Minute && (!refresh || time.Since(cached.checked) < 15*time.Second) {
		return cached.account, nil
	}
	key, err := core.OpenRouterKey(openRouterKeyService(profileID))
	if err != nil {
		return core.OpenRouterAccount{}, err
	}
	result := core.OpenRouterAccount{}
	if key != "" {
		result, err = core.ReadOpenRouterUsage(a.ctx, key)
		if err != nil {
			result.Message = err.Error()
		}
	}
	a.cacheOpenRouter(profileID, result)
	return result, nil
}
func (a *App) cacheOpenRouter(profileID string, account core.OpenRouterAccount) {
	if a.openRouterCache == nil {
		a.openRouterCache = map[string]openRouterReading{}
	}
	a.openRouterCache[profileID] = openRouterReading{account, time.Now()}
}
func (a *App) SetOpenRouterKey(key, profileID string) (core.OpenRouterAccount, error) {
	if err := a.openRouterProfile(profileID); err != nil {
		return core.OpenRouterAccount{}, err
	}
	a.openRouterMu.Lock()
	defer a.openRouterMu.Unlock()
	key = strings.TrimSpace(key)
	result, err := core.ReadOpenRouterUsage(a.ctx, key)
	if err != nil {
		return core.OpenRouterAccount{}, err
	}
	if err := core.SaveOpenRouterKey(openRouterKeyService(profileID), key); err != nil {
		return core.OpenRouterAccount{}, err
	}
	a.cacheOpenRouter(profileID, result)
	return result, nil
}
func (a *App) RemoveOpenRouterKey(profileID string) error {
	if err := a.openRouterProfile(profileID); err != nil {
		return err
	}
	return a.removeOpenRouterKey(profileID)
}
func (a *App) removeOpenRouterKey(profileID string) error {
	a.openRouterMu.Lock()
	defer a.openRouterMu.Unlock()
	if err := core.RemoveOpenRouterKey(openRouterKeyService(profileID)); err != nil {
		return err
	}
	delete(a.openRouterCache, profileID)
	return nil
}
func (a *App) CreateProject(name, directory, description string) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}

	state, err := a.store.CreateProject(name, directory, description)
	if err == nil {
		if detected, e := a.store.DetectRepository(state.SelectedProjectID); e == nil {
			state = detected
		}
		for _, p := range state.Projects {
			if p.ID == state.SelectedProjectID {
				a.Track("project_added", map[string]any{"has_github_repository": p.Repository != ""})
				break
			}
		}
	}
	return state, err
}
func (a *App) RemoveProject(id string) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	return a.store.RemoveProject(id)
}
func (a *App) Rename(id, name string) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	return a.store.Rename(id, name)
}
func (a *App) EditProject(id, name, description, emoji string) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	return a.store.EditProject(id, name, description, emoji)
}
func (a *App) Select(project, destination string) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}

	return a.store.Select(project, destination)
}
func (a *App) ToggleProject(id string) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	return a.store.ToggleProject(id)
}
func (a *App) MoveProject(id string, index int) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	return a.store.MoveProject(id, index)
}
func (a *App) MoveSession(id string, index int) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	return a.store.MoveSession(id, index)
}
func (a *App) MoveScratchpad(projectID string, index int) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	return a.store.MoveScratchpad(projectID, index)
}
func (a *App) SaveScratchpad(id, text string, revision int) (int, error) {
	return a.store.SaveScratchpad(id, text, revision)
}
func (a *App) RenameScratchpad(id, name string) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	return a.store.RenameScratchpad(id, name)
}
func (a *App) SetScratchpadMode(id, mode string) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	return a.store.SetScratchpadMode(id, mode)
}
func (a *App) DeleteScratchpad(id string) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	return a.store.DeleteScratchpad(id)
}
func (a *App) SetPreferences(p core.Preferences) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}

	previousIcon := a.store.Snapshot().Preferences.AppIcon
	state, err := a.store.SetPreferences(p)
	if err == nil {
		if state.Preferences.AppIcon != previousIcon {
			a.applyAppIcon(state.Preferences.AppIcon)
		}
		a.applyAppearance(state.Preferences.Appearance)
	}
	return state, err
}
func (a *App) CreateSession(project, agent, name, prompt string, createWorktree bool, gitHubItem *core.GitHubItem, model, effort, profileID string) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}

	state, err := a.store.CreateSession(project, agent, name, prompt, createWorktree, gitHubItem, model, effort, profileID)
	if err == nil && gitHubItem != nil {
		action := "work_on"
		if gitHubItem.PullRequest {
			action = "review"
		}
		a.Track("github_action", map[string]any{"action": action})
	}
	return state, err
}

// AgentModels lists the models and efforts Work on and Review can start an agent with.
func (a *App) AgentModels(agent string) ([]core.AgentModel, error) {
	return core.AgentModels(a.ctx, agent)
}

// AgentSkillStatus, AgentSkillCommand and InstallAgentSkill back Settings → Agents →
// Agent Skill. Installing runs only when the user clicks Install or Update.
func (a *App) AgentSkillStatus() core.SkillStatus { return core.AgentSkillStatus(a.ctx) }
func (a *App) AgentSkillCommand(agents []string) (string, error) {
	return core.SkillCommand(agents)
}
func (a *App) InstallAgentSkill(agents []string) (core.SkillInstall, error) {
	if err := a.ready(); err != nil {
		return core.SkillInstall{}, err
	}
	return core.InstallAgentSkill(a.ctx, agents, a.store.Profiles())
}

// InstallProfileSkill installs the Orchestra skill where a profile is missing it; "" is Default.
// It runs only when the user clicks Install in Settings → Profiles.
func (a *App) InstallProfileSkill(profileID string) (core.SkillInstall, error) {
	if err := a.ready(); err != nil {
		return core.SkillInstall{}, err
	}
	profile, err := a.store.Profile(profileID)
	if err != nil {
		return core.SkillInstall{}, err
	}
	return core.InstallProfileSkill(a.ctx, profile)
}

// CreateProfile, EditProfile and RemoveProfile back Settings → Profiles.
func (a *App) CreateProfile(name, emoji string) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	return a.store.CreateProfile(name, emoji)
}
func (a *App) EditProfile(id, name, emoji string) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	return a.store.EditProfile(id, name, emoji)
}
func (a *App) RemoveProfile(id string) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	state, err := a.store.RemoveProfile(id)
	if err == nil {
		a.forgetUsage(id)
		// The profile's management key cannot be reached any more, so it leaves Keychain too.
		if keyErr := a.removeOpenRouterKey(id); keyErr != nil && goruntime.GOOS == "darwin" {
			a.log.Warn("OpenRouter key remove failed", "error", keyErr)
		}
	}
	return state, err
}
func (a *App) forgetUsage(profileID string) {
	a.usageMu.Lock()
	defer a.usageMu.Unlock()
	delete(a.usageCache, profileID)
}
func (a *App) SetProjectProfile(project, profileID string) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	return a.store.SetProjectProfile(project, profileID)
}

// ProfileLogins asks each agent's CLI whether the profile is signed in; "" is Default.
func (a *App) ProfileLogins(profileID string) ([]core.ProfileLogin, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	profile, err := a.store.Profile(profileID)
	if err != nil {
		return nil, err
	}
	return core.ProfileLogins(a.ctx, profile), nil
}

// LogIn adds a terminal session in the project that runs the agent's login command with the
// profile's config folder. The user completes the sign-in there.
func (a *App) LogIn(project, profileID, agent string) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	a.forgetUsage(profileID)
	return a.store.LogIn(project, profileID, agent)
}

func (a *App) CloseSession(id string, removeWorktree bool) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	return a.store.CloseSession(id, removeWorktree)
}
func (a *App) StartSession(id string, newConversation bool, cols, rows int) (core.TerminalOutput, error) {
	if err := a.ready(); err != nil {
		return core.TerminalOutput{}, err
	}

	output, err := a.store.StartSession(id, newConversation, cols, rows)
	if err != nil {
		a.Track("error_shown", map[string]any{"code": "session_start_failed"})
	}
	return output, err
}
func (a *App) TerminalSnapshot(id string) (core.TerminalOutput, error) {
	if err := a.ready(); err != nil {
		return core.TerminalOutput{}, err
	}

	return a.store.TerminalSnapshot(id)
}
func (a *App) WriteTerminal(id, text string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.store.WriteTerminal(id, text)
}
func (a *App) ResizeTerminal(id string, cols, rows int) error {
	if err := a.ready(); err != nil {
		return err
	}

	return a.store.ResizeTerminal(id, cols, rows)
}
func (a *App) DetectRepository(id string) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	return a.store.DetectRepository(id)
}
func (a *App) GitHubItems(project, state string) ([]json.RawMessage, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}

	return a.store.GitHubItems(project, state)
}
func (a *App) GitHubDetail(project string, number int, pull bool) (core.GitHubContent, error) {
	if err := a.ready(); err != nil {
		return core.GitHubContent{}, err
	}

	return a.store.GitHubDetail(project, number, pull)
}
func (a *App) GitHubMetadataOptions(project, field string) ([]json.RawMessage, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.store.GitHubMetadataOptions(project, field)
}
func (a *App) UpdateGitHubMetadata(project string, number int, pull bool, field string, values []string) error {
	if err := a.ready(); err != nil {
		return err
	}
	err := a.store.UpdateGitHubMetadata(project, number, pull, field, values)
	if err == nil {
		a.Track("github_action", map[string]any{"action": "edit_metadata"})
	}
	return err
}
func (a *App) SendRequest(from, to, text string) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}

	return a.store.SendRequest(from, to, text)
}
func (a *App) CancelRequest(id string) (core.State, error) { return a.store.CancelRequest(id) }
func (a *App) ExecuteRequest(id string) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	return a.store.ExecuteRequest(id)
}

func (a *App) SystemPalettes() map[string][]string { return appearance.SystemPalettes() }

// TerminalFonts lists the installed monospaced font families by name; it is empty when listing fails.
func (a *App) TerminalFonts() []string { return appearance.MonospaceFonts() }

// AboutInfo returns the build details that macOS shows in its About panel. Platforms
// without that panel show them in Settings.
func (a *App) AboutInfo() string { return aboutMessage() }

// UpdateStatus returns the last update check; CheckForUpdates runs one now and reports
// its error, unlike the automatic checks.
func (a *App) UpdateStatus() (core.UpdateStatus, error) {
	if err := a.ready(); err != nil {
		return core.UpdateStatus{}, err
	}
	return a.updater.Status(), nil
}
func (a *App) CheckForUpdates() (core.UpdateStatus, error) {
	if err := a.ready(); err != nil {
		return core.UpdateStatus{}, err
	}
	return a.updater.Check(a.ctx, true), nil
}

// InstallUpdate downloads and verifies a release and stages it for quitting. The
// frontend then quits through the normal flow when the user chose to restart.
func (a *App) InstallUpdate(version string, restart bool) (core.UpdateStatus, error) {
	if err := a.ready(); err != nil {
		return core.UpdateStatus{}, err
	}
	return a.updater.Install(a.ctx, version, restart), nil
}
func (a *App) CancelUpdateInstall() {
	if a.updater != nil {
		a.updater.CancelInstall()
	}
}

// WindowTranslucency turns the window's vibrancy on or off and reports whether it is in effect,
// which it is not while macOS Reduce Transparency is on.
func (a *App) WindowTranslucency(on bool) bool { return appearance.SetWindowTranslucent(on) }

// themesDirectory is ~/.orchestra/themes. A custom state file uses Themes/ beside it instead,
// keeping QA runs isolated.
func themesDirectory() string {
	if file := os.Getenv("ORCHESTRA_STATE_FILE"); file != "" {
		return filepath.Join(filepath.Dir(file), "Themes")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".orchestra", "themes")
}

// CustomThemes reads the themes directory on every call, so edits apply when the picker opens.
func (a *App) CustomThemes() appearance.CustomThemes {
	return appearance.LoadCustomThemes(themesDirectory())
}

// DuplicateTheme saves the colors of the selected theme as a new custom theme.
func (a *App) DuplicateTheme(name string, light, dark, ansi []string) (appearance.CustomTheme, error) {
	return appearance.DuplicateTheme(themesDirectory(), name, light, dark, ansi)
}

// RevealCustomTheme shows a custom theme's folder in Finder or the file manager.
func (a *App) RevealCustomTheme(id string) error {
	folder, err := appearance.CustomThemeFolder(themesDirectory(), id)
	if err != nil {
		return err
	}
	return appearance.RevealTheme(folder)
}

func (a *App) beforeClose(context.Context) bool {
	if a.quitConfirmed.Load() || !a.frontendReady.Load() || a.ready() != nil {
		return false
	}
	runtime.EventsEmit(a.ctx, "quit-request", a.store.Snapshot())
	return true
}
func (a *App) ConfirmQuit() {
	a.quitConfirmed.Store(true)
	runtime.Quit(a.ctx)
}
func (a *App) domReady(context.Context) {
	a.frontendReady.Store(true)
	if a.ready() != nil {
		return
	}
	a.applyAppIcon(a.store.Snapshot().Preferences.AppIcon)
	a.applyAppearance(a.store.Snapshot().Preferences.Appearance)
}

// applyAppearance sets the native window chrome. With "system" the window follows the OS, and so
// does the webview's prefers-color-scheme.
func (a *App) applyAppearance(mode string) {
	switch mode {
	case "system":
		runtime.WindowSetSystemDefaultTheme(a.ctx)
	case "light":
		runtime.WindowSetLightTheme(a.ctx)
	default:
		runtime.WindowSetDarkTheme(a.ctx)
	}
	appearance.SetWindowAppearance(mode)
}

func (a *App) applyAppIcon(choice string) {
	if goruntime.GOOS != "darwin" {
		return
	}
	data := icon
	if choice == "black" {
		data = blackAppIcon
	}
	if !appearance.SetAppIcon(data) {
		a.log.Error("app icon could not be decoded", "icon", choice)
	}
}

// SessionDiff reads what the session changed in its working folder. It only reads Git.
func (a *App) SessionDiff(id string) (core.SessionDiff, error) {
	if err := a.ready(); err != nil {
		return core.SessionDiff{}, err
	}
	return a.store.SessionDiff(id)
}
