package core

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"
)

// AgentModel is a model a session can start with, and the efforts it accepts.
type AgentModel struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Efforts []string `json:"efforts"`
}

var (
	modelPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/~@+\[\]-]{0,199}$`)
	effortPattern = regexp.MustCompile(`^[a-z]{1,16}$`)
	// Agents whose CLI accepts a model at launch, and the command that lists them.
	modelAgents   = map[string][]string{"claude": nil, "codex": {"codex", "debug", "models"}, "pi": {"pi", "--list-models"}, "opencode": {"opencode", "models"}, "grok": {"grok", "models"}, "antigravity": {"agy", "models"}}
	claudeEfforts = []string{"low", "medium", "high", "xhigh", "max"}
	piEfforts     = []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}
)

// validateLaunch rejects a model or effort the agent cannot receive at launch.
// Codex lists efforts per model, so its values are only checked for shape.
func validateLaunch(agent, model, effort string) error {
	if model == "" && effort == "" {
		return nil
	}
	if _, ok := modelAgents[agent]; !ok {
		return errors.New("This agent does not support choosing a model.")
	}
	if model != "" && !modelPattern.MatchString(model) {
		return errors.New("Invalid model.")
	}
	if effort == "" {
		return nil
	}
	valid := effortPattern.MatchString(effort)
	switch agent {
	case "claude":
		valid = slices.Contains(claudeEfforts, effort)
	case "pi":
		valid = slices.Contains(piEfforts, effort)
	case "codex":
	default:
		return errors.New("This agent does not support choosing an effort.")
	}
	if !valid {
		return errors.New("Invalid effort.")
	}
	return nil
}

// launchFlags passes the session's model and effort through the agent's own CLI
// flags, leaving its configuration files untouched.
func launchFlags(sess *Session) string {
	flags := ""
	if sess.Model != "" {
		flag := " --model "
		if sess.Agent == "codex" {
			flag = " -m "
		}
		flags += flag + shellQuote(sess.Model)
	}
	if sess.Effort != "" {
		switch sess.Agent {
		case "claude":
			flags += " --effort " + shellQuote(sess.Effort)
		case "codex":
			flags += " -c " + shellQuote("model_reasoning_effort="+tomlString(sess.Effort))
		case "pi":
			flags += " --thinking " + shellQuote(sess.Effort)
		}
	}
	return flags
}

// AgentModels lists the models an agent's installed CLI offers. These are
// read-only catalog queries through the login shell sessions use, never agent turns.
func AgentModels(ctx context.Context, agent string) ([]AgentModel, error) {
	command, ok := modelAgents[agent]
	if !ok {
		return []AgentModel{}, nil
	}
	if agent == "claude" {
		return []AgentModel{{"fable", "Fable", claudeEfforts}, {"opus", "Opus", claudeEfforts}, {"sonnet", "Sonnet", claudeEfforts}, {"haiku", "Haiku", []string{}}}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// A neutral working directory prevents project configuration from loading.
	directory, err := os.MkdirTemp("", "orchestra-models-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(directory)
	cmd := exec.CommandContext(ctx, loginShell(), "-l", "-i", "-c", "exec "+strings.Join(command, " ")+" </dev/null")
	cmd.Dir = directory
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	output, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, errors.New("Listing models timed out.")
	}
	var models []AgentModel
	if err == nil {
		models, err = parseModels(agent, output)
	}
	if err != nil || len(models) == 0 {
		return nil, errors.New("Could not list models. Check that " + agentNames[agent] + " is installed and signed in.")
	}
	return models, nil
}

func parseModels(agent string, output []byte) ([]AgentModel, error) {
	models := []AgentModel{}
	add := func(id, name string, efforts []string) {
		if modelPattern.MatchString(id) && !slices.ContainsFunc(models, func(m AgentModel) bool { return m.ID == id }) {
			models = append(models, AgentModel{id, name, efforts})
		}
	}
	if agent == "codex" {
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
			add(m.Slug, name, efforts)
		}
		return models, nil
	}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	listed := agent != "grok"
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		switch agent {
		case "pi":
			// provider  model  context  max-out  thinking  images
			if len(fields) >= 6 && fields[0] != "provider" {
				efforts := []string{}
				if fields[4] == "yes" {
					efforts = piEfforts
				}
				add(fields[0]+"/"+fields[1], fields[0]+"/"+fields[1], efforts)
			}
		case "opencode":
			if len(fields) == 1 && strings.Contains(fields[0], "/") {
				add(fields[0], fields[0], []string{})
			}
		case "grok":
			if strings.HasPrefix(line, "Available models:") {
				listed = true
			} else if listed && strings.HasPrefix(line, "  ") && len(fields) > 0 {
				id := fields[0]
				if id == "*" && len(fields) > 1 {
					id = fields[1]
				}
				add(id, id, []string{})
			}
		case "antigravity":
			if id, name, ok := strings.Cut(line, "\t"); ok {
				add(strings.TrimSpace(id), strings.TrimSpace(name), []string{})
			}
		}
	}
	return models, scanner.Err()
}
