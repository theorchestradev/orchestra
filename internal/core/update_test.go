package core

import (
	"cmp"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCompareSemver(t *testing.T) {
	// Ascending SemVer precedence, from the SemVer 2.0.0 specification.
	ordered := []string{"0.9.9", "1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta", "1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0", "1.0.1", "1.2.0", "10.0.0"}
	for i := range ordered {
		for j := range ordered {
			a, b := parseSemver(ordered[i]), parseSemver(ordered[j])
			if a == nil || b == nil {
				t.Fatalf("could not parse %q or %q", ordered[i], ordered[j])
			}
			if got, want := compareSemver(a, b), cmp.Compare(i, j); got != want {
				t.Errorf("compare(%s, %s) = %d, want %d", ordered[i], ordered[j], got, want)
			}
		}
	}
	if compareSemver(parseSemver("1.0.0+build.5"), parseSemver("1.0.0")) != 0 {
		t.Error("build metadata changed precedence")
	}
	for _, invalid := range []string{"", "dev", "1.0", "1.0.0.0", "v1.0.0", "01.0.0", "1.0.0-", "1.0.0-a..b", "1.-1.0"} {
		if parseSemver(invalid) != nil {
			t.Errorf("parsed invalid version %q", invalid)
		}
	}
}

func TestParseGitHubReleases(t *testing.T) {
	data := []byte(`[
		{"tag_name": "v0.3.0-beta.1", "name": "Orchestra 0.3.0-beta.1", "body": "Beta", "html_url": "https://github.com/theorchestradev/orchestra/releases/tag/v0.3.0-beta.1", "prerelease": true, "published_at": "2026-10-20T00:00:00Z"},
		{"tag_name": "v0.2.0", "name": "Orchestra 0.2.0", "body": "Notes", "html_url": "https://github.com/theorchestradev/orchestra/releases/tag/v0.2.0", "published_at": "2026-10-14T00:00:00Z"},
		{"tag_name": "v0.4.0", "draft": true},
		{"tag_name": "0.5.0"},
		{"tag_name": "nightly"}
	]`)
	releases, err := parseGitHubReleases(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []UpdateRelease{
		{Version: "0.3.0-beta.1", Name: "Orchestra 0.3.0-beta.1", Notes: "Beta", URL: "https://github.com/theorchestradev/orchestra/releases/tag/v0.3.0-beta.1", Prerelease: true, PublishedAt: "2026-10-20T00:00:00Z"},
		{Version: "0.2.0", Name: "Orchestra 0.2.0", Notes: "Notes", URL: "https://github.com/theorchestradev/orchestra/releases/tag/v0.2.0", PublishedAt: "2026-10-14T00:00:00Z"},
	}
	if len(releases) != len(want) || releases[0] != want[0] || releases[1] != want[1] {
		t.Fatalf("releases = %+v", releases)
	}
	if _, err := parseGitHubReleases([]byte(`{"message": "Not Found"}`)); err == nil {
		t.Fatal("accepted an error response")
	}
}

func TestNewestRelease(t *testing.T) {
	releases := []UpdateRelease{{Version: "0.1.0"}, {Version: "0.3.0-beta.1", Prerelease: true}, {Version: "0.2.0"}, {Version: "0.2.1-rc.1"}, {Version: "invalid"}}
	if got := newestRelease(releases, "0.1.0", false); got == nil || got.Version != "0.2.0" {
		t.Fatalf("stable channel: %+v", got)
	}
	if got := newestRelease(releases, "0.1.0", true); got == nil || got.Version != "0.3.0-beta.1" {
		t.Fatalf("beta channel: %+v", got)
	}
	if got := newestRelease(releases, "0.2.0", false); got != nil {
		t.Fatalf("offered a release that is not newer: %+v", got)
	}
	if got := newestRelease(releases, "0.3.0-beta.1", true); got != nil {
		t.Fatalf("offered the current pre-release: %+v", got)
	}
	// A beta user who turns Beta off is offered nothing until a newer stable release.
	if got := newestRelease(releases, "0.3.0-beta.1", false); got != nil {
		t.Fatalf("offered an older stable release: %+v", got)
	}
}

func TestUpdaterCheck(t *testing.T) {
	var prefs Preferences
	var calls atomic.Int32
	var fail atomic.Bool
	source := func(context.Context) ([]UpdateRelease, error) {
		calls.Add(1)
		if fail.Load() {
			return nil, errors.New("offline")
		}
		return []UpdateRelease{{Version: "0.2.0", Notes: "Notes", URL: "https://example.com/v0.2.0"}, {Version: "0.3.0-beta.1", Prerelease: true}}, nil
	}
	var mu sync.Mutex
	var emitted []UpdateStatus
	u := NewUpdater(UpdaterOptions{Current: "0.1.0", Source: source, Preferences: func() Preferences { return prefs }, Emit: func(s UpdateStatus) {
		mu.Lock()
		emitted = append(emitted, s)
		mu.Unlock()
	}})

	status := u.Check(context.Background(), false)
	if status.Checking || status.Error != "" || status.Available == nil || status.Available.Version != "0.2.0" || status.CheckedAt == "" {
		t.Fatalf("check result: %+v", status)
	}
	if len(emitted) != 2 || !emitted[0].Checking || emitted[1].Checking {
		t.Fatalf("emitted statuses: %+v", emitted)
	}
	prefs.BetaUpdates = true
	if status = u.Check(context.Background(), true); status.Available == nil || status.Available.Version != "0.3.0-beta.1" {
		t.Fatalf("beta check: %+v", status)
	}

	// Automatic failures keep the last result without an error; manual ones explain.
	fail.Store(true)
	if status = u.Check(context.Background(), false); status.Error != "" || status.Available == nil {
		t.Fatalf("automatic failure: %+v", status)
	}
	if status = u.Check(context.Background(), true); status.Error != "Could not check for updates: offline" || status.Available == nil {
		t.Fatalf("manual failure: %+v", status)
	}
	fail.Store(false)
	if status = u.Check(context.Background(), true); status.Error != "" {
		t.Fatalf("a successful check kept the error: %+v", status)
	}
}

func TestUpdaterSharesCheckInProgress(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	source := func(context.Context) ([]UpdateRelease, error) {
		calls.Add(1)
		<-release
		return []UpdateRelease{{Version: "0.2.0"}}, nil
	}
	u := NewUpdater(UpdaterOptions{Current: "0.1.0", Source: source, Preferences: func() Preferences { return Preferences{} }})
	var wg sync.WaitGroup
	results := make([]UpdateStatus, 3)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = u.Check(context.Background(), true)
		}()
	}
	for !u.Status().Checking {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(10 * time.Millisecond)
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("source called %d times", calls.Load())
	}
	for _, r := range results {
		if r.Available == nil || r.Checking {
			t.Fatalf("result: %+v", r)
		}
	}
}

func TestUpdaterDevelopmentBuild(t *testing.T) {
	u := NewUpdater(UpdaterOptions{Current: "dev", Source: func(context.Context) ([]UpdateRelease, error) {
		t.Fatal("development build queried the update source")
		return nil, nil
	}, Preferences: func() Preferences { return Preferences{} }})
	if status := u.Check(context.Background(), true); status.Supported || status.Error == "" {
		t.Fatalf("development check: %+v", status)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	u.Run(ctx)
}

func TestUpdatePreferences(t *testing.T) {
	s := fixtureStore(t)
	prefs := s.Snapshot().Preferences
	if prefs.UpdateChecksOff || prefs.BetaUpdates || prefs.SkippedUpdate != "" {
		t.Fatalf("new workspaces should check stable releases automatically: %+v", prefs)
	}
	prefs.UpdateChecksOff, prefs.BetaUpdates, prefs.SkippedUpdate = true, true, "0.2.0"
	if _, err := s.SetPreferences(prefs); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(s.path, s.executable, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	if reopened.Snapshot().Preferences != prefs {
		t.Fatalf("update preferences changed on reopen: %+v", reopened.Snapshot().Preferences)
	}
	invalid := prefs
	invalid.SkippedUpdate = "latest"
	if _, err := s.SetPreferences(invalid); err == nil {
		t.Fatal("accepted an invalid skipped version")
	}
}
