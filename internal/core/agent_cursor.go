package core

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// Cursor creates an empty chat locally before attaching its interactive CLI. The
// ID is captured through the same file hooks use; no model prompt is sent here.
var cursorAgent = &agentSpec{
	ID: "cursor", Name: "Cursor CLI", Executable: "cursor-agent", SkillKey: "cursor",
	Command: func(l launch) string {
		plugin := " --plugin-dir " + shellQuote(filepath.Join(l.Dir, "cursor-status"))
		if l.Resume == "" {
			return l.command(`cursor-agent`+plugin+` --resume "$(id=$(command cursor-agent create-chat) && [ -n "$id" ] && printf %s "$id" | tee "$ORCHESTRA_AGENT_ID_FILE" || printf orchestra-chat-creation-failed)"`, "", "--")
		}
		return l.command("cursor-agent"+plugin, "--resume", "--")
	},
	Prepare: func(s *Store, _ *Session, _ *TerminalSession, _ map[string]string) error {
		dir := filepath.Join(s.runtimeDir, "cursor-status")
		if err := writeAgentJSON(filepath.Join(dir, ".cursor-plugin", "plugin.json"), map[string]string{"name": "orchestra-session-status", "version": "1.0.0"}); err != nil {
			return err
		}
		hooks := map[string]any{}
		for event, state := range map[string]string{"sessionStart": "waiting", "beforeSubmitPrompt": "working", "postToolUse": "working", "postToolUseFailure": "working", "stop": "waiting"} {
			command := agentStatusCommand("cursor", state, event)
			if event == "beforeSubmitPrompt" {
				command += `; printf '{"continue":true}'`
			}
			hooks[event] = []any{map[string]string{"command": command}}
		}
		return writeAgentJSON(filepath.Join(dir, "hooks", "hooks.json"), map[string]any{"hooks": hooks})
	},

	Model: &agentModelSpec{
		Flag:    "--model",
		Command: []string{"cursor-agent", "models"},
		Parse:   parseCursorModels,
	},

	Account: &agentAccountSpec{
		Env:     "CURSOR_CONFIG_DIR",
		Aliases: []string{"CURSOR_DATA_DIR"},
		Login:   "cursor-agent login",
		Status:  "cursor-agent status --format json",
		ParseStatus: func(output []byte) bool {
			var result struct {
				Authenticated bool `json:"isAuthenticated"`
			}
			start := strings.IndexByte(string(output), '{')
			return start >= 0 && json.Unmarshal(output[start:], &result) == nil && result.Authenticated
		},
		SkillDir:  ".cursor",
		CopySkill: true,
	},
}

func parseCursorModels(output []byte) ([]AgentModel, error) {
	listed := false
	return scanModels(output, func(c *modelCatalog, line string, fields []string) {
		line = ansiRE.ReplaceAllString(line, "")
		if strings.TrimSpace(line) == "Available models" {
			listed = true
			return
		}
		if !listed {
			return
		}
		id, name, ok := strings.Cut(line, " - ")
		if !ok {
			return
		}
		name = strings.TrimSuffix(strings.TrimSuffix(name, " (current)"), " (default)")
		c.add(strings.TrimSpace(id), strings.TrimSpace(name), []string{})
	})
}
