package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestGeneralPreferencesPersistence(t *testing.T) {
	for _, view := range []string{"last", "scratchpad", "github", "reviews"} {
		t.Run(view, func(t *testing.T) {
			s := fixtureStore(t)
			workspace := workspaceFixture(t, s)
			session := sessionFixture(t, s, workspace, "pi")
			s.mu.Lock()
			s.state.Workspaces[0].Repositories[0].GitHub = "example/orchestra"
			s.mu.Unlock()
			prefs := s.Snapshot().Preferences
			prefs.DefaultAgent = "pi"
			prefs.StartupView = view
			prefs.ConfirmQuit = false
			prefs.ConfirmSessionClose = false
			prefs.GitHubRefreshInterval = 60
			if _, err := s.SetPreferences(prefs); err != nil {
				t.Fatal(err)
			}
			reopened, err := NewStore(s.path, s.executable, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Shutdown()
			state := reopened.Snapshot()
			p := state.Workspaces[0]
			if !reflect.DeepEqual(state.Preferences, prefs) {
				t.Fatalf("preferences changed on reopen: %+v", state.Preferences)
			}
			if p.SelectedSessionID != session.ID || p.Sessions[0].Running || !p.Sessions[0].Restored {
				t.Fatal("startup changed the conversation or started a process")
			}
			if p.ShowsScratchpad || p.ShowsGitHub || p.ShowsReviews {
				t.Fatalf("startup settings changed the saved destination before UI selection: %+v", p)
			}
			for _, update := range []func(*Preferences){func(p *Preferences) { p.DefaultAgent = "invalid" }, func(p *Preferences) { p.StartupView = "terminal" }, func(p *Preferences) { p.GitHubRefreshInterval = -1 }, func(p *Preferences) { p.GitHubRefreshInterval = 15 }} {
				invalid := prefs
				update(&invalid)
				if _, err := s.SetPreferences(invalid); err == nil {
					t.Fatal("accepted invalid general preference")
				}
				if !reflect.DeepEqual(s.Snapshot().Preferences, prefs) {
					t.Fatal("invalid preference changed saved settings")
				}
			}
			oldPath := s.path
			blocker := filepath.Join(t.TempDir(), "file")
			if err := os.WriteFile(blocker, []byte("not a folder"), 0600); err != nil {
				t.Fatal(err)
			}
			s.path = filepath.Join(blocker, "state.json")
			changed := prefs
			changed.DefaultAgent = "codex"
			_, err = s.SetPreferences(changed)
			s.path = oldPath
			if err == nil || !reflect.DeepEqual(s.Snapshot().Preferences, prefs) {
				t.Fatal("failed save changed live preferences")
			}
		})
	}
}

func TestArcModePersistenceAndWorkspaceSwitch(t *testing.T) {
	s := fixtureStore(t)
	first := workspaceFixture(t, s)
	session := sessionFixture(t, s, first, "codex")
	second, err := s.CreateWorkspace("Empty space", t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	secondID := second.SelectedWorkspaceID
	if s.Snapshot().Preferences.ArcMode {
		t.Fatal("Arc Mode should default to off")
	}
	for _, enabled := range []bool{true, false} {
		prefs := s.Snapshot().Preferences
		prefs.ArcMode = enabled
		if _, err := s.SetPreferences(prefs); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Select(first.ID, "workspace"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Select(secondID, "workspace"); err != nil {
			t.Fatal(err)
		}
		reopened, err := NewStore(s.path, s.executable, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		state := reopened.Snapshot()
		reopened.Shutdown()
		if state.Preferences.ArcMode != enabled || state.SelectedWorkspaceID != secondID {
			t.Fatal("mode or selected space was not restored")
		}
		if state.Workspaces[0].SelectedSessionID != session.ID || state.Workspaces[0].Sessions[0].Running || !state.Workspaces[0].Sessions[0].Restored {
			t.Fatal("switching spaces changed or started a restored session")
		}
	}
}

func TestTerminalPaddingPreferences(t *testing.T) {
	s := fixtureStore(t)
	prefs := s.Snapshot().Preferences
	if prefs.TerminalPaddingX != 12 || prefs.TerminalPaddingY != 8 {
		t.Fatalf("unexpected default terminal padding: %+v", prefs)
	}
	prefs.TerminalPaddingX, prefs.TerminalPaddingY = 0, 24
	if _, err := s.SetPreferences(prefs); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	if got := reopened.Snapshot().Preferences; got.TerminalPaddingX != 0 || got.TerminalPaddingY != 24 {
		t.Fatalf("terminal padding was not restored: %+v", got)
	}
	for _, invalid := range [][2]int{{-1, 8}, {25, 8}, {12, -1}, {12, 25}} {
		prefs.TerminalPaddingX, prefs.TerminalPaddingY = invalid[0], invalid[1]
		if _, err := s.SetPreferences(prefs); err == nil {
			t.Fatalf("accepted out-of-range terminal padding %v", invalid)
		}
	}
}

func TestTerminalFontFamilyPreference(t *testing.T) {
	s := fixtureStore(t)
	prefs := s.Snapshot().Preferences
	if prefs.TerminalFontFamily != "" {
		t.Fatalf("new installs should use System Monospace: %q", prefs.TerminalFontFamily)
	}
	// A font that is not installed is kept, so Settings can label it.
	prefs.TerminalFontFamily = "JetBrainsMono Nerd Font Mono"
	if _, err := s.SetPreferences(prefs); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	if got := reopened.Snapshot().Preferences.TerminalFontFamily; got != prefs.TerminalFontFamily {
		t.Fatalf("terminal font was not restored: %q", got)
	}
	for _, invalid := range []string{`Evil", serif`, `Back\slash`, "Line\nBreak", strings.Repeat("a", 201)} {
		changed := prefs
		changed.TerminalFontFamily = invalid
		if _, err := s.SetPreferences(changed); err == nil {
			t.Fatalf("accepted terminal font %q", invalid)
		}
	}
}

func TestOpenInAppPreference(t *testing.T) {
	s := fixtureStore(t)
	prefs := s.Snapshot().Preferences
	if prefs.OpenInApp != "" {
		t.Fatalf("new installs should not have chosen an app: %q", prefs.OpenInApp)
	}
	if prefs.HideOpenInButton {
		t.Fatal("new installs should show the Open in button")
	}
	prefs.OpenInApp = "zed"
	prefs.HideOpenInButton = true
	if _, err := s.SetPreferences(prefs); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	if got := reopened.Snapshot().Preferences; got.OpenInApp != "zed" || !got.HideOpenInButton {
		t.Fatalf("Open in preferences were not restored: %q %v", got.OpenInApp, got.HideOpenInButton)
	}
	prefs.OpenInApp = "/Applications/Evil.app"
	if _, err := s.SetPreferences(prefs); err == nil {
		t.Fatal("accepted an unknown app")
	}
}

func TestSidebarTabsPreference(t *testing.T) {
	s := fixtureStore(t)
	prefs := s.Snapshot().Preferences
	if len(prefs.SidebarTabs) != 0 {
		t.Fatalf("new installs should use the default tab order: %v", prefs.SidebarTabs)
	}
	prefs.SidebarTabs = []string{"PR", "Changes", "Files"}
	if _, err := s.SetPreferences(prefs); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	if got := reopened.Snapshot().Preferences.SidebarTabs; strings.Join(got, ",") != "PR,Changes,Files" {
		t.Fatalf("sidebar tab order was not restored: %v", got)
	}
	for _, tabs := range [][]string{{"Changes", "Changes"}, {"Terminal"}} {
		prefs.SidebarTabs = tabs
		if _, err := s.SetPreferences(prefs); err == nil {
			t.Fatalf("accepted sidebar tabs %v", tabs)
		}
	}
}

func TestUsageStatusPreferences(t *testing.T) {
	s := fixtureStore(t)
	prefs := s.Snapshot().Preferences
	if prefs.HideUsageStatus || prefs.UsageDetailed {
		t.Fatal("new installs should show compact usage in the status bar")
	}
	prefs.HideUsageStatus, prefs.UsageDetailed = true, true
	if _, err := s.SetPreferences(prefs); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	if got := reopened.Snapshot().Preferences; !got.HideUsageStatus || !got.UsageDetailed {
		t.Fatalf("usage preferences were not restored: %v %v", got.HideUsageStatus, got.UsageDetailed)
	}
}

func TestAppearancePreference(t *testing.T) {
	if defaultPreferences().Appearance != "system" {
		t.Fatal("new installs should follow the system appearance")
	}
	s := fixtureStore(t)
	for _, mode := range []string{"system", "light", "dark"} {
		prefs := s.Snapshot().Preferences
		prefs.Appearance = mode
		if state, err := s.SetPreferences(prefs); err != nil || state.Preferences.Appearance != mode {
			t.Fatalf("appearance %q: %v", mode, err)
		}
	}
	prefs := s.Snapshot().Preferences
	prefs.Appearance = "auto"
	if _, err := s.SetPreferences(prefs); err == nil {
		t.Fatal("accepted an unknown appearance")
	}
}
func TestLegacyPreferencesAndUnavailableStartupView(t *testing.T) {
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	sess := sessionFixture(t, s, p, "claude")
	legacy := []byte(`{"appearance":"light","theme":"ayu","terminalFontSize":16,"terminalFontSmoothing":false,"sidebarWidth":248.4453125}`)
	var prefs Preferences
	if err := json.Unmarshal(legacy, &prefs); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetPreferences(prefs); err != nil {
		t.Fatal(err)
	}
	normalized := s.Snapshot().Preferences
	if normalized.DefaultAgent != "claude" || normalized.StartupView != "last" || !normalized.ConfirmQuit || !normalized.ConfirmSessionClose || normalized.Appearance != "light" || normalized.TerminalFontSmoothing || normalized.SidebarWidth != 248.4453125 {
		t.Fatalf("legacy preferences not preserved: %+v", normalized)
	}
	// Also exercise old JSON read directly, without a SetPreferences migration.
	state := s.Snapshot()
	encoded, _ := json.Marshal(state)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	raw["preferences"] = legacy
	encoded, _ = json.Marshal(raw)
	if err := os.WriteFile(s.path, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	inherited := normalized
	inherited.TerminalPaddingX, inherited.TerminalPaddingY = 12, 8
	if !reflect.DeepEqual(reopened.Snapshot().Preferences, inherited) {
		t.Fatal("old workspace did not inherit new defaults")
	}
	normalized.StartupView = "github"
	if _, err = s.SetPreferences(normalized); err != nil {
		t.Fatal(err)
	}
	unavailable, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer unavailable.Shutdown()
	restored := unavailable.Snapshot().Workspaces[0]
	if restored.ShowsGitHub || restored.SelectedSessionID != sess.ID {
		t.Fatal("unavailable GitHub should preserve last selection")
	}
}

func TestTranslucentWindowPersistence(t *testing.T) {
	s := fixtureStore(t)
	if s.Snapshot().Preferences.Translucent {
		t.Fatal("Translucent Window should default to off")
	}
	for _, enabled := range []bool{true, false} {
		prefs := s.Snapshot().Preferences
		prefs.Translucent = enabled
		if _, err := s.SetPreferences(prefs); err != nil {
			t.Fatal(err)
		}
		reopened, err := NewStore(s.path, s.executable, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		state := reopened.Snapshot()
		reopened.Shutdown()
		if state.Preferences.Translucent != enabled {
			t.Fatalf("Translucent Window was not restored as %v", enabled)
		}
	}
}

func TestWindowOpacityPreference(t *testing.T) {
	s := fixtureStore(t)
	if got := s.Snapshot().Preferences.WindowOpacity; got != defaultWindowOpacity {
		t.Fatalf("Window opacity should default to %d, got %d", defaultWindowOpacity, got)
	}
	for _, c := range [][2]int{{60, 60}, {minWindowOpacity, minWindowOpacity}, {maxWindowOpacity, maxWindowOpacity}, {10, minWindowOpacity}, {100, maxWindowOpacity}, {-5, minWindowOpacity}, {0, defaultWindowOpacity}} {
		prefs := s.Snapshot().Preferences
		prefs.WindowOpacity = c[0]
		if _, err := s.SetPreferences(prefs); err != nil {
			t.Fatal(err)
		}
		reopened, err := NewStore(s.path, s.executable, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		got := reopened.Snapshot().Preferences.WindowOpacity
		reopened.Shutdown()
		if got != c[1] {
			t.Fatalf("window opacity %d was restored as %d, want %d", c[0], got, c[1])
		}
	}
}

func TestStatusBarPreferences(t *testing.T) {
	s := fixtureStore(t)
	prefs := s.Snapshot().Preferences
	if prefs.HideStatusBar || len(prefs.HiddenStatusBarItems) != 0 || prefs.HideUsageStatus {
		t.Fatal("status bar defaults must preserve all existing indicators")
	}
	prefs.HideStatusBar = true
	prefs.HiddenStatusBarItems = []string{"branch", "waiting", "usage"}
	prefs.StatusBarLayout = [][]string{{"theme", "usage"}, {"workspace", "branch"}}
	if _, err := s.SetPreferences(prefs); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	got := reopened.Snapshot().Preferences
	if !got.HideStatusBar || !reflect.DeepEqual(got.StatusBarLayout, prefs.StatusBarLayout) || strings.Join(got.HiddenStatusBarItems, ",") != "branch,waiting,usage" {
		t.Fatalf("status bar preferences were not restored: %+v", got)
	}
}

func TestInvalidStatusBarLayout(t *testing.T) {
	s := fixtureStore(t)
	for _, layout := range [][][]string{{{"branch"}}, {{"branch"}, {"branch"}}, {{"unknown"}, {}}} {
		prefs := s.Snapshot().Preferences
		prefs.StatusBarLayout = layout
		if _, err := s.SetPreferences(prefs); err == nil {
			t.Fatalf("accepted invalid layout: %v", layout)
		}
	}
}
