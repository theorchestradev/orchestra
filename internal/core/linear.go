package core

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
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

// errLinearForbidden is a refusal of the key's access, which edits explain as a missing write scope.
var errLinearForbidden = errors.New("Linear authentication failed. Reconnect in Edit Workspace → Work Items.")

// Never return response bodies, GraphQL messages or transport errors: they may echo credentials.
func (c LinearClient) query(ctx context.Context, query string, variables any, result any) error {
	if c.Key == "" {
		return errors.New("Connect Linear in Edit Workspace → Work Items.")
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
	if resp.StatusCode == 401 {
		return errors.New("Linear authentication failed. Reconnect in Edit Workspace → Work Items.")
	}
	if resp.StatusCode == 403 {
		return errLinearForbidden
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
		case "UNAUTHENTICATED", "AUTHENTICATION_ERROR":
			return errors.New("Linear authentication failed. Reconnect in Edit Workspace → Work Items.")
		case "FORBIDDEN":
			return errLinearForbidden
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

type linearConnection[T any] struct {
	Nodes    []T            `json:"nodes"`
	PageInfo linearPageInfo `json:"pageInfo"`
}

// linearAll reads every page of the connection at path in the result of query, which takes $after.
func linearAll[T any](ctx context.Context, c LinearClient, query string, vars map[string]any, path ...string) ([]T, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	items := []T{}
	cursor := ""
	for {
		vars["after"] = nil
		if cursor != "" {
			vars["after"] = cursor
		}
		var result json.RawMessage
		if err := c.query(ctx, query, vars, &result); err != nil {
			return nil, err
		}
		for _, key := range path {
			var fields map[string]json.RawMessage
			if json.Unmarshal(result, &fields) != nil || len(fields[key]) == 0 || string(fields[key]) == "null" {
				return nil, errors.New("Invalid Linear response.")
			}
			result = fields[key]
		}
		var page linearConnection[T]
		if json.Unmarshal(result, &page) != nil {
			return nil, errors.New("Invalid Linear response.")
		}
		items = append(items, page.Nodes...)
		if !page.PageInfo.HasNextPage {
			return items, nil
		}
		var err error
		if cursor, err = nextLinearPage(page.PageInfo, cursor); err != nil {
			return nil, err
		}
	}
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
	Active    *bool  `json:"active,omitempty"`
}

// LinearState is a workflow status of a team. Type is "triage", "backlog", "unstarted",
// "started", "completed" or "canceled".
type LinearState struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Position float64 `json:"position"`
}

type LinearLabel struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
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
		Nodes []LinearLabel `json:"nodes"`
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

var linearStateOrder = []string{"triage", "backlog", "unstarted", "started", "completed", "canceled"}

// States lists a team's workflow statuses in Linear's order.
func (c LinearClient) States(ctx context.Context, team string) ([]LinearState, error) {
	states, err := linearAll[LinearState](ctx, c, `query($id:String!, $after:String) { team(id:$id) { states(first:100, after:$after) { nodes { id name type position } pageInfo { hasNextPage endCursor } } } }`, map[string]any{"id": team}, "team", "states")
	slices.SortStableFunc(states, func(a, b LinearState) int {
		if order := slices.Index(linearStateOrder, a.Type) - slices.Index(linearStateOrder, b.Type); order != 0 {
			return order
		}
		return cmp.Compare(a.Position, b.Position)
	})
	return states, err
}

// Members lists a team's active members.
func (c LinearClient) Members(ctx context.Context, team string) ([]LinearPerson, error) {
	members, err := linearAll[LinearPerson](ctx, c, `query($id:String!, $after:String) { team(id:$id) { members(first:100, after:$after) { nodes { id name avatarUrl active } pageInfo { hasNextPage endCursor } } } }`, map[string]any{"id": team}, "team", "members")
	return slices.DeleteFunc(members, func(p LinearPerson) bool { return p.Active != nil && !*p.Active }), err
}

// Labels lists the labels a team's issues may use: the team's and the workspace's, without
// groups, which only hold other labels.
func (c LinearClient) Labels(ctx context.Context, team string) ([]LinearLabel, error) {
	filter := map[string]any{
		"isGroup": map[string]bool{"eq": false},
		"or":      []any{map[string]any{"team": map[string]any{"id": map[string]string{"eq": team}}}, map[string]any{"team": map[string]bool{"null": true}}},
	}
	return linearAll[LinearLabel](ctx, c, `query($filter:IssueLabelFilter!, $after:String) { issueLabels(first:100, after:$after, filter:$filter) { nodes { id name color } pageInfo { hasNextPage endCursor } } }`, map[string]any{"filter": filter}, "issueLabels")
}

// UpdateIssue applies input, an IssueUpdateInput, to an issue.
func (c LinearClient) UpdateIssue(ctx context.Context, id string, input map[string]any) error {
	var result struct {
		IssueUpdate struct {
			Success bool `json:"success"`
		} `json:"issueUpdate"`
	}
	err := c.query(ctx, `mutation($id:String!, $input:IssueUpdateInput!) { issueUpdate(id:$id, input:$input) { success } }`, map[string]any{"id": id, "input": input}, &result)
	if errors.Is(err, errLinearForbidden) {
		return errors.New("This Linear API key cannot edit issues. Reconnect Linear in Edit Workspace → Work Items with a key that has write access.")
	}
	if err == nil && !result.IssueUpdate.Success {
		err = errors.New("Linear could not update the issue.")
	}
	return err
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
			p.HiddenSources = slices.DeleteFunc(p.HiddenSources, func(id string) bool { return id == sourceID })
			s.changedLocked()
			return s.snapshotLocked(), nil
		}
	}
	return State{}, errors.New("Linear source not found.")
}

// linearProvider reads the issues of a Linear team, or of one of its projects.
type linearProvider struct{}

func linearWorkItem(issue LinearIssue, source Source) WorkItem {
	state := "open"
	if issue.State.Type == "completed" || issue.State.Type == "canceled" {
		state = "closed"
	}
	item := WorkItem{
		Provider: "linear", SourceID: source.ID, RepositoryID: source.RepositoryID,
		ID: issue.ID, Identifier: issue.Identifier, Title: issue.Title, URL: issue.URL,
		State: state, Status: issue.State.Name, Assignees: []Person{}, Labels: []Label{},
		CreatedAt: issue.CreatedAt, UpdatedAt: issue.UpdatedAt,
		Can: ItemCapabilities{EditState: true, EditAssignees: true, EditLabels: true, ChooseRepository: true},
	}
	person := func(p *LinearPerson) *Person {
		if p == nil {
			return nil
		}
		return &Person{ID: p.ID, Login: p.Name, AvatarURL: p.AvatarURL}
	}
	item.Author = person(issue.Creator)
	if issue.Assignee != nil {
		item.Assignees = append(item.Assignees, *person(issue.Assignee))
	}
	for _, label := range issue.Labels.Nodes {
		item.Labels = append(item.Labels, linearLabel(label))
	}
	return item
}

func linearLabel(label LinearLabel) Label {
	return Label{ID: label.ID, Name: label.Name, Color: strings.TrimPrefix(label.Color, "#")}
}

func (s *Store) linearSource(workspaceID string, source Source) (LinearClient, LinearSource, error) {
	if s.LinearClient == nil {
		return LinearClient{}, LinearSource{}, errors.New("Linear is unavailable.")
	}
	client, err := s.LinearClient(workspaceID)
	if err != nil {
		return LinearClient{}, LinearSource{}, err
	}
	config, err := s.LinearSource(workspaceID, source.ID)
	return client, config, err
}

func (linearProvider) items(s *Store, workspaceID string, source Source, state string) ([]WorkItem, error) {
	client, config, err := s.linearSource(workspaceID, source)
	if err != nil {
		return nil, err
	}
	issues, err := client.Items(context.Background(), config, state)
	if err != nil {
		return nil, err
	}
	items := make([]WorkItem, 0, len(issues))
	for _, issue := range issues {
		items = append(items, linearWorkItem(issue, source))
	}
	return items, nil
}

func (linearProvider) detail(s *Store, workspaceID string, source Source, id string) (WorkItemDetail, error) {
	client, config, err := s.linearSource(workspaceID, source)
	if err != nil {
		return WorkItemDetail{}, err
	}
	issue, err := client.Detail(context.Background(), config, id)
	if err != nil {
		return WorkItemDetail{}, err
	}
	return WorkItemDetail{
		WorkItem: linearWorkItem(issue, source),
		Body:     issue.Description,
		Comments: []Comment{},
		Prompt:   fmt.Sprintf("Work on this Linear issue: %s\n%s: %s\n\nIssue description (Markdown):\n%s", issue.URL, issue.Identifier, issue.Title, issue.Description),
	}, nil
}

// Linear picks statuses, assignees and labels by id: options lists the team's, and update
// takes their ids.
func (linearProvider) options(s *Store, workspaceID string, source Source, field string) (any, error) {
	client, config, err := s.linearSource(workspaceID, source)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	switch field {
	case "state":
		return client.States(ctx, config.TeamID)
	case "assignees":
		members, err := client.Members(ctx, config.TeamID)
		people := []Person{}
		for _, member := range members {
			people = append(people, Person{ID: member.ID, Login: member.Name, AvatarURL: member.AvatarURL})
		}
		return people, err
	case "labels":
		labels, err := client.Labels(ctx, config.TeamID)
		result := []Label{}
		for _, label := range labels {
			result = append(result, linearLabel(label))
		}
		return result, err
	}
	return nil, errors.New("Invalid metadata field.")
}

func (linearProvider) update(s *Store, workspaceID string, source Source, id, field string, values []string) error {
	client, config, err := s.linearSource(workspaceID, source)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, value := range values {
		if uuid.Validate(value) != nil || seen[value] {
			return errors.New("Invalid metadata selection.")
		}
		seen[value] = true
	}
	input := map[string]any{}
	switch field {
	case "state":
		if len(values) != 1 {
			return errors.New("Choose one status.")
		}
		input["stateId"] = values[0]
	case "assignees":
		if len(values) > 1 {
			return errors.New("A Linear issue has one assignee.")
		}
		input["assigneeId"] = nil
		if len(values) == 1 {
			input["assigneeId"] = values[0]
		}
	case "labels":
		if len(values) > 100 {
			return errors.New("Too many selections.")
		}
		input["labelIds"] = append([]string{}, values...)
	default:
		return errors.New("Invalid metadata field.")
	}
	ctx := context.Background()
	// Detail confirms the issue belongs to this source.
	if _, err = client.Detail(ctx, config, id); err != nil {
		return err
	}
	return client.UpdateIssue(ctx, id, input)
}
