package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
	"time"
)

// The frontend agent list and the telemetry enums are kept by hand; they must follow agentOrder.
func TestAgentListsFollowRegistry(t *testing.T) {
	ids, names := []string{}, map[string]string{}
	for _, spec := range agentOrder {
		ids = append(ids, spec.ID)
		names[spec.ID] = spec.Name
		if spec.Command == nil || spec.Executable == "" || (spec.ModelCommand == nil) != (spec.ParseModels == nil) || spec.Efforts != nil && spec.EffortFlag == nil {
			t.Fatalf("%s spec is incomplete", spec.ID)
		}
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
