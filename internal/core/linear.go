package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

type LinearSource struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	TeamID       string `json:"teamID"`
	ProjectID    string `json:"projectID,omitempty"`
	RepositoryID string `json:"repositoryID,omitempty"`
}

type LinearChoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type LinearClient struct {
	Key      string
	HTTP     *http.Client
	Endpoint string
}

// Never return response bodies, GraphQL messages or transport errors: they may echo credentials.
func (c LinearClient) query(ctx context.Context, query string, variables any, result any) error {
	if c.Key == "" {
		return errors.New("Connect Linear in Settings → Integrations.")
	}
	data, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return errors.New("Invalid Linear request.")
	}
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = "https://api.linear.app/graphql"
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return errors.New("Invalid Linear endpoint.")
	}
	req.Header.Set("Authorization", c.Key)
	req.Header.Set("Content-Type", "application/json")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	// Do not forward the API key to redirects.
	safeClient := *client
	safeClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := safeClient.Do(req)
	if err != nil {
		return errors.New("Could not reach Linear. Try again.")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return errors.New("Linear authentication failed. Reconnect in Settings → Integrations.")
	}
	if resp.StatusCode == 429 {
		return errors.New("Linear rate limit reached. Wait before refreshing.")
	}
	if resp.StatusCode != http.StatusOK {
		return errors.New("Linear request failed. Try again later.")
	}
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Extensions struct {
				Code string `json:"code"`
			} `json:"extensions"`
		} `json:"errors"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&envelope) != nil {
		return errors.New("Invalid Linear response.")
	}
	for _, e := range envelope.Errors {
		switch e.Extensions.Code {
		case "RATELIMITED":
			return errors.New("Linear rate limit reached. Wait before refreshing.")
		case "UNAUTHENTICATED", "AUTHENTICATION_ERROR", "FORBIDDEN":
			return errors.New("Linear authentication failed. Reconnect in Settings → Integrations.")
		}
	}
	if len(envelope.Errors) > 0 {
		return errors.New("Linear could not complete the request. Check access to this team or project.")
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" || json.Unmarshal(envelope.Data, result) != nil {
		return errors.New("Invalid Linear response.")
	}
	return nil
}

func (c LinearClient) Viewer(ctx context.Context) (LinearChoice, error) {
	var result struct {
		Viewer LinearChoice `json:"viewer"`
	}
	err := c.query(ctx, `query { viewer { id name } }`, nil, &result)
	if err == nil && result.Viewer.ID == "" {
		err = errors.New("Invalid Linear account response.")
	}
	return result.Viewer, err
}

type linearPageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

func nextLinearPage(info linearPageInfo, previous string) (string, error) {
	if info.EndCursor == "" || info.EndCursor == previous {
		return "", errors.New("Invalid Linear pagination cursor.")
	}
	return info.EndCursor, nil
}

func (c LinearClient) Choices(ctx context.Context, team string) ([]LinearChoice, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	items := []LinearChoice{}
	cursor := ""
	for {
		var result struct {
			Teams struct {
				Nodes    []LinearChoice `json:"nodes"`
				PageInfo linearPageInfo `json:"pageInfo"`
			} `json:"teams"`
			Team *struct {
				Projects struct {
					Nodes    []LinearChoice `json:"nodes"`
					PageInfo linearPageInfo `json:"pageInfo"`
				} `json:"projects"`
			} `json:"team"`
		}
		query := `query($after:String) { teams(first:100, after:$after) { nodes { id name } pageInfo { hasNextPage endCursor } } }`
		vars := map[string]any{"after": nil}
		if cursor != "" {
			vars["after"] = cursor
		}
		if team != "" {
			query = `query($id:String!, $after:String) { team(id:$id) { projects(first:100, after:$after) { nodes { id name } pageInfo { hasNextPage endCursor } } } }`
			vars["id"] = team
		}
		if err := c.query(ctx, query, vars, &result); err != nil {
			return nil, err
		}
		nodes, info := result.Teams.Nodes, result.Teams.PageInfo
		if team != "" {
			if result.Team == nil {
				return nil, errors.New("Linear team is unavailable.")
			}
			nodes, info = result.Team.Projects.Nodes, result.Team.Projects.PageInfo
		}
		items = append(items, nodes...)
		if !info.HasNextPage {
			return items, nil
		}
		var err error
		cursor, err = nextLinearPage(info, cursor)
		if err != nil {
			return nil, err
		}
	}
}

type LinearPerson struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl"`
}
type LinearIssue struct {
	ID          string `json:"id"`
	Identifier  string `json:"identifier"`
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	UpdatedAt   string `json:"updatedAt"`
	CreatedAt   string `json:"createdAt"`
	State       struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"state"`
	Assignee *LinearPerson `json:"assignee"`
	Creator  *LinearPerson `json:"creator"`
	Labels   struct {
		Nodes []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Color string `json:"color"`
		} `json:"nodes"`
	} `json:"labels"`
	Team    LinearChoice  `json:"team"`
	Project *LinearChoice `json:"project"`
}

const linearIssueFields = `id identifier title url updatedAt createdAt state { name type } assignee { id name avatarUrl } creator { id name avatarUrl } labels { nodes { id name color } } team { id name } project { id name }`

func (c LinearClient) Items(ctx context.Context, source LinearSource, state string) ([]LinearIssue, error) {
	if state != "open" && state != "closed" && state != "all" {
		return nil, errors.New("Invalid Linear state.")
	}
	filter := map[string]any{"team": map[string]any{"id": map[string]string{"eq": source.TeamID}}}
	if source.ProjectID != "" {
		filter["project"] = map[string]any{"id": map[string]string{"eq": source.ProjectID}}
	}
	if state != "all" {
		op := "nin"
		if state == "closed" {
			op = "in"
		}
		filter["state"] = map[string]any{"type": map[string]any{op: []string{"completed", "canceled"}}}
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	items := []LinearIssue{}
	cursor := ""
	for {
		var result struct {
			Issues struct {
				Nodes    []LinearIssue  `json:"nodes"`
				PageInfo linearPageInfo `json:"pageInfo"`
			} `json:"issues"`
		}
		vars := map[string]any{"filter": filter, "after": nil}
		if cursor != "" {
			vars["after"] = cursor
		}
		if err := c.query(ctx, `query($filter:IssueFilter!, $after:String) { issues(first:100, after:$after, filter:$filter, orderBy:updatedAt) { nodes { `+linearIssueFields+` } pageInfo { hasNextPage endCursor } } }`, vars, &result); err != nil {
			return nil, err
		}
		items = append(items, result.Issues.Nodes...)
		if !result.Issues.PageInfo.HasNextPage {
			return items, nil
		}
		var err error
		cursor, err = nextLinearPage(result.Issues.PageInfo, cursor)
		if err != nil {
			return nil, err
		}
	}
}
func (c LinearClient) Detail(ctx context.Context, source LinearSource, id string) (LinearIssue, error) {
	var result struct {
		Issue *LinearIssue `json:"issue"`
	}
	if err := c.query(ctx, `query($id:String!) { issue(id:$id) { `+linearIssueFields+` description } }`, map[string]string{"id": id}, &result); err != nil {
		return LinearIssue{}, err
	}
	if result.Issue == nil || result.Issue.Team.ID != source.TeamID || (source.ProjectID != "" && (result.Issue.Project == nil || result.Issue.Project.ID != source.ProjectID)) {
		return LinearIssue{}, errors.New("Linear issue is unavailable in this source.")
	}
	return *result.Issue, nil
}
func validateWorkItemReference(ref WorkItemReference) error {
	u, err := url.Parse(ref.URL)
	if err != nil || u.Scheme != "https" || u.User != nil || ref.ID == "" || (ref.Provider != "github" && ref.Provider != "linear") || (ref.Provider == "linear" && u.Host != "linear.app") || (ref.Provider == "github" && u.Host != "github.com") {
		return errors.New("Invalid work item reference.")
	}
	return nil
}
func (s *Store) LinearSource(workspaceID, sourceID string) (LinearSource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.workspaceLocked(workspaceID)
	if err != nil {
		return LinearSource{}, err
	}
	for _, source := range p.LinearSources {
		if source.ID == sourceID {
			return source, nil
		}
	}
	return LinearSource{}, errors.New("Linear source not found.")
}
func (s *Store) SaveLinearSource(workspaceID string, source LinearSource) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.workspaceLocked(workspaceID)
	if err != nil {
		return State{}, err
	}
	source.Name = strings.TrimSpace(source.Name)
	if source.Name == "" || source.TeamID == "" {
		return State{}, errors.New("Choose a Linear team.")
	}
	if source.RepositoryID != "" {
		found := false
		for _, repo := range p.Repositories {
			if repo.ID == source.RepositoryID {
				found = true
			}
		}
		if !found {
			return State{}, errors.New("Choose a repository in this workspace.")
		}
	}
	for i, old := range p.LinearSources {
		if old.ID == source.ID {
			p.LinearSources[i] = source
			s.changedLocked()
			return s.snapshotLocked(), nil
		}
	}
	if source.ID != "" {
		return State{}, errors.New("Linear source not found.")
	}
	source.ID = "linear:" + uuid.NewString()
	p.LinearSources = append(p.LinearSources, source)
	s.changedLocked()
	return s.snapshotLocked(), nil
}
func (s *Store) RemoveLinearSource(workspaceID, sourceID string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.workspaceLocked(workspaceID)
	if err != nil {
		return State{}, err
	}
	for i, source := range p.LinearSources {
		if source.ID == sourceID {
			p.LinearSources = append(p.LinearSources[:i], p.LinearSources[i+1:]...)
			s.changedLocked()
			return s.snapshotLocked(), nil
		}
	}
	return State{}, errors.New("Linear source not found.")
}
