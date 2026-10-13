package core

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Run struct {
	ID            string  `json:"id"`
	ProjectID     string  `json:"projectID"`
	FromSessionID string  `json:"fromSessionID,omitempty"`
	ToSessionID   string  `json:"toSessionID"`
	Prompt        string  `json:"prompt"`
	Status        string  `json:"status"`
	Response      string  `json:"response"`
	Detail        string  `json:"detail,omitempty"`
	CreatedAt     float64 `json:"createdAt"` // Seconds since 2001-01-01 UTC.
}

func (r *Run) finished() bool { return r.Status != "queued" && r.Status != "running" }

type WireRequest struct {
	Method    string `json:"method"`
	Source    string `json:"source"`
	Token     string `json:"token"`
	Target    string `json:"target,omitempty"`
	Text      string `json:"text,omitempty"`
	RequestID string `json:"requestID,omitempty"`
	Before    string `json:"before,omitempty"`
	// Revision is the Scratchpad revision a scratchpad.write expects; nil writes unconditionally.
	Revision *int `json:"revision,omitempty"`
}
type WaitEdge struct {
	Target  string
	Expires time.Time
}
type Collaboration struct {
	store    *Store
	path     string
	runs     []*Run
	tokens   map[string]string
	waits    map[string]WaitEdge
	listener net.Listener
	stopping bool
}

// Finished runs beyond these limits are pruned from the history. Queued and running
// runs are always kept.
const (
	historyLimit = 500
	historyAge   = 30 * 24 * time.Hour
)

type history struct {
	Version int    `json:"version"`
	Runs    []*Run `json:"runs"`
}

// loadRuns reads the history file. A missing file starts empty; an unreadable one is
// moved aside to path+".corrupt" and reported through warning instead of failing startup.
func loadRuns(path string) (runs []*Run, warning string, err error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []*Run{}, "", nil
	} else if err != nil {
		return nil, "", err
	}
	var h history
	if json.Unmarshal(data, &h) == nil && h.Version == 1 && !slices.Contains(h.Runs, nil) {
		return pruneRuns(h.Runs, time.Now()), "", nil
	}
	if err = os.Rename(path, path+".corrupt"); err != nil {
		return nil, "", err
	}
	return []*Run{}, "Could not read the Inbox history. It was moved to " + path + ".corrupt and a new history was started.", nil
}
func writeRuns(path string, runs []*Run) error {
	data, err := json.Marshal(history{Version: 1, Runs: runs})
	if err != nil {
		return err
	}
	return atomicWrite(path, data, 0600)
}
func pruneRuns(runs []*Run, now time.Time) []*Run {
	cutoff := float64(now.Add(-historyAge).UnixMilli())/1000 - 978307200
	finished := 0
	for _, r := range runs {
		if r.finished() {
			finished++
		}
	}
	kept := []*Run{}
	for _, r := range runs {
		if r.finished() && (finished > historyLimit || r.CreatedAt < cutoff) {
			finished--
			continue
		}
		kept = append(kept, r)
	}
	return kept
}
func newCollaboration(path string, s *Store) (*Collaboration, string, error) {
	runs, warning, err := loadRuns(path)
	if err != nil {
		return nil, "", err
	}
	c := &Collaboration{store: s, path: path, runs: runs, tokens: map[string]string{}, waits: map[string]WaitEdge{}}
	interrupted := false
	for _, run := range runs {
		if run.Status == "running" {
			run.Status = "interrupted"
			run.Detail = "Orchestra stopped before this request finished. It was not resent."
			interrupted = true
		}
	}
	if interrupted {
		if err = writeRuns(path, runs); err != nil {
			return nil, "", err
		}
	}
	shim := "#!/bin/sh\nexec " + shellQuote(s.executable) + " --orchestra-cli \"$@\"\n"
	if err = atomicWrite(filepath.Join(s.runtimeDir, "orchestra"), []byte(shim), 0700); err != nil {
		return nil, "", err
	}
	listener, err := net.Listen("unix", filepath.Join(s.runtimeDir, "broker.sock"))
	if err != nil {
		return nil, "", err
	}
	c.listener = listener
	if err = os.Chmod(filepath.Join(s.runtimeDir, "broker.sock"), 0600); err != nil {
		listener.Close()
		return nil, "", err
	}
	go c.serve()
	return c, warning, nil
}
func (c *Collaboration) serve() {
	slots := make(chan struct{}, 8)
	for {
		conn, err := c.listener.Accept()
		if err != nil {
			return
		}
		select {
		case slots <- struct{}{}:
			go func() {
				defer func() { <-slots; conn.Close() }()
				_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
				reader := bufio.NewReaderSize(conn, 8192)
				var line []byte
				for len(line) <= 2*1024*1024 {
					part, prefix, err := reader.ReadLine()
					if err != nil {
						return
					}
					line = append(line, part...)
					if !prefix {
						break
					}
				}
				var result any
				var req WireRequest
				if len(line) > 2*1024*1024 {
					c.store.log.Warn("broker request too large")
					result = map[string]string{"error": "Collaboration message exceeded the size limit."}
				} else if err = json.Unmarshal(line, &req); err != nil {
					c.store.log.Warn("broker request invalid", "error", err)
					result = map[string]string{"error": "Invalid request."}
				} else {
					c.store.mu.Lock()
					result, err = c.handleLocked(req)
					if err == nil {
						// Results can reference mutable runs. Serialize before releasing ownership.
						var encoded []byte
						encoded, err = json.Marshal(result)
						result = json.RawMessage(encoded)
					}
					c.store.mu.Unlock()
					logRequest(c.store.log, req, err)
					if err != nil {
						result = map[string]string{"error": err.Error()}
					}
				}
				if err := json.NewEncoder(conn).Encode(result); err != nil {
					c.store.log.Debug("broker reply failed", "error", err)
				}
			}()
		default:
			_ = conn.Close()
		}
	}
}

// logRequest records a broker call by method and calling session, never its text or token.
// The CLI polls run.wait, so those calls only appear at debug level.
func logRequest(log *slog.Logger, req WireRequest, err error) {
	level := slog.LevelInfo
	if req.Method == "run.wait" || req.Method == "run.wait.clear" {
		level = slog.LevelDebug
	}
	attrs := []any{"method", req.Method, "session", req.Source}
	if req.Target != "" {
		attrs = append(attrs, "target", req.Target)
	}
	if err != nil {
		attrs = append(attrs, "error", err)
	}
	log.Log(context.Background(), level, "broker request", attrs...)
}
func (c *Collaboration) environmentLocked(id string) (map[string]string, error) {
	token := c.tokens[id]
	if token == "" {
		token = uuid.NewString() + uuid.NewString()
		c.tokens[id] = token
	}
	env := map[string]string{"ORCHESTRA_SOCKET": filepath.Join(c.store.runtimeDir, "broker.sock"), "ORCHESTRA_SESSION_ID": id, "ORCHESTRA_TOKEN": token, "ORCHESTRA_APP_EXECUTABLE": c.store.executable, "ORCHESTRA_CLI_DIRECTORY": c.store.runtimeDir}
	context := filepath.Join(c.store.runtimeDir, id+".json")
	data, _ := json.Marshal(env)
	if err := atomicWrite(context, data, 0600); err != nil {
		return nil, err
	}
	env["ORCHESTRA_CONTEXT"] = context
	return env, nil
}
func (c *Collaboration) historyLocked() []*Run {
	data, _ := json.Marshal(c.runs)
	var copy []*Run
	_ = json.Unmarshal(data, &copy)
	return copy
}
func (c *Collaboration) runLocked(id string) (*Run, error) {
	for _, r := range c.runs {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, errors.New("Request unavailable.")
}

// saveLocked rewrites the history with run added or replaced, then updates the stored
// run in place so callers holding it see the change. Runs only change state a few
// times, so each change is written immediately.
func (c *Collaboration) saveLocked(run *Run) error {
	next := slices.Clone(c.runs)
	i := slices.IndexFunc(next, func(r *Run) bool { return r.ID == run.ID })
	if i < 0 {
		next = append(next, run)
	} else {
		next[i] = run
	}
	next = pruneRuns(next, time.Now())
	if err := writeRuns(c.path, next); err != nil {
		return err
	}
	if j := slices.Index(next, run); j >= 0 {
		if i >= 0 {
			*c.runs[i] = *run
			next[j] = c.runs[i]
		} else {
			copy := *run
			next[j] = &copy
		}
	}
	c.runs = next
	return nil
}
func (c *Collaboration) requestLocked(from, to, text, id string) (*Run, error) {
	p, _, err := c.store.sessionLocked(from)
	if err != nil {
		return nil, err
	}
	q, target, err := c.store.sessionLocked(to)
	if err != nil || p != q {
		return nil, errors.New("Sessions must belong to the same project.")
	}
	if target.Agent == terminalAgent {
		return nil, errors.New("Terminals cannot receive requests.")
	}
	if from == to {
		return nil, errors.New("A session cannot request work from itself.")
	}
	text = strings.TrimSpace(text)
	encoded, _ := json.Marshal(text)
	if text == "" || len(encoded) > 256*1024 {
		return nil, errors.New("Send a non-empty message smaller than 256 KB when encoded as JSON.")
	}
	if id == "" {
		id = uuid.NewString()
	}
	if _, err = uuid.Parse(id); err != nil {
		return nil, errors.New("Invalid request ID.")
	}
	if previous, err := c.runLocked(id); err == nil {
		if previous.FromSessionID != from || previous.ToSessionID != to || previous.Prompt != text {
			return nil, errors.New("This request ID already belongs to a different request.")
		}
		return previous, nil
	}
	run := &Run{ID: id, ProjectID: p.ID, FromSessionID: from, ToSessionID: to, Prompt: text, Status: "queued", CreatedAt: float64(time.Now().UnixMilli())/1000 - 978307200}
	if err = c.saveLocked(run); err != nil {
		return nil, err
	}
	c.store.changedLocked()
	c.store.deliverLocked(to)
	return c.runLocked(run.ID)
}
func (c *Collaboration) claimLocked(id, recipient string) (*Run, error) {
	run, err := c.runLocked(id)
	if err != nil {
		return nil, err
	}
	if run.ToSessionID != recipient || run.Status != "queued" {
		return nil, errors.New("This request is no longer unread.")
	}
	for _, r := range c.runs {
		if r.ToSessionID == recipient && r.Status == "running" {
			return nil, errors.New("This session is awaiting another reply.")
		}
	}
	copy := *run
	copy.Status = "running"
	if err = c.saveLocked(&copy); err != nil {
		return nil, err
	}
	c.store.changedLocked()
	return run, nil
}
func (c *Collaboration) inboxLocked(id string) (*Run, error) {
	for _, r := range c.runs {
		if r.ToSessionID == id && r.Status == "running" {
			return r, nil
		}
	}
	for _, r := range c.runs {
		if r.ToSessionID == id && r.Status == "queued" {
			return c.claimLocked(r.ID, id)
		}
	}
	return nil, nil
}
func (c *Collaboration) replyLocked(id, source, text string) (*Run, error) {
	run, err := c.runLocked(id)
	if err != nil {
		return nil, err
	}
	if run.ToSessionID != source {
		return nil, errors.New("Only the recipient can reply to this request.")
	}
	text = strings.TrimSpace(text)
	if text == "" || len(text) > 128*1024 {
		return nil, errors.New("Send a non-empty reply smaller than 128 KB.")
	}
	if run.Status == "completed" && run.Response == text {
		return run, nil
	}
	if run.Status != "running" {
		return nil, errors.New("This request is not awaiting a reply.")
	}
	copy := *run
	copy.Status = "completed"
	copy.Response = text
	if err = c.saveLocked(&copy); err != nil {
		return nil, err
	}
	c.store.changedLocked()
	c.store.deliverLocked(source)
	return run, nil
}
func (c *Collaboration) cancelLocked(id string) error {
	run, err := c.runLocked(id)
	if err != nil {
		return err
	}
	if run.finished() {
		return nil
	}
	copy := *run
	copy.Status = "cancelled"
	copy.Detail = "Request cancelled. The participating sessions keep running."
	if err = c.saveLocked(&copy); err != nil {
		return err
	}
	c.store.changedLocked()
	c.store.deliverLocked(run.ToSessionID)
	return nil
}
func (c *Collaboration) closeLocked(id string) error {
	for _, run := range c.runs {
		if !run.finished() && (run.FromSessionID == id || run.ToSessionID == id) {
			copy := *run
			copy.Status = "interrupted"
			copy.Detail = "A participating session was closed."
			if err := c.saveLocked(&copy); err != nil {
				return err
			}
		}
	}
	delete(c.tokens, id)
	delete(c.waits, id)
	_ = os.Remove(filepath.Join(c.store.runtimeDir, id+".json"))
	return nil
}
func (c *Collaboration) stopLocked() {
	c.stopping = true
	_ = c.listener.Close()
	for _, r := range c.runs {
		if r.Status == "running" {
			copy := *r
			copy.Status = "interrupted"
			copy.Detail = "Orchestra closed before this request finished. It was not resent."
			if err := c.saveLocked(&copy); err != nil {
				c.store.log.Warn("inbox history save failed", "request", r.ID, "error", err)
			}
		}
	}
}
func (c *Collaboration) waitLocked(source string, run *Run) error {
	for id, edge := range c.waits {
		if time.Now().After(edge.Expires) {
			delete(c.waits, id)
		}
	}
	if run.finished() {
		delete(c.waits, source)
		return nil
	}
	target := run.ToSessionID
	visited := map[string]bool{}
	for !visited[target] {
		visited[target] = true
		if target == source {
			return errors.New("This wait would create a cycle between sessions.")
		}
		edge, ok := c.waits[target]
		if !ok {
			break
		}
		target = edge.Target
	}
	c.waits[source] = WaitEdge{run.ToSessionID, time.Now().Add(15 * time.Second)}
	return nil
}
func (c *Collaboration) handleLocked(req WireRequest) (any, error) {
	token := c.tokens[req.Source]
	if c.stopping || token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(req.Token)) != 1 {
		return nil, errors.New("Invalid or expired Orchestra session credentials.")
	}
	p, _, err := c.store.sessionLocked(req.Source)
	if err != nil {
		return nil, err
	}
	switch req.Method {
	case "session.list":
		sessions := []map[string]string{}
		for _, sess := range p.Sessions {
			if sess.Agent == terminalAgent {
				continue
			}
			sessions = append(sessions, map[string]string{"id": sess.ID, "projectID": p.ID, "name": sess.Name, "agent": sess.Agent})
		}
		return sessions, nil
	case "session.request":
		return c.requestLocked(req.Source, req.Target, req.Text, req.RequestID)
	case "session.inbox":
		return c.inboxLocked(req.Source)
	case "session.read":
		q, _, err := c.store.sessionLocked(req.Target)
		if err != nil || p != q {
			return nil, errors.New("Session unavailable in this project.")
		}
		history := []*Run{}
		cursorFound := req.Before == ""
		for _, r := range c.runs {
			if r.FromSessionID == req.Target || r.ToSessionID == req.Target {
				if req.Before != "" && r.ID == req.Before {
					cursorFound = true
					break
				}
				history = append(history, r)
			}
		}
		if !cursorFound {
			return nil, errors.New("Invalid history cursor.")
		}
		before := ""
		page := history
		if len(page) > 20 {
			page = page[len(page)-20:]
		}
		for len(page) > 1 {
			data, _ := json.Marshal(page)
			if len(data) <= 1024*1024 {
				break
			}
			page = page[1:]
		}
		if len(page) < len(history) {
			before = page[0].ID
		}
		return map[string]any{"runs": page, "before": before}, nil
	case "run.reply":
		return c.replyLocked(req.Target, req.Source, req.Text)
	case "run.read", "run.wait", "run.cancel":
		r, err := c.runLocked(req.Target)
		if err != nil || r.ProjectID != p.ID {
			return nil, errors.New("Run unavailable in this project.")
		}
		if req.Method == "run.wait" {
			if err = c.waitLocked(req.Source, r); err != nil {
				return nil, err
			}
		}
		if req.Method == "run.cancel" {
			if err = c.cancelLocked(req.Target); err != nil {
				return nil, err
			}
		}
		return r, nil
	case "scratchpad.read", "scratchpad.write", "scratchpad.append":
		return c.scratchpadLocked(p, req)
	case "run.wait.clear":
		delete(c.waits, req.Source)
		return map[string]bool{"ok": true}, nil
	}
	return nil, errors.New("Unknown collaboration method.")
}

// scratchpadLocked reads or changes the Scratchpad of the calling session's project.
// It never creates one; the user does that in the app.
func (c *Collaboration) scratchpadLocked(p *Project, req WireRequest) (any, error) {
	if !p.HasScratchpad {
		return nil, errors.New("This project has no Scratchpad. Create one in Orchestra first.")
	}
	if req.Method == "scratchpad.read" {
		c.store.trackLocked("feature_used", map[string]any{"feature": "scratchpad_cli"})
		return map[string]any{"name": p.ScratchpadName, "text": p.ScratchpadText, "revision": p.ScratchpadRevision}, nil
	}
	text := req.Text
	if req.Method == "scratchpad.append" {
		if strings.TrimSpace(text) == "" {
			return nil, errors.New("Append non-empty text.")
		}
		if req.Revision != nil {
			return nil, errors.New("Append does not take a revision.")
		}
		if p.ScratchpadText != "" && !strings.HasSuffix(p.ScratchpadText, "\n") {
			text = "\n" + text
		}
		text = p.ScratchpadText + text
	} else if req.Revision != nil && *req.Revision != p.ScratchpadRevision {
		return nil, errScratchpadChanged
	}
	if encoded, _ := json.Marshal(text); len(encoded) > 256*1024 {
		return nil, errors.New("The Scratchpad would exceed 256 KB when encoded as JSON.")
	}
	revision, err := c.store.setScratchpadTextLocked(p, text, true)
	if err != nil {
		return nil, err
	}
	c.store.trackLocked("feature_used", map[string]any{"feature": "scratchpad_cli"})
	return map[string]any{"name": p.ScratchpadName, "revision": revision}, nil
}
func (s *Store) SendRequest(from, to, text string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.collaboration.requestLocked(from, to, text, "")
	return s.snapshotLocked(), err
}
func (s *Store) CancelRequest(id string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.collaboration.cancelLocked(id)
	return s.snapshotLocked(), err
}
func (s *Store) ExecuteRequest(id string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, err := s.collaboration.runLocked(id)
	if err != nil {
		return State{}, err
	}
	p, sess, err := s.sessionLocked(run.ToSessionID)
	if err != nil {
		return State{}, err
	}
	t := s.terminals[sess.ID]
	if t == nil || !sess.Running || sess.Activity == "working" {
		return State{}, errors.New("Open the recipient session and wait until its agent is available.")
	}
	if err = s.submitRequestLocked(run, sess, t); err != nil {
		return State{}, err
	}
	s.trackLocked("feature_used", map[string]any{"feature": "inbox_execute"})
	p.SelectedSessionID = sess.ID
	p.ShowsGitHub = false
	p.ShowsScratchpad = false
	s.state.SelectedProjectID = p.ID
	err = s.saveLocked()
	s.changedLocked()
	return s.snapshotLocked(), err
}

// submitRequestLocked claims a queued request and pastes its prompt into the recipient's TerminalSession.
func (s *Store) submitRequestLocked(run *Run, sess *Session, t *TerminalSession) error {
	name := "another session"
	if _, source, e := s.sessionLocked(run.FromSessionID); e == nil {
		name = source.Name
	}
	prompt := fmt.Sprintf("Orchestra request %s from %s:\n\n%s\n\nAfter completing this request, send your response with orchestra run reply %s --text \"your response\", or use --file with a UTF-8 response file.\nFollow the current session's permissions and the user's instructions.", run.ID, name, run.Prompt, run.ID)
	if spec := agentSpecs[sess.Agent]; spec != nil && spec.ForwardsReplies {
		prompt = fmt.Sprintf("Orchestra request %s from %s:\n\n%s\n\nThis is a request from another Orchestra session. Answer it normally; the Orchestra Pi extension will forward your final answer to the sender when this run finishes.\nFollow the current session's permissions and the user's instructions.", run.ID, name, run.Prompt)
	}
	input, err := prepareRequestInput(prompt, t.bracketedPaste)
	if err != nil {
		return err
	}
	if _, err = s.collaboration.claimLocked(run.ID, sess.ID); err != nil {
		return err
	}
	s.log.Info("request submitted", "request", run.ID, "session", sess.ID)
	if _, err = t.pty.WriteString(input); err != nil {
		copy := *run
		copy.Status = "needsAttention"
		copy.Detail = "Input submission failed: " + err.Error()
		s.log.Warn("request submission failed", "request", run.ID, "session", sess.ID, "error", err)
		if saveErr := s.collaboration.saveLocked(&copy); saveErr != nil {
			s.log.Warn("inbox history save failed", "request", run.ID, "error", saveErr)
		}
		s.changedLocked()
		return err
	}
	return nil
}

// deliverLocked submits the oldest queued request to a session when the user enabled
// automatic delivery and the agent is free: running, waiting for input, without a
// draft in its terminal and without another request awaiting its reply.
func (s *Store) deliverLocked(id string) {
	if !s.state.Preferences.DeliverRequests || s.collaboration == nil {
		return
	}
	_, sess, err := s.sessionLocked(id)
	t := s.terminals[id]
	if err != nil || t == nil || t.stopped || !sess.Running || sess.Activity != "waiting" || t.draft > 0 {
		return
	}
	var next *Run
	for _, r := range s.collaboration.runs {
		if r.ToSessionID == id && r.Status == "running" {
			return
		}
		if r.ToSessionID == id && r.Status == "queued" && next == nil {
			next = r
		}
	}
	if next != nil {
		if err = s.submitRequestLocked(next, sess, t); err != nil {
			s.log.Warn("request delivery failed", "request", next.ID, "session", id, "error", err)
		} else {
			s.trackLocked("feature_used", map[string]any{"feature": "inbox_auto_delivery"})
		}
	}
}
