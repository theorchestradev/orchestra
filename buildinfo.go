package main

import "fmt"

// Set at build time with -ldflags "-X main.version=... -X main.buildWorktree=... -X main.buildBranch=... -X main.buildCommit=...".
// Release builds set version from the v* tag; local builds keep "dev".
var (
	version       = "dev"
	buildWorktree = "dev"
	buildBranch   = ""
	buildCommit   = "unknown"
	// Injected only when the official release workflow opts into release telemetry.
	buildTelemetryKey  = ""
	buildTelemetryHost = "https://eu.i.posthog.com"
)

func aboutMessage() string {
	if version != "dev" {
		return fmt.Sprintf("Version: %s\nCommit: %s", version, buildCommit)
	}
	worktree := buildWorktree
	if buildBranch != "" {
		worktree += " (" + buildBranch + ")"
	}
	return fmt.Sprintf("Version: dev\nWorktree: %s\nCommit: %s", worktree, buildCommit)
}
