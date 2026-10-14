package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// Profile is a named set of agent logins. Sessions and projects refer to it by ID; an
// empty ID is the Default profile, which uses each CLI's own config folder.
type Profile struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Emoji string `json:"emoji,omitempty"`
	// Folders maps an agent ID to its config folder. A leading "~" is the home folder, and
	// an agent without a folder uses its default one.
	Folders map[string]string `json:"folders,omitempty"`
}

// agentConfigEnv names the variable each agent's CLI reads its config folder from: settings,
// conversations and login. Agents without one cannot have a per-profile folder yet.
var agentConfigEnv = map[string]string{"claude": "CLAUDE_CONFIG_DIR", "codex": "CODEX_HOME"}

// loginCommands sign an agent's CLI in. They run in a terminal the user drives.
var loginCommands = map[string]string{"claude": "claude auth login", "codex": "codex login"}

// loginStatusCommands exit 0 when the CLI is signed in. They only ask the CLI; Orchestra
// never reads credential files.
var loginStatusCommands = map[string]string{"claude": "claude auth status --json", "codex": "codex login status"}

var errProfileRemoved = errors.New("This session’s profile was removed. Orchestra will not resume it with another account. Close the session, or start a new one.")

// expandFolder resolves a leading "~" and requires an absolute path.
func expandFolder(folder string) (string, error) {
	if folder == "~" || strings.HasPrefix(folder, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		folder = filepath.Join(home, folder[1:])
	}
	if len(folder) > 1024 || strings.IndexFunc(folder, unicode.IsControl) >= 0 || !filepath.IsAbs(folder) {
		return "", errors.New("Choose a config folder with a full path, such as ~/.claude-work.")
	}
	return filepath.Clean(folder), nil
}

func validProfile(p *Profile) error {
	if p == nil {
		return errors.New("Invalid profile.")
	}
	if strings.TrimSpace(p.Name) == "" || len(p.Name) > 200 {
		return errors.New("Enter a profile name.")
	}
	if !validProjectEmoji(p.Emoji) {
		return errors.New("Choose a single emoji.")
	}
	for agent, folder := range p.Folders {
		if _, ok := agentConfigEnv[agent]; !ok {
			return errors.New(agentNames[agent] + " has no config folder setting, so profiles cannot choose its account.")
		}
		if _, err := expandFolder(folder); err != nil {
			return err
		}
	}
	return nil
}

// profileFolders returns the config-folder variables a profile sets, without creating folders.
func profileFolders(p *Profile) map[string]string {
	env := map[string]string{}
	if p == nil {
		return env
	}
	for agent, folder := range p.Folders {
		if path, err := expandFolder(folder); err == nil {
			env[agentConfigEnv[agent]] = path
		}
	}
	return env
}

// profileEnvironment returns the config-folder variables a profile sets and creates missing
// folders as private ones. It never copies or reads files inside them.
func profileEnvironment(p *Profile) (map[string]string, error) {
	env := profileFolders(p)
	for _, folder := range env {
		if err := os.MkdirAll(folder, 0700); err != nil {
			return nil, err
		}
	}
	return env, nil
}

func (s *Store) profileLocked(id string) (*Profile, error) {
	if id == "" {
		return nil, nil
	}
	for _, p := range s.state.Profiles {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, errors.New("Profile unavailable.")
}

// Profile returns a copy of the profile with id; an empty ID is Default and returns nil.
func (s *Store) Profile(id string) (*Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.profileLocked(id)
	if p == nil || err != nil {
		return nil, err
	}
	return copyProfile(p), nil
}

// Profiles returns copies of every profile except Default.
func (s *Store) Profiles() []*Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	profiles := []*Profile{}
	for _, p := range s.state.Profiles {
		profiles = append(profiles, copyProfile(p))
	}
	return profiles
}

func copyProfile(p *Profile) *Profile {
	copied := *p
	copied.Folders = map[string]string{}
	for agent, folder := range p.Folders {
		copied.Folders[agent] = folder
	}
	return &copied
}

// profileFolderLocked names a new profile's folder after it, such as profiles/work. A folder
// that exists, even one a removed profile left behind, is never reused.
func (s *Store) profileFolderLocked(name string) string {
	slug := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(slug) > 40 {
		slug = strings.TrimRight(slug[:40], "-")
	}
	if slug == "" {
		slug = "profile"
	}
	root := filepath.Join(filepath.Dir(s.path), "profiles")
	for n := 1; ; n++ {
		folder := filepath.Join(root, slug)
		if n > 1 {
			folder += "-" + strconv.Itoa(n)
		}
		taken := false
		for _, p := range s.state.Profiles {
			for _, f := range profileFolders(p) {
				taken = taken || strings.HasPrefix(f+"/", folder+"/")
			}
		}
		if _, err := os.Lstat(folder); !taken && os.IsNotExist(err) {
			return folder
		}
	}
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// homeRelative shows a folder inside the home folder as ~/…, which expandFolder resolves.
func homeRelative(folder string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if rest, ok := strings.CutPrefix(folder, home+"/"); ok {
			return "~/" + rest
		}
	}
	return folder
}

// CreateProfile adds a profile with its own config folders inside the workspace folder. Orchestra
// manages them, so the user never chooses a folder; they start empty.
func (s *Store) CreateProfile(name, emoji string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	root := s.profileFolderLocked(name)
	folders := map[string]string{}
	for agent := range agentConfigEnv {
		folders[agent] = homeRelative(filepath.Join(root, agent))
	}
	p := &Profile{ID: uuid.NewString(), Name: strings.TrimSpace(name), Emoji: strings.TrimSpace(emoji), Folders: folders}
	if err := validProfile(p); err != nil {
		return State{}, err
	}
	s.state.Profiles = append(s.state.Profiles, p)
	if err := s.saveLocked(); err != nil {
		s.state.Profiles = s.state.Profiles[:len(s.state.Profiles)-1]
		return State{}, err
	}
	s.changedLocked()
	return s.snapshotLocked(), nil
}

// EditProfile changes a profile's name and emoji. Its config folders never change, because
// sessions resume their conversations from them.
func (s *Store) EditProfile(id, name, emoji string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.profileLocked(id)
	if err != nil || p == nil {
		return State{}, errors.New("Profile unavailable.")
	}
	edited := Profile{ID: id, Name: strings.TrimSpace(name), Emoji: strings.TrimSpace(emoji), Folders: p.Folders}
	if err = validProfile(&edited); err != nil {
		return State{}, err
	}
	previous := *p
	*p = edited
	if err = s.saveLocked(); err != nil {
		*p = previous
		return State{}, err
	}
	s.changedLocked()
	return s.snapshotLocked(), nil
}

// RemoveProfile forgets a profile and moves its projects back to Default. Its folders stay on
// disk, and sessions that used it keep its ID so they fail to resume instead of switching account.
func (s *Store) RemoveProfile(id string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, err := s.profileLocked(id); err != nil || p == nil {
		return State{}, errors.New("Profile unavailable.")
	}
	previous := s.state.Profiles
	moved := []*Project{}
	s.state.Profiles = []*Profile{}
	for _, p := range previous {
		if p.ID != id {
			s.state.Profiles = append(s.state.Profiles, p)
		}
	}
	for _, p := range s.state.Projects {
		if p.ProfileID == id {
			p.ProfileID = ""
			moved = append(moved, p)
		}
	}
	if err := s.saveLocked(); err != nil {
		s.state.Profiles = previous
		for _, p := range moved {
			p.ProfileID = id
		}
		return State{}, err
	}
	s.changedLocked()
	return s.snapshotLocked(), nil
}

// SetProjectProfile chooses the profile new sessions in the project use.
func (s *Store) SetProjectProfile(projectID, profileID string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.projectLocked(projectID)
	if err != nil {
		return State{}, err
	}
	if _, err = s.profileLocked(profileID); err != nil {
		return State{}, err
	}
	previous := p.ProfileID
	p.ProfileID = profileID
	if err = s.saveLocked(); err != nil {
		p.ProfileID = previous
		return State{}, err
	}
	s.changedLocked()
	return s.snapshotLocked(), nil
}

// LogIn adds a terminal session in the profile that runs the agent's login command, so the
// user signs in themselves. Orchestra never handles the credentials.
func (s *Store) LogIn(projectID, profileID, agent string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if loginCommands[agent] == "" {
		return State{}, errors.New(agentNames[agent] + " cannot sign in from Orchestra.")
	}
	profile, err := s.profileLocked(profileID)
	if err != nil {
		return State{}, err
	}
	name := "Default"
	if profile != nil {
		name = profile.Name
	}
	return s.createSessionLocked(projectID, terminalAgent, "Log In · "+agentNames[agent]+" · "+name, "", false, nil, "", "", profileID, agent)
}

// ProfileLogin reports whether an agent's CLI is signed in with a profile's config folder.
type ProfileLogin struct {
	Agent string `json:"agent"`
	// Folder is the expanded config folder; empty means the CLI's default folder.
	Folder   string `json:"folder"`
	LoggedIn bool   `json:"loggedIn"`
	// Skill reports whether the agent finds the Orchestra skill with this profile.
	Skill   bool   `json:"skill"`
	Message string `json:"message,omitempty"`
}

// ProfileLogins asks each configurable agent's CLI whether it is signed in for the profile. A
// folder that does not exist yet is reported as signed out without running anything.
func ProfileLogins(ctx context.Context, p *Profile) []ProfileLogin {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	results := []ProfileLogin{}
	for _, agent := range skillAgentOrder() {
		if agentConfigEnv[agent] != "" {
			results = append(results, ProfileLogin{Agent: agent})
		}
	}
	env := profileFolders(p)
	var wg sync.WaitGroup
	for i := range results {
		result := &results[i]
		variable := agentConfigEnv[result.Agent]
		result.Folder = env[variable]
		result.Skill = profileSkillInstalled(p, result.Agent)
		script := "exec " + loginStatusCommands[result.Agent]
		if result.Folder != "" {
			if info, err := os.Stat(result.Folder); err != nil || !info.IsDir() {
				continue
			}
			script = "export " + variable + "=" + shellQuote(result.Folder) + "; " + script
		}
		wg.Go(func() {
			_, code, err := cliShell(ctx, script)
			switch {
			case err != nil:
				result.Message = "Could not check the sign-in."
			case code == 127:
				result.Message = "Install the " + agentExecutables[result.Agent] + " CLI to sign in."
			default:
				result.LoggedIn = code == 0
			}
		})
	}
	wg.Wait()
	return results
}
