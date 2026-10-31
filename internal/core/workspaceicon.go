package core

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Workspace icons are favicons and app icons found in a workspace's first repository folder,
// shown in place of the folder symbol. They are read from disk and never stored in the state.
const (
	iconMaxBytes   = 2 << 20
	iconMaxDepth   = 3
	iconMaxEntries = 4000
	iconCacheTTL   = time.Minute
)

var iconTypes = map[string]string{
	".svg": "image/svg+xml",
	".png": "image/png",
	".ico": "image/x-icon",
}

// iconNames ranks file names, best first; favicon variants such as favicon-32x32.png come after them.
var iconNames = []string{"favicon.svg", "favicon.png", "favicon.ico", "icon.svg", "icon.png", "appicon.png", "apple-touch-icon.png", "apple-icon.png"}

// iconFolders ranks the folders web frameworks and app builders keep icons in, best first.
var iconFolders = []string{"", "public", "static", "app", "assets", "images", "img", "icons", "build", "src-tauri/icons", "resources"}

// skippedFolders are dependencies and generated output, which hold other workspaces' icons.
var skippedFolders = map[string]bool{"node_modules": true, "vendor": true, "dist": true, "out": true, "target": true, "coverage": true, "tmp": true, "Pods": true, "DerivedData": true, "Library": true}

// workspaceMarkers show a folder is a workspace, so its subfolders are worth searching for an icon.
var workspaceMarkers = []string{".git", "package.json", "go.mod", "Cargo.toml", "pyworkspace.toml", "composer.json", "Gemfile", "wails.json", "pubspec.yaml"}

type cachedIcon struct {
	data string
	read time.Time
}

var (
	workspaceIconsMu sync.Mutex
	workspaceIcons   = map[string]cachedIcon{}
)

// WorkspaceIcon returns a data URL for the icon in directory, or "" when it has none. Results are
// cached briefly, so adding an icon to a workspace shows up without restarting.
func WorkspaceIcon(directory string) string {
	workspaceIconsMu.Lock()
	cached, ok := workspaceIcons[directory]
	workspaceIconsMu.Unlock()
	if ok && time.Since(cached.read) < iconCacheTTL {
		return cached.data
	}
	data := ""
	if path := findWorkspaceIcon(directory); path != "" {
		data = iconDataURL(path)
	}
	workspaceIconsMu.Lock()
	workspaceIcons[directory] = cachedIcon{data, time.Now()}
	workspaceIconsMu.Unlock()
	return data
}

// findWorkspaceIcon looks a few levels into directory, so monorepos such as apps/web/public are found,
// and picks the best named icon, then the best folder, then the shallowest. Only repository folders are
// searched below their top level, and the home folder and its parents are never searched.
func findWorkspaceIcon(directory string) string {
	if isHomeOrAbove(directory) {
		return ""
	}
	maxDepth := 0
	if isWorkspace(directory) {
		maxDepth = iconMaxDepth
	}
	type candidate struct {
		path                string
		name, folder, depth int
	}
	var found []candidate
	entries := 0
	var walk func(dir, rel string, depth int)
	walk = func(dir, rel string, depth int) {
		items, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, item := range items {
			if entries++; entries > iconMaxEntries {
				return
			}
			name := item.Name()
			path := filepath.Join(dir, name)
			if item.IsDir() {
				if depth < maxDepth && !strings.HasPrefix(name, ".") && !skippedFolders[name] {
					walk(path, filepath.ToSlash(filepath.Join(rel, name)), depth+1)
				}
				continue
			}
			nameRank, folderRank := iconNameRank(name), iconFolderRank(rel)
			if nameRank < 0 || !item.Type().IsRegular() {
				continue
			}
			// Only favicons count anywhere; a plain icon.png elsewhere is usually a UI image.
			if folderRank == len(iconFolders) && !strings.HasPrefix(strings.ToLower(name), "favicon") {
				continue
			}
			found = append(found, candidate{path, nameRank, folderRank, depth})
		}
	}
	walk(directory, "", 0)
	if len(found) == 0 {
		return ""
	}
	sort.SliceStable(found, func(i, j int) bool {
		a, b := found[i], found[j]
		if a.name != b.name {
			return a.name < b.name
		}
		if a.folder != b.folder {
			return a.folder < b.folder
		}
		return a.depth < b.depth
	})
	for _, c := range found {
		if info, err := os.Stat(c.path); err == nil && info.Size() > 0 && info.Size() <= iconMaxBytes {
			return c.path
		}
	}
	return ""
}

func isWorkspace(directory string) bool {
	for _, marker := range workspaceMarkers {
		if _, err := os.Lstat(filepath.Join(directory, marker)); err == nil {
			return true
		}
	}
	return false
}

// isHomeOrAbove reports whether directory is the home folder or one of its parents, which hold
// unrelated workspaces and folders macOS asks permission to read.
func isHomeOrAbove(directory string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(directory, home)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func iconNameRank(name string) int {
	lower := strings.ToLower(name)
	if _, ok := iconTypes[filepath.Ext(lower)]; !ok {
		return -1
	}
	for i, known := range iconNames {
		if lower == known {
			return i
		}
	}
	if strings.HasPrefix(lower, "favicon") {
		return len(iconNames)
	}
	return -1
}

// iconFolderRank ranks a folder by its ending, so apps/web/public counts as public; other folders
// rank last.
func iconFolderRank(rel string) int {
	for i, folder := range iconFolders {
		if rel == folder || (folder != "" && strings.HasSuffix(rel, "/"+folder)) {
			return i
		}
	}
	return len(iconFolders)
}

func iconDataURL(path string) string {
	bytes, err := os.ReadFile(path)
	if err != nil || len(bytes) == 0 || len(bytes) > iconMaxBytes {
		return ""
	}
	return "data:" + iconTypes[strings.ToLower(filepath.Ext(path))] + ";base64," + base64.StdEncoding.EncodeToString(bytes)
}
