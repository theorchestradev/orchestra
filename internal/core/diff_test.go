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

// repositoryFixture creates a workspace repository with a few committed files.
func repositoryFixture(t *testing.T, s *Store) *Workspace {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	p := workspaceFixture(t, s)
	gitWorktreeFixture(t, p.Repositories[0].Directory, "init", "-b", "main")
	writeFixture(t, filepath.Join(p.Repositories[0].Directory, "keep.txt"), "one\ntwo\nthree\n")
	writeFixture(t, filepath.Join(p.Repositories[0].Directory, "remove.txt"), "gone\n")
	writeFixture(t, filepath.Join(p.Repositories[0].Directory, "rename-me.txt"), strings.Repeat("stable line\n", 20))
	writeFixture(t, filepath.Join(p.Repositories[0].Directory, "image.bin"), "\x00\x01\x02")
	commitFixture(t, p.Repositories[0].Directory, "Fixture")
	return p
}

func TestSessionDiffWorktreeSinceStart(t *testing.T) {
	s := fixtureStore(t)
	p := repositoryFixture(t, s)
	state, err := s.CreateSession(p.ID, "", "claude", "", "", true, nil, "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	sess := state.Workspaces[0].Sessions[0]
	if sess.WorktreeBase != gitWorktreeFixture(t, p.Repositories[0].Directory, "rev-parse", "HEAD") {
		t.Fatalf("worktree base not recorded: %+v", sess)
	}
	// The repository folder moves on after the worktree is created; that must not appear.
	writeFixture(t, filepath.Join(p.Repositories[0].Directory, "later.txt"), "workspace work\n")
	commitFixture(t, p.Repositories[0].Directory, "Later workspace work")
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

	// Older sessions without a saved base fall back to the merge base with the workspace.
	s.mu.Lock()
	s.state.Workspaces[0].Sessions[0].WorktreeBase = ""
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

func TestSessionDiffWorkspaceFolderAndEmptyStates(t *testing.T) {
	s := fixtureStore(t)
	p := repositoryFixture(t, s)
	sess := sessionFixture(t, s, p, "pi")
	clean, err := s.SessionDiff(sess.ID)
	if err != nil || clean.Scope != "folder" || clean.Base != "HEAD" || clean.Files == nil || len(clean.Files) != 0 {
		t.Fatalf("clean folder: %+v %v", clean, err)
	}
	writeFixture(t, filepath.Join(p.Repositories[0].Directory, "keep.txt"), "changed\n")
	writeFixture(t, filepath.Join(p.Repositories[0].Directory, "draft.md"), "draft\n")
	diff, err := s.SessionDiff(sess.ID)
	if err != nil || len(diff.Files) != 2 || diffFile(t, diff, "keep.txt").Status != "M" || !diffFile(t, diff, "draft.md").Untracked {
		t.Fatalf("folder changes: %+v %v", diff, err)
	}
	terminal := sessionFixture(t, s, p, "terminal")
	if diff, err = s.SessionDiff(terminal.ID); err != nil || diff.Scope != "" || diff.Message == "" {
		t.Fatalf("terminal session: %+v %v", diff, err)
	}
	plain := workspaceFixture(t, s)
	other := sessionFixture(t, s, plain, "claude")
	if diff, err = s.SessionDiff(other.ID); err != nil || diff.Scope != "" || !strings.Contains(diff.Message, "Git") {
		t.Fatalf("folder without Git: %+v %v", diff, err)
	}
	if _, err = s.SessionDiff("missing"); err == nil {
		t.Fatal("unknown session accepted")
	}
	// Sessions without a worktree, terminals included, open the repository folder.
	if folder, err := s.SessionFolder(terminal.ID); err != nil || folder != p.Repositories[0].Directory {
		t.Fatalf("repository folder: %q %v", folder, err)
	}
	if _, err = s.SessionFolder("missing"); err == nil {
		t.Fatal("unknown session folder accepted")
	}
	// A repository without commits compares against the empty tree.
	empty := workspaceFixture(t, s)
	gitWorktreeFixture(t, empty.Repositories[0].Directory, "init", "-b", "main")
	writeFixture(t, filepath.Join(empty.Repositories[0].Directory, "first.txt"), "first\n")
	gitWorktreeFixture(t, empty.Repositories[0].Directory, "add", "first.txt")
	if diff, err = s.SessionDiff(sessionFixture(t, s, empty, "claude").ID); err != nil || len(diff.Files) != 1 || diff.Files[0].Status != "A" || diff.Files[0].Additions != 1 {
		t.Fatalf("unborn repository: %+v %v", diff, err)
	}
}

func TestSessionDiffTruncation(t *testing.T) {
	s := fixtureStore(t)
	p := repositoryFixture(t, s)
	sess := sessionFixture(t, s, p, "claude")
	writeFixture(t, filepath.Join(p.Repositories[0].Directory, "keep.txt"), strings.Repeat("generated\n", maxDiffFileLines+1))
	writeFixture(t, filepath.Join(p.Repositories[0].Directory, "large.txt"), strings.Repeat("x", maxDiffFileBytes+1))
	writeFixture(t, filepath.Join(p.Repositories[0].Directory, "small.txt"), "small\n")
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
		writeFixture(t, filepath.Join(p.Repositories[0].Directory, "many", fmt.Sprintf("%04d.txt", i)), "x\n")
	}
	if diff, err = s.SessionDiff(sess.ID); err != nil || len(diff.Files) != maxDiffFiles || !diff.Truncated {
		t.Fatalf("file count limit: %d %v", len(diff.Files), err)
	}
}

// Upstream commits must never become branch changes after integration.
func TestSessionDiffBranchIntegration(t *testing.T) {
	for _, integration := range []string{"rebase", "merge"} {
		t.Run(integration, func(t *testing.T) {
			s := fixtureStore(t)
			p := repositoryFixture(t, s)
			root := p.Repositories[0].Directory
			writeFixture(t, filepath.Join(root, "one.txt"), "old one\n")
			writeFixture(t, filepath.Join(root, "two.txt"), "old two\n")
			commitFixture(t, root, "Before feature")
			start := gitWorktreeFixture(t, root, "rev-parse", "HEAD")
			work := filepath.Join(t.TempDir(), "work")
			gitWorktreeFixture(t, root, "worktree", "add", "-b", "feature", work)
			writeFixture(t, filepath.Join(work, "one.txt"), "one\ntwo\nthree\n")
			writeFixture(t, filepath.Join(work, "two.txt"), "one\ntwo\nthree\n")
			commitFixture(t, work, "Feature")
			writeFixture(t, filepath.Join(root, "unrelated.txt"), "upstream\n")
			commitFixture(t, root, "Upstream")
			gitWorktreeFixture(t, root, "update-ref", "refs/remotes/origin/main", "HEAD")
			gitWorktreeFixture(t, root, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
			gitWorktreeFixture(t, work, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", integration, "main")
			read := func(refs ...string) SessionDiff {
				t.Helper()
				d, err := readSessionDiff("claude", root, work, start, refs...)
				if err != nil {
					t.Fatal(err)
				}
				return d
			}
			diff := read()
			if len(diff.Files) != 2 || diff.Additions != 6 || diff.Deletions != 2 || diff.Reference != "origin/main" || diff.Branch != "feature" {
				t.Fatalf("branch: %+v", diff)
			}
			if old := read("start"); len(old.Files) != 3 || old.Reference != "" {
				t.Fatalf("start: %+v", old)
			}
			// Staged, unstaged, untracked and rename all participate in the same comparison.
			gitWorktreeFixture(t, work, "mv", "rename-me.txt", "renamed.txt")
			writeFixture(t, filepath.Join(work, "one.txt"), "one\ntwo\nthree\nstaged\n")
			gitWorktreeFixture(t, work, "add", "one.txt")
			writeFixture(t, filepath.Join(work, "one.txt"), "one\ntwo\nthree\nstaged\nunstaged\n")
			writeFixture(t, filepath.Join(work, "new.txt"), "new\n")
			indexPath := gitWorktreeFixture(t, work, "rev-parse", "--git-path", "index")
			before, err := os.ReadFile(indexPath)
			if err != nil {
				t.Fatal(err)
			}
			diff = read()
			after, err := os.ReadFile(indexPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("comparison wrote the index")
			}
			if f := diffFile(t, diff, "one.txt"); f.Additions != 5 {
				t.Fatalf("working file: %+v", f)
			}
			if f := diffFile(t, diff, "renamed.txt"); f.OldPath != "rename-me.txt" || f.Status != "R" {
				t.Fatalf("rename: %+v", f)
			}
			if f := diffFile(t, diff, "new.txt"); !f.Untracked || f.Additions != 1 {
				t.Fatalf("untracked: %+v", f)
			}
			gitWorktreeFixture(t, work, "checkout", "--detach")
			if d := read(); !strings.HasPrefix(d.Branch, "Detached HEAD") || d.Reference != "origin/main" {
				t.Fatalf("detached: %+v", d)
			}
			gitWorktreeFixture(t, root, "update-ref", "-d", "refs/remotes/origin/main")
			if d := read(); d.Reference != "" || d.Base != start {
				t.Fatalf("fallback: %+v", d)
			}
		})
	}
}

func TestSessionDiffOpenPRBaseChanges(t *testing.T) {
	s := fixtureStore(t)
	p := repositoryFixture(t, s)
	root := p.Repositories[0].Directory
	gitWorktreeFixture(t, root, "update-ref", "refs/remotes/origin/main", "HEAD")
	gitWorktreeFixture(t, root, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	writeFixture(t, filepath.Join(root, "parent.txt"), "parent\n")
	commitFixture(t, root, "Parent PR")
	gitWorktreeFixture(t, root, "update-ref", "refs/remotes/origin/parent", "HEAD")
	state, err := s.CreateSession(p.ID, "", "claude", "", "", true, nil, "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	sess := state.Workspaces[0].Sessions[0]
	writeFixture(t, filepath.Join(sess.WorktreeDirectory, "child.txt"), "child\n")
	commitFixture(t, sess.WorktreeDirectory, "Child PR")
	s.mu.Lock()
	stored, _ := s.workspaceLocked(p.ID)
	stored.Repositories[0].GitHub = "fixture/repository"
	s.mu.Unlock()
	_, respond := pullRequestFixture(t)
	for _, base := range []string{"parent", "main", "parent"} {
		respond("list", fmt.Sprintf(`[{"headRefName":%q,"baseRefName":%q}]`, sess.WorktreeBranch, base))
		d, err := s.SessionDiff(sess.ID)
		count := 1
		if base == "main" {
			count = 2
		}
		if err != nil || d.Reference != "origin/"+base || len(d.Files) != count {
			t.Fatalf("%s: %+v, %v", base, d, err)
		}
	}
	// Once a PR is closed/squashed, default to origin/HEAD and retain merge-base semantics.
	respond("list", `[]`)
	d, err := s.SessionDiff(sess.ID)
	if err != nil || d.Reference != "origin/main" || len(d.Files) != 2 {
		t.Fatalf("closed PR: %+v, %v", d, err)
	}
	gitWorktreeFixture(t, sess.WorktreeDirectory, "checkout", "-b", "switched")
	if branch, err := s.SessionBranch(sess.ID); err != nil || branch != "switched" {
		t.Fatalf("branch: %q %v", branch, err)
	}
}

func TestSessionDiffUnrelatedBaseAndSquash(t *testing.T) {
	s := fixtureStore(t)
	p := repositoryFixture(t, s)
	root := p.Repositories[0].Directory
	start := gitWorktreeFixture(t, root, "rev-parse", "HEAD")
	work := filepath.Join(t.TempDir(), "work")
	gitWorktreeFixture(t, root, "worktree", "add", "-b", "feature", work)
	writeFixture(t, filepath.Join(work, "feature.txt"), "feature\n")
	commitFixture(t, work, "Feature")
	// A squash produces the same files on main without sharing the feature commit.
	writeFixture(t, filepath.Join(root, "feature.txt"), "feature\n")
	commitFixture(t, root, "Squash feature")
	gitWorktreeFixture(t, root, "update-ref", "refs/remotes/origin/main", "HEAD")
	gitWorktreeFixture(t, root, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	d, err := readSessionDiff("claude", root, work, start)
	if err != nil || len(d.Files) != 1 || d.Base != start || d.Reference != "origin/main" {
		t.Fatalf("squash: %+v, %v", d, err)
	}
	// An existing but unrelated default branch must fall back to the saved baseline.
	gitWorktreeFixture(t, root, "checkout", "--orphan", "unrelated")
	commitFixture(t, root, "Unrelated root")
	gitWorktreeFixture(t, root, "update-ref", "refs/remotes/origin/main", "HEAD")
	d, err = readSessionDiff("claude", root, work, start)
	if err != nil || d.Reference != "" || d.Base != start || len(d.Files) != 1 {
		t.Fatalf("no merge base: %+v, %v", d, err)
	}
}
