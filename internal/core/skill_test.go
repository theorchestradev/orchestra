package core

import (
	"bytes"
	"context"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
)

func fakeSkillShell(t *testing.T, fake func(script string) ([]byte, int, error)) *[]string {
	t.Helper()
	scripts := []string{}
	var mu sync.Mutex // Sign-in checks run concurrently.
	previous := cliShell
	cliShell = func(_ context.Context, script string) ([]byte, int, error) {
		mu.Lock()
		defer mu.Unlock()
		scripts = append(scripts, script)
		return fake(script)
	}
	t.Cleanup(func() { cliShell = previous })
	return &scripts
}

func TestGuideCoversHelp(t *testing.T) {
	var out, stderr bytes.Buffer
	if RunCLI([]string{"help"}, &out, &stderr) != 0 {
		t.Fatal(stderr.String())
	}
	commands := regexp.MustCompile(`(?m)^(orchestra [a-z]+(?: [a-z]+)?)(?: [A-Z(\[]|$)`).FindAllStringSubmatch(out.String(), -1)
	if len(commands) < 12 {
		t.Fatalf("help lists too few commands: %q", commands)
	}
	for _, command := range commands {
		if !strings.Contains(cliGuide, command[1]) {
			t.Errorf("guide does not mention %q", command[1])
		}
	}
	// guide reads no state, so it works outside a session.
	t.Setenv("ORCHESTRA_SOCKET", "")
	t.Setenv("ORCHESTRA_SESSION_ID", "")
	t.Setenv("ORCHESTRA_CONTEXT", "")
	out.Reset()
	if RunCLI([]string{"guide"}, &out, &stderr) != 0 || out.String() != cliGuide || !strings.Contains(cliGuide, "ORCHESTRA_SESSION_ID") {
		t.Fatalf("guide output: %q %q", out.String(), stderr.String())
	}
	defer func(v string) { Version = v }(Version)
	Version = "0.2.0"
	out.Reset()
	if RunCLI([]string{"--version"}, &out, &stderr) != 0 || out.String() != "Orchestra 0.2.0\n" {
		t.Fatalf("version output: %q %q", out.String(), stderr.String())
	}
}

func TestSkillStubPointsToGuide(t *testing.T) {
	data, err := os.ReadFile("../../skills/orchestra/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "---\nname: orchestra\ndescription: ") || !strings.Contains(string(data), "orchestra guide") {
		t.Fatalf("unexpected stub: %s", data)
	}
}

func TestSkillCommand(t *testing.T) {
	command, err := SkillCommand([]string{"claude", "codex", "pi", "opencode", "grok", "antigravity", "vibe", "terminal", "unknown", "claude"})
	if err != nil {
		t.Fatal(err)
	}
	want := "npx --yes " + skillsCLI + " add " + skillSource + " --skill orchestra --global --yes --agent claude-code --agent codex --agent pi --agent opencode --agent grok --agent antigravity-cli --agent mistral-vibe"
	if command != want {
		t.Fatalf("got %q", command)
	}
	if !regexp.MustCompile(`^skills@\d+\.\d+\.\d+$`).MatchString(skillsCLI) {
		t.Fatalf("skills CLI is not pinned: %s", skillsCLI)
	}
	for _, agents := range [][]string{nil, {}, {"terminal", "unknown"}} {
		if _, err := SkillCommand(agents); err == nil {
			t.Fatalf("built a command without agents for %q", agents)
		}
	}
	scripts := fakeSkillShell(t, func(string) ([]byte, int, error) { return nil, 0, nil })
	if _, err := InstallAgentSkill(context.Background(), []string{"unknown"}, nil); err == nil || len(*scripts) != 0 {
		t.Fatal("ran an install without agents")
	}
}

func TestAgentSkillStatus(t *testing.T) {
	fakeSkillShell(t, func(string) ([]byte, int, error) {
		return []byte("motd orchestra-found:fake\norchestra-found:npx\norchestra-found:claude\norchestra-found:agy\n"), 0, nil
	})
	status := AgentSkillStatus(context.Background())
	if !status.Node || !status.Detected {
		t.Fatalf("status: %+v", status)
	}
	installed := []string{}
	for _, agent := range status.Agents {
		if agent.Installed {
			installed = append(installed, agent.ID)
		}
	}
	if !slices.Equal(installed, []string{"claude", "antigravity"}) || len(status.Agents) != len(skillAgentKeys) {
		t.Fatalf("agents: %+v", status.Agents)
	}
}

func TestInstallAgentSkill(t *testing.T) {
	scripts := fakeSkillShell(t, func(string) ([]byte, int, error) {
		return []byte("\x1b[32m✓\x1b[0m Installed orchestra\r\n"), 0, nil
	})
	result, err := InstallAgentSkill(context.Background(), []string{"codex"}, nil)
	if err != nil || result.ExitCode != 0 || result.Output != "✓ Installed orchestra" {
		t.Fatalf("%+v %v", result, err)
	}
	if len(*scripts) != 1 || !strings.HasSuffix((*scripts)[0], "exec npx --yes "+skillsCLI+" add "+skillSource+" --skill orchestra --global --yes --agent codex") {
		t.Fatalf("scripts: %q", *scripts)
	}

	fakeSkillShell(t, func(string) ([]byte, int, error) { return []byte("error: not found"), 1, nil })
	if result, err = InstallAgentSkill(context.Background(), []string{"codex"}, nil); err != nil || result.ExitCode != 1 || result.Output != "error: not found" {
		t.Fatalf("%+v %v", result, err)
	}

	fakeSkillShell(t, func(string) ([]byte, int, error) { return []byte(missingNode + "\n"), 127, nil })
	if _, err = InstallAgentSkill(context.Background(), []string{"codex"}, nil); err == nil || !strings.Contains(err.Error(), "Node.js (npx)") {
		t.Fatalf("missing Node: %v", err)
	}
}
