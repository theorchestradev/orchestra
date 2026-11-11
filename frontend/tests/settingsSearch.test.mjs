import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { searchSettings, settingsSearchItems } from '../src/shared/settingsSearch.js'

test('Escape clears a Settings search and bubbles to Back when the search is empty', () => {
  const source = readFileSync(new URL('../src/components/Settings.vue', import.meta.url), 'utf8')
  const body = source.match(/function escapeSearch\(event\) \{([\s\S]*?)\n\}/)[1]
  const search = { value: 'font' }
  const escapeSearch = new Function('search', 'event', body)
  let stopped = false
  const event = { stopPropagation: () => (stopped = true) }
  escapeSearch(search, event)
  assert.equal(search.value, '')
  assert.equal(stopped, true)
  stopped = false
  escapeSearch(search, event)
  assert.equal(stopped, false)
  assert.match(source, /@keydown\.esc="escapeSearch"/)
})

test('settings search finds controls across sections and combines case-insensitive words', () => {
  assert.equal(searchSettings('  FONT size  ')[0].target, 'font-size')
  assert.equal(searchSettings('running quitting')[0].section, 'General')
  assert.equal(searchSettings('terminal padding').length, 2)
  assert.ok(searchSettings('toggle sidebar').some((item) => item.section === 'Shortcuts'))
  assert.ok(searchSettings('waiting sessions').some((item) => item.target === 'status-item-waiting'))
  assert.deepEqual(searchSettings('does not exist'), [])
  assert.deepEqual(searchSettings('   '), [])
})

test('every labeled Settings control is searchable at its actual destination', () => {
  const source = readFileSync(new URL('../src/components/Settings.vue', import.meta.url), 'utf8').split('<Teleport')[0]
  for (const [, target, label] of source.matchAll(/<label for="([^"]+)">([^<{]+)<\/label\s*>/g)) {
    assert.ok(
      settingsSearchItems.some((item) => item.target === target),
      `${label} is missing from search`,
    )
    assert.ok(
      searchSettings(label).some((item) => item.target === target),
      `${label} is not searchable`,
    )
  }
})
