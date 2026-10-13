import test from 'node:test'
import assert from 'node:assert/strict'
import { resetLabel, remainingLabel, moneyLabel, countLabel } from '../src/shared/usage.js'
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
