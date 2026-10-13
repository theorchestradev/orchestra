//go:build darwin

package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func updateAsset(version string) string { return "Orchestra-" + version + "-macos-universal.zip" }

// installedPath returns the app bundle that contains the running executable.
func installedPath(executable string) (string, error) {
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(executable)))
	if filepath.Base(filepath.Dir(executable)) != "MacOS" || filepath.Ext(bundle) != ".app" {
		return "", errors.New("Orchestra is not running from an app bundle.")
	}
	return bundle, nil
}

// unpackUpdate extracts the app with ditto, which keeps its signature and stapled ticket.
func unpackUpdate(ctx context.Context, archive, dir, _ string) (string, error) {
	if _, err := systemCommand(ctx, "/usr/bin/ditto", "-x", "-k", archive, dir); err != nil {
		return "", err
	}
	app := filepath.Join(dir, "Orchestra.app")
	if info, err := os.Stat(app); err != nil || !info.IsDir() {
		return "", errors.New("The download does not contain Orchestra.app.")
	}
	return app, nil
}

// verifyUpdate accepts only a valid, notarized app signed by the running app's team.
func verifyUpdate(ctx context.Context, path, current, version string) error {
	if _, err := systemCommand(ctx, "/usr/bin/codesign", "--verify", "--deep", "--strict", path); err != nil {
		return fmt.Errorf("The update's code signature is not valid: %w", err)
	}
	want, err := teamID(ctx, current)
	if err != nil {
		return err
	}
	if want == "" {
		return errors.New("This copy of Orchestra is not signed by a developer, so it cannot verify updates.")
	}
	if got, err := teamID(ctx, path); err != nil || got != want {
		return errors.New("The update is not signed by the same developer as this copy of Orchestra.")
	}
	if _, err := systemCommand(ctx, "/usr/sbin/spctl", "--assess", "--type", "execute", path); err != nil {
		return errors.New("The update is not notarized by Apple.")
	}
	out, err := systemCommand(ctx, "/usr/bin/plutil", "-extract", "CFBundleShortVersionString", "raw", filepath.Join(path, "Contents", "Info.plist"))
	if err != nil || strings.TrimSpace(string(out)) != version {
		return fmt.Errorf("The update is not version %s.", version)
	}
	return nil
}

// teamID returns the signing team, or "" for ad hoc and unsigned code.
func teamID(ctx context.Context, path string) (string, error) {
	out, err := systemCommand(ctx, "/usr/bin/codesign", "-dv", "--verbose=2", path)
	if err != nil {
		if strings.Contains(string(out), "not signed") {
			return "", nil
		}
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if team, ok := strings.CutPrefix(line, "TeamIdentifier="); ok && team != "not set" {
			return team, nil
		}
	}
	return "", nil
}

// relaunchCommand opens the app again once this process has exited.
func relaunchCommand(pid int, path string) *exec.Cmd {
	return exec.Command("/bin/sh", "-c", `while kill -0 "$1" 2>/dev/null; do sleep 0.2; done; exec /usr/bin/open -n "$2"`, "sh", strconv.Itoa(pid), path)
}
