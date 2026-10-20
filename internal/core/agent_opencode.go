package core

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// OpenCode reports status and its conversation ID through OpenCodeSessionPlugin.js.
var opencodeAgent = &agentSpec{
	ID: "opencode", Name: "OpenCode", Executable: "opencode", SkillKey: "opencode",
	Command:      func(l launch) string { return l.command("opencode", "--session", "--prompt") },
	Prepare:      sessionPlugin("OPENCODE_CONFIG_CONTENT"),
	ModelFlag:    "--model",
	ModelCommand: []string{"opencode", "models"},
	ParseModels:  parseProviderModels,
}

// sessionPlugin adds OpenCodeSessionPlugin.js to the inline config the CLI reads from
// variable, keeping any config the user already passes there.
func sessionPlugin(variable string) func(*Store, *Session, *TerminalSession, map[string]string) error {
	return func(s *Store, sess *Session, _ *TerminalSession, env map[string]string) error {
		config := map[string]any{}
		if env[variable] != "" {
			if err := json.Unmarshal([]byte(env[variable]), &config); err != nil {
				return err
			}
		}
		plugins, _ := config["plugin"].([]any)
		config["plugin"] = append(plugins, "file://"+filepath.Join(s.runtimeDir, "OpenCodeSessionPlugin.js"))
		data, _ := json.Marshal(config)
		env[variable] = string(data)
		env["ORCHESTRA_RESUME_ID"] = sess.AgentSessionID
		return nil
	}
}

// parseProviderModels reads the provider/model lines OpenCode and Kilo list.
func parseProviderModels(output []byte) ([]AgentModel, error) {
	return scanModels(output, func(c *modelCatalog, _ string, fields []string) {
		if len(fields) == 1 && strings.Contains(fields[0], "/") {
			c.add(fields[0], fields[0], []string{})
		}
	})
}
