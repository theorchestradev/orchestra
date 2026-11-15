//go:build windows

package core

import (
	"log/slog"
	"os/exec"
)

// startLocalProcess will run the session in a ConPTY. Until then, StartSession fails
// earlier with the same error, so no session reaches this point.
func startLocalProcess(*exec.Cmd, int, int, *slog.Logger) (terminalProcess, error) {
	return nil, errSessionsUnsupported
}
