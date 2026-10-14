import test from 'node:test'
import assert from 'node:assert/strict'
import { shortcutLabel } from '../src/shared/platform.js'
test('shortcut labels use Ctrl names outside macOS', () => {
  assert.equal(shortcutLabel('⇧⌘B', true), '⇧⌘B')
  assert.equal(shortcutLabel('⌘N', false), 'Ctrl+N')
  assert.equal(shortcutLabel('⇧⌘N', false), 'Ctrl+Shift+N')
  assert.equal(shortcutLabel('⌘,', false), 'Ctrl+,')
  assert.equal(shortcutLabel('⌘↩', false), 'Ctrl+Enter')
  assert.equal(shortcutLabel('Esc', false), 'Esc')
})
