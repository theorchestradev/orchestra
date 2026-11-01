package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/theorchestradev/orchestra/internal/sessionhost"
)

type Session struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Agent          string `json:"agent"`
	AgentSessionID string `json:"agentSessionID,omitempty"`
	HasLaunched    bool   `json:"hasLaunched"`
	InitialPrompt  string `json:"initialPrompt,omitempty"`
	// Model and Effort are passed to the agent's CLI on every launch; empty uses its defaults.
	Model             string `json:"model,omitempty"`
	Effort            string `json:"effort,omitempty"`
	WorktreeDirectory string `json:"worktreeDirectory,omitempty"`
	WorktreeBranch    string `json:"worktreeBranch,omitempty"`
	WorktreeBase      string `json:"worktreeBase,omitempty"` // Start commit; older sessions have none.
	Activity          string `json:"activity,omitempty"`
	Running           bool   `json:"running,omitempty"`
	Restored          bool   `json:"restored,omitempty"`
	// Background is true while the session host runs the session, so it survives quitting.
	Background bool `json:"background,omitempty"`
	// LaunchID names the session host process last started for the session, so Store never
	// reconnects to a process that belongs to another launch.
	LaunchID         string `json:"launchID,omitempty"`
	ExitMessage      string `json:"exitMessage,omitempty"`
	StatusNeedsSetup bool   `json:"statusNeedsSetup,omitempty"`
	// GitHubItem links a session started from Work on or Review to its issue or pull request.
	GitHubItem *GitHubItem `json:"gitHubItem,omitempty"`
	// ProfileID chooses the agent accounts for the session's whole life; empty is Default.
	ProfileID string `json:"profileID,omitempty"`
	// LoginAgent makes a terminal session run that agent's login command when it first starts.
	LoginAgent string `json:"loginAgent,omitempty"`
	// RepositoryID is the workspace repository the session works in, fixed for its whole life.
	RepositoryID string `json:"repositoryID"`
}

// GitHubItem identifies an issue or pull request in the session's repository.
type GitHubItem struct {
	Number      int  `json:"number"`
	PullRequest bool `json:"pullRequest,omitempty"`
}

// Repository is a local folder that a workspace's sessions can work in. Removing it from the
// workspace never touches the folder.
type Repository struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Directory string `json:"directory"`
	// GitHub is the owner/name of the folder's GitHub remote. DetectRepository sets it and it is
	// never saved.
	GitHub string `json:"gitHub,omitempty"`
}

// Workspace is a workspace: its repositories, sessions and Scratchpad. The UI calls it a workspace;
// the saved state and the CLI keep the workspace name.
type Workspace struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Emoji is one optional grapheme cluster shown before the name and as the Arc Mode space icon.
	Emoji string `json:"emoji,omitempty"`
	// Repositories is never empty; the first one provides the workspace icon.
	Repositories []*Repository `json:"repositories"`
	// LastRepositoryID is the repository New Session chose last.
	LastRepositoryID string `json:"lastRepositoryID,omitempty"`
	// Directory is the single folder of version 1 workspaces, read only to migrate them.
	Directory         string `json:"directory,omitempty"`
	IsExpanded        bool   `json:"isExpanded"`
	SelectedSessionID string `json:"selectedSessionID,omitempty"`
	// ShowsGitHub shows Work Items, ShowsReviews the pull requests to review.
	ShowsGitHub     bool   `json:"showsGitHub"`
	ShowsReviews    bool   `json:"showsReviews,omitempty"`
	ShowsScratchpad bool   `json:"showsScratchpad"`
	HasScratchpad   bool   `json:"hasScratchpad"`
	ScratchpadName  string `json:"scratchpadName,omitempty"`
	ScratchpadIndex int    `json:"scratchpadIndex"`
	ScratchpadText  string `json:"scratchpadText"`
	// ScratchpadRevision increases whenever ScratchpadText changes, so writers can detect
	// edits made since they last read it.
	ScratchpadRevision int `json:"scratchpadRevision"`
	// ScratchpadMode is "read" when the Scratchpad renders its Markdown; empty means Edit.
	ScratchpadMode string `json:"scratchpadMode,omitempty"`
	// ProfileID is the profile new sessions use; empty is Default.
	ProfileID string `json:"profileID,omitempty"`
	// LocalMainNoticeDismissed hides the notice that local main is behind origin.
	LocalMainNoticeDismissed bool       `json:"localMainNoticeDismissed,omitempty"`
	Sessions                 []*Session `json:"sessions"`
}
type Preferences struct {
	TelemetryEnabled    bool   `json:"telemetryEnabled"`
	TelemetryChoiceMade bool   `json:"telemetryChoiceMade"`
	TelemetryInstallID  string `json:"telemetryInstallID,omitempty"`
	ArcMode             bool   `json:"arcMode"`
	Appearance          string `json:"appearance"`
	Theme               string `json:"theme"`
	// TerminalFontFamily is an installed font family for terminals; empty means System Monospace.
	TerminalFontFamily    string  `json:"terminalFontFamily,omitempty"`
	TerminalFontSize      int     `json:"terminalFontSize"`
	TerminalFontSmoothing bool    `json:"terminalFontSmoothing"`
	TerminalPaddingX      int     `json:"terminalPaddingX"`
	TerminalPaddingY      int     `json:"terminalPaddingY"`
	SidebarWidth          float64 `json:"sidebarWidth"`
	DiffPaneWidth         float64 `json:"diffPaneWidth"`
	DiffPaneOpen          bool    `json:"diffPaneOpen"`
	// SidebarTabs is the order of the session sidebar's tabs; empty means Changes, Files, PR.
	SidebarTabs           []string `json:"sidebarTabs,omitempty"`
	DefaultAgent          string   `json:"defaultAgent"`
	StartupView           string   `json:"startupView"`
	ConfirmQuit           bool     `json:"confirmQuit"`
	ConfirmSessionClose   bool     `json:"confirmSessionClose"`
	GitHubRefreshInterval int      `json:"gitHubRefreshInterval"`
	// DeliverRequests lets Store submit queued inbox requests once the recipient agent is free.
	DeliverRequests bool `json:"deliverRequests"`
	// AttentionSound plays a sound when an agent finishes its turn or asks for permission.
	AttentionSound bool `json:"attentionSound"`
	// Translucent shows the desktop through the window with native vibrancy.
	Translucent bool `json:"translucent"`
	// WindowOpacity is the content area's opacity in percent while Translucent is on; the sidebar,
	// Changes pane and status bar scale with it.
	WindowOpacity int `json:"windowOpacity"`
	// UpdateChecksOff turns off the startup and daily update checks; the zero value keeps them on.
	UpdateChecksOff bool `json:"updateChecksOff"`
	// BetaUpdates includes pre-releases in update checks.
	BetaUpdates bool `json:"betaUpdates"`
	// SkippedUpdate is the release version the user chose to skip.
	SkippedUpdate string `json:"skippedUpdate,omitempty"`
	// OpenInApp is the app the session view's Open in button used last; empty means none yet.
	OpenInApp string `json:"openInApp,omitempty"`
	// HideOpenInButton hides the session view's Open in button; the zero value shows it.
	HideOpenInButton bool `json:"hideOpenInButton"`
	// HideStatusBar hides the whole bar; omitted preferences preserve the existing layout.
	HideStatusBar        bool       `json:"hideStatusBar"`
	StatusBarLayout      [][]string `json:"statusBarLayout,omitempty"`
	HiddenStatusBarItems []string   `json:"hiddenStatusBarItems,omitempty"`
	// HideUsageStatus hides the status bar's agent usage; the zero value shows it.
	HideUsageStatus bool `json:"hideUsageStatus"`
	// UsageDetailed shows every limit window in the status bar's usage popover instead of one row per agent.
	UsageDetailed bool `json:"usageDetailed"`
	// KeepLocalMainUpdated fetches origin and fast-forwards local main, when safe, before a
	// session worktree is created.
	KeepLocalMainUpdated bool `json:"keepLocalMainUpdated"`
	// KeepSessionsRunning starts new sessions in the session host, so they keep running
	// after Orchestra quits and reconnect when it opens again.
	KeepSessionsRunning bool `json:"keepSessionsRunning"`
	// KeepAwake keeps the computer from sleeping while a session runs.
	KeepAwake bool `json:"keepAwake"`
}
type State struct {
	Version             int          `json:"version"`
	SelectedWorkspaceID string       `json:"selectedWorkspaceID,omitempty"`
	Workspaces          []*Workspace `json:"workspaces"`
	Profiles            []*Profile   `json:"profiles,omitempty"`
	Preferences         Preferences  `json:"preferences"`
	Error               string       `json:"error,omitempty"`
	Runs                []*Run       `json:"runs,omitempty"`
	// LegacyWorkspaces and LegacySelectedWorkspaceID hold the keys saved before workspaces were
	// named projects in the code, read only to migrate them.
	LegacyWorkspaces          []*Workspace `json:"projects,omitempty"`
	LegacySelectedWorkspaceID string       `json:"selectedProjectID,omitempty"`
}

// Store owns app metadata. TerminalSession owns every live PTY, through the session host
// for sessions that keep running after quitting; Collaboration owns routing.
type Store struct {
	mu            sync.Mutex
	state         State
	path          string
	canSave       bool
	terminals     map[string]*TerminalSession
	collaboration *Collaboration
	runtimeDir    string
	worktreesDir  string
	executable    string
	emit          func(string, any)
	log           *slog.Logger
	track         func(string, map[string]any)
	// closing holds sessions whose worktree CloseSession is removing without the lock.
	closing map[string]bool
	// hostDir is the session host's folder; empty when sessions always stop with the app.
	hostDir string
	host    *sessionhost.Client
	// hostIncompatible is set when an earlier version's host still runs there.
	hostIncompatible bool
	// appLock keeps a second Orchestra from sharing the runtime folder and the host.
	appLock    *os.File
	stopOnQuit bool
	// keepAwake is the process keeping the computer awake while KeepAwake is on and a session runs.
	keepAwake *exec.Cmd
	// keepAwakeUnavailable stops retrying after the system refused to keep the computer awake.
	keepAwakeUnavailable bool
}

// SetTracker connects Store's usage signals to the App-owned analytics client.
func (s *Store) SetTracker(track func(string, map[string]any)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.track = track
}
func (s *Store) trackLocked(event string, props map[string]any) {
	if s.track != nil {
		s.track(event, props)
	}
}

// SetTelemetryConsent owns identity creation. General preference writes cannot supply an identity or consent.
func (s *Store) SetTelemetryConsent(enabled bool) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.state.Preferences
	p := &s.state.Preferences
	p.TelemetryChoiceMade = true
	p.TelemetryEnabled = enabled
	if enabled && p.TelemetryInstallID == "" {
		p.TelemetryInstallID = uuid.NewString()
	}
	if !enabled {
		p.TelemetryInstallID = ""
	}
	if err := s.saveLocked(); err != nil {
		s.state.Preferences = previous
		return State{}, err
	}
	s.changedLocked()
	return s.snapshotLocked(), nil
}

// The workspace moves item by item, with state.json last, so an interrupted migration
// resumes on the next launch.
var stateItems = []string{"collaboration.sqlite", "collaboration.sqlite-journal", "collaboration.sqlite-wal", "collaboration.sqlite-shm", "collaboration.json", "AgentSessions", "state.json"}

// MigrateState moves the workspace from a previous state folder into dir before a Store
// opens it. Worktrees and unknown files stay where they are, and the previous folder is
// removed only when nothing remains in it. It does nothing once dir has a workspace.
func MigrateState(from, dir string) error {
	if _, err := os.Stat(filepath.Join(dir, "state.json")); !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(filepath.Join(from, "state.json")); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	for _, name := range stateItems {
		source, target := filepath.Join(from, name), filepath.Join(dir, name)
		if _, err := os.Lstat(source); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		if _, err := os.Lstat(target); err == nil {
			return fmt.Errorf("Could not move the workspace from %s: %s already exists.", from, target)
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(source, target); err != nil {
			return fmt.Errorf("Could not move the workspace from %s: %w", from, err)
		}
	}
	_ = os.Remove(from) // Fails while worktrees or other files remain.
	return nil
}

// NewStore opens the workspace at path. A nil logger discards records.
// NewStore opens a workspace whose sessions always stop with the app.
func NewStore(path, executable string, emit func(string, any), logger *slog.Logger) (*Store, error) {
	return OpenStore(path, executable, emit, logger, false)
}

// OpenStore opens a workspace. With background, sessions can run in the session host
// beside path, and Store reconnects to the ones still running there.
func OpenStore(path, executable string, emit func(string, any), logger *slog.Logger, background bool) (*Store, error) {
	if emit == nil {
		emit = func(string, any) {}
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	s := &Store{path: path, worktreesDir: filepath.Join(filepath.Dir(path), "Worktrees"), executable: executable, canSave: true, terminals: map[string]*TerminalSession{}, closing: map[string]bool{}, emit: emit, log: logger}
	s.state = State{Version: stateVersion, Workspaces: []*Workspace{}, Preferences: defaultPreferences()}
	migrated := false
	if data, err := os.ReadFile(path); err == nil {
		if err = json.Unmarshal(data, &s.state); err == nil {
			if migrated = s.state.Version == 1 || s.state.LegacyWorkspaces != nil; migrated {
				migrateState(&s.state)
			}
			err = validateState(s.state)
		}
		// Earlier versions cannot read the new format, so the original stays beside it.
		if err == nil && migrated {
			err = atomicWrite(strings.TrimSuffix(path, ".json")+".bkp.json", data, 0600)
		}
		if err != nil {
			s.log.Warn("saved state unreadable", "error", err)
			s.canSave = false
			s.state.Error = "Could not load saved state. The file was left unchanged."
			s.state.Workspaces = []*Workspace{}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	s.restoreLocked()
	if migrated && s.canSave {
		if err := s.saveLocked(); err != nil {
			return nil, err
		}
		s.log.Info("workspaces migrated to repositories", "count", len(s.state.Workspaces))
	}
	var err error
	var live map[string]sessionhost.Info
	if background {
		if live, err = s.openBackgroundLocked(); err != nil {
			s.log.Warn("sessions will stop with the app", "error", err)
		}
	}
	if s.runtimeDir == "" {
		if s.runtimeDir, err = os.MkdirTemp("", "orchestra-"); err != nil {
			return nil, err
		}
		if err = os.Chmod(s.runtimeDir, 0700); err != nil {
			return nil, err
		}
	}
	for name, data := range resources {
		if err = atomicWrite(filepath.Join(s.runtimeDir, name), data, 0600); err != nil {
			return nil, err
		}
	}
	var warning string
	s.collaboration, warning, err = newCollaboration(filepath.Join(filepath.Dir(path), "collaboration.json"), s, live)
	if err != nil {
		s.closeBackgroundLocked()
		if s.hostDir == "" {
			os.RemoveAll(s.runtimeDir)
		}
		return nil, err
	}
	if s.state.Error == "" {
		s.state.Error = warning
	}
	s.reattachLocked(live)
	return s, nil
}

// SetWorktreesDir changes where new session worktrees are created. Existing sessions keep their saved paths.
func (s *Store) SetWorktreesDir(directory string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.worktreesDir = directory
}
func (s *Store) restoreLocked() {
	if s.state.Workspaces == nil {
		s.state.Workspaces = []*Workspace{}
	}
	if s.state.Preferences.Theme == "" {
		s.state.Preferences = defaultPreferences()
	}
	s.state.Preferences = normalizeGeneralPreferences(s.state.Preferences)
	if !s.state.Preferences.TelemetryEnabled || !s.state.Preferences.TelemetryChoiceMade {
		s.state.Preferences.TelemetryEnabled = false
		s.state.Preferences.TelemetryInstallID = ""
	}
	if s.state.Preferences.TelemetryEnabled {
		if _, err := uuid.Parse(s.state.Preferences.TelemetryInstallID); err != nil {
			s.state.Preferences.TelemetryEnabled = false
			s.state.Preferences.TelemetryInstallID = ""
		}
	}
	for _, p := range s.state.Workspaces {
		if p.Sessions == nil {
			p.Sessions = []*Session{}
		}
		if _, err := s.profileLocked(p.ProfileID); err != nil {
			p.ProfileID = ""
		}
		if _, err := s.repositoryLocked(p, p.LastRepositoryID); err != nil {
			p.LastRepositoryID = ""
		}
		p.ScratchpadIndex = max(0, min(p.ScratchpadIndex, len(p.Sessions)))
		if p.ScratchpadMode != "read" {
			p.ScratchpadMode = ""
		}
		for _, sess := range p.Sessions {
			sess.Running = false
			sess.Restored = true
			sess.Activity = "unknown"
			sess.ExitMessage = ""
		}
	}
}
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".orchestra-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func (s *Store) saveLocked() error {
	if !s.canSave {
		return errors.New("Saved state is unreadable; recover it before saving changes.")
	}
	// Persist metadata only, never live process state or terminal scrollback.
	data, err := json.Marshal(s.state)
	if err != nil {
		return err
	}
	var saved State
	if err = json.Unmarshal(data, &saved); err != nil {
		return err
	}
	saved.Runs = nil
	saved.Error = ""
	for _, p := range saved.Workspaces {
		for _, repo := range p.Repositories {
			repo.GitHub = ""
		}
		for _, sess := range p.Sessions {
			sess.Activity = ""
			sess.Running = false
			sess.Restored = false
			sess.Background = false
			sess.ExitMessage = ""
			sess.StatusNeedsSetup = false
		}
	}
	data, err = json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	if err = atomicWrite(s.path, data, 0600); err != nil {
		s.state.Error = err.Error()
		return err
	}
	s.state.Error = ""
	return nil
}
func (s *Store) snapshotLocked() State {
	data, _ := json.Marshal(s.state)
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		s.log.Warn("workspace snapshot failed", "error", err)
	}
	if s.collaboration != nil {
		state.Runs = s.collaboration.historyLocked()
	}
	return state
}
func (s *Store) Snapshot() State { s.mu.Lock(); defer s.mu.Unlock(); return s.snapshotLocked() }
func (s *Store) changedLocked() {
	s.updateKeepAwakeLocked()
	s.emit("state", s.snapshotLocked())
}
func (s *Store) workspaceLocked(id string) (*Workspace, error) {
	for _, p := range s.state.Workspaces {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, errors.New("Workspace unavailable.")
}
func (s *Store) sessionLocked(id string) (*Workspace, *Session, error) {
	for _, p := range s.state.Workspaces {
		for _, sess := range p.Sessions {
			if sess.ID == id {
				return p, sess, nil
			}
		}
	}
	return nil, nil, errors.New("Session unavailable.")
}
func (s *Store) CreateWorkspace(name, directory, description string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		return State{}, errors.New("Enter a workspace name.")
	}
	directory, err := repositoryFolder(nil, directory)
	if err != nil {
		return State{}, err
	}
	repo := &Repository{ID: uuid.NewString(), Name: folderName(directory, name), Directory: directory}
	p := &Workspace{ID: uuid.NewString(), Name: name, Description: strings.TrimSpace(description), Repositories: []*Repository{repo}, IsExpanded: true, Sessions: []*Session{}}
	s.state.Workspaces = append(s.state.Workspaces, p)
	s.state.SelectedWorkspaceID = p.ID
	if err = s.saveLocked(); err != nil {
		return State{}, err
	}
	s.changedLocked()
	return s.snapshotLocked(), nil
}
func (s *Store) RemoveWorkspace(id string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.workspaceLocked(id)
	if err != nil {
		return State{}, err
	}
	previousWorkspaces, previousSelection := s.state.Workspaces, s.state.SelectedWorkspaceID
	s.state.Workspaces = make([]*Workspace, 0, len(previousWorkspaces)-1)
	for _, workspace := range previousWorkspaces {
		if workspace.ID != id {
			s.state.Workspaces = append(s.state.Workspaces, workspace)
		}
	}
	if s.state.SelectedWorkspaceID == id {
		s.state.SelectedWorkspaceID = ""
		if len(s.state.Workspaces) > 0 {
			s.state.SelectedWorkspaceID = s.state.Workspaces[0].ID
		}
	}
	// Save before stopping sessions. A failed workspace write must leave them running.
	if err = s.saveLocked(); err != nil {
		s.state.Workspaces, s.state.SelectedWorkspaceID = previousWorkspaces, previousSelection
		return State{}, err
	}
	for _, sess := range p.Sessions {
		if err = s.collaboration.closeLocked(sess.ID); err != nil {
			s.state.Workspaces, s.state.SelectedWorkspaceID = previousWorkspaces, previousSelection
			return State{}, errors.Join(err, s.saveLocked())
		}
	}
	for _, sess := range p.Sessions {
		if terminal := s.terminals[sess.ID]; terminal != nil {
			if !terminal.stopped {
				s.trackSessionEndLocked(sess, terminal, "killed")
			}
			terminal.terminate()
			delete(s.terminals, sess.ID)
		}
	}
	// Only app metadata and private session resources are removed; never a repository folder.
	s.changedLocked()
	return s.snapshotLocked(), nil
}
func (s *Store) Rename(id, name string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		return State{}, errors.New("Enter a name.")
	}
	if p, err := s.workspaceLocked(id); err == nil {
		p.Name = name
	} else {
		_, sess, err := s.sessionLocked(id)
		if err != nil {
			return State{}, err
		}
		sess.Name = name
	}
	err := s.saveLocked()
	s.changedLocked()
	return s.snapshotLocked(), err
}

// EditWorkspace updates a workspace's name and its optional description and emoji.
func (s *Store) EditWorkspace(id, name, description, emoji string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name, emoji = strings.TrimSpace(name), strings.TrimSpace(emoji)
	if name == "" {
		return State{}, errors.New("Enter a workspace name.")
	}
	if !validWorkspaceEmoji(emoji) {
		return State{}, errors.New("Choose a single emoji.")
	}
	p, err := s.workspaceLocked(id)
	if err != nil {
		return State{}, err
	}
	p.Name, p.Description, p.Emoji = name, strings.TrimSpace(description), emoji
	err = s.saveLocked()
	s.changedLocked()
	return s.snapshotLocked(), err
}
func (s *Store) Select(workspaceID, destination string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.workspaceLocked(workspaceID)
	if err != nil {
		return State{}, err
	}
	if destination != "workspace" && destination != "github" && destination != "reviews" && destination != "scratchpad" {
		_, sess, err := s.sessionLocked(destination)
		if err != nil {
			return State{}, err
		}
		found := false
		for _, x := range p.Sessions {
			found = found || x == sess
		}
		if !found {
			return State{}, errors.New("Session unavailable in this workspace.")
		}
		p.SelectedSessionID = destination
	}
	s.state.SelectedWorkspaceID = workspaceID
	if destination != "workspace" {
		p.ShowsGitHub = destination == "github"
		p.ShowsReviews = destination == "reviews"
		p.ShowsScratchpad = destination == "scratchpad"
	}
	if destination == "scratchpad" {
		p.HasScratchpad = true
		if p.ScratchpadName == "" {
			p.ScratchpadName = "Scratchpad"
		}
		p.IsExpanded = true
	}
	err = s.saveLocked()
	s.changedLocked()
	return s.snapshotLocked(), err
}
func (s *Store) ToggleWorkspace(id string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.workspaceLocked(id)
	if err != nil {
		return State{}, err
	}
	p.IsExpanded = !p.IsExpanded
	err = s.saveLocked()
	s.changedLocked()
	return s.snapshotLocked(), err
}

// MoveWorkspace places a workspace at index in the sidebar order.
func (s *Store) MoveWorkspace(id string, index int) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	from := -1
	for i, p := range s.state.Workspaces {
		if p.ID == id {
			from = i
		}
	}
	if from < 0 {
		return State{}, errors.New("Workspace unavailable.")
	}
	previous := s.state.Workspaces
	s.state.Workspaces = moved(previous, from, index)
	if err := s.saveLocked(); err != nil {
		s.state.Workspaces = previous
		return State{}, err
	}
	s.changedLocked()
	return s.snapshotLocked(), nil
}

// MoveSession places a session at index among its workspace's rows, which include the Scratchpad.
func (s *Store) MoveSession(id string, index int) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, sess, err := s.sessionLocked(id)
	if err != nil {
		return State{}, err
	}
	return s.moveRowLocked(p, sess, index)
}

// MoveScratchpad places the workspace's Scratchpad at index among its sessions.
func (s *Store) MoveScratchpad(workspaceID string, index int) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.workspaceLocked(workspaceID)
	if err != nil {
		return State{}, err
	}
	if !p.HasScratchpad {
		return State{}, errors.New("Scratchpad no longer exists.")
	}
	return s.moveRowLocked(p, nil, index)
}

// moveRowLocked moves a session, or the Scratchpad when sess is nil, to index among the workspace's rows.
func (s *Store) moveRowLocked(p *Workspace, sess *Session, index int) (State, error) {
	rows := append([]*Session{}, p.Sessions...)
	if p.HasScratchpad {
		rows = slices.Insert(rows, max(0, min(p.ScratchpadIndex, len(rows))), nil)
	}
	previous, previousIndex := p.Sessions, p.ScratchpadIndex
	rows = moved(rows, slices.Index(rows, sess), index)
	if p.HasScratchpad {
		p.ScratchpadIndex = slices.Index(rows, nil)
	}
	p.Sessions = slices.DeleteFunc(rows, func(row *Session) bool { return row == nil })
	if err := s.saveLocked(); err != nil {
		p.Sessions, p.ScratchpadIndex = previous, previousIndex
		return State{}, err
	}
	s.changedLocked()
	return s.snapshotLocked(), nil
}

// moved returns a copy of items with items[from] at index, clamped to the list bounds.
func moved[T any](items []T, from, index int) []T {
	index = max(0, min(index, len(items)-1))
	result := append(append([]T{}, items[:from]...), items[from+1:]...)
	return append(result[:index], append([]T{items[from]}, result[index:]...)...)
}

// SaveScratchpad stores the editor's text when it was based on revision, and returns the
// new revision. A stale revision is rejected so edits made through the CLI are not lost.
func (s *Store) SaveScratchpad(id, text string, revision int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.workspaceLocked(id)
	if err != nil {
		return 0, err
	}
	if !p.HasScratchpad {
		return 0, errors.New("Scratchpad no longer exists.")
	}
	if text == p.ScratchpadText {
		return p.ScratchpadRevision, nil
	}
	if revision != p.ScratchpadRevision {
		return 0, errScratchpadChanged
	}
	return s.setScratchpadTextLocked(p, text, false)
}

var errSessionClosing = errors.New("This session is closing.")
var errScratchpadChanged = errors.New("The Scratchpad changed since it was read. Read it again before writing.")

// setScratchpadTextLocked saves new Scratchpad text under a new revision. CLI writes
// notify views, so an open editor shows them.
func (s *Store) setScratchpadTextLocked(p *Workspace, text string, notify bool) (int, error) {
	if text == p.ScratchpadText {
		return p.ScratchpadRevision, nil
	}
	previous, previousRevision := p.ScratchpadText, p.ScratchpadRevision
	p.ScratchpadText = text
	p.ScratchpadRevision++
	if err := s.saveLocked(); err != nil {
		p.ScratchpadText, p.ScratchpadRevision = previous, previousRevision
		return 0, err
	}
	if notify {
		s.changedLocked()
	}
	return p.ScratchpadRevision, nil
}
func (s *Store) RenameScratchpad(id, name string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.workspaceLocked(id)
	if err != nil {
		return State{}, err
	}
	if !p.HasScratchpad {
		return State{}, errors.New("Scratchpad no longer exists.")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return State{}, errors.New("Enter a scratchpad name.")
	}
	p.ScratchpadName = name
	err = s.saveLocked()
	s.changedLocked()
	return s.snapshotLocked(), err
}

// SetScratchpadMode remembers whether the workspace's Scratchpad opens in Edit or Read mode.
func (s *Store) SetScratchpadMode(id, mode string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.workspaceLocked(id)
	if err != nil {
		return State{}, err
	}
	if !p.HasScratchpad {
		return State{}, errors.New("Scratchpad no longer exists.")
	}
	if mode != "edit" && mode != "read" {
		return State{}, errors.New("Unknown Scratchpad mode.")
	}
	if mode == "edit" {
		mode = ""
	}
	if p.ScratchpadMode == mode {
		return s.snapshotLocked(), nil
	}
	p.ScratchpadMode = mode
	err = s.saveLocked()
	if err == nil && mode == "read" {
		s.trackLocked("feature_used", map[string]any{"feature": "scratchpad_read"})
	}
	s.changedLocked()
	return s.snapshotLocked(), err
}
func (s *Store) DeleteScratchpad(id string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.workspaceLocked(id)
	if err != nil {
		return State{}, err
	}
	p.HasScratchpad = false
	p.ShowsScratchpad = false
	p.ScratchpadName = ""
	p.ScratchpadMode = ""
	if p.ScratchpadText != "" {
		p.ScratchpadText = ""
		p.ScratchpadRevision++
	}
	p.ScratchpadIndex = 0
	err = s.saveLocked()
	s.changedLocked()
	return s.snapshotLocked(), err
}

// The frontend clamps the Changes pane to the same range.
const minDiffPaneWidth, maxDiffPaneWidth = 280, 900

// The minimum keeps text readable over busy wallpapers; Settings uses the same range.
const minWindowOpacity, maxWindowOpacity, defaultWindowOpacity = 40, 95, 78

func defaultPreferences() Preferences {
	return Preferences{Appearance: "system", Theme: "system", TerminalFontSize: 14, TerminalFontSmoothing: true, TerminalPaddingX: 12, TerminalPaddingY: 8, SidebarWidth: 280, DiffPaneWidth: 440, DefaultAgent: "claude", StartupView: "last", ConfirmQuit: true, ConfirmSessionClose: true, WindowOpacity: defaultWindowOpacity}
}
func normalizeGeneralPreferences(p Preferences) Preferences {
	// Older workspaces and bridge callers have no general preferences.
	if p.DefaultAgent == "" {
		p.DefaultAgent = "claude"
		p.ConfirmQuit = true
		p.ConfirmSessionClose = true
	}
	if p.StartupView == "" {
		p.StartupView = "last"
	}
	if p.DiffPaneWidth == 0 {
		p.DiffPaneWidth = 440
	}
	if p.WindowOpacity == 0 {
		p.WindowOpacity = defaultWindowOpacity
	}
	p.WindowOpacity = max(minWindowOpacity, min(p.WindowOpacity, maxWindowOpacity))
	return p
}
func (s *Store) SetPreferences(p Preferences) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p = normalizeGeneralPreferences(p)
	if p.Appearance != "system" && p.Appearance != "dark" && p.Appearance != "light" {
		return State{}, errors.New("Invalid appearance.")
	}
	if p.TerminalFontSize < 10 || p.TerminalFontSize > 24 || p.TerminalPaddingX < 0 || p.TerminalPaddingX > 24 || p.TerminalPaddingY < 0 || p.TerminalPaddingY > 24 || p.SidebarWidth < 220 || p.SidebarWidth > 400 || p.DiffPaneWidth < minDiffPaneWidth || p.DiffPaneWidth > maxDiffPaneWidth {
		return State{}, errors.New("Invalid preference value.")
	}
	// The terminal quotes the family in a CSS font-family list.
	if len(p.TerminalFontFamily) > 200 || strings.ContainsAny(p.TerminalFontFamily, "\"\\") || strings.IndexFunc(p.TerminalFontFamily, unicode.IsControl) >= 0 {
		return State{}, errors.New("Invalid terminal font.")
	}
	if _, ok := agentSpecs[p.DefaultAgent]; !ok {
		return State{}, errors.New("Unknown default agent.")
	}
	if p.StartupView != "last" && p.StartupView != "scratchpad" && p.StartupView != "github" && p.StartupView != "reviews" {
		return State{}, errors.New("Invalid startup view.")
	}
	if p.GitHubRefreshInterval != 0 && p.GitHubRefreshInterval != 30 && p.GitHubRefreshInterval != 60 && p.GitHubRefreshInterval != 300 {
		return State{}, errors.New("Invalid GitHub refresh interval.")
	}
	if err := validUpdateVersion(p.SkippedUpdate); err != nil {
		return State{}, err
	}
	// An app id from internal/apps; opening rejects ids it does not know.
	if len(p.OpenInApp) > 40 || strings.Trim(p.OpenInApp, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
		return State{}, errors.New("Unknown app.")
	}
	seen := map[string]bool{}
	for _, tab := range p.SidebarTabs {
		if (tab != "Changes" && tab != "Files" && tab != "PR") || seen[tab] {
			return State{}, errors.New("Invalid sidebar tab order.")
		}
		seen[tab] = true
	}
	if len(p.StatusBarLayout) != 0 && len(p.StatusBarLayout) != 2 {
		return State{}, errors.New("Invalid status bar layout.")
	}
	statusItems := map[string]bool{"workspace": true, "repository": true, "session": true, "profile": true, "activity": true, "branch": true, "exitMessage": true, "view": true, "working": true, "waiting": true, "running": true, "changes": true, "update": true, "inbox": true, "usage": true, "theme": true}
	seenItems := map[string]bool{}
	for _, side := range p.StatusBarLayout {
		for _, item := range side {
			if !statusItems[item] || seenItems[item] {
				return State{}, errors.New("Invalid status bar layout.")
			}
			seenItems[item] = true
		}
	}
	for _, item := range p.HiddenStatusBarItems {
		if !statusItems[item] {
			return State{}, errors.New("Invalid status bar item.")
		}
	}
	previous := s.state.Preferences
	p.TelemetryEnabled = previous.TelemetryEnabled
	p.TelemetryChoiceMade = previous.TelemetryChoiceMade
	p.TelemetryInstallID = previous.TelemetryInstallID
	s.state.Preferences = p
	if err := s.saveLocked(); err != nil {
		s.state.Preferences = previous
		return State{}, err
	}
	s.changedLocked()
	if p.DeliverRequests && !previous.DeliverRequests {
		for _, workspace := range s.state.Workspaces {
			for _, sess := range workspace.Sessions {
				s.deliverLocked(sess.ID)
			}
		}
	}
	return s.snapshotLocked(), nil
}

// maxSessionNameLength bounds names derived from issue and pull request titles.
const maxSessionNameLength = 60

// clampSessionName collapses whitespace and truncates long names on a rune boundary.
func clampSessionName(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	if runes := []rune(name); len(runes) > maxSessionNameLength {
		name = strings.TrimSpace(string(runes[:maxSessionNameLength-1])) + "…"
	}
	return name
}

// CreateSession names the session sessionName when given, otherwise "<Agent name> <n>".
// repositoryID is the workspace repository it works in; empty is the one New Session used last.
// gitHubItem records the issue or pull request in that repository a Work on or Review session came from.
// profileID chooses the session's agent accounts; empty is Default.
// With a worktree, branch names the new session branch, by default orchestra/<session name>,
// and base is where it starts, by default the repository's current commit. It first keeps local main up to date when the preference is on, and
// otherwise emits "local-main-behind" when local main lags origin.
func (s *Store) CreateSession(workspaceID, repositoryID, agent, sessionName, prompt string, createWorktree bool, gitHubItem *GitHubItem, model, effort, profileID, branch, base string) (State, error) {
	s.mu.Lock()
	directory, keepUpdated, notice := "", false, false
	if p, err := s.workspaceLocked(workspaceID); err == nil && createWorktree && agent != terminalAgent {
		if repo, err := s.newSessionRepositoryLocked(p, repositoryID); err == nil {
			repositoryID, directory = repo.ID, repo.Directory
			keepUpdated = s.state.Preferences.KeepLocalMainUpdated
			notice = !keepUpdated && !p.LocalMainNoticeDismissed
		}
	}
	s.mu.Unlock()
	// Fetching can be slow, so it runs outside the lock and never blocks creating the session.
	if keepUpdated {
		if branch, err := updateLocalMain(directory); err != nil {
			s.log.Info("local main not updated", "workspace", workspaceID, "branch", branch, "error", err)
		}
	}
	// ponytail: checkout holds the Store lock; prepare outside it if large repos block navigation.
	s.mu.Lock()
	state, err := s.createSessionLocked(workspaceID, repositoryID, agent, sessionName, prompt, createWorktree, gitHubItem, model, effort, profileID, "", branch, base)
	s.mu.Unlock()
	if err == nil && notice {
		if branch, behind, behindErr := localMainBehind(directory); behindErr == nil && behind > 0 {
			s.emit("local-main-behind", LocalMainBehind{workspaceID, repositoryID, branch, behind})
		}
	}
	return state, err
}

// LocalMainBehind tells the user that local main lags origin by Behind commits.
type LocalMainBehind struct {
	WorkspaceID  string `json:"workspaceID"`
	RepositoryID string `json:"repositoryID"`
	Branch       string `json:"branch"`
	Behind       int    `json:"behind"`
}

// KeepLocalMainUpToDate turns on Keep Local Main Up to Date and updates the repository's local
// main now. The preference stays on when this update is skipped, and the error explains why.
func (s *Store) KeepLocalMainUpToDate(workspaceID, repositoryID string) (State, error) {
	s.mu.Lock()
	p, err := s.workspaceLocked(workspaceID)
	var repo *Repository
	if err == nil {
		repo, err = s.repositoryLocked(p, repositoryID)
	}
	if err != nil {
		s.mu.Unlock()
		return State{}, err
	}
	directory, previous := repo.Directory, s.state.Preferences.KeepLocalMainUpdated
	s.state.Preferences.KeepLocalMainUpdated = true
	if err = s.saveLocked(); err != nil {
		s.state.Preferences.KeepLocalMainUpdated = previous
		s.mu.Unlock()
		return State{}, err
	}
	s.changedLocked()
	s.mu.Unlock()
	if _, err = updateLocalMain(directory); err != nil {
		return State{}, err
	}
	return s.Snapshot(), nil
}

// DismissLocalMainNotice stops the workspace's local-main-behind notice.
func (s *Store) DismissLocalMainNotice(workspaceID string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.workspaceLocked(workspaceID)
	if err != nil {
		return State{}, err
	}
	p.LocalMainNoticeDismissed = true
	if err = s.saveLocked(); err != nil {
		p.LocalMainNoticeDismissed = false
		return State{}, err
	}
	s.changedLocked()
	return s.snapshotLocked(), nil
}
func (s *Store) createSessionLocked(workspaceID, repositoryID, agent, sessionName, prompt string, createWorktree bool, gitHubItem *GitHubItem, model, effort, profileID, loginAgent, branch, base string) (State, error) {
	p, err := s.workspaceLocked(workspaceID)
	if err != nil {
		return State{}, err
	}
	repo, err := s.newSessionRepositoryLocked(p, repositoryID)
	if err != nil {
		return State{}, err
	}
	name, ok := sessionKindName(agent)
	if !ok {
		return State{}, errors.New("Unknown agent.")
	}
	if agent == terminalAgent {
		prompt, createWorktree, gitHubItem = "", false, nil
	}
	if gitHubItem != nil && gitHubItem.Number <= 0 {
		return State{}, errors.New("Invalid GitHub issue or pull request.")
	}
	if err = validateLaunch(agent, model, effort); err != nil {
		return State{}, err
	}
	if _, err = s.profileLocked(profileID); err != nil {
		return State{}, err
	}
	n := 1
	for _, sess := range p.Sessions {
		if sess.Agent == agent {
			n++
		}
	}
	if sessionName = clampSessionName(sessionName); sessionName == "" {
		sessionName = fmt.Sprintf("%s %d", name, n)
	}
	sess := &Session{ID: uuid.NewString(), Name: sessionName, Agent: agent, InitialPrompt: prompt, Model: model, Effort: effort, Activity: "unknown", GitHubItem: gitHubItem, ProfileID: profileID, LoginAgent: loginAgent, RepositoryID: repo.ID}
	if presetsID(agent) {
		sess.AgentSessionID = sess.ID
	}
	worktreeRoot := ""
	if createWorktree {
		if !s.canSave {
			return State{}, errors.New("Repair or import the saved state before creating a worktree.")
		}
		if branch = strings.TrimSpace(branch); branch == "" {
			branch = newSessionBranch(repo.Directory, sessionName, agent)
		} else if err = checkBranchName(repo.Directory, branch); err != nil {
			return State{}, err
		}
		worktreeRoot = worktreeFolder(filepath.Join(s.worktreesDir, repositoryName(repo)), branch)
		sess.WorktreeBranch = branch
		sess.WorktreeDirectory, sess.WorktreeBase, err = createSessionWorktree(repo.Directory, worktreeRoot, branch, strings.TrimSpace(base))
		if err != nil {
			s.log.Warn("worktree create failed", "workspace", p.ID, "session", sess.ID, "error", err)
			return State{}, err
		}
	}
	previousWorkspaceID, previousSessionID, previousRepositoryID := s.state.SelectedWorkspaceID, p.SelectedSessionID, p.LastRepositoryID
	previousExpanded, previousGitHub, previousReviews, previousScratchpad := p.IsExpanded, p.ShowsGitHub, p.ShowsReviews, p.ShowsScratchpad
	p.Sessions = append(p.Sessions, sess)
	if loginAgent == "" {
		p.LastRepositoryID = repo.ID
	}
	p.SelectedSessionID = sess.ID
	p.IsExpanded = true
	p.ShowsGitHub = false
	p.ShowsReviews = false
	p.ShowsScratchpad = false
	s.state.SelectedWorkspaceID = p.ID
	if err = s.saveLocked(); err != nil {
		p.Sessions = p.Sessions[:len(p.Sessions)-1]
		p.SelectedSessionID, s.state.SelectedWorkspaceID, p.LastRepositoryID = previousSessionID, previousWorkspaceID, previousRepositoryID
		p.IsExpanded, p.ShowsGitHub, p.ShowsReviews, p.ShowsScratchpad = previousExpanded, previousGitHub, previousReviews, previousScratchpad
		if worktreeRoot != "" {
			if cleanupErr := removeSessionWorktree(repo.Directory, worktreeRoot, sess.WorktreeBranch); cleanupErr != nil {
				s.log.Warn("worktree remove failed", "workspace", p.ID, "session", sess.ID, "error", cleanupErr)
				return State{}, fmt.Errorf("%w; worktree cleanup failed at %s: %v", err, worktreeRoot, cleanupErr)
			}
		}
		return State{}, err
	}
	s.changedLocked()
	return s.snapshotLocked(), nil
}

// CloseSession keeps the session worktree unless removeWorktree asks to discard a clean checkout.
// Removing a worktree can take seconds, so it runs without the lock. Holding the lock would
// stall every session's output and leave the stopped agent unreaped until it finished.
func (s *Store) CloseSession(id string, removeWorktree bool) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, sess, err := s.sessionLocked(id)
	if err != nil {
		return State{}, err
	}
	if s.closing[id] {
		return State{}, errSessionClosing
	}
	directory := ""
	if _, _, repo, err := s.sessionRepositoryLocked(id); err == nil {
		directory = repo.Directory
	}
	worktreeRoot := ""
	if removeWorktree && sess.WorktreeDirectory != "" {
		if worktreeRoot, err = cleanSessionWorktree(sess.WorktreeDirectory); err != nil {
			s.log.Info("worktree kept", "session", id, "error", err)
			return State{}, err
		}
	}
	if err = s.collaboration.closeLocked(id); err != nil {
		return State{}, err
	}
	var done chan struct{}
	if t := s.terminals[id]; t != nil {
		if !t.stopped {
			s.trackSessionEndLocked(sess, t, "killed")
		}
		t.terminate()
		delete(s.terminals, id)
		done = t.done
	}
	if worktreeRoot != "" {
		branch := sess.WorktreeBranch
		s.closing[id] = true
		s.mu.Unlock()
		// Let the agent finish its shutdown first, so it cannot write into the worktree while Git
		// removes it. terminate sends SIGKILL after a second, so this wait stays short.
		if done != nil {
			select {
			case <-done:
			case <-time.After(3 * time.Second):
			}
		}
		err = discardSessionWorktree(directory, worktreeRoot, branch, s.log)
		s.mu.Lock()
		delete(s.closing, id)
		if err != nil {
			s.log.Warn("worktree remove failed", "session", id, "error", err)
			s.changedLocked()
			return s.snapshotLocked(), err
		}
		// The workspace may have been removed while the lock was released.
		if p, _, err = s.sessionLocked(id); err != nil {
			s.changedLocked()
			return s.snapshotLocked(), nil
		}
	}
	for i, sess := range p.Sessions {
		if sess.ID == id {
			p.Sessions = append(p.Sessions[:i], p.Sessions[i+1:]...)
			if i < p.ScratchpadIndex {
				p.ScratchpadIndex--
			}
			break
		}
	}
	if p.SelectedSessionID == id {
		p.SelectedSessionID = ""
		if len(p.Sessions) > 0 {
			p.SelectedSessionID = p.Sessions[0].ID
		}
	}
	err = s.saveLocked()
	s.changedLocked()
	return s.snapshotLocked(), err
}

// Shutdown stops every session, except those in the session host when the user keeps
// sessions running after quitting. Those are only disconnected.
func (s *Store) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	keep := s.state.Preferences.KeepSessionsRunning && !s.stopOnQuit
	kept := map[string]bool{}
	for id, t := range s.terminals {
		if keep && t.background && !t.stopped {
			// readTerminal ignores the closed connection once the session is no longer listed.
			kept[id] = true
			delete(s.terminals, id)
			continue
		}
		if _, sess, err := s.sessionLocked(id); err == nil && !t.stopped {
			s.trackSessionEndLocked(sess, t, "killed")
		}
		t.terminate()
	}
	if len(kept) > 0 {
		s.log.Info("sessions kept running", "count", len(kept))
	}
	if s.collaboration != nil {
		s.collaboration.stopLocked(kept)
	}
	s.closeBackgroundLocked()
	s.stopKeepAwakeLocked()
	if s.hostDir != "" {
		return
	}
	if err := os.RemoveAll(s.runtimeDir); err != nil {
		s.log.Warn("runtime folder cleanup failed", "error", err)
	}
}
