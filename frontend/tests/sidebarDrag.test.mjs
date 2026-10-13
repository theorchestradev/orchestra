import test from 'node:test'
import assert from 'node:assert/strict'
import { sidebarDragLayout } from '../src/shared/sidebarDrag.js'

test('whole sidebar items open space in either direction, including unequal heights', () => {
  const rows = [
    { top: 0, bottom: 34 },
    { top: 38, bottom: 106 },
    { top: 110, bottom: 160 },
  ]
  assert.deepEqual(sidebarDragLayout(rows, 0, 80), { y: 80, index: 1, offsets: [0, -38, 0], settleY: 72 })
  assert.deepEqual(sidebarDragLayout(rows, 2, -130), { y: -110, index: 0, offsets: [54, 54, 0], settleY: -110 })
  assert.deepEqual(sidebarDragLayout(rows, 0, 999), { y: 126, index: 2, offsets: [0, -38, -38], settleY: 126 })
  assert.equal(sidebarDragLayout(rows, 1, 3).index, 1)
  assert.deepEqual(sidebarDragLayout([{ top: 20, bottom: 60 }], 0, 50), { y: 0, index: 0, offsets: [0], settleY: 0 })
})
