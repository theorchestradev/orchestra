//go:build !windows

package core

import (
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// localProcess runs in the app and stops with it.
type localProcess struct {
	cmd    *exec.Cmd
	pty    *os.File
	exited chan struct{}
	log    *slog.Logger
}

func startLocalProcess(cmd *exec.Cmd, cols, rows int, log *slog.Logger) (*localProcess, error) {
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, err
	}
	return &localProcess{cmd: cmd, pty: f, exited: make(chan struct{}), log: log}, nil
}
func (p *localProcess) Read(b []byte) (int, error) { return p.pty.Read(b) }
func (p *localProcess) Write(b []byte) error {
	_, err := p.pty.Write(b)
	return err
}
func (p *localProcess) Resize(cols, rows int) error {
	return pty.Setsize(p.pty, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}
func (p *localProcess) Terminate() {
	p.signal(syscall.SIGTERM)
	if err := p.pty.Close(); err != nil {
		p.log.Debug("terminal close failed", "error", err)
	}
	go func() {
		select {
		case <-p.exited:
			return
		case <-time.After(time.Second):
			p.log.Info("session did not stop after SIGTERM; sending SIGKILL")
			p.signal(syscall.SIGKILL)
		}
	}()
}

// signal sends sig to the session's process group. A group that already exited is expected.
// macOS returns EPERM instead of ESRCH when the group's only member is an exited leader that
// readTerminal has not reaped yet.
func (p *localProcess) signal(sig syscall.Signal) {
	if err := syscall.Kill(-p.cmd.Process.Pid, sig); errors.Is(err, syscall.ESRCH) || runtime.GOOS == "darwin" && errors.Is(err, syscall.EPERM) {
		p.log.Debug("session signal failed", "signal", sig.String(), "error", err)
	} else if err != nil {
		p.log.Warn("session signal failed", "signal", sig.String(), "error", err)
	}
}
func (p *localProcess) Wait() processExit {
	err := p.cmd.Wait()
	close(p.exited)
	exit := processExit{code: -1, err: err}
	if state := p.cmd.ProcessState; state != nil {
		exit.code = state.ExitCode()
		if status, ok := state.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			exit.signal = status.Signal().String()
		}
	}
	return exit
}
func (p *localProcess) Close() {
	if err := p.pty.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		p.log.Debug("terminal close failed", "error", err)
	}
}
