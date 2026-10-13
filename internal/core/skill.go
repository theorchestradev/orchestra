package core

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"
)

const (
	// skillsCLI pins the community skills CLI. A bare npx skills can reuse a stale
	// cached copy; update the pin deliberately after checking each agent SkillKey against it.
	skillsCLI = "skills@1.7.2"
	// skillSource is the repository holding skills/orchestra/SKILL.md. Pin it to the
	// build's release tag once releases exist; the stub does not depend on the app version.
	skillSource = "https://github.com/theorchestradev/orchestra"
)

// SkillStatus lists the agents the skill can be installed for and whether npx is available.
type SkillStatus struct {
	Agents   []SkillAgent `json:"agents"`
	Node     bool         `json:"node"`
	Detected bool         `json:"detected"`
}
type SkillAgent struct {
	ID        string `json:"id"`
	Installed bool   `json:"installed"`
}

// SkillInstall is the result of running the install command.
type SkillInstall struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exitCode"`
}

// skillArgs builds the install command. It always passes explicit --agent keys:
// --yes without --agent would install into every agent the CLI knows.
func skillArgs(agents []string) ([]string, error) {
	args := []string{"npx", "--yes", skillsCLI, "add", skillSource, "--skill", "orchestra", "--global", "--yes"}
	seen := map[string]bool{}
	for _, agent := range agents {
		if spec, ok := agentSpecs[agent]; ok && spec.SkillKey != "" && !seen[spec.SkillKey] {
			key := spec.SkillKey
			seen[key] = true
			args = append(args, "--agent", key)
		}
	}
	if len(seen) == 0 {
		return nil, errors.New("Choose at least one supported agent.")
	}
	return args, nil
}

// SkillCommand is the exact command Install runs, for copying into a terminal.
func SkillCommand(agents []string) (string, error) {
	args, err := skillArgs(agents)
	if err != nil {
		return "", err
	}
	return strings.Join(args, " "), nil
}

// skillShell runs a script through the login shell sessions use, without a TTY, in a
// neutral directory, and returns its combined output and exit code. Tests replace it.
var skillShell = func(ctx context.Context, script string) ([]byte, int, error) {
	directory, err := os.MkdirTemp("", "orchestra-skill-")
	if err != nil {
		return nil, -1, err
	}
	defer os.RemoveAll(directory)
	cmd := exec.CommandContext(ctx, loginShell(), "-l", "-i", "-c", script+" </dev/null")
	cmd.Dir = directory
	env := environmentMap()
	env["NO_COLOR"] = "1"
	env["DO_NOT_TRACK"] = "1"
	cmd.Env = environmentList(env)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return output, -1, ctx.Err()
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return output, exit.ExitCode(), nil
	}
	if err != nil {
		return output, -1, err
	}
	return output, 0, nil
}

// AgentSkillStatus reports which agents and whether npx are on the login-shell PATH.
func AgentSkillStatus(ctx context.Context) SkillStatus {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	status := SkillStatus{Agents: []SkillAgent{}}
	names := []string{"npx"}
	for _, agent := range skillAgentOrder() {
		names = append(names, agentSpecs[agent].Executable)
	}
	// Login shells may print before the list, so only marked lines count.
	output, code, err := skillShell(ctx, `for c in `+strings.Join(names, " ")+`; do command -v "$c" >/dev/null 2>&1 && printf 'orchestra-found:%s\n' "$c"; done; exit 0`)
	found := map[string]bool{}
	if err == nil && code == 0 {
		status.Detected = true
		for _, line := range strings.Split(string(output), "\n") {
			if name, ok := strings.CutPrefix(strings.TrimSpace(line), "orchestra-found:"); ok {
				found[name] = true
			}
		}
	}
	status.Node = found["npx"]
	for _, agent := range skillAgentOrder() {
		status.Agents = append(status.Agents, SkillAgent{ID: agent, Installed: found[agentSpecs[agent].Executable]})
	}
	return status
}

// skillAgentOrder lists the agents with a skills CLI key, in the app's agent order.
// Agents without a confirmed key are left out and never installed to.
func skillAgentOrder() []string {
	order := []string{}
	for _, spec := range agentOrder {
		if spec.SkillKey != "" {
			order = append(order, spec.ID)
		}
	}
	return order
}

const missingNode = "orchestra-missing-npx"

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07]*\x07`)

// InstallAgentSkill runs the pinned skills CLI for the chosen agents. It runs only when
// the user asks, never edits agent settings itself and never writes to a terminal.
func InstallAgentSkill(ctx context.Context, agents []string) (SkillInstall, error) {
	command, err := SkillCommand(agents)
	if err != nil {
		return SkillInstall{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	output, code, err := skillShell(ctx, `command -v npx >/dev/null 2>&1 || { echo `+missingNode+`; exit 127; }; exec `+command)
	text := strings.TrimSpace(ansiPattern.ReplaceAllString(strings.ReplaceAll(string(output), "\r\n", "\n"), ""))
	if len(text) > 64*1024 {
		text = "…" + text[len(text)-64*1024:]
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return SkillInstall{Output: text, ExitCode: -1}, errors.New("Installing the skill timed out.")
	case err != nil:
		return SkillInstall{Output: text, ExitCode: -1}, err
	case code == 127 && strings.Contains(text, missingNode):
		return SkillInstall{ExitCode: code}, errors.New("Installing the skill requires Node.js (npx).")
	}
	return SkillInstall{Output: text, ExitCode: code}, nil
}
