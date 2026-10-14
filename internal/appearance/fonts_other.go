//go:build !darwin

package appearance

import (
	"context"
	"os/exec"
	"time"
)

// MonospaceFonts lists the monospaced font families fontconfig knows, or nil when fc-list fails.
func MonospaceFonts() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "fc-list", ":spacing=mono", "family").Output()
	if err != nil {
		return nil
	}
	return parseFontconfigFamilies(string(out))
}
