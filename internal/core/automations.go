package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Automation schedules a fresh agent session. Times use the saved IANA time zone.
type Automation struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	WorkspaceID   string    `json:"workspaceID"`
	RepositoryID  string    `json:"repositoryID"`
	Agent         string    `json:"agent"`
	Prompt        string    `json:"prompt"`
	Enabled       bool      `json:"enabled"`
	Days          []int     `json:"days"` // Sunday = 0; empty means a one-time execution.
	Time          string    `json:"time"`
	Date          string    `json:"date"`
	TimeZone      string    `json:"timeZone"`
	NextRun       time.Time `json:"nextRun"`
	LastRun       time.Time `json:"lastRun"`
	LastSessionID string    `json:"lastSessionID"`
	LastError     string    `json:"lastError"`
}

func (a Automation) next(after time.Time) (time.Time, error) {
	zone, err := time.LoadLocation(a.TimeZone)
	if err != nil {
		return time.Time{}, errors.New("Choose a valid time zone.")
	}
	clock, err := time.Parse("15:04", a.Time)
	if err != nil {
		return time.Time{}, errors.New("Choose a valid time.")
	}
	if len(a.Days) == 0 {
		date, err := time.ParseInLocation("2006-01-02", a.Date, zone)
		if err != nil {
			return time.Time{}, errors.New("Choose a valid date.")
		}
		at := time.Date(date.Year(), date.Month(), date.Day(), clock.Hour(), clock.Minute(), 0, 0, zone)
		if at.Hour() != clock.Hour() || at.Minute() != clock.Minute() {
			return time.Time{}, errors.New("This time does not exist because of daylight saving time. Choose another time.")
		}
		if !at.After(after) {
			return time.Time{}, errors.New("Choose a future date and time.")
		}
		return at, nil
	}
	for _, day := range a.Days {
		if day < 0 || day > 6 {
			return time.Time{}, errors.New("Choose valid weekdays.")
		}
	}
	local := after.In(zone)
	for i := 0; i <= 14; i++ {
		date := local.AddDate(0, 0, i)
		at := time.Date(date.Year(), date.Month(), date.Day(), clock.Hour(), clock.Minute(), 0, 0, zone)
		// A skipped DST clock time must not silently move the execution to a different hour.
		if at.Hour() == clock.Hour() && at.Minute() == clock.Minute() && slices.Contains(a.Days, int(at.Weekday())) && at.After(after) {
			return at, nil
		}
	}
	return time.Time{}, errors.New("No upcoming execution found.")
}

func (s *Store) SaveAutomation(a Automation) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a.Name, a.Prompt = strings.TrimSpace(a.Name), strings.TrimSpace(a.Prompt)
	if len(a.Prompt) > 256*1024 {
		return State{}, errors.New("Prompt exceeds 256 KB.")
	}
	if a.Name == "" || a.Prompt == "" {
		return State{}, errors.New("Name and prompt are required.")
	}
	if _, ok := agentSpecs[a.Agent]; !ok {
		return State{}, errors.New("Choose an agent.")
	}
	p, err := s.workspaceLocked(a.WorkspaceID)
	if err != nil {
		return State{}, err
	}
	repo, err := s.newSessionRepositoryLocked(p, a.RepositoryID)
	if err != nil {
		return State{}, err
	}
	a.RepositoryID = repo.ID
	index := slices.IndexFunc(s.state.Automations, func(item Automation) bool { return item.ID == a.ID })
	if a.ID != "" && index < 0 {
		return State{}, errors.New("Automation unavailable.")
	}
	now := time.Now()
	after := now
	// A completed or disabled one-time schedule remains editable. Re-enabling it
	// still requires a future time, and invalid dates/times are always rejected.
	if !a.Enabled && len(a.Days) == 0 {
		after = time.Time{}
	}
	next, err := a.next(after)
	if err != nil {
		return State{}, err
	}
	if !next.After(now) {
		next = time.Time{}
	}
	a.NextRun = next
	a.LastRun, a.LastSessionID, a.LastError = time.Time{}, "", ""
	if index >= 0 {
		old := s.state.Automations[index]
		a.LastRun, a.LastSessionID, a.LastError = old.LastRun, old.LastSessionID, old.LastError
	} else {
		a.ID = uuid.NewString()
	}
	previous := slices.Clone(s.state.Automations)
	if index < 0 {
		s.state.Automations = append(s.state.Automations, a)
	} else {
		s.state.Automations[index] = a
	}
	if err = s.saveLocked(); err != nil {
		s.state.Automations = previous
		return State{}, err
	}
	s.changedLocked()
	return s.snapshotLocked(), nil
}

func (s *Store) DeleteAutomation(id string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := slices.Clone(s.state.Automations)
	index := slices.IndexFunc(previous, func(a Automation) bool { return a.ID == id })
	if index < 0 {
		return State{}, errors.New("Automation unavailable.")
	}
	s.state.Automations = slices.Delete(slices.Clone(previous), index, index+1)
	if err := s.saveLocked(); err != nil {
		s.state.Automations = previous
		return State{}, err
	}
	s.changedLocked()
	return s.snapshotLocked(), nil
}

// RunAutomations belongs to the app lifecycle; the session host never writes schedules.
// Missed occurrences are coalesced into one execution when the app next runs.
func (s *Store) RunAutomations(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.runDueAutomations(now)
		}
	}
}

func (s *Store) runDueAutomations(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shuttingDown || !s.canSave {
		return
	}
	for i := range s.state.Automations {
		a := &s.state.Automations[i]
		if !a.Enabled || a.NextRun.IsZero() || a.NextRun.After(now) {
			continue
		}
		previous := *a
		a.LastRun, a.LastError, a.LastSessionID = now, "", ""
		a.Enabled = len(a.Days) > 0
		if a.Enabled {
			next, err := a.next(now)
			if err != nil {
				a.Enabled = false
				a.LastError = err.Error()
			}
			a.NextRun = next
		} else {
			a.NextRun = time.Time{}
		}
		// Claim the occurrence durably before creating or launching anything. A failed
		// launch is visible and never retried automatically with a duplicate prompt.
		if err := s.saveLocked(); err != nil {
			*a = previous
			continue
		}
		_ = s.launchAutomationLocked(a)
	}
}

// RunAutomation starts an extra run without changing the saved schedule or enabled state.
func (s *Store) RunAutomation(id string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shuttingDown {
		return State{}, errors.New("Orchestra is shutting down.")
	}
	index := slices.IndexFunc(s.state.Automations, func(a Automation) bool { return a.ID == id })
	if index < 0 {
		return State{}, errors.New("Automation unavailable.")
	}
	a := &s.state.Automations[index]
	previous := *a
	a.LastRun, a.LastError, a.LastSessionID = time.Now(), "", ""
	if err := s.saveLocked(); err != nil {
		*a = previous
		return State{}, err
	}
	err := s.launchAutomationLocked(a)
	return s.snapshotLocked(), err
}

func (s *Store) launchAutomationLocked(a *Automation) error {
	p, err := s.workspaceLocked(a.WorkspaceID)
	if err == nil {
		_, err = s.createSessionLocked(a.WorkspaceID, a.RepositoryID, a.Agent, a.Name, a.Prompt, false, nil, "", "", p.ProfileID, "", "", "", false)
		if err == nil {
			a.LastSessionID = p.Sessions[len(p.Sessions)-1].ID
			// Persist the session link before starting the process.
			if err = s.saveLocked(); err == nil {
				_, err = s.startSessionLocked(a.LastSessionID, false, 100, 30)
			}
		}
	}
	if err != nil {
		a.LastError = fmt.Sprintf("Could not start automation: %v", err)
		s.log.Warn("automation failed", "automation", a.ID, "error", err)
	}
	if saveErr := s.saveLocked(); saveErr != nil {
		s.log.Warn("automation result save failed", "error", saveErr)
		if err == nil {
			err = saveErr
		}
	}
	s.changedLocked()
	return err
}
