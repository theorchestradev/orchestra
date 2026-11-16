package core

import (
	"errors"
	"fmt"
	"log/slog"

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
