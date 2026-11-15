# Windows support

Orchestra compiles and builds for Windows, and `.github/workflows/windows.yml` keeps it
that way. It is not released for Windows yet: terminal sessions, which the app is for,
do not run there. This page is the baseline for the port and the list of what is
missing. Issue [#66](https://github.com/theorchestradev/orchestra/issues/66) tracks it.

## Baseline

| | Supported |
| --- | --- |
| Windows version | Windows 10 version 1803 or later, and Windows 11. `AF_UNIX` sockets, which the CLI and the session host use, arrived in 1803. |
| Architecture | x64 (`windows/amd64`). ARM64 is a separate follow-up; nothing in the code prevents it. |
| WebView2 | The Evergreen runtime, which Windows 11 and updated Windows 10 include. Wails builds with `-webview2 download`, so an app started without it offers the Microsoft installer. |
| WSL | Never required. WSL integration is tracked separately. |
| Shell | PowerShell 7 (`pwsh.exe`) when installed, otherwise Windows PowerShell, otherwise `COMSPEC`. `SHELL` is ignored on Windows because it usually names an MSYS or WSL path. |
| State | `%USERPROFILE%\.orchestra\`, the same `~/.orchestra/` convention as macOS and Linux. Moving it needs an explicit migration like `MigrateState`. |
| Local IPC | Unix domain sockets in the runtime folder, as on macOS and Linux. The folder lives under the user's profile, whose ACL keeps other users out. Named pipes were considered and rejected for now: they would add a second transport and a second identity model for no gain on 1803 and later. |

## What works

- `go vet ./...`, `go test -race ./...` and `make build` on Windows, in CI and locally.
- The window, Settings, Workspaces, Work Items, Reviews, Changes, Files and the Board.
- `gh` and `git` run without a console window, with `PATH` joined by `;`.
- The `orchestra` CLI, built from `cmd/orchestra`, and its `orchestra.cmd` shim in the
  runtime folder. It still needs a session to have an app to talk to.

## What is unavailable, and how it fails

Every item fails with a visible error; nothing falls back silently.

| Feature | Behavior on Windows | Follow-up |
| --- | --- | --- |
| Terminal and agent sessions | Starting a session fails with "Terminal sessions are not available on Windows yet." | A ConPTY `terminalProcess` in `internal/core/process_windows.go`, a PowerShell form of the launch script in `loginShellArgs`, and agent commands that do not assume `sh`. Needs resize, Unicode, interrupt, exit status and job objects to stop process trees. |
| Keep Sessions Running After Quitting | The toggle is disabled in Settings. `sessionhost.Supported` is false, so Store never starts or connects to a host. | Run the host on ConPTY once sessions work; bump `sessionhost.Protocol` if messages change. |
| Model lists and the Orchestra Skill install | Fail with the sessions error, because they run through the login shell. | Same PowerShell launch as sessions. |
| Keep Awake | Holds `ES_SYSTEM_REQUIRED` from a hidden PowerShell process. Marked unavailable if PowerShell cannot start. | Verify on a real machine. |
| Linear and OpenRouter keys | Saving fails with "Secure key storage is not available on Windows yet." Nothing is written in plaintext. | Credential Manager or DPAPI in `internal/core/credentials_windows.go`. |
| Automatic updates | The update dialog offers the download; installing is not available. The future asset name is `Orchestra-<version>-windows-x86_64.zip`. | Replace the running executable, relaunch, and recover from a failed swap. Decide signing before advertising it. |
| Releases | `release.yml` builds macOS and Linux only. | Add a Windows job that produces the asset above and lists it in `SHA256SUMS` without changing the existing names or format. |
| Open in… apps | Editors found on `PATH` (`code`, `cursor`, …) and File Explorer. No icons. | Registry or Start Menu discovery and Windows terminals. |
| Monospace fonts | The font list is empty, because it comes from `fc-list`. | Enumerate fonts with DirectWrite or the registry. |
| Stopping helper processes | `killProcessGroup` ends only the helper itself, not its children. | Job objects, shared with the session work. |
| Native menu | None, as on Linux; the page handles shortcuts with Ctrl. | Verify shortcuts and focus in a real window. |

## Paths and Git

`filepath` handles separators, drive letters and spaces, and nothing in Store assumes
`/`. Case behavior is not covered yet: Windows volumes are usually case-insensitive, so
two repositories whose paths differ only by case would collide. Worktrees, Changes and
Files have not been exercised on Windows with non-ASCII paths. Both are part of the real
Windows QA that the port still needs.

## Verifying

Windows CI runs the frontend tests and build, `go vet`, `go test -race` and
`make build`. `.gitattributes` keeps every checkout at LF, which Prettier and the tests
that read source and documentation files expect. Most `internal/core` tests carry
`//go:build !windows`: they start fake agents as `sh` scripts, make symlinks, use `/tmp`
or check POSIX permission bits. They run again on Windows as each of those needs is
ported; pure helpers already run there. Local builds need Go, Node, GNU make and the Wails
CLI. There is no Windows visual fixture yet; dialogs, themes and keyboard navigation
need checking in a real window before a release.
