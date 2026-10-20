package core

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ReviewComment is the user's note on lines of a session's diff. Side is "new" for
// added and context lines or "old" for removed lines; Start and End are line numbers
// on that side and Excerpt holds their text, so the Changes pane can tell when the
// lines changed afterwards. Reply is the agent's answer once the comment was sent.
type ReviewComment struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Side    string `json:"side"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
	Excerpt string `json:"excerpt,omitempty"`
	Text    string `json:"text"`
	Reply   string `json:"reply,omitempty"`
}

// Limits keep pending comments small, since they are saved with the workspace.
const (
	maxReviewComments     = 100
	maxReviewCommentBytes = 16 * 1024
	maxReviewExcerptBytes = 4 * 1024
)

func validReviewComment(c *ReviewComment) error {
	if c == nil {
		return errors.New("Invalid review comment.")
	}
	if _, err := uuid.Parse(c.ID); err != nil {
		return errors.New("Invalid review comment ID.")
	}
	if c.Path == "" || filepath.IsAbs(c.Path) || strings.ContainsAny(c.Path, "\x00\n") {
		return errors.New("Invalid review comment file.")
	}
	if (c.Side != "new" && c.Side != "old") || c.Start <= 0 || c.End < c.Start {
		return errors.New("Invalid review comment lines.")
	}
	if strings.TrimSpace(c.Text) == "" || len(c.Text) > maxReviewCommentBytes {
		return errors.New("Write a comment smaller than 16 KB.")
	}
	if len(c.Excerpt) > maxReviewExcerptBytes {
		return errors.New("Invalid review comment excerpt.")
	}
	return nil
}

// SaveReviewComment adds a pending comment to a session's review, or replaces the one
// with the same ID. It never writes to the session's terminal.
func (s *Store) SaveReviewComment(sessionID string, comment ReviewComment) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, sess, err := s.sessionLocked(sessionID)
	if err != nil {
		return State{}, err
	}
	if sess.Agent == terminalAgent {
		return State{}, errors.New("Terminals cannot receive review comments.")
	}
	if comment.ID == "" {
		comment.ID = uuid.NewString()
	}
	comment.Text = strings.TrimSpace(comment.Text)
	comment.Reply = ""
	comment.Excerpt = truncateUTF8(comment.Excerpt, maxReviewExcerptBytes)
	if err = validReviewComment(&comment); err != nil {
		return State{}, err
	}
	if i := slices.IndexFunc(sess.ReviewComments, func(c *ReviewComment) bool { return c.ID == comment.ID }); i >= 0 {
		sess.ReviewComments[i] = &comment
	} else if len(sess.ReviewComments) >= maxReviewComments {
		return State{}, errors.New("Send or delete some pending comments before adding more.")
	} else {
		sess.ReviewComments = append(sess.ReviewComments, &comment)
	}
	err = s.saveLocked()
	s.changedLocked()
	return s.snapshotLocked(), err
}

// DeleteReviewComment removes a pending comment.
func (s *Store) DeleteReviewComment(sessionID, commentID string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, sess, err := s.sessionLocked(sessionID)
	if err != nil {
		return State{}, err
	}
	sess.ReviewComments = slices.DeleteFunc(sess.ReviewComments, func(c *ReviewComment) bool { return c.ID == commentID })
	err = s.saveLocked()
	s.changedLocked()
	return s.snapshotLocked(), err
}

// SendReview turns pending comments into one Inbox request to the same session, or all
// of them when commentIDs is empty. Choosing Send is the user's explicit action, so the
// request is submitted right away when the agent is free, like Execute Request.
// Otherwise it stays queued and follows the Inbox delivery preference.
func (s *Store) SendReview(sessionID string, commentIDs []string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, sess, err := s.sessionLocked(sessionID)
	if err != nil {
		return State{}, err
	}
	var comments []ReviewComment
	for _, c := range sess.ReviewComments {
		if len(commentIDs) == 0 || slices.Contains(commentIDs, c.ID) {
			comments = append(comments, *c)
		}
	}
	if len(comments) == 0 {
		return State{}, errors.New("There are no pending comments to send.")
	}
	run, err := s.collaboration.reviewLocked(sessionID, comments)
	if err != nil {
		return State{}, err
	}
	sess.ReviewComments = slices.DeleteFunc(sess.ReviewComments, func(c *ReviewComment) bool {
		return slices.ContainsFunc(comments, func(sent ReviewComment) bool { return sent.ID == c.ID })
	})
	s.trackLocked("feature_used", map[string]any{"feature": "diff_review"})
	t := s.terminals[sess.ID]
	if run.Status == "queued" && t != nil && !t.stopped && sess.Running && sess.Activity != "working" && s.collaboration.awaitingLocked(sess.ID) == nil {
		if err = s.submitRequestLocked(run, sess, t); err != nil {
			s.log.Warn("review submission failed", "request", run.ID, "session", sess.ID, "error", err)
		}
	}
	saveErr := s.saveLocked()
	s.changedLocked()
	return s.snapshotLocked(), errors.Join(err, saveErr)
}

// DismissReview hides a sent review from the Changes pane. It stays in Messages.
func (s *Store) DismissReview(id string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, err := s.collaboration.runLocked(id)
	if err != nil {
		return State{}, err
	}
	if run.Kind != reviewKind {
		return State{}, errors.New("This request is not a review.")
	}
	copy := *run
	copy.Dismissed = true
	if err = s.collaboration.saveLocked(&copy); err != nil {
		return State{}, err
	}
	s.changedLocked()
	return s.snapshotLocked(), nil
}

const reviewKind = "review"

// reviewLocked queues the user's comments for a session. The request has no sender
// session; replies arrive per comment through orchestra run reply --comment.
func (c *Collaboration) reviewLocked(to string, comments []ReviewComment) (*Run, error) {
	p, target, err := c.store.sessionLocked(to)
	if err != nil {
		return nil, err
	}
	if target.Agent == terminalAgent {
		return nil, errors.New("Terminals cannot receive requests.")
	}
	prompt := reviewPrompt(comments)
	if len(prompt) > 256*1024 {
		return nil, errors.New("These comments are too large to send together. Send fewer at a time.")
	}
	run := &Run{ID: uuid.NewString(), ProjectID: p.ID, ToSessionID: to, Kind: reviewKind, Comments: comments, Prompt: prompt, Status: "queued", CreatedAt: float64(time.Now().UnixMilli())/1000 - 978307200}
	if err = c.saveLocked(run); err != nil {
		return nil, err
	}
	c.store.deliverLocked(to)
	return c.runLocked(run.ID)
}

// awaitingLocked returns the request a session still has to reply to, if any.
func (c *Collaboration) awaitingLocked(id string) *Run {
	for _, r := range c.runs {
		if r.ToSessionID == id && r.Status == "running" {
			return r
		}
	}
	return nil
}

// reviewPrompt lists the comments with their file, lines and excerpt, numbered from 1
// as orchestra run reply --comment expects.
func reviewPrompt(comments []ReviewComment) string {
	var b strings.Builder
	for i, c := range comments {
		if i > 0 {
			b.WriteString("\n\n")
		}
		lines := fmt.Sprint(c.Start)
		if c.End > c.Start {
			lines = fmt.Sprintf("%d-%d", c.Start, c.End)
		}
		kind := ""
		if c.Side == "old" {
			kind = " (removed lines, numbered as before the change)"
		}
		fmt.Fprintf(&b, "%d. %s:%s%s\n", i+1, c.Path, lines, kind)
		if c.Excerpt != "" {
			for _, line := range strings.Split(c.Excerpt, "\n") {
				b.WriteString("   > " + line + "\n")
			}
		}
		for _, line := range strings.Split(c.Text, "\n") {
			b.WriteString("   " + line + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// replyCommentLocked records the agent's answer to one review comment. The review
// completes when every comment has an answer.
func (c *Collaboration) replyCommentLocked(id, source string, comment int, text string) (*Run, error) {
	run, err := c.runLocked(id)
	if err != nil {
		return nil, err
	}
	if run.ToSessionID != source {
		return nil, errors.New("Only the recipient can reply to this request.")
	}
	if run.Kind != reviewKind {
		return nil, errors.New("Only review requests take --comment.")
	}
	if comment < 1 || comment > len(run.Comments) {
		return nil, fmt.Errorf("Choose a comment from 1 to %d.", len(run.Comments))
	}
	text = strings.TrimSpace(text)
	if text == "" || len(text) > 128*1024 {
		return nil, errors.New("Send a non-empty reply smaller than 128 KB.")
	}
	if run.Comments[comment-1].Reply == text {
		return run, nil
	}
	if run.Status != "running" {
		return nil, errors.New("This request is not awaiting a reply.")
	}
	copy := *run
	copy.Comments = slices.Clone(run.Comments)
	copy.Comments[comment-1].Reply = text
	if !slices.ContainsFunc(copy.Comments, func(c ReviewComment) bool { return c.Reply == "" }) {
		copy.Status = "completed"
	}
	if err = c.saveLocked(&copy); err != nil {
		return nil, err
	}
	c.store.changedLocked()
	if copy.Status == "completed" {
		c.store.deliverLocked(source)
	}
	return run, nil
}

// truncateUTF8 shortens s to at most n bytes without splitting a character.
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && n < len(s) && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
}
