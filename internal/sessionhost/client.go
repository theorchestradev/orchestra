package sessionhost

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// Flag starts the Orchestra executable as the host for the folder that follows it.
const Flag = "--orchestra-host"

var (
	// ErrNotRunning means no host serves the folder.
	ErrNotRunning = errors.New("The session host is not running.")
	// errClosed ends a process when the client disconnects from the host.
	errClosed = errors.New("The session host connection was closed.")
)

// IncompatibleError reports a host that speaks another protocol, usually one started
// by an earlier Orchestra version that is still running sessions.
type IncompatibleError struct {
	Protocol int
	Version  string
}

func (e *IncompatibleError) Error() string {
	return fmt.Sprintf("Sessions started by Orchestra %s are still running in the background and cannot be reconnected.", e.Version)
}

// Client is one connection to the host. Its methods are safe for concurrent use.
type Client struct {
	conn    net.Conn
	wmu     sync.Mutex
	encoder *json.Encoder
	mu      sync.Mutex
	seq     int64
	pending map[int64]chan Message
	procs   map[string]*Process
	closed  bool
	// Version is the Orchestra version that started the host.
	Version string
	// Done closes when the connection ends.
	Done chan struct{}
}

// Connect reaches the host serving dir; it returns ErrNotRunning when there is none.
func Connect(dir string) (*Client, error) {
	conn, err := net.DialTimeout("unix", SocketPath(dir), 2*time.Second)
	if err != nil {
		return nil, ErrNotRunning
	}
	c := &Client{conn: conn, encoder: json.NewEncoder(conn), pending: map[int64]chan Message{}, procs: map[string]*Process{}, Done: make(chan struct{})}
	go c.dispatch()
	hello, err := c.call(Message{Op: "hello", Protocol: Protocol})
	if err != nil {
		c.Close()
		return nil, err
	}
	if hello.Protocol != Protocol {
		c.Close()
		return nil, &IncompatibleError{hello.Protocol, hello.Version}
	}
	c.Version = hello.Version
	return c, nil
}

// Start connects to the host serving dir, starting executable as the host first when
// none runs. The host gets its own session, so it keeps running after the app quits.
func Start(dir, executable string, env []string) (*Client, error) {
	c, err := Connect(dir)
	if !errors.Is(err, ErrNotRunning) {
		return c, err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	cmd := exec.Command(executable, Flag, dir)
	cmd.Env = env
	cmd.Dir = "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	// Reap the host if it exits while the app is open, for example when another host won.
	go func() { _ = cmd.Wait() }()
	until := time.Now().Add(5 * time.Second)
	for {
		c, err = Connect(dir)
		if !errors.Is(err, ErrNotRunning) || time.Now().After(until) {
			return c, err
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// Close disconnects without stopping any session. Every attached Process ends with errClosed.
func (c *Client) Close() { c.conn.Close() }

func (c *Client) dispatch() {
	decoder := json.NewDecoder(bufio.NewReaderSize(c.conn, 64*1024))
	for {
		var m Message
		if err := decoder.Decode(&m); err != nil {
			break
		}
		c.mu.Lock()
		if m.Seq != 0 {
			if ch := c.pending[m.Seq]; ch != nil {
				delete(c.pending, m.Seq)
				ch <- m
			}
		} else if p := c.procs[m.ID]; p != nil {
			if m.Op == "output" {
				p.push(m.Data)
			} else if m.Op == "exit" {
				delete(c.procs, m.ID)
				p.end(m.Exit)
			}
		}
		c.mu.Unlock()
	}
	c.mu.Lock()
	c.closed = true
	for seq, ch := range c.pending {
		delete(c.pending, seq)
		close(ch)
	}
	for id, p := range c.procs {
		delete(c.procs, id)
		p.end(nil)
	}
	c.mu.Unlock()
	close(c.Done)
}

func (c *Client) call(m Message) (Message, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return Message{}, errClosed
	}
	c.seq++
	m.Seq = c.seq
	ch := make(chan Message, 1)
	c.pending[m.Seq] = ch
	c.mu.Unlock()
	c.wmu.Lock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	err := c.encoder.Encode(m)
	c.wmu.Unlock()
	if err != nil {
		c.conn.Close()
		return Message{}, errClosed
	}
	select {
	case reply, ok := <-ch:
		if !ok {
			return Message{}, errClosed
		}
		if reply.Error != "" {
			return reply, errors.New(reply.Error)
		}
		return reply, nil
	case <-time.After(10 * time.Second):
		c.mu.Lock()
		delete(c.pending, m.Seq)
		c.mu.Unlock()
		return Message{}, errors.New("The session host did not respond.")
	}
}

// List returns every session the host holds, running or exited.
func (c *Client) List() ([]Info, error) {
	reply, err := c.call(Message{Op: "list"})
	return reply.Sessions, err
}

// Spawn starts argv on a PTY in the host. meta is returned unchanged by List and Attach.
func (c *Client) Spawn(id string, argv []string, dir string, env []string, cols, rows int, meta json.RawMessage) (*Process, error) {
	p := c.register(id)
	reply, err := c.call(Message{Op: "spawn", ID: id, Argv: argv, Dir: dir, Env: env, Cols: cols, Rows: rows, Meta: meta})
	if err != nil {
		c.unregister(p)
		return nil, err
	}
	p.PID = reply.PID
	return p, nil
}

// Snapshot is a session's recent output when a client attaches.
type Snapshot struct {
	Data       []byte
	Start, End int64
	Meta       json.RawMessage
	// Exit is set when the session already ended; the Process then ends right away.
	Exit *Exit
}

// Attach follows a session the host holds. Output after the snapshot reaches the Process.
func (c *Client) Attach(id string) (*Process, Snapshot, error) {
	p := c.register(id)
	reply, err := c.call(Message{Op: "attach", ID: id})
	if err != nil {
		c.unregister(p)
		return nil, Snapshot{}, err
	}
	p.PID = reply.PID
	if reply.Exit != nil {
		c.unregister(p)
		p.end(reply.Exit)
	}
	return p, Snapshot{reply.Data, reply.Start, reply.End, reply.Meta, reply.Exit}, nil
}

func (c *Client) register(id string) *Process {
	p := &Process{client: c, id: id}
	p.cond = sync.NewCond(&p.mu)
	c.mu.Lock()
	if c.closed {
		p.end(nil)
	} else {
		c.procs[id] = p
	}
	c.mu.Unlock()
	return p
}
func (c *Client) unregister(p *Process) {
	c.mu.Lock()
	if c.procs[p.id] == p {
		delete(c.procs, p.id)
	}
	c.mu.Unlock()
}

// Process is a session followed through the host. Read returns its output in order and
// io.EOF once it has ended; Wait then reports how.
type Process struct {
	client *Client
	id     string
	PID    int
	mu     sync.Mutex
	cond   *sync.Cond
	chunks [][]byte
	exit   *Exit
	ended  bool
}

// push and end never block, so the client's dispatcher cannot stall behind a reader.
func (p *Process) push(data []byte) {
	p.mu.Lock()
	p.chunks = append(p.chunks, data)
	p.mu.Unlock()
	p.cond.Broadcast()
}
func (p *Process) end(exit *Exit) {
	p.mu.Lock()
	if !p.ended {
		p.ended = true
		p.exit = exit
	}
	p.mu.Unlock()
	p.cond.Broadcast()
}
func (p *Process) Read(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for len(p.chunks) == 0 && !p.ended {
		p.cond.Wait()
	}
	if len(p.chunks) == 0 {
		return 0, io.EOF
	}
	n := copy(b, p.chunks[0])
	if n < len(p.chunks[0]) {
		p.chunks[0] = p.chunks[0][n:]
	} else {
		p.chunks = p.chunks[1:]
	}
	return n, nil
}

// Wait blocks until the process ends. A nil Exit means the client disconnected first.
func (p *Process) Wait() *Exit {
	p.mu.Lock()
	defer p.mu.Unlock()
	for !p.ended {
		p.cond.Wait()
	}
	return p.exit
}
func (p *Process) Write(data []byte) error {
	_, err := p.client.call(Message{Op: "write", ID: p.id, Data: data})
	return err
}
func (p *Process) Resize(cols, rows int) error {
	_, err := p.client.call(Message{Op: "resize", ID: p.id, Cols: cols, Rows: rows})
	return err
}
func (p *Process) Signal(sig syscall.Signal) error {
	_, err := p.client.call(Message{Op: "signal", ID: p.id, Signal: int(sig)})
	return err
}

// Terminate stops the process: SIGTERM to its group, then SIGKILL a second later.
func (p *Process) Terminate() error {
	_, err := p.client.call(Message{Op: "terminate", ID: p.id})
	return err
}

// Release stops the process if needed and makes the host forget it.
func (p *Process) Release() error { return p.client.Release(p.id) }

// Release makes the host forget a session, stopping it first if it still runs.
func (c *Client) Release(id string) error {
	_, err := c.call(Message{Op: "release", ID: id})
	return err
}
