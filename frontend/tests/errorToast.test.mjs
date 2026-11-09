import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { effectScope, nextTick, ref, watch } from 'vue'

test('error toasts expire after five seconds, restart for new errors, and support manual dismissal', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const error = ref('')
  const appState = ref({ error: '' })
  const preferences = ref({})
  const source = readFileSync(new URL('../src/App.vue', import.meta.url), 'utf8')
  const watcher = source.slice(source.indexOf('watch(\n  [error,'), source.indexOf('// Custom themes are read again'))
  const scope = effectScope()
  scope.run(() =>
    new Function('watch', 'error', 'appState', 'preferences', watcher)(watch, error, appState, preferences),
  )
  try {
    error.value = 'First error'
    await nextTick()
    t.mock.timers.tick(4999)
    assert.equal(error.value, 'First error')
    appState.value.error = 'New error'
    await nextTick()
    t.mock.timers.tick(1)
    assert.equal(error.value, 'First error')
    t.mock.timers.tick(4999)
    assert.equal(error.value, '')
    assert.equal(appState.value.error, '')
    await nextTick()

    error.value = 'Manual error'
    await nextTick()
    preferences.value.manualErrorDismiss = true
    await nextTick()
    t.mock.timers.tick(10000)
    assert.equal(error.value, 'Manual error')
    preferences.value.manualErrorDismiss = false
    await nextTick()
    t.mock.timers.tick(5000)
    assert.equal(error.value, '')
    await nextTick()

    appState.value.error = 'Closed manually'
    await nextTick()
    appState.value.error = ''
    await nextTick()
    error.value = 'After closing'
    await nextTick()
    scope.stop()
    t.mock.timers.tick(5000)
    assert.equal(error.value, 'After closing')
  } finally {
    scope.stop()
  }
})
