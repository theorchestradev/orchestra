//go:build windows

package core

import (
	"context"
	"errors"
	"os/exec"
)

// updateAsset names the Windows release asset. Releases do not publish it yet; the name is
// fixed now so SHA256SUMS, the website and this updater agree when they do.
func updateAsset(version string) string { return "Orchestra-" + version + "-windows-x86_64.zip" }

// errUpdateUnsupported keeps Installable false, so the update dialog offers the download
// instead. Replacing a running executable and relaunching are Windows follow-ups.
var errUpdateUnsupported = errors.New("Automatic updates are not available on Windows yet. Download the new version from GitHub Releases and replace this one.")

func installedPath(string) (string, error) { return "", errUpdateUnsupported }
func unpackUpdate(context.Context, string, string, string) (string, error) {
	return "", errUpdateUnsupported
}
func verifyUpdate(context.Context, string, string, string) error { return errUpdateUnsupported }

// relaunchCommand starts the executable again; ApplyStaged never reaches it while updates
// are not installable.
func relaunchCommand(_ int, path string) *exec.Cmd { return exec.Command(path) }
