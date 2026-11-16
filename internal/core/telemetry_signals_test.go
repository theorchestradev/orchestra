package core

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestTelemetrySessionSignalsFollowProcessLifecycle(t *testing.T) {
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	sess := sessionFixture(t, s, p, "terminal")
	shell := filepath.Join(t.TempDir(), "fake-shell")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\nexit 7\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", shell)
	var mu sync.Mutex
	var names []string
	var exit map[string]any
	s.SetTracker(func(name string, props map[string]any) {
		mu.Lock()
		defer mu.Unlock()
		names = append(names, name)
		if name == "session_ended" {
			exit = props
		}
	})
	if _, err := s.StartSession(sess.ID, false, 80, 24); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	done := s.terminals[sess.ID].done
	s.mu.Unlock()
	<-done
	if _, err := s.CloseSession(sess.ID, false); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(names) != 3 || names[0] != "session_started" || names[1] != "session_ended" || names[2] != "error_shown" {
		t.Fatalf("wrong/duplicate lifecycle events: %v", names)
	}
	if exit["agent"] != "terminal" || exit["exit_kind"] != "error" || exit["duration_bucket"] != "under_1m" || len(exit) != 3 {
		t.Fatalf("unsafe/wrong exit properties: %v", exit)
	}
}
