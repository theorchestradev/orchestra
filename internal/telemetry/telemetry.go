// Package telemetry sends only consented, allowlisted usage events. It never reads logs or workspace content.
package telemetry

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// Schema is also consumed by the website to generate the public event inventory.
//
//go:embed events.json
var Schema []byte

type rule struct {
	Type   string   `json:"type"`
	Values []string `json:"values"`
}

var allowlist = func() map[string]map[string]rule {
	var result map[string]map[string]rule
	if err := json.Unmarshal(Schema, &result); err != nil {
		panic(err)
	}
	return result
}()
var versionPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

func validVersion(value string) bool { return len(value) <= 64 && versionPattern.MatchString(value) }

// DisabledReason is checked before consent, even in debug mode. Only release-injected configuration can send.
func DisabledReason(key, host, version string, env []string) string {
	values := map[string]string{}
	for _, entry := range env {
		k, v, _ := strings.Cut(entry, "=")
		values[k] = v
	}
	for _, name := range []string{"DO_NOT_TRACK", "ORCHESTRA_TELEMETRY_DISABLED"} {
		if values[name] == "1" {
			return "Disabled by " + name + "=1."
		}
	}
	for _, name := range []string{"CI", "CONTINUOUS_INTEGRATION", "GITHUB_ACTIONS", "GITLAB_CI", "BUILD_NUMBER", "BUILD_ID", "JENKINS_URL", "TEAMCITY_VERSION", "TF_BUILD", "CIRCLECI", "TRAVIS", "APPVEYOR", "BUILDKITE", "DRONE", "CODEBUILD_BUILD_ID", "BITBUCKET_BUILD_NUMBER", "HUDSON_URL", "BAMBOO_BUILDKEY", "bamboo.buildKey"} {
		if _, present := values[name]; present {
			return "Disabled in continuous integration (" + name + ")."
		}
	}
	if key == "" || !validVersion(version) {
		return "Unavailable in this build. Only official releases can share usage data."
	}
	if host != "https://eu.i.posthog.com" && host != "https://us.i.posthog.com" {
		return "Unavailable: invalid release analytics endpoint."
	}
	return ""
}

type Status struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
	Debug     bool   `json:"debug"`
}
type event struct {
	Name       string         `json:"event"`
	DistinctID string         `json:"distinct_id"`
	Properties map[string]any `json:"properties"`
	Timestamp  time.Time      `json:"timestamp"`
}
type rate struct {
	start time.Time
	count int
}

// Client owns a bounded, memory-only queue and one flushing worker. Revoking consent cancels in-flight requests.
type Client struct {
	mu            sync.Mutex
	flushMu       sync.Mutex
	key, host     string
	status        Status
	log           *slog.Logger
	http          *http.Client
	id            string
	queue         []event
	rates         map[string]rate
	consentCtx    context.Context
	cancelConsent context.CancelFunc
	cancelWorker  context.CancelFunc
	done          chan struct{}
	closed        bool
}

func New(key, host, version string, logger *slog.Logger) *Client {
	return newClient(key, host, version, logger, os.Environ(), os.Getenv("ORCHESTRA_TELEMETRY_DEBUG") == "1")
}

func newClient(key, host, version string, logger *slog.Logger, env []string, debug bool) *Client {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	reason := DisabledReason(key, host, version, env)
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{key: key, host: host, log: logger, status: Status{reason == "", reason, debug}, http: &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, cancelWorker: cancel, done: make(chan struct{})}
	go func() {
		defer close(c.done)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.Flush(ctx)
			}
		}
	}()
	return c
}
func (c *Client) Status() Status { return c.status }

func (c *Client) SetConsent(enabled bool, id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if enabled && id != "" && c.id == id {
		return
	}
	if c.cancelConsent != nil {
		c.cancelConsent()
	}
	c.id, c.queue, c.rates = "", nil, nil
	if enabled && id != "" && c.status.Available && !c.closed {
		c.id = id
		c.rates = map[string]rate{}
		c.consentCtx, c.cancelConsent = context.WithCancel(context.Background())
	}
}

// Track is shared by Store and App.Track. It copies only validated scalar properties; caller maps cannot mutate queued events.
func (c *Client) Track(name string, props map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.id == "" || c.closed {
		return
	}
	rules, ok := allowlist[name]
	if !ok {
		c.log.Debug("telemetry dropped unknown event")
		return
	}
	clean := map[string]any{}
	for key, value := range props {
		r, ok := rules[key]
		if !ok {
			c.log.Debug("telemetry dropped unknown property", "event", name)
			continue
		}
		valid := false
		if r.Type == "boolean" {
			_, valid = value.(bool)
		} else if text, ok := value.(string); ok {
			valid = slices.Contains(r.Values, text) || (r.Type == "version" && validVersion(text))
		}
		if !valid {
			c.log.Debug("telemetry dropped invalid property", "event", name)
			return
		}
		clean[key] = value
	}
	if len(clean) != len(rules) {
		c.log.Debug("telemetry dropped incomplete event", "event", name)
		return
	}
	now := time.Now().UTC()
	r := c.rates[name]
	if now.Sub(r.start) >= time.Minute {
		r = rate{start: now}
	}
	if r.count >= 20 || len(c.queue) >= 256 {
		return
	}
	r.count++
	c.rates[name] = r
	clean["$process_person_profile"] = false
	// PostHog derives approximate location from the connection, then discards the IP.
	// The project pipeline retains only city/country; see docs/privacy.md for release configuration.
	clean["$geoip_disable"] = false
	clean["$lib"] = "orchestra"
	e := event{name, c.id, clean, now}
	if c.status.Debug {
		data, _ := json.Marshal(e)
		c.log.Info("telemetry debug (not sent)", "event", json.RawMessage(data))
		return
	}
	c.queue = append(c.queue, e)
}

func (c *Client) Flush(ctx context.Context) {
	c.flushMu.Lock()
	defer c.flushMu.Unlock()
	c.mu.Lock()
	if len(c.queue) == 0 {
		c.mu.Unlock()
		return
	}
	batch, consentCtx := c.queue, c.consentCtx
	c.queue = nil
	c.mu.Unlock()
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(consentCtx, cancel)
	defer stop()
	defer cancel()
	data, _ := json.Marshal(struct {
		Key   string  `json:"api_key"`
		Batch []event `json:"batch"`
	}{c.key, batch})
	for attempt := 0; attempt < 3; attempt++ {
		if consentCtx.Err() != nil || ctx.Err() != nil {
			return
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+"/batch/", bytes.NewReader(data))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.http.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return
			}
			if resp.StatusCode < 500 && resp.StatusCode != 429 {
				return
			}
		}
		if attempt < 2 {
			timer := time.NewTimer(time.Duration(attempt+1) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
	c.log.Debug("telemetry batch dropped after retries")
}

func (c *Client) Close(ctx context.Context) {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.cancelWorker()
	select {
	case <-c.done:
	case <-ctx.Done():
		c.SetConsent(false, "")
		return
	}
	c.Flush(ctx)
	c.SetConsent(false, "")
}

func DurationBucket(duration time.Duration) string {
	switch {
	case duration < time.Minute:
		return "under_1m"
	case duration < 5*time.Minute:
		return "1-5m"
	case duration < 30*time.Minute:
		return "5-30m"
	default:
		return "30m+"
	}
}
