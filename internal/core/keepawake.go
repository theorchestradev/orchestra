package core

import (
	"syscall"
)

// keepAwakeCommand returns a process that keeps the computer from sleeping while it runs
// and exits on its own if Orchestra does. Tests replace it.
var keepAwakeCommand = systemKeepAwakeCommand

// updateKeepAwakeLocked keeps the computer awake while the preference is on and a session runs.
func (s *Store) updateKeepAwakeLocked() {
	want := false
	if s.state.Preferences.KeepAwake {
		for _, p := range s.state.Workspaces {
			for _, sess := range p.Sessions {
				want = want || sess.Running
			}
		}
	}
	if want == (s.keepAwake != nil) || (want && s.keepAwakeUnavailable) {
		return
	}
	if !want {
		s.stopKeepAwakeLocked()
		return
	}
	cmd := keepAwakeCommand()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		s.log.Warn("keep awake failed", "error", err)
		s.keepAwakeUnavailable = true
		return
	}
	s.keepAwake = cmd
	go cmd.Wait()
}

func (s *Store) stopKeepAwakeLocked() {
	if s.keepAwake == nil {
		return
	}
	_ = syscall.Kill(-s.keepAwake.Process.Pid, syscall.SIGKILL)
	s.keepAwake = nil
}
