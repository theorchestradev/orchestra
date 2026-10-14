// Package apps finds the editors, terminals, Git tools and file manager installed on the computer
// and opens folders in them.
package apps

import "strings"

// App is an installed app that can open a folder. Icon is a PNG data URL, or empty when the
// platform has no icon for it.
type App struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind Kind   `json:"kind"`
	Icon string `json:"icon,omitempty"`
}

// Kind groups apps in menus, in this order.
type Kind string

const (
	Editor   Kind = "editor"
	Terminal Kind = "terminal"
	Git      Kind = "git"
	Files    Kind = "files"
)

// known describes an app Orchestra recognizes: its macOS bundle identifiers and Linux
// executables, in order of preference. On Linux, terminals start in the folder and receive
// directoryArgs, where %s is the folder, since a running instance may ignore the working directory.
type known struct {
	id, name      string
	kind          Kind
	bundleIDs     []string
	executables   []string
	directoryArgs []string
}

// FileManager opens folders in Finder or the desktop's file manager; it is listed last.
const FileManager = "files"

// The table is sorted by kind, so menus can separate the groups as they list it.
var knownApps = []known{
	{id: "vscode", name: "VS Code", kind: Editor, bundleIDs: []string{"com.microsoft.VSCode"}, executables: []string{"code"}},
	{id: "vscode-insiders", name: "VS Code Insiders", kind: Editor, bundleIDs: []string{"com.microsoft.VSCodeInsiders"}, executables: []string{"code-insiders"}},
	{id: "cursor", name: "Cursor", kind: Editor, bundleIDs: []string{"com.todesktop.230313mzl4w4u92"}, executables: []string{"cursor"}},
	{id: "windsurf", name: "Windsurf", kind: Editor, bundleIDs: []string{"com.exafunction.windsurf"}, executables: []string{"windsurf"}},
	{id: "zed", name: "Zed", kind: Editor, bundleIDs: []string{"dev.zed.Zed", "dev.zed.Zed-Preview"}, executables: []string{"zed", "zeditor"}},
	{id: "sublime", name: "Sublime Text", kind: Editor, bundleIDs: []string{"com.sublimetext.4", "com.sublimetext.3"}, executables: []string{"subl"}},
	{id: "nova", name: "Nova", kind: Editor, bundleIDs: []string{"com.panic.Nova"}},
	{id: "xcode", name: "Xcode", kind: Editor, bundleIDs: []string{"com.apple.dt.Xcode"}},
	{id: "intellij", name: "IntelliJ IDEA", kind: Editor, bundleIDs: []string{"com.jetbrains.intellij", "com.jetbrains.intellij.ce"}, executables: []string{"idea", "intellij-idea-ultimate", "intellij-idea-community"}},
	{id: "goland", name: "GoLand", kind: Editor, bundleIDs: []string{"com.jetbrains.goland"}, executables: []string{"goland"}},
	{id: "webstorm", name: "WebStorm", kind: Editor, bundleIDs: []string{"com.jetbrains.WebStorm"}, executables: []string{"webstorm"}},
	{id: "pycharm", name: "PyCharm", kind: Editor, bundleIDs: []string{"com.jetbrains.pycharm", "com.jetbrains.pycharm.ce"}, executables: []string{"pycharm", "pycharm-professional", "pycharm-community"}},
	{id: "clion", name: "CLion", kind: Editor, bundleIDs: []string{"com.jetbrains.CLion"}, executables: []string{"clion"}},
	{id: "rider", name: "Rider", kind: Editor, bundleIDs: []string{"com.jetbrains.rider"}, executables: []string{"rider"}},
	{id: "rubymine", name: "RubyMine", kind: Editor, bundleIDs: []string{"com.jetbrains.rubymine"}, executables: []string{"rubymine"}},
	{id: "phpstorm", name: "PhpStorm", kind: Editor, bundleIDs: []string{"com.jetbrains.PhpStorm"}, executables: []string{"phpstorm"}},
	{id: "rustrover", name: "RustRover", kind: Editor, bundleIDs: []string{"com.jetbrains.rustrover"}, executables: []string{"rustrover"}},
	{id: "terminal", name: "Terminal", kind: Terminal, bundleIDs: []string{"com.apple.Terminal"}},
	{id: "iterm", name: "iTerm", kind: Terminal, bundleIDs: []string{"com.googlecode.iterm2"}},
	{id: "ghostty", name: "Ghostty", kind: Terminal, bundleIDs: []string{"com.mitchellh.ghostty"}, executables: []string{"ghostty"}, directoryArgs: []string{"--working-directory=%s"}},
	{id: "warp", name: "Warp", kind: Terminal, bundleIDs: []string{"dev.warp.Warp-Stable"}, executables: []string{"warp-terminal"}},
	{id: "wezterm", name: "WezTerm", kind: Terminal, bundleIDs: []string{"com.github.wez.wezterm"}, executables: []string{"wezterm"}, directoryArgs: []string{"start", "--cwd", "%s"}},
	{id: "kitty", name: "kitty", kind: Terminal, bundleIDs: []string{"net.kovidgoyal.kitty"}, executables: []string{"kitty"}, directoryArgs: []string{"--directory", "%s"}},
	{id: "gnome-terminal", name: "GNOME Terminal", kind: Terminal, executables: []string{"gnome-terminal"}, directoryArgs: []string{"--working-directory=%s"}},
	{id: "konsole", name: "Konsole", kind: Terminal, executables: []string{"konsole"}, directoryArgs: []string{"--workdir", "%s"}},
	{id: "fork", name: "Fork", kind: Git, bundleIDs: []string{"com.DanPristupov.Fork"}},
	{id: "tower", name: "Tower", kind: Git, bundleIDs: []string{"com.fournova.Tower3"}},
	{id: "github-desktop", name: "GitHub Desktop", kind: Git, bundleIDs: []string{"com.github.GitHubClient"}},
	{id: "sublime-merge", name: "Sublime Merge", kind: Git, bundleIDs: []string{"com.sublimemerge"}, executables: []string{"smerge"}},
	{id: "sourcetree", name: "Sourcetree", kind: Git, bundleIDs: []string{"com.torusknot.SourceTreeNotMAS"}},
	{id: FileManager, name: fileManagerName, kind: Files, bundleIDs: []string{"com.apple.finder"}, executables: []string{"xdg-open"}},
}

// arguments are what a Linux executable receives to open folder.
func arguments(app known, folder string) []string {
	if app.kind != Terminal {
		return []string{folder}
	}
	args := make([]string, len(app.directoryArgs))
	for i, arg := range app.directoryArgs {
		args[i] = strings.ReplaceAll(arg, "%s", folder)
	}
	return args
}

func find(id string) (known, bool) {
	for _, app := range knownApps {
		if app.id == id {
			return app, true
		}
	}
	return known{}, false
}
