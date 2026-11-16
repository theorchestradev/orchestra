package sessionhost

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

// TestMain lets Start run the test binary as a separate host process.
func TestMain(m *testing.M) {
	if len(os.Args) > 2 && os.Args[1] == Flag {
		if err := Run(os.Args[2], "test", slog.New(slog.DiscardHandler), Options{Idle: 300 * time.Millisecond}); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
