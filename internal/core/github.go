package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

var repoRE = regexp.MustCompile(`^[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$`)

func ParseRepository(remote string) string {
	remote = strings.TrimSpace(remote)
	path := ""
	if strings.HasPrefix(remote, "git@github.com:") {
		path = strings.TrimPrefix(remote, "git@github.com:")
	} else {
		u, err := url.Parse(remote)
		if err != nil {
			return ""
		}
		host := strings.ToLower(u.Hostname())
		if !(host == "github.com" || (u.Scheme == "ssh" && host == "ssh.github.com")) {
			return ""
		}
		if u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "ssh" && u.Scheme != "git" {
			return ""
		}
		path = strings.Trim(u.Path, "/")
	}
	path = strings.TrimSuffix(path, ".git")
	if !repoRE.MatchString(path) {
		return ""
	}
	for _, part := range strings.Split(path, "/") {
		if part == "." || part == ".." {
			return ""
		}
	}
	return path
}

// Resolve before constructing exec.Cmd: CommandContext caches failed LookPath errors.
func githubCommand(ctx context.Context, searchPath string, args ...string) (*exec.Cmd, error) {
	for _, directory := range filepath.SplitList(searchPath) {
		if directory == "" || !filepath.IsAbs(directory) {
			continue
		}
		if path, err := exec.LookPath(filepath.Join(directory, "gh")); err == nil {
			return exec.CommandContext(ctx, path, args...), nil
		}
	}
	return nil, errors.New("GitHub CLI (gh) was not found. Install it or add its directory to PATH.")
}
func runCommand(directory, command string, args ...string) ([]byte, error) {
	return runCommandInput(directory, command, nil, args...)
}
func runCommandInput(directory, command string, input []byte, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return runCommandContext(ctx, directory, command, input, args...)
}

// runCommandContext runs a command until ctx ends, for work such as downloads that can
// take longer than the usual timeout.
func runCommandContext(ctx context.Context, directory, command string, input []byte, args ...string) ([]byte, error) {
	env := environmentMap()
	env["GH_PROMPT_DISABLED"] = "1"
	env["GIT_TERMINAL_PROMPT"] = "0"
	env["PATH"] += ":/opt/homebrew/bin:/usr/local/bin"
	var cmd *exec.Cmd
	if command == "gh" {
		var err error
		cmd, err = githubCommand(ctx, env["PATH"], args...)
		if err != nil {
			return nil, err
		}
	} else {
		cmd = exec.CommandContext(ctx, command, args...)
	}
	cmd.Dir = directory
	cmd.Env = environmentList(env)
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	// Warnings on stderr must not corrupt parsed output, but still explain failures.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String() + "\n" + string(data))
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%s: request timed out", command)
		}
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("%s: %s", command, detail)
	}
	return data, nil
}
func (s *Store) DetectRepository(projectID string) (State, error) {
	s.mu.Lock()
	p, err := s.projectLocked(projectID)
	if err != nil {
		s.mu.Unlock()
		return State{}, err
	}
	directory := p.Directory
	s.mu.Unlock()
	data, err := runCommand(directory, "/usr/bin/git", "-C", directory, "remote")
	repo := ""
	if err == nil {
		remotes := strings.Fields(string(data))
		rank := func(name string) int {
			if name == "origin" {
				return 0
			}
			if name == "upstream" {
				return 1
			}
			return 2
		}
		sort.Slice(remotes, func(i, j int) bool {
			if rank(remotes[i]) != rank(remotes[j]) {
				return rank(remotes[i]) < rank(remotes[j])
			}
			return remotes[i] < remotes[j]
		})
		for _, remote := range remotes {
			value, e := runCommand(directory, "/usr/bin/git", "-C", directory, "remote", "get-url", remote)
			if e == nil {
				repo = ParseRepository(string(value))
				if repo != "" {
					break
				}
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err = s.projectLocked(projectID)
	if err != nil {
		return State{}, err
	}
	p.Repository = repo
	s.changedLocked()
	return s.snapshotLocked(), nil
}
func (s *Store) repository(projectID string) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.projectLocked(projectID)
	if err != nil {
		return "", "", err
	}
	if !repoRE.MatchString(p.Repository) {
		return "", "", errors.New("This project has no GitHub remote.")
	}
	return p.Repository, p.Directory, nil
}
func ghAPI(directory, endpoint string, paginate, html bool) (json.RawMessage, error) {
	args := []string{"api", "--hostname", "github.com"}
	if paginate {
		args = append(args, "--paginate", "--slurp")
	}
	args = append(args, endpoint)
	if html {
		// Full media type returns both the Markdown source and its rendered HTML.
		args = append(args, "--header", "Accept: application/vnd.github.full+json")
	}
	data, err := runCommand(directory, "gh", args...)
	if err != nil {
		return nil, err
	}
	if !json.Valid(data) {
		return nil, errors.New("Invalid GitHub response.")
	}
	return data, nil
}
func (s *Store) GitHubItems(projectID, state string) ([]json.RawMessage, error) {
	repo, dir, err := s.repository(projectID)
	if err != nil {
		return nil, err
	}
	if state != "open" && state != "closed" && state != "all" {
		return nil, errors.New("Invalid GitHub state.")
	}
	data, err := ghAPI(dir, "repos/"+repo+"/issues?state="+state+"&sort=updated&direction=desc&per_page=100", true, false)
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

type GitHubContent struct {
	Detail   json.RawMessage   `json:"detail"`
	Comments []json.RawMessage `json:"comments"`
}

func (s *Store) GitHubDetail(projectID string, number int, pull bool) (GitHubContent, error) {
	repo, dir, err := s.repository(projectID)
	if err != nil {
		return GitHubContent{}, err
	}
	if number < 1 {
		return GitHubContent{}, errors.New("Invalid item number.")
	}
	kind := "issues"
	if pull {
		kind = "pulls"
	}
	detail, err := ghAPI(dir, fmt.Sprintf("repos/%s/%s/%d", repo, kind, number), false, true)
	if err != nil {
		return GitHubContent{}, err
	}
	data, err := ghAPI(dir, fmt.Sprintf("repos/%s/issues/%d/comments?per_page=100", repo, number), true, true)
	if err != nil {
		return GitHubContent{}, err
	}
	var pages [][]json.RawMessage
	if err = json.Unmarshal(data, &pages); err != nil {
		return GitHubContent{}, err
	}
	comments := []json.RawMessage{}
	for _, page := range pages {
		comments = append(comments, page...)
	}
	return GitHubContent{detail, comments}, nil
}

// Repository choices are paginated; existing selections are retained by the editor.
func (s *Store) GitHubMetadataOptions(projectID, field string) ([]json.RawMessage, error) {
	repo, dir, err := s.repository(projectID)
	if err != nil {
		return nil, err
	}
	if field != "assignees" && field != "labels" {
		return nil, errors.New("Invalid metadata field.")
	}
	data, err := ghAPI(dir, "repos/"+repo+"/"+field+"?per_page=100", true, false)
	if err != nil {
		return nil, err
	}
	var pages [][]json.RawMessage
	if err = json.Unmarshal(data, &pages); err != nil {
		return nil, err
	}
	options := []json.RawMessage{}
	for _, page := range pages {
		options = append(options, page...)
	}
	return options, nil
}

var githubLoginRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,38}(\[bot\])?$`)

// Patch only the selected field, preserving unrelated metadata and content.
func (s *Store) UpdateGitHubMetadata(projectID string, number int, pull bool, field string, values []string) error {
	repo, dir, err := s.repository(projectID)
	if err != nil {
		return err
	}
	if number < 1 {
		return errors.New("Invalid item number.")
	}
	payload := map[string]any{}
	switch field {
	case "body":
		// Values are the new Markdown and the source the edit started from.
		if len(values) != 2 || len(values[0]) > 262144 || strings.ContainsRune(values[0], 0) {
			return errors.New("Invalid description.")
		}
		data, e := ghAPI(dir, fmt.Sprintf("repos/%s/issues/%d", repo, number), false, false)
		if e != nil {
			return e
		}
		var current struct {
			Body *string `json:"body"`
		}
		if e = json.Unmarshal(data, &current); e != nil {
			return e
		}
		if (current.Body != nil && *current.Body != values[1]) || (current.Body == nil && values[1] != "") {
			return errors.New("The description changed on GitHub since you started editing. Copy your text, cancel and edit again.")
		}
		payload[field] = values[0]
	case "state":
		if len(values) < 1 || len(values) > 3 || (values[0] != "open" && values[0] != "closed") {
			return errors.New("Invalid GitHub state.")
		}
		payload[field] = values[0]
		if len(values) > 1 {
			if pull || values[0] != "closed" || (values[1] != "completed" && values[1] != "not_planned" && values[1] != "duplicate") {
				return errors.New("Invalid issue closing reason.")
			}
			payload["state_reason"] = values[1]
			if values[1] == "duplicate" {
				if len(values) != 3 {
					return errors.New("Choose the issue this duplicates.")
				}
				canonicalNumber, e := strconv.Atoi(values[2])
				if e != nil || canonicalNumber < 1 || canonicalNumber == number {
					return errors.New("Invalid duplicate issue number.")
				}
				data, e := ghAPI(dir, fmt.Sprintf("repos/%s/issues/%d", repo, canonicalNumber), false, false)
				if e != nil {
					return e
				}
				var canonical struct {
					ID     int64           `json:"id"`
					Number int             `json:"number"`
					Pull   json.RawMessage `json:"pull_request"`
				}
				if e = json.Unmarshal(data, &canonical); e != nil {
					return e
				}
				if canonical.ID < 1 || canonical.Number != canonicalNumber || (len(canonical.Pull) > 0 && string(canonical.Pull) != "null") {
					return errors.New("Choose an issue from this repository, not a pull request.")
				}
				payload["duplicate_issue_id"] = canonical.ID
			} else if len(values) != 2 {
				return errors.New("Invalid issue closing reason.")
			}
		}
	case "assignees", "labels":
		limit := 100
		if field == "assignees" {
			limit = 10
		}
		if len(values) > limit {
			return errors.New("Too many selections.")
		}
		seen := map[string]bool{}
		for _, value := range values {
			if strings.TrimSpace(value) == "" || len(value) > 200 || strings.ContainsAny(value, "\x00\r\n") || seen[value] {
				return errors.New("Invalid metadata selection.")
			}
			if field == "assignees" && !githubLoginRE.MatchString(value) {
				return errors.New("Invalid assignee login.")
			}
			seen[value] = true
		}
		if values == nil {
			values = []string{}
		}
		payload[field] = values
	default:
		return errors.New("Invalid metadata field.")
	}
	kind := "issues"
	if pull && field == "state" {
		kind = "pulls"
		data, e := ghAPI(dir, fmt.Sprintf("repos/%s/pulls/%d", repo, number), false, false)
		if e != nil {
			return e
		}
		var current struct {
			Merged bool `json:"merged"`
		}
		if e = json.Unmarshal(data, &current); e != nil {
			return e
		}
		if current.Merged {
			return errors.New("A merged pull request cannot change status.")
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	data, err := runCommandInput(dir, "gh", body, "api", "--hostname", "github.com", "--method", "PATCH", fmt.Sprintf("repos/%s/%s/%d", repo, kind, number), "--header", "X-GitHub-Api-Version: 2026-03-10", "--input", "-")
	if err != nil {
		return err
	}
	// GitHub can silently ignore assignments/labels without sufficient permissions.
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if field == "body" {
		var updated struct {
			Body *string `json:"body"`
		}
		if err = json.Unmarshal(data, &updated); err != nil || fields == nil {
			return errors.New("Invalid GitHub update response.")
		}
		normalize := func(text string) string { return strings.ReplaceAll(text, "\r\n", "\n") }
		if (updated.Body == nil && values[0] != "") || (updated.Body != nil && normalize(*updated.Body) != normalize(values[0])) {
			return errors.New("GitHub did not save the description. Check repository permissions and refresh before retrying.")
		}
		return nil
	}
	if fields[field] == nil || string(fields[field]) == "null" {
		return errors.New("Invalid GitHub update response.")
	}
	var updated struct {
		State       string `json:"state"`
		StateReason string `json:"state_reason"`
		Assignees   []struct {
			Login string `json:"login"`
		} `json:"assignees"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	if err = json.Unmarshal(data, &updated); err != nil {
		return err
	}
	actual := []string{}
	switch field {
	case "state":
		actual = append(actual, updated.State)
		if len(values) > 1 {
			actual = append(actual, updated.StateReason)
			if len(values) == 3 {
				actual = append(actual, values[2])
			}
		}
	case "assignees":
		for _, a := range updated.Assignees {
			actual = append(actual, a.Login)
		}
	case "labels":
		for _, l := range updated.Labels {
			actual = append(actual, l.Name)
		}
	}
	expected := slices.Clone(values)
	slices.Sort(actual)
	slices.Sort(expected)
	if !slices.Equal(actual, expected) {
		return errors.New("GitHub did not apply all selections. Check repository permissions and refresh before retrying.")
	}
	return nil
}
