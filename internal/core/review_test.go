package core

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewCommentsPersistAndValidate(t *testing.T) {
	s := fixtureStore(t)
	p := projectFixture(t, s)
	sess := sessionFixture(t, s, p, "claude")
	state, err := s.SaveReviewComment(sess.ID, ReviewComment{Path: "src/a.go", Side: "new", Start: 3, End: 4, Excerpt: "a\nb", Text: "  Why?  "})
	if err != nil {
		t.Fatal(err)
	}
	comments := state.Projects[0].Sessions[0].ReviewComments
	if len(comments) != 1 || comments[0].Text != "Why?" || comments[0].ID == "" {
		t.Fatalf("comment not saved: %+v", comments)
	}
	edited := *comments[0]
	edited.Text = "Rename this."
	if state, err = s.SaveReviewComment(sess.ID, edited); err != nil || len(state.Projects[0].Sessions[0].ReviewComments) != 1 {
		t.Fatalf("edit added a comment: %v", err)
	}
	for _, bad := range []ReviewComment{
		{Path: "/abs", Side: "new", Start: 1, End: 1, Text: "x"},
		{Path: "a", Side: "both", Start: 1, End: 1, Text: "x"},
		{Path: "a", Side: "new", Start: 3, End: 2, Text: "x"},
		{Path: "a", Side: "new", Start: 1, End: 1, Text: " "},
	} {
		if _, err = s.SaveReviewComment(sess.ID, bad); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	reopened, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.Snapshot().Projects[0].Sessions[0].ReviewComments
	reopened.Shutdown()
	if len(got) != 1 || got[0].Text != "Rename this." || got[0].Excerpt != "a\nb" {
		t.Fatalf("comment not restored: %+v", got)
	}
	if state, err = s.DeleteReviewComment(sess.ID, edited.ID); err != nil || len(state.Projects[0].Sessions[0].ReviewComments) != 0 {
		t.Fatalf("comment not deleted: %v", err)
	}
	terminal := sessionFixture(t, s, p, terminalAgent)
	if _, err = s.SaveReviewComment(terminal.ID, ReviewComment{Path: "a", Side: "new", Start: 1, End: 1, Text: "x"}); err == nil {
		t.Fatal("terminal accepted a review comment")
	}
}

func TestSendReviewQueuesWhenTheAgentIsNotRunning(t *testing.T) {
	s := fixtureStore(t)
	p := projectFixture(t, s)
	sess := sessionFixture(t, s, p, "claude")
	for _, text := range []string{"first", "second"} {
		if _, err := s.SaveReviewComment(sess.ID, ReviewComment{Path: "a.go", Side: "old", Start: 2, End: 2, Excerpt: "gone", Text: text}); err != nil {
			t.Fatal(err)
		}
	}
	first := s.Snapshot().Projects[0].Sessions[0].ReviewComments[0].ID
	state, err := s.SendReview(sess.ID, []string{first})
	if err != nil {
		t.Fatal(err)
	}
	if left := state.Projects[0].Sessions[0].ReviewComments; len(left) != 1 || left[0].Text != "second" {
		t.Fatalf("sent comment still pending: %+v", left)
	}
	run := state.Runs[len(state.Runs)-1]
	if run.Status != "queued" || run.Kind != reviewKind || run.FromSessionID != "" || run.ToSessionID != sess.ID || len(run.Comments) != 1 {
		t.Fatalf("unexpected review: %+v", run)
	}
	if !strings.Contains(run.Prompt, "1. a.go:2 (removed lines") || !strings.Contains(run.Prompt, "   > gone") || !strings.Contains(run.Prompt, "   first") {
		t.Fatalf("unexpected prompt: %q", run.Prompt)
	}
	if _, err = s.SendReview(sess.ID, []string{"missing"}); err == nil {
		t.Fatal("sent a review without comments")
	}
	if state, err = s.DismissReview(run.ID); err != nil || !state.Runs[len(state.Runs)-1].Dismissed {
		t.Fatalf("review not dismissed: %v", err)
	}
}

func TestReviewSubmitsAndCompletesWithEveryReply(t *testing.T) {
	s := fixtureStore(t)
	p := projectFixture(t, s)
	agent := sessionFixture(t, s, p, "claude")
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
	waitUntil(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		_, sess, _ := s.sessionLocked(agent.ID)
		return sess.Activity == "waiting"
	})
	for _, text := range []string{"one", "two"} {
		if _, err := s.SaveReviewComment(agent.ID, ReviewComment{Path: "a.go", Side: "new", Start: 1, End: 1, Text: text}); err != nil {
			t.Fatal(err)
		}
	}
	state, err := s.SendReview(agent.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := state.Runs[len(state.Runs)-1].ID
	if state.Runs[len(state.Runs)-1].Status != "running" {
		t.Fatal("review not submitted to a free agent")
	}
	waitUntil(t, func() bool {
		snapshot, _ := s.TerminalSnapshot(agent.ID)
		raw, _ := base64.StdEncoding.DecodeString(snapshot.Data)
		return strings.Contains(string(raw), "Orchestra review "+id)
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.collaboration
	if _, err = c.replyCommentLocked(id, agent.ID, 3, "x"); err == nil {
		t.Fatal("accepted a comment out of range")
	}
	if _, err = c.replyCommentLocked(id, "other", 1, "x"); err == nil {
		t.Fatal("accepted a reply from another session")
	}
	if _, err = c.replyCommentLocked(id, agent.ID, 2, "Done two."); err != nil {
		t.Fatal(err)
	}
	if run, _ := c.runLocked(id); run.Status != "running" || run.Comments[1].Reply != "Done two." {
		t.Fatalf("partial reply: %+v", run)
	}
	if _, err = c.replyCommentLocked(id, agent.ID, 1, "Done one."); err != nil {
		t.Fatal(err)
	}
	if run, _ := c.runLocked(id); run.Status != "completed" {
		t.Fatalf("review not completed: %+v", run)
	}
	if _, err = c.replyCommentLocked(id, agent.ID, 1, "Done one."); err != nil {
		t.Fatal("repeated reply failed")
	}
}
