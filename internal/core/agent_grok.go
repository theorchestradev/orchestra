package core

import "strings"

var grokAgent = &agentSpec{
	ID: "grok", Name: "Grok Build", Executable: "grok", SkillKey: "grok",
	Command: func(l launch) string {
		if l.Resume != "" {
			return "grok --resume " + shellQuote(l.Resume) + " --permission-mode auto" + l.Flags
		}
		return "grok --session-id " + shellQuote(l.Session.ID) + " --permission-mode auto" + l.Flags
	},
	PresetID:     true,
	ModelFlag:    "--model",
	ModelCommand: []string{"grok", "models"},
	ParseModels: func(output []byte) ([]AgentModel, error) {
		listed := false
		return scanModels(output, func(c *modelCatalog, line string, fields []string) {
			if strings.HasPrefix(line, "Available models:") {
				listed = true
			} else if listed && strings.HasPrefix(line, "  ") && len(fields) > 0 {
				id := fields[0]
				if id == "*" && len(fields) > 1 {
					id = fields[1]
				}
				c.add(id, id, []string{})
			}
		})
	},
}
