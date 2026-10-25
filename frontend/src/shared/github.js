// Links between Work on / Review sessions and the GitHub items they came from.
export const gitHubItem = (item) => ({ number: item.number, pullRequest: Boolean(item.pull_request) })
export const itemSessions = (project, item) =>
  project?.sessions?.filter(
    (s) => s.gitHubItem?.number === item.number && Boolean(s.gitHubItem.pullRequest) === Boolean(item.pull_request),
  ) || []
export const sessionsLabel = (sessions) =>
  sessions.length === 1 ? `1 session: ${sessions[0].name}` : `${sessions.length} sessions`
// Work on / Review only makes sense while the item is open; merged PRs are closed.
export const canWorkOn = (item) => item?.state === 'open'
// Efforts for a model; for the agent's default model, those every effort-capable model accepts.
export const modelEfforts = (models, id) => {
  if (id) return models.find((m) => m.id === id)?.efforts || []
  const lists = models.map((m) => m.efforts || []).filter((e) => e.length)
  return lists.length ? lists[0].filter((e) => lists.every((l) => l.includes(e))) : []
}
export const effortLabel = (effort) =>
  ({ xhigh: 'Extra high' })[effort] || effort.charAt(0).toUpperCase() + effort.slice(1)
// Textareas normalize line endings, so compare drafts against the normalized source.
export const descriptionSource = (item) => (item?.body || '').replace(/\r\n?/g, '\n')
// GitHub answers 403 (and 404 for private items) when the user cannot edit an item.
export const isPermissionError = (message) =>
  /\b(403|404)\b|Resource not accessible|Must have (admin|push) access/i.test(String(message))
// A pull request's merge conflict, from the REST detail. GitHub sets `mergeable` to null while
// it computes it, so only a confirmed conflict counts and the next refresh picks it up.
export function pullConflict(pr) {
  if (!canWorkOn(pr) || !pr.head || pr.mergeable == null) return null
  if (pr.mergeable !== false && pr.mergeable_state !== 'dirty') return null
  return {
    number: pr.number,
    title: pr.title,
    url: pr.html_url,
    base: pr.base.ref,
    head: pr.head.ref,
    forkWithoutEdits: pr.head.repo?.full_name !== pr.base.repo?.full_name && !pr.maintainer_can_modify,
  }
}
// The REST API doesn't list the conflicting files, so the agent finds them locally. Merging
// rather than rebasing keeps the push from being a force-push.
export const conflictTask = ({ number, title, url, base, head }) => ({
  name: `#${number} Resolve conflicts: ${title || ''}`,
  prompt: [
    `Resolve the merge conflicts on this GitHub pull request: ${url}. Its branch ${head} has conflicts with ${base} that must be resolved.`,
    `1. Check out the pull request's head branch: gh pr checkout ${url}. If Git refuses because another worktree already has ${head} checked out, check it out to a new local branch instead (gh pr checkout ${url} --branch <new-name>) and push it with HEAD:${head} in step 4.`,
    `2. Fetch ${base} and merge it into the branch. Merge, don't rebase, so the push isn't a force-push.`,
    `3. Resolve each conflicting file, keeping the intent of both sides, then run the project's checks.`,
    `4. Commit the merge and push it to the pull request's head branch, ${head}. Then report which files you resolved and how.`,
    `If the push is refused, for example because the pull request comes from a fork that doesn't allow edits by maintainers, stop and explain why instead of pushing anywhere else.`,
  ].join('\n'),
})
