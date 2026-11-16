package core

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAutomationSchedule(t *testing.T) {
	cases := []struct {
		name        string
		a           Automation
		after, want string
	}{
		{"weekly", Automation{Days: []int{1}, Time: "09:00", TimeZone: "America/Sao_Paulo"}, "2026-11-02T11:59:00Z", "2026-11-02T12:00:00Z"},
		{"next week", Automation{Days: []int{1}, Time: "09:00", TimeZone: "America/Sao_Paulo"}, "2026-11-02T12:00:00Z", "2026-11-09T12:00:00Z"},
		{"once", Automation{Date: "2026-11-03", Time: "09:00", TimeZone: "UTC"}, "2026-11-02T12:00:00Z", "2026-11-03T09:00:00Z"},
		{"DST gap", Automation{Days: []int{0}, Time: "02:30", TimeZone: "America/New_York"}, "2026-03-07T12:00:00Z", "2026-03-15T06:30:00Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			after, _ := time.Parse(time.RFC3339, tc.after)
			got, err := tc.a.next(after)
			if err != nil || got.UTC().Format(time.RFC3339) != tc.want {
				t.Fatalf("next = %s, %v; want %s", got, err, tc.want)
			}
		})
	}
	for _, a := range []Automation{
		{Time: "25:00", TimeZone: "UTC", Days: []int{1}},
		{Time: "09:00", TimeZone: "Unknown", Days: []int{1}},
		{Time: "09:00", TimeZone: "UTC", Days: []int{7}},
		{Time: "09:00", TimeZone: "UTC", Date: "2000-01-01"},
	} {
		if _, err := a.next(time.Now()); err == nil {
			t.Fatalf("accepted invalid schedule: %+v", a)
		}
	}
}

func TestAutomationPersistenceAndFailedRun(t *testing.T) {
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	a := Automation{Name: "Review", WorkspaceID: p.ID, Agent: "codex", Prompt: "Review code", Enabled: true, Days: []int{1}, Time: "09:00", TimeZone: "UTC"}
	state, err := s.SaveAutomation(a)
	if err != nil {
		t.Fatal(err)
	}
	a = state.Automations[0]
	reopened, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	if got := reopened.Snapshot().Automations; len(got) != 1 || got[0].ID != a.ID || got[0].Prompt != a.Prompt {
		t.Fatalf("not persisted: %+v", got)
	}
	// A removed workspace must produce a recorded failure without contacting any agent.
	s.mu.Lock()
	s.state.Automations[0].WorkspaceID = "missing"
	due := s.state.Automations[0].NextRun
	s.mu.Unlock()
	s.runDueAutomations(due)
	first := s.Snapshot().Automations[0]
	if first.LastError == "" || !first.LastRun.Equal(due) || !first.NextRun.After(due) {
		t.Fatalf("bad failed run: %+v", first)
	}
	s.runDueAutomations(due)
	if got := s.Snapshot().Automations[0]; !got.NextRun.Equal(first.NextRun) {
		t.Fatal("duplicate occurrence")
	}
	// Failed persistence rolls back the in-memory edit.
	// A regular file cannot become a parent directory.
	s.path = filepath.Join(reopened.path, "child")
	if _, err := s.DeleteAutomation(a.ID); err == nil {
		t.Fatal("expected write failure")
	}
	if len(s.Snapshot().Automations) != 1 {
		t.Fatal("failed delete changed state")
	}
}

func TestAutomationStartsFreshSessionOnce(t *testing.T) {
	fakeAgent(t, "printf '%s\\n' \"$@\"\nexec sleep 30\n")
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	original := sessionFixture(t, s, p, "claude")
	a := Automation{Name: "Scheduled review", WorkspaceID: p.ID, Agent: "claude", Prompt: "Only a fake agent receives this", Enabled: true, Date: time.Now().AddDate(0, 0, 1).Format("2006-01-02"), Time: "09:00", TimeZone: "UTC"}
	state, err := s.SaveAutomation(a)
	if err != nil {
		t.Fatal(err)
	}
	due := state.Automations[0].NextRun
	s.runDueAutomations(due)
	state = s.Snapshot()
	got := state.Automations[0]
	if got.Enabled || got.LastError != "" || got.LastSessionID == "" {
		t.Fatalf("run failed: %+v", got)
	}
	if state.Workspaces[0].SelectedSessionID != original.ID {
		t.Fatal("automation changed selected session")
	}
	sess := sessionState(s, got.LastSessionID)
	if len(got.Runs) != 1 || got.Runs[0].SessionID != sess.ID || got.Runs[0].Status != "started" {
		t.Fatalf("missing session history: %+v", got.Runs)
	}
	if !sess.Running || sess.Agent != "claude" || sess.InitialPrompt != a.Prompt {
		t.Fatalf("wrong session: %+v", sess)
	}
	s.runDueAutomations(due.Add(time.Minute))
	if len(s.Snapshot().Workspaces[0].Sessions) != 2 {
		t.Fatal("one-time automation ran twice")
	}
}

func TestCLICreateSessionDoesNotLaunch(t *testing.T) {
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	source := sessionFixture(t, s, p, "claude")
	s.mu.Lock()
	env, err := s.collaboration.environmentLocked(source.ID)
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range env {
		t.Setenv(key, value)
	}
	var out bytes.Buffer
	if err := runCLI([]string{"session", "create", "--agent", "codex", "--prompt", "A saved prompt", "--name", "From CLI"}, &out); err != nil {
		t.Fatal(err)
	}
	var sess Session
	if err := json.Unmarshal(out.Bytes(), &sess); err != nil {
		t.Fatal(err)
	}
	state := s.Snapshot()
	if sess.ID == "" || sess.Name != "From CLI" || sess.InitialPrompt != "A saved prompt" || len(state.Workspaces[0].Sessions) != 2 {
		t.Fatalf("wrong created session: %s", out.String())
	}
	if sess.Running || sess.HasLaunched || len(s.terminals) != 0 {
		t.Fatal("CLI launched a process")
	}
	if state.Workspaces[0].SelectedSessionID != source.ID {
		t.Fatal("CLI changed selection")
	}
	out.Reset()
	if err := runCLI([]string{"session", "create", "--agent", "unknown", "--prompt", "test"}, &out); err == nil {
		t.Fatal("accepted unknown agent")
	}
}

func TestCLICreateSessionChoosesModelAndPrompt(t *testing.T) {
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	source := sessionFixture(t, s, p, "claude")
	s.mu.Lock()
	env, err := s.collaboration.environmentLocked(source.ID)
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range env {
		t.Setenv(key, value)
	}
	create := func(args ...string) (Session, error) {
		var out bytes.Buffer
		var sess Session
		if err := runCLI(append([]string{"session", "create"}, args...), &out); err != nil {
			return sess, err
		}
		return sess, json.Unmarshal(out.Bytes(), &sess)
	}
	sess, err := create("--agent", "claude", "--model", "opus", "--effort", "high", "--prompt", "Review")
	if err != nil || sess.Model != "opus" || sess.Effort != "high" {
		t.Fatalf("model not saved: %+v %v", sess, err)
	}
	file := filepath.Join(t.TempDir(), "task.md")
	_ = os.WriteFile(file, []byte("From a file"), 0600)
	if sess, err = create("--agent", "codex", "--prompt-file", file); err != nil || sess.InitialPrompt != "From a file" {
		t.Fatalf("prompt file not saved: %+v %v", sess, err)
	}
	// These agents refuse an initial prompt, so the prompt has to be optional.
	if sess, err = create("--agent", "aider"); err != nil || sess.InitialPrompt != "" {
		t.Fatalf("session without a prompt: %+v %v", sess, err)
	}
	before := len(s.Snapshot().Workspaces[0].Sessions)
	for _, bad := range [][]string{
		{"--prompt", "No agent"},
		{"--agent", "claude", "--model", "haiku", "--effort", "high"},
		{"--agent", "claude", "--model", "opus; rm -rf /"},
		{"--agent", "claude", "--prompt", "a", "--prompt-file", file},
		{"--agent", "claude", "--prompt", "  "},
		{"--agent", "claude", "--text", "not a create option"},
		{"--agent", "claude", "--repository", "unknown"},
	} {
		if _, err := create(bad...); err == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
	if _, err := create("--agent", "unknown"); err == nil || !strings.Contains(err.Error(), "claude") {
		t.Fatalf("unknown agent does not list the choices: %v", err)
	}
	if after := len(s.Snapshot().Workspaces[0].Sessions); after != before {
		t.Fatalf("rejected commands created %d sessions", after-before)
	}
	var out bytes.Buffer
	if err := runCLI([]string{"session", "list", "--timeout", "5s"}, &out); err == nil {
		t.Fatal("accepted a misplaced option")
	}
	if err := runCLI([]string{"session", "list"}, &out); err != nil {
		t.Fatal(err)
	}
	var list []map[string]string
	if err := json.Unmarshal(out.Bytes(), &list); err != nil || len(list) != before {
		t.Fatalf("wrong list: %s %v", out.String(), err)
	}
	if got := list[1]; got["model"] != "opus" || got["effort"] != "high" || got["status"] != "stopped" || got["repositoryID"] != source.RepositoryID {
		t.Fatalf("list lacks the session's details: %+v", got)
	}
	out.Reset()
	var stderr bytes.Buffer
	if RunCLI([]string{"session", "create", "--help"}, &out, &stderr) != 0 || out.String() != cliHelp {
		t.Fatalf("help: %q %q", out.String(), stderr.String())
	}
}

func TestCLIStartNeedsPreference(t *testing.T) {
	fakeAgent(t, "printf '%s\\n' \"$@\"\nexec sleep 30\n")
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	source := sessionFixture(t, s, p, "claude")
	s.mu.Lock()
	env, err := s.collaboration.environmentLocked(source.ID)
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range env {
		t.Setenv(key, value)
	}
	args := []string{"session", "create", "--agent", "claude", "--model", "opus", "--prompt", "Only a fake agent receives this", "--start"}
	var out bytes.Buffer
	if err := runCLI(args, &out); err == nil || !strings.Contains(err.Error(), "Let Agents Start Sessions") {
		t.Fatalf("started with the setting off: %v", err)
	}
	if count := len(s.Snapshot().Workspaces[0].Sessions); count != 1 || len(s.terminals) != 0 {
		t.Fatalf("refused start left %d sessions", count)
	}
	prefs := s.Snapshot().Preferences
	prefs.AgentsStartSessions = true
	if _, err := s.SetPreferences(prefs); err != nil {
		t.Fatal(err)
	}
	if err := runCLI(args, &out); err != nil {
		t.Fatal(err)
	}
	var sess Session
	if err := json.Unmarshal(out.Bytes(), &sess); err != nil {
		t.Fatal(err)
	}
	if !sess.Running || !sess.HasLaunched {
		t.Fatalf("session did not start: %s", out.String())
	}
	waitUntil(t, func() bool {
		return strings.Contains(terminalText(s, sess.ID), "--model") && strings.Contains(terminalText(s, sess.ID), "opus")
	})
	if s.Snapshot().Workspaces[0].SelectedSessionID != source.ID {
		t.Fatal("start changed the selection")
	}
	// Without --start the setting changes nothing.
	out.Reset()
	if err := runCLI([]string{"session", "create", "--agent", "codex", "--prompt", "Saved only"}, &out); err != nil {
		t.Fatal(err)
	}
	var saved Session
	if err := json.Unmarshal(out.Bytes(), &saved); err != nil || saved.Running || saved.HasLaunched {
		t.Fatalf("started without --start: %s %v", out.String(), err)
	}
}

// cliCreate runs session create as the source session and returns the session it prints.
func cliCreate(t *testing.T, s *Store, source *Session, args ...string) (Session, error) {
	t.Helper()
	s.mu.Lock()
	env, err := s.collaboration.environmentLocked(source.ID)
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range env {
		t.Setenv(key, value)
	}
	var out bytes.Buffer
	var sess Session
	if err := runCLI(append([]string{"session", "create"}, args...), &out); err != nil {
		return sess, err
	}
	return sess, json.Unmarshal(out.Bytes(), &sess)
}

func TestCLILimitsAgentStartedSessionsPerWorkspace(t *testing.T) {
	fakeAgent(t, "exec sleep 30\n")
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	source := sessionFixture(t, s, p, "claude")
	// A session the user starts does not count.
	if _, err := s.StartSession(source.ID, false, 100, 30); err != nil {
		t.Fatal(err)
	}
	prefs := s.Snapshot().Preferences
	prefs.AgentsStartSessions = true
	if _, err := s.SetPreferences(prefs); err != nil {
		t.Fatal(err)
	}
	first := Session{}
	for i := range maxAgentStartedSessions {
		sess, err := cliCreate(t, s, source, "--agent", "claude", "--start")
		if err != nil || !sess.Running || !sess.AgentStarted {
			t.Fatalf("start %d: %+v %v", i, sess, err)
		}
		if i == 0 {
			first = sess
		}
	}
	before := len(s.Snapshot().Workspaces[0].Sessions)
	if _, err := cliCreate(t, s, source, "--agent", "claude", "--start"); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("started past the limit: %v", err)
	}
	if count := len(s.Snapshot().Workspaces[0].Sessions); count != before {
		t.Fatalf("refused start left %d sessions", count-before)
	}
	if sess, err := cliCreate(t, s, source, "--agent", "claude"); err != nil || sess.Running {
		t.Fatalf("limit blocked a session that does not start: %+v %v", sess, err)
	}
	// Another workspace has its own limit.
	other := workspaceFixture(t, s)
	otherSource := sessionFixture(t, s, other, "claude")
	if sess, err := cliCreate(t, s, otherSource, "--agent", "claude", "--start"); err != nil || !sess.Running {
		t.Fatalf("limit crossed workspaces: %+v %v", sess, err)
	}
	if _, err := s.CloseSession(first.ID, false); err != nil {
		t.Fatal(err)
	}
	if sess, err := cliCreate(t, s, source, "--agent", "claude", "--start"); err != nil || !sess.Running {
		t.Fatalf("closing a session did not free the limit: %+v %v", sess, err)
	}
}

func TestCLICreateSessionInWorktree(t *testing.T) {
	s := fixtureStore(t)
	p := repositoryFixture(t, s)
	directory := p.Repositories[0].Directory
	source := sessionFixture(t, s, p, "claude")
	head := gitWorktreeFixture(t, directory, "rev-parse", "HEAD")
	sess, err := cliCreate(t, s, source, "--agent", "codex", "--prompt", "Review", "--worktree", "--branch", "review/x", "--base", "main")
	if err != nil {
		t.Fatal(err)
	}
	if sess.WorktreeBranch != "review/x" || sess.WorktreeBase != head || sess.WorktreeDirectory == "" || sess.Running || sess.HasLaunched {
		t.Fatalf("wrong worktree session: %+v", sess)
	}
	if branch := gitWorktreeFixture(t, sess.WorktreeDirectory, "rev-parse", "--abbrev-ref", "HEAD"); branch != "review/x" {
		t.Fatalf("worktree is on %s", branch)
	}
	if branch := gitWorktreeFixture(t, directory, "rev-parse", "--abbrev-ref", "HEAD"); branch != "main" {
		t.Fatalf("main checkout moved to %s", branch)
	}
	if sess, err = cliCreate(t, s, source, "--agent", "codex", "--worktree"); err != nil || sess.WorktreeBranch == "" {
		t.Fatalf("default branch: %+v %v", sess, err)
	}
	before := len(s.Snapshot().Workspaces[0].Sessions)
	for _, bad := range [][]string{
		{"--agent", "codex", "--branch", "no-worktree"},
		{"--agent", "codex", "--base", "main"},
		{"--agent", "codex", "--worktree", "--branch", "review/x"},
		{"--agent", "codex", "--worktree", "--base", "missing"},
	} {
		if _, err := cliCreate(t, s, source, bad...); err == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
	if count := len(s.Snapshot().Workspaces[0].Sessions); count != before {
		t.Fatalf("rejected commands created %d sessions", count-before)
	}
}

func TestCLICreateWorktreeKeepsLocalMainUpdated(t *testing.T) {
	s := fixtureStore(t)
	p, latest := localMainFixture(t, s)
	source := sessionFixture(t, s, p, "claude")
	prefs := s.Snapshot().Preferences
	prefs.KeepLocalMainUpdated = true
	if _, err := s.SetPreferences(prefs); err != nil {
		t.Fatal(err)
	}
	sess, err := cliCreate(t, s, source, "--agent", "codex", "--worktree")
	if err != nil {
		t.Fatal(err)
	}
	if sess.WorktreeBase != latest {
		t.Fatalf("worktree starts at %s, not %s", sess.WorktreeBase, latest)
	}
}

func TestCLICreateRequestIDIsIdempotent(t *testing.T) {
	fakeAgent(t, "exec sleep 30\n")
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	source := sessionFixture(t, s, p, "claude")
	id := "3f0a8f4e-6b57-4a54-9d0c-0a1e4c2b7d11"
	args := []string{"--agent", "claude", "--prompt", "Review", "--request-id", id}
	first, err := cliCreate(t, s, source, args...)
	if err != nil {
		t.Fatal(err)
	}
	again, err := cliCreate(t, s, source, args...)
	if err != nil || again.ID != first.ID {
		t.Fatalf("retry made another session: %+v %v", again, err)
	}
	if _, err = cliCreate(t, s, source, "--agent", "claude", "--prompt", "Different", "--request-id", id); err == nil {
		t.Fatal("reused the ID for a different prompt")
	}
	if _, err = cliCreate(t, s, source, "--agent", "claude", "--request-id", "not-a-uuid"); err == nil {
		t.Fatal("accepted an invalid ID")
	}
	// A retry with --start finishes the start without launching it twice.
	prefs := s.Snapshot().Preferences
	prefs.AgentsStartSessions = true
	if _, err = s.SetPreferences(prefs); err != nil {
		t.Fatal(err)
	}
	started, err := cliCreate(t, s, source, append(args, "--start")...)
	if err != nil || started.ID != first.ID || !started.Running {
		t.Fatalf("retry did not start the session: %+v %v", started, err)
	}
	s.mu.Lock()
	terminal := s.terminals[first.ID]
	s.mu.Unlock()
	if again, err = cliCreate(t, s, source, append(args, "--start")...); err != nil || again.ID != first.ID {
		t.Fatalf("second start: %+v %v", again, err)
	}
	s.mu.Lock()
	same := s.terminals[first.ID] == terminal
	s.mu.Unlock()
	if !same {
		t.Fatal("retry launched the session again")
	}
	if count := len(s.Snapshot().Workspaces[0].Sessions); count != 2 {
		t.Fatalf("retries left %d sessions", count)
	}
}

func TestAutomationRunNowPreservesSchedule(t *testing.T) {
	fakeAgent(t, "printf 'manual fixture run\\n'\nexec sleep 30\n")
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	original := sessionFixture(t, s, p, "claude")
	for _, enabled := range []bool{true, false} {
		state, err := s.SaveAutomation(Automation{Name: "Manual review", WorkspaceID: p.ID, Agent: "claude", Prompt: "Only the fake CLI receives this", Enabled: enabled, Days: []int{1}, Time: "09:00", TimeZone: "UTC"})
		if err != nil {
			t.Fatal(err)
		}
		before := state.Automations[len(state.Automations)-1]
		state, err = s.RunAutomation(before.ID)
		if err != nil {
			t.Fatal(err)
		}
		got := state.Automations[len(state.Automations)-1]
		if got.Enabled != enabled || !got.NextRun.Equal(before.NextRun) {
			t.Fatalf("Run Now changed schedule: %+v", got)
		}
		if got.LastSessionID == "" || got.LastRun.IsZero() || got.LastError != "" {
			t.Fatalf("missing run metadata: %+v", got)
		}
		if state.Workspaces[0].SelectedSessionID != original.ID {
			t.Fatal("manual run changed selection")
		}
		sess := sessionState(s, got.LastSessionID)
		if !sess.Running || sess.InitialPrompt != before.Prompt || sess.Agent != before.Agent {
			t.Fatalf("wrong run: %+v", sess)
		}
	}
	if _, err := s.RunAutomation("missing"); err == nil {
		t.Fatal("accepted unknown automation")
	}
}

func TestAutomationEditCompletedOneTimeSchedule(t *testing.T) {
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	future := time.Now().UTC().AddDate(0, 0, 2).Format("2006-01-02")
	state, err := s.SaveAutomation(Automation{Name: "One-time review", WorkspaceID: p.ID, Agent: "claude", Prompt: "Review", Enabled: true, Date: future, Time: "09:00", TimeZone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	completed := state.Automations[0]
	completed.Date = time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	completed.Enabled, completed.NextRun = false, time.Time{}
	completed.LastRun, completed.LastSessionID = time.Now().UTC().Add(-time.Hour), "previous-session"
	s.mu.Lock()
	s.state.Automations[0] = completed
	s.mu.Unlock()
	completed.Name, completed.Prompt = "Renamed review", "Updated instructions"
	state, err = s.SaveAutomation(completed)
	if err != nil {
		t.Fatal(err)
	}
	got := state.Automations[0]
	if got.Enabled || !got.NextRun.IsZero() || got.Name != completed.Name || got.Prompt != completed.Prompt {
		t.Fatalf("incorrect completed edit: %+v", got)
	}
	if !got.LastRun.Equal(completed.LastRun) || got.LastSessionID != completed.LastSessionID {
		t.Fatal("edit lost run history")
	}
	reopened, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	if got := reopened.Snapshot().Automations[0]; got.Name != completed.Name || !got.NextRun.IsZero() {
		t.Fatalf("edit not persisted: %+v", got)
	}
	completed.Enabled = true
	if _, err := s.SaveAutomation(completed); err == nil {
		t.Fatal("enabled a past schedule")
	}
	completed.Enabled, completed.Date = false, "invalid"
	if _, err := s.SaveAutomation(completed); err == nil {
		t.Fatal("accepted invalid disabled schedule")
	}
	completed.Enabled, completed.Date = true, future
	state, err = s.SaveAutomation(completed)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Automations[0].NextRun.After(time.Now()) || !state.Automations[0].Enabled {
		t.Fatal("could not reschedule completed automation")
	}
}

func TestAutomationHistorySurvivesEditsReopenAndClear(t *testing.T) {
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	state, err := s.SaveAutomation(Automation{Name: "Review", WorkspaceID: p.ID, Agent: "codex", Prompt: "Original prompt", Enabled: true, Days: []int{1}, Time: "09:00", TimeZone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	id := state.Automations[0].ID
	// Fail without contacting an agent, through both scheduler and manual paths.
	s.state.Automations[0].WorkspaceID = "missing"
	s.runDueAutomations(state.Automations[0].NextRun)
	if _, err = s.RunAutomation(id); err == nil {
		t.Fatal("expected launch failure")
	}
	a := s.Snapshot().Automations[0]
	if len(a.Runs) != 2 || a.Runs[0].Trigger != "scheduled" || a.Runs[1].Trigger != "manual" || a.Runs[0].ID == a.Runs[1].ID {
		t.Fatalf("wrong history: %+v", a.Runs)
	}
	for _, run := range a.Runs {
		if run.Status != "failed" || run.Error == "" || run.Prompt != "Original prompt" {
			t.Fatalf("wrong run: %+v", run)
		}
	}
	a.WorkspaceID, a.Name, a.Prompt = p.ID, "Renamed", "New prompt"
	// Clients cannot replace authoritative history while saving settings.
	a.Runs = nil
	if _, err = s.SaveAutomation(a); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	a = reopened.Snapshot().Automations[0]
	if len(a.Runs) != 2 || a.Runs[0].Name != "Review" || a.Runs[0].Prompt != "Original prompt" {
		t.Fatalf("history changed: %+v", a)
	}
	path := reopened.path
	reopened.path = filepath.Join(path, "child")
	if _, err = reopened.ClearAutomationHistory(id); err == nil || len(reopened.Snapshot().Automations[0].Runs) != 2 {
		t.Fatal("failed clear must roll back")
	}
	reopened.path = path
	if _, err = reopened.ClearAutomationHistory(id); err != nil {
		t.Fatal(err)
	}
	again, err := NewStore(path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Shutdown()
	if len(again.Snapshot().Automations[0].Runs) != 0 {
		t.Fatal("cleared history resurrected")
	}
}

func TestAutomationHistoryLegacyAndInterruptedLaunch(t *testing.T) {
	s := fixtureStore(t)
	at := time.Now().UTC().Truncate(time.Second)
	s.state.Automations = []Automation{{ID: "legacy", LastRun: at, LastSessionID: "deleted", Prompt: "Current prompt is not historical"}, {ID: "interrupted", HistoryInitialized: true, Runs: []AutomationRun{{ID: "run", Status: "starting"}}}}
	if err := s.saveLocked(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	a := reopened.Snapshot().Automations
	if len(a[0].Runs) != 1 || a[0].Runs[0].Trigger != "legacy" || a[0].Runs[0].Prompt != "" || a[0].Runs[0].SessionID != "deleted" || !a[0].Runs[0].StartedAt.Equal(at) {
		t.Fatalf("bad migration: %+v", a[0])
	}
	if a[1].Runs[0].Status != "unknown" {
		t.Fatal("interrupted launch must not claim success")
	}
}
