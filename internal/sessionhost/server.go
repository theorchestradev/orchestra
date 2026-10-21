package sessionhost

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// Options tune when an unused host exits. Zero values use the defaults.
type Options struct {
	// Idle is how long the host waits without clients or running sessions before it exits.
	Idle time.Duration
	// Retain keeps a session that exited without a client for this long, so Orchestra can
	// still show how it ended when it opens again.
	Retain time.Duration
}

type server struct {
	mu       sync.Mutex
	sessions map[string]*session
	clients  map[*client]bool
	version  string
	log      *slog.Logger
	options  Options
	// quiet is when the host last had a client or a running session.
	quiet time.Time
}
type session struct {
	id       string
	meta     json.RawMessage
	cmd      *exec.Cmd
	pty      *os.File
	started  time.Time
	buffer   []byte
	offset   int64
	exit     *Exit
	exited   time.Time
	done     chan struct{}
	stopped  bool
	watchers map[*client]bool
}
type client struct {
	conn net.Conn
	out  chan Message
	once sync.Once
}

// Run serves the host in dir until it has been unused for Options.Idle. It returns nil
// at once when another host already serves dir.
func Run(dir, version string, log *slog.Logger, options Options) error {
	if options.Idle == 0 {
		options.Idle = 30 * time.Second
	}
	if options.Retain == 0 {
		options.Retain = time.Hour
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(lockPath(dir), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			log.Info("session host already running")
			return nil
		}
		return err
	}
	socket := SocketPath(dir)
	_ = os.Remove(socket)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer os.Remove(socket)
	if err = os.Chmod(socket, 0600); err != nil {
		listener.Close()
		return err
	}
	// The app that started the host may quit at any time; only an explicit stop ends sessions.
	signal.Ignore(syscall.SIGHUP)
	s := &server{sessions: map[string]*session{}, clients: map[*client]bool{}, version: version, log: log, options: options, quiet: time.Now()}
	log.Info("session host started", "pid", os.Getpid(), "responsible", responsiblePID(os.Getpid()))
	go s.exitWhenUnused(listener)
	for {
		conn, err := listener.Accept()
		if err != nil {
			break
		}
		go s.serve(conn)
	}
	log.Info("session host stopped")
	return nil
}

func (s *server) exitWhenUnused(listener net.Listener) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		busy := len(s.clients) > 0
		for id, sess := range s.sessions {
			if sess.exit == nil {
				busy = true
			} else if time.Since(sess.exited) > s.options.Retain {
				delete(s.sessions, id)
			} else {
				busy = true
			}
		}
		if busy {
			s.quiet = time.Now()
		}
		unused := time.Since(s.quiet) > s.options.Idle
		s.mu.Unlock()
		if unused {
			listener.Close()
			return
		}
	}
}

func (s *server) serve(conn net.Conn) {
	c := &client{conn: conn, out: make(chan Message, 1024)}
	s.mu.Lock()
	s.clients[c] = true
	s.mu.Unlock()
	go c.write(s.log)
	reader := bufio.NewReaderSize(conn, 64*1024)
	decoder := json.NewDecoder(reader)
	for {
		var m Message
		if err := decoder.Decode(&m); err != nil {
			break
		}
		reply := s.handle(c, m)
		reply.Seq = m.Seq
		s.send(c, reply)
	}
	c.close()
	s.mu.Lock()
	delete(s.clients, c)
	for _, sess := range s.sessions {
		delete(sess.watchers, c)
	}
	s.quiet = time.Now()
	s.mu.Unlock()
}

func (c *client) write(log *slog.Logger) {
	encoder := json.NewEncoder(c.conn)
	for m := range c.out {
		_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := encoder.Encode(m); err != nil {
			log.Debug("session host write failed", "error", err)
			c.conn.Close()
			for range c.out {
			}
			return
		}
	}
}
func (c *client) close() {
	c.once.Do(func() {
		c.conn.Close()
		close(c.out)
	})
}

// send queues m without blocking; a client that stops reading is disconnected and
// catches up from the buffer when it attaches again.
func (s *server) send(c *client, m Message) {
	defer func() { _ = recover() }() // The client closed while m was being queued.
	select {
	case c.out <- m:
	default:
		s.log.Warn("session host client too slow; disconnecting")
		c.close()
	}
}

func (s *server) handle(c *client, m Message) Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m.Op == "hello" {
		return Message{Protocol: Protocol, Version: s.version}
	}
	if m.Op == "list" {
		infos := []Info{}
		for _, sess := range s.sessions {
			infos = append(infos, Info{ID: sess.id, PID: sess.cmd.Process.Pid, Started: sess.started, End: sess.offset, Meta: sess.meta, Exit: sess.exit})
		}
		return Message{Sessions: infos}
	}
	if m.Op == "spawn" {
		return s.spawnLocked(c, m)
	}
	sess := s.sessions[m.ID]
	if sess == nil {
		return Message{Error: "Session is not running."}
	}
	switch m.Op {
	case "attach":
		// The reply and later events share c.out, so the client sees no gap or overlap.
		start := min(max(m.Start, sess.offset-int64(len(sess.buffer))), sess.offset)
		reply := Message{ID: sess.id, Data: sess.buffer[len(sess.buffer)-int(sess.offset-start):], Start: start, End: sess.offset, PID: sess.cmd.Process.Pid, Meta: sess.meta, Exit: sess.exit}
		if sess.exit == nil {
			sess.watchers[c] = true
		}
		return reply
	case "write":
		if sess.exit != nil {
			return Message{Error: "Session is not running."}
		}
		if _, err := sess.pty.Write(m.Data); err != nil {
			return Message{Error: err.Error()}
		}
	case "resize":
		if sess.exit == nil && !sess.stopped {
			if err := pty.Setsize(sess.pty, &pty.Winsize{Cols: uint16(m.Cols), Rows: uint16(m.Rows)}); err != nil {
				return Message{Error: err.Error()}
			}
		}
	case "signal":
		s.signalLocked(sess, syscall.Signal(m.Signal))
	case "terminate":
		s.terminateLocked(sess)
	case "release":
		s.terminateLocked(sess)
		delete(s.sessions, sess.id)
	default:
		return Message{Error: fmt.Sprintf("Unknown session host request %q.", m.Op)}
	}
	return Message{}
}

func (s *server) spawnLocked(c *client, m Message) Message {
	if old := s.sessions[m.ID]; old != nil {
		if old.exit == nil {
			return Message{Error: "Session is already running."}
		}
		delete(s.sessions, m.ID)
	}
	if len(m.Argv) == 0 {
		return Message{Error: "Missing command."}
	}
	cmd := exec.Command(m.Argv[0], m.Argv[1:]...)
	cmd.Dir = m.Dir
	cmd.Env = m.Env
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(m.Cols), Rows: uint16(m.Rows)})
	if err != nil {
		return Message{Error: err.Error()}
	}
	sess := &session{id: m.ID, meta: m.Meta, cmd: cmd, pty: f, started: time.Now(), done: make(chan struct{}), watchers: map[*client]bool{c: true}}
	s.sessions[m.ID] = sess
	s.log.Info("session process started", "session", m.ID, "pid", cmd.Process.Pid)
	go s.read(sess)
	return Message{ID: m.ID, PID: cmd.Process.Pid}
}

func (s *server) read(sess *session) {
	buf := make([]byte, 32768)
	for {
		n, err := sess.pty.Read(buf)
		if n > 0 {
			s.mu.Lock()
			start := sess.offset
			sess.buffer = append(sess.buffer, buf[:n]...)
			sess.offset += int64(n)
			if len(sess.buffer) > bufferLimit {
				sess.buffer = append([]byte{}, sess.buffer[len(sess.buffer)-bufferLimit:]...)
			}
			event := Message{Op: "output", ID: sess.id, Data: append([]byte{}, buf[:n]...), Start: start, End: sess.offset}
			for c := range sess.watchers {
				s.send(c, event)
			}
			s.mu.Unlock()
		}
		if err != nil {
			break
		}
	}
	err := sess.cmd.Wait()
	exit := &Exit{Code: -1}
	if state := sess.cmd.ProcessState; state == nil {
		exit.Error = err.Error()
	} else if status, ok := state.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		exit.Signal = status.Signal().String()
	} else {
		exit.Code = state.ExitCode()
	}
	_ = sess.pty.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	sess.exit = exit
	sess.exited = time.Now()
	close(sess.done)
	s.log.Info("session process exited", "session", sess.id, "code", exit.Code, "signal", exit.Signal)
	event := Message{Op: "exit", ID: sess.id, End: sess.offset, Exit: exit}
	for c := range sess.watchers {
		s.send(c, event)
	}
	sess.watchers = map[*client]bool{}
}

// terminateLocked stops a session like Orchestra always has: SIGTERM to its process
// group, then SIGKILL if it is still running a second later.
func (s *server) terminateLocked(sess *session) {
	if sess.stopped || sess.exit != nil {
		return
	}
	sess.stopped = true
	s.signalLocked(sess, syscall.SIGTERM)
	if err := sess.pty.Close(); err != nil {
		s.log.Debug("terminal close failed", "session", sess.id, "error", err)
	}
	go func() {
		select {
		case <-sess.done:
		case <-time.After(time.Second):
			s.log.Info("session did not stop after SIGTERM; sending SIGKILL", "session", sess.id)
			s.mu.Lock()
			s.signalLocked(sess, syscall.SIGKILL)
			s.mu.Unlock()
		}
	}()
}

// signalLocked sends sig to the session's process group. macOS returns EPERM instead of
// ESRCH when the group's only member is an exited leader that has not been reaped yet.
func (s *server) signalLocked(sess *session, sig syscall.Signal) {
	if sess.exit != nil {
		return
	}
	if err := syscall.Kill(-sess.cmd.Process.Pid, sig); errors.Is(err, syscall.ESRCH) || runtime.GOOS == "darwin" && errors.Is(err, syscall.EPERM) {
		s.log.Debug("session signal failed", "session", sess.id, "signal", sig.String(), "error", err)
	} else if err != nil {
		s.log.Warn("session signal failed", "session", sess.id, "signal", sig.String(), "error", err)
	}
}
