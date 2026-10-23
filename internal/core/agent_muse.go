package core

import (
	"encoding/json"
	"regexp"
)

var museEfforts = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"}

// Muse Code takes no conversation ID at launch and creates its session with the first
// prompt, whose hooks report the ID. Muse clears the environment of hook commands, so each
// hook exports the session's Orchestra files itself. Muse lists no models, so the catalog
// is fixed. The skills CLI has no Muse key; its Codex key installs into ~/.agents/skills,
// which Muse reads.
var museAgent = &agentSpec{
	ID: "muse", Name: "Muse Code", Executable: "muse", SkillKey: "codex",
	Command: func(l launch) string {
		statusFile, idFile := sessionFiles(l.Dir, l.Session.ID)
		exports := "export ORCHESTRA_STATUS_FILE=" + shellQuote(statusFile) + " ORCHESTRA_AGENT_ID_FILE=" + shellQuote(idFile) + " ORCHESTRA_CLI_DIRECTORY=" + shellQuote(l.Dir) + "; "
		hooks := map[string]any{}
		states := map[string]string{"PermissionRequest": "waiting", "PostToolUse": "working"}
		for _, event := range hookEvents {
			states[event] = hookState(event)
		}
		for event, state := range states {
			group := map[string]any{"hooks": []any{map[string]string{"type": "command", "command": exports + statusCommand(state)}}}
			if event == "PreToolUse" || event == "PostToolUse" || event == "PermissionRequest" {
				group["matcher"] = "*"
			}
			hooks[event] = []any{group}
		}
		data, _ := json.Marshal(hooks)
		command := "muse --permission-profile :auto-review -c " + shellQuote("hooks="+string(data)) + l.Flags
		if l.Resume != "" {
			return command + " resume " + shellQuote(l.Resume)
		}
		if l.Prompt != "" {
			command += " " + l.Prompt
		}
		return command
	},
	ResumeFailed: regexp.MustCompile(`retained session not found`),
	ModelFlag:    "--model",
	Models:       []AgentModel{{"muse-spark-1.3", "Muse Spark 1.3", museEfforts}, {"muse-spark-1.2", "Muse Spark 1.2", museEfforts}},
	EffortFlag:   quotedFlag("--reasoning-effort"),
	Efforts:      museEfforts,
}
