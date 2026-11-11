package core

import (
	"errors"
	"strings"
)

// Pointers distinguish omitted update fields from explicit empty values.
type AutomationPatch struct {
	Name         *string `json:"name,omitempty"`
	Agent        *string `json:"agent,omitempty"`
	Model        *string `json:"model,omitempty"`
	Effort       *string `json:"effort,omitempty"`
	Prompt       *string `json:"prompt,omitempty"`
	RepositoryID *string `json:"repositoryID,omitempty"`
	Time         *string `json:"time,omitempty"`
	Date         *string `json:"date,omitempty"`
	TimeZone     *string `json:"timeZone,omitempty"`
	Days         *[]int  `json:"days,omitempty"`
	Enabled      *bool   `json:"enabled,omitempty"`
}

func (c *Collaboration) automationLocked(p *Workspace, req WireRequest) (any, error) {
	s := c.store
	if req.Method == "automation.list" {
		result := []Automation{}
		if req.Workspace != "" {
			var err error
			p, err = c.createWorkspaceLocked(p, req.Workspace)
			if err != nil {
				return nil, err
			}
		}
		for _, a := range s.state.Automations {
			if req.Workspace == "" || a.WorkspaceID == p.ID {
				result = append(result, a)
			}
		}
		return result, nil
	}
	a := Automation{TimeZone: "UTC"}
	if req.Method == "automation.create" {
		var err error
		p, err = c.createWorkspaceLocked(p, req.Workspace)
		if err != nil {
			return nil, err
		}
		_, source, err := s.sessionLocked(req.Source)
		if err != nil {
			return nil, err
		}
		a.WorkspaceID = p.ID
		a.RepositoryID = createRepositoryID(p, source, req)
	} else {
		found := false
		for _, item := range s.state.Automations {
			if item.ID == req.Target {
				a = item
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New("Automation unavailable. Use a stable automation ID from automation list.")
		}
		if req.Workspace != "" {
			return nil, errors.New("Use --workspace only with automation create or list.")
		}
	}
	switch req.Method {
	case "automation.show":
		return a, nil
	case "automation.delete":
		_, err := s.deleteAutomationLocked(a.ID)
		return map[string]any{"ok": err == nil, "id": a.ID}, err
	case "automation.run":
		_, err := s.runAutomationLocked(a.ID)
		for _, item := range s.state.Automations {
			if item.ID == a.ID {
				return item, err
			}
		}
		return nil, err
	case "automation.enable":
		a.Enabled = true
	case "automation.disable":
		a.Enabled = false
	case "automation.create", "automation.update":
		patch := req.Automation
		if patch == nil {
			return nil, errors.New("Automation configuration is required.")
		}
		for _, field := range []struct {
			dst *string
			src *string
		}{
			{&a.Name, patch.Name}, {&a.Agent, patch.Agent}, {&a.Model, patch.Model}, {&a.Effort, patch.Effort},
			{&a.Prompt, patch.Prompt}, {&a.RepositoryID, patch.RepositoryID}, {&a.Time, patch.Time}, {&a.Date, patch.Date}, {&a.TimeZone, patch.TimeZone},
		} {
			if field.src != nil {
				*field.dst = *field.src
			}
		}
		if patch.Days != nil {
			a.Days = *patch.Days
		}
		if patch.Enabled != nil {
			a.Enabled = *patch.Enabled
		}
	default:
		return nil, errors.New("Unknown automation command.")
	}
	state, err := s.saveAutomationLocked(a)
	if err != nil {
		return nil, err
	}
	if a.ID == "" {
		return state.Automations[len(state.Automations)-1], nil
	}
	for _, item := range state.Automations {
		if item.ID == a.ID {
			return item, nil
		}
	}
	return nil, errors.New("Automation unavailable.")
}

func automationDays(value string) ([]int, error) {
	if value == "once" {
		return []int{}, nil
	}
	if value == "daily" {
		return []int{0, 1, 2, 3, 4, 5, 6}, nil
	}
	days := []int{}
	for _, day := range strings.Split(value, ",") {
		index := -1
		for i, name := range []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"} {
			if day == name {
				index = i
			}
		}
		if index < 0 {
			return nil, errors.New("Use --days once, daily or comma-separated weekdays (sun,mon,tue,wed,thu,fri,sat).")
		}
		days = append(days, index)
	}
	return days, nil
}
