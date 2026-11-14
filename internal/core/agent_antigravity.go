package core

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Antigravity prints its resume command only when it exits on its own, so it
// writes a per-session log where CaptureID follows the conversation it streams.
var antigravityAgent = &agentSpec{
	ID: "antigravity", Name: "Antigravity CLI", Executable: "agy", SkillKey: "antigravity-cli",
	Command: func(l launch) string {
		return l.command(`agy --mode accept-edits --log-file "$ORCHESTRA_ANTIGRAVITY_LOG"`, "--conversation", "--prompt-interactive")
	},
	Prepare: func(s *Store, sess *Session, t *TerminalSession, env map[string]string) error {
		t.agentDirectory = filepath.Join(filepath.Dir(s.path), "AgentSessions", "Antigravity", sess.ID)
		if err := os.MkdirAll(t.agentDirectory, 0700); err != nil {
			return err
		}
		log := filepath.Join(t.agentDirectory, "cli.log")
		// Each launch starts a fresh log, so an earlier conversation is never captured again.
		if err := os.Remove(log); err != nil && !os.IsNotExist(err) {
			return err
		}
		env["ORCHESTRA_ANTIGRAVITY_LOG"] = log
		return nil
	},
	CaptureID: captureAntigravityID,

	Model: &agentModelSpec{
		Flag:    "--model",
		Command: []string{"agy", "models"},
		Parse: func(output []byte) ([]AgentModel, error) {
			return scanModels(output, func(c *modelCatalog, line string, _ []string) {
				if id, name, ok := strings.Cut(line, "\t"); ok {
					c.add(strings.TrimSpace(id), strings.TrimSpace(name), []string{})
				}
			})
		},
	},
}

var antigravityStreamRE = regexp.MustCompile(`Streaming conversation ([0-9a-fA-F-]{36})`)

// antigravityLogTail bounds each read; an ID older than the tail was already captured.
const antigravityLogTail = 256 * 1024

// captureAntigravityID returns the conversation the CLI streamed last, new or resumed.
func captureAntigravityID(t *TerminalSession) string {
	if t.agentDirectory == "" {
		return ""
	}
	file, err := os.Open(filepath.Join(t.agentDirectory, "cli.log"))
	if err != nil {
		return ""
	}
	defer file.Close()
	if info, err := file.Stat(); err == nil && info.Size() > antigravityLogTail {
		_, _ = file.Seek(-antigravityLogTail, io.SeekEnd)
	}
	data, _ := io.ReadAll(file)
	matches := antigravityStreamRE.FindAllSubmatch(data, -1)
	if len(matches) == 0 {
		return ""
	}
	return string(matches[len(matches)-1][1])
}
