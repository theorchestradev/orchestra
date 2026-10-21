package core

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/theorchestradev/orchestra/internal/sessionhost"
)

// terminalProcess is a session's process and PTY. The app runs it itself, or the session
// host runs it so it keeps running after Orchestra quits.
type terminalProcess interface {
	Read([]byte) (int, error)
	Write([]byte) error
	Resize(cols, rows int) error
	// Terminate sends SIGTERM to the process group, closes the PTY and sends SIGKILL if the
	// process still runs a second later.
	Terminate()
	// Wait reports how the process ended, once Read has returned an error.
	Wait() processExit
	// Close releases what is left after the exit was recorded.
	Close()
}

// processExit follows exec.Cmd.Wait: err is nil only for exit code 0, and code is -1 when
// a signal ended the process or its exit is unknown.
type processExit struct {
	code   int
	signal string
	err    error
}

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

// hostProcess runs in the session host and is followed through its connection.
type hostProcess struct {
	process *sessionhost.Process
	log     *slog.Logger
}

func (p *hostProcess) Read(b []byte) (int, error)  { return p.process.Read(b) }
func (p *hostProcess) Write(b []byte) error        { return p.process.Write(b) }
func (p *hostProcess) Resize(cols, rows int) error { return p.process.Resize(cols, rows) }
// Terminate also makes the host forget the session: Store asked for the exit, so there is
// nothing to reconcile if Orchestra quits before it is recorded.
func (p *hostProcess) Terminate() {
	if err := p.process.Release(); err != nil {
		p.log.Debug("session stop failed", "error", err)
	}
}
func (p *hostProcess) Wait() processExit {
	exit := p.process.Wait()
	switch {
	case exit == nil:
		return processExit{code: -1, err: errors.New("The session host connection was closed.")}
	case exit.Error != "":
		return processExit{code: -1, err: errors.New(exit.Error)}
	case exit.Signal != "":
		return processExit{code: -1, signal: exit.Signal, err: fmt.Errorf("signal: %s", exit.Signal)}
	case exit.Code != 0:
		return processExit{code: exit.Code, err: fmt.Errorf("exit status %d", exit.Code)}
	}
	return processExit{}
}

// Close makes the host forget the session once Store has recorded how it ended.
func (p *hostProcess) Close() {
	if err := p.process.Release(); err != nil {
		p.log.Debug("session release failed", "error", err)
	}
}
