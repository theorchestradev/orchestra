// The Open in button, the session menu and the command bar share these helpers. Go lists the
// installed apps and resolves the folder from the session; the file manager comes last.
export const fileManager = 'files'

// The folder a session runs in, for Copy Path.
export const sessionPath = (project, session) => session?.worktreeDirectory || project?.directory || ''

export const worktreeName = (session) =>
  session?.worktreeDirectory
    ? session.worktreeBranch || session.worktreeDirectory.split('/').filter(Boolean).at(-1)
    : ''

// The last app used, while it is still installed.
export const lastApp = (apps = [], id = '') => (id && apps.find((a) => a.id === id)) || null

// Menus list editors, terminals and Git tools in separate groups; the file manager goes with Copy Path.
const groups = [
  ['editor', 'Editors'],
  ['terminal', 'Terminals'],
  ['git', 'Git Tools'],
]
export const appGroups = (apps = []) =>
  groups
    .map(([kind, label]) => ({ kind, label, apps: apps.filter((a) => a.kind === kind) }))
    .filter((g) => g.apps.length)

// Apps without an icon from the system use a generic symbol.
export const appSymbol = (app) => (app.id === fileManager ? 'folder' : 'macwindow')
