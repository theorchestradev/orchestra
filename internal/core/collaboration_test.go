package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHistoryPersistsAsJSON(t *testing.T) {
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	a := sessionFixture(t, s, p, "claude")
	b := sessionFixture(t, s, p, "codex")
	if _, err := s.SendRequest(a.ID, b.ID, "first"); err != nil {
		t.Fatal(err)
	}
	state, err := s.SendRequest(a.ID, b.ID, "second")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Dir(s.path), "collaboration.json")
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("history file missing or not private", err)
	}
	data, _ := os.ReadFile(path)
	var saved history
	if err = json.Unmarshal(data, &saved); err != nil || saved.Version != 1 || len(saved.Runs) != 2 || saved.Runs[0].Prompt != "first" || saved.Runs[1].ID != state.Runs[1].ID {
		t.Fatalf("unexpected history: %s", data)
	}
	reopened, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	if runs := reopened.Snapshot().Runs; len(runs) != 2 || runs[0].Status != "queued" || runs[1].Prompt != "second" {
		t.Fatalf("unread requests lost on restart: %+v", runs)
	}
}

func TestHistoryRecoversFromMissingOrCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "collaboration.json")
	runs, warning, err := loadRuns(path)
	if err != nil || warning != "" || len(runs) != 0 {
		t.Fatal("missing history did not start empty", warning, err)
	}
	for _, bad := range []string{"{not json", `{"version":2,"runs":[]}`, `{"version":1,"runs":[null]}`} {
		_ = os.WriteFile(path, []byte(bad), 0600)
		runs, warning, err = loadRuns(path)
		if err != nil || warning == "" || len(runs) != 0 {
			t.Fatalf("%s: unreadable history not reported: %q %v", bad, warning, err)
		}
		if got, _ := os.ReadFile(path + ".corrupt"); string(got) != bad {
			t.Fatalf("%s: unreadable history not kept aside", bad)
		}
		if _, err = os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("unreadable history left in place")
		}
	}
	// Earlier histories named the workspace projectID.
	_ = os.WriteFile(path, []byte(`{"version":1,"runs":[{"id":"r","projectID":"w","toSessionID":"s","status":"queued"}]}`), 0600)
	if runs, warning, err = loadRuns(path); err != nil || warning != "" || len(runs) != 1 || runs[0].WorkspaceID != "w" || runs[0].LegacyWorkspaceID != "" {
		t.Fatalf("projectID not read as workspaceID: %+v %q %v", runs, warning, err)
	}
	state := filepath.Join(dir, "state.json")
	_ = os.WriteFile(path, []byte("garbage"), 0600)
	exe, _ := os.Executable()
	s, err := NewStore(state, exe, nil, nil)
	if err != nil {
		t.Fatal("corrupt history prevented startup", err)
	}
	defer s.Shutdown()
	if !strings.Contains(s.Snapshot().Error, "collaboration.json.corrupt") {
		t.Fatal("corrupt history not reported to the user")
	}
}

func TestHistoryPruningKeepsActiveRuns(t *testing.T) {
	now := time.Now()
	created := func(age time.Duration) float64 {
		return float64(now.Add(-age).UnixMilli())/1000 - 978307200
	}
	runs := []*Run{
		{ID: "old-queued", Status: "queued", CreatedAt: created(90 * 24 * time.Hour)},
		{ID: "old-running", Status: "running", CreatedAt: created(90 * 24 * time.Hour)},
		{ID: "old-done", Status: "completed", CreatedAt: created(31 * 24 * time.Hour)},
	}
	for i := range historyLimit + 5 {
		runs = append(runs, &Run{ID: fmt.Sprint("done-", i), Status: "completed", CreatedAt: created(time.Hour)})
	}
	kept := pruneRuns(runs, now)
	if len(kept) != historyLimit+2 || kept[0].ID != "old-queued" || kept[1].ID != "old-running" {
		t.Fatalf("active runs pruned or limit ignored: %d", len(kept))
	}
	if kept[2].ID != "done-5" || kept[len(kept)-1].ID != fmt.Sprint("done-", historyLimit+4) {
		t.Fatal("pruning did not drop the oldest finished runs", kept[2].ID)
	}
}

func TestScratchpadBroker(t *testing.T) {
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	a := sessionFixture(t, s, p, "claude")
	other := workspaceFixture(t, s)
	b := sessionFixture(t, s, other, "codex")
	s.mu.Lock()
	envA, err := s.collaboration.environmentLocked(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	envB, err := s.collaboration.environmentLocked(b.ID)
	emitted := 0
	s.emit = func(name string, _ any) {
		if name == "state" {
			emitted++
		}
	}
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	call := func(env map[string]string, method, text string, revision *int) (map[string]any, error) {
		t.Helper()
		data, err := CallBroker(env["ORCHESTRA_SOCKET"], WireRequest{Method: method, Source: env["ORCHESTRA_SESSION_ID"], Token: env["ORCHESTRA_TOKEN"], Text: text, Revision: revision})
		if err != nil {
			return nil, err
		}
		var result map[string]any
		return result, json.Unmarshal(data, &result)
	}
	if _, err = call(envA, "scratchpad.read", "", nil); err == nil || !strings.Contains(err.Error(), "no Scratchpad") {
		t.Fatal("read without a Scratchpad", err)
	}
	if _, err = call(envA, "scratchpad.append", "note", nil); err == nil || s.Snapshot().Workspaces[0].HasScratchpad {
		t.Fatal("append created a Scratchpad", err)
	}
	for _, id := range []string{p.ID, other.ID} {
		if _, err = s.Select(id, "scratchpad"); err != nil {
			t.Fatal(err)
		}
	}
	revision, err := s.SaveScratchpad(p.ID, "User notes", 0)
	if err != nil || revision != 1 {
		t.Fatal("user save did not bump the revision", revision, err)
	}
	if _, err = s.SaveScratchpad(p.ID, "Stale editor", 0); err != errScratchpadChanged {
		t.Fatal("stale editor save accepted", err)
	}
	if _, err = s.SaveScratchpad(other.ID, "Other workspace", 0); err != nil {
		t.Fatal(err)
	}
	read, err := call(envA, "scratchpad.read", "", nil)
	if err != nil || read["text"] != "User notes" || read["revision"] != 1.0 || read["name"] != "Scratchpad" {
		t.Fatalf("unexpected read: %v %v", read, err)
	}
	if read, err = call(envB, "scratchpad.read", "", nil); err != nil || read["text"] != "Other workspace" {
		t.Fatal("workspace scoping failed", read, err)
	}
	emitted = 0
	if _, err = call(envA, "scratchpad.append", "Agent note", nil); err != nil || emitted != 1 {
		t.Fatal("append did not notify views", err, emitted)
	}
	stale := 1
	if _, err = call(envA, "scratchpad.write", "Replace", &stale); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatal("stale write accepted", err)
	}
	current := 2
	written, err := call(envA, "scratchpad.write", "Replaced", &current)
	if err != nil || written["revision"] != 3.0 {
		t.Fatal("current write rejected", written, err)
	}
	if _, err = call(envA, "scratchpad.append", "  ", nil); err == nil {
		t.Fatal("empty append accepted")
	}
	if _, err = call(envA, "scratchpad.write", strings.Repeat("x", 256*1024), nil); err == nil {
		t.Fatal("oversized write accepted")
	}
	if _, err = call(envA, "scratchpad.write", strings.Repeat("x", 200*1024), nil); err != nil {
		t.Fatal(err)
	}
	if _, err = call(envA, "scratchpad.append", strings.Repeat("y", 100*1024), nil); err == nil {
		t.Fatal("append grew the Scratchpad past the limit")
	}
	if _, err = call(envA, "scratchpad.write", "Line one", nil); err != nil {
		t.Fatal(err)
	}
	if _, err = call(envA, "scratchpad.append", "Line two\n", nil); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	if saved := reopened.Snapshot().Workspaces[0]; saved.ScratchpadText != "Line one\nLine two\n" || saved.ScratchpadRevision != 6 {
		t.Fatalf("CLI writes not persisted: %q %d", saved.ScratchpadText, saved.ScratchpadRevision)
	}
	if read, err = call(envB, "scratchpad.read", "", nil); err != nil || read["text"] != "Other workspace" {
		t.Fatal("write reached another workspace", read, err)
	}
}

func TestScratchpadCLI(t *testing.T) {
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	a := sessionFixture(t, s, p, "claude")
	if _, err := s.Select(p.ID, "scratchpad"); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	env, err := s.collaboration.environmentLocked(a.ID)
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ORCHESTRA_SOCKET", "ORCHESTRA_SESSION_ID", "ORCHESTRA_TOKEN"} {
		t.Setenv(key, env[key])
	}
	t.Setenv("ORCHESTRA_CONTEXT", "")
	run := func(args ...string) (string, error) {
		var out, stderr bytes.Buffer
		if RunCLI(args, &out, &stderr) != 0 {
			return "", fmt.Errorf("%s", strings.TrimSpace(stderr.String()))
		}
		return out.String(), nil
	}
	file := filepath.Join(t.TempDir(), "plan.md")
	_ = os.WriteFile(file, []byte("# Plan\n"), 0600)
	if _, err = run("scratchpad", "write", "--file", file); err != nil {
		t.Fatal(err)
	}
	if _, err = run("scratchpad", "append", "--text", "- step"); err != nil {
		t.Fatal(err)
	}
	if out, err := run("scratchpad", "read"); err != nil || out != "# Plan\n- step\n" {
		t.Fatalf("unexpected read: %q %v", out, err)
	}
	out, err := run("scratchpad", "read", "--json")
	if err != nil || !strings.Contains(out, `"revision":2`) {
		t.Fatalf("read --json lacks the revision: %q %v", out, err)
	}
	if _, err = run("scratchpad", "write", "--text", "x", "--if-revision", "1"); err == nil {
		t.Fatal("stale --if-revision accepted")
	}
	if _, err = run("scratchpad", "write", "--text", "x", "--if-revision", "2"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]string{{"scratchpad", "write"}, {"scratchpad", "write", "--text", "a", "--file", file}, {"scratchpad", "append", "--text", "a", "--if-revision", "3"}, {"scratchpad", "read", "--stdin"}, {"scratchpad", "write", "--text", "a", "--if-revision", "-1"}} {
		if _, err = run(bad...); err == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
	t.Setenv("ORCHESTRA_TOKEN", "")
	if _, err = run("scratchpad", "read"); err == nil || !strings.Contains(err.Error(), "Orchestra session") {
		t.Fatal("read outside a session", err)
	}
}
