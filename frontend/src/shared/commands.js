import { agents, themeOptions, isTerminal } from './appearance.js'
import { hasWorkItems, workspaceTitle, sessionRepository, workspaceAgent } from './workspace.js'
import { shortcut } from './shortcuts.js'
import { appSymbol } from './openIn.js'

// Command bar entries. `children` opens a nested list; leaves are run by App.vue through `run`.
export function commandItems({
  workspaces = [],
  workspace,
  session,
  apps = [],
  preferences = {},
  sidebarHidden = false,
}) {
  const defaultAgent = workspaceAgent(workspace, preferences)
  const sessionAgents = [...agents]
    .sort((a, b) => Number(b.id === defaultAgent) - Number(a.id === defaultAgent))
    .map((a) => ({ id: a.id, name: a.name, agent: a.id, detail: a.id === defaultAgent ? 'Default' : '' }))
  const items = [
    {
      id: 'goto',
      group: 'Navigation',
      name: 'Go to…',
      icon: 'magnifyingglass',
      placeholder: 'Search workspaces and sessions',
      children: destinations(workspaces),
    },
    hasWorkItems(workspace) && {
      id: 'github',
      group: 'Navigation',
      name: 'Open Work Items',
      icon: 'list.bullet.rectangle',
    },
    hasWorkItems(workspace) && {
      id: 'reviews',
      group: 'Navigation',
      name: 'Open Reviews',
      icon: 'arrow.triangle.pull',
    },
    workspace && {
      id: 'scratchpad',
      group: 'Navigation',
      name: workspace.hasScratchpad ? 'Open ' + (workspace.scratchpadName || 'Scratchpad') : 'New Scratchpad',
      icon: 'note.text',
    },
    // Leaves with `command` run that command with themselves as the item.
    ...(session
      ? apps.map((a) => ({
          id: 'openIn:' + a.id,
          command: 'openIn',
          app: a.id,
          group: 'Session',
          name: 'Open in ' + a.name,
          image: a.icon,
          icon: appSymbol(a),
        }))
      : []),
    session && { id: 'renameSession', group: 'Session', name: 'Rename Session…', icon: 'square.and.pencil' },
    session && { id: 'copyPath', group: 'Session', name: 'Copy Path', icon: 'doc.on.doc' },
    session && { id: 'closeSession', group: 'Session', name: 'Close Current Session', icon: 'trash' },
    workspace && {
      id: 'session',
      group: 'Create',
      name: 'New Session…',
      icon: 'terminal',
      shortcut: shortcut('session'),
      placeholder: 'Search agents',
      children: sessionAgents,
    },
    workspace && {
      id: 'worktreeSession',
      group: 'Create',
      name: 'New Session in Worktree…',
      icon: 'arrow.triangle.branch',
      placeholder: 'Search agents',
      children: sessionAgents,
    },
    workspace && { id: 'terminal', group: 'Create', name: 'New Terminal', icon: 'terminal' },
    { id: 'workspace', group: 'Create', name: 'New Workspace…', icon: 'folder', shortcut: shortcut('workspace') },
    { id: 'settings', group: 'App', name: 'Settings…', icon: 'gearshape', shortcut: shortcut('settings') },
    {
      id: 'statusBar',
      group: 'App',
      name: preferences.hideStatusBar ? 'Show Status Bar' : 'Hide Status Bar',
      icon: 'list.bullet.rectangle',
    },
    { id: 'update', group: 'App', name: 'Check for Updates…', icon: 'arrow.down.circle' },
    {
      id: 'sidebar',
      group: 'App',
      name: sidebarHidden ? 'Show Sidebar' : 'Hide Sidebar',
      icon: 'sidebar.left',
      shortcut: shortcut('sidebar'),
    },
    workspace && {
      id: 'diff',
      group: 'App',
      name: preferences.diffPaneOpen ? 'Hide Changes' : 'Show Changes',
      icon: 'sidebar.right',
      shortcut: shortcut('diff'),
    },
    workspace && { id: 'files', group: 'App', name: 'Toggle Files', icon: 'folder', shortcut: shortcut('files') },
    (session ? sessionRepository(workspace, session)?.gitHub : hasWorkItems(workspace)) && {
      id: 'pullRequest',
      group: 'App',
      name: 'Toggle Pull Request',
      icon: 'arrow.triangle.pull',
    },
    {
      id: 'appearance',
      group: 'App',
      // `preferences.appearance` is the appearance shown, never "system".
      name: preferences.appearance === 'light' ? 'Switch to Dark Appearance' : 'Switch to Light Appearance',
      icon: preferences.appearance === 'light' ? 'moon' : 'sun.max',
    },
    {
      id: 'theme',
      group: 'App',
      name: 'Change Theme…',
      icon: 'paintpalette',
      placeholder: 'Search themes',
      children: themeOptions().map(({ custom, ...t }) => ({ ...t, theme: t.id, detail: custom ? 'Custom' : '' })),
    },
  ]
  return items.filter(Boolean)
}

export function destinations(workspaces = []) {
  return workspaces.flatMap((p) => [
    { id: p.id + ':workspace', name: workspaceTitle(p), icon: 'folder', workspace: p, destination: 'workspace' },
    ...(hasWorkItems(p)
      ? [
          {
            id: p.id + ':github',
            name: 'Work Items',
            detail: workspaceTitle(p),
            icon: 'list.bullet.rectangle',
            workspace: p,
            destination: 'github',
          },
          {
            id: p.id + ':reviews',
            name: 'Reviews',
            detail: workspaceTitle(p),
            icon: 'arrow.triangle.pull',
            workspace: p,
            destination: 'reviews',
          },
        ]
      : []),
    ...(p.hasScratchpad
      ? [
          {
            id: p.id + ':scratchpad',
            name: p.scratchpadName || 'Scratchpad',
            detail: workspaceTitle(p),
            icon: 'note.text',
            workspace: p,
            destination: 'scratchpad',
          },
        ]
      : []),
    ...p.sessions.map((s) => ({
      id: s.id,
      name: s.name,
      detail: [workspaceTitle(p), p.repositories?.length > 1 && sessionRepository(p, s)?.name, s.worktreeBranch]
        .filter(Boolean)
        .join(' · '),
      ...(isTerminal(s) ? { icon: 'terminal' } : { agent: s.agent }),
      workspace: p,
      destination: s.id,
    })),
  ])
}

// Every query word must appear in the item's name or detail.
export function filterCommands(items, query) {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean)
  return items.filter((item) => {
    const text = (item.name + ' ' + (item.detail || '')).toLowerCase()
    return words.every((w) => text.includes(w))
  })
}
