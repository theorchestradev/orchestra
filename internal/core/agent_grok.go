package core

import "strings"

var grokAgent = &agentSpec{
	ID: "grok", Name: "Grok Build", Executable: "grok", SkillKey: "grok",
	Command: func(l launch) string {
		command := "grok --session-id " + shellQuote(l.conversationID()) + " --permission-mode auto" + l.Flags
		if l.Resume != "" {
			command = "grok --resume " + shellQuote(l.Resume) + " --permission-mode auto" + l.Flags
		}
		if l.Prompt != "" {
			command += " -- " + l.Prompt
		}
		return command
	},
	PresetID: true,

	Model: &agentModelSpec{
		Flag:    "--model",
		Command: []string{"grok", "models"},
		Parse: func(output []byte) ([]AgentModel, error) {
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
	},
}
