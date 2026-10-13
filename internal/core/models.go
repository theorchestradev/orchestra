package core

import (
	"context"
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
)

// validateLaunch rejects a model or effort the agent cannot receive at launch.
func validateLaunch(agent, model, effort string) error {
	if model == "" && effort == "" {
		return nil
	}
	spec, ok := agentSpecs[agent]
	if !ok || spec.ModelFlag == "" {
		return errors.New("This agent does not support choosing a model.")
	}
	if model != "" && !modelPattern.MatchString(model) {
		return errors.New("Invalid model.")
	}
	if effort == "" {
		return nil
	}
	if spec.EffortFlag == nil {
		return errors.New("This agent does not support choosing an effort.")
	}
	if !effortPattern.MatchString(effort) || spec.Efforts != nil && !slices.Contains(spec.Efforts, effort) {
		return errors.New("Invalid effort.")
	}
	return nil
}

// launchFlags passes the session's model and effort through the agent's own CLI
// flags, leaving its configuration files untouched.
func launchFlags(sess *Session) string {
	spec, ok := agentSpecs[sess.Agent]
	if !ok {
		return ""
	}
	flags := ""
	if sess.Model != "" && spec.ModelFlag != "" {
		flags += " " + spec.ModelFlag + " " + shellQuote(sess.Model)
	}
	if sess.Effort != "" && spec.EffortFlag != nil {
		flags += spec.EffortFlag(sess.Effort)
	}
	return flags
}

// AgentModels lists the models an agent's installed CLI offers. These are
// read-only catalog queries through the login shell sessions use, never agent turns.
func AgentModels(ctx context.Context, agent string) ([]AgentModel, error) {
	spec, ok := agentSpecs[agent]
	if !ok || spec.ModelFlag == "" {
		return []AgentModel{}, nil
	}
	if spec.ModelCommand == nil {
		return slices.Clone(spec.Models), nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// A neutral working directory prevents project configuration from loading.
	directory, err := os.MkdirTemp("", "orchestra-models-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(directory)
	cmd := exec.CommandContext(ctx, loginShell(), "-l", "-i", "-c", "exec "+strings.Join(spec.ModelCommand, " ")+" </dev/null")
	cmd.Dir = directory
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	output, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, errors.New("Listing models timed out.")
	}
	var models []AgentModel
	if err == nil {
		models, err = spec.ParseModels(output)
	}
	if err != nil || len(models) == 0 {
		return nil, errors.New("Could not list models. Check that " + spec.Name + " is installed and signed in.")
	}
	return models, nil
}
