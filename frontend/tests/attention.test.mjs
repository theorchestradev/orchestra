import test from 'node:test'
import assert from 'node:assert/strict'
import { sessionsNeedingYou, attentionSound } from '../src/shared/attention.js'

const workspace = (...sessions) => [{ sessions }]
const ids = (sessions) => sessions.map((s) => s.id)

test('only a working session that starts waiting needs you', () => {
  const before = workspace(
    { id: 'a', agent: 'claude', activity: 'working' },
    { id: 'b', agent: 'codex', activity: 'unknown' },
    { id: 'c', agent: 'terminal', activity: 'working' },
    { id: 'd', agent: 'claude', activity: 'waiting' },
  )
  const after = workspace(
    { id: 'a', agent: 'claude', activity: 'waiting' },
    { id: 'b', agent: 'codex', activity: 'waiting' },
    { id: 'c', agent: 'terminal', activity: 'waiting' },
    { id: 'd', agent: 'claude', activity: 'waiting' },
    { id: 'e', agent: 'claude', activity: 'waiting' },
  )
  assert.deepEqual(ids(sessionsNeedingYou(before, after)), ['a'])
  assert.deepEqual(ids(sessionsNeedingYou([], after)), [])
  assert.deepEqual(ids(sessionsNeedingYou(before, after, (s) => s.id === 'a')), [])
})

test('sessions stopping within a second play one sound', () => {
  let now = 0,
    plays = 0
  const chime = attentionSound(
    () => plays++,
    () => now,
  )
  chime()
  now = 400
  chime()
  now = 999
  chime()
  assert.equal(plays, 1)
  now = 1500
  chime()
  assert.equal(plays, 2)
})
