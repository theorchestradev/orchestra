package core

import (
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/creack/pty"
	"github.com/theorchestradev/orchestra/internal/telemetry"
)

//go:embed resources/*.js
var resourceFS embed.FS
var resources = func() map[string][]byte {
	m := map[string][]byte{}
	for _, name := range []string{"PiStatusExtension.js", "OpenCodeSessionPlugin.js", "OmpStatusExtension.js"} {
		m[name], _ = resourceFS.ReadFile("resources/" + name)
	}
	return m
}()

// terminalAgent marks a plain login shell session; it is not an agent and stays out of agentSpecs.
const terminalAgent = "terminal"

func sessionKindName(agent string) (string, bool) {
	if agent == terminalAgent {
		return "Terminal", true
	}
	if spec, ok := agentSpecs[agent]; ok {
		return spec.Name, true
	}
	return "", false
}
func loginShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	if runtime.GOOS == "darwin" {
		return "/bin/zsh"
	}
	// Linux desktop sessions can start apps without SHELL; use the passwd entry instead.
	if u, err := user.Current(); err == nil {
		data, _ := os.ReadFile("/etc/passwd")
		for _, line := range strings.Split(string(data), "\n") {
			if fields := strings.Split(line, ":"); len(fields) == 7 && fields[0] == u.Username && fields[6] != "" {
				return fields[6]
			}
		}
	}
	return "/bin/sh"
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
func statusCommand(state string) string {
	return "if [ -n \"$ORCHESTRA_AGENT_ID_FILE\" ]; then \"$ORCHESTRA_CLI_DIRECTORY/orchestra\" agent-id >/dev/null 2>&1 || :; else /bin/cat >/dev/null; fi; if [ -n \"$ORCHESTRA_STATUS_FILE\" ]; then printf " + state + " > \"$ORCHESTRA_STATUS_FILE\"; fi"
}
func launchCommand(sess *Session, dir string) (string, error) {
	if sess.Agent == terminalAgent {
		// A login session runs only the login command and ends with it; restarting it opens a
		// shell. A shell started after the login would read the user's rc files again, which
		// may choose another config folder.
		if command := loginCommands[sess.LoginAgent]; command != "" && !sess.HasLaunched {
			return command, nil
		}
		return shellQuote(loginShell()) + " -l", nil
	}
	spec, ok := agentSpecs[sess.Agent]
	if !ok {
		return "", errors.New("Unknown agent.")
	}
	l := launch{Session: sess, Flags: launchFlags(sess), Dir: dir}
	if sess.HasLaunched {
		l.Resume = sess.AgentSessionID
	}
	if l.Resume == "" && sess.InitialPrompt != "" {
		l.Prompt = shellQuote(sess.InitialPrompt)
	}
	return spec.Command(l), nil
}
func tomlString(v string) string {
	return "\"" + strings.ReplaceAll(strings.ReplaceAll(v, "\\", "\\\\"), "\"", "\\\"") + "\""
}
func environmentMap() map[string]string {
	m := map[string]string{}
	for _, entry := range os.Environ() {
		k, v, ok := strings.Cut(entry, "=")
		if ok {
			m[k] = v
		}
	}
	return m
}
func environmentList(m map[string]string) []string {
	out := []string{}
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

type TerminalSession struct {
	cmd                *exec.Cmd
	pty                *os.File
	done               chan struct{}
	statusFile, idFile string
	// agentDirectory is a folder the agent's Prepare reserved for its own files.
	agentDirectory string
	launched       time.Time
	buffer         []byte
	offset         int64
	bracketedPaste bool
	// resumed is true when the launch continued a saved conversation.
	resumed        bool
	stopped        bool
	telemetryEnded bool
	// draft approximates how many characters the user has typed without submitting them.
	draft int
	log   *slog.Logger
}
type TerminalOutput struct {
	ID    string `json:"id"`
	Data  string `json:"data"`
	Start int64  `json:"start"`
	End   int64  `json:"end"`
}

func (t *TerminalSession) output(id string) TerminalOutput {
	return TerminalOutput{id, base64.StdEncoding.EncodeToString(t.buffer), t.offset - int64(len(t.buffer)), t.offset}
}
func (t *TerminalSession) terminate() {
	if t.stopped {
		return
	}
	t.stopped = true
	t.signal(syscall.SIGTERM)
	if err := t.pty.Close(); err != nil {
		t.log.Debug("terminal close failed", "error", err)
	}
	go func() {
		select {
		case <-t.done:
			return
		case <-time.After(time.Second):
			t.log.Info("session did not stop after SIGTERM; sending SIGKILL")
			t.signal(syscall.SIGKILL)
		}
	}()
}

// signal sends sig to the session's process group. A group that already exited is expected.
func (t *TerminalSession) signal(sig syscall.Signal) {
	if err := syscall.Kill(-t.cmd.Process.Pid, sig); errors.Is(err, syscall.ESRCH) {
		t.log.Debug("session signal failed", "signal", sig.String(), "error", err)
	} else if err != nil {
		t.log.Warn("session signal failed", "signal", sig.String(), "error", err)
	}
}
func (s *Store) StartSession(id string, newConversation bool, cols, rows int) (TerminalOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, sess, err := s.sessionLocked(id)
	if err != nil {
		return TerminalOutput{}, err
	}
	if t := s.terminals[id]; t != nil && !t.stopped && sess.Running {
		return t.output(id), nil
	}
	if sess.Agent != terminalAgent && sess.HasLaunched && sess.AgentSessionID == "" && !newConversation {
		return TerminalOutput{}, errors.New("Conversation ID unavailable. Choose Start New Conversation explicitly.")
	}
	if newConversation {
		sess.HasLaunched = false
		sess.AgentSessionID = ""
		if presetsID(sess.Agent) {
			sess.AgentSessionID = sess.ID
		}
	}
	directory, err := sessionFolder(p.Directory, sess.WorktreeDirectory)
	if err != nil {
		return TerminalOutput{}, err
	}
	profile, err := s.profileLocked(sess.ProfileID)
	if err != nil {
		return TerminalOutput{}, errProfileRemoved
	}
	command, err := launchCommand(sess, s.runtimeDir)
	if err != nil {
		return TerminalOutput{}, err
	}
	env := environmentMap()
	// The profile's config folders select the agent accounts. A resumed session uses the same
	// folders, because the agent keeps the conversation there.
	folders, err := profileEnvironment(profile)
	if err != nil {
		return TerminalOutput{}, err
	}
	shell := loginShell()
	env["TERM"] = "xterm-256color"
	env["COLORTERM"] = "truecolor"
	if env["LANG"] == "" {
		env["LANG"] = "en_US.UTF-8"
	}
	t := &TerminalSession{statusFile: filepath.Join(s.runtimeDir, id+".status"), idFile: filepath.Join(s.runtimeDir, id+".agent"), done: make(chan struct{}), launched: time.Now(), log: s.log.With("session", id)}
	env["ORCHESTRA_STATUS_FILE"] = t.statusFile
	env["ORCHESTRA_AGENT_ID_FILE"] = t.idFile
	collab, err := s.collaboration.environmentLocked(id)
	if err != nil {
		return TerminalOutput{}, err
	}
	for k, v := range collab {
		env[k] = v
	}
	if spec, ok := agentSpecs[sess.Agent]; ok && spec.Prepare != nil {
		if err = spec.Prepare(s, sess, t, env); err != nil {
			return TerminalOutput{}, err
		}
	}
	// The profile's folders are exported after the shell reads the user's rc files, so an export
	// there cannot switch account. env runs the agent's executable, never an rc alias or function.
	setup := "export PATH=" + shellQuote(s.runtimeDir) + ":$PATH; "
	for _, k := range slices.Sorted(maps.Keys(folders)) {
		setup += "export " + k + "=" + shellQuote(folders[k]) + "; "
	}
	setup += "cd -- " + shellQuote(directory) + " && exec /usr/bin/env " + command
	t.cmd = exec.Command(shell, "-l", "-i", "-c", setup)
	t.cmd.Dir = directory
	t.cmd.Env = environmentList(env)
	if cols < 2 || cols > 1000 {
		cols = 100
	}
	if rows < 2 || rows > 1000 {
		rows = 32
	}
	t.pty, err = pty.StartWithSize(t.cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		t.log.Warn("session start failed", "agent", sess.Agent, "error", err)
		return TerminalOutput{}, err
	}
	t.resumed = sess.Agent != terminalAgent && sess.HasLaunched && sess.AgentSessionID != ""
	t.log.Info("session started", "agent", sess.Agent, "worktree", sess.WorktreeDirectory != "", "resume", t.resumed)
	s.terminals[id] = t
	sess.HasLaunched = true
	sess.LoginAgent = ""
	sess.Restored = false
	sess.Running = true
	sess.Activity = "unknown"
	sess.ExitMessage = ""
	sess.StatusNeedsSetup = false
	if err = s.saveLocked(); err != nil {
		t.terminate()
		return TerminalOutput{}, err
	}
	s.changedLocked()
	s.trackLocked("session_started", map[string]any{"agent": sess.Agent, "worktree": sess.WorktreeDirectory != "", "from_github_item": sess.GitHubItem != nil})
	go s.readTerminal(id, t)
	go s.monitorTerminal(id, t)
	return t.output(id), nil
}

// resumeFailureWindow is how long after launch a failed resume can still be blamed on a missing conversation.
const resumeFailureWindow = 30 * time.Second

var ansiRE = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

func (s *Store) readTerminal(id string, t *TerminalSession) {
	buf := make([]byte, 32768)
	for {
		n, err := t.pty.Read(buf)
		if n > 0 {
			s.mu.Lock()
			if s.terminals[id] == t {
				start := t.offset
				t.buffer = append(t.buffer, buf[:n]...)
				t.offset += int64(n)
				if len(t.buffer) > 4*1024*1024 {
					t.buffer = append([]byte{}, t.buffer[len(t.buffer)-4*1024*1024:]...)
				}
				tail := t.buffer
				if len(tail) > 4096 {
					tail = tail[len(tail)-4096:]
				}
				on, off := strings.LastIndex(string(tail), "\x1b[?2004h"), strings.LastIndex(string(tail), "\x1b[?2004l")
				if on >= 0 || off >= 0 {
					t.bracketedPaste = on > off
				}
				s.emit("terminal", TerminalOutput{id, base64.StdEncoding.EncodeToString(buf[:n]), start, t.offset})
			}
			s.mu.Unlock()
		}
		if err != nil {
			break
		}
	}
	err := t.cmd.Wait()
	// done closes after the exit has been saved, so waiting for it covers the final write.
	defer close(t.done)
	s.mu.Lock()
	defer s.mu.Unlock()
	logExit(t, err)
	if s.terminals[id] != t {
		return
	}
	_, sess, e := s.sessionLocked(id)
	if e != nil {
		return
	}
	s.captureLocked(sess, t)
	kind := "normal"
	if t.stopped {
		kind = "killed"
	} else if status, ok := t.cmd.ProcessState.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		kind = "killed"
	} else if err != nil {
		kind = "error"
	}
	s.trackSessionEndLocked(sess, t, kind)
	text := ansiRE.ReplaceAllString(string(t.buffer), "")
	spec := agentSpecs[sess.Agent]
	if spec != nil && spec.RecoverID != nil && sess.AgentSessionID == "" {
		matches := spec.RecoverID.FindAllStringSubmatch(text, -1)
		if len(matches) > 0 {
			sess.AgentSessionID = matches[len(matches)-1][1]
		}
	}
	sess.Running = false
	sess.Activity = "ended"
	sess.StatusNeedsSetup = false
	t.stopped = true
	code := t.cmd.ProcessState.ExitCode()
	name, _ := sessionKindName(sess.Agent)
	sess.ExitMessage = fmt.Sprintf("%s exited (code %d).", name, code)
	if code == 127 && spec != nil {
		sess.ExitMessage = fmt.Sprintf("Could not start %s. Check that the %s CLI is in your shell’s PATH.", spec.Name, spec.Executable)
	} else if code != 0 && t.resumed && time.Since(t.launched) < resumeFailureWindow && spec != nil && spec.ResumeFailed != nil && spec.ResumeFailed.MatchString(text) {
		// Resuming again would fail the same way; without the ID the session offers a new conversation.
		// A resume that fails exits right away, so a long session that merely printed the text keeps its ID.
		t.log.Info("saved conversation not found", "agent", sess.Agent)
		sess.AgentSessionID = ""
		sess.ExitMessage = fmt.Sprintf("%s has no saved conversation for this session. Start a new conversation to continue.", spec.Name)
	} else if err != nil && code < 0 {
		sess.ExitMessage = "The terminal connection was closed."
	}
	_ = os.Remove(t.statusFile)
	_ = os.Remove(t.idFile)
	if err = t.pty.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		t.log.Debug("terminal close failed", "error", err)
	}
	if err = s.saveLocked(); err != nil {
		t.log.Warn("workspace save failed", "error", err)
		s.state.Error = err.Error()
	}
	s.changedLocked()
}

func (s *Store) trackSessionEndLocked(sess *Session, t *TerminalSession, kind string) {
	if t.telemetryEnded {
		return
	}
	t.telemetryEnded = true
	s.trackLocked("session_ended", map[string]any{"agent": sess.Agent, "exit_kind": kind, "duration_bucket": telemetry.DurationBucket(time.Since(t.launched))})
	if kind == "error" {
		s.trackLocked("error_shown", map[string]any{"code": "session_exit_error"})
	}
}

// logExit records how a session's process ended: its exit code, or the signal that stopped it.
func logExit(t *TerminalSession, err error) {
	attrs := []any{"stopped", t.stopped}
	if state := t.cmd.ProcessState; state == nil {
		attrs = append(attrs, "error", err)
	} else if status, ok := state.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		attrs = append(attrs, "signal", status.Signal().String())
	} else {
		attrs = append(attrs, "code", state.ExitCode())
	}
	t.log.Info("session exited", attrs...)
}
func (s *Store) captureLocked(sess *Session, t *TerminalSession) bool {
	changed := false
	if data, err := os.ReadFile(t.idFile); err == nil {
		value := strings.TrimSpace(string(data))
		if value != "" && len(value) < 1024 && sess.AgentSessionID != value {
			sess.AgentSessionID = value
			changed = true
		}
	}
	if spec := agentSpecs[sess.Agent]; spec != nil && spec.CaptureID != nil {
		if value := spec.CaptureID(t); value != "" && value != sess.AgentSessionID {
			sess.AgentSessionID = value
			changed = true
		}
	}
	return changed
}
func (s *Store) monitorTerminal(id string, t *TerminalSession) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	checks := 0
	for {
		select {
		case <-t.done:
			return
		case <-ticker.C:
			s.mu.Lock()
			_, sess, err := s.sessionLocked(id)
			if err != nil || s.terminals[id] != t || !sess.Running {
				s.mu.Unlock()
				return
			}
			changed := s.captureLocked(sess, t)
			if changed {
				if err = s.saveLocked(); err != nil {
					t.log.Warn("workspace save failed", "error", err)
					s.state.Error = err.Error()
				}
			}
			if data, err := os.ReadFile(t.statusFile); err == nil {
				value := strings.TrimSpace(string(data))
				if value == "waiting" || value == "working" {
					if sess.Activity != value {
						sess.Activity = value
						changed = true
						if value == "working" {
							t.draft = 0
						}
					}
					if sess.StatusNeedsSetup {
						sess.StatusNeedsSetup = false
						changed = true
					}
					checks = 0
				}
			} else {
				checks++
				if spec := agentSpecs[sess.Agent]; spec != nil && spec.StatusSetup && checks >= 40 && !sess.StatusNeedsSetup {
					sess.StatusNeedsSetup = true
					changed = true
				}
			}
			if changed {
				s.changedLocked()
				if sess.Activity == "waiting" {
					s.deliverLocked(id)
				}
			}
			s.mu.Unlock()
		}
	}
}
func (s *Store) TerminalSnapshot(id string) (TerminalOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t := s.terminals[id]; t != nil {
		return t.output(id), nil
	}
	return TerminalOutput{ID: id}, nil
}
func (s *Store) WriteTerminal(id, data string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, sess, err := s.sessionLocked(id)
	if err != nil {
		return err
	}
	t := s.terminals[id]
	if t == nil || !sess.Running {
		return errors.New("Session is not running.")
	}
	if len(data) > 1024*1024 {
		return errors.New("Terminal input is too large.")
	}
	if _, err = t.pty.WriteString(data); err != nil {
		return err
	}
	t.draft = draftLength(t.draft, data)
	return nil
}

var inputEscapeRE = regexp.MustCompile(`\x1b(\[[0-?]*[ -/]*[@-~]|O.|.)`)

// draftLength estimates the unsubmitted prompt after the user's input so automatic
// delivery never mixes a request with text being typed. Pastes always count as a draft.
func draftLength(draft int, data string) int {
	if strings.Contains(data, "\x1b[200~") {
		return draft + utf8.RuneCountInString(inputEscapeRE.ReplaceAllString(data, ""))
	}
	for _, r := range inputEscapeRE.ReplaceAllString(data, "") {
		switch {
		case r == '\r' || r == '\x03' || r == '\x15': // Enter, Ctrl-C, Ctrl-U
			draft = 0
		case r == '\x7f' || r == '\b':
			draft = max(draft-1, 0)
		case !unicode.IsControl(r):
			draft++
		}
	}
	return draft
}
func (s *Store) ResizeTerminal(id string, cols, rows int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cols < 2 || rows < 2 || cols > 1000 || rows > 1000 {
		return errors.New("Invalid terminal dimensions.")
	}
	t := s.terminals[id]
	if t == nil || t.stopped {
		return nil
	}
	return pty.Setsize(t.pty, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}
func prepareRequestInput(prompt string, bracketed bool) (string, error) {
	for _, r := range prompt {
		if unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r' {
			return "", errors.New("The request contains unsupported terminal control characters.")
		}
	}
	text := strings.ReplaceAll(strings.ReplaceAll(prompt, "\r\n", "\n"), "\r", "\n")
	if bracketed {
		return "\x1b[200~" + text + "\x1b[201~\r", nil
	}
	return strings.NewReplacer("\n", " ", "\t", " ").Replace(text) + "\r", nil
}
