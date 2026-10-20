package core

import (
	"encoding/json"
	"os"
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
	for _, m := range regexp.MustCompile(`\{ id: '([a-z]+)', name: '([^']+)'`).FindAllStringSubmatch(string(data), -1) {
		frontend = append(frontend, m[1])
		if names[m[1]] != m[2] {
			t.Fatalf("appearance.js names %s %q, the registry %q", m[1], m[2], names[m[1]])
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
		if (spec.ModelCommand == nil) != (spec.ParseModels == nil) || spec.ModelCommand != nil && spec.Models != nil {
			fail("needs either ModelCommand with ParseModels or a fixed Models catalog")
		}
		if (spec.ModelFlag != "") != (spec.ModelCommand != nil || spec.Models != nil) {
			fail("ModelFlag and a model catalog go together")
		}
		if spec.EffortFlag != nil && spec.ModelFlag == "" || spec.Efforts != nil && spec.EffortFlag == nil {
			fail("efforts need EffortFlag, and EffortFlag needs ModelFlag")
		}
		for _, effort := range spec.Efforts {
			if !effortPattern.MatchString(effort) {
				fail("invalid effort " + effort)
			}
		}
		for _, model := range spec.Models {
			unknown := slices.ContainsFunc(model.Efforts, func(e string) bool { return !slices.Contains(spec.Efforts, e) })
			if !modelPattern.MatchString(model.ID) || model.Efforts == nil || unknown {
				fail("invalid model " + model.ID)
			}
		}
		// Profiles list sign-ins for agents with a config folder and a skills CLI key.
		if (spec.ConfigEnv != "") != (spec.LoginStatusCommand != "") || spec.ConfigEnv != "" && spec.SkillKey == "" {
			fail("ConfigEnv needs LoginStatusCommand and SkillKey")
		}
		if (spec.LoginCommand != "" || spec.ProfileSkillDir != "") && spec.ConfigEnv == "" {
			fail("LoginCommand and ProfileSkillDir need ConfigEnv")
		}
		for _, command := range []string{spec.LoginCommand, spec.LoginStatusCommand} {
			if command != "" && !strings.HasPrefix(command, spec.Executable+" ") {
				fail(command + " does not run " + spec.Executable)
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
