//go:build !windows

package core

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// extraToolPath follows PATH when the app looks for gh and git, which a desktop launch
// without a login shell may not see.
const extraToolPath = ":/opt/homebrew/bin:/usr/local/bin"

// errLocked reports that another process holds a lock file.
var errLocked = errors.New("The file is locked by another process.")

// lockFile takes an exclusive lock that lasts until f is closed, without waiting.
func lockFile(f *os.File) error {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return errLocked
	}
	return err
}

// setProcessGroup gives cmd its own process group, so killProcessGroup stops its children too.
func setProcessGroup(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func killProcessGroup(cmd *exec.Cmd) error {
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

// hideConsole does nothing: helper commands show no window on Unix.
func hideConsole(*exec.Cmd) {}

// writableDirectory reports whether the app can replace files in dir.
func writableDirectory(dir string) bool { return syscall.Access(dir, 2) == nil }

// loginShellArgs runs script through the user's login shell, with its rc files, so agents
// are found the way the user's own terminal finds them.
func loginShellArgs(script string) ([]string, error) {
	return []string{loginShell(), "-l", "-i", "-c", script}, nil
}
