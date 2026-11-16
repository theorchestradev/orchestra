//go:build windows

package sessionhost

import "log/slog"

// Supported is false on Windows: the host needs a ConPTY-backed process boundary before
// sessions can outlive the app there. Store keeps every session in the app process instead.
const Supported = false

// Run reports that the host cannot serve sessions on this platform.
func Run(string, string, *slog.Logger, Options) error { return ErrUnsupported }

func startHost(string, string, []string) error { return ErrUnsupported }

// responsiblePID is 0 where no process answers for another one's privacy permissions.
func responsiblePID(int) int { return 0 }
