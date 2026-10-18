package core

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// OpenCode reports status and its conversation ID through OpenCodeSessionPlugin.js.
var opencodeAgent = &agentSpec{
	ID: "opencode", Name: "OpenCode", Executable: "opencode", SkillKey: "opencode",
	Command: func(l launch) string { return l.command("opencode", "--session", "--prompt") },
	Prepare: func(s *Store, sess *Session, _ *TerminalSession, env map[string]string) error {
		config := map[string]any{}
		if env["OPENCODE_CONFIG_CONTENT"] != "" {
			if err := json.Unmarshal([]byte(env["OPENCODE_CONFIG_CONTENT"]), &config); err != nil {
				return err
			}
		}
		plugins, _ := config["plugin"].([]any)
		config["plugin"] = append(plugins, "file://"+filepath.Join(s.runtimeDir, "OpenCodeSessionPlugin.js"))
		data, _ := json.Marshal(config)
		env["OPENCODE_CONFIG_CONTENT"] = string(data)
		env["ORCHESTRA_RESUME_ID"] = sess.AgentSessionID
		return nil
	},
	ModelFlag:    "--model",
	ModelCommand: []string{"opencode", "models"},
	ParseModels: func(output []byte) ([]AgentModel, error) {
		return scanModels(output, func(c *modelCatalog, _ string, fields []string) {
			if len(fields) == 1 && strings.Contains(fields[0], "/") {
				c.add(fields[0], fields[0], []string{})
			}
		})
	},
}
