package main

import "testing"

func TestAboutMessage(t *testing.T) {
	defer func(v, w, b, c string) { version, buildWorktree, buildBranch, buildCommit = v, w, b, c }(version, buildWorktree, buildBranch, buildCommit)
	version, buildWorktree, buildBranch, buildCommit = "dev", "dev", "", "unknown"
	if got, want := aboutMessage(), "Version: dev\nWorktree: dev\nCommit: unknown"; got != want {
		t.Fatalf("aboutMessage() = %q, want %q", got, want)
	}
	buildWorktree, buildBranch, buildCommit = "orchestra", "main", "9be17d2-dirty"
	if got, want := aboutMessage(), "Version: dev\nWorktree: orchestra (main)\nCommit: 9be17d2-dirty"; got != want {
		t.Fatalf("aboutMessage() = %q, want %q", got, want)
	}
	version, buildCommit = "0.2.0", "9be17d2"
	if got, want := aboutMessage(), "Version: 0.2.0\nCommit: 9be17d2"; got != want {
		t.Fatalf("aboutMessage() = %q, want %q", got, want)
	}
}
