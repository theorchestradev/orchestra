package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"

	"github.com/theorchestradev/orchestra/internal/core"
)

func linearKeyService(workspace string) string {
	service := "com.orchestra.linear"
	if path := os.Getenv("ORCHESTRA_STATE_FILE"); path != "" {
		service += fmt.Sprintf(".workspace-%x", sha256.Sum256([]byte(path)))
	}
	return service + fmt.Sprintf(".%x", sha256.Sum256([]byte(workspace)))
}
func (a *App) linearClient(workspace string) (core.LinearClient, error) {
	if err := a.ready(); err != nil {
		return core.LinearClient{}, err
	}
	if err := a.linearWorkspace(workspace); err != nil {
		return core.LinearClient{}, err
	}
	key, err := core.LinearKey(linearKeyService(workspace))
	return core.LinearClient{Key: key}, err
}
func (a *App) LinearAccount(workspace string) (core.LinearChoice, error) {
	client, err := a.linearClient(workspace)
	if err != nil {
		return core.LinearChoice{}, err
	}
	if client.Key == "" {
		return core.LinearChoice{}, nil
	}
	return client.Viewer(a.ctx)
}
func (a *App) ConnectLinear(workspace, key string) (core.LinearChoice, error) {
	if err := a.ready(); err != nil {
		return core.LinearChoice{}, err
	}
	if err := a.linearWorkspace(workspace); err != nil {
		return core.LinearChoice{}, err
	}
	key = strings.TrimSpace(key)
	account, err := (core.LinearClient{Key: key}).Viewer(a.ctx)
	if err != nil {
		return core.LinearChoice{}, err
	}
	if err = core.SaveLinearKey(linearKeyService(workspace), key); err != nil {
		return core.LinearChoice{}, err
	}
	return account, nil
}
func (a *App) DisconnectLinear(workspace string) error {
	if err := a.ready(); err != nil {
		return err
	}
	if err := a.linearWorkspace(workspace); err != nil {
		return err
	}
	return core.RemoveLinearKey(linearKeyService(workspace))
}
func (a *App) LinearChoices(workspace, team string) ([]core.LinearChoice, error) {
	client, err := a.linearClient(workspace)
	if err != nil {
		return nil, err
	}
	return client.Choices(a.ctx, team)
}
func (a *App) SaveLinearSource(workspace string, source core.LinearSource) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	// Resolve names and validate access, rather than trusting frontend IDs or labels.
	client, err := a.linearClient(workspace)
	if err != nil {
		return core.State{}, err
	}
	teams, err := client.Choices(a.ctx, "")
	if err != nil {
		return core.State{}, err
	}
	source.Name = ""
	for _, team := range teams {
		if team.ID == source.TeamID {
			source.Name = team.Name
		}
	}
	if source.Name == "" {
		return core.State{}, fmt.Errorf("Choose an accessible Linear team.")
	}
	if source.ProjectID != "" {
		projects, err := client.Choices(a.ctx, source.TeamID)
		if err != nil {
			return core.State{}, err
		}
		found := false
		for _, project := range projects {
			if project.ID == source.ProjectID {
				source.Name += " / " + project.Name
				found = true
			}
		}
		if !found {
			return core.State{}, fmt.Errorf("Choose a project in this Linear team.")
		}
	}
	return a.store.SaveLinearSource(workspace, source)
}
func (a *App) RemoveLinearSource(workspace, source string) (core.State, error) {
	if err := a.ready(); err != nil {
		return core.State{}, err
	}
	return a.store.RemoveLinearSource(workspace, source)
}
func (a *App) LinearItems(workspace, source, state string) ([]core.LinearIssue, error) {
	client, err := a.linearClient(workspace)
	if err != nil {
		return nil, err
	}
	config, err := a.store.LinearSource(workspace, source)
	if err != nil {
		return nil, err
	}
	return client.Items(a.ctx, config, state)
}
func (a *App) LinearDetail(workspace, source, id string) (core.LinearIssue, error) {
	client, err := a.linearClient(workspace)
	if err != nil {
		return core.LinearIssue{}, err
	}
	config, err := a.store.LinearSource(workspace, source)
	if err != nil {
		return core.LinearIssue{}, err
	}
	return client.Detail(a.ctx, config, id)
}

func (a *App) linearWorkspace(id string) error {
	for _, workspace := range a.store.Snapshot().Workspaces {
		if workspace.ID == id {
			return nil
		}
	}
	return fmt.Errorf("Workspace not found.")
}
