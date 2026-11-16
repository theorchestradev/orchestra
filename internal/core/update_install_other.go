//go:build !darwin

package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func updateAsset(version string) string { return "Orchestra-" + version + "-linux-x86_64.tar.gz" }

// installedPath returns the running binary, which releases replace in place.
func installedPath(executable string) (string, error) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return "", errors.New("Releases include Linux builds for x86_64 only.")
	}
	return executable, nil
}

func unpackUpdate(ctx context.Context, archive, dir, version string) (string, error) {
	if _, err := systemCommand(ctx, "tar", "-xzf", archive, "-C", dir); err != nil {
		return "", err
	}
	binary := filepath.Join(dir, "Orchestra-"+version+"-linux-x86_64", "orchestra")
	if info, err := os.Stat(binary); err != nil || !info.Mode().IsRegular() {
		return "", errors.New("The download does not contain the orchestra binary.")
	}
	return binary, nil
}

// verifyUpdate checks that the binary runs and reports the expected version. Linux
// builds are not signed, so the checksum is their integrity check.
func verifyUpdate(ctx context.Context, path, _, version string) error {
	out, err := systemCommand(ctx, path, "--version")
	if err != nil || strings.TrimSpace(string(out)) != "Orchestra "+version {
		return fmt.Errorf("The update is not version %s.", version)
	}
	return nil
}

// relaunchCommand starts the binary again once this process has exited.
func relaunchCommand(pid int, path string) *exec.Cmd {
	return exec.Command("/bin/sh", "-c", `while kill -0 "$1" 2>/dev/null; do sleep 0.2; done; exec "$2" >/dev/null 2>&1`, "sh", strconv.Itoa(pid), path)
}
