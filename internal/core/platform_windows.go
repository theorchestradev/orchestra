//go:build windows

package core

import (
	"errors"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// extraToolPath is empty: Windows installers put gh and git on PATH themselves.
const extraToolPath = ""

// errLocked reports that another process holds a lock file.
var errLocked = errors.New("The file is locked by another process.")

// errSessionsUnsupported is returned wherever a session or a login shell would start.
// Sessions need a ConPTY-backed terminalProcess and a PowerShell launch; see docs/windows.md.
var errSessionsUnsupported = errors.New("Terminal sessions are not available on Windows yet.")

// lockFile takes an exclusive lock on the first byte that lasts until f is closed, without waiting.
func lockFile(f *os.File) error {
	err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return errLocked
	}
	return err
}

// setProcessGroup starts cmd in its own process group without a console window.
// killProcessGroup ends only cmd itself: stopping a whole tree needs a job object, which
// the ConPTY session work will add.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW}
}
func killProcessGroup(cmd *exec.Cmd) error { return cmd.Process.Kill() }

// hideConsole keeps a helper command from opening a console window beside the app.
func hideConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}

// writableDirectory reports whether the app can create files in dir. Windows has no
// access(2); ACLs decide, so the check creates and removes a file.
func writableDirectory(dir string) bool {
	f, err := os.CreateTemp(dir, ".orchestra-write-")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

// loginShellArgs has no PowerShell form yet: the setup script and every agent command
// are written for a POSIX shell.
func loginShellArgs(string) ([]string, error) { return nil, errSessionsUnsupported }
