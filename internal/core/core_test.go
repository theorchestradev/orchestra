package core

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func fixtureStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	exe, _ := os.Executable()
	s, err := NewStore(path, exe, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { shutdownAndWait(t, s) })
	return s
}

// shutdownAndWait stops the store and waits for its terminal processes to record their
// exit, so nothing writes into the test's temporary directory while it is removed.
func shutdownAndWait(t *testing.T, s *Store) {
	t.Helper()
	s.mu.Lock()
	terminals := make([]*TerminalSession, 0, len(s.terminals))
	for _, terminal := range s.terminals {
		terminals = append(terminals, terminal)
	}
	s.mu.Unlock()
	s.Shutdown()
	for _, terminal := range terminals {
		select {
		case <-terminal.done:
		case <-time.After(5 * time.Second):
			t.Error("terminal process did not stop after shutdown")
		}
	}
}
func projectFixture(t *testing.T, s *Store) *Project {
	t.Helper()
	state, err := s.CreateProject("Fixture", t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	return state.Projects[len(state.Projects)-1]
}
func sessionFixture(t *testing.T, s *Store, p *Project, agent string) *Session {
	t.Helper()
	state, err := s.CreateSession(p.ID, "", agent, "", "", false, nil, "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range state.Projects {
		if p.ID == state.SelectedProjectID {
			return p.Sessions[len(p.Sessions)-1]
		}
	}
	t.Fatal("session missing")
	return nil
}
func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()
	until := time.Now().Add(4 * time.Second)
	for time.Now().Before(until) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("fixture timed out")
}
func TestSidebarWidthPreferencesRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  float64
	}{{"280", 280}, {"248.4453125", 248.4453125}} {
		t.Run(tc.input, func(t *testing.T) {
			s := fixtureStore(t)
			var prefs Preferences
			payload := `{"appearance":"dark","theme":"system","terminalFontSize":14,"terminalFontSmoothing":true,"sidebarWidth":` + tc.input + `}`
			if err := json.Unmarshal([]byte(payload), &prefs); err != nil {
				t.Fatal(err)
			}
			if _, err := s.SetPreferences(prefs); err != nil {
				t.Fatal(err)
			}
			prefs.Theme = "ayu"
			if _, err := s.SetPreferences(prefs); err != nil {
				t.Fatal(err)
			}
			reopened, err := NewStore(s.path, s.executable, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Shutdown()
			state := reopened.Snapshot()
			if state.Error != "" || state.Preferences.SidebarWidth != tc.want || state.Preferences.Theme != "ayu" {
				t.Fatalf("preferences were not restored: %+v", state)
			}
			for _, width := range []float64{219.9, 400.1} {
				prefs.SidebarWidth = width
				if _, err := s.SetPreferences(prefs); err == nil {
					t.Fatalf("accepted out-of-range sidebar width %v", width)
				}
			}
		})
	}
}
func TestRemoveProjectPreservesFolderAndStopsOnlyItsSessions(t *testing.T) {
	s := fixtureStore(t)
	p := projectFixture(t, s)
	agent := sessionFixture(t, s, p, "claude")
	sender := sessionFixture(t, s, p, "pi")
	file := filepath.Join(p.Repositories[0].Directory, "keep.txt")
	if err := os.WriteFile(file, []byte("project files stay on disk"), 0600); err != nil {
		t.Fatal(err)
	}
	fixture := t.TempDir()
	shell := filepath.Join(fixture, "shell")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\nexec /bin/sh -c \"$4\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture, "claude"), []byte("#!/bin/sh\nprintf 'ready\\r\\n'\nwhile IFS= read -r line; do :; done\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", fixture+":"+os.Getenv("PATH"))
	if _, err := s.StartSession(agent.ID, false, 80, 24); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool {
		output, _ := s.TerminalSnapshot(agent.ID)
		data, _ := base64.StdEncoding.DecodeString(output.Data)
		return strings.Contains(string(data), "ready")
	})
	state, err := s.SendRequest(sender.ID, agent.ID, "isolated request")
	if err != nil {
		t.Fatal(err)
	}
	runID := state.Runs[0].ID
	keeper := projectFixture(t, s)
	if _, err = s.Select(keeper.ID, "scratchpad"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveScratchpad(keeper.ID, "keep other notes", 0); err != nil {
		t.Fatal(err)
	}
	unselected := projectFixture(t, s)
	if _, err = s.Select(p.ID, "project"); err != nil {
		t.Fatal(err)
	}
	if state, err = s.RemoveProject(unselected.ID); err != nil || state.SelectedProjectID != p.ID {
		t.Fatalf("removing another project changed selection: %+v %v", state, err)
	}
	s.mu.Lock()
	terminal, token, savedPath := s.terminals[agent.ID], s.collaboration.tokens[agent.ID], s.path
	s.path = t.TempDir() // Replacing a directory with the state file must fail.
	s.mu.Unlock()
	if _, err = s.RemoveProject(p.ID); err == nil {
		t.Fatal("removal succeeded despite failed persistence")
	}
	s.mu.Lock()
	s.path = savedPath
	retained := len(s.state.Projects) == 2 && s.state.SelectedProjectID == p.ID && !terminal.stopped && s.collaboration.tokens[agent.ID] == token
	s.mu.Unlock()
	if !retained {
		t.Fatal("failed removal changed metadata or stopped a session")
	}
	state, err = s.RemoveProject(p.ID)
	if err != nil || len(state.Projects) != 1 || state.SelectedProjectID != keeper.ID || state.Projects[0].ScratchpadText != "keep other notes" {
		t.Fatalf("project removal affected other metadata: %+v %v", state, err)
	}
	s.mu.Lock()
	_, live := s.terminals[agent.ID]
	_, authorized := s.collaboration.tokens[agent.ID]
	run, _ := s.collaboration.runLocked(runID)
	interrupted := run.Status == "interrupted"
	s.mu.Unlock()
	if live || authorized || !interrupted {
		t.Fatal("removed project's terminal, capability or pending request retained")
	}
	select {
	case <-terminal.done:
	case <-time.After(3 * time.Second):
		t.Fatal("removed project's process did not stop")
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "project files stay on disk" {
		t.Fatalf("project file changed: %q %v", data, err)
	}
	reopened, err := NewStore(savedPath, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	if restored := reopened.Snapshot(); len(restored.Projects) != 1 || restored.SelectedProjectID != keeper.ID {
		t.Fatal("removed project reappeared after reopening")
	}
	if state, err = s.RemoveProject(keeper.ID); err != nil || len(state.Projects) != 0 || state.SelectedProjectID != "" {
		t.Fatalf("last removal did not clear selection: %+v %v", state, err)
	}
	if _, err = s.RemoveProject(p.ID); err == nil {
		t.Fatal("accepted unavailable project")
	}
	if _, err = s.CreateProject("Re-added", p.Repositories[0].Directory, ""); err != nil {
		t.Fatal("removed project's folder could not be re-added", err)
	}
}
func TestWorkspacePersistence(t *testing.T) {
	s := fixtureStore(t)
	p := projectFixture(t, s)
	sess := sessionFixture(t, s, p, "claude")
	if _, err := s.Select(p.ID, "scratchpad"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveScratchpad(p.ID, "notes α\nsecond line", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Rename(sess.ID, "Renamed"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	var saved State
	_ = json.Unmarshal(data, &saved)
	if saved.Projects[0].Sessions[0].Running || saved.Projects[0].Sessions[0].Restored || saved.Runs != nil {
		t.Fatal("persisted live state")
	}
	other, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Shutdown()
	restored := other.Snapshot()
	if len(other.terminals) != 0 || restored.Projects[0].ScratchpadText != "notes α\nsecond line" || !restored.Projects[0].ShowsScratchpad || !restored.Projects[0].Sessions[0].Restored {
		t.Fatal("restore lost metadata or launched a process")
	}
}
func TestMoveProjectsAndSessionsPersistsOrder(t *testing.T) {
	s := fixtureStore(t)
	a, b, c := projectFixture(t, s), projectFixture(t, s), projectFixture(t, s)
	first, second, third := sessionFixture(t, s, b, "claude"), sessionFixture(t, s, b, "codex"), sessionFixture(t, s, b, "claude")
	other := sessionFixture(t, s, a, "claude")
	if _, err := s.MoveProject(c.ID, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveProject(a.ID, 99); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveSession(third.ID, 0); err != nil {
		t.Fatal(err)
	}
	state, err := s.MoveSession(first.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.MoveProject("missing", 0); err == nil {
		t.Fatal("moved a missing project")
	}
	ids := func(state State) string {
		var out []string
		for _, p := range state.Projects {
			out = append(out, p.ID)
			for _, sess := range p.Sessions {
				out = append(out, sess.ID)
			}
		}
		return strings.Join(out, ",")
	}
	want := strings.Join([]string{c.ID, b.ID, third.ID, second.ID, first.ID, a.ID, other.ID}, ",")
	if ids(state) != want {
		t.Fatalf("order = %s, want %s", ids(state), want)
	}
	restored, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Shutdown()
	if ids(restored.Snapshot()) != want {
		t.Fatal("order was not persisted")
	}
}
func TestCorruptStatePreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	raw := []byte(`{"version":999,"projects":[]}`)
	_ = os.WriteFile(path, raw, 0600)
	s, err := NewStore(path, "/tmp/test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()
	if s.Snapshot().Error == "" {
		t.Fatal("missing recovery error")
	}
	_, _ = s.CreateProject("x", t.TempDir(), "")
	after, _ := os.ReadFile(path)
	if !bytes.Equal(raw, after) {
		t.Fatal("corrupt state overwritten")
	}
}
func TestScratchpadRenameDeleteAndRecreate(t *testing.T) {
	s := fixtureStore(t)
	p := projectFixture(t, s)
	sess := sessionFixture(t, s, p, "claude")
	other := projectFixture(t, s)
	if _, err := s.Select(p.ID, "scratchpad"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveScratchpad(p.ID, "Keep these notes α", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RenameScratchpad(p.ID, "   "); err == nil {
		t.Fatal("accepted empty scratchpad name")
	}
	if _, err := s.RenameScratchpad(p.ID, "  Project Notes  "); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetScratchpadMode(p.ID, "preview"); err == nil {
		t.Fatal("accepted unknown scratchpad mode")
	}
	if _, err := s.SetScratchpadMode(p.ID, "read"); err != nil {
		t.Fatal(err)
	}
	checkSaved := func() *Project {
		t.Helper()
		reopened, err := NewStore(s.path, s.executable, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Shutdown()
		return reopened.Snapshot().Projects[0]
	}
	saved := checkSaved()
	if saved.ScratchpadName != "Project Notes" || saved.ScratchpadMode != "read" || saved.ScratchpadText != "Keep these notes α" || !saved.HasScratchpad || !saved.ShowsScratchpad || saved.Name != p.Name {
		t.Fatalf("scratchpad rename lost metadata: %+v", saved)
	}
	state, err := s.DeleteScratchpad(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	saved = checkSaved()
	if saved.HasScratchpad || saved.ShowsScratchpad || saved.ScratchpadName != "" || saved.ScratchpadMode != "" || saved.ScratchpadText != "" || saved.SelectedSessionID != sess.ID {
		t.Fatalf("scratchpad deletion did not persist cleanly: %+v", saved)
	}
	if len(state.Projects) != 2 || state.Projects[1].ID != other.ID || len(saved.Sessions) != 1 {
		t.Fatal("scratchpad deletion affected projects or sessions")
	}
	if _, err := s.SaveScratchpad(p.ID, "Late autosave", 1); err == nil {
		t.Fatal("late autosave accepted after deletion")
	}
	if _, err := s.SetScratchpadMode(p.ID, "read"); err == nil {
		t.Fatal("changed the mode of a deleted scratchpad")
	}
	if _, err := s.RenameScratchpad(p.ID, "Missing notes"); err == nil {
		t.Fatal("renamed deleted scratchpad")
	}
	if _, err := s.Select(p.ID, "scratchpad"); err != nil {
		t.Fatal(err)
	}
	saved = checkSaved()
	if !saved.HasScratchpad || !saved.ShowsScratchpad || saved.ScratchpadName != "Scratchpad" || saved.ScratchpadText != "" {
		t.Fatalf("scratchpad was not recreated empty: %+v", saved)
	}
}
func TestCollaborationRoutingAndRecovery(t *testing.T) {
	s := fixtureStore(t)
	p := projectFixture(t, s)
	a := sessionFixture(t, s, p, "claude")
	b := sessionFixture(t, s, p, "codex")
	other := projectFixture(t, s)
	cross := sessionFixture(t, s, other, "pi")
	if _, err := s.SendRequest(a.ID, cross.ID, "cross project"); err == nil {
		t.Fatal("cross project request accepted")
	}
	if _, err := s.SendRequest(a.ID, a.ID, "self"); err == nil {
		t.Fatal("self request accepted")
	}
	s.mu.Lock()
	env, err := s.collaboration.environmentLocked(a.ID)
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	bad := WireRequest{Method: "session.list", Source: a.ID, Token: "wrong"}
	if _, err = CallBroker(env["ORCHESTRA_SOCKET"], bad); err == nil {
		t.Fatal("unauthenticated socket request")
	}
	good := bad
	good.Token = env["ORCHESTRA_TOKEN"]
	data, err := CallBroker(env["ORCHESTRA_SOCKET"], good)
	if err != nil {
		t.Fatal(err)
	}
	var list []map[string]string
	_ = json.Unmarshal(data, &list)
	if len(list) != 2 {
		t.Fatal("project session isolation failed")
	}
	s.mu.Lock()
	run, err := s.collaboration.requestLocked(a.ID, b.ID, "Review fixture", "12345678-1234-1234-1234-123456789abc")
	if err != nil {
		t.Fatal(err)
	}
	id := run.ID
	retry, err := s.collaboration.requestLocked(a.ID, b.ID, "Review fixture", id)
	if err != nil || retry.ID != id {
		t.Fatal("idempotent retry failed")
	}
	if _, err = s.collaboration.requestLocked(a.ID, b.ID, "changed", id); err == nil {
		t.Fatal("conflicting retry accepted")
	}
	s.mu.Unlock()
	if _, err = s.ExecuteRequest(id); err == nil {
		t.Fatal("execution started a terminal")
	}
	if len(s.terminals) != 0 {
		t.Fatal("passive message launched agent")
	}
	s.mu.Lock()
	claimed, err := s.collaboration.inboxLocked(b.ID)
	if err != nil || claimed.ID != id {
		t.Fatal("inbox claim failed")
	}
	again, _ := s.collaboration.inboxLocked(b.ID)
	if again.ID != id {
		t.Fatal("inbox double claim")
	}
	if _, err = s.collaboration.replyLocked(id, a.ID, "wrong author"); err == nil {
		t.Fatal("sender replied")
	}
	if _, err = s.collaboration.replyLocked(id, b.ID, "done"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.collaboration.replyLocked(id, b.ID, "done"); err != nil {
		t.Fatal("reply retry failed")
	}
	s.mu.Unlock()
	state, err := s.SendRequest(a.ID, b.ID, "recover this request")
	if err != nil {
		t.Fatal(err)
	}
	recoverID := state.Runs[len(state.Runs)-1].ID
	s.mu.Lock()
	_, err = s.collaboration.inboxLocked(b.ID)
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	for _, r := range reopened.Snapshot().Runs {
		if r.ID == recoverID && r.Status != "interrupted" {
			t.Fatal("claimed request resent on restart")
		}
	}
	t.Setenv("ORCHESTRA_CONTEXT", env["ORCHESTRA_CONTEXT"])
	t.Setenv("ORCHESTRA_TOKEN", "")
	t.Setenv("ORCHESTRA_SESSION_ID", "")
	t.Setenv("ORCHESTRA_SOCKET", "")
	var out, stderr bytes.Buffer
	if RunCLI([]string{"session", "list"}, &out, &stderr) != 0 {
		t.Fatal(stderr.String())
	}
	if !strings.Contains(out.String(), a.ID) {
		t.Fatal("context fallback failed")
	}
}
func TestWaitCycleAndCancellation(t *testing.T) {
	s := fixtureStore(t)
	p := projectFixture(t, s)
	a := sessionFixture(t, s, p, "claude")
	b := sessionFixture(t, s, p, "pi")
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.collaboration.requestLocked(a.ID, b.ID, "a", " "); err == nil {
		t.Fatal("invalid request ID accepted")
	}
	r1, err := s.collaboration.requestLocked(a.ID, b.ID, "a", "")
	if err != nil {
		t.Fatal(err)
	}
	r2, err := s.collaboration.requestLocked(b.ID, a.ID, "b", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.collaboration.waitLocked(a.ID, r1); err != nil {
		t.Fatal(err)
	}
	if err = s.collaboration.waitLocked(b.ID, r2); err == nil {
		t.Fatal("wait cycle accepted")
	}
	if err = s.collaboration.cancelLocked(r1.ID); err != nil {
		t.Fatal(err)
	}
	if r1.Status != "cancelled" {
		t.Fatal("cancel failed")
	}
}
func TestAllAgentCommandsAndPromptQuoting(t *testing.T) {
	for agent := range agentSpecs {
		sess := &Session{ID: "id", Agent: agent, InitialPrompt: "literal ' $(touch /tmp/no)\nmessage"}
		command, err := launchCommand(sess, "/tmp/extensions")
		if err != nil {
			t.Fatal(err)
		}
		if agent != "grok" && !strings.Contains(command, shellQuote(sess.InitialPrompt)) {
			t.Fatalf("%s did not quote prompt", agent)
		}
		sess.HasLaunched = true
		sess.AgentSessionID = "resume ' id"
		command, err = launchCommand(sess, "/tmp/extensions")
		if err != nil || !strings.Contains(command, shellQuote(sess.AgentSessionID)) || strings.Contains(command, shellQuote(sess.InitialPrompt)) {
			t.Fatalf("%s resume resent prompt or lost ID", agent)
		}
	}
}
func TestAgentCommandsStartInAutoMode(t *testing.T) {
	modes := map[string]string{
		"claude":      " --permission-mode auto ",
		"codex":       " --approve-for-me",
		"grok":        " --permission-mode auto",
		"antigravity": "agy --mode accept-edits",
		"opencode":    "",
		"vibe":        "",
		"pi":          "",
		"cline":       " --auto-approve false",
		"omp":         "omp --approval-mode write ",
		"kilo":        "",
		"muse":        "muse --permission-profile :auto-review ",
	}
	for agent := range agentSpecs {
		mode, ok := modes[agent]
		if !ok {
			t.Fatalf("%s has no expected permission mode", agent)
		}
		for _, launched := range []bool{false, true} {
			sess := &Session{ID: "id", Agent: agent, InitialPrompt: "prompt", HasLaunched: launched, AgentSessionID: "resume-id"}
			command, err := launchCommand(sess, "/tmp/extensions")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(command, mode) {
				t.Fatalf("%s (resume %v) missing %q: %s", agent, launched, mode, command)
			}
			for _, flag := range []string{"--auto", "--dangerously", "bypassPermissions", "--always-approve", "--yolo"} {
				// Turning Cline's auto-approval off is not a bypass.
				if strings.Contains(strings.ReplaceAll(command, "--auto-approve false", ""), flag) {
					t.Fatalf("%s (resume %v) uses %s: %s", agent, launched, flag, command)
				}
			}
		}
	}
}
func TestTerminalPTYLifecycleAndExplicitExecution(t *testing.T) {
	s := fixtureStore(t)
	p := projectFixture(t, s)
	agent := sessionFixture(t, s, p, "claude")
	sender := sessionFixture(t, s, p, "pi")
	fixture := t.TempDir()
	shell := filepath.Join(fixture, "shell")
	_ = os.WriteFile(shell, []byte("#!/bin/sh\nexec /bin/sh -c \"$4\"\n"), 0700)
	fake := filepath.Join(fixture, "claude")
	_ = os.WriteFile(fake, []byte("#!/bin/sh\nprintf waiting > \"$ORCHESTRA_STATUS_FILE\"\nprintf fixture-conversation > \"$ORCHESTRA_AGENT_ID_FILE\"\nstty -echo\nprintf '\033[?2004hfixture α\\r\\n'\nwhile IFS= read -r line; do printf 'received: %s\\r\\n' \"$line\"; done\n"), 0700)
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", fixture+":"+os.Getenv("PATH"))
	if _, err := s.StartSession(agent.ID, false, 80, 24); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool {
		snapshot, _ := s.TerminalSnapshot(agent.ID)
		raw, _ := base64.StdEncoding.DecodeString(snapshot.Data)
		return strings.Contains(string(raw), "fixture α")
	})
	if err := s.ResizeTerminal(agent.ID, 120, 40); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteTerminal(agent.ID, "hello\n"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool {
		snapshot, _ := s.TerminalSnapshot(agent.ID)
		raw, _ := base64.StdEncoding.DecodeString(snapshot.Data)
		return strings.Contains(string(raw), "received: hello")
	})
	waitUntil(t, func() bool {
		for _, sess := range s.Snapshot().Projects[0].Sessions {
			if sess.ID == agent.ID {
				return sess.Activity == "waiting" && sess.AgentSessionID == "fixture-conversation"
			}
		}
		return false
	})
	if _, err := s.Select(p.ID, "scratchpad"); err != nil {
		t.Fatal(err)
	}
	if !s.Snapshot().Projects[0].Sessions[0].Running {
		t.Fatal("navigation terminated process")
	}
	state, err := s.SendRequest(sender.ID, agent.ID, "test request")
	if err != nil {
		t.Fatal(err)
	}
	run := state.Runs[0]
	if _, err = s.ExecuteRequest(run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExecuteRequest(run.ID); err == nil {
		t.Fatal("request executed twice")
	}
	if _, err = s.CloseSession(agent.ID, false); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	_, exists := s.terminals[agent.ID]
	s.mu.Unlock()
	if exists {
		t.Fatal("closed PTY retained")
	}
}
func TestRepositoryAndTerminalTrustBoundaries(t *testing.T) {
	for _, remote := range []string{"git@github.com:owner/repo.git", "https://github.com/owner/repo.git", "ssh://git@ssh.github.com/owner/repo"} {
		if ParseRepository(remote) != "owner/repo" {
			t.Fatal(remote)
		}
	}
	for _, remote := range []string{"https://github.com.evil/owner/repo", "https://github.com/../repo", "https://github.com/owner/repo/extra", "git@other:owner/repo"} {
		if ParseRepository(remote) != "" {
			t.Fatal(remote)
		}
	}
	if _, err := prepareRequestInput("bad\x1b[2J", true); err == nil {
		t.Fatal("control injection accepted")
	}
	value, _ := prepareRequestInput("a\nb\tc", false)
	if value != "a b c\r" {
		t.Fatal("unbracketed request made multiple submissions")
	}
	value, _ = prepareRequestInput("a\nb", true)
	if value != "\x1b[200~a\nb\x1b[201~\r" {
		t.Fatal("bracketed request framing")
	}
}

func TestGitHubCommandResolvesBeforeCreation(t *testing.T) {
	directory := t.TempDir()
	// Finder-launched apps may have no Homebrew entry in their inherited PATH.
	t.Setenv("PATH", "/usr/bin:/bin")
	executable := filepath.Join(directory, "gh")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '%s' \"$1\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	// Skip a directory without gh; system directories could hold a real gh, as on CI runners.
	cmd, err := githubCommand(context.Background(), t.TempDir()+string(filepath.ListSeparator)+directory, "api")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Err != nil {
		t.Fatalf("cached executable lookup error: %v", cmd.Err)
	}
	output, err := cmd.CombinedOutput()
	if err != nil || string(output) != "api" {
		t.Fatalf("output %q, error %v", output, err)
	}
	if _, err = githubCommand(context.Background(), t.TempDir()); err == nil {
		t.Fatal("missing executable must return a useful error")
	}
	// A process failure without stderr must still expose the underlying error.
	script := filepath.Join(directory, "fail")
	if err = os.WriteFile(script, []byte("#!/bin/sh\nexit 7\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = runCommand(directory, script); err == nil || !strings.Contains(err.Error(), "exit status 7") {
		t.Fatalf("lost process error: %v", err)
	}
}
func TestCreateSessionNames(t *testing.T) {
	s := fixtureStore(t)
	p := projectFixture(t, s)
	long := strings.Repeat("é", 70)
	for _, tc := range []struct{ name, want string }{
		{"", "Claude Code 1"},
		{"  \t\n ", "Claude Code 2"},
		{"  Fix   the\tsidebar  ", "Fix the sidebar"},
		{strings.Repeat("a", maxSessionNameLength), strings.Repeat("a", maxSessionNameLength)},
		{long, strings.Repeat("é", maxSessionNameLength-1) + "…"},
		{"#28 Confirm before quitting when work is in progress", "#28 Confirm before quitting when work is in progress"},
		{"#28 " + long, "#28 " + strings.Repeat("é", maxSessionNameLength-5) + "…"},
		{"#7 ", "#7"},
	} {
		state, err := s.CreateSession(p.ID, "", "claude", tc.name, "", false, nil, "", "", "", "", "")
		if err != nil {
			t.Fatal(err)
		}
		sessions := state.Projects[len(state.Projects)-1].Sessions
		got := sessions[len(sessions)-1].Name
		if got != tc.want || !utf8.ValidString(got) {
			t.Fatalf("name %q became %q, want %q", tc.name, got, tc.want)
		}
	}
	sessions := s.Snapshot().Projects[len(s.Snapshot().Projects)-1].Sessions
	state, err := s.Rename(sessions[len(sessions)-1].ID, "Renamed")
	if err != nil {
		t.Fatal(err)
	}
	if sessions = state.Projects[len(state.Projects)-1].Sessions; sessions[len(sessions)-1].Name != "Renamed" {
		t.Fatal("rename failed")
	}
}
func TestSessionGitHubItemPersists(t *testing.T) {
	s := fixtureStore(t)
	p := projectFixture(t, s)
	if _, err := s.CreateSession(p.ID, "", "claude", "#22 Show sessions", "", false, &GitHubItem{Number: 22}, "", "", "", "", ""); err != nil {
		t.Fatal(err)
	}
	state, err := s.CreateSession(p.ID, "", "pi", "#23 Review", "", false, &GitHubItem{Number: 23, PullRequest: true}, "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateSession(p.ID, "", "claude", "", "", false, &GitHubItem{}, "", "", "", "", ""); err == nil {
		t.Fatal("accepted an invalid GitHub item")
	}
	if state, err = s.CreateSession(p.ID, "", terminalAgent, "", "", false, &GitHubItem{Number: 24}, "", "", "", "", ""); err != nil || state.Projects[0].Sessions[2].GitHubItem != nil {
		t.Fatalf("terminal kept a GitHub item: %+v, %v", state.Projects[0].Sessions[2].GitHubItem, err)
	}
	issue, review := state.Projects[0].Sessions[0], state.Projects[0].Sessions[1]
	if _, err = s.Rename(issue.ID, "Renamed"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CloseSession(review.ID, false); err != nil {
		t.Fatal(err)
	}
	path := s.path
	s.Shutdown()
	exe, _ := os.Executable()
	restored, err := NewStore(path, exe, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restored.Shutdown)
	sessions := restored.Snapshot().Projects[0].Sessions
	if len(sessions) != 2 || sessions[0].Name != "Renamed" || sessions[0].GitHubItem == nil || *sessions[0].GitHubItem != (GitHubItem{Number: 22}) || sessions[1].GitHubItem != nil {
		t.Fatalf("restored sessions lost their GitHub items: %+v", sessions)
	}
}
func TestPlainTerminalLaunchAndDirectory(t *testing.T) {
	s := fixtureStore(t)
	p := projectFixture(t, s)
	sender := sessionFixture(t, s, p, "claude")
	state, err := s.CreateSession(p.ID, "", terminalAgent, "", "ignored prompt", true, nil, "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	terminal := state.Projects[0].Sessions[1]
	if terminal.Name != "Terminal 1" || terminal.InitialPrompt != "" || terminal.WorktreeDirectory != "" || terminal.AgentSessionID != "" {
		t.Fatalf("unexpected terminal session: %+v", terminal)
	}
	fixture := t.TempDir()
	shell := filepath.Join(fixture, "user shell")
	_ = os.WriteFile(shell, []byte("#!/bin/sh\nif [ \"$3\" = -c ]; then exec /bin/sh -c \"$4\"; fi\nprintf 'args=%s cwd=%s\\r\\n' \"$*\" \"$(pwd -P)\"\nwhile IFS= read -r line; do :; done\n"), 0700)
	t.Setenv("SHELL", shell)
	command, err := launchCommand(terminal, "/tmp/extensions")
	if err != nil || command != shellQuote(shell)+" -l" {
		t.Fatalf("terminal command = %q, %v", command, err)
	}
	resolved, _ := filepath.EvalSymlinks(p.Repositories[0].Directory)
	if _, err = s.StartSession(terminal.ID, false, 80, 24); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool {
		output, _ := s.TerminalSnapshot(terminal.ID)
		data, _ := base64.StdEncoding.DecodeString(output.Data)
		return strings.Contains(string(data), "args=-l cwd="+resolved)
	})
	if _, err = s.SendRequest(sender.ID, terminal.ID, "hello"); err == nil {
		t.Fatal("terminal accepted a request")
	}
	prefs := s.Snapshot().Preferences
	prefs.DefaultAgent = terminalAgent
	if _, err = s.SetPreferences(prefs); err == nil {
		t.Fatal("terminal accepted as default agent")
	}
	reopened, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer shutdownAndWait(t, reopened)
	restored := reopened.Snapshot().Projects[0].Sessions[1]
	if reopened.Snapshot().Error != "" || !restored.Restored || !restored.HasLaunched {
		t.Fatalf("terminal was not restored: %+v", restored)
	}
	if _, err = reopened.StartSession(restored.ID, false, 80, 24); err != nil {
		t.Fatalf("restored terminal did not start a fresh shell: %v", err)
	}
}
func TestMoveScratchpadAmongSessionsPersistsPosition(t *testing.T) {
	s := fixtureStore(t)
	p := projectFixture(t, s)
	a, b, c := sessionFixture(t, s, p, "claude"), sessionFixture(t, s, p, "terminal"), sessionFixture(t, s, p, "codex")
	if _, err := s.MoveScratchpad(p.ID, 1); err == nil {
		t.Fatal("moved a missing scratchpad")
	}
	if _, err := s.Select(p.ID, "scratchpad"); err != nil {
		t.Fatal(err)
	}
	rows := func(p *Project) string {
		var out []string
		for _, sess := range p.Sessions {
			out = append(out, sess.ID)
		}
		if p.HasScratchpad {
			out = slices.Insert(out, p.ScratchpadIndex, "scratchpad")
		}
		return strings.Join(out, ",")
	}
	reopen := func() *Project {
		t.Helper()
		reopened, err := NewStore(s.path, s.executable, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Shutdown()
		return reopened.Snapshot().Projects[0]
	}
	if got, want := rows(reopen()), strings.Join([]string{"scratchpad", a.ID, b.ID, c.ID}, ","); got != want {
		t.Fatalf("default rows = %s, want %s", got, want)
	}
	if _, err := s.MoveScratchpad(p.ID, 2); err != nil {
		t.Fatal(err)
	}
	if got, want := rows(reopen()), strings.Join([]string{a.ID, b.ID, "scratchpad", c.ID}, ","); got != want {
		t.Fatalf("after moving scratchpad = %s, want %s", got, want)
	}
	if _, err := s.MoveSession(c.ID, 0); err != nil {
		t.Fatal(err)
	}
	state, err := s.MoveSession(a.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := rows(state.Projects[0]), strings.Join([]string{c.ID, b.ID, "scratchpad", a.ID}, ","); got != want {
		t.Fatalf("after moving sessions = %s, want %s", got, want)
	}
	if state, err = s.CloseSession(c.ID, false); err != nil {
		t.Fatal(err)
	}
	if got, want := rows(reopen()), strings.Join([]string{b.ID, "scratchpad", a.ID}, ","); got != want {
		t.Fatalf("after closing = %s, want %s", got, want)
	}
	if _, err = s.MoveScratchpad(p.ID, 99); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DeleteScratchpad(p.ID); err != nil {
		t.Fatal(err)
	}
	if state, err = s.Select(p.ID, "scratchpad"); err != nil {
		t.Fatal(err)
	}
	if got, want := rows(state.Projects[0]), strings.Join([]string{"scratchpad", b.ID, a.ID}, ","); got != want {
		t.Fatalf("recreated scratchpad = %s, want %s", got, want)
	}
}
func TestScratchpadIndexDefaultsAndClampsOnLoad(t *testing.T) {
	s := fixtureStore(t)
	p := projectFixture(t, s)
	sessionFixture(t, s, p, "claude")
	sessionFixture(t, s, p, "terminal")
	if _, err := s.Select(p.ID, "scratchpad"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveScratchpad(p.ID, 1); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	load := func(raw string) *Project {
		t.Helper()
		path := filepath.Join(t.TempDir(), "state.json")
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		reopened, err := NewStore(path, s.executable, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Shutdown()
		return reopened.Snapshot().Projects[0]
	}
	index := regexp.MustCompile(`"scratchpadIndex":\s*1,`)
	if !index.Match(saved) {
		t.Fatalf("scratchpad index not saved: %s", saved)
	}
	if got := load(index.ReplaceAllString(string(saved), "")); got.ScratchpadIndex != 0 || !got.HasScratchpad {
		t.Fatalf("existing workspace scratchpad index = %d, want 0", got.ScratchpadIndex)
	}
	if got := load(index.ReplaceAllString(string(saved), `"scratchpadIndex": 9,`)); got.ScratchpadIndex != 2 {
		t.Fatalf("out of range scratchpad index = %d, want 2", got.ScratchpadIndex)
	}
}
func TestProjectDescriptionPersistsAndClears(t *testing.T) {
	s := fixtureStore(t)
	state, err := s.CreateProject("Described", t.TempDir(), "  Client API  ")
	if err != nil {
		t.Fatal(err)
	}
	p := state.Projects[len(state.Projects)-1]
	if p.Description != "Client API" {
		t.Fatalf("description was not trimmed: %q", p.Description)
	}
	if _, err = s.EditProject(p.ID, "Renamed", "Billing service\nfor invoices", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.EditProject(p.ID, " ", "x", ""); err == nil {
		t.Fatal("accepted an empty project name")
	}
	reopened, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.Snapshot().Projects[0]
	reopened.Shutdown()
	if got.Name != "Renamed" || got.Description != "Billing service\nfor invoices" {
		t.Fatalf("description was not restored: %+v", got)
	}
	if state, err = s.EditProject(p.ID, "Renamed", "   ", ""); err != nil || state.Projects[0].Description != "" {
		t.Fatalf("description was not cleared: %+v %v", state.Projects[0], err)
	}
	data, err := os.ReadFile(s.path)
	if err != nil || bytes.Contains(data, []byte(`"description"`)) {
		t.Fatalf("cleared description was saved: %s %v", data, err)
	}
}
func TestProjectEmojiPersistsAndClears(t *testing.T) {
	s := fixtureStore(t)
	state, err := s.CreateProject("Emoji", t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	p := state.Projects[len(state.Projects)-1]
	for _, emoji := range []string{"🚀", "👍🏽", "🇧🇷", "👨‍💻", "1️⃣"} {
		if state, err = s.EditProject(p.ID, "Emoji", "", " "+emoji+" "); err != nil {
			t.Fatalf("rejected %q: %v", emoji, err)
		}
		if got := state.Projects[len(state.Projects)-1]; got.Emoji != emoji || got.Name != "Emoji" {
			t.Fatalf("emoji was not stored apart from the name: %+v", got)
		}
	}
	for _, emoji := range []string{"🚀🚀", "a b", "\a", "👨 💻"} {
		if _, err = s.EditProject(p.ID, "Emoji", "", emoji); err == nil {
			t.Fatalf("accepted %q", emoji)
		}
	}
	reopened, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.Snapshot().Projects[len(state.Projects)-1]
	reopened.Shutdown()
	if got.Emoji != "1️⃣" {
		t.Fatalf("emoji was not restored: %+v", got)
	}
	if state, err = s.EditProject(p.ID, "Emoji", "", ""); err != nil || state.Projects[len(state.Projects)-1].Emoji != "" {
		t.Fatalf("emoji was not cleared: %v", err)
	}
	data, err := os.ReadFile(s.path)
	if err != nil || bytes.Contains(data, []byte(`"emoji"`)) {
		t.Fatalf("cleared emoji was saved: %s %v", data, err)
	}
}
func TestAutomaticRequestDelivery(t *testing.T) {
	s := fixtureStore(t)
	p := projectFixture(t, s)
	agent := sessionFixture(t, s, p, "claude")
	stopped := sessionFixture(t, s, p, "codex")
	sender := sessionFixture(t, s, p, "pi")
	fixture := t.TempDir()
	shell := filepath.Join(fixture, "shell")
	_ = os.WriteFile(shell, []byte("#!/bin/sh\nexec /bin/sh -c \"$4\"\n"), 0700)
	fake := filepath.Join(fixture, "claude")
	_ = os.WriteFile(fake, []byte("#!/bin/sh\nprintf waiting > \"$ORCHESTRA_STATUS_FILE\"\nstty -echo\nprintf '\033[?2004hready\\r\\n'\nwhile IFS= read -r line; do printf 'received: %s\\r\\n' \"$line\"; done\n"), 0700)
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", fixture+":"+os.Getenv("PATH"))
	if _, err := s.StartSession(agent.ID, false, 80, 24); err != nil {
		t.Fatal(err)
	}
	activity := func(value string) {
		t.Helper()
		s.mu.Lock()
		file := s.terminals[agent.ID].statusFile
		s.mu.Unlock()
		_ = os.WriteFile(file, []byte(value), 0600)
		waitUntil(t, func() bool {
			s.mu.Lock()
			defer s.mu.Unlock()
			_, sess, _ := s.sessionLocked(agent.ID)
			return sess.Activity == value
		})
	}
	status := func(id string) string {
		s.mu.Lock()
		defer s.mu.Unlock()
		run, _ := s.collaboration.runLocked(id)
		return run.Status
	}
	send := func(to, text string) string {
		t.Helper()
		state, err := s.SendRequest(sender.ID, to, text)
		if err != nil {
			t.Fatal(err)
		}
		return state.Runs[len(state.Runs)-1].ID
	}
	reply := func(id string) {
		t.Helper()
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, err := s.collaboration.replyLocked(id, agent.ID, "done"); err != nil {
			t.Fatal(err)
		}
	}
	output := func(text string) {
		t.Helper()
		waitUntil(t, func() bool {
			snapshot, _ := s.TerminalSnapshot(agent.ID)
			raw, _ := base64.StdEncoding.DecodeString(snapshot.Data)
			return strings.Contains(string(raw), text)
		})
	}
	output("ready")
	activity("waiting")

	first := send(agent.ID, "first")
	if status(first) != "queued" {
		t.Fatal("request delivered with the setting off")
	}
	prefs := s.Snapshot().Preferences
	prefs.DeliverRequests = true
	if _, err := s.SetPreferences(prefs); err != nil {
		t.Fatal(err)
	}
	if status(first) != "running" {
		t.Fatal("queued request not delivered to a free agent")
	}
	output("Orchestra request " + first)
	if selected := s.Snapshot().Projects[0].SelectedSessionID; selected != sender.ID {
		t.Fatal("automatic delivery changed the selection")
	}

	second, third := send(agent.ID, "second"), send(agent.ID, "third")
	idle := send(stopped.ID, "not running")
	activity("working")
	reply(first)
	if status(second) != "queued" {
		t.Fatal("request delivered to a busy agent")
	}
	activity("waiting")
	if status(second) != "running" || status(third) != "queued" {
		t.Fatal("requests not delivered one at a time, oldest first")
	}

	activity("working")
	reply(second)
	if err := s.WriteTerminal(agent.ID, "draft\x1b[D"); err != nil {
		t.Fatal(err)
	}
	activity("waiting")
	if status(third) != "queued" {
		t.Fatal("request mixed with a draft")
	}
	if err := s.WriteTerminal(agent.ID, "\r"); err != nil {
		t.Fatal(err)
	}
	activity("working")
	activity("waiting")
	if status(third) != "running" {
		t.Fatal("request not delivered after the draft was submitted")
	}

	cancelled := send(agent.ID, "cancelled")
	if _, err := s.CancelRequest(cancelled); err != nil {
		t.Fatal(err)
	}
	reply(third)
	if status(cancelled) != "cancelled" || status(idle) != "queued" {
		t.Fatal("cancelled request delivered or stopped session received input")
	}
	if _, err := s.CloseSession(agent.ID, false); err != nil {
		t.Fatal(err)
	}
}
func TestDraftLength(t *testing.T) {
	for _, c := range []struct {
		draft int
		data  string
		want  int
	}{
		{0, "abc", 3}, {3, "\x7f", 2}, {2, "\r", 0}, {0, "\x1b[A\x1b[I\x1bOB", 0}, {4, "\x15", 0},
		{0, "\x1b[200~line\rline\x1b[201~", 9}, {1, "\x1b\r", 1}, {2, "\x03", 0}, {0, "ação", 4},
	} {
		if got := draftLength(c.draft, c.data); got != c.want {
			t.Errorf("draftLength(%d, %q) = %d, want %d", c.draft, c.data, got, c.want)
		}
	}
}
func TestMigrateStateMovesWorkspaceOnce(t *testing.T) {
	home := t.TempDir()
	from, dir := filepath.Join(home, "Library", "Application Support", "Orchestra Rewrite"), filepath.Join(home, ".orchestra")
	for name, data := range map[string]string{"state.json": `{"version":1}`, "collaboration.sqlite": "history", "AgentSessions/Vibe/a/log": "vibe", "Worktrees/repo/abc/file": "work"} {
		_ = os.MkdirAll(filepath.Dir(filepath.Join(from, name)), 0700)
		_ = os.WriteFile(filepath.Join(from, name), []byte(data), 0600)
	}
	_ = os.MkdirAll(filepath.Join(dir, "worktrees"), 0700)
	if err := MigrateState(from, dir); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"state.json": `{"version":1}`, "collaboration.sqlite": "history", "AgentSessions/Vibe/a/log": "vibe"} {
		if got, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(got) != data {
			t.Fatalf("%s not moved: %q %v", name, got, err)
		}
		if _, err := os.Stat(filepath.Join(from, name)); !os.IsNotExist(err) {
			t.Fatalf("%s left behind", name)
		}
	}
	if _, err := os.Stat(filepath.Join(from, "Worktrees/repo/abc/file")); err != nil {
		t.Fatal("existing worktree moved", err)
	}
	if err := MigrateState(from, dir); err != nil {
		t.Fatal("second migration", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "state.json")); string(got) != `{"version":1}` {
		t.Fatal("second migration changed the workspace")
	}
	_ = os.RemoveAll(filepath.Join(from, "Worktrees"))
	_ = os.WriteFile(filepath.Join(from, "state.json"), []byte("stale"), 0600)
	if err := MigrateState(from, dir); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(from, "state.json")); string(got) != "stale" {
		t.Fatal("migrated over an existing workspace")
	}
}
func TestMigrateStateResumesAndReportsConflicts(t *testing.T) {
	home := t.TempDir()
	from, dir := filepath.Join(home, "old"), filepath.Join(home, "new")
	_ = os.MkdirAll(dir, 0700)
	_ = os.MkdirAll(from, 0700)
	_ = os.WriteFile(filepath.Join(from, "state.json"), []byte("state"), 0600)
	_ = os.WriteFile(filepath.Join(from, "collaboration.sqlite"), []byte("old"), 0600)
	_ = os.WriteFile(filepath.Join(dir, "collaboration.sqlite"), []byte("new"), 0600)
	if err := MigrateState(from, dir); err == nil {
		t.Fatal("overwrote existing history")
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "collaboration.sqlite")); string(got) != "new" {
		t.Fatal("conflict changed the target")
	}
	if _, err := os.Stat(filepath.Join(from, "state.json")); err != nil {
		t.Fatal("conflict moved the workspace", err)
	}
	_ = os.Remove(filepath.Join(from, "collaboration.sqlite"))
	if err := MigrateState(from, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Fatal("empty previous folder kept")
	}
	if err := MigrateState(filepath.Join(home, "missing"), filepath.Join(home, "fresh")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "fresh")); !os.IsNotExist(err) {
		t.Fatal("fresh install created a folder without a workspace")
	}
}
func TestSaveAgentIDReadsHookPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id")
	if err := saveAgentID(strings.NewReader(`{"session_id":"abc-123","prompt":"`+strings.Repeat("x", 100000)+`"}`), path); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "abc-123" {
		t.Fatalf("saved %q", got)
	}
	for _, payload := range []string{`{"prompt":"no id"}`, "not json"} {
		if err := saveAgentID(strings.NewReader(payload), path); err == nil {
			t.Fatalf("%s accepted", payload)
		}
	}
	if got, _ := os.ReadFile(path); string(got) != "abc-123" {
		t.Fatal("invalid payload replaced the saved ID")
	}
	if err := saveAgentID(strings.NewReader(`{"taskId":"conv_1","sessionContext":{"rootSessionId":"1792454132274_9mr36"}}`), path); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "1792454132274_9mr36" {
		t.Fatalf("saved %q from a Cline payload", got)
	}
	if strings.Contains(statusCommand("waiting"), "plutil") {
		t.Fatal("hooks depend on macOS tools")
	}
}
func TestLoginShellFallback(t *testing.T) {
	t.Setenv("SHELL", "")
	if shell := loginShell(); shell == "" || !filepath.IsAbs(shell) {
		t.Fatalf("unusable fallback shell %q", shell)
	}
}

func TestMissingClaudeConversationOffersNewConversation(t *testing.T) {
	s := fixtureStore(t)
	p := projectFixture(t, s)
	agent := sessionFixture(t, s, p, "claude")
	fixture := t.TempDir()
	shell := filepath.Join(fixture, "shell")
	_ = os.WriteFile(shell, []byte("#!/bin/sh\nexec /bin/sh -c \"$4\"\n"), 0700)
	// Like Claude Code, the fake has no transcript to resume and exits with code 1.
	_ = os.WriteFile(filepath.Join(fixture, "claude"), []byte("#!/bin/sh\nif [ \"$1\" = --resume ]; then printf 'No conversation found with session ID: %s\\r\\n' \"$2\"; exit 1; fi\nprintf 'started %s\\r\\n' \"$1\"\n"), 0700)
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", fixture+":"+os.Getenv("PATH"))
	run := func(newConversation bool) Session {
		t.Helper()
		if _, err := s.StartSession(agent.ID, newConversation, 80, 24); err != nil {
			t.Fatal(err)
		}
		var sess Session
		waitUntil(t, func() bool {
			s.mu.Lock()
			defer s.mu.Unlock()
			_, current, _ := s.sessionLocked(agent.ID)
			sess = *current
			return !current.Running
		})
		return sess
	}
	if sess := run(false); sess.AgentSessionID != agent.ID {
		t.Fatalf("first launch lost the preset conversation ID: %+v", sess)
	}
	sess := run(false)
	if sess.AgentSessionID != "" || !strings.Contains(sess.ExitMessage, "no saved conversation") {
		t.Fatalf("failed resume kept a dead conversation: %q %q", sess.AgentSessionID, sess.ExitMessage)
	}
	if _, err := s.StartSession(agent.ID, false, 80, 24); err == nil {
		t.Fatal("resumed without a conversation ID")
	}
	if sess = run(true); sess.AgentSessionID != agent.ID || sess.ExitMessage != "Claude Code exited (code 0)." {
		t.Fatalf("new conversation did not start: %q %q", sess.AgentSessionID, sess.ExitMessage)
	}
}
