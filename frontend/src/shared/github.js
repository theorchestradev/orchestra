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
