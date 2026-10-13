package core

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// logRotateSize is the size above which OpenLog keeps the log as orchestra.log.1 and starts a new one.
const logRotateSize = 5 * 1024 * 1024

// OpenLog opens dir/orchestra.log for JSON lines at info level, or debug when level is "debug".
// Records carry IDs, methods and errors only, never prompts, terminal data, Scratchpad text or credentials.
func OpenLog(dir, level string) (*slog.Logger, io.Closer, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, nil, err
	}
	path := filepath.Join(dir, "orchestra.log")
	if info, err := os.Stat(path); err == nil && info.Size() > logRotateSize {
		if err = os.Rename(path, path+".1"); err != nil {
			return nil, nil, err
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, nil, err
	}
	minimum := slog.LevelInfo
	if level == "debug" {
		minimum = slog.LevelDebug
	}
	return slog.New(slog.NewJSONHandler(f, &slog.HandlerOptions{Level: minimum})), f, nil
}
