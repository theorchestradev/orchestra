package core

import (
	"path/filepath"
	"regexp"
)

var qwenAgent = &agentSpec{
	ID: "qwen", Name: "Qwen Code", Executable: "qwen", SkillKey: "qwen-code",
	Command: func(l launch) string {
		if l.Resume != "" {
			return l.command("qwen --approval-mode auto", "--resume", "--prompt-interactive")
		}
		return l.command(`qwen --approval-mode auto --session-id "$ORCHESTRA_CONVERSATION_ID"`, "", "--prompt-interactive")
	},
	Prepare: func(s *Store, sess *Session, t *TerminalSession, env map[string]string) error {
		if err := prepareConversationID(s, sess, t, env); err != nil {
			return err
		}
		folder, err := agentConfigFolder(env, "QWEN_HOME", ".qwen")
		if err != nil {
			return err
		}
		dir := filepath.Join(folder, "extensions", "orchestra-session-status")
		if err = managedAgentDirectory(dir); err != nil {
			return err
		}
		return writeAgentJSON(filepath.Join(dir, "qwen-extension.json"), map[string]any{"name": "orchestra-session-status", "version": "1.0.0", "hooks": groupedStatusHooks("qwen", map[string]string{"SessionStart": "waiting", "UserPromptSubmit": "working", "PreToolUse": "working", "PostToolUse": "working", "PostToolUseFailure": "working", "PermissionRequest": "waiting", "PermissionDenied": "working", "Notification": "waiting", "Stop": "waiting", "StopFailure": "waiting"})})
	},
	ResumeFailed: regexp.MustCompile(`No saved session found with ID`),

	Model: &agentModelSpec{
		Flag: "--model",
	},
}
