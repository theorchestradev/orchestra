package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Limits keep a long CI log from flooding the clipboard when a failure is copied.
const (
	maxCheckLogLines   = 200
	maxCheckLogBytes   = 16 << 10
	maxCheckAnnotation = 20
)

// PullRequestCheck is one check run or commit status on the pull request's head commit.
// Status is success, failure, pending, cancelled or skipped.
type PullRequestCheck struct {
	Name     string `json:"name"`
	Workflow string `json:"workflow,omitempty"`
	Status   string `json:"status"`
	URL      string `json:"url,omitempty"`
	Duration string `json:"duration,omitempty"`
	// JobID is the GitHub Actions job, which is also its check run, when the check has one.
	JobID int64 `json:"jobID,omitempty"`
}

// PullRequestStatus is the pull request for a session's branch. Without a number, Message
// explains why there is none to show.
type PullRequestStatus struct {
	Message        string             `json:"message,omitempty"`
	Branch         string             `json:"branch,omitempty"`
	Repository     string             `json:"repository,omitempty"`
	Number         int                `json:"number,omitempty"`
	URL            string             `json:"url,omitempty"`
	Title          string             `json:"title,omitempty"`
	State          string             `json:"state,omitempty"` // open, draft, merged or closed
	ReviewDecision string             `json:"reviewDecision,omitempty"`
	Checks         []PullRequestCheck `json:"checks"`
	// Reviews and Comments are GitHub REST objects with rendered bodies, read only on request.
	Reviews  []json.RawMessage `json:"reviews,omitempty"`
	Comments []json.RawMessage `json:"comments,omitempty"`
}

const pullRequestFields = "number,url,title,state,isDraft,headRefName,headRefOid,isCrossRepository,reviewDecision,statusCheckRollup"

type ghPullRequest struct {
	Number            int    `json:"number"`
	URL               string `json:"url"`
	Title             string `json:"title"`
	State             string `json:"state"`
	IsDraft           bool   `json:"isDraft"`
	HeadRefName       string `json:"headRefName"`
	HeadRefOid        string `json:"headRefOid"`
	IsCrossRepository bool   `json:"isCrossRepository"`
	ReviewDecision    string `json:"reviewDecision"`
	StatusCheckRollup []struct {
		Typename     string `json:"__typename"`
		Name         string `json:"name"`
		WorkflowName string `json:"workflowName"`
		Status       string `json:"status"`
		Conclusion   string `json:"conclusion"`
		DetailsURL   string `json:"detailsUrl"`
		StartedAt    string `json:"startedAt"`
		CompletedAt  string `json:"completedAt"`
		Context      string `json:"context"`
		State        string `json:"state"`
		TargetURL    string `json:"targetUrl"`
	} `json:"statusCheckRollup"`
}

var actionsJobRE = regexp.MustCompile(`/actions/runs/(\d+)/job/(\d+)`)

// SessionPullRequest only reads Git and GitHub. It never starts, stops or writes to a
// session's process. A session started from Review shows that pull request; others show
// the pull request whose head is the branch checked out in the session folder.
func (s *Store) SessionPullRequest(id string, activity bool) (PullRequestStatus, error) {
	s.mu.Lock()
	p, sess, err := s.sessionLocked(id)
	if err != nil {
		s.mu.Unlock()
		return PullRequestStatus{}, err
	}
	repo, projectDirectory, worktreeDirectory, number := p.Repository, p.Directory, sess.WorktreeDirectory, 0
	if sess.GitHubItem != nil && sess.GitHubItem.PullRequest {
		number = sess.GitHubItem.Number
	}
	s.mu.Unlock()
	result := PullRequestStatus{Checks: []PullRequestCheck{}, Repository: repo}
	if !repoRE.MatchString(repo) {
		result.Message = "This project has no GitHub remote."
		return result, nil
	}
	directory, err := sessionFolder(projectDirectory, worktreeDirectory)
	if err != nil {
		return PullRequestStatus{}, err
	}
	var pr *ghPullRequest
	if number > 0 {
		data, err := runCommand(directory, "gh", "pr", "view", strconv.Itoa(number), "--repo", repo, "--json", pullRequestFields)
		if err != nil {
			return PullRequestStatus{}, gitHubCLIError(err)
		}
		pr = &ghPullRequest{}
		if err = json.Unmarshal(data, pr); err != nil {
			return PullRequestStatus{}, errors.New("Invalid GitHub response.")
		}
		result.Branch = pr.HeadRefName
	} else {
		branch, head := sessionBranch(directory)
		if branch == "" {
			result.Message = "This session isn’t on a branch, so it has no pull request."
			return result, nil
		}
		result.Branch = branch
		data, err := runCommand(directory, "gh", "pr", "list", "--repo", repo, "--head", branch, "--state", "all", "--limit", "10", "--json", pullRequestFields)
		if err != nil {
			return PullRequestStatus{}, gitHubCLIError(err)
		}
		var candidates []ghPullRequest
		if err = json.Unmarshal(data, &candidates); err != nil {
			return PullRequestStatus{}, errors.New("Invalid GitHub response.")
		}
		pr = pickPullRequest(candidates, branch, head)
		if pr == nil {
			result.Message = "No pull request for " + branch + " yet. Push the branch and open one to see its checks here."
			return result, nil
		}
	}
	result.Number, result.URL, result.Title, result.ReviewDecision = pr.Number, pr.URL, pr.Title, pr.ReviewDecision
	result.State = pullRequestState(pr)
	result.Checks = pullRequestChecks(pr, time.Now())
	if activity {
		if result.Reviews, err = ghPages(directory, fmt.Sprintf("repos/%s/pulls/%d/reviews?per_page=100", repo, pr.Number)); err != nil {
			return PullRequestStatus{}, gitHubCLIError(err)
		}
		if result.Comments, err = ghPages(directory, fmt.Sprintf("repos/%s/issues/%d/comments?per_page=100", repo, pr.Number)); err != nil {
			return PullRequestStatus{}, gitHubCLIError(err)
		}
	}
	return result, nil
}

// sessionBranch is the branch name on the remote for the folder's checkout, and its head
// commit. The upstream name wins because a local branch can track one with another name.
func sessionBranch(directory string) (string, string) {
	data, err := runCommand(directory, "git", "rev-parse", "--abbrev-ref", "HEAD")
	branch := strings.TrimSpace(string(data))
	if err != nil || branch == "" || branch == "HEAD" {
		return "", ""
	}
	if data, err = runCommand(directory, "git", "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"); err == nil {
		upstream := strings.TrimSpace(string(data))
		if _, name, ok := strings.Cut(upstream, "/"); ok && name != "" {
			branch = name
		}
	}
	head := ""
	if data, err = runCommand(directory, "git", "rev-parse", "HEAD"); err == nil {
		head = strings.TrimSpace(string(data))
	}
	return branch, head
}

// pickPullRequest prefers an open pull request from this repository. A closed or merged one
// only counts while its head is still the local commit, so a reused branch name doesn't show
// an old pull request.
func pickPullRequest(candidates []ghPullRequest, branch, head string) *ghPullRequest {
	var found *ghPullRequest
	for i := range candidates {
		c := &candidates[i]
		if c.IsCrossRepository || c.HeadRefName != branch {
			continue
		}
		if c.State == "OPEN" {
			return c
		}
		if found == nil && head != "" && c.HeadRefOid == head {
			found = c
		}
	}
	return found
}

func pullRequestState(pr *ghPullRequest) string {
	switch {
	case pr.State == "MERGED":
		return "merged"
	case pr.State != "OPEN":
		return "closed"
	case pr.IsDraft:
		return "draft"
	}
	return "open"
}

// pullRequestChecks keeps the latest run of each check, in GitHub's order. Re-running a
// workflow leaves the earlier runs in the rollup under the same name.
func pullRequestChecks(pr *ghPullRequest, now time.Time) []PullRequestCheck {
	checks := []PullRequestCheck{}
	type seen struct {
		index   int
		run     int64
		started string
	}
	latest := map[string]seen{}
	for _, c := range pr.StatusCheckRollup {
		check := PullRequestCheck{}
		var run int64
		if c.Typename == "StatusContext" {
			check.Name, check.URL, check.Status = c.Context, c.TargetURL, contextStatus(c.State)
		} else {
			check.Name, check.Workflow, check.URL = c.Name, c.WorkflowName, c.DetailsURL
			check.Status = checkRunStatus(c.Status, c.Conclusion)
			check.Duration = checkDuration(c.StartedAt, c.CompletedAt, check.Status == "pending", now)
			if m := actionsJobRE.FindStringSubmatch(c.DetailsURL); m != nil {
				run, _ = strconv.ParseInt(m[1], 10, 64)
				check.JobID, _ = strconv.ParseInt(m[2], 10, 64)
			}
		}
		if check.Name == "" {
			continue
		}
		key := check.Workflow + "\x00" + check.Name
		if previous, ok := latest[key]; ok {
			if run < previous.run || (run == previous.run && c.StartedAt <= previous.started) {
				continue
			}
			checks[previous.index] = check
			latest[key] = seen{previous.index, run, c.StartedAt}
			continue
		}
		latest[key] = seen{len(checks), run, c.StartedAt}
		checks = append(checks, check)
	}
	return checks
}

func checkRunStatus(status, conclusion string) string {
	if status != "COMPLETED" {
		return "pending"
	}
	switch conclusion {
	case "SUCCESS":
		return "success"
	case "FAILURE", "TIMED_OUT", "ACTION_REQUIRED", "STARTUP_FAILURE":
		return "failure"
	case "CANCELLED":
		return "cancelled"
	case "SKIPPED", "NEUTRAL", "STALE":
		return "skipped"
	}
	return "pending"
}

func contextStatus(state string) string {
	switch state {
	case "SUCCESS":
		return "success"
	case "FAILURE", "ERROR":
		return "failure"
	}
	return "pending"
}

// checkDuration formats the time a check ran, such as "16m 18s". A running check counts
// until now.
func checkDuration(started, completed string, running bool, now time.Time) string {
	start, err := time.Parse(time.RFC3339, started)
	if err != nil || start.Year() < 2000 {
		return ""
	}
	end, err := time.Parse(time.RFC3339, completed)
	if running || err != nil || end.Year() < 2000 {
		if !running {
			return ""
		}
		end = now
	}
	seconds := int(end.Sub(start).Round(time.Second) / time.Second)
	if seconds < 0 {
		seconds = 0
	}
	parts := []string{}
	if h := seconds / 3600; h > 0 {
		parts = append(parts, fmt.Sprintf("%dh", h))
	}
	if m := seconds % 3600 / 60; m > 0 {
		parts = append(parts, fmt.Sprintf("%dm", m))
	}
	if sec := seconds % 60; sec > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", sec))
	}
	return strings.Join(parts, " ")
}

// gitHubCLIError replaces gh's sign-in errors with how to sign in.
func gitHubCLIError(err error) error {
	text := strings.ToLower(err.Error())
	for _, signal := range []string{"gh auth login", "not logged into", "authentication required", "bad credentials", "http 401"} {
		if strings.Contains(text, signal) {
			return errors.New("Sign in to the GitHub CLI with “gh auth login” to see pull request checks.")
		}
	}
	return err
}

func ghPages(directory, endpoint string) ([]json.RawMessage, error) {
	data, err := ghAPI(directory, endpoint, true, true)
	if err != nil {
		return nil, err
	}
	var pages [][]json.RawMessage
	if err = json.Unmarshal(data, &pages); err != nil {
		return nil, err
	}
	items := []json.RawMessage{}
	for _, page := range pages {
		items = append(items, page...)
	}
	return items, nil
}

var logTimestampRE = regexp.MustCompile(`^\d{4}-\d\d-\d\dT[\d:.]+Z ?`)

// PullRequestCheckFailure describes a failed GitHub Actions job as plain text to paste into
// an agent: its output, annotations and the end of its log.
func (s *Store) PullRequestCheckFailure(id string, jobID int64) (string, error) {
	s.mu.Lock()
	p, sess, err := s.sessionLocked(id)
	if err != nil {
		s.mu.Unlock()
		return "", err
	}
	repo, projectDirectory, worktreeDirectory := p.Repository, p.Directory, sess.WorktreeDirectory
	s.mu.Unlock()
	if !repoRE.MatchString(repo) {
		return "", errors.New("This project has no GitHub remote.")
	}
	if jobID < 1 {
		return "", errors.New("Invalid check.")
	}
	directory, err := sessionFolder(projectDirectory, worktreeDirectory)
	if err != nil {
		return "", err
	}
	data, err := ghAPI(directory, fmt.Sprintf("repos/%s/check-runs/%d", repo, jobID), false, false)
	if err != nil {
		return "", gitHubCLIError(err)
	}
	var run struct {
		Name       string `json:"name"`
		Conclusion string `json:"conclusion"`
		HTMLURL    string `json:"html_url"`
		Output     struct {
			Title   string `json:"title"`
			Summary string `json:"summary"`
			Text    string `json:"text"`
		} `json:"output"`
	}
	if err = json.Unmarshal(data, &run); err != nil {
		return "", errors.New("Invalid GitHub response.")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Check: %s\n", run.Name)
	if run.Conclusion != "" {
		fmt.Fprintf(&b, "Conclusion: %s\n", run.Conclusion)
	}
	if run.HTMLURL != "" {
		fmt.Fprintf(&b, "URL: %s\n", run.HTMLURL)
	}
	for _, text := range []string{run.Output.Title, run.Output.Summary, run.Output.Text} {
		if text = strings.TrimSpace(text); text != "" {
			fmt.Fprintf(&b, "\n%s\n", text)
		}
	}
	// Annotations and logs are optional: a check can have neither, and logs expire.
	if data, err = ghAPI(directory, fmt.Sprintf("repos/%s/check-runs/%d/annotations?per_page=%d", repo, jobID, maxCheckAnnotation), false, false); err == nil {
		var annotations []struct {
			Path      string `json:"path"`
			StartLine int    `json:"start_line"`
			EndLine   int    `json:"end_line"`
			Level     string `json:"annotation_level"`
			Message   string `json:"message"`
		}
		if json.Unmarshal(data, &annotations) == nil && len(annotations) > 0 {
			b.WriteString("\nAnnotations:\n")
			for _, a := range annotations {
				location := a.Path
				if a.StartLine > 0 {
					location += ":" + strconv.Itoa(a.StartLine)
					if a.EndLine > a.StartLine {
						location += "-" + strconv.Itoa(a.EndLine)
					}
				}
				fmt.Fprintf(&b, "- %s %s: %s\n", location, a.Level, strings.TrimSpace(a.Message))
			}
		}
	}
	if data, err = runCommand(directory, "gh", "api", "--hostname", "github.com", fmt.Sprintf("repos/%s/actions/jobs/%d/logs", repo, jobID)); err == nil {
		tail, truncated := logTail(string(data))
		if tail != "" {
			b.WriteString("\nLog")
			if truncated {
				b.WriteString(" (last lines)")
			}
			b.WriteString(":\n```\n" + tail + "\n```\n")
		}
	} else {
		b.WriteString("\nThe job log is unavailable.\n")
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// logTail keeps the last lines of a job log without timestamps or terminal colors.
func logTail(log string) (string, bool) {
	lines := strings.Split(strings.TrimRight(strings.ReplaceAll(log, "\r\n", "\n"), "\n"), "\n")
	truncated := false
	if len(lines) > maxCheckLogLines {
		lines, truncated = lines[len(lines)-maxCheckLogLines:], true
	}
	for i, line := range lines {
		lines[i] = ansiRE.ReplaceAllString(logTimestampRE.ReplaceAllString(line, ""), "")
	}
	tail := strings.Join(lines, "\n")
	if len(tail) > maxCheckLogBytes {
		tail = tail[len(tail)-maxCheckLogBytes:]
		if i := strings.IndexByte(tail, '\n'); i >= 0 {
			tail = tail[i+1:]
		}
		truncated = true
	}
	return strings.TrimSpace(tail), truncated
}
