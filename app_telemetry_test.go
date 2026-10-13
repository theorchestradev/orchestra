package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/theorchestradev/orchestra/internal/core"
	"github.com/theorchestradev/orchestra/internal/telemetry"
)

func TestTelemetryBridgeCannotEnableContributorBuild(t *testing.T) {
	s, err := core.NewStore(filepath.Join(t.TempDir(), "state.json"), "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()
	c := telemetry.New("", "https://eu.i.posthog.com", "", nil)
	defer c.Close(context.Background())
	a := &App{store: s, telemetry: c}
	if _, err := a.SetTelemetryConsent(true); err == nil {
		t.Fatal("non-release bridge accepted opt-in")
	}
	if s.Snapshot().Preferences.TelemetryInstallID != "" {
		t.Fatal("non-release bridge created identity")
	}
	a.Track("unknown", map[string]any{"prompt": "private"})
	state, err := a.SetTelemetryConsent(false)
	if err != nil || !state.Preferences.TelemetryChoiceMade || state.Preferences.TelemetryEnabled {
		t.Fatal("bridge could not remember No Thanks")
	}
}
