import test from 'node:test'
import assert from 'node:assert/strict'
import { commandItems, destinations, filterCommands } from '../src/shared/commands.js'

const workspaces = [
  {
    id: 'p1',
    name: 'Orchestra',
    repositories: [{ id: 'r1', name: 'orchestra', gitHub: 'theorchestradev/orchestra' }],
    hasScratchpad: true,
    scratchpadName: 'Notes',
    sessions: [{ id: 's1', name: 'Claude Code', agent: 'claude', worktreeBranch: 'feature' }],
  },
  { id: 'p2', name: 'Site', sessions: [{ id: 's2', name: 'Codex', agent: 'codex' }] },
]

test('command bar lists navigation, creation and app commands for the selected workspace', () => {
  const ids = commandItems({
    workspaces,
    workspace: workspaces[0],
    preferences: { appearance: 'dark', defaultAgent: 'claude' },
  }).map((c) => c.id)
  assert.deepEqual(ids, [
    'goto',
    'github',
    'reviews',
    'scratchpad',
    'session',
    'worktreeSession',
    'terminal',
    'workspace',
    'settings',
    'statusBar',
    'update',
    'sidebar',
    'diff',
    'files',
    'pullRequest',
    'appearance',
    'theme',
  ])
  assert.equal(
    commandItems({ workspaces, workspace: workspaces[1], preferences: {} }).some((c) => c.id === 'pullRequest'),
    false,
  )
  const none = commandItems({ workspaces: [], preferences: {} }).map((c) => c.id)
  assert.deepEqual(none, ['goto', 'workspace', 'settings', 'statusBar', 'update', 'sidebar', 'appearance', 'theme'])
})

test('command labels follow current state', () => {
  const names = (items) => Object.fromEntries(items.map((c) => [c.id, c.name]))
  const light = names(
    commandItems({ workspaces, workspace: workspaces[1], preferences: { appearance: 'light' }, sidebarHidden: true }),
  )
  assert.equal(light.appearance, 'Switch to Dark Appearance')
  assert.equal(light.sidebar, 'Show Sidebar')
  assert.equal(light.statusBar, 'Hide Status Bar')
  assert.equal(
    names(commandItems({ workspaces: [], preferences: { hideStatusBar: true } })).statusBar,
    'Show Status Bar',
  )
  assert.equal(light.diff, 'Show Changes')
  const open = commandItems({ workspaces, workspace: workspaces[0], preferences: { diffPaneOpen: true } })
  assert.equal(open.find((c) => c.id === 'diff').name, 'Hide Changes')
  assert.equal(light.scratchpad, 'New Scratchpad')
  assert.equal(light.github, undefined)
  const session = commandItems({ workspaces, workspace: workspaces[0], preferences: { defaultAgent: 'pi' } }).find(
    (c) => c.id === 'session',
  )
  assert.equal(session.children[0].agent, 'pi')
  assert.equal(session.children[0].detail, 'Default')
})

test('new session commands prefer the workspace agent over the Default Agent', () => {
  const workspace = { ...workspaces[0], defaultAgent: 'codex' }
  const session = commandItems({ workspaces, workspace, preferences: { defaultAgent: 'pi' } }).find(
    (c) => c.id === 'session',
  )
  assert.deepEqual(
    session.children.filter((c) => c.detail).map((c) => c.agent),
    ['codex'],
  )
  assert.equal(session.children[0].agent, 'codex')
})

test('go to lists every workspace page and session', () => {
  const items = destinations(workspaces)
  assert.deepEqual(
    items.map((i) => [i.workspace.id, i.destination]),
    [
      ['p1', 'workspace'],
      ['p1', 'github'],
      ['p1', 'reviews'],
      ['p1', 'scratchpad'],
      ['p1', 's1'],
      ['p2', 'workspace'],
      ['p2', 's2'],
    ],
  )
  assert.equal(items[4].detail, 'Orchestra · feature')
})

test('command search matches every word across name and detail', () => {
  const items = destinations(workspaces)
  assert.deepEqual(
    filterCommands(items, 'orch claude').map((i) => i.id),
    ['s1'],
  )
  assert.deepEqual(filterCommands(items, '  ').length, items.length)
  assert.deepEqual(filterCommands(items, 'missing'), [])
})

test('plain terminals are not offered as agents and show the terminal symbol', () => {
  const session = commandItems({ workspaces, workspace: workspaces[1], preferences: { defaultAgent: 'claude' } }).find(
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
    workspaces,
    workspace: workspaces[0],
    session: workspaces[0].sessions[0],
    apps,
    preferences: {},
  })
  const open = items.filter((c) => c.group === 'Session')
  assert.deepEqual(
    open.map((c) => [c.id, c.command, c.app, c.name, c.image, c.icon]),
    [
      ['openIn:zed', 'openIn', 'zed', 'Open in Zed', apps[0].icon, 'macwindow'],
      ['openIn:files', 'openIn', 'files', 'Open in Finder', undefined, 'folder'],
      ['renameSession', undefined, undefined, 'Rename Session…', undefined, 'square.and.pencil'],
      ['copyPath', undefined, undefined, 'Copy Path', undefined, 'doc.on.doc'],
      ['closeSession', undefined, undefined, 'Close Current Session', undefined, 'trash'],
    ],
  )
  // Without a session there is no folder to open.
  assert.equal(
    commandItems({ workspaces, workspace: workspaces[0], apps, preferences: {} }).filter((c) => c.group === 'Session')
      .length,
    0,
  )
})
