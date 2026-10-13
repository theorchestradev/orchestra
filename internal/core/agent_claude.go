package core

import "encoding/json"

var claudeEfforts = []string{"low", "medium", "high", "xhigh", "max"}

var claudeAgent = &agentSpec{
	ID: "claude", Name: "Claude Code", Executable: "claude", SkillKey: "claude-code",
	Command: func(l launch) string {
		hooks := map[string]any{}
		for _, event := range append(hookEvents, "PermissionRequest") {
			group := map[string]any{"hooks": []any{map[string]string{"type": "command", "command": statusCommand(hookState(event))}}}
			if event == "PreToolUse" || event == "PermissionRequest" {
				group["matcher"] = "*"
			}
			hooks[event] = []any{group}
		}
		data, _ := json.Marshal(map[string]any{"hooks": hooks})
		command := "claude"
		if l.Resume != "" {
			command += " --resume " + shellQuote(l.Resume)
		} else {
			command += " --session-id " + shellQuote(l.Session.ID)
		}
		command += " --permission-mode auto --settings " + shellQuote(string(data)) + l.Flags
		if l.Prompt != "" {
			command += " " + l.Prompt
		}
		return command
	},
	PresetID:   true,
	ModelFlag:  "--model",
	Models:     []AgentModel{{"fable", "Fable", claudeEfforts}, {"opus", "Opus", claudeEfforts}, {"sonnet", "Sonnet", claudeEfforts}, {"haiku", "Haiku", []string{}}},
	EffortFlag: quotedFlag("--effort"),
	Efforts:    claudeEfforts,
}
