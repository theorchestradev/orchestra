package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionFiles(t *testing.T) {
	s := fixtureStore(t)
	p := projectFixture(t, s)
	sess := sessionFixture(t, s, p, "terminal")
	writeFixture(t, filepath.Join(p.Directory, "src", "main.go"), "package main\n")
	writeFixture(t, filepath.Join(p.Directory, "README.md"), "hello\n")
	writeFixture(t, filepath.Join(p.Directory, ".git", "config"), "private")
	files, err := s.SessionFiles(sess.ID, "")
	if err != nil || !files.Directory || len(files.Entries) != 2 || files.Entries[0].Name != "src" {
		t.Fatalf("listing: %+v %v", files, err)
	}
	files, err = s.SessionFiles(sess.ID, "src/main.go")
	if err != nil || files.Content != "package main\n" {
		t.Fatalf("preview: %+v %v", files, err)
	}
	outside := t.TempDir()
	writeFixture(t, filepath.Join(outside, "secret"), "secret")
	if err := os.Symlink(outside, filepath.Join(p.Directory, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../secret", outside, "escape/secret", ".git/config"} {
		if _, err := s.SessionFiles(sess.ID, path); err == nil {
			t.Fatalf("accepted unsafe path %q", path)
		}
	}
	for name, content := range map[string]string{"binary": "a\x00b", "large": strings.Repeat("a", 256*1024+1)} {
		writeFixture(t, filepath.Join(p.Directory, name), content)
		files, err = s.SessionFiles(sess.ID, name)
		if err != nil || files.Content != "" || files.Message == "" {
			t.Fatalf("unsupported preview: %+v %v", files, err)
		}
	}
	worktree := t.TempDir()
	writeFixture(t, filepath.Join(worktree, "only.txt"), "worktree")
	s.mu.Lock()
	_, stored, lookupErr := s.sessionLocked(sess.ID)
	if lookupErr != nil {
		s.mu.Unlock()
		t.Fatal(lookupErr)
	}
	stored.WorktreeDirectory = worktree
	s.mu.Unlock()
	files, err = s.SessionFiles(sess.ID, "only.txt")
	if err != nil || files.Content != "worktree" {
		t.Fatalf("worktree: %+v %v", files, err)
	}
	if _, err := s.SessionFiles("missing", ""); err == nil {
		t.Fatal("accepted missing session")
	}
}
