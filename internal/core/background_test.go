package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// backgroundStore opens a workspace whose sessions keep running after quitting. The
// folder is short because Unix socket paths are limited to about 100 bytes.
func backgroundStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "orc-")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "state.json")
	s := openBackground(t, path)
	p := s.Snapshot().Preferences
	p.KeepSessionsRunning = true
	if _, err = s.SetPreferences(p); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Stop whatever the test left in the host, then let the unused host exit.
		cleanup := openBackground(t, path)
		cleanup.StopSessionsOnQuit(true)
		shutdownAndWait(t, cleanup)
		waitUntil(t, func() bool {
			_, err := os.Stat(filepath.Join(dir, "host", "host.sock"))
			return os.IsNotExist(err)
		})
		os.RemoveAll(dir)
	})
	return s, path
}
func openBackground(t *testing.T, path string) *Store {
	t.Helper()
	exe, _ := os.Executable()
	s, err := OpenStore(path, exe, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func fakeAgent(t *testing.T, script string) {
	t.Helper()
	fixture := t.TempDir()
	shell := filepath.Join(fixture, "shell")
	_ = os.WriteFile(shell, []byte("#!/bin/sh\nexec /bin/sh -c \"$4\"\n"), 0700)
	_ = os.WriteFile(filepath.Join(fixture, "claude"), []byte("#!/bin/sh\n"+script), 0700)
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", fixture+":"+os.Getenv("PATH"))
}
func sessionState(s *Store, id string) *Session {
	for _, p := range s.Snapshot().Projects {
		for _, sess := range p.Sessions {
			if sess.ID == id {
				return sess
			}
		}
	}
	return nil
}

func TestBackgroundSessionSurvivesQuitAndReconnects(t *testing.T) {
	fakeAgent(t, "printf waiting > \"$ORCHESTRA_STATUS_FILE\"\nprintf fixture-conversation > \"$ORCHESTRA_AGENT_ID_FILE\"\nstty -echo\nprintf 'fixture ready\\r\\n'\nwhile IFS= read -r line; do printf 'received: %s\\r\\n' \"$line\"; done\n")
	s, path := backgroundStore(t)
	p := projectFixture(t, s)
	agent := sessionFixture(t, s, p, "claude")
	if _, err := s.StartSession(agent.ID, false, 80, 24); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return strings.Contains(terminalText(s, agent.ID), "fixture ready") })
	if sess := sessionState(s, agent.ID); !sess.Background || sess.LaunchID == "" {
		t.Fatalf("session did not start in the host: %+v", sess)
	}
	s.mu.Lock()
	token := s.collaboration.tokens[agent.ID]
	s.mu.Unlock()
	shutdownAndWait(t, s)

	reopened := openBackground(t, path)
	t.Cleanup(func() { shutdownAndWait(t, reopened) })
	sess := sessionState(reopened, agent.ID)
	if !sess.Running || sess.Restored || !sess.Background {
		t.Fatalf("session not reconnected: %+v", sess)
	}
	if !strings.Contains(terminalText(reopened, agent.ID), "fixture ready") {
		t.Fatal("scrollback lost")
	}
	if err := reopened.WriteTerminal(agent.ID, "hello\n"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return strings.Contains(terminalText(reopened, agent.ID), "received: hello") })
	waitUntil(t, func() bool {
		sess := sessionState(reopened, agent.ID)
		return sess.Activity == "waiting" && sess.AgentSessionID == "fixture-conversation"
	})
	reopened.mu.Lock()
	restored := reopened.collaboration.tokens[agent.ID]
	reopened.mu.Unlock()
	if token == "" || restored != token {
		t.Fatal("session credentials changed after reconnecting")
	}
	if _, err := reopened.CloseSession(agent.ID, false); err != nil {
		t.Fatal(err)
	}
	reopened.mu.Lock()
	infos, err := reopened.host.List()
	reopened.mu.Unlock()
	if err != nil || len(infos) != 0 {
		t.Fatalf("closed session kept in the host: %+v, %v", infos, err)
	}
}

func TestBackgroundSessionThatEndsWhileClosed(t *testing.T) {
	fakeAgent(t, "printf 'Session ID: 0f8fad5b-d9cb-469f-a165-70867728950e\\r\\n'\nsleep 0.5\nexit 3\n")
	s, path := backgroundStore(t)
	p := projectFixture(t, s)
	agent := sessionFixture(t, s, p, "codex")
	// The fake codex is the same script as claude.
	_ = os.Symlink(filepath.Join(filepath.SplitList(os.Getenv("PATH"))[0], "claude"), filepath.Join(filepath.SplitList(os.Getenv("PATH"))[0], "codex"))
	if _, err := s.StartSession(agent.ID, false, 80, 24); err != nil {
		t.Fatal(err)
	}
	shutdownAndWait(t, s)
	time.Sleep(time.Second)

	reopened := openBackground(t, path)
	t.Cleanup(func() { shutdownAndWait(t, reopened) })
	waitUntil(t, func() bool {
		sess := sessionState(reopened, agent.ID)
		return !sess.Running && strings.Contains(sess.ExitMessage, "code 3")
	})
	sess := sessionState(reopened, agent.ID)
	if sess.AgentSessionID != "0f8fad5b-d9cb-469f-a165-70867728950e" || sess.Background {
		t.Fatalf("exit not recorded like a live one: %+v", sess)
	}
	reopened.mu.Lock()
	infos, _ := reopened.host.List()
	reopened.mu.Unlock()
	if len(infos) != 0 {
		t.Fatalf("ended session kept in the host: %+v", infos)
	}
}

func TestStopSessionsOnQuitAndPreferenceOff(t *testing.T) {
	fakeAgent(t, "printf 'up\\r\\n'\nexec sleep 30\n")
	s, path := backgroundStore(t)
	p := projectFixture(t, s)
	agent := sessionFixture(t, s, p, "claude")
	if _, err := s.StartSession(agent.ID, false, 80, 24); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return strings.Contains(terminalText(s, agent.ID), "up") })
	s.StopSessionsOnQuit(true)
	shutdownAndWait(t, s)

	reopened := openBackground(t, path)
	if sess := sessionState(reopened, agent.ID); sess.Running || !sess.Restored {
		t.Fatalf("stopped session reconnected: %+v", sess)
	}
	// With the preference off, new sessions run in the app and stop with it.
	prefs := reopened.Snapshot().Preferences
	prefs.KeepSessionsRunning = false
	if _, err := reopened.SetPreferences(prefs); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.StartSession(agent.ID, true, 80, 24); err != nil {
		t.Fatal(err)
	}
	if sess := sessionState(reopened, agent.ID); !sess.Running || sess.Background || sess.LaunchID != "" {
		t.Fatalf("session started in the host with the preference off: %+v", sess)
	}
	shutdownAndWait(t, reopened)
	again := openBackground(t, path)
	t.Cleanup(func() { shutdownAndWait(t, again) })
	if sess := sessionState(again, agent.ID); sess.Running {
		t.Fatalf("app session survived quitting: %+v", sess)
	}
}

// When the connection to the host drops, the session keeps running there. Starting it
// again must not run a second agent beside it, and reopening reconnects it.
func TestLostHostConnectionNeverStartsSecondAgent(t *testing.T) {
	fakeAgent(t, "printf fixture-conversation > \"$ORCHESTRA_AGENT_ID_FILE\"\nprintf 'fixture ready\\r\\n'\nexec sleep 30\n")
	s, path := backgroundStore(t)
	p := projectFixture(t, s)
	agent := sessionFixture(t, s, p, "claude")
	if _, err := s.StartSession(agent.ID, false, 80, 24); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool {
		return strings.Contains(terminalText(s, agent.ID), "fixture ready") && sessionState(s, agent.ID).AgentSessionID == "fixture-conversation"
	})
	s.mu.Lock()
	s.host.Close()
	s.mu.Unlock()
	waitUntil(t, func() bool { return !sessionState(s, agent.ID).Running })

	if _, err := s.StartSession(agent.ID, false, 80, 24); err == nil || !strings.Contains(err.Error(), "still running in the background") {
		t.Fatalf("started the session again: %v", err)
	}
	s.mu.Lock()
	infos, err := s.host.List()
	local := s.terminals[agent.ID] != nil && !s.terminals[agent.ID].background
	s.mu.Unlock()
	if err != nil || len(infos) != 1 || infos[0].Exit != nil || local {
		t.Fatalf("host sessions %+v, %v; local process %v", infos, err, local)
	}
	shutdownAndWait(t, s)

	reopened := openBackground(t, path)
	t.Cleanup(func() { shutdownAndWait(t, reopened) })
	if sess := sessionState(reopened, agent.ID); !sess.Running || !sess.Background {
		t.Fatalf("session not reconnected: %+v", sess)
	}
}
