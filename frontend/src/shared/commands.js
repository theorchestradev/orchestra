import { agents, themeOptions, isTerminal } from './appearance.js'
import { projectTitle } from './project.js'
import { shortcut } from './shortcuts.js'

// Command bar entries. `children` opens a nested list; leaves are run by App.vue through `run`.
export function commandItems({ projects = [], project, preferences = {}, sidebarHidden = false }) {
  const sessionAgents = [...agents]
    .sort((a, b) => Number(b.id === preferences.defaultAgent) - Number(a.id === preferences.defaultAgent))
    .map((a) => ({ id: a.id, name: a.name, agent: a.id, detail: a.id === preferences.defaultAgent ? 'Default' : '' }))
  const items = [
    {
      id: 'goto',
      group: 'Navigation',
      name: 'Go to…',
      icon: 'magnifyingglass',
      placeholder: 'Search projects and sessions',
      children: destinations(projects),
    },
    project?.repository && {
      id: 'github',
      group: 'Navigation',
      name: 'Open Issues & PRs',
      icon: 'list.bullet.rectangle',
    },
    project && {
      id: 'scratchpad',
      group: 'Navigation',
      name: project.hasScratchpad ? 'Open ' + (project.scratchpadName || 'Scratchpad') : 'New Scratchpad',
      icon: 'note.text',
    },
    project && {
      id: 'session',
      group: 'Create',
      name: 'New Session…',
      icon: 'terminal',
      shortcut: shortcut('session'),
      placeholder: 'Search agents',
      children: sessionAgents,
    },
    project && {
      id: 'worktreeSession',
      group: 'Create',
      name: 'New Session in Worktree…',
      icon: 'arrow.triangle.branch',
      placeholder: 'Search agents',
      children: sessionAgents,
    },
    project && { id: 'terminal', group: 'Create', name: 'New Terminal', icon: 'terminal' },
    { id: 'project', group: 'Create', name: 'New Project…', icon: 'folder', shortcut: shortcut('project') },
    { id: 'settings', group: 'App', name: 'Settings…', icon: 'gearshape', shortcut: shortcut('settings') },
    { id: 'update', group: 'App', name: 'Check for Updates…', icon: 'arrow.down.circle' },
    {
      id: 'sidebar',
      group: 'App',
      name: sidebarHidden ? 'Show Sidebar' : 'Hide Sidebar',
      icon: 'sidebar.left',
      shortcut: shortcut('sidebar'),
    },
    project && {
      id: 'diff',
      group: 'App',
      name: preferences.diffPaneOpen ? 'Hide Changes' : 'Show Changes',
      icon: 'sidebar.right',
      shortcut: shortcut('diff'),
    },
    {
      id: 'appearance',
      group: 'App',
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

export function destinations(projects = []) {
  return projects.flatMap((p) => [
    { id: p.id + ':project', name: projectTitle(p), icon: 'folder', project: p, destination: 'project' },
    ...(p.repository
      ? [
          {
            id: p.id + ':github',
            name: 'Issues & PRs',
            detail: projectTitle(p),
            icon: 'list.bullet.rectangle',
            project: p,
            destination: 'github',
          },
        ]
      : []),
    ...(p.hasScratchpad
      ? [
          {
            id: p.id + ':scratchpad',
            name: p.scratchpadName || 'Scratchpad',
            detail: projectTitle(p),
            icon: 'note.text',
            project: p,
            destination: 'scratchpad',
          },
        ]
      : []),
    ...p.sessions.map((s) => ({
      id: s.id,
      name: s.name,
      detail: [projectTitle(p), s.worktreeBranch].filter(Boolean).join(' · '),
      ...(isTerminal(s) ? { icon: 'terminal' } : { agent: s.agent }),
      project: p,
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
