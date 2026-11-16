package core

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCLICreateWorkspace(t *testing.T) {
	s := fixtureStore(t)
	origin := workspaceFixture(t, s)
	source := sessionFixture(t, s, origin, "claude")
	target := workspaceFixture(t, s)
	s.mu.Lock()
	p, _ := s.workspaceLocked(target.ID)
	p.Name = "Target"
	p.Repositories = append(p.Repositories, &Repository{ID: "second", Name: "Second", Directory: t.TempDir()})
	p.LastRepositoryID = "second"
	s.mu.Unlock()
	for _, selector := range []string{target.ID, "Target"} {
		sess, err := cliCreate(t, s, source, "--agent", "codex", "--workspace", selector)
		if err != nil || sess.RepositoryID != "second" || sess.Running {
			t.Fatalf("target: %+v %v", sess, err)
		}
		sess, err = cliCreate(t, s, source, "--agent", "codex", "--workspace", selector, "--repository", target.Repositories[0].ID)
		if err != nil || sess.RepositoryID != target.Repositories[0].ID {
			t.Fatalf("explicit repo: %+v %v", sess, err)
		}
	}
	sess, err := cliCreate(t, s, source, "--agent", "codex")
	if err != nil || sess.RepositoryID != source.RepositoryID {
		t.Fatalf("default changed: %+v %v", sess, err)
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--workspace", "missing"}, "does not exist"},
		{[]string{"--workspace", "target"}, "does not exist"},
		{[]string{"--workspace", ""}, "workspace must"},
		{[]string{"--workspace", target.ID, "--repository", source.RepositoryID}, "Repository unavailable"},
		{[]string{"--repository", target.Repositories[0].ID}, "Repository unavailable"},
		{[]string{"--workspace", target.ID, "--start"}, "Let Agents Start Sessions"},
	} {
		_, err := cliCreate(t, s, source, append([]string{"--agent", "codex"}, tc.args...)...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%v: %v", tc.args, err)
		}
	}
	// Duplicate names fail, while an ID always wins over names.
	s.mu.Lock()
	p, _ = s.workspaceLocked(target.ID)
	p.Name = origin.Name
	s.mu.Unlock()
	if _, err := cliCreate(t, s, source, "--agent", "codex", "--workspace", origin.Name); err == nil || !strings.Contains(err.Error(), "Use --workspace with a workspace ID") {
		t.Fatalf("ambiguity: %v", err)
	}
	if _, err := cliCreate(t, s, source, "--agent", "codex", "--workspace", target.ID); err != nil {
		t.Fatal(err)
	}
	// A workspace ID wins even when another workspace has that exact name.
	s.mu.Lock()
	original, _ := s.workspaceLocked(origin.ID)
	original.Name = target.ID
	s.mu.Unlock()
	byID, err := cliCreate(t, s, source, "--agent", "codex", "--workspace", target.ID)
	if err != nil || byID.RepositoryID == source.RepositoryID {
		t.Fatalf("ID precedence: %+v %v", byID, err)
	}
	// The parser rejects the option outside create before reaching the broker.
	for _, args := range [][]string{{"session", "list"}, {"session", "inbox"}, {"scratchpad", "read"}} {
		var out bytes.Buffer
		if err := runCLI(append(args, "--workspace", target.ID), &out); err == nil || !strings.Contains(err.Error(), "Unknown option --workspace") {
			t.Fatalf("parser scope: %v", err)
		}
	}
	s.mu.Lock()
	p, _ = s.workspaceLocked(target.ID)
	p.LastRepositoryID = "missing"
	s.mu.Unlock()
	fallback, err := cliCreate(t, s, source, "--agent", "codex", "--workspace", target.ID)
	if err != nil || fallback.RepositoryID != target.Repositories[0].ID {
		t.Fatalf("fallback: %+v %v", fallback, err)
	}
	// Raw broker requests cannot select workspaces for other methods or bypass authentication.
	req := WireRequest{Source: source.ID, Token: "invalid", Method: "session.create", Agent: "codex", Workspace: target.ID}
	if _, err := CallBroker(s.collaboration.listener.Addr().String(), req); err == nil {
		t.Fatal("invalid credentials accepted")
	}
	s.mu.Lock()
	req.Token = s.collaboration.tokens[source.ID]
	s.mu.Unlock()
	for _, method := range []string{"session.list", "session.inbox", "scratchpad.read", "session.request"} {
		req.Method = method
		if _, err := CallBroker(s.collaboration.listener.Addr().String(), req); err == nil || !strings.Contains(err.Error(), "only supported") {
			t.Fatalf("scope %s: %v", method, err)
		}
	}
	req.Workspace = ""
	req.Method = "session.list"
	data, err := CallBroker(s.collaboration.listener.Addr().String(), req)
	if err != nil {
		t.Fatal(err)
	}
	var listed []map[string]string
	if err := json.Unmarshal(data, &listed); err != nil {
		t.Fatal(err)
	}
	for _, item := range listed {
		if item["workspaceID"] != origin.ID {
			t.Fatalf("leaked session: %v", item)
		}
	}
}

func TestCLICreateTargetWorktreeAndStart(t *testing.T) {
	fakeAgent(t, "exec sleep 30\n")
	s := fixtureStore(t)
	origin := workspaceFixture(t, s)
	source := sessionFixture(t, s, origin, "claude")
	target, latest := localMainFixture(t, s)
	prefs := s.Snapshot().Preferences
	prefs.KeepLocalMainUpdated = true
	prefs.AgentsStartSessions = true
	if _, err := s.SetPreferences(prefs); err != nil {
		t.Fatal(err)
	}
	id := "f2c9c3b1-8429-4b87-b921-2cb438875caa"
	args := []string{"--agent", "claude", "--workspace", target.ID, "--worktree", "--start", "--request-id", id}
	first, err := cliCreate(t, s, source, args...)
	if err != nil || first.WorktreeBase != latest || !first.Running || !first.AgentStarted {
		t.Fatalf("target worktree/start: %+v %v", first, err)
	}
	again, err := cliCreate(t, s, source, args...)
	if err != nil || again.ID != first.ID {
		t.Fatalf("retry: %+v %v", again, err)
	}
	for i := 1; i < maxAgentStartedSessions; i++ {
		if _, err := cliCreate(t, s, source, "--agent", "claude", "--workspace", target.ID, "--start"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := cliCreate(t, s, source, "--agent", "claude", "--workspace", target.ID, "--start"); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("target limit: %v", err)
	}
	if _, err := cliCreate(t, s, source, "--agent", "claude", "--start"); err != nil {
		t.Fatalf("target limit affected source: %v", err)
	}
}

// Optional native-binary check against the same isolated app Store and Unix broker.
func TestWorkspaceNativeCLI(t *testing.T) {
	binary := os.Getenv("ORCHESTRA_TEST_CLI_BINARY")
	if binary == "" {
		t.Skip("set ORCHESTRA_TEST_CLI_BINARY to validate the built application CLI")
	}
	s := fixtureStore(t)
	origin := workspaceFixture(t, s)
	source := sessionFixture(t, s, origin, "claude")
	target := workspaceFixture(t, s)
	s.mu.Lock()
	env, err := s.collaboration.environmentLocked(source.ID)
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "--orchestra-cli", "session", "create", "--agent", "codex", "--workspace", target.ID)
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	data, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("native CLI: %s %v", data, err)
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		t.Fatal(err)
	}
	if sess.RepositoryID != target.Repositories[0].ID || sess.Running {
		t.Fatalf("native destination: %+v", sess)
	}
}
