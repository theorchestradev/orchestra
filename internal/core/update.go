package core

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// UpdateRepository publishes the releases that update checks read, through gh or the
// public API.
const UpdateRepository = "theorchestradev/orchestra"

// The first automatic check waits so it does not compete with startup work.
const updateStartDelay, updateInterval = 10 * time.Second, 24 * time.Hour

type UpdateRelease struct {
	Version     string `json:"version"`
	Name        string `json:"name"`
	Notes       string `json:"notes"`
	URL         string `json:"url"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"publishedAt"`
}

type UpdateStatus struct {
	Current string `json:"current"`
	// Supported is false for development builds, which have no version to compare.
	Supported bool           `json:"supported"`
	Checking  bool           `json:"checking"`
	CheckedAt string         `json:"checkedAt,omitempty"`
	Available *UpdateRelease `json:"available,omitempty"`
	Error     string         `json:"error,omitempty"`
	// Installable is false when the app cannot replace itself; InstallNote says why and
	// the update dialog offers the release page instead.
	Installable bool   `json:"installable"`
	InstallNote string `json:"installNote,omitempty"`
	// Preparing is true while an update downloads and is verified.
	Preparing bool `json:"preparing"`
	// Staged is the verified version that quitting installs.
	Staged       string `json:"staged,omitempty"`
	InstallError string `json:"installError,omitempty"`
}

// UpdateSource lists published releases, newest first or in any order.
type UpdateSource func(context.Context) ([]UpdateRelease, error)

// GitHubReleases reads the releases through the user's GitHub CLI login and falls back
// to the public API without gh.
// Drafts and tags that are not SemVer are left out.
func GitHubReleases(ctx context.Context) ([]UpdateRelease, error) {
	return withFallback(func() ([]UpdateRelease, error) {
		home, _ := os.UserHomeDir()
		data, err := runCommandContext(ctx, home, "gh", nil, "api", "--hostname", "github.com", "repos/"+UpdateRepository+"/releases?per_page=30")
		if err != nil {
			return nil, err
		}
		return parseGitHubReleases(data)
	}, func() ([]UpdateRelease, error) { return publicReleases(ctx, nil) })
}

func parseGitHubReleases(data []byte) ([]UpdateRelease, error) {
	var releases []struct {
		TagName     string `json:"tag_name"`
		Name        string `json:"name"`
		Body        string `json:"body"`
		HTMLURL     string `json:"html_url"`
		Draft       bool   `json:"draft"`
		Prerelease  bool   `json:"prerelease"`
		PublishedAt string `json:"published_at"`
	}
	if err := json.Unmarshal(data, &releases); err != nil {
		return nil, err
	}
	result := []UpdateRelease{}
	for _, r := range releases {
		version := strings.TrimPrefix(r.TagName, "v")
		if r.Draft || !strings.HasPrefix(r.TagName, "v") || parseSemver(version) == nil {
			continue
		}
		result = append(result, UpdateRelease{Version: version, Name: r.Name, Notes: r.Body, URL: r.HTMLURL, Prerelease: r.Prerelease, PublishedAt: r.PublishedAt})
	}
	return result, nil
}

// Updater checks for newer releases and installs one only when the user asks.
type Updater struct {
	source      UpdateSource
	download    UpdateDownloader
	preferences func() Preferences
	emit        func(UpdateStatus)
	log         *slog.Logger
	// target is the app bundle on macOS or the binary on Linux that an update replaces.
	target   string
	unpack   func(ctx context.Context, archive, dir, version string) (string, error)
	verify   func(ctx context.Context, path, current, version string) error
	relaunch func(pid int, path string) *exec.Cmd
	mu       sync.Mutex
	status   UpdateStatus
	running  chan struct{}
	cancel   context.CancelFunc
	// stageDir holds the verified update at stagedPath until ApplyStaged.
	stageDir, stagedPath string
	restart              bool
}

type UpdaterOptions struct {
	Current string
	// Executable is the running executable, used to find what an update replaces.
	Executable  string
	Source      UpdateSource
	Download    UpdateDownloader
	Preferences func() Preferences
	Emit        func(UpdateStatus)
	Log         *slog.Logger
}

func NewUpdater(o UpdaterOptions) *Updater {
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	if o.Emit == nil {
		o.Emit = func(UpdateStatus) {}
	}
	u := &Updater{source: o.Source, download: o.Download, preferences: o.Preferences, emit: o.Emit, log: o.Log, unpack: unpackUpdate, verify: verifyUpdate, relaunch: relaunchCommand, status: UpdateStatus{Current: o.Current, Supported: parseSemver(o.Current) != nil}}
	if u.status.Supported && o.Download != nil {
		u.target, u.status.InstallNote = installTarget(o.Executable)
		u.status.Installable = u.target != ""
		if u.target != "" {
			removeStaleStages(u.target)
		}
	}
	return u
}

func (u *Updater) Status() UpdateStatus {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.status
}

// Check asks the source for the newest release above the current version. Automatic
// checks fail silently and keep the last result; manual checks report the error.
// Concurrent calls wait for the check in progress and return its result.
func (u *Updater) Check(ctx context.Context, manual bool) UpdateStatus {
	u.mu.Lock()
	if !u.status.Supported {
		if manual {
			u.status.Error = "Development builds cannot check for updates."
		}
		defer u.mu.Unlock()
		return u.status
	}
	if running := u.running; running != nil {
		u.mu.Unlock()
		select {
		case <-running:
		case <-ctx.Done():
		}
		return u.Status()
	}
	u.running = make(chan struct{})
	u.status.Checking = true
	u.status.Error = ""
	status := u.status
	u.mu.Unlock()
	u.emit(status)

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	releases, err := u.source(ctx)
	beta := u.preferences().BetaUpdates

	u.mu.Lock()
	u.status.Checking = false
	if err != nil {
		u.log.Info("update check failed", "manual", manual, "error", err)
		if manual {
			u.status.Error = "Could not check for updates: " + err.Error()
		}
	} else {
		u.status.Available = newestRelease(releases, u.status.Current, beta)
		u.status.CheckedAt = time.Now().UTC().Format(time.RFC3339)
		if u.status.Available != nil {
			u.log.Info("update available", "version", u.status.Available.Version)
		}
	}
	close(u.running)
	u.running = nil
	status = u.status
	u.mu.Unlock()
	u.emit(status)
	return status
}

// Run checks shortly after startup and then once a day while automatic checks are on.
func (u *Updater) Run(ctx context.Context) {
	if !u.Status().Supported {
		return
	}
	wait := updateStartDelay
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if !u.preferences().UpdateChecksOff {
			u.Check(ctx, false)
		}
		wait = updateInterval
	}
}

// newestRelease returns the highest release above current, ignoring pre-releases
// unless beta is on.
func newestRelease(releases []UpdateRelease, current string, beta bool) *UpdateRelease {
	best := parseSemver(current)
	var found *UpdateRelease
	for i := range releases {
		r := &releases[i]
		v := parseSemver(r.Version)
		if v == nil || (!beta && (r.Prerelease || v.pre != nil)) {
			continue
		}
		if best == nil || compareSemver(v, best) > 0 {
			best, found = v, r
		}
	}
	if found == nil {
		return nil
	}
	release := *found
	return &release
}

type semver struct {
	core [3]int
	pre  []string
}

// parseSemver accepts MAJOR.MINOR.PATCH with an optional -pre-release and +build.
func parseSemver(s string) *semver {
	s, _, _ = strings.Cut(s, "+")
	s, pre, hasPre := strings.Cut(s, "-")
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return nil
	}
	v := &semver{}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || (len(part) > 1 && part[0] == '0') {
			return nil
		}
		v.core[i] = n
	}
	if hasPre {
		v.pre = strings.Split(pre, ".")
		for _, id := range v.pre {
			if id == "" {
				return nil
			}
		}
	}
	return v
}

// compareSemver orders versions by SemVer precedence: a pre-release sorts before its
// release, numeric identifiers compare numerically and sort before alphanumeric ones.
func compareSemver(a, b *semver) int {
	for i := range a.core {
		if a.core[i] != b.core[i] {
			return cmp.Compare(a.core[i], b.core[i])
		}
	}
	if a.pre == nil || b.pre == nil {
		return cmp.Compare(len(b.pre), len(a.pre))
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		x, xErr := strconv.Atoi(a.pre[i])
		y, yErr := strconv.Atoi(b.pre[i])
		switch {
		case xErr == nil && yErr == nil && x != y:
			return cmp.Compare(x, y)
		case xErr == nil && yErr != nil:
			return -1
		case xErr != nil && yErr == nil:
			return 1
		case xErr != nil && a.pre[i] != b.pre[i]:
			return strings.Compare(a.pre[i], b.pre[i])
		}
	}
	return cmp.Compare(len(a.pre), len(b.pre))
}

func validUpdateVersion(version string) error {
	if version != "" && parseSemver(version) == nil {
		return errors.New("Invalid skipped update version.")
	}
	return nil
}
