<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="frontend/public/app-icons/white.png">
    <img src="frontend/public/app-icons/black.png" alt="Orchestra logo" width="160">
  </picture>
</p>

<h1 align="center">Orchestra</h1>

<p align="center">
  A desktop app for running coding agents side by side in project-based terminal sessions.<br>
  <a href="https://github.com/theorchestradev/orchestra/releases/latest">Download</a> ·
  <a href="https://theorchestra.dev">Website</a>
</p>

- **Any agent:** Claude Code, Codex, OpenCode, Pi, Mistral Vibe, Antigravity CLI, Grok
  Build, or a plain shell, each in its own session.
- **Worktrees:** every session can run in its own Git worktree and branch, so agents
  don't step on each other.
- **Sessions talk to each other:** agents send requests between sessions and share a
  project Scratchpad through the `orchestra` CLI. Nothing is typed into a terminal
  unless you choose to.
- **GitHub and diffs:** browse issues and pull requests, start a session from one, and
  review each session's changes.

## Install

Download the latest release:

- **macOS 13 or later:**
  [Orchestra-macos-universal.dmg](https://github.com/theorchestradev/orchestra/releases/latest/download/Orchestra-macos-universal.dmg)
  (Apple silicon and Intel).
- **Linux x86_64:** `Orchestra-…-linux-x86_64.tar.gz`. Needs GTK 3 and WebKitGTK 4.1
  (`libgtk-3-0 libwebkit2gtk-4.1-0` on Ubuntu 22.04 or later).

Orchestra updates itself from GitHub Releases when you ask it to.

Install the CLI of each agent you want to use and make sure it is on your shell's `PATH`.
For the GitHub views, install the [GitHub CLI](https://cli.github.com/) and run
`gh auth login`.

## Use

1. Add a project from an existing folder.
2. Add a session and choose an agent. **Create a new worktree** is on by default and puts
   the session on a new branch under `~/.orchestra/worktrees/`.
3. When you close the session, **Remove the worktree** deletes a clean checkout and
   keeps any branch with unmerged commits.

To let agents discover the `orchestra` CLI, install the skill from
Settings → Agents → Agent Skill, or run:

```sh
npx skills add https://github.com/theorchestradev/orchestra --skill orchestra -g
```

## Build from source

Requires Go, Node.js and the [Wails CLI](https://wails.io) v2.16.0 or later
(`go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0`), plus the Xcode command
line tools on macOS or these packages on Ubuntu:

```sh
sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
```

```sh
npm --prefix frontend ci
wails dev                # on Linux: wails dev -tags webkit2_41
make build               # build/bin/Orchestra.app, or build/bin/orchestra on Linux
```

See [RELEASING.md](RELEASING.md) for versioned, signed builds.

## Contributing

Read [AGENTS.md](AGENTS.md) for the conventions and the checks to run before opening a
pull request. Tests never send prompts to agent services or touch your agent settings.

## Privacy

Usage sharing is off until you opt in. [Privacy & usage data](docs/privacy.md) lists
every event and how to disable it.

## License

[MIT](LICENSE)
