//go:build !darwin

package appearance

import "os/exec"

// RevealTheme opens a theme folder in the file manager.
func RevealTheme(folder string) error { return exec.Command("xdg-open", folder).Run() }
