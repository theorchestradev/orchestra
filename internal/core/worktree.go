package core

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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

// Worktrees share Git history but never copy uncommitted project changes.
// It returns the working folder and the commit the session branch starts from.
func createSessionWorktree(directory, destination, branch string) (string, string, error) {
	root, err := runCommand(directory, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", fmt.Errorf("A worktree requires a Git repository: %w", err)
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
	repositoryDirectory, err := filepath.EvalSymlinks(strings.TrimSpace(string(root)))
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
