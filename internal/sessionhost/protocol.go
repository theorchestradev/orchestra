// Package sessionhost runs session processes in a background host that outlives the app
// window. The host owns each process, its PTY and its recent output; Store in
// internal/core keeps every other piece of session state and reconnects to the host
// when Orchestra opens again.
package sessionhost

import (
	"encoding/json"
	"path/filepath"
	"time"
)

// Protocol changes whenever a message changes meaning. A client only reconnects to a
// host that speaks the same protocol, so a host started by an older build keeps its
// sessions until they are stopped.
const Protocol = 1

// bufferLimit is how much recent output the host keeps for each session.
const bufferLimit = 4 * 1024 * 1024

// Message is one JSON line in either direction. A request carries Seq and Op, its reply
// the same Seq, and events from the host Op "output" or "exit" without a Seq.
type Message struct {
	Seq      int64           `json:"seq,omitempty"`
	Op       string          `json:"op,omitempty"`
	ID       string          `json:"id,omitempty"`
	Error    string          `json:"error,omitempty"`
	Protocol int             `json:"protocol,omitempty"`
	Version  string          `json:"version,omitempty"`
	Argv     []string        `json:"argv,omitempty"`
	Dir      string          `json:"dir,omitempty"`
	Env      []string        `json:"env,omitempty"`
	Cols     int             `json:"cols,omitempty"`
	Rows     int             `json:"rows,omitempty"`
	Signal   int             `json:"signal,omitempty"`
	Meta     json.RawMessage `json:"meta,omitempty"`
	Data     []byte          `json:"data,omitempty"`
	Start    int64           `json:"start,omitempty"`
	End      int64           `json:"end,omitempty"`
	PID      int             `json:"pid,omitempty"`
	Exit     *Exit           `json:"exit,omitempty"`
	Sessions []Info          `json:"sessions,omitempty"`
}

// Exit describes how a session's process ended.
type Exit struct {
	Code   int    `json:"code"`             // -1 when a signal ended it or the exit is unknown.
	Signal string `json:"signal,omitempty"` // The signal that ended it.
	Error  string `json:"error,omitempty"`  // Set when the host could not wait for the process.
}

// Info describes a session the host holds. Meta is the client's own data, returned unchanged.
type Info struct {
	ID      string          `json:"id"`
	PID     int             `json:"pid"`
	Started time.Time       `json:"started"`
	End     int64           `json:"end"`
	Meta    json.RawMessage `json:"meta,omitempty"`
	Exit    *Exit           `json:"exit,omitempty"`
}

// SocketPath and lockPath are the host's files in its folder.
func SocketPath(dir string) string { return filepath.Join(dir, "host.sock") }
func lockPath(dir string) string   { return filepath.Join(dir, "host.lock") }
