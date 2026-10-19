import test from 'node:test'
import assert from 'node:assert/strict'
import {
  resetLabel,
  remainingLabel,
  moneyLabel,
  countLabel,
  durationLabel,
  usedPercent,
  usageLevel,
  constrainedWindow,
  usageSummary,
} from '../src/shared/usage.js'
test('usage reset labels distinguish unknown, expired and upcoming windows', () => {
  const now = 1800000000000
  assert.equal(resetLabel(0, now), 'Reset time unavailable')
  assert.equal(resetLabel(now / 1000, now), 'Reset time passed · refresh to update')
  assert.equal(resetLabel(now / 1000 + 61, now), 'Resets in 2m')
  assert.equal(resetLabel(now / 1000 + 9000, now), 'Resets in 2h 30m')
  assert.equal(resetLabel(now / 1000 + 6 * 86400 + 10 * 3600, now), 'Resets in 6d 10h')
  assert.equal(remainingLabel(0.4), '0.4%')
  assert.equal(remainingLabel(97), '97%')
})
test('OpenRouter totals show USD, zero, negative and unavailable balances', () => {
  assert.equal(moneyLabel(74.75), '$74.75')
  assert.equal(moneyLabel(0), '$0.00')
  assert.equal(moneyLabel(-1), '-$1.00')
  assert.equal(moneyLabel(null), 'Unavailable')
  assert.equal(moneyLabel(0.000014, 6), '$0.000014')
  assert.equal(countLabel(1550700), '1,550,700')
})
test('status bar durations are compact and empty once unknown or passed', () => {
  const now = 1800000000000
  assert.equal(durationLabel(0, now), '')
  assert.equal(durationLabel(now / 1000, now), '')
  assert.equal(durationLabel(now / 1000 + 4 * 3600 + 25 * 60, now), '4h 25m')
  assert.equal(durationLabel(now / 1000 + 3600, now), '1h')
})
test('used percent and warning levels', () => {
  assert.equal(usedPercent({ remaining: 93 }), 7)
  assert.equal(usedPercent({ remaining: 120 }), 0)
  assert.equal(usageLevel(79.9), '')
  assert.equal(usageLevel(80), 'warning')
  assert.equal(usageLevel(95), 'critical')
})
test('the status bar summarizes the session agent, or else the most constrained agent', () => {
  const now = 1800000000000,
    later = now / 1000 + 3600
  const codex = { agent: 'codex', windows: [{ label: 'Weekly', remaining: 40, resetsAt: later }] }
  const claude = {
    agent: 'claude',
    windows: [
      { label: 'Session · 5h', remaining: 90, resetsAt: later },
      { label: 'Weekly', remaining: 70 },
      { label: 'Expired', remaining: 1, resetsAt: now / 1000 - 60 },
    ],
  }
  assert.equal(constrainedWindow(claude, now).label, 'Weekly')
  assert.equal(constrainedWindow({ agent: 'codex', windows: [] }, now), null)
  assert.equal(usageSummary([codex, claude], 'claude', now).agent, 'claude')
  assert.equal(usageSummary([codex, claude], 'pi', now).agent, 'codex')
  assert.equal(
    usageSummary([codex, { agent: 'claude', windows: [], message: 'Sign in' }], 'claude', now).agent,
    'codex',
  )
  assert.equal(usageSummary([], 'claude', now), null)
})
