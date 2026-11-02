package core

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// Aider restores a chat-history file instead of a conversation ID. Each explicit
// new conversation gets its own persistent folder, including in a shared checkout.
var aiderAgent = &agentSpec{
	ID: "aider", Name: "Aider", Executable: "aider",
	Command: func(l launch) string {
		command := `aider --notifications --notifications-command ` + shellQuote(agentStatusCommand("aider", "waiting")+" </dev/null") + ` --chat-history-file "$ORCHESTRA_AIDER_HISTORY/chat.md" --input-history-file "$ORCHESTRA_AIDER_HISTORY/input"`
		if l.Resume != "" {
			command += " --restore-chat-history"
		}
		return command + l.Flags
	},
	NoInitialPrompt: true,
	Prepare: func(s *Store, sess *Session, t *TerminalSession, env map[string]string) error {
		id := sess.AgentSessionID
		if !sess.HasLaunched || id == "" {
			id = uuid.NewString()
		}
		if _, err := uuid.Parse(id); err != nil {
			return err
		}
		t.agentDirectory = filepath.Join(filepath.Dir(s.path), "AgentSessions", "Aider", id)
		env["ORCHESTRA_AIDER_HISTORY"] = t.agentDirectory
		return os.MkdirAll(t.agentDirectory, 0700)
	},
	CaptureID: func(t *TerminalSession) string {
		if t.agentDirectory == "" {
			return ""
		}
		return filepath.Base(t.agentDirectory)
	},

	Model: &agentModelSpec{
		Flag:    "--model",
		Command: []string{"aider", "--list-models", ""},
		Parse: func(output []byte) ([]AgentModel, error) {
			return scanModels(output, func(c *modelCatalog, line string, _ []string) {
				if id, ok := strings.CutPrefix(strings.TrimSpace(line), "- "); ok {
					c.add(id, id, []string{})
				}
			})
		},
	},
}
