package core

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

const (
	// skillsCLI pins the community skills CLI. A bare npx skills can reuse a stale
	// cached copy; update the pin deliberately after checking skillAgentKeys against it.
	skillsCLI = "skills@1.7.2"
	// skillSource is the repository holding skills/orchestra/SKILL.md. Pin it to the
	// build's release tag once releases exist; the stub does not depend on the app version.
	skillSource = "https://github.com/theorchestradev/orchestra"
)

// skillAgentKeys maps Orchestra agent IDs to the agent keys of the pinned skills CLI.
// Agents without a confirmed key are left out and never installed to.
var skillAgentKeys = map[string]string{"claude": "claude-code", "codex": "codex", "pi": "pi", "opencode": "opencode", "grok": "grok", "antigravity": "antigravity-cli", "vibe": "mistral-vibe"}

// agentExecutables names the CLI each agent runs, for detecting installed agents.
var agentExecutables = map[string]string{"claude": "claude", "codex": "codex", "pi": "pi", "opencode": "opencode", "grok": "grok", "antigravity": "agy", "vibe": "vibe"}

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
		if key, ok := skillAgentKeys[agent]; ok && !seen[key] {
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

// cliShell runs a script through the login shell sessions use, without a TTY, in a
// neutral directory, and returns its combined output and exit code. Skill installs and
// sign-in checks use it; tests replace it.
var cliShell = func(ctx context.Context, script string) ([]byte, int, error) {
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
		names = append(names, agentExecutables[agent])
	}
	// Login shells may print before the list, so only marked lines count.
	output, code, err := cliShell(ctx, `for c in `+strings.Join(names, " ")+`; do command -v "$c" >/dev/null 2>&1 && printf 'orchestra-found:%s\n' "$c"; done; exit 0`)
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
		status.Agents = append(status.Agents, SkillAgent{ID: agent, Installed: found[agentExecutables[agent]]})
	}
	return status
}

// skillAgentOrder lists the supported agents in the app's agent order.
func skillAgentOrder() []string {
	order := []string{}
	for _, agent := range []string{"codex", "claude", "pi", "vibe", "opencode", "antigravity", "grok"} {
		if _, ok := skillAgentKeys[agent]; ok {
			order = append(order, agent)
		}
	}
	return order
}

const missingNode = "orchestra-missing-npx"

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07]*\x07`)

// profileSkillAgents are the agents whose global skills live in their config folder, so each
// profile needs its own install. The pinned skills CLI puts Codex skills in ~/.agents/skills,
// which every profile shares.
var profileSkillAgents = map[string]bool{"claude": true}

// InstallAgentSkill runs the pinned skills CLI for the chosen agents, then again for each
// profile's own config folders. It runs only when the user asks, never edits agent settings
// itself and never writes to a terminal.
func InstallAgentSkill(ctx context.Context, agents []string, profiles []*Profile) (SkillInstall, error) {
	command, err := SkillCommand(agents)
	if err != nil {
		return SkillInstall{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	result, err := installSkill(ctx, "", command)
	for _, p := range profiles {
		if err != nil || result.ExitCode != 0 {
			break
		}
		var next SkillInstall
		var ran bool
		if next, ran, err = installProfileSkill(ctx, p, agents); ran {
			result.Output = strings.TrimSpace(result.Output + "\n\nProfile " + p.Name + ":\n" + next.Output)
			result.ExitCode = next.ExitCode
		}
	}
	if len(result.Output) > 64*1024 {
		result.Output = "…" + result.Output[len(result.Output)-64*1024:]
	}
	return result, err
}

// InstallProfileSkill installs the Orchestra skill where a profile is missing it; nil is
// Default. Skills every profile shares are installed once, for Default.
func InstallProfileSkill(ctx context.Context, p *Profile) (SkillInstall, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	shared, own := []string{}, []string{}
	for _, agent := range skillAgentOrder() {
		if agentConfigEnv[agent] == "" || profileSkillInstalled(p, agent) {
			continue
		}
		if _, ok := profileFolders(p)[agentConfigEnv[agent]]; ok && profileSkillAgents[agent] {
			own = append(own, agent)
		} else {
			shared = append(shared, agent)
		}
	}
	result, err := SkillInstall{}, error(nil)
	if len(shared) > 0 {
		command, _ := SkillCommand(shared)
		if result, err = installSkill(ctx, "", command); err != nil || result.ExitCode != 0 {
			return result, err
		}
	}
	if len(own) > 0 {
		next, _, e := installProfileSkill(ctx, p, own)
		result.Output = strings.TrimSpace(result.Output + "\n\n" + next.Output)
		result.ExitCode, err = next.ExitCode, e
	}
	return result, err
}

// installProfileSkill installs the skill into the profile's own config folders and reports
// whether any of the agents needed it.
func installProfileSkill(ctx context.Context, p *Profile, agents []string) (SkillInstall, bool, error) {
	exports, chosen := "", []string{}
	for _, agent := range agents {
		folder, ok := p.Folders[agent]
		if !ok || !profileSkillAgents[agent] {
			continue
		}
		path, err := expandFolder(folder)
		if err == nil {
			err = os.MkdirAll(path, 0700)
		}
		if err != nil {
			return SkillInstall{}, true, err
		}
		// The skills CLI installs into CLAUDE_CONFIG_DIR when it is set.
		exports += "export " + agentConfigEnv[agent] + "=" + shellQuote(path) + "; "
		chosen = append(chosen, agent)
	}
	if len(chosen) == 0 {
		return SkillInstall{}, false, nil
	}
	command, _ := SkillCommand(chosen)
	result, err := installSkill(ctx, exports, command)
	return result, true, err
}

// profileSkillInstalled reports whether the agent finds the Orchestra skill with the profile.
// It only checks that the skill file exists.
func profileSkillInstalled(p *Profile, agent string) bool {
	home, _ := os.UserHomeDir()
	folder := profileFolders(p)[agentConfigEnv[agent]]
	switch {
	case folder != "" && profileSkillAgents[agent]:
	case agent == "claude":
		if folder = os.Getenv("CLAUDE_CONFIG_DIR"); folder == "" {
			folder = filepath.Join(home, ".claude")
		}
	default:
		folder = filepath.Join(home, ".agents")
	}
	_, err := os.Stat(filepath.Join(folder, "skills", "orchestra", "SKILL.md"))
	return err == nil
}

func installSkill(ctx context.Context, exports, command string) (SkillInstall, error) {
	output, code, err := cliShell(ctx, `command -v npx >/dev/null 2>&1 || { echo `+missingNode+`; exit 127; }; `+exports+`exec `+command)
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
