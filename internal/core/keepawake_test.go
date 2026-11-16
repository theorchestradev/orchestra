package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestKeepAwakeFollowsPreferenceAndRunningSessions(t *testing.T) {
	previous := keepAwakeCommand
	keepAwakeCommand = func() *exec.Cmd { return exec.Command("sleep", "60") }
	t.Cleanup(func() { keepAwakeCommand = previous })
	awake := func(s *Store) bool { s.mu.Lock(); defer s.mu.Unlock(); return s.keepAwake != nil }

	fixture := t.TempDir()
	shell := filepath.Join(fixture, "shell")
	_ = os.WriteFile(shell, []byte("#!/bin/sh\nexec /bin/sh -c \"$4\"\n"), 0700)
	_ = os.WriteFile(filepath.Join(fixture, "claude"), []byte("#!/bin/sh\nsleep 60\n"), 0700)
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", fixture+":"+os.Getenv("PATH"))
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	start := func() *Session {
		session := sessionFixture(t, s, p, "claude")
		if _, err := s.StartSession(session.ID, false, 80, 24); err != nil {
			t.Fatal(err)
		}
		return session
	}
	session := start()
	if awake(s) {
		t.Fatal("kept the computer awake while the preference is off")
	}
	prefs := s.Snapshot().Preferences
	prefs.KeepAwake = true
	if _, err := s.SetPreferences(prefs); err != nil {
		t.Fatal(err)
	}
	if !awake(s) {
		t.Fatal("did not keep the computer awake with a running session")
	}
	if _, err := s.CloseSession(session.ID, false); err != nil {
		t.Fatal(err)
	}
	if awake(s) {
		t.Fatal("kept the computer awake without running sessions")
	}
	start()
	if !awake(s) {
		t.Fatal("did not keep the computer awake for a new session")
	}
	s.Shutdown()
	if awake(s) {
		t.Fatal("kept the computer awake after shutdown")
	}
}
