import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { shortcuts, shortcutKeys, appCommand, terminalKeeps } from '../src/shared/shortcuts.js'
import { commandItems } from '../src/shared/commands.js'

const shifted = { '[': '{', ']': '}', ',': '<' }
const codes = { '[': 'BracketLeft', ']': 'BracketRight', ',': 'Comma', '←': 'ArrowLeft', '→': 'ArrowRight' }
// A keydown event for a shortcut in macOS notation, pressed with ⌘ on macOS and Ctrl elsewhere.
function press(text, isMac) {
  const base = text.replace(/[⇧⌘]/g, '').toLowerCase()
  const shiftKey = text.includes('⇧')
  return {
    key: codes[base]?.startsWith('Arrow') ? codes[base] : shiftKey ? shifted[base] || base.toUpperCase() : base,
    code: codes[base] || 'Key' + base.toUpperCase(),
    shiftKey,
    metaKey: isMac,
    ctrlKey: !isMac,
  }
}
const app = shortcuts.filter((s) => s.app)
const keys = [...'abcdefghijklmnopqrstuvwxyz', '[', ']', ',', '←', '→']
const combos = keys.flatMap((k) => ['⌘' + k.toUpperCase(), '⇧⌘' + k.toUpperCase()])

test('every listed app shortcut is handled, in and out of terminals', () => {
  for (const isMac of [true, false])
    for (const entry of app) {
      assert.equal(appCommand(press(entry.keys, isMac)), entry.id, entry.id)
      if (!isMac && entry.terminal === null) {
        assert.equal(terminalKeeps(press(entry.keys, false), false), true, entry.id + ' reaches the terminal')
        continue
      }
      const inTerminal = press(isMac ? entry.keys : entry.terminal || entry.keys, isMac)
      assert.equal(terminalKeeps(inTerminal, isMac), false, entry.id + ' leaves a terminal')
      assert.equal(appCommand(inTerminal), entry.id, entry.id + ' in a terminal')
    }
})

test('every handled app shortcut is listed', () => {
  for (const isMac of [true, false])
    for (const combo of combos) {
      const id = appCommand(press(combo, isMac))
      // Ctrl+Shift forms for terminals also work outside them, and with ⌘ on macOS.
      const entry = app.find((s) => s.id === id)
      if (id) assert.ok([entry?.keys, entry?.terminal].includes(combo), combo + ' runs ' + id)
      if (!terminalKeeps(press(combo, isMac), isMac)) assert.ok(id, combo + ' leaves a terminal but runs nothing')
    }
})

test('the macOS menu uses the listed shortcuts', () => {
  const main = readFileSync(new URL('../../main.go', import.meta.url), 'utf8')
  const menu = [
    ...main.matchAll(
      /keys\.(?:CmdOrCtrl\("(.)"\)|Combo\("(.)", keys\.CmdOrCtrlKey, keys\.ShiftKey\)).*"command", "(\w+)"/g,
    ),
  ].map(([, plain, shift, id]) => [id, plain ? '⌘' + plain.toUpperCase() : '⇧⌘' + shift.toUpperCase()])
  assert.deepEqual(
    menu.sort(),
    app
      .filter((s) => s.menu !== false)
      .map((s) => [s.id, s.keys])
      .sort(),
  )
  for (const entry of app.filter((s) => s.menu === false))
    assert.match(main, new RegExp(`nil, func.*"command", "${entry.id}"`), entry.id + ' has a menu item')
})

test('project shortcuts leave text fields alone', () => {
  const field = (inTerminal) => ({ closest: (selector) => (selector === '.xterm' ? inTerminal : true) })
  const event = (target) => ({ ...press('⇧⌘→', true), target })
  assert.equal(appCommand(event(field(false))), null)
  assert.equal(appCommand(event(field(true))), 'nextProject')
  assert.equal(appCommand(event({ closest: () => null })), 'nextProject')
})

test('the command bar shows the listed shortcuts', () => {
  const project = { id: 'p', name: 'P', sessions: [] }
  for (const item of commandItems({ projects: [project], project, preferences: {} }))
    if (item.shortcut) assert.equal(item.shortcut, shortcuts.find((s) => s.id === item.id)?.keys, item.id)
})

test('shortcut keys follow the platform', () => {
  const entry = (id) => shortcuts.find((s) => s.id === id)
  const labels = (id, isMac) => shortcutKeys(entry(id), isMac).map((k) => [k.label, k.name, k.note])
  assert.deepEqual(labels('command', true), [['⌘K', 'Command K', '']])
  assert.deepEqual(labels('command', false), [
    ['Ctrl+K', 'Control K', ''],
    ['Ctrl+Shift+K', 'Control Shift K', 'In a terminal'],
  ])
  assert.deepEqual(labels('session', false), [['Ctrl+N', 'Control N', 'Outside terminals']])
  assert.deepEqual(labels('project', false), [['Ctrl+Shift+N', 'Control Shift N', '']])
  assert.deepEqual(labels('copy', false), [['Ctrl+Shift+C', 'Control Shift C', '']])
  assert.deepEqual(labels('previous', true), [['⇧⌘[', 'Shift Command Left Bracket', '']])
  assert.deepEqual(labels('nextProject', true), [['⇧⌘→', 'Shift Command Right Arrow', '']])
  assert.deepEqual(labels('previousProject', false), [['Ctrl+Shift+←', 'Control Shift Left Arrow', '']])
  assert.deepEqual(labels('saveDescription', true), [['⌘↩', 'Command Return', '']])
  assert.deepEqual(labels('saveDescription', false), [['Ctrl+Enter', 'Control Enter', '']])
  assert.deepEqual(labels('contextMenu', false), [['Shift+F10', 'Shift F10', '']])
  assert.deepEqual(labels('dismiss', false), [['Esc', 'Escape', '']])
})
