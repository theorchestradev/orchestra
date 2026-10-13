import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { arcScrollDelta } from '../src/shared/appearance.js'

test('Arc scroll follows the gesture to one neighbor and settles through Store selection', async () => {
  const source = readFileSync(new URL('../src/components/ArcProjectPager.vue', import.meta.url), 'utf8')
    .match(/<script setup>([\s\S]*?)<\/script>/)[1]
    .replace(/^import .*$/gm, '')
  const requests = [],
    timers = new Map(),
    watchers = []
  let timerID = 0,
    cleanup,
    now = 0,
    hit = 'sidebar',
    listener,
    resolveSelection
  const props = {
    items: [{ id: 'a' }, { id: 'b' }, { id: 'c' }, { id: 'd' }],
    selectedId: 'a',
    selectProject: (p) => {
      requests.push(p.id)
      return new Promise((resolve) => {
        resolveSelection = resolve
      })
    },
  }
  const el = { clientWidth: 280, scrollLeft: 0, contains: (node) => node === 'sidebar' }
  const pager = new Function(
    'ref',
    'computed',
    'watch',
    'onMounted',
    'onBeforeUnmount',
    'defineOptions',
    'defineProps',
    'setTimeout',
    'clearTimeout',
    'arcScrollDelta',
    'window',
    'document',
    source + '\nreturn {viewport,wheel,activeIndex,visualIndex,gesture,createProgress,settingsProgress}',
  )(
    (value) => ({ value }),
    (fn) => ({
      get value() {
        return fn()
      },
    }),
    (_fn, callback) => watchers.push(callback),
    (fn) => fn(),
    (fn) => {
      cleanup = fn
    },
    () => {},
    () => props,
    (callback, delay) => {
      timers.set(++timerID, { callback, at: now + delay })
      return timerID
    },
    (id) => timers.delete(id),
    arcScrollDelta,
    {
      addEventListener: (type, fn, options) => {
        assert.equal(type, 'wheel')
        assert.deepEqual(options, { capture: true, passive: false })
        listener = fn
      },
      removeEventListener: (type, fn, capture) => {
        assert.equal(type, 'wheel')
        assert.equal(fn, listener)
        assert.equal(capture, true)
        listener = null
      },
    },
    { elementFromPoint: () => hit },
  )
  pager.viewport.value = el
  const wheel = (delta) => pager.wheel({ deltaX: delta, deltaY: 0, deltaMode: 0, preventDefault() {} })
  const tick = (duration) => {
    now += duration
    for (const [id, timer] of [...timers])
      if (timer.at <= now) {
        timers.delete(id)
        timer.callback()
      }
  }
  const idle = () => tick(150)
  const acknowledge = async (success = true) => {
    if (success) {
      props.selectedId = requests.at(-1)
      watchers[0]()
    }
    resolveSelection(success ? props : null)
    await Promise.resolve()
  }
  wheel(10)
  wheel(20)
  wheel(15)
  assert.deepEqual(requests, [])
  assert.ok(Math.abs(pager.visualIndex.value - 45 / 280) < 1e-9)
  idle()
  assert.deepEqual(requests, ['b'])
  assert.equal(pager.visualIndex.value, 1)
  assert.equal(pager.activeIndex.value, 0)
  await acknowledge()
  assert.equal(pager.gesture.value, null)
  assert.equal(pager.visualIndex.value, 1)
  // Long input moves progressively, reaches the neighbor and cannot skip to a third page.
  const stationaryWheel = {
    clientX: 100,
    clientY: 180,
    target: 'outgoing-page',
    deltaX: 90,
    deltaY: 0,
    deltaMode: 0,
    preventDefault() {},
  }
  listener(stationaryWheel)
  assert.equal(pager.visualIndex.value, 1 + 90 / 280)
  for (let i = 0; i < 20; i++) {
    tick(60)
    listener(stationaryWheel)
  }
  assert.deepEqual(requests, ['b', 'c'])
  assert.equal(pager.visualIndex.value, 2)
  await acknowledge()
  tick(140)
  listener({ ...stationaryWheel, deltaX: 2 })
  assert.deepEqual(requests, ['b', 'c'])
  idle()
  // Another gesture at the same coordinates can return to the previous space.
  listener({ ...stationaryWheel, deltaX: -45 })
  idle()
  assert.deepEqual(requests, ['b', 'c', 'b'])
  await acknowledge()
  assert.equal(pager.visualIndex.value, 1)
  wheel(10)
  idle()
  assert.deepEqual(requests, ['b', 'c', 'b'])
  assert.equal(pager.visualIndex.value, 1)
  // A failed bridge request must restore the Store-selected page and footer alignment.
  wheel(45)
  idle()
  assert.deepEqual(requests, ['b', 'c', 'b', 'c'])
  await acknowledge(false)
  assert.equal(pager.visualIndex.value, 1)
  // Footer navigation, reordering and resizing cancel or align an unfinished pan.
  wheel(10)
  props.selectedId = 'd'
  watchers[0]()
  assert.equal(pager.visualIndex.value, 3)
  idle()
  props.items.reverse()
  watchers[1]()
  assert.equal(pager.visualIndex.value, 0)
  el.clientWidth = 400
  wheel(-90)
  assert.equal(pager.visualIndex.value, 0)
  idle()
  wheel(100)
  assert.equal(pager.visualIndex.value, 0.25)
  props.selectedId = 'a'
  watchers[0]()
  idle()
  wheel(100)
  assert.equal(pager.visualIndex.value, 3)
  idle()
  pager.wheel({
    deltaX: 0,
    deltaY: 60,
    deltaMode: 0,
    preventDefault() {
      assert.fail('vertical scroll intercepted')
    },
  })
  for (const outside of ['overlay', 'main-content']) {
    hit = outside
    listener({
      ...stationaryWheel,
      preventDefault() {
        assert.fail('outside scroll intercepted')
      },
    })
  }
  // With New Project enabled, panning past the last space fills the ring and creates only when full.
  let created = 0
  props.createProject = () => created++
  props.selectedId = 'a'
  watchers[0]()
  assert.equal(pager.activeIndex.value, 3)
  wheel(200)
  assert.equal(pager.createProgress.value, 0.5)
  assert.equal(pager.visualIndex.value, 3.5)
  idle()
  assert.equal(created, 0)
  assert.equal(pager.visualIndex.value, 3)
  wheel(250)
  wheel(250)
  assert.equal(created, 1)
  assert.equal(pager.gesture.value, null)
  assert.equal(pager.createProgress.value, 0)
  // The rest of the same gesture cannot open another New Project.
  wheel(400)
  wheel(400)
  assert.equal(created, 1)
  idle()
  wheel(-45)
  assert.equal(pager.createProgress.value, 0)
  idle()
  assert.deepEqual(requests.slice(-1), ['b'])
  await acknowledge()
  props.createProject = undefined
  // Panning before the first space fills the Settings ring and opens Settings only when full.
  let settings = 0
  props.openSettings = () => settings++
  props.selectedId = 'd'
  watchers[0]()
  wheel(-200)
  assert.equal(pager.settingsProgress.value, 0.5)
  assert.equal(pager.createProgress.value, 0)
  idle()
  assert.equal(settings, 0)
  assert.equal(pager.visualIndex.value, 0)
  wheel(-400)
  assert.equal(settings, 1)
  assert.equal(pager.gesture.value, null)
  wheel(-400)
  assert.equal(settings, 1)
  idle()
  props.openSettings = undefined
  // Momentum after a switch is ignored, but the next pan responds without waiting for a quiet period.
  hit = 'sidebar'
  let time = 0
  const pan = (deltaX, gap = 16) => listener({ ...stationaryWheel, deltaX, timeStamp: (time += gap) })
  pan(200)
  pan(200)
  assert.deepEqual(requests.slice(-1), ['c'])
  for (const deltaX of [180, 150, 120]) pan(deltaX)
  await acknowledge()
  for (const deltaX of [90, 60, 30, 10]) pan(deltaX)
  assert.equal(pager.visualIndex.value, 1)
  pan(20, 60)
  assert.equal(pager.visualIndex.value, 1 + 20 / 400)
  pan(380)
  assert.deepEqual(requests.slice(-1), ['b'])
  // A reversed pan during a slow Store reply stays unlocked after the reply.
  pan(-100)
  await acknowledge()
  pan(-100)
  assert.equal(pager.visualIndex.value, 2 - 100 / 400)
  idle()
  assert.deepEqual(requests.slice(-1), ['c'])
  await acknowledge()
  wheel(-45)
  cleanup()
  assert.equal(timers.size, 0)
  assert.equal(listener, null)
})
