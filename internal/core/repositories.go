package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// stateVersion 2 gives every workspace a list of repositories and every session one of them.
const stateVersion = 2

// migrateState moves the saved projects keys to workspaces, then turns version 1 workspaces,
// which had one folder, into workspaces with a single repository and links their sessions to
// it. IDs, sessions and settings are kept.
func migrateState(state *State) {
	if state.LegacyWorkspaces != nil {
		state.Workspaces, state.SelectedWorkspaceID = state.LegacyWorkspaces, state.LegacySelectedWorkspaceID
		state.LegacyWorkspaces, state.LegacySelectedWorkspaceID = nil, ""
	}
	for _, p := range state.Workspaces {
		if p == nil || len(p.Repositories) > 0 {
			continue
		}
		repo := &Repository{ID: uuid.NewString(), Name: folderName(p.Directory, p.Name), Directory: p.Directory}
		p.Repositories, p.Directory = []*Repository{repo}, ""
		for _, sess := range p.Sessions {
			if sess != nil {
				sess.RepositoryID = repo.ID
			}
		}
	}
	state.Version = stateVersion
}

// folderName is a repository's initial display name: its folder's name, or fallback for a root.
func folderName(directory, fallback string) string {
	if name := filepath.Base(directory); name != "." && name != string(filepath.Separator) && name != "" {
		return name
	}
	return fallback
}

// repositoryLocked finds a repository in the workspace. An empty id names the workspace's only
// repository, so callers never act on a guessed one when there are several.
func (s *Store) repositoryLocked(p *Workspace, id string) (*Repository, error) {
	if id == "" && len(p.Repositories) == 1 {
		return p.Repositories[0], nil
	}
	for _, repo := range p.Repositories {
		if repo.ID == id {
			return repo, nil
		}
	}
	if id == "" {
		return nil, errors.New("Choose a repository.")
	}
	return nil, errors.New("Repository unavailable.")
}

// workspaceRepository is repositoryLocked for callers that hold no lock.
func (s *Store) workspaceRepository(workspaceID, repositoryID string) (Repository, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.workspaceLocked(workspaceID)
	if err != nil {
		return Repository{}, err
	}
	repo, err := s.repositoryLocked(p, repositoryID)
	if err != nil {
		return Repository{}, err
	}
	return *repo, nil
}

// sessionRepositoryLocked returns the repository a session works in. validateState and
// RemoveRepository keep it present for every saved session.
func (s *Store) sessionRepositoryLocked(id string) (*Workspace, *Session, *Repository, error) {
	p, sess, err := s.sessionLocked(id)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, repo := range p.Repositories {
		if repo.ID == sess.RepositoryID {
			return p, sess, repo, nil
		}
	}
	return nil, nil, nil, errors.New("The session's repository is no longer in this workspace.")
}

// newSessionRepositoryLocked is where a new session works: the chosen repository, or without a
// choice the one New Session used last, else the first.
func (s *Store) newSessionRepositoryLocked(p *Workspace, id string) (*Repository, error) {
	if id == "" {
		id = p.LastRepositoryID
		if _, err := s.repositoryLocked(p, id); err != nil {
			id = p.Repositories[0].ID
		}
	}
	return s.repositoryLocked(p, id)
}

// repositoryFolder checks a folder chosen for a workspace and returns its absolute path.
func repositoryFolder(p *Workspace, directory string) (string, error) {
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return "", errors.New("Choose an existing folder.")
	}
	if directory, err = filepath.Abs(directory); err != nil {
		return "", err
	}
	if p != nil {
		for _, repo := range p.Repositories {
			if repo.Directory == directory {
				return "", fmt.Errorf("%s is already in this workspace as %s.", directory, repo.Name)
			}
		}
	}
	return directory, nil
}

// AddRepository adds a local folder to the workspace, named after the folder. It detects the
// folder's GitHub remote but never runs anything else in it.
func (s *Store) AddRepository(workspaceID, directory string) (State, error) {
	s.mu.Lock()
	p, err := s.workspaceLocked(workspaceID)
	if err == nil {
		directory, err = repositoryFolder(p, directory)
	}
	if err != nil {
		s.mu.Unlock()
		return State{}, err
	}
	repo := &Repository{ID: uuid.NewString(), Name: folderName(directory, p.Name), Directory: directory}
	previous := p.Repositories
	p.Repositories = append(append([]*Repository{}, previous...), repo)
	if err = s.saveLocked(); err != nil {
		p.Repositories = previous
		s.mu.Unlock()
		return State{}, err
	}
	s.changedLocked()
	s.mu.Unlock()
	return s.DetectRepository(workspaceID)
}

// RenameRepository changes the name the workspace shows for a repository; the folder stays.
func (s *Store) RenameRepository(workspaceID, repositoryID, name string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		return State{}, errors.New("Enter a repository name.")
	}
	p, err := s.workspaceLocked(workspaceID)
	if err != nil {
		return State{}, err
	}
	repo, err := s.repositoryLocked(p, repositoryID)
	if err != nil {
		return State{}, err
	}
	previous := repo.Name
	repo.Name = name
	if err = s.saveLocked(); err != nil {
		repo.Name = previous
		return State{}, err
	}
	s.changedLocked()
	return s.snapshotLocked(), nil
}

// RemoveRepository removes a repository from the workspace and keeps its folder on disk.
// Sessions stay linked to their repository for life, so it refuses while any session uses it,
// and a workspace keeps at least one repository.
func (s *Store) RemoveRepository(workspaceID, repositoryID string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.workspaceLocked(workspaceID)
	if err != nil {
		return State{}, err
	}
	repo, err := s.repositoryLocked(p, repositoryID)
	if err != nil {
		return State{}, err
	}
	if len(p.Repositories) == 1 {
		return State{}, errors.New("A workspace needs at least one repository. Add another before removing " + repo.Name + ".")
	}
	if n := len(repositorySessions(p, repo.ID)); n > 0 {
		return State{}, fmt.Errorf("Close the %d %s in %s before removing it.", n, plural(n, "session", "sessions"), repo.Name)
	}
	previous, previousLast := p.Repositories, p.LastRepositoryID
	p.Repositories = make([]*Repository, 0, len(previous)-1)
	for _, r := range previous {
		if r != repo {
			p.Repositories = append(p.Repositories, r)
		}
	}
	if p.LastRepositoryID == repo.ID {
		p.LastRepositoryID = ""
	}
	if err = s.saveLocked(); err != nil {
		p.Repositories, p.LastRepositoryID = previous, previousLast
		return State{}, err
	}
	s.changedLocked()
	return s.snapshotLocked(), nil
}

func repositorySessions(p *Workspace, repositoryID string) []*Session {
	var sessions []*Session
	for _, sess := range p.Sessions {
		if sess.RepositoryID == repositoryID {
			sessions = append(sessions, sess)
		}
	}
	return sessions
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
