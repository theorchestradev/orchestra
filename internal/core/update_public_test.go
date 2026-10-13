package core

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeGitHub func(*http.Request) (int, string)

func (f fakeGitHub) RoundTrip(req *http.Request) (*http.Response, error) {
	status, body := f(req)
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

func TestWithFallback(t *testing.T) {
	fallbackCalls := 0
	fallback := func() (string, error) { fallbackCalls++; return "public", nil }
	if got, err := withFallback(func() (string, error) { return "gh", nil }, fallback); got != "gh" || err != nil || fallbackCalls != 0 {
		t.Fatalf("primary success: %q %v, fallback called %d times", got, err, fallbackCalls)
	}
	if got, err := withFallback(func() (string, error) { return "", errors.New("gh: not found") }, fallback); got != "public" || err != nil {
		t.Fatalf("fallback: %q %v", got, err)
	}
	_, err := withFallback(func() (string, error) { return "", errors.New("gh: not found") }, func() (string, error) { return "", errors.New("404") })
	if err == nil || err.Error() != "gh: not found" {
		t.Fatalf("both failed: %v", err)
	}
}

func TestPublicReleases(t *testing.T) {
	transport := fakeGitHub(func(req *http.Request) (int, string) {
		if req.URL.String() != publicReleasesURL || req.Header.Get("Accept") != "application/vnd.github+json" || req.Header.Get("User-Agent") == "" {
			t.Errorf("request: %s %v", req.URL, req.Header)
		}
		return http.StatusOK, `[{"tag_name": "v0.4.0", "html_url": "https://github.com/theorchestradev/orchestra/releases/tag/v0.4.0"}, {"tag_name": "v0.5.0", "draft": true}]`
	})
	releases, err := publicReleases(context.Background(), transport)
	if err != nil || len(releases) != 1 || releases[0].Version != "0.4.0" {
		t.Fatalf("releases = %+v, %v", releases, err)
	}
	// A private repository looks missing to the public API.
	missing := fakeGitHub(func(*http.Request) (int, string) { return http.StatusNotFound, `{"message": "Not Found"}` })
	if _, err := publicReleases(context.Background(), missing); err == nil || !strings.Contains(err.Error(), "Not Found") {
		t.Fatalf("missing repository: %v", err)
	}
}

func TestPublicDownload(t *testing.T) {
	dir := t.TempDir()
	served := map[string]string{
		publicDownloadURL + "v0.4.0/" + updateAsset("0.4.0"): "archive",
		publicDownloadURL + "v0.4.0/SHA256SUMS":              "sums",
	}
	transport := fakeGitHub(func(req *http.Request) (int, string) {
		if body, ok := served[req.URL.String()]; ok {
			return http.StatusOK, body
		}
		return http.StatusNotFound, ""
	})
	if err := publicDownload(context.Background(), transport, "0.4.0", []string{updateAsset("0.4.0"), "SHA256SUMS"}, dir); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{updateAsset("0.4.0"): "archive", "SHA256SUMS": "sums"} {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(data) != want {
			t.Fatalf("%s = %q, %v", name, data, err)
		}
	}
	if err := publicDownload(context.Background(), transport, "9.9.9", []string{"missing.zip"}, dir); err == nil {
		t.Fatal("downloaded a missing asset")
	}
}

func TestPublicGetLimit(t *testing.T) {
	transport := fakeGitHub(func(*http.Request) (int, string) { return http.StatusOK, strings.Repeat("x", 11) })
	var out bytes.Buffer
	if err := publicGet(context.Background(), transport, publicReleasesURL, &out, 10); err == nil {
		t.Fatal("accepted a response over the limit")
	}
	out.Reset()
	if err := publicGet(context.Background(), transport, publicReleasesURL, &out, 11); err != nil || out.Len() != 11 {
		t.Fatalf("response at the limit: %d bytes, %v", out.Len(), err)
	}
}
