package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/theorchestradev/orchestra/internal/appearance"
	"github.com/theorchestradev/orchestra/internal/apps"
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
	ctx               context.Context
	log               *slog.Logger
	store             *core.Store
	updater           *core.Updater
	telemetry         *telemetry.Client
	telemetryMu       sync.Mutex
	startupError      error
	frontendReady     atomic.Bool
	quitConfirmed     atomic.Bool
	usageMu           sync.Mutex
	usageCache        map[string]usageReading // By profile ID; "" is Default.
	openRouterMu      sync.Mutex
	openRouterCache   core.OpenRouterAccount
	openRouterChecked time.Time
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
	a.store, a.startupError = core.OpenStore(path, exe, func(event string, data any) { runtime.EventsEmit(ctx, event, data) }, a.log, true)
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

// QA workspaces use a separate Keychain service instead of the user's key.
func openRouterKeyService() string {
	service := "com.orchestra.openrouter"
	if path := os.Getenv("ORCHESTRA_STATE_FILE"); path != "" {
		service += fmt.Sprintf(".workspace-%x", sha256.Sum256([]byte(path)))
	}
	return service
}
func (a *App) OpenRouterUsage(refresh bool) (core.OpenRouterAccount, error) {
	if err := a.ready(); err != nil {
		return core.OpenRouterAccount{}, err
	}
	a.openRouterMu.Lock()
	defer a.openRouterMu.Unlock()
	if !a.openRouterChecked.IsZero() && time.Since(a.openRouterChecked) < 5*time.Minute && (!refresh || time.Since(a.openRouterChecked) < 15*time.Second) {
		return a.openRouterCache, nil
	}
	key, err := core.OpenRouterKey(openRouterKeyService())
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
	a.openRouterCache, a.openRouterChecked = result, time.Now()
	return result, nil
}
func (a *App) SetOpenRouterKey(key string) (core.OpenRouterAccount, error) {
	if err := a.ready(); err != nil {
		return core.OpenRouterAccount{}, err
	}
	a.openRouterMu.Lock()
	defer a.openRouterMu.Unlock()
	key = strings.TrimSpace(key)
	result, err := core.ReadOpenRouterUsage(a.ctx, key)
	if err != nil {
		return core.OpenRouterAccount{}, err
	}
	if err := core.SaveOpenRouterKey(openRouterKeyService(), key); err != nil {
		return core.OpenRouterAccount{}, err
	}
	a.openRouterCache, a.openRouterChecked = result, time.Now()
	return result, nil
}
func (a *App) RemoveOpenRouterKey() error {
	if err := a.ready(); err != nil {
		return err
	}
	a.openRouterMu.Lock()
	defer a.openRouterMu.Unlock()
	if err := core.RemoveOpenRouterKey(openRouterKeyService()); err != nil {
		return err
	}
	a.openRouterCache, a.openRouterChecked = core.OpenRouterAccount{}, time.Time{}
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
		a.applyAppearance(state.Preferences.Appearance)
		if state.Preferences.AppIcon != previousIcon {
			a.applyAppIcon(state.Preferences.AppIcon)
		}
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

// CanCreateWorktree reports whether new sessions in the project can start in a worktree.
func (a *App) CanCreateWorktree(project string) (bool, error) {
	if err := a.ready(); err != nil {
		return false, err
	}
	return a.store.CanCreateWorktree(project)
}

// KeepLocalMainUpToDate and DismissLocalMainNotice back the notice shown when a new session
// worktree finds local main behind origin.
func (a *App) KeepLocalMainUpToDate(projectID string) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	return a.store.KeepLocalMainUpToDate(projectID)
}
func (a *App) DismissLocalMainNotice(projectID string) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	return a.store.DismissLocalMainNotice(projectID)
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
func (a *App) InitializeGitRepository(project string) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	return a.store.InitializeGitRepository(project)
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

// OpenInApps lists the installed editors, terminals and file manager that can open a session's folder.
func (a *App) OpenInApps() []apps.App { return apps.Installed() }

// OpenSessionIn opens a session's worktree, or its project folder, in an installed app. The folder
// comes from the session, never from the caller.
func (a *App) OpenSessionIn(id, app string) error {
	if err := a.ready(); err != nil {
		return err
	}
	folder, err := a.store.SessionFolder(id)
	if err != nil {
		return err
	}
	return apps.Open(app, folder)
}

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
// ConfirmQuit quits after the Quit dialog. stopSessions also stops the sessions that would
// keep running in the background.
func (a *App) ConfirmQuit(stopSessions bool) {
	if a.store != nil {
		a.store.StopSessionsOnQuit(stopSessions)
	}
	a.quitConfirmed.Store(true)
	runtime.Quit(a.ctx)
}
func (a *App) domReady(context.Context) {
	moveAppMenuItems()
	a.frontendReady.Store(true)
	if a.ready() != nil {
		return
	}
	p := a.store.Snapshot().Preferences
	a.applyAppearance(p.Appearance)
	a.applyAppIcon(p.AppIcon)
}

func (a *App) applyAppIcon(name string) {
	data, err := appIcons.ReadFile("frontend/public/app-icons/" + name + ".png")
	if err != nil {
		a.log.Error("app icon unavailable", "icon", name, "error", err)
		return
	}
	appearance.SetAppIcon(data)
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

// SessionDiff reads what the session changed in its working folder. It only reads Git.
func (a *App) SessionDiff(id string) (core.SessionDiff, error) {
	if err := a.ready(); err != nil {
		return core.SessionDiff{}, err
	}
	return a.store.SessionDiff(id)
}

// SessionPullRequest reads the pull request and checks for the session's branch. It only
// reads Git and GitHub; activity adds the pull request's reviews and comments.
func (a *App) SessionPullRequest(id string, activity bool) (core.PullRequestStatus, error) {
	if err := a.ready(); err != nil {
		return core.PullRequestStatus{}, err
	}
	return a.store.SessionPullRequest(id, activity)
}

// PullRequestCheckFailure describes a failed check as text the user can copy.
func (a *App) PullRequestCheckFailure(id string, jobID int64) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.store.PullRequestCheckFailure(id, jobID)
}

// GitHubPullRequest reads a project's pull request and its checks for the GitHub view.
func (a *App) GitHubPullRequest(project string, number int) (core.PullRequestStatus, error) {
	if err := a.ready(); err != nil {
		return core.PullRequestStatus{}, err
	}
	return a.store.GitHubPullRequest(project, number)
}

// GitHubCheckFailure describes a failed check of a project's pull request as text to copy.
func (a *App) GitHubCheckFailure(project string, jobID int64) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.store.GitHubCheckFailure(project, jobID)
}

// SessionFiles lists a folder or previews a text file inside the session folder.
func (a *App) SessionFiles(id, path string) (core.SessionFiles, error) {
	if err := a.ready(); err != nil {
		return core.SessionFiles{}, err
	}
	return a.store.SessionFiles(id, path)
}

// ProjectIcons maps project IDs to the favicon or app icon found in their folders, as data URLs.
// Projects without one are left out.
func (a *App) ProjectIcons() map[string]string {
	result := map[string]string{}
	if a.ready() != nil {
		return result
	}
	for _, p := range a.store.Snapshot().Projects {
		if icon := core.ProjectIcon(p.Directory); icon != "" {
			result[p.ID] = icon
		}
	}
	return result
}
