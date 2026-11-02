package core

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

const ampPluginMarker = "// Orchestra session status plugin.\n"

var ampAgent = &agentSpec{
	ID: "amp", Name: "Amp", Executable: "amp", SkillKey: "amp",
	Command: func(l launch) string { return l.command("amp", "threads continue", "") },
	Prepare: func(_ *Store, sess *Session, _ *TerminalSession, env map[string]string) error {
		if sess.HasLaunched {
			env["ORCHESTRA_ROOT_AGENT_ID"] = sess.AgentSessionID
		}
		folder := env["XDG_CONFIG_HOME"]
		if folder == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			folder = filepath.Join(home, ".config")
		}
		dir := filepath.Join(folder, "amp", "plugins")
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		path := filepath.Join(dir, "orchestra-session-status.ts")
		data, err := os.ReadFile(path)
		if err == nil && !bytes.HasPrefix(data, []byte(ampPluginMarker)) {
			return fmt.Errorf("%s already exists and is not managed by Orchestra", path)
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		return atomicWrite(path, resources["AmpStatusPlugin.js"], 0600)
	},
	NoInitialPrompt: true,
	RecoverID:       regexp.MustCompile(`https://ampcode\.com/threads/(T-[a-zA-Z0-9-]+)`),
}
