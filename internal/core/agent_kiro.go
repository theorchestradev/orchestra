package core

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var kiroAgent = &agentSpec{
	ID: "kiro", Name: "Kiro CLI", Executable: "kiro-cli", SkillKey: "kiro-cli",
	Command: func(l launch) string { return l.command("kiro-cli chat --v3", "--resume-id", "--") },
	Prepare: func(_ *Store, _ *Session, _ *TerminalSession, env map[string]string) error {
		folder, err := agentConfigFolder(env, "KIRO_HOME", ".kiro")
		if err != nil {
			return err
		}
		dir := filepath.Join(folder, "hooks", "orchestra-session-status")
		path := filepath.Join(folder, "hooks", "orchestra-session-status.json")
		if _, err := os.Lstat(path); err == nil {
			if _, err = os.Stat(filepath.Join(dir, ".orchestra-managed")); err != nil {
				return fmt.Errorf("%s already exists and is not managed by Orchestra", path)
			}
		}
		if err = managedAgentDirectory(dir); err != nil {
			return err
		}
		hooks := []any{}
		for event, state := range map[string]string{"SessionStart": "waiting", "UserPromptSubmit": "working", "PreToolUse": "working", "PostToolUse": "working", "Stop": "waiting"} {
			hooks = append(hooks, map[string]any{"name": "Orchestra " + event, "trigger": event, "action": map[string]string{"type": "command", "command": agentStatusCommand("kiro", state, event)}})
		}
		// Kiro discovers files directly under hooks/, not nested directories.
		return writeAgentJSON(filepath.Join(folder, "hooks", "orchestra-session-status.json"), map[string]any{"version": "v1", "hooks": hooks})
	},
	RecoverID: regexp.MustCompile(`kiro-cli chat --resume-id[= ]+([a-zA-Z0-9][a-zA-Z0-9-]*)`),

	Model: &agentModelSpec{
		Flag: "--model",
	},
}
