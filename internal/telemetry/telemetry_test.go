package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (fn roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }
func fakeClient(t *testing.T, transport roundTrip) *Client {
	t.Helper()
	c := newClient("fake-workspace-token", "https://eu.i.posthog.com", "1.2.3", nil, nil, false)
	c.http.Transport = transport
	t.Cleanup(func() { c.Close(context.Background()) })
	return c
}
func success() *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}"))}
}

func TestEnvironmentGates(t *testing.T) {
	for _, env := range []string{"DO_NOT_TRACK=1", "ORCHESTRA_TELEMETRY_DISABLED=1", "CI=", "CI=false", "GITHUB_ACTIONS=true", "GITLAB_CI=1", "BUILD_NUMBER=42", "TF_BUILD=True", "CODEBUILD_BUILD_ID=123"} {
		if reason := DisabledReason("key", "https://eu.i.posthog.com", "1.2.3", []string{env}); reason == "" {
			t.Fatalf("not disabled by %s", env)
		}
	}
	for _, config := range [][3]string{{"", "https://eu.i.posthog.com", "1.0.0"}, {"key", "https://eu.i.posthog.com", "dev"}, {"key", "http://localhost", "1.0.0"}} {
		if DisabledReason(config[0], config[1], config[2], nil) == "" {
			t.Fatal("unsafe build enabled")
		}
	}
	if DisabledReason("key", "https://us.i.posthog.com", "v1.0.0", nil) != "" {
		t.Fatal("valid release disabled")
	}
}

func TestConsentAndAllowlist(t *testing.T) {
	var requests int
	var payload []byte
	c := fakeClient(t, func(r *http.Request) (*http.Response, error) {
		requests++
		payload, _ = io.ReadAll(r.Body)
		return success(), nil
	})
	c.Track("feature_used", map[string]any{"feature": "command_bar"})
	c.Flush(context.Background())
	if requests != 0 || len(c.queue) != 0 {
		t.Fatal("recorded or sent before consent")
	}
	c.SetConsent(true, "random-installation-id")
	c.Track("prompt_submitted", map[string]any{"prompt": "private prompt"})
	c.Track("feature_used", map[string]any{"feature": "private path"})
	c.Track("feature_used", map[string]any{"feature": []string{"command_bar"}})
	c.Track("session_started", map[string]any{"agent": "codex"})
	if len(c.queue) != 0 {
		t.Fatal("unknown/invalid events accepted")
	}
	props := map[string]any{"feature": "command_bar", "prompt": "private prompt", "path": "/private/workspace", "name": "private name", "$set": map[string]any{"email": "private@email"}, "$process_person_profile": true}
	c.Track("feature_used", props)
	props["feature"] = "private path" // Caller mutation must not alter the queued copy.
	c.Flush(context.Background())
	for _, secret := range []string{"private prompt", "/private/workspace", "private name", "private@email", "private path"} {
		if bytes.Contains(payload, []byte(secret)) {
			t.Fatalf("leaked %s", secret)
		}
	}
	var batch struct {
		Batch []event `json:"batch"`
	}
	if err := json.Unmarshal(payload, &batch); err != nil {
		t.Fatal(err)
	}
	if len(batch.Batch) != 1 || batch.Batch[0].Properties["$process_person_profile"] != false || batch.Batch[0].Properties["$geoip_disable"] != false || batch.Batch[0].Properties["feature"] != "command_bar" {
		t.Fatalf("bad payload: %s", payload)
	}
	c.Track("feature_used", map[string]any{"feature": "command_bar"})
	c.SetConsent(false, "")
	c.Flush(context.Background())
	if requests != 1 || c.id != "" || len(c.queue) != 0 {
		t.Fatal("opt-out did not discard queue and identity")
	}
	c.SetConsent(true, "different-installation-id")
	c.Track("feature_used", map[string]any{"feature": "command_bar"})
	if c.queue[0].DistinctID != "different-installation-id" {
		t.Fatal("old identity reused")
	}
}

func TestOptOutCancelsInflightAndRetry(t *testing.T) {
	started := make(chan struct{})
	var requests atomic.Int32
	c := fakeClient(t, func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		close(started)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	c.SetConsent(true, "id")
	c.Track("feature_used", map[string]any{"feature": "diff_view"})
	done := make(chan struct{})
	go func() { c.Flush(context.Background()); close(done) }()
	<-started
	c.SetConsent(false, "")
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("in-flight request did not cancel")
	}
	if requests.Load() != 1 {
		t.Fatal("retried after revocation")
	}
}

func TestRateLimitDebugAndQuit(t *testing.T) {
	requests := 0
	c := fakeClient(t, func(*http.Request) (*http.Response, error) { requests++; return success(), nil })
	c.SetConsent(true, "id")
	for i := 0; i < 100; i++ {
		c.Track("feature_used", map[string]any{"feature": "diff_view"})
	}
	if len(c.queue) != 20 {
		t.Fatal("missing per-event rate limit")
	}
	c.Close(context.Background())
	if requests != 1 {
		t.Fatal("quit did not flush")
	}
	var output bytes.Buffer
	d := fakeClient(t, func(*http.Request) (*http.Response, error) { t.Fatal("debug sent a request"); return success(), nil })
	d.status.Debug = true
	d.log = slog.New(slog.NewJSONHandler(&output, nil))
	d.Track("feature_used", map[string]any{"feature": "emoji_picker"})
	if output.Len() != 0 {
		t.Fatal("debug logged before consent")
	}
	d.SetConsent(true, "id")
	d.Track("feature_used", map[string]any{"feature": "emoji_picker"})
	d.Flush(context.Background())
	if !strings.Contains(output.String(), "emoji_picker") || len(d.queue) != 0 {
		t.Fatal("debug did not stay local")
	}
}

func TestRetryIsBounded(t *testing.T) {
	requests := 0
	c := fakeClient(t, func(*http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	c.SetConsent(true, "id")
	c.Track("error_shown", map[string]any{"code": "operation_failed"})
	c.Flush(context.Background())
	if requests != 3 || len(c.queue) != 0 {
		t.Fatal("retry did not stop and drop")
	}
}

func TestDurationBuckets(t *testing.T) {
	for duration, want := range map[time.Duration]string{0: "under_1m", time.Minute: "1-5m", 5 * time.Minute: "5-30m", 30 * time.Minute: "30m+"} {
		if DurationBucket(duration) != want {
			t.Fatalf("wrong duration bucket at %s", duration)
		}
	}
}

// App.shutdown tracks update_installed after the swap and before Close flushes.
func TestUpdateInstalledIsSentOnClose(t *testing.T) {
	var payload []byte
	c := fakeClient(t, func(r *http.Request) (*http.Response, error) {
		payload, _ = io.ReadAll(r.Body)
		return success(), nil
	})
	c.SetConsent(true, "random-installation-id")
	c.Track("update_installed", map[string]any{"from_version": "dev", "to_version": "0.3.1"})
	if len(c.queue) != 0 {
		t.Fatal("accepted a development version")
	}
	c.Track("update_installed", map[string]any{"from_version": "0.3.0", "to_version": "0.3.1"})
	c.Close(context.Background())
	if !bytes.Contains(payload, []byte(`"update_installed"`)) || !bytes.Contains(payload, []byte(`"to_version":"0.3.1"`)) {
		t.Fatalf("payload: %s", payload)
	}
}
