# Changelog

What's new in each Orchestra release. Download the latest from
[GitHub Releases](https://github.com/theorchestradev/orchestra/releases/latest), or
let Orchestra update itself when you ask it to.

## Unreleased

## 0.8.0 — 2026-10-20

### New

- **Browse a session's files.** A Files tab beside Changes lists the session's
  project folder or worktree. Select a file to read it with syntax highlighting
  and line numbers in the center of the window; open files appear in the
  sidebar until you close them. Toggle it from View → Toggle Files, the command
  bar, or Shift+Cmd+E (Ctrl+Shift+E on Linux).
  ([#44](https://github.com/theorchestradev/orchestra/pull/44))

### Changed

- **Usage shows what is left.** The status bar and its popover now report the
  allowance left in each window, matching Settings → Usage. Refresh usage from
  the popover. ([#45](https://github.com/theorchestradev/orchestra/pull/45))
- **A clearer local branch notice.** When local main is behind origin, the
  notice now appears as a card in the bottom-right corner that explains why it
  matters, links to the setting and offers to keep main up to date. The
  Settings section is now called Source Control.
  ([#37](https://github.com/theorchestradev/orchestra/pull/37))

### Fixed

- **Claude Code sessions with no saved conversation.** When Claude Code cannot
  find the conversation it is asked to resume, the session now says so and
  offers Start New Conversation instead of only reporting exit code 1.
  ([#43](https://github.com/theorchestradev/orchestra/pull/43))
- **Removing a worktree session no longer freezes the app.** Other sessions
  and the interface keep responding while the worktree is deleted.
  ([#41](https://github.com/theorchestradev/orchestra/pull/41))

## 0.7.0 — 2026-10-19

### New

- **Run Cline, omp and Kilo.** Start sessions with three more agent CLIs. Each
  session shows when the agent is working or waiting for you, and continues its
  conversation when reopened.
  - **Cline** starts with tool approvals on; press Shift+Tab in Cline to approve
    everything. Orchestra adds status hooks to Cline's `hooks` folder that do
    nothing outside Orchestra and never replace hooks of your own.
  - **omp** (oh-my-pi) auto-approves reads and edits and asks before running
    commands. Choose its model and thinking level in New Session.
  - **Kilo** keeps its own permissions, which ask before risky commands.
    Choose its model in New Session.
- **Close the current session from the command bar.** Search for Close Current
  Session to close the session you're viewing.

### Changed

- **OpenCode sessions show their status.** The sidebar now shows when OpenCode
  is working or waiting for you, including while a permission prompt is open.
- **Fit more project spaces in the sidebar.** Crowded spaces use compact dots
  that scroll, keeping every space within reach.

## 0.6.0 — 2026-10-19

### New

- **Drop files into a terminal.** Drag files onto a session to insert their
  shell-quoted paths, ready to use in a command.
  ([#35](https://github.com/theorchestradev/orchestra/pull/35))
- **Keep your local main branch up to date.** Settings → Git & Source Control
  can fetch and fast-forward the local base branch when creating a session
  worktree. It skips branches with uncommitted changes or local-only commits.
  A notice offers to enable it when the local branch is behind.
  ([#36](https://github.com/theorchestradev/orchestra/pull/36))

### Fixed

- Use the arrow keys to navigate choices in New Session.
- The local branch notice opens the correct Git & Source Control setting.
- Settings → Usage preserves the selected profile.

## 0.5.0 — 2026-10-19

### New

- **See agent usage in the status bar.** Check the current session agent's usage
  and reset time, or the agent closest to its limit when no agent is selected.
  Click to see every agent in Detailed or Compact view. Hide it in Settings → Usage.
  ([#29](https://github.com/theorchestradev/orchestra/pull/29))
- **Choose a profile when starting from GitHub.** The **Work on** and **Review**
  popups now let you pick the profile for the new session.
  ([#25](https://github.com/theorchestradev/orchestra/pull/25))
- **Recognize projects by their icon.** The sidebar now shows favicons and app
  icons found in the project folder.
  ([#31](https://github.com/theorchestradev/orchestra/pull/31))

### Changed

- On macOS, **Settings** and **Check for Updates** are now in the Orchestra menu.

### Fixed

- Worktree options are only offered for project folders inside a Git repository.
  ([#27](https://github.com/theorchestradev/orchestra/pull/27))
- Settings → Usage now opens with the selected profile instead of always using Default.
  ([#29](https://github.com/theorchestradev/orchestra/pull/29))

## 0.4.0 — 2026-10-18

### New

- **Choose your Dock icon.** Settings → Appearance now offers Color, White and Black
  app icons on macOS. The Dock updates as soon as you choose one.
  ([#23](https://github.com/theorchestradev/orchestra/pull/23))
- **Initialize Git when creating a session.** If a worktree session cannot start
  because the project folder is not a Git repository, the error now offers
  **Initialize Git Repository** and explains that a first commit is still needed.

### Fixed

- **Switch spaces with consecutive trackpad swipes.** A new swipe now works even
  while momentum from the previous gesture is still settling.

## 0.3.1 — 2026-10-15

### Changed

- **New app icon.** Orchestra has a new icon, and the White/Black choice in
  Settings → Appearance is gone.
  ([#21](https://github.com/theorchestradev/orchestra/pull/21))

### Fixed

- **Open in WezTerm works on macOS.** The WezTerm window no longer closes right
  after it opens.
  ([#21](https://github.com/theorchestradev/orchestra/pull/21))

## 0.3.0 — 2026-10-14

### New

- **Use more than one agent account.** Profiles let sessions in the same project sign
  in to different Claude Code or Codex accounts, so you can keep work and personal
  usage apart. Add a profile in Settings → Profiles and choose **Log In**. Each
  project picks the profile its new sessions use, and the New Session dialog can pick
  another. Settings → Usage shows usage for each profile.
  ([#14](https://github.com/theorchestradev/orchestra/pull/14))
- **Open a session's folder in the app you use.** The **Open in** button at the top
  right of a session opens its worktree in your editor, terminal, Git tool or Finder,
  or copies its path. It remembers the last app you chose. The same actions are in the
  session menu and the command bar, and Settings → Terminal can hide the button.
  ([#13](https://github.com/theorchestradev/orchestra/pull/13))

## 0.2.0 — 2026-10-14

### New

- **Hear when an agent needs you.** Orchestra can play a chime when a session stops
  working and waits for your input, so you can leave agents running and do something
  else. It stays quiet for the session you're looking at, and agents that finish
  together share one sound. Turn it on in Settings → Agents.
  ([#8](https://github.com/theorchestradev/orchestra/pull/8))
- **Follow your system's light or dark mode.** Settings → Appearance → Mode now offers
  System, Light and Dark. System switches with macOS as it changes and is the default
  for new installs. ([#9](https://github.com/theorchestradev/orchestra/pull/9))
- **Choose your terminal font.** Settings → Terminal → Font lists the monospaced fonts
  installed on your computer, and open terminals switch right away.
  ([#10](https://github.com/theorchestradev/orchestra/pull/10))
- **See every keyboard shortcut.** Settings → Shortcuts lists them all, grouped by
  area, with the right keys for your platform.
  ([#11](https://github.com/theorchestradev/orchestra/pull/11))

### Fixed

- On Linux, Ctrl+Enter now saves descriptions and sends requests, as the labels in the
  app already said. ([#11](https://github.com/theorchestradev/orchestra/pull/11))

## 0.1.0 — 2026-10-13

The first public release.

- **Run any agent.** Claude Code, Codex, OpenCode, Pi, Mistral Vibe, Antigravity CLI,
  Grok Build or a plain shell, each in its own session and grouped by project.
- **Keep agents out of each other's way.** Each session can run in its own Git worktree
  and branch.
- **Let sessions work together.** Agents send requests to other sessions and share a
  project Scratchpad through the `orchestra` CLI. Nothing is typed into a terminal
  unless you choose to.
- **Review work without leaving the app.** Browse GitHub issues and pull requests,
  start a session from one, and review the changes each session made.
- **Make it yours.** Built-in and custom themes, in light and dark.
- **Available for macOS and Linux.** A signed and notarized app for macOS 13 or later
  (Apple silicon and Intel), and a build for Linux x86_64.
