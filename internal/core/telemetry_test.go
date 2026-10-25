package core

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func TestTelemetryConsentIdentityPersistence(t *testing.T) {
	s := fixtureStore(t)
	fresh := s.Snapshot().Preferences
	if fresh.TelemetryEnabled || fresh.TelemetryChoiceMade || fresh.TelemetryInstallID != "" {
		t.Fatal("fresh install has consent or identity")
	}
	// The general bridge cannot forge consent or a typed/account-derived identity.
	forged := fresh
	forged.TelemetryEnabled, forged.TelemetryChoiceMade, forged.TelemetryInstallID = true, true, "email@example.com"
	state, err := s.SetPreferences(forged)
	if err != nil {
		t.Fatal(err)
	}
	if state.Preferences.TelemetryEnabled || state.Preferences.TelemetryInstallID != "" {
		t.Fatal("general preferences bypassed consent")
	}
	state, err = s.SetTelemetryConsent(false)
	if err != nil || !state.Preferences.TelemetryChoiceMade || state.Preferences.TelemetryInstallID != "" {
		t.Fatal("No Thanks did not persist without identity")
	}
	state, err = s.SetTelemetryConsent(true)
	if err != nil {
		t.Fatal(err)
	}
	first := state.Preferences.TelemetryInstallID
	if _, err := uuid.Parse(first); err != nil {
		t.Fatal("identity is not random UUID")
	}
	reopened, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	if reopened.Snapshot().Preferences.TelemetryInstallID != first {
		t.Fatal("identity not persisted")
	}
	if _, err := s.SetPreferences(fresh); err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Preferences.TelemetryInstallID != first {
		t.Fatal("stale preference write cleared consent")
	}
	state, err = s.SetTelemetryConsent(false)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(s.path)
	if state.Preferences.TelemetryInstallID != "" || state.Preferences.TelemetryEnabled || bytes.Contains(data, []byte(first)) {
		t.Fatal("opt-out kept identity")
	}
	state, err = s.SetTelemetryConsent(true)
	if err != nil || state.Preferences.TelemetryInstallID == first {
		t.Fatal("re-consent reused identity")
	}
	previous := state.Preferences
	s.path = filepath.Join(t.TempDir(), "blocked", "state.json")
	if err := os.WriteFile(filepath.Dir(s.path), []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetTelemetryConsent(false); err == nil || !reflect.DeepEqual(s.Snapshot().Preferences, previous) {
		t.Fatal("failed consent save changed persisted preferences")
	}
}
