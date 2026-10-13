package core

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Saved sessions must keep their original launch and resume location.
func TestExistingWorktreeSessionRestoreLaunchAndPreserve(t *testing.T) {
	s := fixtureStore(t)
	p := projectFixture(t, s)
	ordinary := sessionFixture(t, s, p, "claude")
	worktree := sessionFixture(t, s, p, "claude")
	directory := t.TempDir()
	file := filepath.Join(directory, "keep.txt")
	if err := os.WriteFile(file, []byte("existing session work"), 0600); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	_, saved, err := s.sessionLocked(worktree.ID)
	if err == nil {
		saved.WorktreeDirectory = directory
		saved.WorktreeBranch = "codex/orchestra-claude-existing"
		err = s.saveLocked()
	}
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer shutdownAndWait(t, reopened)
	state := reopened.Snapshot()
	restored := state.Projects[0].Sessions[1]
	if state.Error != "" || !restored.Restored || restored.Running || restored.WorktreeDirectory != directory || restored.WorktreeBranch != "codex/orchestra-claude-existing" {
		t.Fatalf("existing worktree session was not restored: %+v", state)
	}
	if state.Projects[0].Sessions[0].WorktreeDirectory != "" {
		t.Fatal("ordinary session acquired a worktree")
	}
	fixture := t.TempDir()
	if err = os.WriteFile(filepath.Join(fixture, "shell"), []byte("#!/bin/sh\nexec /bin/sh -c \"$4\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(fixture, "claude"), []byte("#!/bin/sh\nprintf 'cwd=%s\\r\\n' \"$(pwd -P)\"\nwhile IFS= read -r line; do :; done\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", filepath.Join(fixture, "shell"))
	t.Setenv("PATH", fixture+":"+os.Getenv("PATH"))
	for _, tc := range []struct{ id, directory string }{{ordinary.ID, p.Directory}, {restored.ID, directory}} {
		if _, err = reopened.StartSession(tc.id, false, 80, 24); err != nil {
			t.Fatal(err)
		}
		resolved, err := filepath.EvalSymlinks(tc.directory)
		if err != nil {
			t.Fatal(err)
		}
		waitUntil(t, func() bool {
			output, _ := reopened.TerminalSnapshot(tc.id)
			data, _ := base64.StdEncoding.DecodeString(output.Data)
			return strings.Contains(string(data), "cwd="+resolved)
		})
	}
	if _, err = reopened.CloseSession(restored.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err = reopened.RemoveProject(p.ID); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "existing session work" {
		t.Fatal("session close/project removal discarded worktree files")
	}
	// Missing saved folders must never launch a process in the original checkout instead.
	if err = os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(directory); err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartSession(worktree.ID, false, 80, 24); err == nil || !strings.Contains(err.Error(), "worktree is no longer available") {
		t.Fatalf("missing worktree did not produce an explicit error: %v", err)
	}
	for _, invalid := range []Session{{WorktreeDirectory: "relative", WorktreeBranch: "branch"}, {WorktreeDirectory: "/tmp/path"}, {WorktreeBranch: "branch"}} {
		state.Projects[0].Sessions[1].WorktreeDirectory = invalid.WorktreeDirectory
		state.Projects[0].Sessions[1].WorktreeBranch = invalid.WorktreeBranch
		if validateState(state) == nil {
			t.Fatal("invalid imported worktree metadata accepted")
		}
	}
}

func gitWorktreeFixture(t *testing.T, directory string, args ...string) string {
	t.Helper()
	output, err := runCommand(directory, "git", args...)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(output))
}

func TestCreateSessionWorktreesAndRollback(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	s := fixtureStore(t)
	p := projectFixture(t, s)
	root := p.Directory
	gitWorktreeFixture(t, root, "init", "-b", "main")
	gitWorktreeFixture(t, root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "Fixture")
	if err := os.Mkdir(filepath.Join(root, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(root, "nested", "tracked.txt")
	if err := os.WriteFile(original, []byte("committed"), 0600); err != nil {
		t.Fatal(err)
	}
	gitWorktreeFixture(t, root, "add", ".")
	gitWorktreeFixture(t, root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "Subfolder")
	if err := os.WriteFile(original, []byte("dirty original"), 0600); err != nil {
		t.Fatal(err)
	}
	p.Directory = filepath.Join(root, "nested")
	s.state.Projects[0].Directory = p.Directory
	before := gitWorktreeFixture(t, root, "status", "--porcelain")
	state, err := s.CreateSession(p.ID, "claude", "", "", true, nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	first := state.Projects[0].Sessions[0]
	expected := filepath.Join(filepath.Dir(s.path), "Worktrees", filepath.Base(root), first.ID[:8], "nested")
	if first.WorktreeDirectory != expected || !strings.HasPrefix(first.WorktreeBranch, "orchestra/claude-") {
		t.Fatalf("unexpected worktree metadata: %+v", first)
	}
	data, err := os.ReadFile(filepath.Join(first.WorktreeDirectory, "tracked.txt"))
	if err != nil || string(data) != "committed" || gitWorktreeFixture(t, first.WorktreeDirectory, "branch", "--show-current") != first.WorktreeBranch {
		t.Fatalf("worktree did not start from the commit on its own branch: %s %v", data, err)
	}
	if gitWorktreeFixture(t, root, "branch", "--show-current") != "main" || gitWorktreeFixture(t, root, "status", "--porcelain") != before {
		t.Fatal("worktree creation changed the original checkout")
	}
	state, err = s.CreateSession(p.ID, "pi", "", "", true, nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	second := state.Projects[0].Sessions[1]
	if first.WorktreeDirectory == second.WorktreeDirectory || first.WorktreeBranch == second.WorktreeBranch {
		t.Fatal("sessions share the same checkout or branch")
	}
	// A failed atomic state rename must restore metadata and remove only the new worktree.
	beforeTrees := gitWorktreeFixture(t, root, "worktree", "list", "--porcelain")
	beforeBranches := gitWorktreeFixture(t, root, "branch", "--list")
	if err = os.Remove(s.path); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(s.path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateSession(p.ID, "claude", "", "", true, nil, "", ""); err == nil {
		t.Fatal("expected failed save")
	}
	after := s.Snapshot().Projects[0]
	if len(after.Sessions) != 2 || after.SelectedSessionID != second.ID || gitWorktreeFixture(t, root, "worktree", "list", "--porcelain") != beforeTrees || gitWorktreeFixture(t, root, "branch", "--list") != beforeBranches {
		t.Fatal("failed creation left metadata, branch or worktree changes behind")
	}
	// Refuse to touch a pre-existing branch or folder when preparing a checkout.
	destination := filepath.Join(t.TempDir(), "checkout")
	if _, _, err = createSessionWorktree(root, destination, first.WorktreeBranch); err == nil {
		t.Fatal("existing branch accepted")
	}
	if err = os.Mkdir(destination, 0700); err != nil {
		t.Fatal(err)
	}
	if _, _, err = createSessionWorktree(root, destination, "orchestra/unused-fixture"); err == nil {
		t.Fatal("existing folder accepted")
	}
	gitWorktreeFixture(t, root, "show-ref", "--verify", "refs/heads/"+first.WorktreeBranch)
}

func TestCreateSessionWorktreeRequiresCommittedProject(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, kind := range []string{"not-git", "no-commit", "uncommitted-subfolder"} {
		t.Run(kind, func(t *testing.T) {
			s := fixtureStore(t)
			p := projectFixture(t, s)
			root := p.Directory
			if kind != "not-git" {
				gitWorktreeFixture(t, root, "init", "-b", "main")
			}
			if kind == "uncommitted-subfolder" {
				gitWorktreeFixture(t, root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "Fixture")
				p.Directory = filepath.Join(root, "uncommitted")
				if err := os.Mkdir(p.Directory, 0700); err != nil {
					t.Fatal(err)
				}
				s.state.Projects[0].Directory = p.Directory
			}
			if _, err := s.CreateSession(p.ID, "pi", "", "", true, nil, "", ""); err == nil || len(s.Snapshot().Projects[0].Sessions) != 0 {
				t.Fatal("invalid worktree creation was accepted or left a session behind")
			}
			if kind == "uncommitted-subfolder" && (strings.Count(gitWorktreeFixture(t, root, "worktree", "list", "--porcelain"), "worktree ") != 1 || gitWorktreeFixture(t, root, "branch", "--list", "orchestra/*") != "") {
				t.Fatal("failed checkout left a worktree or branch behind")
			}
		})
	}
}

// Removing a worktree on close must never discard uncommitted files or unmerged commits.
func TestCloseSessionRemovesWorktree(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	s := fixtureStore(t)
	p := projectFixture(t, s)
	gitWorktreeFixture(t, p.Directory, "init", "-b", "main")
	gitWorktreeFixture(t, p.Directory, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "Fixture")
	if _, err := s.CreateSession(p.ID, "claude", "", "", true, nil, "", ""); err != nil {
		t.Fatal(err)
	}
	state, err := s.CreateSession(p.ID, "pi", "", "", true, nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	merged, unmerged := state.Projects[0].Sessions[0], state.Projects[0].Sessions[1]
	untracked := filepath.Join(merged.WorktreeDirectory, "draft.txt")
	if err = os.WriteFile(untracked, []byte("draft"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CloseSession(merged.ID, true); err == nil || len(s.Snapshot().Projects[0].Sessions) != 2 {
		t.Fatal("dirty worktree was removed or its session closed")
	}
	if _, err = os.Stat(untracked); err != nil {
		t.Fatal("dirty worktree lost files")
	}
	if err = os.Remove(untracked); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CloseSession(merged.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(merged.WorktreeDirectory); !os.IsNotExist(err) || gitWorktreeFixture(t, p.Directory, "branch", "--list", merged.WorktreeBranch) != "" {
		t.Fatal("clean worktree or merged branch was kept")
	}
	gitWorktreeFixture(t, unmerged.WorktreeDirectory, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "Session work")
	if _, err = s.CloseSession(unmerged.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(unmerged.WorktreeDirectory); !os.IsNotExist(err) || len(s.Snapshot().Projects[0].Sessions) != 0 {
		t.Fatal("worktree session was not closed")
	}
	gitWorktreeFixture(t, p.Directory, "show-ref", "--verify", "refs/heads/"+unmerged.WorktreeBranch)
}
