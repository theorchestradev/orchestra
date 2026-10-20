package core

import (
	"bytes"
	"encoding/json"
	"path/filepath"
)

// omp (oh-my-pi) reports status and its conversation ID through OmpStatusExtension.js.
// It approves every tool by default, so sessions only auto-approve reads and edits.
var ompAgent = &agentSpec{
	ID: "omp", Name: "omp", Executable: "omp",
	Command: func(l launch) string {
		base := "omp --approval-mode write --extension " + shellQuote(filepath.Join(l.Dir, "OmpStatusExtension.js"))
		return l.command(base, "--resume", "--")
	},
	ModelFlag:    "--model",
	ModelCommand: []string{"omp", "models", "--json"},
	ParseModels:  parseOmpModels,
	// omp lists thinking levels per model, so its values are only checked for shape.
	EffortFlag: quotedFlag("--thinking"),
}

func parseOmpModels(output []byte) ([]AgentModel, error) {
	// Login shells may print before the catalog.
	start := max(bytes.Index(output, []byte(`{"models"`)), 0)
	var catalog struct {
		Models []struct {
			Kind, Selector, Name string
			Thinking             []string
		}
	}
	if err := json.NewDecoder(bytes.NewReader(output[start:])).Decode(&catalog); err != nil {
		return nil, err
	}
	c := modelCatalog{}
	for _, m := range catalog.Models {
		if m.Kind != "chat" {
			continue
		}
		efforts := []string{}
		for _, effort := range m.Thinking {
			if effortPattern.MatchString(effort) {
				efforts = append(efforts, effort)
			}
		}
		name := m.Name
		if name == "" {
			name = m.Selector
		}
		c.add(m.Selector, name, efforts)
	}
	return c, nil
}
