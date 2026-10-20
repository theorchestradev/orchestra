import test from 'node:test'
import assert from 'node:assert/strict'
import { parsePatch } from '../src/shared/diff.js'
import { excerpt, outdated, rangeLabel, sessionReviews, linesInRange } from '../src/shared/review.js'

const hunks = parsePatch('@@ -1,4 +1,4 @@\n one\n-two\n-three\n+2\n+3\n four\n')

test('ranges select lines on one side only', () => {
  assert.deepEqual(
    linesInRange(hunks, 'new', 1, 3).map((l) => l.text),
    ['one', '2', '3'],
  )
  assert.equal(excerpt(hunks, 'old', 2, 3), 'two\nthree')
  assert.equal(excerpt(hunks, 'new', 9, 9), '')
})

test('comments become outdated when their lines change or leave the diff', () => {
  const comment = { side: 'new', start: 2, end: 3, excerpt: '2\n3' }
  assert.equal(outdated(comment, { hunks }), false)
  assert.equal(outdated(comment, { hunks: parsePatch('@@ -1,2 +1,2 @@\n one\n-two\n+TWO\n') }), true)
  assert.equal(outdated(comment, undefined), true)
})

test('long excerpts stop at a character boundary', () => {
  const long = parsePatch('@@ -0,0 +1 @@\n+' + 'é'.repeat(3000) + '\n')
  const text = excerpt(long, 'new', 1, 1)
  assert.ok(new TextEncoder().encode(text).length <= 4000)
  assert.ok(!text.includes('�'))
})

test('labels and listed reviews', () => {
  assert.equal(rangeLabel({ side: 'new', start: 4, end: 4 }), 'Line 4')
  assert.equal(rangeLabel({ side: 'old', start: 2, end: 5 }), 'Lines 2–5 (removed)')
  const runs = [
    { id: 'a', kind: 'review', toSessionID: 's', status: 'completed', comments: [] },
    { id: 'b', kind: 'review', toSessionID: 's', status: 'cancelled', comments: [{}] },
    { id: 'c', kind: 'review', toSessionID: 's', status: 'running', dismissed: true },
    { id: 'd', toSessionID: 's', status: 'queued' },
    { id: 'e', kind: 'review', toSessionID: 's', status: 'cancelled', comments: [{ reply: 'ok' }] },
  ]
  assert.deepEqual(
    sessionReviews(runs, 's').map((r) => r.id),
    ['e', 'a'],
  )
})
