import test from 'node:test'
import assert from 'node:assert/strict'
import { pendingUpdate } from '../src/shared/update.js'

test('the status bar offers the newest release unless it was skipped', () => {
  const status = { current: '0.1.0', available: { version: '0.2.0' } }
  assert.equal(pendingUpdate(status, {}), status.available)
  assert.equal(pendingUpdate(status, { skippedUpdate: '0.2.0' }), null)
  assert.equal(
    pendingUpdate({ ...status, available: { version: '0.3.0' } }, { skippedUpdate: '0.2.0' }).version,
    '0.3.0',
  )
  assert.equal(pendingUpdate({ current: '0.1.0' }, {}), null)
  assert.equal(pendingUpdate(null, {}), null)
  assert.equal(pendingUpdate({ ...status, staged: '0.2.0' }, { skippedUpdate: '0.2.0' }), status.available)
})
