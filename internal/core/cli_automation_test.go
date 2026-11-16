package core

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCLIAutomationLifecycle(t *testing.T) {
	fakeAgent(t, "exec sleep 30\n")
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	source := sessionFixture(t, s, p, "codex")
	s.mu.Lock()
	env, err := s.collaboration.environmentLocked(source.ID)
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	call := func(args ...string) ([]byte, error) {
		t.Helper()
		if binary := os.Getenv("ORCHESTRA_TEST_CLI_BINARY"); binary != "" {
			cmd := exec.Command(binary, append([]string{"--orchestra-cli", "automation"}, args...)...)
			return cmd.CombinedOutput()
		}
		var out bytes.Buffer
		err := runCLI(append([]string{"automation"}, args...), &out)
		return out.Bytes(), err
	}
	must := func(args ...string) []byte {
		t.Helper()
		data, err := call(args...)
		if err != nil {
			t.Fatal(args, err)
		}
		return data
	}
	var a Automation
	if err := json.Unmarshal(must("create", "--name", "Review", "--agent", "codex", "--prompt", "Review code", "--days", "mon", "--time", "09:00", "--model", "gpt-6.1-sol", "--effort", "high"), &a); err != nil {
		t.Fatal(err)
	}
	if a.Enabled || a.TimeZone != "UTC" || a.WorkspaceID != p.ID || a.RepositoryID != source.RepositoryID {
		t.Fatalf("creation: %+v", a)
	}
	original := a
	promptFile := filepath.Join(t.TempDir(), "prompt.txt")
	if err := os.WriteFile(promptFile, []byte("Updated prompt"), 0600); err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(must("update", a.ID, "--prompt-file", promptFile), &a)
	if a.Prompt != "Updated prompt" || a.Model != original.Model || a.Effort != original.Effort || !a.NextRun.Equal(original.NextRun) {
		t.Fatalf("update: %+v", a)
	}
	must("enable", a.ID)
	if !s.Snapshot().Automations[0].Enabled {
		t.Fatal("enable invisible in UI snapshot")
	}
	must("disable", a.ID)
	before := s.Snapshot().Automations[0]
	json.Unmarshal(must("run", a.ID), &a)
	if a.Enabled || !a.NextRun.Equal(before.NextRun) || a.LastSessionID == "" {
		t.Fatalf("manual run: %+v", a)
	}
	snap := s.Snapshot()
	var launched *Session
	for _, sess := range snap.Workspaces[0].Sessions {
		if sess.ID == a.LastSessionID {
			launched = sess
		}
	}
	if launched == nil || launched.Model != a.Model || launched.Effort != a.Effort {
		t.Fatalf("launch model/effort: %+v", launched)
	}
	reopened, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Snapshot().Automations; len(got) != 1 || got[0].Prompt != a.Prompt || got[0].ID != a.ID {
		t.Fatalf("persistence: %+v", got)
	}
	reopened.Shutdown()
	for _, args := range [][]string{
		{"show", "missing"}, {"delete", "missing"}, {"run", "missing"}, {"enable", "missing"},
		{"update", a.ID, "--timezone", ""}, {"update", a.ID, "--timezone", "Unknown"}, {"update", a.ID, "--time", "25:00"},
		{"update", a.ID, "--days", "monday"}, {"update", a.ID, "--repository", "missing"},
		{"update", a.ID, "--days", "once", "--date", "bad"}, {"update", a.ID, "--enabled", "--disabled"},
		{"update", a.ID, "--prompt", "one", "--prompt-file", promptFile}, {"show", a.ID, "--name", "x"},
	} {
		if _, err := call(args...); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	var list []Automation
	json.Unmarshal(must("list"), &list)
	if len(list) != 1 || list[0].ID != a.ID {
		t.Fatal("list", list)
	}
	must("show", a.ID)
	must("delete", a.ID)
	if len(s.Snapshot().Automations) != 0 {
		t.Fatal("delete invisible")
	}
	t.Setenv("ORCHESTRA_TOKEN", "invalid")
	if _, err := call("list"); err == nil {
		t.Fatal("accepted invalid credentials")
	}
	t.Setenv("ORCHESTRA_SOCKET", "")
	if _, err := call("list"); err == nil {
		t.Fatal("accepted missing context")
	}
}

func TestCLIAutomationScopeAndInput(t *testing.T) {
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	source := sessionFixture(t, s, p, "codex")
	other := workspaceFixture(t, s)
	s.mu.Lock()
	env, err := s.collaboration.environmentLocked(source.ID)
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	input, err := os.OpenFile(filepath.Join(t.TempDir(), "stdin"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	input.WriteString("User-requested review")
	input.Seek(0, 0)
	previous := os.Stdin
	os.Stdin = input
	defer func() { os.Stdin = previous }()
	var out bytes.Buffer
	err = runCLI([]string{"automation", "create", "--workspace", other.ID, "--name", "Once", "--agent", "codex", "--stdin", "--date", "2099-01-01", "--time", "09:00", "--timezone", "America/Sao_Paulo", "--enabled"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	var a Automation
	if err := json.Unmarshal(out.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	if !a.Enabled || a.WorkspaceID != other.ID || a.Prompt != "User-requested review" {
		t.Fatalf("create %+v", a)
	}
	for _, tc := range []struct {
		workspace string
		count     int
	}{{p.ID, 0}, {other.ID, 1}} {
		out.Reset()
		if err := runCLI([]string{"automation", "list", "--workspace", tc.workspace}, &out); err != nil {
			t.Fatal(err)
		}
		var list []Automation
		if err := json.Unmarshal(out.Bytes(), &list); err != nil || len(list) != tc.count {
			t.Fatalf("list %s: %s %v", tc.workspace, out.String(), err)
		}
	}
	// UI edits and CLI reads share the same persisted model.
	a.Name = "Edited in app"
	if _, err := s.SaveAutomation(a); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runCLI([]string{"automation", "show", a.ID}, &out); err != nil {
		t.Fatal(err)
	}
	var got Automation
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || got.Name != a.Name {
		t.Fatalf("UI consistency: %s %v", out.String(), err)
	}
}
