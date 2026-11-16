package core

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gitHubFixture gives the workspace's repository a GitHub remote and a gh that answers with body.
func gitHubFixture(t *testing.T, s *Store, p *Workspace) func(body string) {
	t.Helper()
	s.mu.Lock()
	stored, _ := s.workspaceLocked(p.ID)
	stored.Repositories[0].GitHub = "fixture/repository"
	s.mu.Unlock()
	dir := t.TempDir()
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte("#!/bin/sh\ncase \"$*\" in *comments*) cat \""+dir+"/comments\";; *) cat \""+dir+"/response\";; esac\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "comments"), []byte(`[[{"id":7,"user":{"login":"reviewer"},"created_at":"2026-01-02T00:00:00Z","body_html":"<p>Looks good</p>"}]]`), 0600); err != nil {
		t.Fatal(err)
	}
	return func(body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "response"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWorkspaceSourcesAndVisibility(t *testing.T) {
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	gitHubFixture(t, s, p)
	repo := p.Repositories[0].ID
	state, err := s.SaveLinearSource(p.ID, LinearSource{Name: "Team", TeamID: "team"})
	if err != nil {
		t.Fatal(err)
	}
	linear := state.Workspaces[0].LinearSources[0].ID
	ids := []string{}
	for _, source := range state.Workspaces[0].Sources {
		ids = append(ids, source.Kind+" "+source.ID)
	}
	if want := "workItems github:issues:" + repo + ",reviews github:pulls:" + repo + ",workItems " + linear; strings.Join(ids, ",") != want {
		t.Fatalf("sources: %v", ids)
	}
	if _, err = s.ShowWorkspaceSource(p.ID, "github:pulls:"+repo, false); err != nil {
		t.Fatal(err)
	}
	if state, err = s.ShowWorkspaceSource(p.ID, linear, false); err != nil {
		t.Fatal(err)
	}
	hidden := map[string]bool{}
	for _, source := range state.Workspaces[0].Sources {
		hidden[source.ID] = source.Hidden
	}
	if hidden["github:issues:"+repo] || !hidden["github:pulls:"+repo] || !hidden[linear] {
		t.Fatalf("visibility: %v", hidden)
	}
	if state, err = s.ShowWorkspaceSource(p.ID, linear, true); err != nil || len(state.Workspaces[0].HiddenSources) != 1 {
		t.Fatalf("shown again: %v, %v", state.Workspaces[0].HiddenSources, err)
	}
	if _, err = s.ShowWorkspaceSource(p.ID, "github:issues:outside", false); err == nil {
		t.Fatal("unknown source accepted")
	}
	if _, err = s.WorkItems(p.ID, "github:issues:outside", "open"); err == nil {
		t.Fatal("listed an unknown source")
	}
	// Sources are derived, never saved.
	if data, _ := os.ReadFile(s.path); strings.Contains(string(data), `"sources"`) || !strings.Contains(string(data), "github:pulls:"+repo) {
		t.Fatalf("saved state: %s", data)
	}
}

func TestGitHubItemsAreNormalized(t *testing.T) {
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	respond := gitHubFixture(t, s, p)
	repo := p.Repositories[0].ID
	respond(`[[{"number":1,"title":"Bug","html_url":"https://github.com/fixture/repository/issues/1","state":"closed","state_reason":"not_planned","user":{"login":"a","avatar_url":"https://avatars/a"},"assignees":[{"login":"b"}],"labels":[{"name":"bug","color":"ff0000"}],"updated_at":"2026-01-01T00:00:00Z"},{"number":2,"title":"Change","html_url":"https://github.com/fixture/repository/pull/2","state":"closed","pull_request":{"merged_at":"2026-01-01T00:00:00Z"}}]]`)
	issues, err := s.WorkItems(p.ID, "github:issues:"+repo, "all")
	if err != nil || len(issues) != 1 {
		t.Fatalf("issues: %+v, %v", issues, err)
	}
	issue := issues[0]
	if issue.ID != repo+"#1" || issue.Identifier != "#1" || issue.Status != "Closed" || issue.StateReason != "not_planned" || issue.Author.AvatarURL != "https://avatars/a" || issue.Assignees[0].Login != "b" || issue.Labels[0].Color != "ff0000" || !issue.Can.EditLabels || issue.Can.Checks {
		t.Fatalf("issue: %+v", issue)
	}
	pulls, err := s.WorkItems(p.ID, "github:pulls:"+repo, "all")
	if err != nil || len(pulls) != 1 || pulls[0].Status != "Merged" || !pulls[0].Review || pulls[0].Can.EditState || !pulls[0].Can.Checks {
		t.Fatalf("pulls: %+v, %v", pulls, err)
	}
	respond(`{"number":2,"title":"Change","html_url":"https://github.com/fixture/repository/pull/2","state":"open","body":"Text","body_html":"<p>Text</p>","head":{"label":"me:feature","ref":"feature","repo":{"full_name":"me/repository"}},"base":{"label":"fixture:main","ref":"main","repo":{"full_name":"fixture/repository"}},"mergeable":false,"changed_files":3,"additions":5,"deletions":1}`)
	detail, err := s.WorkItemDetail(p.ID, "github:pulls:"+repo, repo+"#2")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Body != "Text" || detail.BodyHTML != "<p>Text</p>" || len(detail.Comments) != 1 || detail.Comments[0].Author.Login != "reviewer" || detail.Branches.ChangedFiles != 3 {
		t.Fatalf("detail: %+v", detail)
	}
	if c := detail.Conflict; c == nil || c.Base != "main" || c.Head != "feature" || !c.ForkWithoutEdits || c.Reference.ID != repo+"#2" || !c.Reference.Review {
		t.Fatalf("conflict: %+v", c)
	}
	if !strings.Contains(detail.Prompt, "gh api repos/fixture/repository/pulls/2/comments") {
		t.Fatalf("prompt: %s", detail.Prompt)
	}
	if _, err = s.WorkItemDetail(p.ID, "github:pulls:"+repo, "other#2"); err == nil {
		t.Fatal("accepted an item of another repository")
	}
}

func TestLinearItemsAreNormalized(t *testing.T) {
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"issues":{"nodes":[{"id":"uuid","identifier":"ENG-1","title":"Task","url":"https://linear.app/acme/issue/ENG-1","state":{"name":"In Review","type":"started"},"assignee":{"name":"Ana"},"labels":{"nodes":[{"name":"bug","color":"#00ff00"}]},"team":{"id":"team"}}],"pageInfo":{"hasNextPage":false}}}}`)
	}))
	defer server.Close()
	s.LinearClient = func(string) (LinearClient, error) { return LinearClient{Key: "key", Endpoint: server.URL}, nil }
	state, err := s.SaveLinearSource(p.ID, LinearSource{Name: "Team", TeamID: "team", RepositoryID: p.Repositories[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	source := state.Workspaces[0].LinearSources[0].ID
	items, err := s.WorkItems(p.ID, source, "open")
	if err != nil || len(items) != 1 {
		t.Fatalf("items: %+v, %v", items, err)
	}
	item := items[0]
	if item.ID != "uuid" || item.Status != "In Review" || item.State != "open" || item.RepositoryID != p.Repositories[0].ID || item.Labels[0].Color != "00ff00" || item.Assignees[0].Login != "Ana" || !item.Can.ChooseRepository || !item.Can.EditLabels || item.Can.EditDescription {
		t.Fatalf("item: %+v", item)
	}
}

func TestLegacyGitHubLinksMigrate(t *testing.T) {
	dir := t.TempDir()
	folder := filepath.Join(dir, "app")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	pid, rid := "8a1e3c52-5d0a-4c55-9f8e-3a1f0a5b6c01", "8a1e3c52-5d0a-4c55-9f8e-3a1f0a5b6c03"
	session := func(id, link string) string {
		return `{"id":"8a1e3c52-5d0a-4c55-9f8e-3a1f0a5b6c` + id + `","name":"S` + id + `","agent":"claude","repositoryID":"` + rid + `"` + link + `}`
	}
	data := `{"version":2,"workspaces":[{"id":"` + pid + `","name":"x","repositories":[{"id":"` + rid + `","name":"app","directory":"` + folder + `"}],"sessions":[` +
		session("11", `,"gitHubItem":{"number":7,"pullRequest":true}`) + `,` +
		session("12", `,"gitHubItem":{"number":8},"workItem":{"provider":"github","id":"123456","url":"https://github.com/o/r/issues/8","identifier":"#8"}`) + `,` +
		session("13", `,"workItem":{"provider":"linear","id":"uuid","url":"https://linear.app/a/issue/ENG-1"}`) + `]}]}`
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	s, err := NewStore(path, exe, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()
	sessions := s.Snapshot().Workspaces[0].Sessions
	if ref := sessions[0].WorkItem; ref == nil || ref.ID != rid+"#7" || ref.Number != 7 || !ref.Review || sessions[0].LegacyGitHubItem != nil {
		t.Fatalf("pull request link: %+v", ref)
	}
	if ref := sessions[1].WorkItem; ref.ID != rid+"#8" || ref.Number != 8 || ref.URL != "https://github.com/o/r/issues/8" {
		t.Fatalf("issue link: %+v", ref)
	}
	if ref := sessions[2].WorkItem; ref.ID != "uuid" || ref.Provider != "linear" {
		t.Fatalf("Linear link: %+v", ref)
	}
	if saved, _ := os.ReadFile(path); strings.Contains(string(saved), "gitHubItem") {
		t.Fatalf("legacy link still saved: %s", saved)
	}
}

func TestGitHubConflictNeedsConfirmation(t *testing.T) {
	no, yes := false, true
	branch := func(ref, repo string) *ghBranch {
		b := &ghBranch{Ref: ref}
		if repo != "" {
			b.Repo = &struct {
				FullName string `json:"full_name"`
			}{repo}
		}
		return b
	}
	pr := ghItem{Number: 50, Title: "PR tab", State: "open", HTMLURL: "https://github.com/o/r/pull/50", Mergeable: &no, MergeableState: "dirty", Head: branch("feature", "o/r"), Base: branch("main", "o/r")}
	if c := gitHubConflict(pr, "repo"); c == nil || *c != (ReviewConflict{Number: 50, Title: "PR tab", URL: "https://github.com/o/r/pull/50", Base: "main", Head: "feature", Reference: gitHubReference("repo", 50, "https://github.com/o/r/pull/50", true)}) {
		t.Fatalf("conflict: %+v", c)
	}
	for name, other := range map[string]func(*ghItem){
		"clean":     func(p *ghItem) { p.Mergeable, p.MergeableState = &yes, "clean" },
		"computing": func(p *ghItem) { p.Mergeable, p.MergeableState = nil, "unknown" },
		"closed":    func(p *ghItem) { p.State = "closed" },
		"no head":   func(p *ghItem) { p.Head = nil },
	} {
		changed := pr
		other(&changed)
		if c := gitHubConflict(changed, "repo"); c != nil {
			t.Errorf("%s: unexpected conflict %+v", name, c)
		}
	}
	fork := pr
	fork.Head = branch("feature", "someone/r")
	if !gitHubConflict(fork, "repo").ForkWithoutEdits {
		t.Fatal("fork without edits allowed")
	}
	fork.MaintainerCanModify = true
	if gitHubConflict(fork, "repo").ForkWithoutEdits {
		t.Fatal("fork allows edits by maintainers")
	}
	fork.Head = branch("feature", "")
	fork.MaintainerCanModify = false
	if !gitHubConflict(fork, "repo").ForkWithoutEdits {
		t.Fatal("deleted fork allowed")
	}
}
