package core

import "path/filepath"

var piEfforts = []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}

// Pi reports status and forwards Inbox replies through PiStatusExtension.js.
var piAgent = &agentSpec{
	ID: "pi", Name: "Pi", Executable: "pi", SkillKey: "pi",
	Command: func(l launch) string {
		id := l.Resume
		if id == "" {
			id = l.Session.ID
		}
		command := "pi --session-id " + shellQuote(id) + " --extension " + shellQuote(filepath.Join(l.Dir, "PiStatusExtension.js")) + l.Flags
		if l.Prompt != "" {
			command += " -- " + l.Prompt
		}
		return command
	},
	PresetID:        true,
	ForwardsReplies: true,
	ModelFlag:       "--model",
	ModelCommand:    []string{"pi", "--list-models"},
	ParseModels: func(output []byte) ([]AgentModel, error) {
		return scanModels(output, func(c *modelCatalog, _ string, fields []string) {
			// provider  model  context  max-out  thinking  images
			if len(fields) >= 6 && fields[0] != "provider" {
				efforts := []string{}
				if fields[4] == "yes" {
					efforts = piEfforts
				}
				c.add(fields[0]+"/"+fields[1], fields[0]+"/"+fields[1], efforts)
			}
		})
	},
	EffortFlag: quotedFlag("--thinking"),
	Efforts:    piEfforts,
}
