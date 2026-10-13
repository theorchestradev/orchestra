package core

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// The public GitHub API and release downloads need no login. They serve users without
// the GitHub CLI.
const (
	publicReleasesURL = "https://api.github.com/repos/" + UpdateRepository + "/releases?per_page=30"
	publicDownloadURL = "https://github.com/" + UpdateRepository + "/releases/download/"
)

// Release assets are about 10 MB; the limits only stop a runaway response.
const maxReleasesResponse, maxAssetDownload = 8 << 20, 512 << 20

// withFallback returns primary's result, or fallback's when primary fails. When both
// fail, primary's error is the one reported.
func withFallback[T any](primary, fallback func() (T, error)) (T, error) {
	result, err := primary()
	if err == nil {
		return result, nil
	}
	if result, fallbackErr := fallback(); fallbackErr == nil {
		return result, nil
	}
	return result, err
}

func publicReleases(ctx context.Context, transport http.RoundTripper) ([]UpdateRelease, error) {
	var body bytes.Buffer
	if err := publicGet(ctx, transport, publicReleasesURL, &body, maxReleasesResponse); err != nil {
		return nil, err
	}
	return parseGitHubReleases(body.Bytes())
}

func publicDownload(ctx context.Context, transport http.RoundTripper, version string, assets []string, dir string) error {
	for _, asset := range assets {
		file, err := os.Create(filepath.Join(dir, asset))
		if err != nil {
			return err
		}
		err = publicGet(ctx, transport, publicDownloadURL+"v"+version+"/"+asset, file, maxAssetDownload)
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// publicGet copies a GitHub response into w. Downloads follow GitHub's redirect to its
// asset storage; ctx bounds the whole request.
func publicGet(ctx context.Context, transport http.RoundTripper, url string, w io.Writer, limit int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Orchestra")
	response, err := (&http.Client{Transport: transport}).Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub returned %s.", response.Status)
	}
	n, err := io.Copy(w, io.LimitReader(response.Body, limit+1))
	if err == nil && n > limit {
		err = fmt.Errorf("GitHub's response exceeded %d MB.", limit>>20)
	}
	return err
}
