package core

import (
	"context"
	"errors"
	"fmt"
	"maps"
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
	modelPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/~@+\[\]=,-]{0,199}$`)
	effortPattern = regexp.MustCompile(`^[a-z]{1,16}$`)
)

// validateLaunch rejects a model or effort the agent cannot receive at launch.
func validateLaunch(agent, model, effort string) error {
	if model == "" && effort == "" {
		return nil
	}
	spec, ok := agentSpecs[agent]
	if !ok || spec.Model == nil {
		return errors.New("This agent does not support choosing a model.")
	}
	if model != "" && !modelPattern.MatchString(model) {
		return errors.New("Invalid model.")
	}
	if effort == "" {
		return nil
	}
	if spec.Model.EffortFlag == nil && spec.Model.Efforts == nil {
		return errors.New("This agent does not support choosing an effort.")
	}
	if !effortPattern.MatchString(effort) || spec.Model.Efforts != nil && !slices.Contains(spec.Model.Efforts, effort) {
		return errors.New("Invalid effort.")
	}
	return nil
}

// validateModelEffort rejects an effort the agent's fixed catalog does not list for the model.
// validateLaunch also checks saved sessions, so this stricter rule stays out of it.
func validateModelEffort(agent, model, effort string) error {
	spec := agentSpecs[agent]
	if spec == nil || spec.Model == nil || effort == "" {
		return nil
	}
	index := slices.IndexFunc(spec.Model.Catalog, func(m AgentModel) bool { return m.ID == model })
	if index >= 0 && !slices.Contains(spec.Model.Catalog[index].Efforts, effort) {
		return fmt.Errorf("%s does not support effort %s.", spec.Model.Catalog[index].Name, effort)
	}
	return nil
}

// launchFlags passes the session's model and effort through the agent's own CLI
// flags, leaving its configuration files untouched.
func launchFlags(sess *Session) string {
	spec, ok := agentSpecs[sess.Agent]
	if !ok || spec.Model == nil {
		return ""
	}
	flags := ""
	if sess.Model != "" && spec.Model.Flag != "" {
		flags += " " + spec.Model.Flag + " " + shellQuote(sess.Model)
	}
	if sess.Effort != "" && spec.Model.EffortFlag != nil {
		flags += spec.Model.EffortFlag(sess.Effort)
	}
	return flags
}

// AgentModels lists the models an agent's installed CLI offers. These are
// read-only catalog queries through the login shell sessions use, never agent turns.
func AgentModels(ctx context.Context, agent string) ([]AgentModel, error) {
	return agentModelsForProfile(ctx, agent, nil)
}
func agentModelsForProfile(ctx context.Context, agent string, profile *Profile) ([]AgentModel, error) {
	spec, ok := agentSpecs[agent]
	if !ok || spec.Model == nil {
		return []AgentModel{}, nil
	}
	if profile != nil && spec.Account != nil && profile.Folders[agent] == "" {
		return nil, errors.New("This profile has no account folder for " + spec.Name + ".")
	}
	if spec.Model.Command == nil {
		return slices.Clone(spec.Model.Catalog), nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// A neutral working directory prevents project configuration from loading.
	directory, err := os.MkdirTemp("", "orchestra-models-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(directory)
	script := ""
	folders := profileFolders(profile)
	for _, variable := range slices.Sorted(maps.Keys(folders)) {
		script += "export " + variable + "=" + shellQuote(folders[variable]) + "; "
	}
	cmd := exec.CommandContext(ctx, loginShell(), "-l", "-i", "-c", script+"exec "+strings.Join(quotedModelCommand(spec.Model.Command), " ")+" </dev/null")
	cmd.Dir = directory
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	output, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, errors.New("Listing models timed out.")
	}
	var models []AgentModel
	if err == nil {
		models, err = spec.Model.Parse(output)
	}
	if err != nil || len(models) == 0 {
		return nil, errors.New("Could not list models. Check that " + spec.Name + " is installed and signed in.")
	}
	return models, nil
}

// AgentLaunchOptions also describes CLIs that accept a model ID but expose no safe
// catalog command. The UI can offer a manual ID without reading credential files.
type AgentLaunchOptions struct {
	Models      []AgentModel `json:"models"`
	CustomModel bool         `json:"customModel"`
	Efforts     []string     `json:"efforts"`
}

func LaunchOptions(ctx context.Context, agent string, profile *Profile) (AgentLaunchOptions, error) {
	models, err := agentModelsForProfile(ctx, agent, profile)
	options := AgentLaunchOptions{Models: models, Efforts: []string{}}
	if spec := agentSpecs[agent]; spec != nil && spec.Model != nil {
		options.CustomModel = true
		if spec.Model.Efforts != nil {
			options.Efforts = slices.Clone(spec.Model.Efforts)
		}
	}
	return options, err
}
func quotedModelCommand(args []string) []string {
	out := make([]string, len(args))
	for i, arg := range args {
		out[i] = shellQuote(arg)
	}
	return out
}
