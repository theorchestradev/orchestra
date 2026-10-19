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

// Project icons are favicons and app icons found in a project's folder, shown in place of the
// folder symbol. They are read from disk and never stored in the workspace state.
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

// skippedFolders are dependencies and generated output, which hold other projects' icons.
var skippedFolders = map[string]bool{"node_modules": true, "vendor": true, "dist": true, "out": true, "target": true, "coverage": true, "tmp": true, "Pods": true, "DerivedData": true}

type cachedIcon struct {
	data string
	read time.Time
}

var (
	projectIconsMu sync.Mutex
	projectIcons   = map[string]cachedIcon{}
)

// ProjectIcon returns a data URL for the icon in directory, or "" when it has none. Results are
// cached briefly, so adding an icon to a project shows up without restarting.
func ProjectIcon(directory string) string {
	projectIconsMu.Lock()
	cached, ok := projectIcons[directory]
	projectIconsMu.Unlock()
	if ok && time.Since(cached.read) < iconCacheTTL {
		return cached.data
	}
	data := ""
	if path := findProjectIcon(directory); path != "" {
		data = iconDataURL(path)
	}
	projectIconsMu.Lock()
	projectIcons[directory] = cachedIcon{data, time.Now()}
	projectIconsMu.Unlock()
	return data
}

// findProjectIcon looks a few levels into directory, so monorepos such as apps/web/public are found,
// and picks the best named icon, then the best folder, then the shallowest.
func findProjectIcon(directory string) string {
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
				if depth < iconMaxDepth && !strings.HasPrefix(name, ".") && !skippedFolders[name] {
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
