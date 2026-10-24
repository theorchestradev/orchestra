import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { ref, computed, reactive, watch, nextTick, markRaw } from 'vue'
import * as Vue from 'vue'
import { compile } from '@vue/compiler-dom'
import { renderToString } from '@vue/server-renderer'
import { spaceSymbol } from '../src/shared/project.js'

test('Arc spaces prefer emoji, then the project favicon, then the initial, and keep compact dots', async () => {
  const template = readFileSync(new URL('../src/components/ArcSpaces.vue', import.meta.url), 'utf8').match(
    /<template>([\s\S]*?)<\/template>/,
  )[1]
  const render = new Function('Vue', compile(template, { prefixIdentifiers: true }).code)(Vue)
  const state = {
    projects: [{ id: 'project', name: 'Orchestra', emoji: '🎵' }],
    projectIcons: { project: 'data:image/png;base64,icon' },
    selectedId: 'project',
    settingsOpen: false,
    compact: false,
    spaceSymbol,
    focusSpace() {},
  }
  const html = () =>
    renderToString(Vue.createSSRApp({ render, setup: () => state, components: { Icon: { render: () => null } } }))
  assert.match(await html(), /🎵/)
  assert.doesNotMatch(await html(), /<img/)
  state.projects[0].emoji = ''
  assert.match(await html(), /<img class="project-icon" src="data:image\/png;base64,icon" alt(?:="")?>/)
  state.projectIcons = {}
  assert.match(await html(), /aria-hidden="true">O<\/span>/)
  state.projectIcons = { project: 'data:image/png;base64,icon' }
  state.compact = true
  assert.match(await html(), /arc-space-dot/)
  assert.doesNotMatch(await html(), /<img/)
})

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
