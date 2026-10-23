import test from 'node:test'
import assert from 'node:assert/strict'
import { commandItems, destinations, filterCommands } from '../src/shared/commands.js'

const projects = [
  {
    id: 'p1',
    name: 'Orchestra',
    repository: 'theorchestradev/orchestra',
    hasScratchpad: true,
    scratchpadName: 'Notes',
    sessions: [{ id: 's1', name: 'Claude Code', agent: 'claude', worktreeBranch: 'feature' }],
  },
  { id: 'p2', name: 'Site', sessions: [{ id: 's2', name: 'Codex', agent: 'codex' }] },
]

test('command bar lists navigation, creation and app commands for the selected project', () => {
  const ids = commandItems({
    projects,
    project: projects[0],
    preferences: { appearance: 'dark', defaultAgent: 'claude' },
  }).map((c) => c.id)
  assert.deepEqual(ids, [
    'goto',
    'github',
    'scratchpad',
    'session',
    'worktreeSession',
    'terminal',
    'project',
    'settings',
    'update',
    'sidebar',
    'diff',
    'files',
    'pullRequest',
    'appearance',
    'theme',
  ])
  assert.equal(
    commandItems({ projects, project: projects[1], preferences: {} }).some((c) => c.id === 'pullRequest'),
    Boolean(projects[1].repository),
  )
  const none = commandItems({ projects: [], preferences: {} }).map((c) => c.id)
  assert.deepEqual(none, ['goto', 'project', 'settings', 'update', 'sidebar', 'appearance', 'theme'])
})

test('command labels follow current state', () => {
  const names = (items) => Object.fromEntries(items.map((c) => [c.id, c.name]))
  const light = names(
    commandItems({ projects, project: projects[1], preferences: { appearance: 'light' }, sidebarHidden: true }),
  )
  assert.equal(light.appearance, 'Switch to Dark Appearance')
  assert.equal(light.sidebar, 'Show Sidebar')
  assert.equal(light.diff, 'Show Changes')
  const open = commandItems({ projects, project: projects[0], preferences: { diffPaneOpen: true } })
  assert.equal(open.find((c) => c.id === 'diff').name, 'Hide Changes')
  assert.equal(light.scratchpad, 'New Scratchpad')
  assert.equal(light.github, undefined)
  const session = commandItems({ projects, project: projects[0], preferences: { defaultAgent: 'pi' } }).find(
    (c) => c.id === 'session',
  )
  assert.equal(session.children[0].agent, 'pi')
  assert.equal(session.children[0].detail, 'Default')
})

test('go to lists every project page and session', () => {
  const items = destinations(projects)
  assert.deepEqual(
    items.map((i) => [i.project.id, i.destination]),
    [
      ['p1', 'project'],
      ['p1', 'github'],
      ['p1', 'scratchpad'],
      ['p1', 's1'],
      ['p2', 'project'],
      ['p2', 's2'],
    ],
  )
  assert.equal(items[3].detail, 'Orchestra · feature')
})

test('command search matches every word across name and detail', () => {
  const items = destinations(projects)
  assert.deepEqual(
    filterCommands(items, 'orch claude').map((i) => i.id),
    ['s1'],
  )
  assert.deepEqual(filterCommands(items, '  ').length, items.length)
  assert.deepEqual(filterCommands(items, 'missing'), [])
})

test('plain terminals are not offered as agents and show the terminal symbol', () => {
  const session = commandItems({ projects, project: projects[1], preferences: { defaultAgent: 'claude' } }).find(
    (c) => c.id === 'session',
  )
  assert.ok(!session.children.some((c) => c.agent === 'terminal'))
  const terminal = destinations([
    { id: 'p3', name: 'Shell', sessions: [{ id: 's3', name: 'Terminal 1', agent: 'terminal' }] },
  ]).find((d) => d.id === 's3')
  assert.equal(terminal.icon, 'terminal')
  assert.equal(terminal.agent, undefined)
})

test('command bar opens the selected session folder in installed apps', () => {
  const apps = [
    { id: 'zed', name: 'Zed', icon: 'data:image/png;base64,AA==' },
    { id: 'files', name: 'Finder' },
  ]
  const items = commandItems({
    projects,
    project: projects[0],
    session: projects[0].sessions[0],
    apps,
    preferences: {},
  })
  const open = items.filter((c) => c.group === 'Session')
  assert.deepEqual(
    open.map((c) => [c.id, c.command, c.app, c.name, c.image, c.icon]),
    [
      ['openIn:zed', 'openIn', 'zed', 'Open in Zed', apps[0].icon, 'macwindow'],
      ['openIn:files', 'openIn', 'files', 'Open in Finder', undefined, 'folder'],
      ['copyPath', undefined, undefined, 'Copy Path', undefined, 'doc.on.doc'],
      ['closeSession', undefined, undefined, 'Close Current Session', undefined, 'trash'],
    ],
  )
  // Without a session there is no folder to open.
  assert.equal(
    commandItems({ projects, project: projects[0], apps, preferences: {} }).filter((c) => c.group === 'Session').length,
    0,
  )
})
