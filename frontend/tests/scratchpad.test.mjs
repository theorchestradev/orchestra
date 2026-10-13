import test from 'node:test'
import assert from 'node:assert/strict'
import { reconcileScratchpad } from '../src/shared/scratchpad.js'

const base = { text: 'Notes', revision: 3 }

test('older or current revisions leave the editor alone', () => {
  assert.equal(reconcileScratchpad('Notes typed', base, { text: 'Old', revision: 3 }), null)
  assert.equal(reconcileScratchpad('Notes typed', base, { text: 'Old', revision: 2 }), null)
})

test('an editor without local edits shows the new text', () => {
  assert.deepEqual(reconcileScratchpad('Notes', base, { text: 'Replaced', revision: 4 }), {
    text: 'Replaced',
    base: { text: 'Replaced', revision: 4 },
    conflict: false,
  })
})

test('echoes of the editor’s own saves keep later typing', () => {
  const result = reconcileScratchpad('Notes and more', base, { text: 'Notes and', revision: 4 }, ['Notes and'])
  assert.deepEqual(result, { text: 'Notes and more', base: { text: 'Notes and', revision: 4 }, conflict: false })
})

test('appends merge after unsaved local edits', () => {
  const result = reconcileScratchpad('My Notes', base, { text: 'Notes\nAgent note', revision: 4 })
  assert.deepEqual(result, {
    text: 'My Notes\nAgent note',
    base: { text: 'Notes\nAgent note', revision: 4 },
    conflict: false,
  })
})

test('replacements keep local edits and report a conflict', () => {
  const result = reconcileScratchpad('My Notes', base, { text: 'Rewritten', revision: 5 })
  assert.deepEqual(result, { text: 'My Notes', base: { text: 'Rewritten', revision: 5 }, conflict: true })
})
