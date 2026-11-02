package core

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// --prompt runs headlessly with automatic permissions. Keep the native TUI and
// require initial messages to be entered there until Kimi supports a TUI prompt.
var kimiAgent = &agentSpec{
	ID: "kimi", Name: "Kimi Code CLI", Executable: "kimi", SkillKey: "kimi-code-cli",
	Command:         func(l launch) string { return l.command("kimi", "--session", "") },
	NoInitialPrompt: true,
	Prepare: func(_ *Store, _ *Session, _ *TerminalSession, env map[string]string) error {
		folder, err := agentConfigFolder(env, "KIMI_CODE_HOME", ".kimi-code")
		if err != nil {
			return err
		}
		return installKimiStatusHooks(filepath.Join(folder, "config.toml"))
	},

	Model: &agentModelSpec{
		Flag: "--model",
	},
}

func installKimiStatusHooks(path string) error {
	const begin = "# Orchestra session status hooks BEGIN"
	const end = "# Orchestra session status hooks END"
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	original := string(data)
	base := original
	if start := strings.Index(base, begin); start >= 0 {
		finish := strings.Index(base[start:], end)
		if finish < 0 {
			return fmt.Errorf("Orchestra hook marker is incomplete in %s", path)
		}
		base = base[:start] + base[start+finish+len(end):]
	}
	// Inline arrays cannot be extended with TOML array-of-table declarations.
	if regexp.MustCompile(`(?m)^\s*hooks\s*=`).MatchString(base) {
		return fmt.Errorf("Kimi's inline hooks cannot be extended automatically; use [[hooks]] entries in %s", path)
	}
	block := begin + "\n"
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "TurnStarted", "PreToolUse", "PostToolUse", "PostToolUseFailure", "PermissionRequest", "PermissionResult", "Stop", "StopFailure", "Interrupt"} {
		state := "working"
		switch event {
		case "SessionStart", "PermissionRequest", "Stop", "StopFailure", "Interrupt":
			state = "waiting"
		}
		block += "\n[[hooks]]\nevent = " + tomlString(event) + "\ncommand = " + tomlString(agentStatusCommand("kimi", state, event)) + "\ntimeout = 5\n"
	}
	updated := strings.TrimRight(base, "\n") + "\n\n" + block + end + "\n"
	if updated == original {
		return nil
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return atomicWrite(path, []byte(updated), 0600)
}
