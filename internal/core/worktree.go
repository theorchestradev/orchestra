package core

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// repositoryName keeps worktree paths readable. Non-repositories fall back to the project ID
// and are rejected by createSessionWorktree.
func repositoryName(p *Project) string {
	root, err := runCommand(p.Directory, "git", "rev-parse", "--show-toplevel")
	if name := filepath.Base(strings.TrimSpace(string(root))); err == nil && name != "." && name != string(filepath.Separator) {
		return name
	}
	return p.ID
}

// gitTopLevel returns the repository root. Git runs in the C locale so isNotGitRepository
// can recognize its message whatever language the user's Git speaks.
func gitTopLevel(directory string) (string, error) {
	root, err := runCommand(directory, "/usr/bin/env", "LC_ALL=C", "git", "rev-parse", "--show-toplevel")
	return strings.TrimSpace(string(root)), err
}

// isNotGitRepository separates a folder outside any repository from missing Git,
// permission problems and other Git failures.
func isNotGitRepository(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not a git repository")
}

// InitializeGitRepository runs git init in the project folder, only when it is not already
// inside a repository. It never stages, commits, configures identity or adds remotes.
func (s *Store) InitializeGitRepository(projectID string) (State, error) {
	// Holding the lock serializes repeated requests, so a second one sees the new repository.
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.projectLocked(projectID)
	if err != nil {
		return State{}, err
	}
	root, err := gitTopLevel(p.Directory)
	if err == nil {
		return State{}, fmt.Errorf("The project folder is already in the Git repository at %s.", root)
	}
	if !isNotGitRepository(err) {
		return State{}, fmt.Errorf("Could not check the project folder: %w", err)
	}
	if _, err = runCommand(p.Directory, "git", "init"); err != nil {
		s.log.Warn("git init failed", "project", p.ID, "error", err)
		return State{}, fmt.Errorf("Could not initialize Git: %w", err)
	}
	s.log.Info("git repository initialized", "project", p.ID)
	return s.snapshotLocked(), nil
}

// CanCreateWorktree reports whether New Session can offer a worktree. Only a folder outside
// any repository is ruled out; other Git failures still surface when the session starts.
func (s *Store) CanCreateWorktree(projectID string) (bool, error) {
	s.mu.Lock()
	p, err := s.projectLocked(projectID)
	if err != nil {
		s.mu.Unlock()
		return false, err
	}
	directory := p.Directory
	s.mu.Unlock()
	_, err = gitTopLevel(directory)
	return !isNotGitRepository(err), nil
}

// Worktrees share Git history but never copy uncommitted project changes.
// It returns the working folder and the commit the session branch starts from.
func createSessionWorktree(directory, destination, branch string) (string, string, error) {
	root, err := gitTopLevel(directory)
	if isNotGitRepository(err) {
		// New Session offers Initialize Git Repository for this message; keep it stable.
		return "", "", fmt.Errorf("A worktree requires a Git repository: %w", err)
	}
	if err != nil {
		return "", "", fmt.Errorf("Could not read the project's Git repository: %w", err)
	}
	head, err := runCommand(directory, "git", "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", "", fmt.Errorf("Commit the repository before creating a worktree: %w", err)
	}
	base := strings.TrimSpace(string(head))
	// Resolve symlinks so a project opened through /var or another alias keeps its subfolder.
	projectDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return "", "", err
	}
	repositoryDirectory, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", err
	}
	relative, err := filepath.Rel(repositoryDirectory, projectDirectory)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", errors.New("The project folder is outside the Git repository.")
	}
	if _, err = runCommand(directory, "git", "show-ref", "--verify", "refs/heads/"+branch); err == nil {
		return "", "", errors.New("The session branch already exists. Create another session to use a new branch.")
	}
	if _, err = os.Lstat(destination); !os.IsNotExist(err) {
		return "", "", errors.New("The session worktree folder already exists or is inaccessible.")
	}
	if err = os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return "", "", err
	}
	if _, err = runCommand(directory, "git", "worktree", "add", "-b", branch, "--", destination, base); err != nil {
		// Git normally rolls back failed checkouts. Also handle interruption after registration.
		if _, statErr := os.Stat(destination); statErr == nil {
			if cleanupErr := removeSessionWorktree(directory, destination, branch); cleanupErr != nil {
				return "", "", fmt.Errorf("%w; worktree cleanup failed at %s: %v", err, destination, cleanupErr)
			}
		} else if _, branchErr := runCommand(directory, "git", "show-ref", "--verify", "refs/heads/"+branch); branchErr == nil {
			if _, cleanupErr := runCommand(directory, "git", "branch", "-d", "--", branch); cleanupErr != nil {
				return "", "", fmt.Errorf("%w; branch cleanup failed: %v", err, cleanupErr)
			}
		}
		return "", "", err
	}
	workingDirectory := filepath.Join(destination, relative)
	if info, statErr := os.Stat(workingDirectory); statErr != nil || !info.IsDir() {
		err = errors.New("The project subfolder does not exist in the current commit. Commit it before creating a worktree.")
		if cleanupErr := removeSessionWorktree(directory, destination, branch); cleanupErr != nil {
			return "", "", fmt.Errorf("%w; worktree cleanup failed at %s: %v", err, destination, cleanupErr)
		}
		return "", "", err
	}
	return workingDirectory, base, nil
}

// Only used to roll back creation. Never force removal of a checkout with user changes.
func removeSessionWorktree(directory, destination, branch string) error {
	if _, err := runCommand(directory, "git", "worktree", "remove", "--", destination); err != nil {
		return err
	}
	_, err := runCommand(directory, "git", "branch", "-d", "--", branch)
	return err
}

// cleanSessionWorktree returns the checkout root only when nothing uncommitted would be lost.
func cleanSessionWorktree(workingDirectory string) (string, error) {
	root, err := runCommand(workingDirectory, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("The session worktree is no longer available: %w", err)
	}
	status, err := runCommand(workingDirectory, "git", "status", "--porcelain")
	if err != nil {
		return "", err
	}
	if len(strings.TrimSpace(string(status))) > 0 {
		return "", errors.New("The session worktree has uncommitted changes. Commit or discard them before removing it.")
	}
	return strings.TrimSpace(string(root)), nil
}

// discardSessionWorktree never forces removal, and keeps an unmerged branch so its commits survive.
// An unmerged branch is expected, so its failure is only logged at debug level.
func discardSessionWorktree(directory, destination, branch string, log *slog.Logger) error {
	if _, err := runCommand(directory, "git", "worktree", "remove", "--", destination); err != nil {
		return err
	}
	if _, err := runCommand(directory, "git", "branch", "-d", "--", branch); err != nil {
		log.Debug("worktree branch kept", "branch", branch, "error", err)
	}
	return nil
}

// defaultBranch names the branch origin/HEAD points to, falling back to main, then master.
func defaultBranch(directory string) (string, error) {
	if head, err := runCommand(directory, "git", "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
		if branch := strings.TrimPrefix(strings.TrimSpace(string(head)), "origin/"); branch != "" {
			return branch, nil
		}
	}
	for _, branch := range []string{"main", "master"} {
		if _, err := runCommand(directory, "git", "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+branch); err == nil {
			return branch, nil
		}
	}
	return "", errors.New("The repository has no origin default branch.")
}

// localMainBehind counts the commits local main lacks from origin. It reads fetched refs only.
func localMainBehind(directory string) (string, int, error) {
	branch, err := defaultBranch(directory)
	if err != nil {
		return "", 0, err
	}
	count, err := runCommand(directory, "git", "rev-list", "--count", "refs/heads/"+branch+"..refs/remotes/origin/"+branch)
	if err != nil {
		return branch, 0, err
	}
	behind, err := strconv.Atoi(strings.TrimSpace(string(count)))
	return branch, behind, err
}

// updateLocalMain fetches the default branch and fast-forwards local main only when nothing
// local would be lost: no local-only commits, and a clean checkout wherever it is checked out.
// It never rebases, resets, stashes or forces.
func updateLocalMain(directory string) (string, error) {
	branch, err := defaultBranch(directory)
	if err != nil {
		return "", err
	}
	if _, err = runCommand(directory, "git", "fetch", "--quiet", "--no-tags", "origin", "+refs/heads/"+branch+":refs/remotes/origin/"+branch); err != nil {
		return branch, err
	}
	local, err := runCommand(directory, "git", "rev-parse", "--verify", "--quiet", "refs/heads/"+branch+"^{commit}")
	if err != nil {
		// No local branch to keep up to date.
		return branch, nil
	}
	remote, err := runCommand(directory, "git", "rev-parse", "--verify", "refs/remotes/origin/"+branch+"^{commit}")
	if err != nil {
		return branch, err
	}
	localCommit, remoteCommit := strings.TrimSpace(string(local)), strings.TrimSpace(string(remote))
	if localCommit == remoteCommit {
		return branch, nil
	}
	if _, err = runCommand(directory, "git", "merge-base", "--is-ancestor", localCommit, remoteCommit); err != nil {
		return branch, fmt.Errorf("Local %s has commits that are not on origin, so it was not updated.", branch)
	}
	checkout, err := branchCheckout(directory, branch)
	if err != nil {
		return branch, err
	}
	if checkout == "" {
		_, err = runCommand(directory, "git", "update-ref", "refs/heads/"+branch, remoteCommit, localCommit)
		return branch, err
	}
	status, err := runCommand(checkout, "git", "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return branch, err
	}
	if len(strings.TrimSpace(string(status))) > 0 {
		return branch, fmt.Errorf("Local %s has uncommitted changes, so it was not updated.", branch)
	}
	_, err = runCommand(checkout, "git", "merge", "--ff-only", "--quiet", remoteCommit)
	return branch, err
}

// branchCheckout returns the worktree that has branch checked out, or "" when none does.
func branchCheckout(directory, branch string) (string, error) {
	list, err := runCommand(directory, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return "", err
	}
	path := ""
	for _, line := range strings.Split(string(list), "\n") {
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			path = p
		} else if line == "branch refs/heads/"+branch {
			return path, nil
		}
	}
	return "", nil
}
