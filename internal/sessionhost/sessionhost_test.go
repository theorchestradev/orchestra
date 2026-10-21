package sessionhost

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

// hostDir is short because Unix socket paths are limited to about 100 bytes.
func hostDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "osh-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func serveHost(t *testing.T, dir string, options Options) chan error {
	t.Helper()
	stopped := make(chan error, 1)
	go func() { stopped <- Run(dir, "test", slog.New(slog.DiscardHandler), options) }()
	until := time.Now().Add(5 * time.Second)
	for {
		c, err := Connect(dir)
		if err == nil {
			c.Close()
			return stopped
		}
		if time.Now().After(until) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func connect(t *testing.T, dir string) *Client {
	t.Helper()
	c, err := Connect(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func readUntil(t *testing.T, p *Process, want string) string {
	t.Helper()
	var out bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for !strings.Contains(out.String(), want) {
			n, err := p.Read(buf)
			out.Write(buf[:n])
			if err != nil {
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("output %q never contained %q", out.String(), want)
	}
	return out.String()
}

func TestSessionSurvivesClientAndReattaches(t *testing.T) {
	dir := hostDir(t)
	serveHost(t, dir, Options{})
	first := connect(t, dir)
	script := "stty -echo; printf 'ready\\r\\n'; while IFS= read -r line; do printf 'got %s\\r\\n' \"$line\"; done"
	p, err := first.Spawn("one", []string{"/bin/sh", "-c", script}, "/", os.Environ(), 80, 24, []byte(`{"launched":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	readUntil(t, p, "ready")
	if err = p.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, p, "got hello")
	first.Close()
	if exit := p.Wait(); exit != nil {
		t.Fatalf("disconnect reported exit %+v", exit)
	}

	second := connect(t, dir)
	sessions, err := second.List()
	if err != nil || len(sessions) != 1 || sessions[0].ID != "one" || sessions[0].Exit != nil || string(sessions[0].Meta) != `{"launched":"x"}` {
		t.Fatalf("sessions %+v, %v", sessions, err)
	}
	p, snapshot, err := second.Attach("one")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(snapshot.Data), "ready") || !strings.Contains(string(snapshot.Data), "got hello") || snapshot.Start != 0 || snapshot.End != int64(len(snapshot.Data)) {
		t.Fatalf("snapshot %q [%d, %d)", snapshot.Data, snapshot.Start, snapshot.End)
	}
	if err = p.Write([]byte("again\n")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, p, "got again")
	if err = p.Terminate(); err != nil {
		t.Fatal(err)
	}
	if _, err = io.ReadAll(p); err != nil {
		t.Fatal(err)
	}
	if exit := p.Wait(); exit == nil || exit.Signal == "" && exit.Code == 0 {
		t.Fatalf("terminated exit %+v", exit)
	}
}

func TestExitedSessionIsKeptUntilReleased(t *testing.T) {
	dir := hostDir(t)
	serveHost(t, dir, Options{})
	c := connect(t, dir)
	p, err := c.Spawn("done", []string{"/bin/sh", "-c", "printf 'bye\\r\\n'; exit 3"}, "/", os.Environ(), 80, 24, nil)
	if err != nil {
		t.Fatal(err)
	}
	if exit := p.Wait(); exit == nil || exit.Code != 3 {
		t.Fatalf("exit %+v", exit)
	}
	c.Close()

	c = connect(t, dir)
	p, snapshot, err := c.Attach("done")
	if err != nil || snapshot.Exit == nil || snapshot.Exit.Code != 3 || !strings.Contains(string(snapshot.Data), "bye") {
		t.Fatalf("snapshot %+v, %v", snapshot, err)
	}
	if exit := p.Wait(); exit == nil || exit.Code != 3 {
		t.Fatalf("attached exit %+v", exit)
	}
	if err = c.Release("done"); err != nil {
		t.Fatal(err)
	}
	if sessions, _ := c.List(); len(sessions) != 0 {
		t.Fatalf("released session kept: %+v", sessions)
	}
	if _, _, err = c.Attach("done"); err == nil {
		t.Fatal("attached a released session")
	}
}

func TestSignalReachesProcessGroup(t *testing.T) {
	dir := hostDir(t)
	serveHost(t, dir, Options{})
	c := connect(t, dir)
	p, err := c.Spawn("sig", []string{"/bin/sh", "-c", "printf 'up\\r\\n'; exec sleep 30"}, "/", os.Environ(), 80, 24, nil)
	if err != nil {
		t.Fatal(err)
	}
	readUntil(t, p, "up")
	if err = p.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	if exit := p.Wait(); exit == nil || exit.Signal == "" {
		t.Fatalf("exit %+v", exit)
	}
}

func TestUnusedHostExitsAndSecondHostDefers(t *testing.T) {
	dir := hostDir(t)
	stopped := serveHost(t, dir, Options{Idle: 200 * time.Millisecond})
	// A second host in the same folder returns at once and leaves the first one serving.
	if err := Run(dir, "test", slog.New(slog.DiscardHandler), Options{}); err != nil {
		t.Fatal(err)
	}
	c := connect(t, dir)
	p, err := c.Spawn("short", []string{"/bin/sh", "-c", "sleep 0.3"}, "/", os.Environ(), 80, 24, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.Wait()
	select {
	case err := <-stopped:
		t.Fatalf("host stopped with a client connected: %v", err)
	case <-time.After(1500 * time.Millisecond):
	}
	if err = c.Release("short"); err != nil {
		t.Fatal(err)
	}
	c.Close()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("unused host kept running")
	}
	if _, err = Connect(dir); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("connect after exit: %v", err)
	}
}

// The host ignores SIGHUP without passing that on: a session's processes must still stop
// when their terminal closes.
func TestSessionReceivesSIGHUP(t *testing.T) {
	dir := hostDir(t)
	serveHost(t, dir, Options{})
	c := connect(t, dir)
	p, err := c.Spawn("hup", []string{"/bin/sh", "-c", "kill -HUP $$; printf 'survived\\r\\n'"}, "/", os.Environ(), 80, 24, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(p)
	if exit := p.Wait(); exit == nil || exit.Signal != syscall.SIGHUP.String() || strings.Contains(string(out), "survived") {
		t.Fatalf("exit %+v, output %q", exit, out)
	}
}

// A session that stops reading its input holds up only its own writes.
func TestBlockedInputLeavesHostResponsive(t *testing.T) {
	dir := hostDir(t)
	serveHost(t, dir, Options{})
	c := connect(t, dir)
	stuck, err := c.Spawn("stuck", []string{"/bin/sh", "-c", "stty raw -echo; printf 'up\\r\\n'; exec sleep 30"}, "/", os.Environ(), 80, 24, nil)
	if err != nil {
		t.Fatal(err)
	}
	readUntil(t, stuck, "up")
	written := make(chan error, 1)
	go func() { written <- stuck.Write(bytes.Repeat([]byte("x"), 1024*1024)) }()
	select {
	case err = <-written:
		t.Fatalf("a terminal that never reads took 1 MiB of input: %v", err)
	case <-time.After(300 * time.Millisecond):
	}

	other, err := c.Spawn("other", []string{"/bin/sh", "-c", "stty -echo; while IFS= read -r line; do printf 'got %s\\r\\n' \"$line\"; done"}, "/", os.Environ(), 80, 24, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = other.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, other, "got hello")
	if sessions, err := c.List(); err != nil || len(sessions) != 2 {
		t.Fatalf("sessions %+v, %v", sessions, err)
	}

	if err = stuck.Terminate(); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-written:
		if err == nil {
			t.Fatal("the blocked write succeeded after the session stopped")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stopping the session left its write blocked")
	}
	other.Terminate()
}

func TestResizeReachesTerminal(t *testing.T) {
	dir := hostDir(t)
	serveHost(t, dir, Options{})
	c := connect(t, dir)
	p, err := c.Spawn("size", []string{"/bin/sh", "-c", "stty -echo; printf 'ready\\r\\n'; read -r line; stty size"}, "/", os.Environ(), 80, 24, nil)
	if err != nil {
		t.Fatal(err)
	}
	readUntil(t, p, "ready")
	if err = p.Resize(132, 43); err != nil {
		t.Fatal(err)
	}
	if err = p.Write([]byte("\n")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, p, "43 132")
}
