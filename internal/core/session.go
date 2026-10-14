package core

import (
	"embed"
	"encoding/base64"
	"encoding/json"
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
	for _, name := range []string{"PiStatusExtension.js", "OpenCodeSessionPlugin.js"} {
		m[name], _ = resourceFS.ReadFile("resources/" + name)
	}
	return m
}()
var agentNames = map[string]string{"codex": "Codex", "claude": "Claude Code", "pi": "Pi", "vibe": "Mistral Vibe", "opencode": "OpenCode", "antigravity": "Antigravity CLI", "grok": "Grok Build"}

// terminalAgent marks a plain login shell session; it is not an agent and stays out of agentNames.
const terminalAgent = "terminal"

func sessionKindName(agent string) (string, bool) {
	if agent == terminalAgent {
		return "Terminal", true
	}
	name, ok := agentNames[agent]
	return name, ok
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
	resume := ""
	if sess.HasLaunched {
		resume = sess.AgentSessionID
	}
	flags := launchFlags(sess)
	prompt := ""
	if resume == "" && sess.InitialPrompt != "" {
		prompt = shellQuote(sess.InitialPrompt)
	}
	events := []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "Stop"}
	state := func(event string) string {
		if event == "UserPromptSubmit" || event == "PreToolUse" {
			return "working"
		}
		return "waiting"
	}
	// Agents start in their auto mode: routine approvals are skipped while a
	// safety layer still reviews risky actions. Full bypass flags are never used.
	// Vibe and OpenCode only offer approve-all modes, so they keep their default
	// prompts; Pi has no tool approval prompts. Antigravity has no reviewed auto
	// mode, so it only auto-approves edits.
	switch sess.Agent {
	case "codex":
		command := "codex"
		if resume != "" {
			command += " resume " + shellQuote(resume)
		}
		command += " --approve-for-me"
		for _, event := range append(events, "Interrupt") {
			hook := fmt.Sprintf("[{hooks=[{type=\"command\",command=%s}]}]", tomlString(statusCommand(state(event))))
			command += " -c " + shellQuote("hooks."+event+"="+hook)
		}
		command += flags
		if prompt != "" {
			command += " " + prompt
		}
		return command, nil
	case "claude":
		hooks := map[string]any{}
		for _, event := range append(events, "PermissionRequest") {
			group := map[string]any{"hooks": []any{map[string]string{"type": "command", "command": statusCommand(state(event))}}}
			if event == "PreToolUse" || event == "PermissionRequest" {
				group["matcher"] = "*"
			}
			hooks[event] = []any{group}
		}
		data, _ := json.Marshal(map[string]any{"hooks": hooks})
		command := "claude"
		if resume != "" {
			command += " --resume " + shellQuote(resume)
		} else {
			command += " --session-id " + shellQuote(sess.ID)
		}
		command += " --permission-mode auto --settings " + shellQuote(string(data)) + flags
		if prompt != "" {
			command += " " + prompt
		}
		return command, nil
	case "pi":
		id := resume
		if id == "" {
			id = sess.ID
		}
		command := "pi --session-id " + shellQuote(id) + " --extension " + shellQuote(filepath.Join(dir, "PiStatusExtension.js")) + flags
		if prompt != "" {
			command += " -- " + prompt
		}
		return command, nil
	case "grok":
		if resume != "" {
			return "grok --resume " + shellQuote(resume) + " --permission-mode auto" + flags, nil
		}
		return "grok --session-id " + shellQuote(sess.ID) + " --permission-mode auto" + flags, nil
	case "opencode", "vibe", "antigravity":
		command := sess.Agent
		resumeFlag := "--resume"
		promptFlag := "--"
		if sess.Agent == "opencode" {
			resumeFlag = "--session"
			promptFlag = "--prompt"
		}
		if sess.Agent == "antigravity" {
			command = "agy --mode accept-edits"
			resumeFlag = "--conversation"
			promptFlag = "--prompt-interactive"
		}
		if resume != "" {
			command += " " + resumeFlag + " " + shellQuote(resume)
		}
		command += flags
		if prompt != "" {
			command += " " + promptFlag + " " + prompt
		}
		return command, nil
	}
	return "", errors.New("Unknown agent.")
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
	cmd                               *exec.Cmd
	pty                               *os.File
	done                              chan struct{}
	statusFile, idFile, vibeDirectory string
	launched                          time.Time
	buffer                            []byte
	offset                            int64
	bracketedPaste                    bool
	stopped                           bool
	telemetryEnded                    bool
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
		if sess.Agent == "claude" || sess.Agent == "pi" || sess.Agent == "grok" {
			sess.AgentSessionID = sess.ID
		}
	}
	directory := p.Directory
	if sess.WorktreeDirectory != "" {
		directory = sess.WorktreeDirectory
	}
	if info, e := os.Stat(directory); e != nil || !info.IsDir() {
		if sess.WorktreeDirectory != "" {
			return TerminalOutput{}, errWorktreeMissing
		}
		return TerminalOutput{}, errors.New("The project folder is no longer available.")
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
	if sess.Agent == "opencode" {
		config := map[string]any{}
		if env["OPENCODE_CONFIG_CONTENT"] != "" {
			if err = json.Unmarshal([]byte(env["OPENCODE_CONFIG_CONTENT"]), &config); err != nil {
				return TerminalOutput{}, err
			}
		}
		plugins, _ := config["plugin"].([]any)
		config["plugin"] = append(plugins, "file://"+filepath.Join(s.runtimeDir, "OpenCodeSessionPlugin.js"))
		data, _ := json.Marshal(config)
		env["OPENCODE_CONFIG_CONTENT"] = string(data)
		env["ORCHESTRA_RESUME_ID"] = sess.AgentSessionID
	}
	if sess.Agent == "vibe" {
		t.vibeDirectory = filepath.Join(filepath.Dir(s.path), "AgentSessions", "Vibe", sess.ID)
		if err = os.MkdirAll(t.vibeDirectory, 0700); err != nil {
			return TerminalOutput{}, err
		}
		config := map[string]any{}
		if env["VIBE_SESSION_LOGGING"] != "" {
			if err = json.Unmarshal([]byte(env["VIBE_SESSION_LOGGING"]), &config); err != nil {
				return TerminalOutput{}, err
			}
		}
		config["enabled"] = true
		config["save_dir"] = t.vibeDirectory
		data, _ := json.Marshal(config)
		env["VIBE_SESSION_LOGGING"] = string(data)
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
	t.log.Info("session started", "agent", sess.Agent, "worktree", sess.WorktreeDirectory != "", "resume", sess.HasLaunched && sess.AgentSessionID != "")
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

var ansiRE = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
var codexIDRE = regexp.MustCompile(`Session ID:\s*([0-9a-fA-F-]{36})`)
var agyIDRE = regexp.MustCompile(`Resume:\s*agy\s+--conversation(?:=|\s+)([0-9a-fA-F-]{36})`)

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
	re := codexIDRE
	if sess.Agent == "antigravity" {
		re = agyIDRE
	}
	if sess.AgentSessionID == "" && (sess.Agent == "codex" || sess.Agent == "antigravity") {
		matches := re.FindAllStringSubmatch(text, -1)
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
	if code == 127 && sess.Agent != terminalAgent {
		command := sess.Agent
		if command == "antigravity" {
			command = "agy"
		}
		sess.ExitMessage = fmt.Sprintf("Could not start %s. Check that the %s CLI is in your shell’s PATH.", agentNames[sess.Agent], command)
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
	if t.vibeDirectory != "" {
		entries, _ := os.ReadDir(t.vibeDirectory)
		latest := t.launched
		for _, entry := range entries {
			path := filepath.Join(t.vibeDirectory, entry.Name())
			if entry.IsDir() {
				path = filepath.Join(path, "meta.json")
			}
			info, err := os.Stat(path)
			if err != nil || info.ModTime().Before(latest) || info.Size() > 1024*1024 {
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var meta map[string]any
			if json.Unmarshal(data, &meta) != nil {
				continue
			}
			if nested, ok := meta["metadata"].(map[string]any); ok {
				meta = nested
			}
			value, _ := meta["session_id"].(string)
			if value != "" {
				latest = info.ModTime()
				if value != sess.AgentSessionID {
					sess.AgentSessionID = value
					changed = true
				}
			}
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
				if sess.Agent == "codex" && checks >= 40 && !sess.StatusNeedsSetup {
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
