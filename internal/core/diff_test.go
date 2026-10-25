package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func commitFixture(t *testing.T, directory, message string) {
	t.Helper()
	gitWorktreeFixture(t, directory, "add", "-A")
	gitWorktreeFixture(t, directory, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", message)
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func diffFile(t *testing.T, diff SessionDiff, path string) DiffFile {
	t.Helper()
	for _, f := range diff.Files {
		if f.Path == path {
			return f
		}
	}
	t.Fatalf("%s missing from diff: %+v", path, diff.Files)
	return DiffFile{}
}

// repositoryFixture creates a project repository with a few committed files.
func repositoryFixture(t *testing.T, s *Store) *Project {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	p := projectFixture(t, s)
	gitWorktreeFixture(t, p.Directory, "init", "-b", "main")
	writeFixture(t, filepath.Join(p.Directory, "keep.txt"), "one\ntwo\nthree\n")
	writeFixture(t, filepath.Join(p.Directory, "remove.txt"), "gone\n")
	writeFixture(t, filepath.Join(p.Directory, "rename-me.txt"), strings.Repeat("stable line\n", 20))
	writeFixture(t, filepath.Join(p.Directory, "image.bin"), "\x00\x01\x02")
	commitFixture(t, p.Directory, "Fixture")
	return p
}

func TestSessionDiffWorktreeSinceStart(t *testing.T) {
	s := fixtureStore(t)
	p := repositoryFixture(t, s)
	state, err := s.CreateSession(p.ID, "claude", "", "", true, nil, "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	sess := state.Projects[0].Sessions[0]
	if sess.WorktreeBase != gitWorktreeFixture(t, p.Directory, "rev-parse", "HEAD") {
		t.Fatalf("worktree base not recorded: %+v", sess)
	}
	// The project folder moves on after the worktree is created; that must not appear.
	writeFixture(t, filepath.Join(p.Directory, "later.txt"), "project work\n")
	commitFixture(t, p.Directory, "Later project work")
	w := sess.WorktreeDirectory
	writeFixture(t, filepath.Join(w, "committed.txt"), "session commit\n")
	commitFixture(t, w, "Session work")
	writeFixture(t, filepath.Join(w, "keep.txt"), "one\n2\nthree\n")
	if err = os.Remove(filepath.Join(w, "remove.txt")); err != nil {
		t.Fatal(err)
	}
	gitWorktreeFixture(t, w, "mv", "rename-me.txt", "renamed.txt")
	writeFixture(t, filepath.Join(w, "image.bin"), "\x00\x09")
	writeFixture(t, filepath.Join(w, "nested", "new.txt"), "fresh\nfile")
	writeFixture(t, filepath.Join(w, "untracked.bin"), "a\x00b")
	writeFixture(t, filepath.Join(w, ".gitignore"), "ignored.log\n")
	writeFixture(t, filepath.Join(w, "ignored.log"), "noise\n")

	diff, err := s.SessionDiff(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Scope != "worktree" || diff.Base != sess.WorktreeBase || diff.Message != "" {
		t.Fatalf("unexpected scope: %+v", diff)
	}
	for _, path := range []string{"later.txt", "ignored.log", "rename-me.txt"} {
		for _, f := range diff.Files {
			if f.Path == path {
				t.Fatalf("%s should not be listed: %+v", path, f)
			}
		}
	}
	if f := diffFile(t, diff, "committed.txt"); f.Status != "A" || f.Additions != 1 || f.Untracked || !strings.Contains(f.Patch, "+session commit") {
		t.Fatalf("committed session file: %+v", f)
	}
	if f := diffFile(t, diff, "keep.txt"); f.Status != "M" || f.Additions != 1 || f.Deletions != 1 || !strings.HasPrefix(f.Patch, "@@ ") || !strings.Contains(f.Patch, "-two\n+2\n") {
		t.Fatalf("modified file: %+v", f)
	}
	if f := diffFile(t, diff, "remove.txt"); f.Status != "D" || f.Deletions != 1 {
		t.Fatalf("deleted file: %+v", f)
	}
	if f := diffFile(t, diff, "renamed.txt"); f.Status != "R" || f.OldPath != "rename-me.txt" || f.Additions != 0 {
		t.Fatalf("renamed file: %+v", f)
	}
	if f := diffFile(t, diff, "image.bin"); !f.Binary || f.Patch != "" {
		t.Fatalf("binary file: %+v", f)
	}
	if f := diffFile(t, diff, "untracked.bin"); !f.Binary || !f.Untracked || f.Patch != "" {
		t.Fatalf("untracked binary file: %+v", f)
	}
	if f := diffFile(t, diff, "nested/new.txt"); !f.Untracked || f.Status != "A" || f.Additions != 2 || f.Patch != "@@ -0,0 +1,2 @@\n+fresh\n+file\n\\ No newline at end of file\n" {
		t.Fatalf("untracked file: %+v", f)
	}
	if diff.Additions < 5 || diff.Deletions < 2 || diff.Truncated {
		t.Fatalf("unexpected totals: %+v", diff)
	}

	// Older sessions without a saved base fall back to the merge base with the project.
	s.mu.Lock()
	s.state.Projects[0].Sessions[0].WorktreeBase = ""
	s.mu.Unlock()
	fallback, err := s.SessionDiff(sess.ID)
	if err != nil || fallback.Base != sess.WorktreeBase || len(fallback.Files) != len(diff.Files) {
		t.Fatalf("merge-base fallback: %+v %v", fallback, err)
	}

	if folder, err := s.SessionFolder(sess.ID); err != nil || folder != w {
		t.Fatalf("worktree folder: %q %v", folder, err)
	}
	// A missing worktree never falls back to the original checkout.
	if err = os.RemoveAll(w); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SessionDiff(sess.ID); err != errWorktreeMissing {
		t.Fatalf("missing worktree: %v", err)
	}
	if _, err = s.SessionFolder(sess.ID); err != errWorktreeMissing {
		t.Fatalf("missing worktree folder: %v", err)
	}
}

func TestSessionDiffProjectFolderAndEmptyStates(t *testing.T) {
	s := fixtureStore(t)
	p := repositoryFixture(t, s)
	sess := sessionFixture(t, s, p, "pi")
	clean, err := s.SessionDiff(sess.ID)
	if err != nil || clean.Scope != "folder" || clean.Base != "HEAD" || clean.Files == nil || len(clean.Files) != 0 {
		t.Fatalf("clean folder: %+v %v", clean, err)
	}
	writeFixture(t, filepath.Join(p.Directory, "keep.txt"), "changed\n")
	writeFixture(t, filepath.Join(p.Directory, "draft.md"), "draft\n")
	diff, err := s.SessionDiff(sess.ID)
	if err != nil || len(diff.Files) != 2 || diffFile(t, diff, "keep.txt").Status != "M" || !diffFile(t, diff, "draft.md").Untracked {
		t.Fatalf("folder changes: %+v %v", diff, err)
	}
	terminal := sessionFixture(t, s, p, "terminal")
	if diff, err = s.SessionDiff(terminal.ID); err != nil || diff.Scope != "" || diff.Message == "" {
		t.Fatalf("terminal session: %+v %v", diff, err)
	}
	plain := projectFixture(t, s)
	other := sessionFixture(t, s, plain, "claude")
	if diff, err = s.SessionDiff(other.ID); err != nil || diff.Scope != "" || !strings.Contains(diff.Message, "Git") {
		t.Fatalf("folder without Git: %+v %v", diff, err)
	}
	if _, err = s.SessionDiff("missing"); err == nil {
		t.Fatal("unknown session accepted")
	}
	// Sessions without a worktree, terminals included, open the project folder.
	if folder, err := s.SessionFolder(terminal.ID); err != nil || folder != p.Directory {
		t.Fatalf("project folder: %q %v", folder, err)
	}
	if _, err = s.SessionFolder("missing"); err == nil {
		t.Fatal("unknown session folder accepted")
	}
	// A repository without commits compares against the empty tree.
	empty := projectFixture(t, s)
	gitWorktreeFixture(t, empty.Directory, "init", "-b", "main")
	writeFixture(t, filepath.Join(empty.Directory, "first.txt"), "first\n")
	gitWorktreeFixture(t, empty.Directory, "add", "first.txt")
	if diff, err = s.SessionDiff(sessionFixture(t, s, empty, "claude").ID); err != nil || len(diff.Files) != 1 || diff.Files[0].Status != "A" || diff.Files[0].Additions != 1 {
		t.Fatalf("unborn repository: %+v %v", diff, err)
	}
}

func TestSessionDiffTruncation(t *testing.T) {
	s := fixtureStore(t)
	p := repositoryFixture(t, s)
	sess := sessionFixture(t, s, p, "claude")
	writeFixture(t, filepath.Join(p.Directory, "keep.txt"), strings.Repeat("generated\n", maxDiffFileLines+1))
	writeFixture(t, filepath.Join(p.Directory, "large.txt"), strings.Repeat("x", maxDiffFileBytes+1))
	writeFixture(t, filepath.Join(p.Directory, "small.txt"), "small\n")
	diff, err := s.SessionDiff(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if f := diffFile(t, diff, "keep.txt"); !f.Truncated || f.Patch != "" || f.Additions != maxDiffFileLines+1 {
		t.Fatalf("long tracked file: %+v", f)
	}
	if f := diffFile(t, diff, "large.txt"); !f.Truncated || f.Patch != "" {
		t.Fatalf("large untracked file: %+v", f)
	}
	if f := diffFile(t, diff, "small.txt"); f.Truncated || f.Patch == "" || !diff.Truncated {
		t.Fatalf("small file or total flag: %+v %+v", f, diff)
	}
	for i := 0; i <= maxDiffFiles; i++ {
		writeFixture(t, filepath.Join(p.Directory, "many", fmt.Sprintf("%04d.txt", i)), "x\n")
	}
	if diff, err = s.SessionDiff(sess.ID); err != nil || len(diff.Files) != maxDiffFiles || !diff.Truncated {
		t.Fatalf("file count limit: %d %v", len(diff.Files), err)
	}
}
