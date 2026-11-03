package core

import (
	"bytes"
	"encoding/json"
	"path/filepath"
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
