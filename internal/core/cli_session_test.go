package core

import (
	"bytes"
	"strings"
	"testing"
)

func TestCLIRenameAndCloseSession(t *testing.T) {
	fakeAgent(t, "exec sleep 30\n")
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	source := sessionFixture(t, s, p, "claude")
	target := sessionFixture(t, s, p, "claude")
	other := sessionFixture(t, s, workspaceFixture(t, s), "claude")
	s.mu.Lock()
	env, err := s.collaboration.environmentLocked(source.ID)
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	run := func(args ...string) error {
		t.Helper()
		var out bytes.Buffer
		return runCLI(append([]string{"session"}, args...), &out)
	}
	for _, args := range [][]string{
		{"rename", target.ID}, {"rename", target.ID, "--name", " "},
		{"rename"}, {"close"}, {"close", target.ID, "--name", "x"},
		{"rename", other.ID, "--name", "x"}, {"close", other.ID},
		{"rename", "missing", "--name", "x"}, {"close", "missing"},
	} {
		if err := run(args...); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	if err := run("rename", target.ID, "--name", " Review "); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	_, renamed, err := s.sessionLocked(target.ID)
	if err != nil || renamed.Name != "Review" {
		t.Fatalf("rename: %+v %v", renamed, err)
	}
	s.mu.Unlock()
	if _, err := s.StartSession(target.ID, false, 100, 30); err != nil {
		t.Fatal(err)
	}
	if err := run("close", target.ID); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	_, _, err = s.sessionLocked(target.ID)
	terminal := s.terminals[target.ID]
	s.mu.Unlock()
	if err == nil || terminal != nil {
		t.Fatal("closed session or terminal retained")
	}
	for _, action := range []string{"rename", "close"} {
		args := []string{action, other.ID, "--workspace", "Fixture"}
		if action == "rename" {
			args = append(args, "--name", "x")
		}
		if err := run(args...); err == nil || !strings.Contains(err.Error(), "ambiguous") {
			t.Fatalf("ambiguous workspace: %v", err)
		}
	}
	otherWorkspace := workspaceFixture(t, s)
	remote := sessionFixture(t, s, otherWorkspace, "claude")
	if _, err := s.Rename(otherWorkspace.ID, "Remote"); err != nil {
		t.Fatal(err)
	}
	for _, selector := range []string{otherWorkspace.ID, "Remote"} {
		if err := run("rename", remote.ID, "--name", "Remote review", "--workspace", selector); err != nil {
			t.Fatal(err)
		}
	}
	if err := run("close", remote.ID, "--workspace", "Remote"); err != nil {
		t.Fatal(err)
	}
	if err := run("rename", source.ID, "--name", "x", "--workspace", "missing"); err == nil {
		t.Fatal("unknown workspace accepted")
	}
	if err := run("close", source.ID, "--workspace", otherWorkspace.ID); err == nil {
		t.Fatal("target outside selected workspace accepted")
	}
	// Authentication remains required for both mutations.
	t.Setenv("ORCHESTRA_TOKEN", "invalid")
	for _, args := range [][]string{{"rename", source.ID, "--name", "x"}, {"close", source.ID}} {
		if err := run(args...); err == nil || !strings.Contains(err.Error(), "credentials") {
			t.Fatalf("authentication: %v", err)
		}
	}
}
