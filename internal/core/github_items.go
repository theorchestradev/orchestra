package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// gitHubProvider reads a repository's issues for Work Items and its pull requests for Reviews.
type gitHubProvider struct{}

type ghUser struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
}
type ghBranch struct {
	Label string `json:"label"`
	Ref   string `json:"ref"`
	Repo  *struct {
		FullName string `json:"full_name"`
	} `json:"repo"`
}

// ghItem holds the fields Orchestra reads from an issue, or from a pull request's detail.
type ghItem struct {
	Number      int     `json:"number"`
	Title       string  `json:"title"`
	HTMLURL     string  `json:"html_url"`
	State       string  `json:"state"`
	StateReason *string `json:"state_reason"`
	Draft       bool    `json:"draft"`
	Merged      bool    `json:"merged"`
	Body        *string `json:"body"`
	BodyHTML    string  `json:"body_html"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
	PullRequest *struct {
		MergedAt *string `json:"merged_at"`
	} `json:"pull_request"`
	User      *ghUser  `json:"user"`
	Assignees []ghUser `json:"assignees"`
	Labels    []Label  `json:"labels"`
	// Pull request details only.
	Head                *ghBranch `json:"head"`
	Base                *ghBranch `json:"base"`
	Mergeable           *bool     `json:"mergeable"`
	MergeableState      string    `json:"mergeable_state"`
	MaintainerCanModify bool      `json:"maintainer_can_modify"`
	ChangedFiles        int       `json:"changed_files"`
	Additions           int       `json:"additions"`
	Deletions           int       `json:"deletions"`
}

// An issue's number is only unique within its repository.
func gitHubItemID(repositoryID string, number int) string {
	return repositoryID + "#" + strconv.Itoa(number)
}

// gitHubReference links a session to an issue or pull request of a workspace repository.
func gitHubReference(repositoryID string, number int, url string, review bool) WorkItemReference {
	return WorkItemReference{Provider: "github", ID: gitHubItemID(repositoryID, number), URL: url, Identifier: "#" + strconv.Itoa(number), Number: number, Review: review}
}

func gitHubNumber(source Source, id string) (int, error) {
	value, ok := strings.CutPrefix(id, source.RepositoryID+"#")
	number, err := strconv.Atoi(value)
	if !ok || err != nil || number < 1 {
		return 0, errors.New("Invalid item number.")
	}
	return number, nil
}

func gitHubPerson(user *ghUser) *Person {
	if user == nil {
		return nil
	}
	return &Person{Login: user.Login, AvatarURL: user.AvatarURL}
}

func gitHubWorkItem(item ghItem, source Source) WorkItem {
	review := source.Kind == "reviews"
	merged := item.Merged || (item.PullRequest != nil && item.PullRequest.MergedAt != nil)
	status := "Open"
	switch {
	case merged:
		status = "Merged"
	case item.State == "closed":
		status = "Closed"
	case item.Draft:
		status = "Draft"
	}
	result := WorkItem{
		Provider: "github", SourceID: source.ID, RepositoryID: source.RepositoryID,
		ID: gitHubItemID(source.RepositoryID, item.Number), Number: item.Number, Identifier: "#" + strconv.Itoa(item.Number),
		Title: item.Title, URL: item.HTMLURL, State: item.State, Status: status, Review: review,
		Author: gitHubPerson(item.User), Assignees: []Person{}, Labels: []Label{},
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		Can: ItemCapabilities{EditState: !merged, EditAssignees: true, EditLabels: true, EditDescription: true, Comments: true, Checks: review},
	}
	if item.StateReason != nil {
		result.StateReason = *item.StateReason
	}
	for _, user := range item.Assignees {
		result.Assignees = append(result.Assignees, *gitHubPerson(&user))
	}
	if item.Labels != nil {
		result.Labels = item.Labels
	}
	return result
}

// GitHub lists pull requests among issues; each source keeps its own kind.
func (gitHubProvider) items(s *Store, workspaceID string, source Source, state string) ([]WorkItem, error) {
	raw, err := s.GitHubItems(workspaceID, source.RepositoryID, state)
	if err != nil {
		return nil, err
	}
	items := []WorkItem{}
	for _, data := range raw {
		var item ghItem
		if err = json.Unmarshal(data, &item); err != nil {
			return nil, errors.New("Invalid GitHub response.")
		}
		if (item.PullRequest != nil) == (source.Kind == "reviews") {
			items = append(items, gitHubWorkItem(item, source))
		}
	}
	return items, nil
}

func (gitHubProvider) detail(s *Store, workspaceID string, source Source, id string) (WorkItemDetail, error) {
	number, err := gitHubNumber(source, id)
	if err != nil {
		return WorkItemDetail{}, err
	}
	content, err := s.GitHubDetail(workspaceID, source.RepositoryID, number, source.Kind == "reviews")
	if err != nil {
		return WorkItemDetail{}, err
	}
	var item ghItem
	if err = json.Unmarshal(content.Detail, &item); err != nil {
		return WorkItemDetail{}, errors.New("Invalid GitHub response.")
	}
	detail := WorkItemDetail{WorkItem: gitHubWorkItem(item, source), BodyHTML: item.BodyHTML, Comments: []Comment{}}
	if item.Body != nil {
		detail.Body = *item.Body
	}
	for _, data := range content.Comments {
		var comment struct {
			ID        int64   `json:"id"`
			User      *ghUser `json:"user"`
			CreatedAt string  `json:"created_at"`
			BodyHTML  string  `json:"body_html"`
		}
		if err = json.Unmarshal(data, &comment); err != nil {
			return WorkItemDetail{}, errors.New("Invalid GitHub response.")
		}
		detail.Comments = append(detail.Comments, Comment{ID: strconv.FormatInt(comment.ID, 10), Author: gitHubPerson(comment.User), CreatedAt: comment.CreatedAt, BodyHTML: comment.BodyHTML})
	}
	if detail.Review {
		if item.Head != nil && item.Base != nil {
			detail.Branches = &ReviewBranches{Head: item.Head.Label, Base: item.Base.Label, ChangedFiles: item.ChangedFiles, Additions: item.Additions, Deletions: item.Deletions}
		}
		detail.Conflict = gitHubConflict(item, source.RepositoryID)
		// Agents often read only the description, so ask for the discussion explicitly.
		detail.Prompt = fmt.Sprintf("Review this GitHub pull request: %s. First read its description, comments and reviews (gh pr view %s --json title,body,comments,reviews) and its inline review comments (gh api repos/%s/pulls/%d/comments).", item.HTMLURL, item.HTMLURL, source.Name, number)
	} else {
		detail.Prompt = fmt.Sprintf("Work on this GitHub issue: %s. First read the issue and all of its comments (gh issue view %s --json title,body,comments).", item.HTMLURL, item.HTMLURL)
	}
	return detail, nil
}

// gitHubConflict is an open pull request's merge conflict. GitHub reports mergeable as null
// while it computes it, so only a confirmed conflict counts and the next refresh picks it up.
func gitHubConflict(pr ghItem, repositoryID string) *ReviewConflict {
	if pr.State != "open" || pr.Head == nil || pr.Base == nil || pr.Mergeable == nil {
		return nil
	}
	if *pr.Mergeable && pr.MergeableState != "dirty" {
		return nil
	}
	repo := func(b *ghBranch) string {
		if b.Repo == nil {
			return ""
		}
		return b.Repo.FullName
	}
	return &ReviewConflict{
		Number: pr.Number, Title: pr.Title, URL: pr.HTMLURL, Base: pr.Base.Ref, Head: pr.Head.Ref,
		ForkWithoutEdits: repo(pr.Head) != repo(pr.Base) && !pr.MaintainerCanModify,
		Reference:        gitHubReference(repositoryID, pr.Number, pr.HTMLURL, true),
	}
}

func (gitHubProvider) options(s *Store, workspaceID string, source Source, field string) (any, error) {
	raw, err := s.GitHubMetadataOptions(workspaceID, source.RepositoryID, field)
	if err != nil {
		return nil, err
	}
	if field == "labels" {
		labels := []Label{}
		for _, data := range raw {
			var label Label
			if err = json.Unmarshal(data, &label); err != nil {
				return nil, errors.New("Invalid GitHub response.")
			}
			labels = append(labels, label)
		}
		return labels, nil
	}
	people := []Person{}
	for _, data := range raw {
		var user ghUser
		if err = json.Unmarshal(data, &user); err != nil {
			return nil, errors.New("Invalid GitHub response.")
		}
		people = append(people, *gitHubPerson(&user))
	}
	return people, nil
}

func (gitHubProvider) update(s *Store, workspaceID string, source Source, id, field string, values []string) error {
	number, err := gitHubNumber(source, id)
	if err != nil {
		return err
	}
	return s.UpdateGitHubMetadata(workspaceID, source.RepositoryID, number, source.Kind == "reviews", field, values)
}

func (gitHubProvider) reviewStatus(s *Store, workspaceID string, source Source, id string) (PullRequestStatus, error) {
	number, err := gitHubNumber(source, id)
	if err != nil {
		return PullRequestStatus{}, err
	}
	return s.GitHubPullRequest(workspaceID, source.RepositoryID, number)
}

func (gitHubProvider) checkFailure(s *Store, workspaceID string, source Source, jobID int64) (string, error) {
	return s.GitHubCheckFailure(workspaceID, source.RepositoryID, jobID)
}
