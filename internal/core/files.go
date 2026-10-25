package core

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

type SessionFileEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
}

type SessionFiles struct {
	Entries   []SessionFileEntry `json:"entries"`
	Content   string             `json:"content"`
	Directory bool               `json:"directory"`
	Truncated bool               `json:"truncated"`
	Message   string             `json:"message"`
}

// SessionFiles reads only inside the session's repository or worktree, without starting a process.
func (s *Store) SessionFiles(id, relative string) (SessionFiles, error) {
	s.mu.Lock()
	_, sess, repo, err := s.sessionRepositoryLocked(id)
	if err != nil {
		s.mu.Unlock()
		return SessionFiles{}, err
	}
	project, worktree := repo.Directory, sess.WorktreeDirectory
	s.mu.Unlock()
	root, err := sessionFolder(project, worktree)
	if err != nil {
		return SessionFiles{}, err
	}
	return readSessionFiles(root, relative)
}

func readSessionFiles(root, relative string) (SessionFiles, error) {
	result := SessionFiles{Entries: []SessionFileEntry{}}
	if relative == "" {
		relative = "."
	}
	if !filepath.IsLocal(relative) {
		return result, fmt.Errorf("File must be inside the session folder")
	}
	for _, part := range strings.Split(filepath.Clean(relative), string(filepath.Separator)) {
		if part == ".git" {
			return result, fmt.Errorf("Git metadata is not available")
		}
	}
	// os.Root keeps symlink resolution inside the session folder, including during renames.
	folder, err := os.OpenRoot(root)
	if err != nil {
		return result, err
	}
	defer folder.Close()
	info, err := folder.Stat(relative)
	if err != nil {
		return result, err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		result.Message = "This file type cannot be previewed."
		return result, nil
	}
	f, err := folder.Open(relative)
	if err != nil {
		return result, err
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return result, err
	}
	result.Directory = info.IsDir()
	if info.IsDir() {
		entries, err := f.ReadDir(2001)
		if err != nil && err != io.EOF {
			return result, err
		}
		if len(entries) > 2000 {
			result.Truncated = true
			entries = entries[:2000]
		}
		for _, entry := range entries {
			if entry.Name() == ".git" {
				continue
			}
			result.Entries = append(result.Entries, SessionFileEntry{Name: entry.Name(), Path: filepath.ToSlash(filepath.Join(relative, entry.Name())), Directory: entry.IsDir()})
		}
		sort.Slice(result.Entries, func(i, j int) bool {
			a, b := result.Entries[i], result.Entries[j]
			if a.Directory != b.Directory {
				return a.Directory
			}
			return a.Name < b.Name
		})
		return result, nil
	}
	if !info.Mode().IsRegular() {
		result.Message = "This file type cannot be previewed."
		return result, nil
	}
	data, err := io.ReadAll(io.LimitReader(f, 256*1024+1))
	if err != nil {
		return result, err
	}
	if len(data) > 256*1024 {
		result.Message = "This file is too large to preview (256 KB limit)."
		return result, nil
	}
	if !utf8.Valid(data) || strings.ContainsRune(string(data), 0) {
		result.Message = "Binary file cannot be previewed."
		return result, nil
	}
	result.Content = string(data)
	return result, nil
}
