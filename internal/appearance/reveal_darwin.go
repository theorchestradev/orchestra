package appearance

import (
	"os/exec"
	"path/filepath"
)

// RevealTheme selects a theme folder's theme.json in Finder.
func RevealTheme(folder string) error {
	return exec.Command("open", "-R", filepath.Join(folder, ThemeFile)).Run()
}
