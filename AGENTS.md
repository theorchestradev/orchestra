# Working on Orchestra

Orchestra is a macOS and Linux app built with Vue 3, Wails 2, Go and xterm.js.

## UI and themes

- Every page, sidebar, dialog, overlay, menu trigger, field, button and GitHub view
  must follow the selected theme and light/dark mode, including while open.
- Use the shared semantic colors `canvas`, `sidebar`, `surface`, `text`, `muted`,
  `rule`, `accent` and `selection` from `frontend/src/shared/appearance.js` and CSS
  variables. Add shared semantic tokens when needed; do not define local palettes.
- Preserve agent brand colors and their light/dark assets. Status, errors and neutral
  backdrop shadows may use purpose-specific colors.
- Preserve keyboard navigation, focus indication, accessible names and selected states.
- Verify actual dialogs/overlays in light, dark and a custom theme, with smaller
  windows and long labels. The preview fixture does not prove native bridge behavior.
  `python3 scripts/visual-fixture.py /tmp/orchestra-visual` creates an isolated
  workspace with fake agents and GitHub data; open the built app with
  `ORCHESTRA_STATE_FILE`, `SHELL` and `PATH` pointing into that folder.

## Ownership and development

- Keep UI strings and repository documentation in English. Preserve unrelated changes.
  Reuse existing helpers and models before adding abstractions or dependencies.
- Session processes belong to TerminalSession in `internal/core/session.go`; workspace
  metadata belongs to Store and persistence in `internal/core/state.go`.
- Inbox routing and history belong to the existing collaboration store and its history file.
  Messaging must reuse sessions without creating another session type, a second
  harness, or automatically injecting terminal input. Only the user's explicit
  Execute Request action, or Store delivery after the user turns on **Deliver Inbox
  Requests Automatically**, may submit through TerminalSession. Views and CLI clients
  must not start agent processes.
- Keep feature UI in `frontend/src/components/` and shared appearance in
  `frontend/src/shared/` and `internal/appearance/`.
- The website lives in the separate `theorchestradev/site` repository. Its product
  preview mirrors the app's sidebar, status bar, GitHub view, Messages and Settings →
  Appearance markup, and its build reads `frontend/src/shared/style.css`, `themes.json`,
  `terminalColors.json`, `appearance.js` and `frontend/public/` from this repository.
  Keep those paths stable, and update the preview there when those views change.
- Build from the repository root with `make build`, which passes the About panel build
  info to Wails. When required, set
  `DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer` for native compilation.
  On Linux it builds with `-tags webkit2_41`; keep platform code behind build tags or
  `_darwin`/`_other` files so `.github/workflows/linux.yml` keeps passing.
- Releases come from `.github/workflows/release.yml`. Keep the release asset names, the
  `SHA256SUMS` format and the bundle identifier `dev.theorchestra.Orchestra` stable:
  the in-app updater, the website and macOS permissions depend on them.
- Application state lives in `~/.orchestra/`. Changing that location needs an explicit
  migration like `MigrateState` in `internal/core/state.go`.
  Generated builds, bindings, dependencies and local QA captures are ignored.

## Verification

Run checks appropriate to the change:

```sh
npm --prefix frontend run format:check
npm --prefix frontend test
npm --prefix frontend run build
go test -race ./...
go vet ./...
node scripts/PiStatusSmoke.mjs
node scripts/PiCollaborationSmoke.mjs
node scripts/OpenCodeSessionSmoke.mjs
make build
```

Keep tests isolated from real agent services and user state. Never send model prompts
or modify CLI credentials/settings as part of verification.
