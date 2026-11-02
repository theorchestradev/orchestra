package core

import "path/filepath"

// Goose hooks capture its actual conversation ID, independently of its display
// name. smart_approve still asks about risky tools.
var gooseAgent = &agentSpec{
	ID: "goose", Name: "Goose", Executable: "goose", SkillKey: "goose",
	OverrideEnv: []string{"GOOSE_MODE"},
	Command: func(l launch) string {
		if l.Resume != "" {
			return l.command("goose session --resume", "--session-id", "--text")
		}
		base := "goose session --name " + shellQuote(l.Session.ID)
		if l.Prompt != "" {
			base = "goose run --interactive --name " + shellQuote(l.Session.ID)
		}
		return l.command(base, "", "--text")
	},
	Prepare: func(_ *Store, sess *Session, _ *TerminalSession, env map[string]string) error {
		env["GOOSE_MODE"] = "smart_approve"
		if sess.Model != "" {
			env["GOOSE_MODEL"] = sess.Model
		}
		folder, err := agentConfigFolder(env, "", ".agents/plugins")
		if err != nil {
			return err
		}
		dir := filepath.Join(folder, "orchestra-session-status")
		if err = managedAgentDirectory(dir); err != nil {
			return err
		}
		if err = writeAgentJSON(filepath.Join(dir, "plugin.json"), map[string]string{"name": "orchestra-session-status", "version": "1.0.0", "description": "Orchestra session activity"}); err != nil {
			return err
		}
		return writeAgentJSON(filepath.Join(dir, "hooks", "hooks.json"), map[string]any{"hooks": groupedStatusHooks("goose", map[string]string{"SessionStart": "waiting", "UserPromptSubmit": "working", "PreToolUse": "working", "PostToolUse": "working", "PostToolUseFailure": "working", "Stop": "waiting"})})
	},

	Model: &agentModelSpec{Env: "GOOSE_MODEL"},
}
