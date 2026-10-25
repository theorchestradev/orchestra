package core

import (
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func profileFixture(t *testing.T, s *Store, name string) *Profile {
	t.Helper()
	state, err := s.CreateProfile(name, "")
	if err != nil {
		t.Fatal(err)
	}
	return state.Profiles[len(state.Profiles)-1]
}

// fakeAgents puts a login shell and agent CLIs that print their config folders on PATH.
func fakeAgents(t *testing.T) {
	t.Helper()
	fixture := t.TempDir()
	shell := filepath.Join(fixture, "shell")
	_ = os.WriteFile(shell, []byte("#!/bin/sh\nexec /bin/sh -c \"$4\"\n"), 0700)
	for _, agent := range []string{"claude", "codex"} {
		script := "#!/bin/sh\nprintf '" + agent + " args=%s claude=%s codex=%s\\r\\n' \"$*\" \"$CLAUDE_CONFIG_DIR\" \"$CODEX_HOME\"\nwhile IFS= read -r line; do :; done\n"
		_ = os.WriteFile(filepath.Join(fixture, agent), []byte(script), 0700)
	}
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", fixture+":"+os.Getenv("PATH"))
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
}

func terminalText(s *Store, id string) string {
	output, _ := s.TerminalSnapshot(id)
	data, _ := base64.StdEncoding.DecodeString(output.Data)
	return string(data)
}

func TestProfilesChooseEachSessionsConfigFolders(t *testing.T) {
	fakeAgents(t)
	s := fixtureStore(t)
	p := projectFixture(t, s)
	work := profileFixture(t, s, "Work")
	workClaude := filepath.Join(filepath.Dir(s.path), "profiles", "work", "claude")
	if work.Folders["claude"] != workClaude || work.Folders["codex"] != filepath.Join(filepath.Dir(s.path), "profiles", "work", "codex") {
		t.Fatalf("new profile folders = %+v", work.Folders)
	}
	if _, err := s.SetProjectProfile(p.ID, work.ID); err != nil {
		t.Fatal(err)
	}
	personal := sessionFixture(t, s, p, "claude")
	state, err := s.CreateSession(p.ID, "", "claude", "", "", false, nil, "", "", work.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	working := state.Projects[0].Sessions[1]
	if personal.ProfileID != "" || working.ProfileID != work.ID {
		t.Fatalf("session profiles = %q, %q", personal.ProfileID, working.ProfileID)
	}
	if _, err = s.CreateSession(p.ID, "", "claude", "", "", false, nil, "", "", "missing", "", ""); err == nil {
		t.Fatal("unknown profile accepted")
	}
	for _, id := range []string{personal.ID, working.ID} {
		if _, err = s.StartSession(id, false, 80, 24); err != nil {
			t.Fatal(err)
		}
	}
	waitUntil(t, func() bool { return strings.Contains(terminalText(s, personal.ID), "claude=") })
	waitUntil(t, func() bool { return strings.Contains(terminalText(s, working.ID), "claude="+workClaude+" codex=") })
	if strings.Contains(terminalText(s, personal.ID), "claude=/") {
		t.Fatalf("Default session got a config folder: %q", terminalText(s, personal.ID))
	}
	if info, err := os.Stat(workClaude); err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("profile folder was not created privately: %v %v", info, err)
	}

	// Restarting resumes each session with its own profile.
	reopened, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer shutdownAndWait(t, reopened)
	restored := reopened.Snapshot()
	if restored.Projects[0].ProfileID != work.ID || restored.Projects[0].Sessions[1].ProfileID != work.ID || len(restored.Profiles) != 1 {
		t.Fatalf("profiles were not restored: %+v", restored)
	}
	if _, err = reopened.StartSession(working.ID, false, 80, 24); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool {
		text := terminalText(reopened, working.ID)
		return strings.Contains(text, "--resume") && strings.Contains(text, "claude="+workClaude)
	})

	// Removing a profile keeps its folders, moves projects to Default and blocks resuming.
	state, err = reopened.RemoveProfile(work.ID)
	if err != nil || len(state.Profiles) != 0 || state.Projects[0].ProfileID != "" || state.Projects[0].Sessions[1].ProfileID != work.ID {
		t.Fatalf("remove profile: %+v %v", state, err)
	}
	if _, err = os.Stat(workClaude); err != nil {
		t.Fatal("removing a profile deleted its folder")
	}
	reopened.mu.Lock()
	reopened.terminals[working.ID].terminate()
	reopened.mu.Unlock()
	if _, err = reopened.StartSession(working.ID, false, 80, 24); err != errProfileRemoved {
		t.Fatalf("resume with removed profile = %v", err)
	}
}

func TestNewProfileFoldersAreNamedAfterTheProfile(t *testing.T) {
	s := fixtureStore(t)
	root := filepath.Join(filepath.Dir(s.path), "profiles")
	if err := os.MkdirAll(filepath.Join(root, "client-x"), 0700); err != nil { // Left by a removed profile.
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, folder string }{{"Work", "work"}, {"Work", "work-2"}, {" Client X! ", "client-x-2"}, {"💼", "profile"}} {
		if p := profileFixture(t, s, tc.name); p.Folders["codex"] != filepath.Join(root, tc.folder, "codex") {
			t.Fatalf("%q folder = %q, want %s", tc.name, p.Folders["codex"], tc.folder)
		}
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := homeRelative(filepath.Join(home, ".orchestra", "profiles", "work", "claude")); got != "~/.orchestra/profiles/work/claude" {
		t.Fatalf("home-relative folder = %q", got)
	}
}

func TestEditProfileKeepsItsFolders(t *testing.T) {
	s := fixtureStore(t)
	work := profileFixture(t, s, "Work")
	state, err := s.EditProfile(work.ID, " Work Account ", "💼")
	if err != nil {
		t.Fatal(err)
	}
	edited := state.Profiles[0]
	if edited.Name != "Work Account" || edited.Emoji != "💼" || edited.Folders["claude"] != work.Folders["claude"] || edited.Folders["codex"] != work.Folders["codex"] {
		t.Fatalf("edited profile = %+v", edited)
	}
	if _, err = s.EditProfile(work.ID, " ", ""); err == nil {
		t.Fatal("empty name accepted")
	}
	if _, err = s.CreateProfile("Two", "ab"); err == nil {
		t.Fatal("two emoji accepted")
	}
	if _, err = s.EditProfile("", "Default", ""); err == nil {
		t.Fatal("Default profile was edited")
	}
	if _, err = s.RemoveProfile(""); err == nil {
		t.Fatal("Default profile was removed")
	}
}

func TestSavedProfileFolders(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if env := profileFolders(&Profile{Folders: map[string]string{"claude": "~/.orchestra/profiles/work/claude"}}); env["CLAUDE_CONFIG_DIR"] != filepath.Join(home, ".orchestra", "profiles", "work", "claude") || env["CODEX_HOME"] != "" {
		t.Fatalf("profile environment = %v", env)
	}
	for _, folders := range []map[string]string{{"claude": "relative"}, {"pi": "/tmp/pi"}, {"codex": "/tmp/a\nb"}} {
		if validProfile(&Profile{Name: "Work", Folders: folders}) == nil {
			t.Fatalf("invalid folders accepted: %v", folders)
		}
	}
}

func TestLogInRunsTheLoginCommandInATerminal(t *testing.T) {
	fakeAgents(t)
	s := fixtureStore(t)
	p := projectFixture(t, s)
	work := profileFixture(t, s, "Work")
	if _, err := s.LogIn(p.ID, work.ID, "pi"); err == nil {
		t.Fatal("agent without a login command accepted")
	}
	state, err := s.LogIn(p.ID, work.ID, "codex")
	if err != nil {
		t.Fatal(err)
	}
	login := state.Projects[0].Sessions[0]
	if login.Agent != terminalAgent || login.ProfileID != work.ID || login.LoginAgent != "codex" || login.Name != "Log In · Codex · Work" {
		t.Fatalf("login session = %+v", login)
	}
	command, err := launchCommand(login, "")
	if err != nil || command != "codex login" {
		t.Fatalf("login command = %q %v", command, err)
	}
	if _, err = s.StartSession(login.ID, false, 80, 24); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool {
		return strings.Contains(terminalText(s, login.ID), "codex args=login claude="+filepath.Join(filepath.Dir(s.path), "profiles", "work", "claude"))
	})
	if sess := s.Snapshot().Projects[0].Sessions[0]; sess.LoginAgent != "" {
		t.Fatal("the login command would run again")
	}
}

// A real zsh reads the user's rc files before the session's command, and they may export
// another config folder or wrap the agent in a function. The profile still wins.
func TestProfileFoldersOutliveTheUsersRCFiles(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is not installed")
	}
	rc := t.TempDir()
	// System rc files are skipped: some run compinit, which can stop to ask about folder permissions.
	if err = os.WriteFile(filepath.Join(rc, ".zshenv"), []byte("setopt no_global_rcs\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(rc, ".zshrc"), []byte("export CLAUDE_CONFIG_DIR=/from-rc\nclaude() { echo from-function; }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", zsh)
	t.Setenv("ZDOTDIR", rc)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	s := fixtureStore(t)
	// The runtime folder leads PATH after the rc files, so the fake CLI wins over any real one.
	script := "#!/bin/sh\nprintf 'claude=%s\\r\\n' \"$CLAUDE_CONFIG_DIR\"\nwhile IFS= read -r line; do :; done\n"
	if err = os.WriteFile(filepath.Join(s.runtimeDir, "claude"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	p := projectFixture(t, s)
	work := profileFixture(t, s, "Work")
	state, err := s.CreateSession(p.ID, "", "claude", "", "", false, nil, "", "", work.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	sess := state.Projects[0].Sessions[0]
	if _, err = s.StartSession(sess.ID, false, 80, 24); err != nil {
		t.Fatal(err)
	}
	text := ""
	for until := time.Now().Add(10 * time.Second); !strings.Contains(text, "claude=") && time.Now().Before(until); {
		time.Sleep(20 * time.Millisecond)
		text = terminalText(s, sess.ID)
	}
	if !strings.Contains(text, "claude="+work.Folders["claude"]+"\r") {
		t.Fatalf("session output = %q", text)
	}
}

func TestProfileLoginsAskTheCLI(t *testing.T) {
	folder := t.TempDir()
	scripts := fakeSkillShell(t, func(script string) ([]byte, int, error) {
		if strings.Contains(script, "codex") {
			return nil, 0, nil
		}
		return []byte(`{"loggedIn": false}`), 1, nil
	})
	profile := &Profile{ID: "p", Name: "Work", Folders: map[string]string{"codex": folder, "claude": filepath.Join(folder, "missing")}}
	logins := ProfileLogins(context.Background(), profile)
	if len(logins) != 2 || logins[0].Agent != "codex" || !logins[0].LoggedIn || logins[0].Folder != folder || logins[1].LoggedIn {
		t.Fatalf("logins = %+v", logins)
	}
	// A folder that does not exist yet is signed out without running the CLI.
	if len(*scripts) != 1 || (*scripts)[0] != "export CODEX_HOME="+shellQuote(folder)+"; exec codex login status" {
		t.Fatalf("scripts = %q", *scripts)
	}
	if _, err := os.Stat(filepath.Join(folder, "missing")); !os.IsNotExist(err) {
		t.Fatal("checking the sign-in created a folder")
	}
	*scripts = nil
	logins = ProfileLogins(context.Background(), nil)
	if len(*scripts) != 2 || logins[0].LoggedIn != true || logins[1].LoggedIn || logins[1].Folder != "" {
		t.Fatalf("Default logins = %+v, %q", logins, *scripts)
	}
	for _, script := range *scripts {
		if strings.Contains(script, "export") {
			t.Fatalf("Default profile set a config folder: %q", script)
		}
	}
}

func TestInstallAgentSkillIntoProfiles(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "claude-work")
	scripts := fakeSkillShell(t, func(string) ([]byte, int, error) { return []byte("installed"), 0, nil })
	profiles := []*Profile{{ID: "a", Name: "Work", Folders: map[string]string{"claude": folder}}, {ID: "b", Name: "Codex only", Folders: map[string]string{"codex": "/unused"}}}
	result, err := InstallAgentSkill(context.Background(), []string{"claude", "pi"}, profiles)
	if err != nil || result.ExitCode != 0 {
		t.Fatal(result, err)
	}
	if len(*scripts) != 2 || strings.Contains((*scripts)[0], "export") || !strings.Contains((*scripts)[1], "export CLAUDE_CONFIG_DIR="+shellQuote(folder)+"; exec ") || !strings.HasSuffix((*scripts)[1], "--agent claude-code") {
		t.Fatalf("scripts = %q", *scripts)
	}
	if !strings.Contains(result.Output, "Profile Work:\ninstalled") {
		t.Fatalf("output = %q", result.Output)
	}
	if info, err := os.Stat(folder); err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("profile folder was not created privately")
	}
}

func TestUsageNeedsAProfileFolder(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	profile := &Profile{ID: "p", Name: "Work", Folders: map[string]string{"claude": filepath.Join(t.TempDir(), "missing")}}
	results := ReadAgentUsage(context.Background(), profile)
	if results[1].Agent != "claude" || results[1].Message != "Log in to this profile to see subscription limits." {
		t.Fatalf("usage = %+v", results)
	}
}

func TestSavedProfilesAreValidated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	data := `{"version":1,"projects":[],"profiles":[{"id":"not-a-uuid","name":"Work"}]}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(path, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer shutdownAndWait(t, s)
	if s.Snapshot().Error == "" {
		t.Fatal("invalid saved profile accepted")
	}
}

func TestProfileSkillInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	installed := func(folder string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(folder, "skills", "orchestra"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(folder, "skills", "orchestra", "SKILL.md"), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	work := &Profile{ID: "p", Name: "Work", Folders: map[string]string{"claude": "~/.orchestra/profiles/work/claude", "codex": "~/.orchestra/profiles/work/codex"}}
	installed(filepath.Join(home, ".claude"))
	installed(filepath.Join(home, ".agents")) // Codex skills are shared by every profile.
	if !profileSkillInstalled(nil, "claude") || !profileSkillInstalled(nil, "codex") || profileSkillInstalled(work, "claude") || !profileSkillInstalled(work, "codex") {
		t.Fatal("skill detection")
	}
	scripts := fakeSkillShell(t, func(string) ([]byte, int, error) { return []byte("installed"), 0, nil })
	if _, err := InstallProfileSkill(context.Background(), work); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(home, ".orchestra", "profiles", "work", "claude")
	if len(*scripts) != 1 || !strings.Contains((*scripts)[0], "export CLAUDE_CONFIG_DIR="+shellQuote(folder)+"; exec ") || !strings.HasSuffix((*scripts)[0], "--agent claude-code") {
		t.Fatalf("scripts = %q", *scripts)
	}
	// Nothing runs when every agent already has the skill.
	*scripts = nil
	if _, err := InstallProfileSkill(context.Background(), nil); err != nil || len(*scripts) != 0 {
		t.Fatalf("Default install ran %q, %v", *scripts, err)
	}
	if err := os.RemoveAll(filepath.Join(home, ".agents")); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallProfileSkill(context.Background(), work); err != nil || len(*scripts) != 2 || strings.Contains((*scripts)[0], "export") || !strings.HasSuffix((*scripts)[0], "--agent codex") {
		t.Fatalf("shared Codex install = %q, %v", *scripts, err)
	}
}
