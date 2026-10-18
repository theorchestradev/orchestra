package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
)

var codexAgent = &agentSpec{
	ID: "codex", Name: "Codex", Executable: "codex", SkillKey: "codex",
	Command: func(l launch) string {
		command := "codex"
		if l.Resume != "" {
			command += " resume " + shellQuote(l.Resume)
		}
		command += " --approve-for-me"
		for _, event := range append(hookEvents, "Interrupt") {
			hook := fmt.Sprintf("[{hooks=[{type=\"command\",command=%s}]}]", tomlString(statusCommand(hookState(event))))
			command += " -c " + shellQuote("hooks."+event+"="+hook)
		}
		command += l.Flags
		if l.Prompt != "" {
			command += " " + l.Prompt
		}
		return command
	},
	RecoverID:    regexp.MustCompile(`Session ID:\s*([0-9a-fA-F-]{36})`),
	StatusSetup:  true,
	ModelFlag:    "-m",
	ModelCommand: []string{"codex", "debug", "models"},
	ParseModels:  parseCodexModels,
	// Codex lists efforts per model, so its values are only checked for shape.
	EffortFlag: func(effort string) string {
		return " -c " + shellQuote("model_reasoning_effort="+tomlString(effort))
	},

	// The skills CLI puts Codex skills in ~/.agents/skills, which every profile shares.
	ConfigEnv:          "CODEX_HOME",
	LoginCommand:       "codex login",
	LoginStatusCommand: "codex login status",
}

func parseCodexModels(output []byte) ([]AgentModel, error) {
	// Login shells may print before the catalog.
	start := max(bytes.Index(output, []byte(`{"models"`)), 0)
	var catalog struct {
		Models []struct {
			Slug        string                    `json:"slug"`
			DisplayName string                    `json:"display_name"`
			Visibility  string                    `json:"visibility"`
			Levels      []struct{ Effort string } `json:"supported_reasoning_levels"`
		} `json:"models"`
	}
	if err := json.NewDecoder(bytes.NewReader(output[start:])).Decode(&catalog); err != nil {
		return nil, err
	}
	c := modelCatalog{}
	for _, m := range catalog.Models {
		if m.Visibility != "list" {
			continue
		}
		efforts := []string{}
		for _, level := range m.Levels {
			if effortPattern.MatchString(level.Effort) {
				efforts = append(efforts, level.Effort)
			}
		}
		name := m.DisplayName
		if name == "" {
			name = m.Slug
		}
		c.add(m.Slug, name, efforts)
	}
	return c, nil
}
