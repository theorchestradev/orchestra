//go:build !windows

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLinearPaginationAndFilters(t *testing.T) {
	pages := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "test-secret" {
			t.Error("missing key")
		}
		var req struct {
			Variables map[string]json.RawMessage `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if string(req.Variables["filter"]) != `{"project":{"id":{"eq":"project"}},"state":{"type":{"nin":["completed","canceled"]}},"team":{"id":{"eq":"team"}}}` {
			t.Error(string(req.Variables["filter"]))
		}
		pages++
		if pages == 1 {
			fmt.Fprint(w, `{"data":{"issues":{"nodes":[{"id":"one"}],"pageInfo":{"hasNextPage":true,"endCursor":"next"}}}}`)
		} else {
			if string(req.Variables["after"]) != `"next"` {
				t.Error("cursor lost")
			}
			fmt.Fprint(w, `{"data":{"issues":{"nodes":[{"id":"two"}],"pageInfo":{"hasNextPage":false}}}}`)
		}
	}))
	defer server.Close()
	client := LinearClient{Key: "test-secret", HTTP: server.Client(), Endpoint: server.URL}
	items, err := client.Items(context.Background(), LinearSource{TeamID: "team", ProjectID: "project"}, "open")
	if err != nil || len(items) != 2 || items[1].ID != "two" || pages != 2 {
		t.Fatalf("%+v %v", items, err)
	}
	if _, err := client.Items(context.Background(), LinearSource{}, "invalid"); err == nil {
		t.Fatal("invalid state accepted")
	}
}
func TestLinearFailuresNeverEchoSecrets(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		body, want string
	}{
		{"auth", 401, `test-secret`, "authentication"},
		{"forbidden", 403, `test-secret`, "authentication"},
		{"rate", 429, `test-secret`, "rate limit"},
		{"server", 500, `test-secret`, "failed"},
		{"graphql", 200, `{"data":{"viewer":{"id":"partial"}},"errors":[{"message":"test-secret"}]}`, "could not complete"},
		{"graphql rate", 200, `{"errors":[{"message":"test-secret","extensions":{"code":"RATELIMITED"}}]}`, "rate limit"},
		{"graphql auth", 200, `{"errors":[{"extensions":{"code":"UNAUTHENTICATED"}}]}`, "authentication"},
		{"malformed", 200, `test-secret`, "Invalid"},
		{"null", 200, `{"data":null}`, "Invalid"},
		{"missing viewer", 200, `{"data":{"viewer":null}}`, "Invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			_, err := (LinearClient{Key: "test-secret", Endpoint: server.URL, HTTP: server.Client()}).Viewer(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "test-secret") {
				t.Fatal(err)
			}
		})
	}
}
func TestLinearRedirectAndSourceBoundaries(t *testing.T) {
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected = true }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	client := LinearClient{Key: "test-secret", Endpoint: server.URL, HTTP: server.Client()}
	if _, err := client.Viewer(context.Background()); err == nil || redirected {
		t.Fatal("followed redirect")
	}
	detail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"issue":{"id":"one","team":{"id":"other"}}}}`)
	}))
	defer detail.Close()
	client.Endpoint = detail.URL
	if _, err := client.Detail(context.Background(), LinearSource{TeamID: "team"}, "one"); err == nil {
		t.Fatal("cross-source detail accepted")
	}
	if _, err := nextLinearPage(linearPageInfo{HasNextPage: true, EndCursor: "same"}, "same"); err == nil {
		t.Fatal("stuck cursor")
	}
}
func TestLinearSourcesAndSessionState(t *testing.T) {
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	state, err := s.SaveLinearSource(p.ID, LinearSource{Name: "Team", TeamID: "team", RepositoryID: p.Repositories[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	source := state.Workspaces[0].LinearSources[0]
	if source.ID == "" {
		t.Fatal("source identity missing")
	}
	if _, err = s.SaveLinearSource(p.ID, LinearSource{Name: "Team", TeamID: "team", RepositoryID: "outside"}); err == nil {
		t.Fatal("foreign repo accepted")
	}
	source.Name = "Updated"
	if _, err = s.SaveLinearSource(p.ID, source); err != nil {
		t.Fatal(err)
	}
	ref := &WorkItemReference{Provider: "linear", ID: "stable-uuid", Identifier: "ENG-1", URL: "https://linear.app/acme/issue/ENG-1"}
	if _, err = s.CreateSession(p.ID, "", "codex", "Linear", "", true, ref, "", "", "", "", ""); err == nil {
		t.Fatal("implicit execution repository accepted")
	}
	state, err = s.CreateSession(p.ID, p.Repositories[0].ID, "codex", "Linear", "", false, ref, "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	sess := state.Workspaces[0].Sessions[len(state.Workspaces[0].Sessions)-1]
	if sess.WorkItem == nil || *sess.WorkItem != *ref {
		t.Fatalf("bad link: %+v", sess)
	}
	encoded, _ := json.Marshal(state)
	var restored State
	if err = json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Workspaces[0].LinearSources[0].Name != "Updated" {
		t.Fatal("lost source")
	}
	if _, err = s.RemoveLinearSource(p.ID, source.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.LinearSource(p.ID, source.ID); err == nil {
		t.Fatal("source not removed")
	}
	for _, bad := range []WorkItemReference{{Provider: "linear", ID: "x", URL: "javascript:alert(1)"}, {Provider: "other", ID: "x", URL: "https://linear.app"}, {Provider: "linear", URL: "https://linear.app"}} {
		if validateWorkItemReference(bad) == nil {
			t.Fatal("invalid reference accepted")
		}
	}
}
func TestLinearMetadataOptionsAndUpdates(t *testing.T) {
	const issue = "00000000-0000-4000-8000-000000000001"
	const state = "00000000-0000-4000-8000-000000000002"
	var inputs []string
	forbidden := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string                     `json:"query"`
			Variables map[string]json.RawMessage `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		switch {
		case strings.Contains(req.Query, "states("):
			fmt.Fprint(w, `{"data":{"team":{"states":{"nodes":[{"id":"done","name":"Done","type":"completed","position":0},{"id":"later","name":"Later","type":"started","position":2.5},{"id":"now","name":"Now","type":"started","position":2}],"pageInfo":{"hasNextPage":false}}}}}`)
		case strings.Contains(req.Query, "members("):
			fmt.Fprint(w, `{"data":{"team":{"members":{"nodes":[{"id":"a","name":"Ana","active":true},{"id":"b","name":"Bia","active":false}],"pageInfo":{"hasNextPage":false}}}}}`)
		case strings.Contains(req.Query, "issueLabels("):
			if !strings.Contains(string(req.Variables["filter"]), `"isGroup":{"eq":false}`) {
				t.Error(string(req.Variables["filter"]))
			}
			fmt.Fprint(w, `{"data":{"issueLabels":{"nodes":[{"id":"l","name":"Bug","color":"#ff0000"}],"pageInfo":{"hasNextPage":false}}}}`)
		case strings.Contains(req.Query, "issue(id"):
			fmt.Fprint(w, `{"data":{"issue":{"id":"`+issue+`","team":{"id":"team"}}}}`)
		case strings.Contains(req.Query, "issueUpdate"):
			if forbidden {
				fmt.Fprint(w, `{"errors":[{"message":"test-secret","extensions":{"code":"FORBIDDEN"}}]}`)
				return
			}
			inputs = append(inputs, string(req.Variables["input"]))
			fmt.Fprint(w, `{"data":{"issueUpdate":{"success":true}}}`)
		default:
			t.Error(req.Query)
		}
	}))
	defer server.Close()
	s := fixtureStore(t)
	p := workspaceFixture(t, s)
	state0, err := s.SaveLinearSource(p.ID, LinearSource{Name: "Team", TeamID: "team"})
	if err != nil {
		t.Fatal(err)
	}
	s.LinearClient = func(string) (LinearClient, error) {
		return LinearClient{Key: "test-secret", Endpoint: server.URL, HTTP: server.Client()}, nil
	}
	sourceID := state0.Workspaces[0].LinearSources[0].ID
	states, err := s.WorkItemOptions(p.ID, sourceID, "state")
	if got := fmt.Sprint(states); err != nil || got != "[{now Now started 2} {later Later started 2.5} {done Done completed 0}]" {
		t.Fatal(got, err)
	}
	people, err := s.WorkItemOptions(p.ID, sourceID, "assignees")
	if got := fmt.Sprint(people); err != nil || got != "[{a Ana }]" {
		t.Fatal(got, err)
	}
	labels, err := s.WorkItemOptions(p.ID, sourceID, "labels")
	if got := fmt.Sprint(labels); err != nil || got != "[{l Bug ff0000}]" {
		t.Fatal(got, err)
	}
	for _, update := range []struct {
		field  string
		values []string
	}{{"state", []string{state}}, {"assignees", nil}, {"assignees", []string{state}}, {"labels", nil}} {
		if err = s.UpdateWorkItem(p.ID, sourceID, issue, update.field, update.values); err != nil {
			t.Fatal(update.field, err)
		}
	}
	if got := strings.Join(inputs, " "); got != `{"stateId":"`+state+`"} {"assigneeId":null} {"assigneeId":"`+state+`"} {"labelIds":[]}` {
		t.Fatal(got)
	}
	for _, bad := range []struct {
		field  string
		values []string
	}{{"state", nil}, {"assignees", []string{state, issue}}, {"labels", []string{"not-an-id"}}, {"labels", []string{state, state}}, {"body", []string{"text"}}} {
		if err = s.UpdateWorkItem(p.ID, sourceID, issue, bad.field, bad.values); err == nil {
			t.Fatal("accepted", bad)
		}
	}
	forbidden = true
	if err = s.UpdateWorkItem(p.ID, sourceID, issue, "state", []string{state}); err == nil || !strings.Contains(err.Error(), "write access") || strings.Contains(err.Error(), "test-secret") {
		t.Fatal(err)
	}
}
