import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import {
  themeNames,
  palette,
  terminalTheme,
  translucencyTokens,
  agentIcon,
  effectiveAppearance,
  destination,
  runLabel,
  startupDestination,
  sidebarItems,
  projectRows,
  adjacentItem,
  dropIndex,
  activityLabel,
  statusSummary,
  visibleSidebarProjects,
  arcScrollDelta,
} from '../src/shared/appearance.js'
import themes from '../src/shared/themes.json' with { type: 'json' }
// internal/appearance validates the colors and names in themes.json with the custom-theme rules.
test('every built-in theme is named from themes.json and drives the terminal', () => {
  assert.deepEqual(
    themeNames.map((t) => t.name),
    Object.values(themes).map((t) => t.name),
  )
  for (const [id, name] of [
    ['system', 'System'],
    ['github', 'GitHub'],
    ['vs-code-plus', 'VS Code Plus'],
    ['og', 'OG'],
    ['dracula', 'Dracula'],
  ])
    assert.equal(themeNames.find((t) => t.id === id)?.name, name)
  for (const theme of themeNames)
    for (const mode of ['dark', 'light']) {
      const prefs = { theme: theme.id, appearance: mode }
      assert.equal(Object.keys(palette(prefs)).length, 8)
      assert.equal(terminalTheme(prefs).background, palette(prefs).canvas)
      assert.equal(terminalTheme(prefs).selectionBackground, palette(prefs).selection)
    }
})
test('original agent brand variants exist in both modes', () => {
  for (const agent of ['codex', 'claude', 'pi', 'vibe', 'opencode', 'antigravity', 'grok'])
    for (const mode of ['dark', 'light'])
      assert.ok(fs.existsSync(new URL('../public' + agentIcon(agent, mode), import.meta.url)))
})
test('System mode shows the OS appearance and explicit modes keep their own', () => {
  assert.equal(effectiveAppearance('system', 'light'), 'light')
  assert.equal(effectiveAppearance('system', 'dark'), 'dark')
  assert.equal(effectiveAppearance('light', 'dark'), 'light')
  assert.equal(effectiveAppearance('dark', 'light'), 'dark')
  // Outside macOS, and without matchMedia in tests, System falls back to Dark.
  assert.equal(effectiveAppearance('system'), 'dark')
  assert.equal(effectiveAppearance(undefined, 'light'), 'dark')
})
test('project navigation preserves page priority and messaging labels', () => {
  assert.equal(destination({ showsScratchpad: true, showsGitHub: true, selectedSessionID: 'a' }), 'scratchpad')
  assert.equal(destination({ showsGitHub: true, selectedSessionID: 'a' }), 'github')
  assert.equal(destination({ selectedSessionID: 'a' }), 'a')
  assert.equal(destination({}), 'empty')
  assert.equal(runLabel('queued'), 'In inbox')
  assert.equal(runLabel('running'), 'Awaiting reply')
})

test('startup views never select or resume a restored agent', () => {
  const project = { repository: 'example/orchestra', selectedSessionID: 'restored-agent' }
  assert.equal(startupDestination(project, { startupView: 'last' }), null)
  assert.equal(startupDestination(project, { startupView: 'scratchpad' }), 'scratchpad')
  assert.equal(startupDestination(project, { startupView: 'github' }), 'github')
  assert.equal(startupDestination({ ...project, repository: '' }, { startupView: 'github' }), null)
  assert.equal(startupDestination(undefined, { startupView: 'scratchpad' }), null)
})

test('sidebar navigation follows visible order and wraps', () => {
  const projects = [
    { id: 'a', isExpanded: true, repository: 'o/a', hasScratchpad: true, sessions: [{ id: 's1' }, { id: 's2' }] },
    { id: 'b', isExpanded: false, repository: 'o/b', hasScratchpad: true, sessions: [{ id: 's3' }] },
    { id: 'c', isExpanded: true, repository: '', hasScratchpad: false, sessions: [{ id: 's4' }] },
  ]
  assert.deepEqual(
    sidebarItems(projects).map((i) => i.project.id + ':' + i.destination),
    ['a:github', 'a:scratchpad', 'a:s1', 'a:s2', 'c:s4'],
  )
  assert.equal(adjacentItem(projects, 'a', 's2', 1).destination, 's4')
  assert.equal(adjacentItem(projects, 'c', 's4', 1).destination, 'github')
  assert.equal(adjacentItem(projects, 'a', 'github', -1).destination, 's4')
  assert.equal(adjacentItem(projects, 'a', 'scratchpad', -1).destination, 'github')
  assert.equal(adjacentItem(projects, 'a', 'empty', 1).destination, 'github')
  assert.equal(adjacentItem(projects, 'a', 'empty', -1).destination, 's4')
  assert.equal(adjacentItem([], 'a', 'empty', 1), null)
})
test('the Scratchpad sits at its saved position among sessions, below Issues & PRs', () => {
  const p = {
    id: 'a',
    isExpanded: true,
    repository: 'o/a',
    hasScratchpad: true,
    scratchpadIndex: 1,
    sessions: [{ id: 's1' }, { id: 's2' }],
  }
  assert.deepEqual(
    sidebarItems([p]).map((i) => i.destination),
    ['github', 's1', 'scratchpad', 's2'],
  )
  assert.equal(adjacentItem([p], 'a', 's1', 1).destination, 'scratchpad')
  assert.equal(adjacentItem([p], 'a', 'scratchpad', 1).destination, 's2')
  assert.deepEqual(
    projectRows({ ...p, scratchpadIndex: 9 }).map((r) => r.id),
    ['s1', 's2', 'scratchpad'],
  )
  assert.deepEqual(
    projectRows({ ...p, scratchpadIndex: undefined }).map((r) => r.id),
    ['scratchpad', 's1', 's2'],
  )
  assert.deepEqual(
    projectRows({ ...p, hasScratchpad: false }).map((r) => r.id),
    ['s1', 's2'],
  )
})
test('Arc Mode scopes the list and navigation to the active space without changing expansion', () => {
  const projects = [
    { id: 'a', isExpanded: false, hasScratchpad: true, sessions: [{ id: 's1' }] },
    { id: 'b', isExpanded: true, sessions: [] },
  ]
  assert.equal(visibleSidebarProjects(projects, 'a', false), projects)
  const active = visibleSidebarProjects(projects, 'a', true)
  assert.deepEqual(active, [projects[0]])
  const navigation = active.map((p) => ({ ...p, isExpanded: true }))
  assert.equal(adjacentItem(navigation, 'a', 's1', 1).destination, 'scratchpad')
  assert.equal(adjacentItem(navigation, 'a', 'scratchpad', -1).destination, 's1')
  assert.equal(projects[0].isExpanded, false)
  assert.deepEqual(visibleSidebarProjects(projects, 'b', true), [projects[1]])
  assert.equal(adjacentItem([projects[1]], 'b', 'empty', 1), null)
  assert.deepEqual(visibleSidebarProjects(projects, 'missing', true), [projects[0]])
  assert.deepEqual(visibleSidebarProjects([], 'missing', true), [])
})
test('status bar summarizes running sessions and inbox requests', () => {
  const projects = [
    {
      sessions: [
        { running: true, activity: 'working' },
        { running: true, activity: 'waiting' },
        { running: false, activity: 'waiting' },
      ],
    },
    { sessions: [{ running: true, activity: 'unknown' }] },
    {},
  ]
  const runs = [
    { status: 'queued' },
    { status: 'queued' },
    { status: 'needsAttention' },
    { status: 'running' },
    { status: 'completed' },
  ]
  assert.deepEqual(statusSummary(projects, runs), { running: 3, working: 1, waiting: 1, queued: 2, attention: 1 })
  assert.deepEqual(statusSummary(undefined, undefined), { running: 0, working: 0, waiting: 0, queued: 0, attention: 0 })
  assert.equal(activityLabel('waiting'), 'Waiting for input')
  assert.equal(activityLabel('working'), 'Agent working')
  assert.equal(activityLabel(''), 'Status unavailable')
})
test('drop positions map to final sidebar indices', () => {
  const ids = ['a', 'b', 'c', 'd']
  assert.equal(dropIndex(ids, 'a', 'c', false), 1)
  assert.equal(dropIndex(ids, 'a', 'c', true), 2)
  assert.equal(dropIndex(ids, 'd', 'a', false), 0)
  assert.equal(dropIndex(ids, 'd', 'b', true), 2)
  assert.equal(dropIndex(ids, 'b', 'c', false), -1)
  assert.equal(dropIndex(ids, 'b', 'a', true), -1)
  assert.equal(dropIndex(ids, 'b', 'b', true), -1)
  assert.equal(dropIndex(ids, 'x', 'b', true), -1)
})

test('Arc wheel routing preserves vertical scroll and pinch zoom', () => {
  const wheel = { deltaX: 12, deltaY: 2, deltaMode: 0 }
  assert.equal(arcScrollDelta(wheel, 280), 12)
  assert.equal(arcScrollDelta({ ...wheel, deltaX: -12 }, 280), -12)
  assert.equal(arcScrollDelta({ ...wheel, deltaY: 24 }, 280), 0)
  assert.equal(arcScrollDelta({ ...wheel, ctrlKey: true }, 280), 0)
  assert.equal(arcScrollDelta({ ...wheel, deltaX: 0, deltaY: 4, shiftKey: true }, 280), 4)
  assert.equal(arcScrollDelta({ ...wheel, deltaMode: 1 }, 280), 192)
  assert.equal(arcScrollDelta({ ...wheel, deltaX: 1, deltaY: 0, deltaMode: 2 }, 280), 280)
})

test('custom themes are listed after built-ins and fall back to System when removed', async () => {
  const { setCustomThemes, themeOptions, missingCustomTheme, themeSource } = await import('../src/shared/appearance.js')
  const { commandItems } = await import('../src/shared/commands.js')
  const custom = {
    id: 'custom:dracula',
    name: 'My Dracula',
    light: ['#FFFFFF', '#F0F0F0', '#FAFAFA', '#111111', '#555555', '#DDDDDD', '#AA00AA', '#EEDDEE'],
    dark: ['#101010', '#141414', '#181818', '#F0F0F0', '#A0A0A0', '#303030', '#BD93F9', '#44475A'],
  }
  try {
    setCustomThemes([custom])
    const options = themeOptions()
    assert.equal(options.length, 31)
    assert.deepEqual(options.at(-1), { id: 'custom:dracula', name: 'My Dracula', custom: true })
    // A custom folder named like a built-in theme never replaces it.
    assert.equal(palette({ theme: 'dracula', appearance: 'dark' }).canvas, themes.dracula.dark[0])
    assert.equal(palette({ theme: 'custom:dracula', appearance: 'dark' }).accent, '#BD93F9')
    assert.equal(palette({ theme: 'custom:dracula', appearance: 'light' }).text, '#111111')
    // Without `ansi`, the default terminal colors apply; canvas and text follow the theme.
    const terminal = terminalTheme({ theme: 'custom:dracula', appearance: 'dark' })
    assert.equal(terminal.background, '#101010')
    assert.equal(terminal.red, terminalTheme({ theme: 'system', appearance: 'dark' }).red)
    setCustomThemes([{ ...custom, ansi: Array.from({ length: 16 }, (_, i) => '#0000' + String(i).padStart(2, '0')) }])
    assert.equal(terminalTheme({ theme: 'custom:dracula', appearance: 'dark' }).red, '#000001')
    const themeCommand = commandItems({}).find((i) => i.id === 'theme')
    assert.equal(themeCommand.children.at(-1).detail, 'Custom')
    assert.equal(missingCustomTheme('custom:dracula'), false)
    // Duplicating copies the colors in use; a theme without `ansi` gets the default terminal colors.
    assert.equal(themeSource('custom:dracula').ansi.length, 16)
    assert.equal(themeSource('custom:dracula').name, 'My Dracula')
    assert.deepEqual(themeSource('og').light, themes.og.light)
    assert.equal(themeSource('og').ansi[1], terminalTheme({ theme: 'og', appearance: 'dark' }).red)
    assert.deepEqual(themeSource('ayu').ansi, themes.ayu.ansi)

    setCustomThemes(null)
    assert.equal(themeOptions().length, 30)
    assert.equal(missingCustomTheme('custom:dracula'), true)
    assert.equal(missingCustomTheme('dracula'), false)
    assert.deepEqual(
      palette({ theme: 'custom:dracula', appearance: 'dark' }),
      palette({ theme: 'system', appearance: 'dark' }),
    )
  } finally {
    setCustomThemes([])
  }
})
test('a translucent terminal keeps theme text and an opaque cursor accent', () => {
  for (const appearance of ['dark', 'light']) {
    const prefs = { theme: 'tokyo-night', appearance },
      opaque = terminalTheme(prefs),
      translucent = terminalTheme(prefs, true)
    assert.equal(translucent.background, '#00000000')
    assert.equal(translucent.cursorAccent, palette(prefs).canvas)
    assert.deepEqual({ ...translucent, background: opaque.background }, opaque)
  }
})
test('window opacity keeps the default look, the sidebar ratio and the readable range', () => {
  assert.deepEqual(translucencyTokens(78), { '--canvas-opacity': '78%', '--sidebar-opacity': '62%' })
  assert.deepEqual(translucencyTokens(undefined), translucencyTokens(78))
  assert.deepEqual(translucencyTokens(10), { '--canvas-opacity': '40%', '--sidebar-opacity': '32%' })
  assert.deepEqual(translucencyTokens(100), { '--canvas-opacity': '95%', '--sidebar-opacity': '76%' })
})
