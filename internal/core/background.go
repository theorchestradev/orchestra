package core

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/theorchestradev/orchestra/internal/sessionhost"
)

// hostMeta is what Store needs about a running session after Orchestra reopens. The
// session host keeps it beside the process and returns it unchanged.
type hostMeta struct {
	LaunchID       string    `json:"launchID"`
	AgentDirectory string    `json:"agentDirectory,omitempty"`
	Launched       time.Time `json:"launched"`
	Resumed        bool      `json:"resumed,omitempty"`
}

// openBackgroundLocked prepares sessions that outlive the app. The runtime folder moves
// beside the workspace, because running agents keep its paths in their environment,
// and Store connects to a host that is still running sessions.
func (s *Store) openBackgroundLocked() (map[string]sessionhost.Info, error) {
	dir := filepath.Dir(s.path)
	run := filepath.Join(dir, "run")
	if err := os.MkdirAll(run, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(run, 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(run, "app.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errors.New("Another Orchestra is open with this workspace.")
		}
		return nil, err
	}
	s.appLock, s.runtimeDir, s.hostDir = lock, run, filepath.Join(dir, "host")
	live := map[string]sessionhost.Info{}
	// listed is true when Store knows every session the host runs, or that none runs.
	listed := false
	client, err := sessionhost.Connect(s.hostDir)
	var incompatible *sessionhost.IncompatibleError
	switch {
	case errors.As(err, &incompatible):
		s.log.Warn("session host from another version", "version", incompatible.Version)
		s.hostIncompatible = true
		s.state.Error = err.Error()
	case errors.Is(err, sessionhost.ErrNotRunning):
		listed = true
	case err != nil:
		s.log.Warn("session host unavailable", "error", err)
	default:
		s.host = client
		infos, err := client.List()
		if err != nil {
			s.log.Warn("session host list failed", "error", err)
		}
		listed = err == nil
		for _, info := range infos {
			live[info.ID] = info
		}
	}
	if !listed {
		// The host may still run sessions whose agents use these files.
		return live, nil
	}
	// Session files from earlier launches would report a stale status to a restarted session.
	entries, _ := os.ReadDir(run)
	for _, entry := range entries {
		id, _, _ := strings.Cut(entry.Name(), ".")
		if _, ok := live[id]; !ok && entry.Name() != "app.lock" {
			_ = os.RemoveAll(filepath.Join(run, entry.Name()))
		}
	}
	return live, nil
}

// reattachLocked reconnects the sessions still in the host. A session that ended while
// Orchestra was closed is recorded the same way as one that ends while it is open.
func (s *Store) reattachLocked(live map[string]sessionhost.Info) {
	for id, info := range live {
		var meta hostMeta
		_ = json.Unmarshal(info.Meta, &meta)
		if _, sess, err := s.sessionLocked(id); err != nil || meta.LaunchID == "" || sess.LaunchID != meta.LaunchID {
			// The session was closed, its workspace removed or it was started again elsewhere;
			// the process must not keep running unseen.
			s.log.Info("stopping unknown background session", "session", id)
			if err = s.host.Release(id); err != nil {
				s.log.Warn("background session stop failed", "session", id, "error", err)
			}
			continue
		}
		process, snapshot, err := s.host.Attach(id)
		if err != nil {
			s.log.Warn("session reconnect failed", "session", id, "error", err)
			continue
		}
		statusFile, idFile := sessionFiles(s.runtimeDir, id)
		t := &TerminalSession{statusFile: statusFile, idFile: idFile, done: make(chan struct{}), agentDirectory: meta.AgentDirectory, launched: meta.Launched, resumed: meta.Resumed, background: true, log: s.log.With("session", id)}
		t.proc = &hostProcess{process, t.log}
		t.buffer, t.offset = snapshot.Data, snapshot.End
		t.updateBracketedPaste()
		_, sess, _ := s.sessionLocked(id)
		sess.Running, sess.Restored, sess.Background, sess.Activity = true, false, true, "unknown"
		s.terminals[id] = t
		s.collaboration.restoreTokenLocked(id)
		t.log.Info("session reconnected", "agent", sess.Agent, "exited", snapshot.Exit != nil)
		go s.readTerminal(id, t)
		go s.monitorTerminal(id, t)
	}
}

// startProcessLocked runs a session's command in the host when the user keeps sessions
// running after quitting, and in the app otherwise or when the host cannot be reached.
// Once the host answered, its error stands: it may still run this session, and a second
// agent in the same folder must never start beside it.
func (s *Store) startProcessLocked(sess *Session, t *TerminalSession, argv []string, dir string, env []string, cols, rows int) error {
	if s.hostDir != "" && !s.hostIncompatible && s.state.Preferences.KeepSessionsRunning {
		launch := uuid.NewString()
		meta, _ := json.Marshal(hostMeta{LaunchID: launch, AgentDirectory: t.agentDirectory, Launched: t.launched, Resumed: t.resumed})
		var err error
		// A connection that closed before the host replied is retried once with a new one.
		for attempt := 0; attempt < 2; attempt++ {
			var client *sessionhost.Client
			if client, err = s.hostLocked(); err != nil {
				break
			}
			var process *sessionhost.Process
			if process, err = client.Spawn(sess.ID, argv, dir, env, cols, rows, meta); err == nil {
				t.proc, t.background = &hostProcess{process, t.log}, true
				sess.LaunchID = launch
				return nil
			}
			if !errors.Is(err, sessionhost.ErrClosed) {
				t.log.Warn("session host refused the session", "error", err)
				if errors.Is(err, sessionhost.ErrRunning) {
					return errors.New("This session is still running in the background. Quit and reopen Orchestra to reconnect it.")
				}
				return err
			}
			client.Close()
			s.host = nil
		}
		t.log.Warn("session host unavailable; the session will stop with the app", "error", err)
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	p, err := startLocalProcess(cmd, cols, rows, t.log)
	if err != nil {
		return err
	}
	t.proc = p
	sess.LaunchID = ""
	return nil
}

// hostLocked connects to the host, starting it when none runs.
func (s *Store) hostLocked() (*sessionhost.Client, error) {
	if s.host != nil {
		select {
		case <-s.host.Done:
			s.host = nil
		default:
			return s.host, nil
		}
	}
	client, err := sessionhost.Start(s.hostDir, s.executable, os.Environ())
	if err != nil {
		return nil, err
	}
	s.host = client
	s.log.Info("session host connected", "version", client.Version)
	return client, nil
}

func (s *Store) closeBackgroundLocked() {
	if s.host != nil {
		s.host.Close()
		s.host = nil
	}
	if s.appLock != nil {
		s.appLock.Close()
		s.appLock = nil
	}
}

// StopSessionsOnQuit makes the next Shutdown stop the sessions that would keep running.
func (s *Store) StopSessionsOnQuit(stop bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopOnQuit = stop
}
