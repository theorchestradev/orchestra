import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { ref, computed, reactive, watch, nextTick, markRaw } from 'vue'

test('project spaces compact when crowded, reveal selection and preserve keyboard navigation', async () => {
  const source = readFileSync(new URL('../src/components/ArcSpaces.vue', import.meta.url), 'utf8')
    .match(/<script setup>([\s\S]*?)<\/script>/)[1]
    .replace(/^import .*$/gm, '')
  const props = reactive({ projects: Array.from({ length: 4 }, (_, i) => ({ id: String(i) })), selectedId: '0' })
  let resize,
    mounted,
    cleanup,
    disconnected = false,
    reveals = 0,
    focused
  const buttons = props.projects.map((p) => ({
    focus: () => {
      focused = p.id
    },
  }))
  const nav = {
    querySelector: () => ({
      scrollIntoView: (options) => {
        assert.deepEqual(options, { block: 'nearest', inline: 'nearest' })
        reveals++
      },
    }),
    querySelectorAll: () => buttons,
  }
  const spaces = new Function(
    'ref',
    'computed',
    'watch',
    'nextTick',
    'onMounted',
    'onBeforeUnmount',
    'defineProps',
    'defineEmits',
    'ResizeObserver',
    'document',
    source + '\nreturn {spaces, compact, focusSpace}',
  )(
    ref,
    computed,
    watch,
    nextTick,
    (fn) => {
      mounted = fn
    },
    (fn) => {
      cleanup = fn
    },
    () => props,
    () => () => {},
    class {
      constructor(callback) {
        resize = callback
      }
      observe(el) {
        assert.equal(el, nav)
      }
      disconnect() {
        disconnected = true
      }
    },
    { activeElement: buttons[0] },
  )
  spaces.spaces.value = markRaw(nav)
  mounted()
  resize([{ contentRect: { width: 160 } }])
  await nextTick()
  assert.equal(spaces.compact.value, false)
  resize([{ contentRect: { width: 100 } }])
  await nextTick()
  assert.equal(spaces.compact.value, true)
  resize([{ contentRect: { width: 300 } }])
  props.projects = Array.from({ length: 8 }, (_, i) => ({ id: String(i) }))
  assert.equal(spaces.compact.value, false)
  props.projects.push({ id: '8' })
  assert.equal(spaces.compact.value, true)
  await nextTick()
  const previousReveals = reveals
  props.selectedId = '8'
  await nextTick()
  await nextTick()
  assert.ok(reveals > previousReveals)
  for (const [key, expected] of [
    ['End', '3'],
    ['Home', '0'],
    ['ArrowRight', '1'],
    ['ArrowLeft', '3'],
  ]) {
    let prevented = false
    spaces.focusSpace({
      key,
      currentTarget: nav,
      preventDefault() {
        prevented = true
      },
    })
    assert.equal(prevented, true)
    assert.equal(focused, expected)
  }
  cleanup()
  assert.equal(disconnected, true)
})
