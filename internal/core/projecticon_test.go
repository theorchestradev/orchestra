package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeIconFile(t *testing.T, root, rel string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindProjectIcon(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []string
		want  string
	}{
		{"none", []string{"README.md", "src/components/icon.png"}, ""},
		{"public favicon", []string{"public/favicon.ico", "logos/favicon-48.png"}, "public/favicon.ico"},
		{"monorepo", []string{"apps/web/public/favicon.ico", "apps/docs/favicon-32x32.png"}, "apps/web/public/favicon.ico"},
		{"svg first", []string{"public/favicon.png", "static/favicon.svg"}, "static/favicon.svg"},
		{"app icon", []string{"build/appicon.png"}, "build/appicon.png"},
		{"dependencies skipped", []string{"node_modules/x/favicon.ico", ".git/favicon.ico"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeIconFile(t, root, "package.json")
			for _, f := range tc.files {
				writeIconFile(t, root, f)
			}
			got := findProjectIcon(root)
			if tc.want == "" && got != "" || tc.want != "" && got != filepath.Join(root, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFindProjectIconOutsideProjects(t *testing.T) {
	// A folder without project markers is only searched at its top level.
	root := t.TempDir()
	writeIconFile(t, root, "site/public/favicon.ico")
	if got := findProjectIcon(root); got != "" {
		t.Fatalf("got %q in a folder that is not a project", got)
	}
	writeIconFile(t, root, "favicon.ico")
	if got := findProjectIcon(root); got != filepath.Join(root, "favicon.ico") {
		t.Fatalf("got %q, want the top-level favicon", got)
	}

	// The home folder and its parents are never searched, even with project markers.
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	writeIconFile(t, home, ".git/HEAD")
	writeIconFile(t, home, "favicon.ico")
	for _, dir := range []string{home, filepath.Dir(home)} {
		if got := findProjectIcon(dir); got != "" {
			t.Fatalf("got %q for %s", got, dir)
		}
	}
}

func TestProjectIconDataURL(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "favicon.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ProjectIcon(root); !strings.HasPrefix(got, "data:image/svg+xml;base64,") {
		t.Fatalf("got %q", got)
	}
}
