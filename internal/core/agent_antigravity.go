package core

import (
	"regexp"
	"strings"
)

var antigravityAgent = &agentSpec{
	ID: "antigravity", Name: "Antigravity CLI", Executable: "agy", SkillKey: "antigravity-cli",
	Command: func(l launch) string {
		return l.command("agy --mode accept-edits", "--conversation", "--prompt-interactive")
	},
	RecoverID:    regexp.MustCompile(`Resume:\s*agy\s+--conversation(?:=|\s+)([0-9a-fA-F-]{36})`),
	ModelFlag:    "--model",
	ModelCommand: []string{"agy", "models"},
	ParseModels: func(output []byte) ([]AgentModel, error) {
		return scanModels(output, func(c *modelCatalog, line string, _ []string) {
			if id, name, ok := strings.Cut(line, "\t"); ok {
				c.add(strings.TrimSpace(id), strings.TrimSpace(name), []string{})
			}
		})
	},
}
