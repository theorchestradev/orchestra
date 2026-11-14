// Normalize Linear's read-only fields for the existing Work Items view.
export const linearItem = (issue, source) => ({
  provider: 'linear',
  id: issue.id,
  identifier: issue.identifier,
  title: issue.title,
  body: issue.description || '',
  html_url: issue.url,
  updated_at: issue.updatedAt,
  created_at: issue.createdAt,
  state: ['completed', 'canceled'].includes(issue.state.type) ? 'closed' : 'open',
  nativeStatus: issue.state.name,
  user: issue.creator && { login: issue.creator.name, avatar_url: issue.creator.avatarUrl },
  assignees: issue.assignee ? [{ login: issue.assignee.name, avatar_url: issue.assignee.avatarUrl }] : [],
  labels: issue.labels.nodes.map((label) => ({ ...label, color: label.color.replace(/^#/, '') })),
  sourceID: source.id,
  repositoryName: source.name,
  repositoryID: source.repositoryID || '',
})
export const executionRepository = (workspace, source, chosen = '') =>
  workspace?.repositories?.find((r) => r.id === (chosen || source?.repositoryID))?.id ||
  (workspace?.repositories?.length === 1 ? workspace.repositories[0].id : '')
