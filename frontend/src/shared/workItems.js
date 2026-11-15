// Work Items and Reviews show items that their providers normalize (WorkItem in
// internal/core/workitems.go). Only names and icons differ by provider here.
const providers = {
  github: { name: 'GitHub', icon: 'github' },
  linear: { name: 'Linear', icon: 'linear' },
}
export const providerName = (provider) => providers[provider]?.name || provider
export const providerIcon = (provider) => providers[provider]?.icon || 'circle.dotted'
// The link a session keeps to the item it came from.
export const itemReference = (item) => ({
  provider: item.provider,
  id: item.id,
  url: item.url,
  identifier: item.identifier,
  ...(item.number ? { number: item.number, review: Boolean(item.review) } : {}),
})
export const itemSessions = (workspace, item) =>
  workspace?.sessions?.filter((s) => s.workItem?.provider === item.provider && s.workItem.id === item.id) || []
export const itemKey = (item) => `${item.sourceID}:${item.id}`
// Work on starts in the item's repository, or in the one chosen for an item outside any
// repository, else the workspace's only one.
export const executionRepository = (workspace, item, chosen = '') =>
  workspace?.repositories?.find((r) => r.id === (chosen || item?.repositoryID))?.id ||
  (workspace?.repositories?.length === 1 ? workspace.repositories[0].id : '')
// Filters for Work Items: '' matches every source or assignee, and '-' items without assignees.
export const matchesWorkItem = (item, { source = '', assignee = '', search = '' }) =>
  (!source || item.sourceID === source) &&
  (!assignee || (assignee === '-' ? !item.assignees?.length : item.assignees?.some((a) => a.login === assignee))) &&
  (!search ||
    item.title.toLowerCase().includes(search.toLowerCase()) ||
    item.identifier.toLowerCase().includes(search.toLowerCase()))
export const workItemAssignees = (items) =>
  [...new Set(items.flatMap((item) => item.assignees?.map((a) => a.login) || []))].sort((a, b) => a.localeCompare(b))
export const sessionsLabel = (sessions) =>
  sessions.length === 1 ? `1 session: ${sessions[0].name}` : `${sessions.length} sessions`
// Work on / Review only makes sense while the item is open; merged pull requests are closed.
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
