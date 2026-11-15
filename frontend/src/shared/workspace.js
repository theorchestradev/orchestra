// A workspace's emoji is stored apart from its name, so sorting, search and the CLI keep the plain name.
export const workspaceTitle = (p) => (p?.emoji ? p.emoji + ' ' + p.name : p?.name || '')

// Arc Mode spaces show the emoji, or else the name's first letter.
export const spaceSymbol = (p) => p.emoji || Array.from(p.name.trim())[0]?.toUpperCase() || '•'

// New sessions preselect the workspace's agent, else the Default Agent preference.
export const workspaceAgent = (p, preferences) => p?.defaultAgent || preferences?.defaultAgent || ''

// New Session offers Initialize Git Repository only for this error from createSessionWorktree.
export const needsGitRepository = (error) =>
  /^(Error: )?A worktree requires a Git repository\b/.test(String(error ?? ''))

// A workspace's repositories, and the one each session works in for its whole life.
export const repositories = (p) => p?.repositories || []
export const sessionRepository = (p, session) => repositories(p).find((r) => r.id === session?.repositoryID) || null
// New Session starts in the repository chosen last in the workspace, else its first.
export const defaultRepository = (p) =>
  repositories(p).find((r) => r.id === p.lastRepositoryID) || repositories(p)[0] || null
// The sources Work Items ('workItems') or Reviews ('reviews') list, unless hidden in the
// workspace settings.
export const workspaceSources = (p, kind) => (p?.sources || []).filter((s) => s.kind === kind && !s.hidden)
export const hasWorkItems = (p) => workspaceSources(p, 'workItems').length > 0
export const hasReviews = (p) => workspaceSources(p, 'reviews').length > 0
// The repository's name, shown beside sessions only when the workspace has several.
export const repositoryLabel = (p, session) =>
  repositories(p).length > 1 ? sessionRepository(p, session)?.name || '' : ''
