package core

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLaunchPassesModelAndEffortFlags(t *testing.T) {
	cases := map[string][2]string{
		"claude":      {"opus", " --model 'opus' --effort 'high'"},
		"codex":       {"gpt-6.1-sol", ` -m 'gpt-6.1-sol' -c 'model_reasoning_effort="high"'`},
		"pi":          {"openrouter/~anthropic/claude-opus-latest", " --model 'openrouter/~anthropic/claude-opus-latest' --thinking 'high'"},
		"opencode":    {"opencode/big-pickle", " --model 'opencode/big-pickle'"},
		"kilo":        {"kilo/kilo-auto/free", " --model 'kilo/kilo-auto/free'"},
		"grok":        {"grok-4.7", " --model 'grok-4.7'"},
		"antigravity": {"gemini-3.8-flash-high", " --model 'gemini-3.8-flash-high'"},
		"omp":         {"openrouter/~anthropic/claude-opus-latest", " --model 'openrouter/~anthropic/claude-opus-latest' --thinking 'high'"},
		"muse":        {"muse-spark-1.3", " --model 'muse-spark-1.3' --reasoning-effort 'high'"},
	}
	for agent, tc := range cases {
		effort := "high"
		if agent == "opencode" || agent == "kilo" || agent == "grok" || agent == "antigravity" {
			effort = ""
		}
		if err := validateLaunch(agent, tc[0], effort); err != nil {
			t.Fatalf("%s rejected: %v", agent, err)
		}
		for _, launched := range []bool{false, true} {
			sess := &Session{ID: "id", Agent: agent, InitialPrompt: "prompt", HasLaunched: launched, AgentSessionID: "resume-id", Model: tc[0], Effort: effort}
			command, err := launchCommand(sess, "/tmp/extensions")
			if err != nil || !strings.Contains(command, tc[1]) {
				t.Fatalf("%s (resume %v) missing %q: %s", agent, launched, tc[1], command)
			}
			if !launched && !strings.HasSuffix(command, shellQuote("prompt")) {
				t.Fatalf("%s prompt is not last: %s", agent, command)
			}
		}
		sess := &Session{ID: "id", Agent: agent}
		if command, _ := launchCommand(sess, "/tmp/extensions"); strings.Contains(command, "--model") || strings.Contains(command, " -m ") || strings.Contains(command, "effort") || strings.Contains(command, "--thinking") {
			t.Fatalf("%s passed flags without a choice: %s", agent, command)
		}
	}
}

func TestLaunchRejectsUnsupportedModelOrEffort(t *testing.T) {
	for _, tc := range [][3]string{{"vibe", "x", ""}, {terminalAgent, "x", ""}, {"claude", "opus; rm -rf /", ""}, {"claude", "", "ultra"}, {"pi", "", "turbo"}, {"opencode", "a/b", "high"}, {"codex", "", "HIGH"}} {
		if validateLaunch(tc[0], tc[1], tc[2]) == nil {
			t.Fatalf("accepted %v", tc)
		}
	}
	if validateLaunch("vibe", "", "") != nil {
		t.Fatal("rejected defaults")
	}
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	if _, err := s.CreateSession(p.ID, "", "vibe", "", "", false, nil, "model", "", "", "", ""); err == nil {
		t.Fatal("created a Vibe session with a model")
	}
	state, err := s.CreateSession(p.ID, "", "claude", "", "", false, &GitHubItem{Number: 21}, "sonnet", "max", "", "", "")
	if err != nil || state.Workspaces[0].Sessions[0].Model != "sonnet" || state.Workspaces[0].Sessions[0].Effort != "max" {
		t.Fatalf("model and effort not stored: %v", err)
	}
}

func TestParseAgentModels(t *testing.T) {
	codex := []byte("shell banner\n" + `{"models":[{"slug":"gpt-6.1-sol","display_name":"GPT-6.1-Sol","visibility":"list","supported_reasoning_levels":[{"effort":"low"},{"effort":"ultra"}]},{"slug":"hidden","visibility":"hide"}]}` + "\n")
	pi := []byte("provider    model                          context  max-out  thinking  images\nopenrouter  ~anthropic/claude-opus-latest  1M       128K     yes       yes\nopenrouter  ~deepseek/deepseek-pro-latest  1.0M     384K     no        no\n")
	grok := []byte("You are not authenticated.\n\nDefault model: grok-4.7\n\nAvailable models:\n  * grok-4.7 (default)\n  grok-4.6\n")
	agy := []byte("Fetching available models...\ngemini-3.8-flash-high\tGemini 3.8 Flash (High)\n")
	omp := []byte("shell banner\n" + `{"models":[{"kind":"chat","selector":"openrouter/~anthropic/claude-opus-latest","name":"Claude Opus Latest","thinking":["low","high","Turbo"]},{"kind":"image","selector":"openai/gpt-image"},{"kind":"chat","selector":"ollama/gemma4:latest","thinking":[]}]}` + "\n")
	opencode := []byte("opencode/big-pickle\nopenrouter/~anthropic/claude-haiku-latest\n")
	cases := map[string]struct {
		output []byte
		want   []AgentModel
	}{
		"codex":       {codex, []AgentModel{{"gpt-6.1-sol", "GPT-6.1-Sol", []string{"low", "ultra"}}}},
		"pi":          {pi, []AgentModel{{"openrouter/~anthropic/claude-opus-latest", "openrouter/~anthropic/claude-opus-latest", piEfforts}, {"openrouter/~deepseek/deepseek-pro-latest", "openrouter/~deepseek/deepseek-pro-latest", []string{}}}},
		"grok":        {grok, []AgentModel{{"grok-4.7", "grok-4.7", []string{}}, {"grok-4.6", "grok-4.6", []string{}}}},
		"antigravity": {agy, []AgentModel{{"gemini-3.8-flash-high", "Gemini 3.8 Flash (High)", []string{}}}},
		"omp":         {omp, []AgentModel{{"openrouter/~anthropic/claude-opus-latest", "Claude Opus Latest", []string{"low", "high"}}, {"ollama/gemma4:latest", "ollama/gemma4:latest", []string{}}}},
		"opencode":    {opencode, []AgentModel{{"opencode/big-pickle", "opencode/big-pickle", []string{}}, {"openrouter/~anthropic/claude-haiku-latest", "openrouter/~anthropic/claude-haiku-latest", []string{}}}},
	}
	for agent, tc := range cases {
		got, err := agentSpecs[agent].Model.Parse(tc.output)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s: %v %#v", agent, err, got)
		}
	}
	if models, err := AgentModels(context.Background(), "vibe"); err != nil || len(models) != 0 {
		t.Fatal("Vibe listed models")
	}
	if models, err := AgentModels(context.Background(), "claude"); err != nil || len(models) == 0 {
		t.Fatal("Claude aliases missing")
	}
}

func TestModelCatalogUsesSelectedAccountAfterShellStartup(t *testing.T) {
	dir := t.TempDir()
	record := filepath.Join(dir, "account")
	// Emulate an rc file that selects another account. The query must export the
	// selected profile afterward, just as terminal sessions do.
	shell := filepath.Join(dir, "shell")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\nexport CURSOR_CONFIG_DIR=wrong-account\nexport CURSOR_DATA_DIR=wrong-account\nexec /bin/sh -c \"$4\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cli := "#!/bin/sh\nprintf '%s\\n' \"$CURSOR_CONFIG_DIR\" \"$CURSOR_DATA_DIR\" > " + shellQuote(record) + "\nprintf 'Available models\\naccount-model - Account Model\\n'\n"
	if err := os.WriteFile(filepath.Join(dir, "cursor-agent"), []byte(cli), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	folder := filepath.Join(dir, "Work Account")
	profile := &Profile{ID: "work", Folders: map[string]string{"cursor": folder}}
	options, err := LaunchOptions(context.Background(), "cursor", profile)
	if err != nil || len(options.Models) != 1 || options.Models[0].ID != "account-model" {
		t.Fatalf("model catalog: %+v %v", options, err)
	}
	data, _ := os.ReadFile(record)
	if string(data) != folder+"\n"+folder+"\n" {
		t.Fatalf("queried another account: %q", data)
	}
	if _, err = os.Stat(folder); !os.IsNotExist(err) {
		t.Fatal("read-only query created the account folder")
	}
	if _, err = LaunchOptions(context.Background(), "cursor", &Profile{ID: "missing"}); err == nil {
		t.Fatal("missing profile account fell back to Default")
	}
}
