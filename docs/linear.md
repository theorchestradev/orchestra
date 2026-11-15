# Linear Work Items

Open the workspace menu and choose **Edit Workspace → Linear** and connect Linear with a personal API key from
Linear's **Settings → Security & access**. Grant read access to the teams you want
to browse. This initial integration uses personal keys; OAuth is not implemented.
Orchestra stores the key in macOS Keychain or the Linux desktop Secret Service
(`secret-tool`, provided by `libsecret-tools` on Ubuntu). The desktop keyring must
be available and unlocked. There is no plaintext fallback. The key is never saved
in workspace state or included in session prompts.

Choose a Linear team and optionally a project, and click
**Add Source**. You can edit or remove sources here. An optional default execution
repository preselects where **Work on** runs; it does not bind the team or project
to a single repository. Removing a local repository leaves the source available;
choose a remaining repository before starting work.

**Work Items** combines GitHub issues and Linear issues, ordered by recent updates.
Filter by source, assignee, identifier or title. Open/Closed groups Linear's native
workflow states: completed and canceled are closed, and the other states are open.
Rows and details still show the native status name. **Settings → General → Work
Items Auto Refresh** controls refreshing both providers and Reviews. Orchestra follows Linear's
pagination cursors and reports source failures without hiding successful sources.

Open an issue to read its description, status, assignee and labels, or use its
original Linear link. Descriptions use the same sanitized Markdown renderer as
other external content. Private Linear attachments may require opening Linear;
Orchestra does not forward its API key to images or external links.

**Work on** asks for an execution repository and offers the existing account,
agent, model and effort choices. It starts an ordinary session with the current
issue description as Markdown, using the existing worktree behavior. Stable
provider identity links the issue to sessions even when they work in different
repositories or the issue appears in several sources. Previously saved GitHub
session links continue to work.

Linear is read-only: issue editing, comments, status changes, pull request checks
and merge actions are not offered. GitHub editing and **Reviews** retain their
existing behavior. **Disconnect** removes the stored key but keeps workspace
source configuration and existing session links; reconnect to browse again.

API reference: <https://linear.app/developers/graphql>.

Each workspace has its own Linear connection. Connecting or disconnecting one workspace
does not affect another. Existing sources are preserved; if you previously used the
global connection, reconnect Linear separately in each workspace.
