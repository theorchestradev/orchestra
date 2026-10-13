import test from 'node:test'
test('privacy documentation matches every event, property and allowed value', async () => {
  await import('../../scripts/TelemetryDocs.mjs')
})
