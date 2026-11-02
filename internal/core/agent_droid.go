package core

import (
	"path/filepath"
	"regexp"
)

var droidAgent = &agentSpec{
	ID: "droid", Name: "Factory Droid", Executable: "droid", SkillKey: "droid",
	Command: func(l launch) string {
		return l.command("droid --settings "+shellQuote(filepath.Join(l.Dir, l.Session.ID+"-droid-settings.json")), "--resume", "--")
	},
	Prepare: func(s *Store, sess *Session, _ *TerminalSession, _ map[string]string) error {
		settings := map[string]any{"hooks": groupedStatusHooks("droid", map[string]string{"SessionStart": "waiting", "UserPromptSubmit": "working", "PreToolUse": "working", "PostToolUse": "working", "Notification": "waiting", "Stop": "waiting"})}
		if sess.Model != "" {
			settings["model"] = sess.Model
		}
		if sess.Effort != "" {
			settings["reasoningEffort"] = sess.Effort
		}
		return writeAgentJSON(filepath.Join(s.runtimeDir, sess.ID+"-droid-settings.json"), settings)
	},
	ResumeFailed: regexp.MustCompile(`(?i)(?:session|conversation).*(?:not found|does not exist)`),
	RecoverID:    regexp.MustCompile(`droid (?:--resume[= ]+|resume[ ]+)([a-zA-Z0-9][a-zA-Z0-9-]*)`),

	Model: &agentModelSpec{
		Efforts: []string{"off", "none", "low", "medium", "high", "xhigh", "max"},
	},
}
