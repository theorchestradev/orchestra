package core

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// The frontend agent list and the telemetry enums are kept by hand; they must follow agentOrder.
func TestAgentListsFollowRegistry(t *testing.T) {
	ids, names := []string{}, map[string]string{}
	for _, spec := range agentOrder {
		ids = append(ids, spec.ID)
		names[spec.ID] = spec.Name
	}
	data, err := os.ReadFile("../../frontend/src/shared/appearance.js")
	if err != nil {
		t.Fatal(err)
	}
	frontend := []string{}
	for _, m := range regexp.MustCompile(`\{ id: '([a-z]+)', name: '([^']+)'[^\n]*`).FindAllStringSubmatch(string(data), -1) {
		frontend = append(frontend, m[1])
		if names[m[1]] != m[2] {
			t.Fatalf("appearance.js names %s %q, the registry %q", m[1], m[2], names[m[1]])
		}
		if (agentSpecs[m[1]].Account != nil) != strings.Contains(m[0], "profiles: true") {
			t.Fatalf("appearance.js profile capability differs for %s", m[1])
		}
		if agentSpecs[m[1]].NoInitialPrompt != strings.Contains(m[0], "initialPrompt: false") {
			t.Fatalf("appearance.js initial prompt capability differs for %s", m[1])
		}
	}
	if !slices.Equal(frontend, ids) {
		t.Fatalf("appearance.js agents %v, registry %v", frontend, ids)
	}
	data, err = os.ReadFile("../telemetry/events.json")
	if err != nil {
		t.Fatal(err)
	}
	var events map[string]map[string]struct{ Values []string }
	if err = json.Unmarshal(data, &events); err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{"session_started", "session_ended"} {
		values := slices.Sorted(slices.Values(events[event]["agent"].Values))
		want := slices.Sorted(slices.Values(append(slices.Clone(ids), terminalAgent)))
		if !slices.Equal(values, want) {
			t.Fatalf("%s agents %v, want %v", event, values, want)
		}
	}
}

// The registry has no other checks, so a spec must hold together on its own.
func TestAgentSpecsAreConsistent(t *testing.T) {
	seen := map[string]bool{}
	for _, spec := range agentOrder {
		fail := func(problem string) { t.Errorf("%s: %s", spec.ID, problem) }
		if spec.ID == "" || spec.ID == terminalAgent || seen[spec.ID] {
			fail("needs a unique ID other than terminal")
		}
		seen[spec.ID] = true
		if spec.Name == "" || spec.Executable == "" || spec.Command == nil {
			fail("needs a name, an executable and a command")
		}
		// The exit 127 message names Executable, so every command must start with it.
		for _, l := range []launch{{Session: &Session{ID: "id"}}, {Session: &Session{ID: "id"}, Resume: "resume", Prompt: "'prompt'"}} {
			if command := spec.Command(l); !strings.HasPrefix(command+" ", spec.Executable+" ") {
				fail("command " + command + " does not run " + spec.Executable)
			}
		}
		// Each agent learns its conversation ID one way at most; none means ORCHESTRA_AGENT_ID_FILE.
		ways := 0
		for _, set := range []bool{spec.PresetID, spec.RecoverID != nil, spec.CaptureID != nil} {
			if set {
				ways++
			}
		}
		if ways > 1 {
			fail("sets more than one of PresetID, RecoverID and CaptureID")
		}
		if spec.RecoverID != nil && spec.RecoverID.NumSubexp() != 1 {
			fail("RecoverID needs exactly one group for the ID")
		}
		if model := spec.Model; model != nil {
			if model.Env != "" && (model.Flag != "" || spec.Prepare == nil) {
				fail("environment model transport needs Prepare and cannot also use a flag")
			}
			if (model.Command == nil) != (model.Parse == nil) || model.Command != nil && model.Catalog != nil {
				fail("needs a catalog command/parser or a fixed catalog")
			}
			if model.Flag == "" && spec.Prepare == nil {
				fail("a model without a CLI flag needs Prepare")
			}
			if model.Efforts != nil && model.EffortFlag == nil && spec.Prepare == nil {
				fail("effort choices need a flag or Prepare")
			}
			for _, effort := range model.Efforts {
				if !effortPattern.MatchString(effort) {
					fail("invalid effort " + effort)
				}
			}
			for _, m := range model.Catalog {
				unknown := slices.ContainsFunc(m.Efforts, func(e string) bool { return !slices.Contains(model.Efforts, e) })
				if !modelPattern.MatchString(m.ID) || m.Efforts == nil || unknown {
					fail("invalid model " + m.ID)
				}
			}
		}
		if len(spec.OverrideEnv) > 0 && spec.Prepare == nil {
			fail("environment overrides need Prepare")
		}
		if account := spec.Account; account != nil {
			if account.Env == "" || account.Status == "" || account.Login == "" || spec.SkillKey == "" {
				fail("account needs config isolation, sign-in, status and a skill key")
			}
			for _, command := range []string{account.Login, account.Status} {
				if !strings.HasPrefix(command, spec.Executable+" ") {
					fail(command + " does not run " + spec.Executable)
				}
			}
			if account.CopySkill && account.SkillDir == "" {
				fail("copying profile skills needs their default directory")
			}
		}
	}
}

func TestCaptureVibeID(t *testing.T) {
	dir := t.TempDir()
	term := &TerminalSession{agentDirectory: dir, launched: time.Now().Add(-time.Minute)}
	if captureVibeID(term) != "" {
		t.Fatal("captured an ID from an empty folder")
	}
	if err := os.MkdirAll(filepath.Join(dir, "a"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a", "meta.json"), []byte(`{"metadata":{"session_id":"vibe-id"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if id := captureVibeID(term); id != "vibe-id" {
		t.Fatalf("captured %q", id)
	}
	term.launched = time.Now().Add(time.Minute)
	if captureVibeID(term) != "" {
		t.Fatal("captured a log from before launch")
	}
}

func TestInstallClineHooksKeepsUserHooks(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hooks")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	own := []byte("#!/bin/sh\necho mine\n")
	if err := os.WriteFile(filepath.Join(dir, "TaskStart.sh"), own, 0755); err != nil {
		t.Fatal(err)
	}
	stale := []byte("#!/bin/sh\n" + clineHookMarker + "\nold\n")
	if err := os.WriteFile(filepath.Join(dir, "TaskComplete.sh"), stale, 0755); err != nil {
		t.Fatal(err)
	}
	if err := installClineHooks(dir); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "TaskStart.sh")); string(got) != string(own) {
		t.Fatal("replaced the user's hook")
	}
	for event, state := range clineHooks {
		if event == "TaskStart" {
			continue
		}
		path := filepath.Join(dir, event+".sh")
		data, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(data), "$ORCHESTRA_CLINE") || !strings.Contains(string(data), statusCommand(state)) {
			t.Fatalf("%s hook is %q", event, data)
		}
		if info, _ := os.Stat(path); info.Mode()&0100 == 0 {
			t.Fatalf("%s hook is not executable", event)
		}
	}
}

// Simulates omp lifecycle events; never contacts a model service.
func TestOmpExtensionReportsMainSessionStatus(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("Node.js is required for the omp extension check")
	}
	dir := t.TempDir()
	extension := filepath.Join(dir, "OmpStatusExtension.js")
	if err = os.WriteFile(extension, resources["OmpStatusExtension.js"], 0600); err != nil {
		t.Fatal(err)
	}
	harness := filepath.Join(dir, "omp.mjs")
	code := `import {readFileSync} from 'node:fs';
const {default: register} = await import(process.argv[2]);
const handlers = {};
register({on: (event, handler) => (handlers[event] ||= []).push(handler)});
const main = {agent: {kind: 'main'}, sessionManager: {getSessionId: () => 'main-id'}};
const sub = {agent: {kind: 'sub'}, sessionManager: {getSessionId: () => 'sub-id'}};
const seen = [];
const emit = (event, ctx) => {
  for (const handler of handlers[event] || []) handler({}, ctx);
  seen.push(readFileSync(process.env.ORCHESTRA_STATUS_FILE, 'utf8'));
};
emit('session_start', main);
emit('before_agent_start', main);
emit('agent_end', sub);
emit('tool_approval_requested', main);
emit('tool_approval_resolved', main);
emit('agent_end', main);
emit('auto_compaction_start', main);
emit('auto_compaction_end', main);
console.log(seen.join(' ') + ' ' + readFileSync(process.env.ORCHESTRA_AGENT_ID_FILE, 'utf8'));
`
	if err = os.WriteFile(harness, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, harness, "file://"+extension)
	cmd.Env = append(os.Environ(), "ORCHESTRA_STATUS_FILE="+filepath.Join(dir, "status"), "ORCHESTRA_AGENT_ID_FILE="+filepath.Join(dir, "id"))
	out, err := cmd.CombinedOutput()
	want := "waiting working working waiting working waiting working waiting main-id"
	if err != nil || strings.TrimSpace(string(out)) != want {
		t.Fatalf("got %q (%v), want %q", out, err, want)
	}
}

// Muse clears the environment of hook commands, so its hooks must find the session's files
// without Orchestra's variables.
func TestMuseHooksReportWithoutEnvironment(t *testing.T) {
	dir := t.TempDir()
	exe, _ := os.Executable()
	if err := os.WriteFile(filepath.Join(dir, "orchestra"), []byte("#!/bin/sh\nexec "+shellQuote(exe)+" --orchestra-cli \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	// A fake muse prints the value of its -c flag, after the permission profile.
	if err := os.WriteFile(filepath.Join(dir, "muse"), []byte("#!/bin/sh\nprintf %s \"$4\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	command := museAgent.Command(launch{Session: &Session{ID: "id"}, Dir: dir})
	out, err := exec.Command("/bin/sh", "-c", "PATH="+shellQuote(dir)+" "+command).Output()
	if err != nil {
		t.Fatal(err)
	}
	var hooks map[string][]struct {
		Matcher string
		Hooks   []struct{ Type, Command string }
	}
	if err = json.Unmarshal([]byte(strings.TrimPrefix(string(out), "hooks=")), &hooks); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	statusFile, idFile := sessionFiles(dir, "id")
	want := map[string]string{"SessionStart": "waiting", "UserPromptSubmit": "working", "PreToolUse": "working", "PostToolUse": "working", "PermissionRequest": "waiting", "Stop": "waiting", "StopFailure": "waiting"}
	if len(hooks) != len(want) {
		t.Fatalf("hooks %v", hooks)
	}
	for event, state := range want {
		groups := hooks[event]
		if len(groups) != 1 || len(groups[0].Hooks) != 1 || groups[0].Hooks[0].Type != "command" {
			t.Fatalf("%s hooks %+v", event, groups)
		}
		cmd := exec.Command("/bin/sh", "-c", groups[0].Hooks[0].Command)
		cmd.Env = []string{}
		cmd.Stdin = strings.NewReader(`{"session_id":"muse-id"}`)
		_ = os.Remove(statusFile)
		if err = cmd.Run(); err != nil {
			t.Fatal(err)
		}
		status, _ := os.ReadFile(statusFile)
		id, _ := os.ReadFile(idFile)
		if string(status) != state || string(id) != "muse-id" {
			t.Fatalf("%s wrote status %q and ID %q", event, status, id)
		}
	}
}

func TestCaptureAntigravityID(t *testing.T) {
	dir := t.TempDir()
	term := &TerminalSession{agentDirectory: dir}
	if captureAntigravityID(term) != "" {
		t.Fatal("captured an ID without a log")
	}
	log := "I1114 00:32:35.967670 256 manager.go:1122] Full redraw completed (RerenderAll) for conversation  (epoch 0, items 1) in 1ms\n" +
		"I1114 00:32:38.183293 491 conversation_manager.go:1044] Streaming conversation 0c2e620a-6dc4-44d1-ba1a-c98d35e59ef3\n" +
		"I1114 00:40:01.000000 491 conversation_manager.go:1044] Streaming conversation 7f54d120-074f-4dd8-b2a9-26c7366e6cee\n"
	if err := os.WriteFile(filepath.Join(dir, "cli.log"), []byte(log+strings.Repeat("x", antigravityLogTail)), 0600); err != nil {
		t.Fatal(err)
	}
	if id := captureAntigravityID(term); id != "" {
		t.Fatalf("read beyond the log tail: %q", id)
	}
	if err := os.WriteFile(filepath.Join(dir, "cli.log"), []byte(log), 0600); err != nil {
		t.Fatal(err)
	}
	if id := captureAntigravityID(term); id != "7f54d120-074f-4dd8-b2a9-26c7366e6cee" {
		t.Fatalf("captured %q", id)
	}
}
