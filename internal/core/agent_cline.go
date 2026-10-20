package core

import (
	"os"
	"path/filepath"
	"strings"
)

// clineHooks are the status each Cline hook event reports. TaskResume replaces
// TaskStart in a resumed conversation.
var clineHooks = map[string]string{
	"TaskStart": "working", "TaskResume": "working", "UserPromptSubmit": "working", "PreToolUse": "working",
	"TaskComplete": "waiting", "TaskError": "waiting", "TaskCancel": "waiting",
}

// clineHookMarker identifies the hook files Orchestra owns, so a user's own hook is never replaced.
const clineHookMarker = "# Orchestra session status for Cline."

// Cline only reads hooks from its config folder (--hooks-dir is ignored), so Prepare installs
// hook files there that do nothing outside a Cline session Orchestra started. They report status
// and the conversation ID. Cline auto-approves every tool by default, so sessions start with
// approvals on; Shift+Tab in Cline turns them off.
var clineAgent = &agentSpec{
	ID: "cline", Name: "Cline", Executable: "cline", SkillKey: "cline",
	Command: func(l launch) string {
		// --id always opens the interactive view and takes no prompt.
		if l.Resume != "" {
			return "cline --id " + shellQuote(l.Resume) + " --auto-approve false" + l.Flags
		}
		command := "cline --tui --auto-approve false" + l.Flags
		if l.Prompt != "" {
			command += " " + l.Prompt
		}
		return command
	},
	Prepare: func(_ *Store, _ *Session, _ *TerminalSession, env map[string]string) error {
		folder := env["CLINE_DIR"]
		if folder == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			folder = filepath.Join(home, ".cline")
		}
		env["ORCHESTRA_CLINE"] = "1"
		return installClineHooks(filepath.Join(folder, "hooks"))
	},
}

// installClineHooks writes Orchestra's hook files, leaving any other file of the same name alone.
func installClineHooks(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	for event, state := range clineHooks {
		path := filepath.Join(dir, event+".sh")
		script := "#!/bin/sh\n" + clineHookMarker + "\n" +
			"if [ -z \"$ORCHESTRA_CLINE\" ]; then /bin/cat >/dev/null; exit 0; fi\n" + statusCommand(state) + "\n"
		existing, err := os.ReadFile(path)
		if err == nil && (string(existing) == script || !strings.Contains(string(existing), clineHookMarker)) {
			continue
		}
		if err = atomicWrite(path, []byte(script), 0755); err != nil {
			return err
		}
	}
	return nil
}
