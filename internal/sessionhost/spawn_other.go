//go:build !darwin

package sessionhost

import (
	"os/exec"
	"syscall"
)

// startHost runs executable as the host in its own session, so it keeps running after the app quits.
func startHost(executable, dir string, env []string) error {
	cmd := exec.Command(executable, Flag, dir)
	cmd.Env = env
	cmd.Dir = "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Reap the host if it exits while the app is open, for example when another host won.
	go func() { _ = cmd.Wait() }()
	return nil
}

// responsiblePID is 0 where no process answers for another one's privacy permissions.
func responsiblePID(int) int { return 0 }
