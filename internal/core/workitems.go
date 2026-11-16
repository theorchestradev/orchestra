package core

import (
	"errors"
	"net/url"
	"slices"
	"strconv"
)

// Work Items lists issues and Reviews lists pull requests, each from the workspace's sources:
// one per provider query, such as a repository's GitHub issues or a Linear team. Providers
// normalize their items into WorkItem, and Can tells the views what the provider supports.

// Source is one place a workspace's Work Items or Reviews read items.
type Source struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	// Kind is "workItems" or "reviews".
	Kind string `json:"kind"`
	Name string `json:"name"`
	URL  string `json:"url"`
	// RepositoryID is the repository the items belong to, or else the default one sessions
	// started from them work in.
	RepositoryID string `json:"repositoryID,omitempty"`
	Hidden       bool   `json:"hidden,omitempty"`
}

// workspaceSources derives the workspace's sources: each GitHub repository's issues and pull
// requests, then the Linear sources.
func workspaceSources(p *Workspace) []Source {
	sources := []Source{}
	for _, repo := range p.Repositories {
		if repo.GitHub == "" {
			continue
		}
		sources = append(sources,
			Source{ID: "github:issues:" + repo.ID, Provider: "github", Kind: "workItems", Name: repo.GitHub, URL: "https://github.com/" + repo.GitHub, RepositoryID: repo.ID},
			Source{ID: "github:pulls:" + repo.ID, Provider: "github", Kind: "reviews", Name: repo.GitHub, URL: "https://github.com/" + repo.GitHub, RepositoryID: repo.ID})
	}
	for _, source := range p.LinearSources {
		sources = append(sources, Source{ID: source.ID, Provider: "linear", Kind: "workItems", Name: source.Name, URL: "https://linear.app", RepositoryID: source.RepositoryID})
	}
	for i := range sources {
		sources[i].Hidden = slices.Contains(p.HiddenSources, sources[i].ID)
	}
	return sources
}

// ID identifies people and labels for providers that pick them by id, such as Linear.
type Person struct {
	ID        string `json:"id,omitempty"`
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

// Label colors are six hex digits without "#".
type Label struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// ItemCapabilities are what the views may offer for an item.
type ItemCapabilities struct {
	EditState       bool `json:"editState,omitempty"`
	EditAssignees   bool `json:"editAssignees,omitempty"`
	EditLabels      bool `json:"editLabels,omitempty"`
	EditDescription bool `json:"editDescription,omitempty"`
	Comments        bool `json:"comments,omitempty"`
	// Checks reads a review's CI checks with ReviewStatus.
	Checks bool `json:"checks,omitempty"`
	// ChooseRepository lets Work on pick the repository, for items outside any repository.
	ChooseRepository bool `json:"chooseRepository,omitempty"`
}

type WorkItem struct {
	Provider     string `json:"provider"`
	SourceID     string `json:"sourceID"`
	RepositoryID string `json:"repositoryID,omitempty"`
	// ID is unique within the provider and stays the same for the item's life.
	ID         string `json:"id"`
	Number     int    `json:"number,omitempty"`
	Identifier string `json:"identifier"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	// State is "open" or "closed", for the status filter and Work on. StateReason says why a
	// GitHub issue closed, and Status is the name the views show.
	State       string           `json:"state"`
	StateReason string           `json:"stateReason,omitempty"`
	Status      string           `json:"status"`
	Review      bool             `json:"review,omitempty"`
	Author      *Person          `json:"author,omitempty"`
	Assignees   []Person         `json:"assignees"`
	Labels      []Label          `json:"labels"`
	CreatedAt   string           `json:"createdAt"`
	UpdatedAt   string           `json:"updatedAt"`
	Can         ItemCapabilities `json:"can"`
}

// Body is a comment's Markdown source; BodyHTML is the provider's rendering of it, when it has one.
type Comment struct {
	ID        string  `json:"id"`
	Author    *Person `json:"author,omitempty"`
	CreatedAt string  `json:"createdAt"`
	Body      string  `json:"body,omitempty"`
	BodyHTML  string  `json:"bodyHTML,omitempty"`
}

type ReviewBranches struct {
	Head         string `json:"head"`
	Base         string `json:"base"`
	ChangedFiles int    `json:"changedFiles"`
	Additions    int    `json:"additions"`
	Deletions    int    `json:"deletions"`
}

// ReviewConflict is a review's confirmed merge conflict, which a session can resolve.
type ReviewConflict struct {
	Number           int    `json:"number"`
	Title            string `json:"title"`
	URL              string `json:"url"`
	Base             string `json:"base"`
	Head             string `json:"head"`
	ForkWithoutEdits bool   `json:"forkWithoutEdits,omitempty"`
	// Reference links the session that resolves the conflict to the review.
	Reference WorkItemReference `json:"reference"`
}

type WorkItemDetail struct {
	WorkItem
	// Body is the Markdown source; BodyHTML is the provider's rendering of it, when it has one.
	Body     string          `json:"body"`
	BodyHTML string          `json:"bodyHTML,omitempty"`
	Comments []Comment       `json:"comments"`
	Branches *ReviewBranches `json:"branches,omitempty"`
	Conflict *ReviewConflict `json:"conflict,omitempty"`
	// Prompt is the task Work on or Review gives a new session.
	Prompt string `json:"prompt"`
}

// A provider reads the items of its sources; itemEditor and reviewChecker are optional.
type itemProvider interface {
	items(s *Store, workspaceID string, source Source, state string) ([]WorkItem, error)
	detail(s *Store, workspaceID string, source Source, id string) (WorkItemDetail, error)
}
type itemEditor interface {
	options(s *Store, workspaceID string, source Source, field string) (any, error)
	update(s *Store, workspaceID string, source Source, id, field string, values []string) error
}
type reviewChecker interface {
	reviewStatus(s *Store, workspaceID string, source Source, id string) (PullRequestStatus, error)
	checkFailure(s *Store, workspaceID string, source Source, jobID int64) (string, error)
}

var itemProviders = map[string]itemProvider{"github": gitHubProvider{}, "linear": linearProvider{}}

// providerHosts are the hosts a provider's item links may use.
var providerHosts = map[string]string{"github": "github.com", "linear": "linear.app"}

func validateWorkItemReference(ref WorkItemReference) error {
	u, err := url.Parse(ref.URL)
	host, known := providerHosts[ref.Provider]
	if err != nil || !known || u.Scheme != "https" || u.User != nil || u.Host != host || ref.ID == "" || (ref.Provider == "github" && ref.Number <= 0) {
		return errors.New("Invalid work item reference.")
	}
	return nil
}

// migrateWorkItemLinks moves the GitHub links of earlier versions into WorkItem. It reports
// whether any changed.
func migrateWorkItemLinks(state *State) bool {
	changed := false
	for _, p := range state.Workspaces {
		if p == nil {
			continue
		}
		for _, sess := range p.Sessions {
			if sess == nil || sess.LegacyGitHubItem == nil {
				continue
			}
			legacy := sess.LegacyGitHubItem
			sess.LegacyGitHubItem, changed = nil, true
			if sess.WorkItem == nil {
				sess.WorkItem = &WorkItemReference{Provider: "github", Identifier: "#" + strconv.Itoa(legacy.Number)}
			}
			if sess.WorkItem.Provider == "github" {
				sess.WorkItem.ID = gitHubItemID(sess.RepositoryID, legacy.Number)
				sess.WorkItem.Number, sess.WorkItem.Review = legacy.Number, legacy.PullRequest
			}
		}
	}
	return changed
}

func (s *Store) workspaceSource(workspaceID, sourceID string) (Source, itemProvider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.workspaceLocked(workspaceID)
	if err != nil {
		return Source{}, nil, err
	}
	for _, source := range workspaceSources(p) {
		if source.ID == sourceID {
			return source, itemProviders[source.Provider], nil
		}
	}
	return Source{}, nil, errors.New("This source is no longer in the workspace.")
}

// WorkItems lists a source's items in state "open", "closed" or "all".
func (s *Store) WorkItems(workspaceID, sourceID, state string) ([]WorkItem, error) {
	source, provider, err := s.workspaceSource(workspaceID, sourceID)
	if err != nil {
		return nil, err
	}
	if state != "open" && state != "closed" && state != "all" {
		return nil, errors.New("Invalid item state.")
	}
	return provider.items(s, workspaceID, source, state)
}

func (s *Store) WorkItemDetail(workspaceID, sourceID, id string) (WorkItemDetail, error) {
	source, provider, err := s.workspaceSource(workspaceID, sourceID)
	if err != nil {
		return WorkItemDetail{}, err
	}
	return provider.detail(s, workspaceID, source, id)
}

// WorkItemOptions lists the people or labels an item of the source can have.
func (s *Store) WorkItemOptions(workspaceID, sourceID, field string) (any, error) {
	source, provider, err := s.workspaceSource(workspaceID, sourceID)
	if err != nil {
		return nil, err
	}
	editor, ok := provider.(itemEditor)
	if !ok {
		return nil, errors.New(source.Name + " items cannot be edited here.")
	}
	return editor.options(s, workspaceID, source, field)
}

// UpdateWorkItem changes one field of an item: "body", "state", "assignees" or "labels".
func (s *Store) UpdateWorkItem(workspaceID, sourceID, id, field string, values []string) error {
	source, provider, err := s.workspaceSource(workspaceID, sourceID)
	if err != nil {
		return err
	}
	editor, ok := provider.(itemEditor)
	if !ok {
		return errors.New(source.Name + " items cannot be edited here.")
	}
	return editor.update(s, workspaceID, source, id, field, values)
}

// ReviewStatus reads a review's checks.
func (s *Store) ReviewStatus(workspaceID, sourceID, id string) (PullRequestStatus, error) {
	source, provider, err := s.workspaceSource(workspaceID, sourceID)
	if err != nil {
		return PullRequestStatus{}, err
	}
	checker, ok := provider.(reviewChecker)
	if !ok || source.Kind != "reviews" {
		return PullRequestStatus{}, errors.New(source.Name + " has no checks.")
	}
	return checker.reviewStatus(s, workspaceID, source, id)
}

// ReviewCheckFailure describes a failed check of a review as text to copy.
func (s *Store) ReviewCheckFailure(workspaceID, sourceID string, jobID int64) (string, error) {
	source, provider, err := s.workspaceSource(workspaceID, sourceID)
	if err != nil {
		return "", err
	}
	checker, ok := provider.(reviewChecker)
	if !ok || source.Kind != "reviews" {
		return "", errors.New(source.Name + " has no checks.")
	}
	return checker.checkFailure(s, workspaceID, source, jobID)
}

// ShowWorkspaceSource shows or hides a source in Work Items or Reviews. Hidden sources keep
// their configuration.
func (s *Store) ShowWorkspaceSource(workspaceID, sourceID string, shown bool) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.workspaceLocked(workspaceID)
	if err != nil {
		return State{}, err
	}
	if !slices.ContainsFunc(workspaceSources(p), func(source Source) bool { return source.ID == sourceID }) {
		return State{}, errors.New("This source is no longer in the workspace.")
	}
	previous := p.HiddenSources
	hidden := slices.DeleteFunc(slices.Clone(previous), func(id string) bool { return id == sourceID })
	if !shown {
		hidden = append(hidden, sourceID)
	}
	p.HiddenSources = hidden
	if err = s.saveLocked(); err != nil {
		p.HiddenSources = previous
		return State{}, err
	}
	s.changedLocked()
	return s.snapshotLocked(), nil
}
