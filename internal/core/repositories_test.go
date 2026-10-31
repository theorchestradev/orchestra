package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateVersion1Workspaces(t *testing.T) {
	dir := t.TempDir()
	folder := filepath.Join(dir, "app")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	pid, sid := "8a1e3c52-5d0a-4c55-9f8e-3a1f0a5b6c01", "8a1e3c52-5d0a-4c55-9f8e-3a1f0a5b6c02"
	original := `{"version":1,"selectedProjectID":"` + pid + `","projects":[{"id":"` + pid + `","name":"Orchestra","directory":"` + folder + `","isExpanded":true,"scratchpadText":"notes","hasScratchpad":true,"selectedSessionID":"` + sid + `","sessions":[{"id":"` + sid + `","name":"Claude Code 1","agent":"claude","hasLaunched":true,"gitHubItem":{"number":7}}]}],"preferences":{"theme":"system"}}`
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	s, err := NewStore(path, exe, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()
	state := s.Snapshot()
	p := state.Workspaces[0]
	if state.Error != "" || state.Version != stateVersion || p.ID != pid || p.Directory != "" || len(p.Repositories) != 1 || p.ScratchpadText != "notes" {
		t.Fatalf("unexpected migration: %+v", state)
	}
	repo := p.Repositories[0]
	if repo.Name != "app" || repo.Directory != folder || p.Sessions[0].ID != sid || p.Sessions[0].RepositoryID != repo.ID || p.Sessions[0].GitHubItem.Number != 7 {
		t.Fatalf("unexpected repository or session: %+v %+v", repo, p.Sessions[0])
	}
	if backup, err := os.ReadFile(filepath.Join(dir, "state.bkp.json")); err != nil || string(backup) != original {
		t.Fatalf("version 1 state not kept: %v", err)
	}
	var saved State
	data, _ := os.ReadFile(path)
	if err = json.Unmarshal(data, &saved); err != nil || saved.Version != stateVersion || saved.SelectedWorkspaceID != pid || saved.Workspaces[0].Repositories[0].ID != repo.ID || saved.Workspaces[0].Directory != "" {
		t.Fatalf("migration not saved: %s", data)
	}
	if strings.Contains(string(data), `"projects"`) || strings.Contains(string(data), "selectedProjectID") {
		t.Fatalf("project keys still saved: %s", data)
	}
	// A second launch reads version 2 and keeps the same repository.
	again, err := NewStore(path, exe, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Shutdown()
	if got := again.Snapshot().Workspaces[0].Repositories[0].ID; got != repo.ID {
		t.Fatalf("repository ID changed on reload: %s", got)
	}
}

// Version 2 states saved before the code renamed projects to workspaces keep their projects keys.
func TestMigrateProjectKeys(t *testing.T) {
	dir := t.TempDir()
	pid, rid := "8a1e3c52-5d0a-4c55-9f8e-3a1f0a5b6c01", "8a1e3c52-5d0a-4c55-9f8e-3a1f0a5b6c03"
	original := `{"version":2,"selectedProjectID":"` + pid + `","projects":[{"id":"` + pid + `","name":"Orchestra","repositories":[{"id":"` + rid + `","name":"app","directory":"` + dir + `"}],"sessions":[]}],"preferences":{"theme":"system"}}`
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	s, err := NewStore(path, exe, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()
	state := s.Snapshot()
	if state.Error != "" || state.SelectedWorkspaceID != pid || len(state.Workspaces) != 1 || state.Workspaces[0].Repositories[0].ID != rid || state.LegacyWorkspaces != nil {
		t.Fatalf("project keys not migrated: %+v", state)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"workspaces"`) || strings.Contains(string(data), `"projects"`) {
		t.Fatalf("migration not saved: %s", data)
	}
	if backup, err := os.ReadFile(filepath.Join(dir, "state.bkp.json")); err != nil || string(backup) != original {
		t.Fatalf("previous state not kept: %v", err)
	}
}

func TestInvalidRepositoriesAreRejected(t *testing.T) {
	pid, rid, sid := "8a1e3c52-5d0a-4c55-9f8e-3a1f0a5b6c01", "8a1e3c52-5d0a-4c55-9f8e-3a1f0a5b6c03", "8a1e3c52-5d0a-4c55-9f8e-3a1f0a5b6c02"
	session := func(repository string) []*Session {
		return []*Session{{ID: sid, Name: "Claude Code 1", Agent: "claude", RepositoryID: repository}}
	}
	for name, p := range map[string]*Workspace{
		"none":            {ID: pid, Name: "x"},
		"relative":        {ID: pid, Name: "x", Repositories: []*Repository{{ID: rid, Name: "a", Directory: "a"}}},
		"unnamed":         {ID: pid, Name: "x", Repositories: []*Repository{{ID: rid, Directory: "/a"}}},
		"duplicateFolder": {ID: pid, Name: "x", Repositories: []*Repository{{ID: rid, Name: "a", Directory: "/a"}, {ID: sid, Name: "b", Directory: "/a"}}},
		"duplicateID":     {ID: pid, Name: "x", Repositories: []*Repository{{ID: rid, Name: "a", Directory: "/a"}, {ID: rid, Name: "b", Directory: "/b"}}},
		"dangling":        {ID: pid, Name: "x", Repositories: []*Repository{{ID: rid, Name: "a", Directory: "/a"}}, Sessions: session("8a1e3c52-5d0a-4c55-9f8e-3a1f0a5b6c09")},
		"legacyFolder":    {ID: pid, Name: "x", Directory: "/a", Repositories: []*Repository{{ID: rid, Name: "a", Directory: "/a"}}},
	} {
		if validateState(State{Version: stateVersion, Workspaces: []*Workspace{p}}) == nil {
			t.Errorf("%s: invalid workspace accepted", name)
		}
	}
	valid := &Workspace{ID: pid, Name: "x", Repositories: []*Repository{{ID: rid, Name: "a", Directory: "/a"}}, Sessions: session(rid)}
	if err := validateState(State{Version: stateVersion, Workspaces: []*Workspace{valid}}); err != nil {
		t.Fatal(err)
	}
}

func TestSessionsWorkInTheirRepository(t *testing.T) {
	s := fixtureStore(t)
	p := repositoryFixture(t, s)
	site := filepath.Join(t.TempDir(), "site")
	gitWorktreeFixture(t, filepath.Dir(site), "init", "-b", "main", site)
	writeFixture(t, filepath.Join(site, "index.html"), "<h1>site</h1>\n")
	commitFixture(t, site, "Site")
	state, err := s.AddRepository(p.ID, site)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddRepository(p.ID, site); err == nil {
		t.Fatal("added the same folder twice")
	}
	repos := state.Workspaces[0].Repositories
	if len(repos) != 2 || repos[1].Name != "site" || repos[1].Directory != site {
		t.Fatalf("unexpected repositories: %+v", repos)
	}
	app, other := repos[0], repos[1]
	// Without a choice, sessions need one when the workspace has several repositories, except
	// New Session's fallback, which uses the last repository chosen.
	if _, err = s.CanCreateWorktree(p.ID, ""); err == nil {
		t.Fatal("guessed a repository")
	}
	state, err = s.CreateSession(p.ID, other.ID, "claude", "", "", true, nil, "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	sess := state.Workspaces[0].Sessions[0]
	if sess.RepositoryID != other.ID || state.Workspaces[0].LastRepositoryID != other.ID || !strings.Contains(sess.WorktreeDirectory, string(filepath.Separator)+"site"+string(filepath.Separator)) {
		t.Fatalf("session not in its repository: %+v", sess)
	}
	files, err := s.SessionFiles(sess.ID, "")
	if err != nil || len(files.Entries) != 1 || files.Entries[0].Name != "index.html" {
		t.Fatalf("files from the wrong repository: %+v %v", files, err)
	}
	writeFixture(t, filepath.Join(sess.WorktreeDirectory, "about.html"), "about\n")
	if diff, err := s.SessionDiff(sess.ID); err != nil || len(diff.Files) != 1 || diff.Files[0].Path != "about.html" {
		t.Fatalf("diff from the wrong repository: %+v %v", diff, err)
	}
	state, err = s.CreateSession(p.ID, "", "terminal", "", "", false, nil, "", "", "", "", "")
	if err != nil || state.Workspaces[0].Sessions[1].RepositoryID != other.ID {
		t.Fatalf("fallback ignored the last repository: %v", err)
	}
	terminal := state.Workspaces[0].Sessions[1]
	if folder, err := s.SessionFolder(terminal.ID); err != nil || folder != site {
		t.Fatalf("terminal folder: %q %v", folder, err)
	}
	if branches, err := s.WorktreeBranches(p.ID, app.ID); err != nil || branches.Current != "main" {
		t.Fatalf("branches: %+v %v", branches, err)
	}

	// Removing keeps the folder, and waits for the repository's sessions to close.
	if _, err = s.RemoveRepository(p.ID, other.ID); err == nil || !strings.Contains(err.Error(), "Close the 2 sessions in site") {
		t.Fatalf("removed a repository with sessions: %v", err)
	}
	for _, id := range []string{sess.ID, terminal.ID} {
		if _, err = s.CloseSession(id, false); err != nil {
			t.Fatal(err)
		}
	}
	if state, err = s.RemoveRepository(p.ID, other.ID); err != nil || len(state.Workspaces[0].Repositories) != 1 || state.Workspaces[0].LastRepositoryID != "" {
		t.Fatalf("remove: %+v %v", state.Workspaces[0], err)
	}
	if _, err = os.Stat(filepath.Join(site, "index.html")); err != nil {
		t.Fatal("removing the repository touched its folder")
	}
	if _, err = s.RemoveRepository(p.ID, app.ID); err == nil || !strings.Contains(err.Error(), "at least one repository") {
		t.Fatalf("removed the last repository: %v", err)
	}
	// With one repository left, an empty choice names it again.
	if ok, err := s.CanCreateWorktree(p.ID, ""); err != nil || !ok {
		t.Fatalf("single repository: %v %v", ok, err)
	}
}
