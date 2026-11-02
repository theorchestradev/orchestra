package core

import "path/filepath"

var copilotAgent = &agentSpec{
	ID: "copilot", Name: "GitHub Copilot CLI", Executable: "copilot", SkillKey: "github-copilot",
	Command: func(l launch) string {
		plugin := " --plugin-dir " + shellQuote(filepath.Join(l.Dir, "copilot-status"))
		if l.Resume != "" {
			return l.command("copilot"+plugin, "--session-id", "--interactive")
		}
		return l.command(`copilot`+plugin+` --session-id "$ORCHESTRA_CONVERSATION_ID"`, "", "--interactive")
	},
	Prepare: func(s *Store, sess *Session, t *TerminalSession, env map[string]string) error {
		if err := prepareConversationID(s, sess, t, env); err != nil {
			return err
		}
		dir := filepath.Join(s.runtimeDir, "copilot-status")
		if err := writeAgentJSON(filepath.Join(dir, ".plugin", "plugin.json"), map[string]string{"name": "orchestra-session-status", "version": "1.0.0"}); err != nil {
			return err
		}
		hooks := map[string]any{}
		for event, state := range map[string]string{"sessionStart": "waiting", "userPromptSubmitted": "working", "preToolUse": "working", "postToolUse": "working", "postToolUseFailure": "working", "notification": "waiting", "agentStop": "waiting", "errorOccurred": "waiting"} {
			hooks[event] = []any{map[string]string{"type": "command", "bash": agentStatusCommand("copilot", state, event)}}
		}
		return writeAgentJSON(filepath.Join(dir, "hooks.json"), map[string]any{"version": 1, "hooks": hooks})
	},

	Model: &agentModelSpec{
		Flag:       "--model",
		EffortFlag: quotedFlag("--reasoning-effort"),
		Efforts:    []string{"low", "medium", "high", "xhigh", "max"},
	},
}
