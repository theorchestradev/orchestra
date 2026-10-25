package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// pullRequestFixture puts a fake gh first in PATH. It records its arguments and answers
// from files named after the request, so tests never reach GitHub.
func pullRequestFixture(t *testing.T) (string, func(name, body string)) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	script := `#!/bin/sh
base="$(dirname "$0")"
printf '%s\n' "$@" >> "$base/args"
case "$*" in
*"pr list"*) name=list;;
*"pr view"*) name=view;;
*/reviews*) name=reviews;;
*/comments*) name=comments;;
*/annotations*) name=annotations;;
*/logs*) name=logs;;
*check-runs/*) name=run;;
*) name=unknown;;
esac
if [ -f "$base/$name.fail" ]; then cat "$base/$name.fail" >&2; exit 1; fi
cat "$base/$name"
`
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return dir, func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSessionPullRequestFindsBranchPullRequest(t *testing.T) {
	s := fixtureStore(t)
	p := repositoryFixture(t, s)
	dir, respond := pullRequestFixture(t)
	state, err := s.CreateSession(p.ID, "claude", "", "", true, nil, "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	sess := state.Projects[0].Sessions[0]
	status, err := s.SessionPullRequest(sess.ID, false)
	if err != nil || status.Message != "This project has no GitHub remote." {
		t.Fatalf("without remote: %+v, %v", status, err)
	}
	s.mu.Lock()
	stored, _ := s.projectLocked(p.ID)
	stored.Repository = "fixture/repository"
	s.mu.Unlock()
	branch := sess.WorktreeBranch
	respond("list", `[
		{"number":7,"title":"Fork","state":"OPEN","headRefName":"`+branch+`","isCrossRepository":true},
		{"number":8,"title":"Old","state":"CLOSED","headRefName":"`+branch+`","headRefOid":"0000"},
		{"number":9,"url":"https://github.com/fixture/repository/pull/9","title":"Session work","state":"OPEN","isDraft":true,"headRefName":"`+branch+`","reviewDecision":"REVIEW_REQUIRED","statusCheckRollup":[
			{"__typename":"CheckRun","name":"build","workflowName":"CI","status":"COMPLETED","conclusion":"FAILURE","detailsUrl":"https://github.com/fixture/repository/actions/runs/10/job/100","startedAt":"2026-10-21T17:00:00Z","completedAt":"2026-10-21T17:16:18Z"},
			{"__typename":"CheckRun","name":"build","workflowName":"CI","status":"COMPLETED","conclusion":"SUCCESS","detailsUrl":"https://github.com/fixture/repository/actions/runs/11/job/110","startedAt":"2026-10-21T18:00:00Z","completedAt":"2026-10-21T18:00:05Z"},
			{"__typename":"CheckRun","name":"build","workflowName":"Docker","status":"IN_PROGRESS","detailsUrl":"https://github.com/fixture/repository/actions/runs/12/job/120","startedAt":"2026-10-21T18:00:00Z","completedAt":"0001-01-01T00:00:00Z"},
			{"__typename":"CheckRun","name":"lint","workflowName":"CI","status":"COMPLETED","conclusion":"SKIPPED","detailsUrl":"https://github.com/fixture/repository/actions/runs/11/job/111"},
			{"__typename":"StatusContext","context":"deploy/preview","state":"ERROR","targetUrl":"https://example.invalid/deploy"}
		]}
	]`)
	respond("reviews", `[[{"id":1,"state":"APPROVED"}]]`)
	respond("comments", `[[{"id":2}],[{"id":3}]]`)
	status, err = s.SessionPullRequest(sess.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if status.Number != 9 || status.State != "draft" || status.Branch != branch || status.ReviewDecision != "REVIEW_REQUIRED" {
		t.Fatalf("picked the wrong pull request: %+v", status)
	}
	got := []string{}
	for _, c := range status.Checks {
		got = append(got, c.Workflow+"/"+c.Name+"="+c.Status)
	}
	if strings.Join(got, " ") != "CI/build=success Docker/build=pending CI/lint=skipped /deploy/preview=failure" {
		t.Fatalf("checks: %v", got)
	}
	if status.Checks[0].JobID != 110 || status.Checks[0].Duration != "5s" || status.Checks[1].Duration == "" {
		t.Fatalf("check details: %+v", status.Checks)
	}
	if len(status.Reviews) != 1 || len(status.Comments) != 2 {
		t.Fatalf("activity: %+v", status)
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args"))
	if !strings.Contains(string(args), "--head\n"+branch+"\n") || !strings.Contains(string(args), "repos/fixture/repository/pulls/9/reviews") {
		t.Fatalf("gh arguments: %s", args)
	}

	respond("list", `[{"number":8,"title":"Old","state":"MERGED","headRefName":"`+branch+`","headRefOid":"0000"}]`)
	status, err = s.SessionPullRequest(sess.ID, false)
	if err != nil || status.Number != 0 || !strings.Contains(status.Message, "No pull request for "+branch) {
		t.Fatalf("stale pull request: %+v, %v", status, err)
	}
	head := gitWorktreeFixture(t, sess.WorktreeDirectory, "rev-parse", "HEAD")
	respond("list", `[{"number":8,"title":"Done","state":"MERGED","headRefName":"`+branch+`","headRefOid":"`+head+`"}]`)
	if status, err = s.SessionPullRequest(sess.ID, false); err != nil || status.Number != 8 || status.State != "merged" {
		t.Fatalf("merged pull request: %+v, %v", status, err)
	}

	respond("list.fail", "To get started with GitHub CLI, please run:  gh auth login")
	if _, err = s.SessionPullRequest(sess.ID, false); err == nil || !strings.Contains(err.Error(), "gh auth login") || strings.Contains(err.Error(), "To get started") {
		t.Fatalf("sign-in error: %v", err)
	}
}

func TestSessionPullRequestUsesReviewedPullRequest(t *testing.T) {
	s := fixtureStore(t)
	p := repositoryFixture(t, s)
	dir, respond := pullRequestFixture(t)
	s.mu.Lock()
	stored, _ := s.projectLocked(p.ID)
	stored.Repository = "fixture/repository"
	s.mu.Unlock()
	state, err := s.CreateSession(p.ID, "claude", "", "", false, &GitHubItem{Number: 42, PullRequest: true}, "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	respond("view", `{"number":42,"title":"Reviewed","state":"OPEN","headRefName":"feature","baseRefName":"main","isCrossRepository":true,"mergeable":"CONFLICTING","statusCheckRollup":[]}`)
	status, err := s.SessionPullRequest(state.Projects[0].Sessions[0].ID, false)
	if err != nil || status.Number != 42 || status.Branch != "feature" || status.State != "open" {
		t.Fatalf("reviewed pull request: %+v, %v", status, err)
	}
	if !status.Conflicts || status.BaseBranch != "main" || !status.ForkWithoutEdits {
		t.Fatalf("conflicts: %+v", status)
	}
	// GitHub reports UNKNOWN while it computes mergeability; that isn't a conflict yet.
	respond("view", `{"number":42,"title":"Reviewed","state":"OPEN","headRefName":"feature","isCrossRepository":true,"maintainerCanModify":true,"mergeable":"UNKNOWN","statusCheckRollup":[]}`)
	if status, err = s.SessionPullRequest(state.Projects[0].Sessions[0].ID, false); err != nil || status.Conflicts || status.ForkWithoutEdits {
		t.Fatalf("unknown mergeability: %+v, %v", status, err)
	}
	if args, _ := os.ReadFile(filepath.Join(dir, "args")); !strings.HasPrefix(string(args), "pr\nview\n42\n--repo\nfixture/repository\n") {
		t.Fatalf("gh arguments: %s", args)
	}
}

func TestGitHubPullRequest(t *testing.T) {
	s := fixtureStore(t)
	p := repositoryFixture(t, s)
	dir, respond := pullRequestFixture(t)
	s.mu.Lock()
	stored, _ := s.projectLocked(p.ID)
	stored.Repository = "fixture/repository"
	s.mu.Unlock()
	respond("view", `{"number":56,"title":"Bump","state":"OPEN","headRefName":"bump","baseRefName":"main","statusCheckRollup":[{"__typename":"CheckRun","name":"build","workflowName":"CI","status":"COMPLETED","conclusion":"FAILURE","detailsUrl":"https://github.com/fixture/repository/actions/runs/10/job/100"}]}`)
	status, err := s.GitHubPullRequest(p.ID, 56)
	if err != nil || status.Number != 56 || status.Branch != "bump" || status.Repository != "fixture/repository" {
		t.Fatalf("pull request: %+v, %v", status, err)
	}
	if len(status.Checks) != 1 || status.Checks[0].Status != "failure" || status.Checks[0].JobID != 100 {
		t.Fatalf("checks: %+v", status.Checks)
	}
	if args, _ := os.ReadFile(filepath.Join(dir, "args")); !strings.HasPrefix(string(args), "pr\nview\n56\n--repo\nfixture/repository\n") {
		t.Fatalf("gh arguments: %s", args)
	}
	respond("run", `{"name":"build","conclusion":"failure"}`)
	respond("annotations", `[]`)
	respond("logs", "2026-10-21T17:00:00.0000000Z FAIL")
	if text, err := s.GitHubCheckFailure(p.ID, 100); err != nil || !strings.Contains(text, "Check: build\n") || !strings.Contains(text, "FAIL\n```") {
		t.Fatalf("failure: %q, %v", text, err)
	}
	if _, err = s.GitHubPullRequest(p.ID, 0); err == nil {
		t.Fatal("invalid number accepted")
	}
}

func TestPullRequestCheckFailure(t *testing.T) {
	s := fixtureStore(t)
	p := repositoryFixture(t, s)
	_, respond := pullRequestFixture(t)
	s.mu.Lock()
	stored, _ := s.projectLocked(p.ID)
	stored.Repository = "fixture/repository"
	s.mu.Unlock()
	sess := sessionFixture(t, s, p, "terminal")
	respond("run", `{"name":"build","conclusion":"failure","html_url":"https://github.com/fixture/repository/actions/runs/10/job/100","output":{"title":"Build failed","summary":"1 error"}}`)
	respond("annotations", `[{"path":"main.go","start_line":3,"end_line":5,"annotation_level":"failure","message":"undefined: x"}]`)
	lines := []string{}
	for i := 0; i < maxCheckLogLines+50; i++ {
		lines = append(lines, "2026-10-21T17:00:00.1234567Z \x1b[31mline "+strings.Repeat("x", i%3)+"\x1b[0m")
	}
	respond("logs", strings.Join(append(lines, "2026-10-21T17:00:01.0000000Z ##[error]Process completed with exit code 1."), "\n"))
	text, err := s.PullRequestCheckFailure(sess.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Check: build\n", "Conclusion: failure\n", "Build failed\n\n1 error", "- main.go:3-5 failure: undefined: x", "Log (last lines):\n```\n", "##[error]Process completed with exit code 1.\n```"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "\x1b") || strings.Contains(text, "2026-10-21T") || strings.Count(text, "line") > maxCheckLogLines {
		t.Fatalf("log tail not cleaned or capped:\n%s", text)
	}
	respond("logs.fail", "HTTP 410: Gone")
	if text, err = s.PullRequestCheckFailure(sess.ID, 100); err != nil || !strings.HasSuffix(text, "The job log is unavailable.") {
		t.Fatalf("expired log: %q, %v", text, err)
	}
	if _, err = s.PullRequestCheckFailure(sess.ID, 0); err == nil {
		t.Fatal("invalid check accepted")
	}
}

func TestCheckDuration(t *testing.T) {
	now := time.Date(2026, 10, 21, 18, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		started, completed string
		running            bool
		want               string
	}{
		{"2026-10-21T17:43:42Z", "2026-10-21T18:00:00Z", false, "16m 18s"},
		{"2026-10-21T16:59:57Z", "2026-10-21T18:00:00Z", false, "1h 3s"},
		{"2026-10-21T18:00:00Z", "2026-10-21T18:00:00Z", false, "0s"},
		{"2026-10-21T17:58:00Z", "0001-01-01T00:00:00Z", true, "2m"},
		{"0001-01-01T00:00:00Z", "", true, ""},
		{"2026-10-21T17:58:00Z", "", false, ""},
	} {
		if got := checkDuration(tc.started, tc.completed, tc.running, now); got != tc.want {
			t.Errorf("checkDuration(%s, %s) = %q, want %q", tc.started, tc.completed, got, tc.want)
		}
	}
}

func TestLogTailKeepsValidUTF8(t *testing.T) {
	tail, truncated := logTail(strings.Repeat("é", maxCheckLogBytes))
	if !truncated || !utf8.ValidString(tail) {
		t.Fatalf("truncated=%v valid=%v", truncated, utf8.ValidString(tail))
	}
}
