//go:build darwin

package sessionhost

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// Agents in the host must answer to the host for privacy permissions, never to the app
// that started it, so their attribution does not change when the app quits.
func TestStartedHostAnswersForItsSessions(t *testing.T) {
	dir := hostDir(t)
	exe, _ := os.Executable()
	c, err := Start(dir, exe, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	p, err := c.Spawn("tcc", []string{"/bin/sh", "-c", "printf 'up\\r\\n'; exec sleep 30"}, "/", os.Environ(), 80, 24, nil)
	if err != nil {
		t.Fatal(err)
	}
	readUntil(t, p, "up")
	defer p.Release()
	out, err := exec.Command("/bin/ps", "-o", "ppid=", "-p", strconv.Itoa(p.PID)).Output()
	if err != nil {
		t.Fatal(err)
	}
	host, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	if host == os.Getpid() {
		t.Fatal("session ran in the test process instead of a host")
	}
	if got := responsiblePID(host); got != host {
		t.Fatalf("host %d answers to %d, not itself", host, got)
	}
	if got := responsiblePID(p.PID); got != host {
		t.Fatalf("session %d answers to %d, not host %d", p.PID, got, host)
	}
}
