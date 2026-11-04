# Changes comparison

Changes defaults to the diff from the branch's merge base to the current files,
including committed, staged and unstaged edits, plus non-ignored untracked files.
Renames are detected by Git. The reference appears as **Compared to origin/main**
(or the selected PR base).

For the branch currently checked out, Orchestra queries GitHub for an open PR and
uses its base branch, including stacked PRs. The base is queried again on refresh,
so editing the PR base takes effect. Otherwise it uses `origin/HEAD`. Changes
refreshes every 15 seconds while visible; Refresh updates it immediately. The
status bar reads the actual local branch every 3 seconds, including detached HEAD.

All Git operations are read-only: no fetch, index refresh, checkout or ref updates.
Comparisons use local remote-tracking refs. Run your normal fetch outside Orchestra
if those refs are stale. If GitHub is unavailable, Orchestra uses `origin/HEAD`.
If a candidate has no local merge base, it tries the next candidate. Without a
usable base it falls back to **Since the worktree started**, using the saved start
commit. Repository-folder sessions fall back to uncommitted changes against HEAD.
The comparison selector also offers the original baseline explicitly.

Detached HEAD uses `origin/HEAD` without looking up a PR. Repositories without a
remote use the original baseline. Older worktrees without a saved start commit
retain their existing fallback to the merge base with the repository checkout.

After squash merge, the PR is no longer open, so Changes uses `origin/HEAD` and
normal merge-base semantics. Squash does not make the original branch commits
ancestors of main: their changes can remain visible even though an equivalent
patch was merged. Orchestra does not guess patch equivalence or hide these changes.
Rebase the branch onto the updated base or start a new branch for subsequent work.
