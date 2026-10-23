package main

import (
	"embed"
	"github.com/theorchestradev/orchestra/internal/core"
	"github.com/theorchestradev/orchestra/internal/sessionhost"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"io/fs"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	goruntime "runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

//go:embed frontend/public/app-icons/*.png
var appIcons embed.FS

func main() {
	core.Version = version
	if len(os.Args) > 1 && os.Args[1] == "--orchestra-cli" {
		os.Exit(core.RunCLI(os.Args[2:], os.Stdout, os.Stderr))
	}
	if len(os.Args) > 2 && os.Args[1] == sessionhost.Flag {
		os.Exit(runSessionHost(os.Args[2]))
	}
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		os.Exit(core.RunCLI(os.Args[1:2], os.Stdout, os.Stderr))
	}
	dist, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}
	// Logs go beside the workspace, so QA runs with ORCHESTRA_STATE_FILE never write to ~/.orchestra.
	logger, logFile, err := core.OpenLog(filepath.Join(filepath.Dir(statePath()), "logs"), os.Getenv("ORCHESTRA_LOG_LEVEL"))
	if err != nil {
		log.Printf("Logging is unavailable: %v", err)
		logger = slog.New(slog.DiscardHandler)
	} else {
		defer logFile.Close()
	}
	logger.Info("startup", "version", version, "worktree", buildWorktree, "branch", buildBranch, "commit", buildCommit, "os", goruntime.GOOS, "arch", goruntime.GOARCH)
	app := &App{log: logger}
	appMenu := menu.NewMenu()
	appMenu.Append(menu.AppMenu())
	file := appMenu.AddSubmenu("File")
	file.AddText("New Project…", keys.Combo("n", keys.CmdOrCtrlKey, keys.ShiftKey), func(*menu.CallbackData) { runtime.EventsEmit(app.ctx, "command", "project") })
	file.AddText("New Session", keys.CmdOrCtrl("n"), func(*menu.CallbackData) { runtime.EventsEmit(app.ctx, "command", "session") })
	// domReady moves these registered items into the native Orchestra menu.
	file.AddText("Settings…", keys.CmdOrCtrl(","), func(*menu.CallbackData) { runtime.EventsEmit(app.ctx, "command", "settings") })
	file.AddText("Check for Updates…", nil, func(*menu.CallbackData) { runtime.EventsEmit(app.ctx, "command", "update") })
	appMenu.Append(menu.EditMenu())
	view := appMenu.AddSubmenu("View")
	view.AddText("Toggle Sidebar", keys.CmdOrCtrl("b"), func(*menu.CallbackData) { runtime.EventsEmit(app.ctx, "command", "sidebar") })
	view.AddText("Toggle Changes", keys.Combo("b", keys.CmdOrCtrlKey, keys.ShiftKey), func(*menu.CallbackData) { runtime.EventsEmit(app.ctx, "command", "diff") })
	view.AddText("Toggle Files", keys.Combo("e", keys.CmdOrCtrlKey, keys.ShiftKey), func(*menu.CallbackData) { runtime.EventsEmit(app.ctx, "command", "files") })
	view.AddText("Toggle Pull Request", nil, func(*menu.CallbackData) { runtime.EventsEmit(app.ctx, "command", "pullRequest") })
	nav := appMenu.AddSubmenu("Navigate")
	nav.AddText("Command Bar…", keys.CmdOrCtrl("k"), func(*menu.CallbackData) { runtime.EventsEmit(app.ctx, "command", "command") })
	nav.AddText("Previous Item", keys.Combo("[", keys.CmdOrCtrlKey, keys.ShiftKey), func(*menu.CallbackData) { runtime.EventsEmit(app.ctx, "command", "previous") })
	nav.AddText("Next Item", keys.Combo("]", keys.CmdOrCtrlKey, keys.ShiftKey), func(*menu.CallbackData) { runtime.EventsEmit(app.ctx, "command", "next") })
	appMenu.Append(menu.WindowMenu())
	if goruntime.GOOS != "darwin" {
		// GTK menu accelerators run before the webview, so Ctrl+B, Ctrl+K and Ctrl+N would
		// never reach the terminal. The page handles shortcuts and Settings shows build info.
		appMenu = nil
	}
	// The webview draws no background of its own, so Settings can turn on the native vibrancy at
	// runtime; the page paints the opaque theme colors otherwise. Autoplay lets the attention sound
	// play while the user is in another app. File drops resolve to paths that terminals type.
	err = wails.Run(&options.App{Title: "Orchestra", Width: 1200, Height: 800, MinWidth: 700, MinHeight: 450, WindowStartState: options.Maximised, AssetServer: &assetserver.Options{Assets: dist}, BackgroundColour: &options.RGBA{R: 36, G: 36, B: 36, A: 255}, OnStartup: app.startup, OnDomReady: app.domReady, OnShutdown: app.shutdown, OnBeforeClose: app.beforeClose, Bind: []interface{}{app}, Menu: appMenu, DragAndDrop: &options.DragAndDrop{EnableFileDrop: true}, Mac: &mac.Options{TitleBar: mac.TitleBarHiddenInset(), WebviewIsTransparent: true, Preferences: &mac.Preferences{EnableAutoplayWithoutUserAction: mac.Enabled}, WindowIsTranslucent: false, About: &mac.AboutInfo{Title: "Orchestra", Message: aboutMessage()}}, Linux: &linux.Options{ProgramName: "Orchestra", Icon: icon}})
	if err != nil {
		logger.Error("window failed", "error", err)
		log.Fatal(err)
	}
	logger.Info("shutdown")
}

// runSessionHost serves sessions that keep running after the app quits. Its log lines go
// to the app's log, beside the workspace that owns dir.
func runSessionHost(dir string) int {
	logger, logFile, err := core.OpenLog(filepath.Join(filepath.Dir(dir), "logs"), os.Getenv("ORCHESTRA_LOG_LEVEL"))
	if err != nil {
		logger = slog.New(slog.DiscardHandler)
	} else {
		defer logFile.Close()
	}
	logger = logger.With("process", "host")
	if err = sessionhost.Run(dir, version, logger, sessionhost.Options{}); err != nil {
		logger.Error("session host failed", "error", err)
		return 1
	}
	return 0
}
