package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitHubMetadataEditing(t *testing.T) {
	s := fixtureStore(t)
	p := projectFixture(t, s)
	s.mu.Lock()
	stored, err := s.projectLocked(p.ID)
	if err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	stored.Repositories[0].GitHub = "fixture/repository"
	s.mu.Unlock()
	dir := t.TempDir()
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	script := `#!/bin/sh
base="$(dirname "$0")"
printf '%s\n' "$@" > "$base/args"
cat > "$base/input"
if [ -f "$base/fail" ]; then printf 'HTTP 403: Resource not accessible' >&2; exit 1; fi
case "$*" in *repos/fixture/repository/issues/84*) cat "$base/canonical"; exit 0;; esac
cat "$base/response"
`
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	response := func(body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "response"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	read := func(file string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if err = os.WriteFile(filepath.Join(dir, "canonical"), []byte(`{"id":984,"number":84}`), 0600); err != nil {
		t.Fatal(err)
	}
	response(`[[{"login":"first"}],[{"login":"second"}]]`)
	options, err := s.GitHubMetadataOptions(p.ID, "", "assignees")
	if err != nil || len(options) != 2 || !strings.Contains(read("args"), "--paginate\n--slurp") {
		t.Fatalf("pagination: %s, %v", options, err)
	}
	response(`[[{"name":"a label"}]]`)
	if _, err = s.GitHubMetadataOptions(p.ID, "", "labels"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		field           string
		values          []string
		pull            bool
		reply, endpoint string
	}{
		{"state", []string{"closed"}, false, `{"state":"closed"}`, "issues"},
		{"state", []string{"closed", "completed"}, false, `{"state":"closed","state_reason":"completed"}`, "issues"},
		{"state", []string{"closed", "not_planned"}, false, `{"state":"closed","state_reason":"not_planned"}`, "issues"},
		{"state", []string{"closed", "duplicate", "84"}, false, `{"state":"closed","state_reason":"duplicate"}`, "issues"},
		{"state", []string{"open"}, true, `{"state":"open","merged":false}`, "pulls"},
		{"assignees", []string{"second", "first"}, true, `{"assignees":[{"login":"first"},{"login":"second"}]}`, "issues"},
		{"assignees", nil, false, `{"assignees":[]}`, "issues"},
		{"labels", []string{`label with "quotes", commas, spaces & unicode: ação`}, false, `{"labels":[{"name":"label with \"quotes\", commas, spaces & unicode: ação"}]}`, "issues"},
		{"labels", []string{}, true, `{"labels":[]}`, "issues"},
	} {
		response(tc.reply)
		if err = s.UpdateGitHubMetadata(p.ID, "", 42, tc.pull, tc.field, tc.values); err != nil {
			t.Fatalf("%s: %v", tc.field, err)
		}
		args := read("args")
		if !strings.Contains(args, "--method\nPATCH\nrepos/fixture/repository/"+tc.endpoint+"/42") || !strings.Contains(args, "--input\n-") {
			t.Fatal(args)
		}
		var body map[string]json.RawMessage
		wantFields := 1
		if tc.field == "state" && len(tc.values) > 1 {
			wantFields = 2
			if len(tc.values) == 3 {
				wantFields = 3
			}
		}
		if err = json.Unmarshal([]byte(read("input")), &body); err != nil || len(body) != wantFields || body[tc.field] == nil {
			t.Fatalf("unrelated fields modified: %s", read("input"))
		}
		if tc.field == "state" && len(tc.values) > 1 && string(body["state_reason"]) != `"`+tc.values[1]+`"` {
			t.Fatal("wrong closing reason")
		}
		if tc.field == "state" && len(tc.values) == 3 && string(body["duplicate_issue_id"]) != "984" {
			t.Fatal("must use canonical issue ID, not its number")
		}
		if tc.field != "state" && len(tc.values) == 0 && string(body[tc.field]) != "[]" {
			t.Fatal("clear must send an empty array")
		}
	}
	response(`{"state":"closed","state_reason":"completed"}`)
	if err = s.UpdateGitHubMetadata(p.ID, "", 42, false, "state", []string{"closed", "not_planned"}); err == nil {
		t.Fatal("ignored reason reported success")
	}
	if err = s.UpdateGitHubMetadata(p.ID, "", 42, true, "state", []string{"closed", "completed"}); err == nil {
		t.Fatal("PR accepted an issue reason")
	}
	os.WriteFile(filepath.Join(dir, "canonical"), []byte(`{"id":984,"number":84,"pull_request":{}}`), 0600)
	if err = s.UpdateGitHubMetadata(p.ID, "", 42, false, "state", []string{"closed", "duplicate", "84"}); err == nil || strings.Contains(read("args"), "PATCH") {
		t.Fatal("PR accepted as duplicate issue")
	}
	response(`{"merged":true,"state":"closed"}`)
	if err = s.UpdateGitHubMetadata(p.ID, "", 42, true, "state", []string{"open"}); err == nil || !strings.Contains(err.Error(), "merged") || strings.Contains(read("args"), "PATCH") {
		t.Fatalf("merged PR mutated: %v", err)
	}
	response(`{"assignees":[]}`)
	if err = s.UpdateGitHubMetadata(p.ID, "", 42, false, "assignees", []string{"first"}); err == nil {
		t.Fatal("silently ignored edit reported success")
	}
	response(`null`)
	if err = s.UpdateGitHubMetadata(p.ID, "", 42, false, "labels", nil); err == nil {
		t.Fatal("null response accepted")
	}
	response(`not JSON`)
	if err = s.UpdateGitHubMetadata(p.ID, "", 42, false, "labels", nil); err == nil {
		t.Fatal("invalid response accepted")
	}
	os.WriteFile(filepath.Join(dir, "fail"), nil, 0600)
	if err = s.UpdateGitHubMetadata(p.ID, "", 42, false, "state", []string{"closed"}); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("permission error lost: %v", err)
	}
	os.Remove(filepath.Join(dir, "fail"))
	for _, tc := range []struct {
		number int
		field  string
		values []string
	}{
		{0, "state", []string{"closed"}}, {42, "title", []string{"bad"}}, {42, "state", []string{"merged"}},
		{42, "state", []string{"closed", "duplicate", "42"}}, {42, "state", []string{"closed", "duplicate", "x"}}, {42, "state", []string{"closed", "duplicate"}}, {42, "state", []string{"closed", "completed", "84"}}, {42, "state", []string{"open", "completed"}},
		{42, "labels", []string{"bad\nlabel"}}, {42, "assignees", []string{"--bad"}}, {42, "labels", []string{"duplicate", "duplicate"}},
	} {
		os.Remove(filepath.Join(dir, "args"))
		if err = s.UpdateGitHubMetadata(p.ID, "", tc.number, false, tc.field, tc.values); err == nil {
			t.Fatalf("invalid edit accepted: %+v", tc)
		}
		if _, err = os.Stat(filepath.Join(dir, "args")); !os.IsNotExist(err) {
			t.Fatal("invalid edit launched gh")
		}
	}
}

func TestGitHubDescriptionEditing(t *testing.T) {
	s := fixtureStore(t)
	p := projectFixture(t, s)
	s.mu.Lock()
	stored, err := s.projectLocked(p.ID)
	if err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	stored.Repositories[0].GitHub = "fixture/repository"
	s.mu.Unlock()
	dir := t.TempDir()
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	script := `#!/bin/sh
base="$(dirname "$0")"
case "$*" in *PATCH*) ;; *) printf '%s\n' "$@" > "$base/fetch"; cat "$base/current"; exit 0;; esac
printf '%s\n' "$@" > "$base/args"
cat > "$base/input"
if [ -f "$base/fail" ]; then printf 'HTTP 403: Resource not accessible' >&2; exit 1; fi
cat "$base/response"
`
	write := func(file, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	read := func(file string) string {
		t.Helper()
		data, _ := os.ReadFile(filepath.Join(dir, file))
		return string(data)
	}
	if err = os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	original := "Intro\r\n\r\n- [ ] task\n\n```go\nfmt.Println(\"x\")\n```\n[link](https://example.com)"
	edited := "Intro\n\n- [x] task\n\n```go\nfmt.Println(\"x\")\n```\n[link](https://example.com)"
	current, _ := json.Marshal(map[string]string{"body": original})
	write("current", string(current))
	reply, _ := json.Marshal(map[string]string{"body": edited})
	write("response", string(reply))
	for _, pull := range []bool{false, true} {
		if err = s.UpdateGitHubMetadata(p.ID, "", 42, pull, "body", []string{edited, original}); err != nil {
			t.Fatal(err)
		}
		if args := read("args"); !strings.Contains(args, "--method\nPATCH\nrepos/fixture/repository/issues/42") {
			t.Fatal(args)
		}
		if !strings.Contains(read("fetch"), "repos/fixture/repository/issues/42") {
			t.Fatal("description was not checked before saving")
		}
		var body map[string]string
		if err = json.Unmarshal([]byte(read("input")), &body); err != nil || len(body) != 1 || body["body"] != edited {
			t.Fatalf("unexpected payload: %s", read("input"))
		}
	}
	// An empty description is allowed, and GitHub reports it as null.
	write("current", `{"body":null}`)
	write("response", `{"body":null}`)
	if err = s.UpdateGitHubMetadata(p.ID, "", 42, false, "body", []string{"", ""}); err != nil {
		t.Fatal(err)
	}
	if read("input") != `{"body":""}` {
		t.Fatalf("empty description payload: %s", read("input"))
	}
	write("response", `{"body":"x"}`)
	if err = s.UpdateGitHubMetadata(p.ID, "", 42, false, "body", []string{"x", ""}); err != nil {
		t.Fatal(err)
	}
	// A description changed on GitHub since editing started is never overwritten.
	write("current", `{"body":"changed elsewhere"}`)
	os.Remove(filepath.Join(dir, "args"))
	if err = s.UpdateGitHubMetadata(p.ID, "", 42, false, "body", []string{"x", ""}); err == nil || !strings.Contains(err.Error(), "changed on GitHub") {
		t.Fatalf("stale description overwritten: %v", err)
	}
	if read("args") != "" {
		t.Fatal("stale description launched PATCH")
	}
	write("current", `{"body":"same"}`)
	write("response", `{"body":"same"}`)
	if err = s.UpdateGitHubMetadata(p.ID, "", 42, false, "body", []string{"new", "same"}); err == nil {
		t.Fatal("ignored description reported success")
	}
	write("fail", "")
	if err = s.UpdateGitHubMetadata(p.ID, "", 42, false, "body", []string{"new", "same"}); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("permission error lost: %v", err)
	}
	os.Remove(filepath.Join(dir, "fail"))
	for _, values := range [][]string{nil, {"only new"}, {"a", "b", "c"}, {"nul\x00", "same"}, {strings.Repeat("x", 262145), "same"}} {
		os.Remove(filepath.Join(dir, "fetch"))
		if err = s.UpdateGitHubMetadata(p.ID, "", 42, false, "body", values); err == nil {
			t.Fatalf("invalid description accepted: %d values", len(values))
		}
		if read("fetch") != "" {
			t.Fatal("invalid description launched gh")
		}
	}
}
