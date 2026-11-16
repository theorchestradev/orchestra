package core

import "testing"

func TestInterfaceFontSizePreferences(t *testing.T) {
	s := fixtureStore(t)
	p := s.Snapshot().Preferences
	if p.UIFontSize != "" && p.UIFontSize != "default" {
		t.Fatal("new state must use the existing font size")
	}
	for _, size := range []string{"smaller", "small", "default", "large", "larger", ""} {
		p.UIFontSize = size
		if _, err := s.SetPreferences(p); err != nil {
			t.Fatalf("%q: %v", size, err)
		}
	}
	p.UIFontSize = "larger"
	if _, err := s.SetPreferences(p); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	if reopened.Snapshot().Preferences.UIFontSize != "larger" {
		t.Fatal("font size was not persisted")
	}
	p.UIFontSize = "huge"
	if _, err := s.SetPreferences(p); err == nil {
		t.Fatal("accepted invalid font size")
	}
	if s.Snapshot().Preferences.UIFontSize != "larger" {
		t.Fatal("invalid update changed font size")
	}
}
