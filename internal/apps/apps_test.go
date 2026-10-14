package apps

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func isKnown(id string) bool { _, ok := find(id); return ok }

func TestKnownApps(t *testing.T) {
	seen := map[string]bool{}
	for _, app := range knownApps {
		if app.id == "" || app.name == "" || seen[app.id] || len(app.bundleIDs)+len(app.executables) == 0 {
			t.Fatalf("invalid or duplicate app %+v", app)
		}
		if strings.Trim(app.id, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" || len(app.id) > 40 {
			t.Fatalf("core.SetPreferences would reject id %q", app.id)
		}
		seen[app.id] = true
	}
	order := []Kind{Editor, Terminal, Git, Files}
	for i := 1; i < len(knownApps); i++ {
		if slices.Index(order, knownApps[i].kind) < slices.Index(order, knownApps[i-1].kind) || slices.Index(order, knownApps[i].kind) < 0 {
			t.Fatalf("%s is out of its group", knownApps[i].id)
		}
	}
	if knownApps[len(knownApps)-1].id != FileManager || !isKnown("zed") || isKnown("") || isKnown("custom") {
		t.Fatal("the table must end with the file manager and recognize only its own apps")
	}
}

func TestInstalledDoesNotFail(t *testing.T) {
	// The list depends on the machine; it must only contain known apps with valid icons.
	for _, app := range Installed() {
		if !isKnown(app.ID) || app.Name == "" || (app.Icon != "" && !strings.HasPrefix(app.Icon, "data:image/png;base64,")) {
			t.Fatalf("listed %+v", app)
		}
	}
}

func TestOpenRejectsUnknownApps(t *testing.T) {
	if err := Open("custom", os.TempDir()); err == nil {
		t.Fatal("opened an unknown app")
	}
}
