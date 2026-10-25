package core

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Limits keep a huge diff from freezing the Changes pane. Files over a limit keep their
// counts but omit their text.
const (
	maxDiffFiles     = 1000
	maxDiffFileBytes = 256 << 10
	maxDiffBytes     = 2 << 20
	maxDiffFileLines = 5000
	maxDiffLines     = 40000
)

var errWorktreeMissing = errors.New("The session worktree is no longer available. Restore its folder before resuming.")

// DiffFile is one changed file. Patch holds its hunks, starting at the first "@@" line.
type DiffFile struct {
	Path      string `json:"path"`
	OldPath   string `json:"oldPath,omitempty"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Binary    bool   `json:"binary,omitempty"`
	Untracked bool   `json:"untracked,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	Patch     string `json:"patch,omitempty"`
}

// SessionDiff is what a session changed. Scope is "worktree" or "folder"; without a scope,
// Message explains why there is nothing to compare.
type SessionDiff struct {
	Scope     string     `json:"scope,omitempty"`
	Message   string     `json:"message,omitempty"`
	Directory string     `json:"directory,omitempty"`
	Base      string     `json:"base,omitempty"`
	Files     []DiffFile `json:"files"`
	Additions int        `json:"additions"`
	Deletions int        `json:"deletions"`
	Truncated bool       `json:"truncated,omitempty"`
}

// sessionFolder is where a session runs: its worktree, or its repository folder without one.
func sessionFolder(projectDirectory, worktreeDirectory string) (string, error) {
	if worktreeDirectory == "" {
		if info, err := os.Stat(projectDirectory); err != nil || !info.IsDir() {
			return "", errors.New("The repository folder " + projectDirectory + " is no longer available.")
		}
		return projectDirectory, nil
	}
	if info, err := os.Stat(worktreeDirectory); err != nil || !info.IsDir() {
		return "", errWorktreeMissing
	}
	return worktreeDirectory, nil
}

// SessionFolder returns the existing folder a session runs in, for opening it in other apps.
func (s *Store) SessionFolder(id string) (string, error) {
	s.mu.Lock()
	_, sess, repo, err := s.sessionRepositoryLocked(id)
	if err != nil {
		s.mu.Unlock()
		return "", err
	}
	projectDirectory, worktreeDirectory := repo.Directory, sess.WorktreeDirectory
	s.mu.Unlock()
	return sessionFolder(projectDirectory, worktreeDirectory)
}

// SessionDiff only reads Git. It never starts, stops or writes to a session's process.
func (s *Store) SessionDiff(id string) (SessionDiff, error) {
	s.mu.Lock()
	_, sess, repo, err := s.sessionRepositoryLocked(id)
	if err != nil {
		s.mu.Unlock()
		return SessionDiff{}, err
	}
	agent, projectDirectory, worktreeDirectory, base := sess.Agent, repo.Directory, sess.WorktreeDirectory, sess.WorktreeBase
	s.mu.Unlock()
	return readSessionDiff(agent, projectDirectory, worktreeDirectory, base)
}

func readSessionDiff(agent, projectDirectory, worktreeDirectory, savedBase string) (SessionDiff, error) {
	result := SessionDiff{Files: []DiffFile{}}
	if agent == terminalAgent {
		result.Message = "Terminal sessions don’t track changes. Agent sessions show what they changed here."
		return result, nil
	}
	directory, err := sessionFolder(projectDirectory, worktreeDirectory)
	if err != nil {
		return SessionDiff{}, err
	}
	scope := "folder"
	if worktreeDirectory != "" {
		scope = "worktree"
	}
	if _, err := runCommand(directory, "git", "rev-parse", "--is-inside-work-tree"); err != nil {
		if scope == "worktree" {
			return SessionDiff{}, errWorktreeMissing
		}
		result.Message = "This folder isn’t a Git repository, so there are no changes to show."
		return result, nil
	}
	base, err := diffBase(directory, projectDirectory, savedBase, scope)
	if err != nil {
		return SessionDiff{}, err
	}
	result.Scope, result.Directory, result.Base = scope, directory, base
	files, err := trackedChanges(directory, base)
	if err != nil {
		return SessionDiff{}, err
	}
	untracked, err := runCommand(directory, "git", "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return SessionDiff{}, err
	}
	for _, path := range strings.Split(string(untracked), "\x00") {
		if path != "" {
			files = append(files, DiffFile{Path: path, Status: "A", Untracked: true})
		}
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	if len(files) > maxDiffFiles {
		files, result.Truncated = files[:maxDiffFiles], true
	}
	budget := maxDiffBytes
	for i := range files {
		f := &files[i]
		if f.Untracked {
			readUntracked(directory, f, budget)
		}
		if len(f.Patch) > budget {
			f.Patch, f.Truncated = "", true
		}
		budget -= len(f.Patch)
		result.Truncated = result.Truncated || f.Truncated
		result.Additions += f.Additions
		result.Deletions += f.Deletions
	}
	if files != nil {
		result.Files = files
	}
	return result, nil
}

// diffBase is the worktree's start commit, or HEAD for a session in the project folder.
func diffBase(directory, projectDirectory, saved, scope string) (string, error) {
	if scope == "folder" {
		if _, err := runCommand(directory, "git", "rev-parse", "--verify", "--quiet", "HEAD^{commit}"); err == nil {
			return "HEAD", nil
		}
		// A repository without commits compares against the empty tree.
		tree, err := runCommandInput(directory, "git", []byte{}, "hash-object", "-t", "tree", "--stdin")
		return strings.TrimSpace(string(tree)), err
	}
	if saved != "" {
		if _, err := runCommand(directory, "git", "rev-parse", "--verify", "--quiet", saved+"^{commit}"); err == nil {
			return saved, nil
		}
	}
	// Older sessions have no saved start commit.
	head, err := runCommand(projectDirectory, "git", "rev-parse", "--verify", "HEAD^{commit}")
	if err == nil {
		head, err = runCommand(directory, "git", "merge-base", "HEAD", strings.TrimSpace(string(head)))
	}
	if err != nil {
		return "", fmt.Errorf("Could not find the commit the session worktree started from: %w", err)
	}
	return strings.TrimSpace(string(head)), nil
}

// diffArgs uses plumbing diff-index because porcelain git diff refreshes the index,
// taking index.lock even with --no-optional-locks, which can fail the agent's git add.
func diffArgs(base string, extra ...string) []string {
	args := []string{"--literal-pathspecs", "-c", "core.quotePath=false", "diff-index", "--no-color", "--no-ext-diff", "--no-textconv", "-M", "--relative"}
	return append(append(args, extra...), base, "--")
}

// trackedChanges compares base with the working tree. Patches are read only for files
// within the line limits, so a large generated file is never loaded.
func trackedChanges(directory, base string) ([]DiffFile, error) {
	data, err := runCommand(directory, "git", diffArgs(base, "--numstat", "-z")...)
	if err != nil {
		return nil, err
	}
	var files []DiffFile
	fields := strings.Split(string(data), "\x00")
	for i := 0; i < len(fields); i++ {
		parts := strings.SplitN(fields[i], "\t", 3)
		if len(parts) != 3 {
			continue
		}
		f := DiffFile{Path: parts[2], Status: "M", Binary: parts[0] == "-"}
		f.Additions, _ = strconv.Atoi(parts[0])
		f.Deletions, _ = strconv.Atoi(parts[1])
		if f.Path == "" && i+2 < len(fields) {
			f.OldPath, f.Path, f.Status = fields[i+1], fields[i+2], "R"
			i += 2
		}
		files = append(files, f)
	}
	var selected []int
	var paths []string
	lines := 0
	for i, f := range files {
		changed := f.Additions + f.Deletions
		if !f.Binary && changed <= maxDiffFileLines && lines+changed <= maxDiffLines && len(selected) < maxDiffFiles {
			lines += changed
			selected = append(selected, i)
			paths = append(paths, f.Path)
			if f.OldPath != "" {
				paths = append(paths, f.OldPath)
			}
		} else if !f.Binary {
			files[i].Truncated = true
		}
	}
	if len(selected) == 0 {
		return files, nil
	}
	data, err = runCommand(directory, "git", append(diffArgs(base, "--patch"), paths...)...)
	if err != nil {
		return nil, err
	}
	patches := splitPatch(data)
	for n, i := range selected {
		if len(patches) != len(selected) {
			files[i].Truncated = true
			continue
		}
		header, hunks, _ := strings.Cut(patches[n], "\n@@")
		switch {
		case strings.Contains(header, "\nnew file mode"):
			files[i].Status = "A"
		case strings.Contains(header, "\ndeleted file mode"):
			files[i].Status = "D"
		}
		if hunks != "" {
			files[i].Patch = "@@" + hunks
		}
		if len(files[i].Patch) > maxDiffFileBytes {
			files[i].Patch, files[i].Truncated = "", true
		}
	}
	return files, nil
}

// splitPatch returns one section per "diff --git" header. Hunk lines always start with a
// marker, so the header cannot appear inside file content.
func splitPatch(data []byte) []string {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	return strings.Split(string(data), "\ndiff --git ")
}

// readUntracked renders a new file as a single added hunk, within budget.
func readUntracked(directory string, f *DiffFile, budget int) {
	path := filepath.Join(directory, f.Path)
	info, err := os.Lstat(path)
	if err != nil || info.IsDir() {
		return
	}
	var content []byte
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return
		}
		content = []byte(target)
	} else if !info.Mode().IsRegular() {
		return
	} else if info.Size() > int64(min(maxDiffFileBytes, budget)) {
		f.Truncated = true
		return
	} else if content, err = os.ReadFile(path); err != nil {
		return
	}
	if bytes.IndexByte(content[:min(len(content), 8000)], 0) >= 0 {
		f.Binary = true
		return
	}
	if len(content) == 0 {
		return
	}
	text := string(content)
	missingNewline := !strings.HasSuffix(text, "\n")
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	f.Additions = len(lines)
	var patch strings.Builder
	fmt.Fprintf(&patch, "@@ -0,0 +1,%d @@\n", len(lines))
	for _, line := range lines {
		patch.WriteString("+" + line + "\n")
	}
	if missingNewline {
		patch.WriteString("\\ No newline at end of file\n")
	}
	f.Patch = patch.String()
}
