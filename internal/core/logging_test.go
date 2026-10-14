package core

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}
func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestOpenLogRotatesLargeFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "orchestra.log")
	if err := os.WriteFile(path+".1", []byte("oldest\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), logRotateSize+1), 0600); err != nil {
		t.Fatal(err)
	}
	logger, f, err := OpenLog(dir, "info")
	if err != nil {
		t.Fatal(err)
	}
	logger.Debug("hidden")
	logger.Info("startup", "os", "test")
	f.Close()
	if info, err := os.Stat(path + ".1"); err != nil || info.Size() != logRotateSize+1 {
		t.Fatal("large log was not kept as orchestra.log.1", err)
	}
	data, _ := os.ReadFile(path)
	var record map[string]any
	if err = json.Unmarshal(data, &record); err != nil || record["msg"] != "startup" || record["level"] != "INFO" {
		t.Fatalf("unexpected log %q: %v", data, err)
	}
	// A small log keeps growing, and debug records appear when requested.
	logger, f, err = OpenLog(dir, "debug")
	if err != nil {
		t.Fatal(err)
	}
	logger.Debug("details")
	f.Close()
	if data, _ = os.ReadFile(path); strings.Count(string(data), "\n") != 2 || !strings.Contains(string(data), "details") {
		t.Fatalf("unexpected log %q", data)
	}
}

func TestLogsOmitPromptsAndNotes(t *testing.T) {
	var out lockedBuffer
	exe, _ := os.Executable()
	s, err := NewStore(filepath.Join(t.TempDir(), "state.json"), exe, nil, slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})))
	if err != nil {
		t.Fatal(err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			shutdownAndWait(t, s)
		}
	})
	fixture := t.TempDir()
	shell := filepath.Join(fixture, "shell")
	if err = os.WriteFile(shell, []byte("#!/bin/sh\nexec /bin/sh -c \"$4\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	// The agent echoes its arguments, so the prompt also passes through terminal output.
	if err = os.WriteFile(filepath.Join(fixture, "claude"), []byte("#!/bin/sh\nprintf 'ready %s\\r\\n' \"$*\"\nwhile IFS= read -r line; do :; done\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", fixture+":"+os.Getenv("PATH"))
	p := projectFixture(t, s)
	state, err := s.CreateSession(p.ID, "claude", "", "private initial prompt", false, nil, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	agent := state.Projects[0].Sessions[0]
	sender := sessionFixture(t, s, p, "pi")
	if _, err = s.StartSession(agent.ID, false, 80, 24); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool {
		output, _ := s.TerminalSnapshot(agent.ID)
		data, _ := base64.StdEncoding.DecodeString(output.Data)
		return strings.Contains(string(data), "private initial prompt")
	})
	if _, err = s.Select(p.ID, "scratchpad"); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	env, err := s.collaboration.environmentLocked(sender.ID)
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	for _, req := range []WireRequest{
		{Method: "session.request", Target: agent.ID, Text: "private request body"},
		{Method: "scratchpad.write", Text: "private note text"},
	} {
		req.Source, req.Token = env["ORCHESTRA_SESSION_ID"], env["ORCHESTRA_TOKEN"]
		if _, err = CallBroker(env["ORCHESTRA_SOCKET"], req); err != nil {
			t.Fatal(req.Method, err)
		}
	}
	shutdownAndWait(t, s)
	stopped = true
	logs := out.String()
	for _, want := range []string{`"msg":"session started"`, `"agent":"claude"`, `"worktree":false`, `"msg":"session exited"`, `"method":"session.request"`, `"method":"scratchpad.write"`, `"session":"` + sender.ID + `"`, `"session":"` + agent.ID + `"`} {
		if !strings.Contains(logs, want) {
			t.Errorf("log is missing %s:\n%s", want, logs)
		}
	}
	for _, private := range []string{"private", env["ORCHESTRA_TOKEN"], p.Name} {
		if strings.Contains(logs, private) {
			t.Errorf("log contains %q:\n%s", private, logs)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		if !json.Valid([]byte(line)) {
			t.Errorf("log line is not JSON: %s", line)
		}
	}
}
