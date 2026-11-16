//go:build windows

package appearance

import (
	"os/exec"
	"path/filepath"
)

// RevealTheme selects a theme folder's theme.json in File Explorer. Explorer returns at
// once and reports exit code 1 even on success, so only starting it can fail.
func RevealTheme(folder string) error {
	cmd := exec.Command("explorer.exe", "/select,"+filepath.Join(folder, ThemeFile))
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
