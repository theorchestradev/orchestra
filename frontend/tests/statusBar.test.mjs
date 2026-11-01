import test from 'node:test'
import assert from 'node:assert/strict'
import {
  statusBarGroups,
  statusBarItemVisible,
  statusBarItemChange,
  statusBarLayout,
  moveStatusBarItem,
} from '../src/shared/statusBar.js'

test('existing preferences show all indicators and changes preserve other settings', () => {
  for (const { items } of statusBarGroups) {
    for (const [id] of items) {
      assert.equal(statusBarItemVisible({}, id), true)
      const hidden = statusBarItemChange({ theme: 'ayu' }, id)
      assert.equal(statusBarItemVisible(hidden, id), false)
      assert.equal(hidden.theme, 'ayu')
      const restored = statusBarItemChange(hidden, id)
      assert.equal(statusBarItemVisible(restored, id), true)
      assert.deepEqual(restored.hiddenStatusBarItems, [])
    }
  }
})

test('legacy usage preference can be toggled without hiding other indicators', () => {
  const preferences = { hideUsageStatus: true, hiddenStatusBarItems: ['branch'] }
  assert.equal(statusBarItemVisible(preferences, 'usage'), false)
  const shown = statusBarItemChange(preferences, 'usage')
  assert.equal(statusBarItemVisible(shown, 'usage'), true)
  assert.equal(statusBarItemVisible(shown, 'branch'), false)
  assert.equal(statusBarItemVisible(preferences, 'workspace'), true)
})

test('items move between sides and reorder without affecting visibility', () => {
  const original = { hiddenStatusBarItems: ['inbox'] }
  const moved = moveStatusBarItem(original, 'inbox', 0, 0)
  assert.equal(statusBarLayout(moved)[0].items[0][0], 'inbox')
  assert.equal(
    statusBarLayout(moved)[1].items.some(([id]) => id === 'inbox'),
    false,
  )
  assert.equal(statusBarItemVisible(moved, 'inbox'), false)
  const reordered = moveStatusBarItem(moved, 'inbox', 0, 2)
  assert.deepEqual(
    statusBarLayout(reordered)[0]
      .items.slice(0, 3)
      .map(([id]) => id),
    ['workspace', 'repository', 'inbox'],
  )
  const allRight = { statusBarLayout: [[], statusBarGroups.flatMap((g) => g.items.map(([id]) => id))] }
  assert.deepEqual(statusBarLayout(allRight)[0].items, [])
  assert.equal(statusBarLayout(allRight)[1].items.length, 14)
  assert.equal(original.statusBarLayout, undefined)
})

test('saved layouts ignore unknown and duplicate IDs and restore missing indicators', () => {
  const layout = statusBarLayout({ statusBarLayout: [['inbox', 'inbox', 'unknown'], []] })
  const ids = layout.flatMap((g) => g.items.map(([id]) => id))
  assert.equal(ids.length, 14)
  assert.equal(new Set(ids).size, 14)
  assert.equal(ids.includes('unknown'), false)
  assert.equal(layout[0].items[0][0], 'inbox')
})

test('rendered indicators follow their configured side and order', async () => {
  const { readFileSync, writeFileSync, mkdtempSync, rmSync } = await import('node:fs')
  const { pathToFileURL } = await import('node:url')
  const { parse, compileScript, compileTemplate } = await import('@vue/compiler-sfc')
  const { JSDOM } = await import('jsdom')
  const dom = new JSDOM('<div id="root"></div>')
  const globals = ['window', 'document', 'Element', 'HTMLElement', 'SVGElement', 'MutationObserver']
  for (const key of globals) globalThis[key] = dom.window[key]
  const { descriptor } = parse(readFileSync(new URL('../src/components/StatusBar.vue', import.meta.url), 'utf8'))
  const script = compileScript(descriptor, { id: 'status-bar-test' })
  const template = compileTemplate({
    source: descriptor.template.content,
    filename: 'StatusBar.vue',
    id: 'status-bar-test',
    compilerOptions: { bindingMetadata: script.bindings },
  })
  assert.deepEqual(template.errors, [])
  const dir = mkdtempSync(new URL('../node_modules/.status-bar-test-', import.meta.url))
  const module = dir + '/component.mjs'
  writeFileSync(
    module,
    script.content
      .replace("import Icon from './Icon.vue'", "const Icon = { template: '<i />' }")
      .replace("import UsageStatus from './UsageStatus.vue'", "const UsageStatus = { template: '<span>Usage</span>' }")
      .replace(
        /from '(\.\.\/shared\/[^']+)'/g,
        (_, path) =>
          `from '${new URL(path + '.js', new URL('../src/components/StatusBar.vue', import.meta.url)).href}'`,
      )
      .replace('export default', 'const component =') +
      '\n' +
      template.code +
      '\ncomponent.render = render; export default component',
  )
  let app
  try {
    const { createApp, h, reactive, nextTick } = await import('vue')
    const { default: StatusBar } = await import(pathToFileURL(module).href)
    const preferences = reactive({ appearance: 'dark', theme: 'system', hiddenStatusBarItems: ['usage'] })
    const workspace = { name: 'Example', repositories: [] }
    const session = { name: 'Session', agent: 'codex', worktreeBranch: 'feature/test', restored: true }
    app = createApp({
      render: () => h(StatusBar, { preferences, workspace, session, workspaces: [], runs: [], profiles: [] }),
    })
    app.mount('#root')
    assert.match(document.querySelector('.status-context').textContent, /Example.*Session.*feature\/test/)
    Object.assign(preferences, moveStatusBarItem(preferences, 'branch', 1, 0))
    await nextTick()
    assert.equal(document.querySelector('.status-context').textContent.includes('feature/test'), false)
    assert.match(document.querySelector('.status-summary').textContent, /^feature\/test/)
    Object.assign(preferences, moveStatusBarItem(preferences, 'session', 1, 0))
    await nextTick()
    assert.match(document.querySelector('.status-summary').textContent, /^Sessionfeature\/test/)
    Object.assign(preferences, statusBarItemChange(preferences, 'branch'))
    await nextTick()
    assert.equal(document.querySelector('footer').textContent.includes('feature/test'), false)
    assert.match(document.querySelector('.status-summary').textContent, /No running sessions/)
    assert.equal(document.querySelector('footer').textContent.includes('Changes / Files'), false)
    assert.equal(document.querySelector('[aria-label^="Theme"]'), null)
  } finally {
    app?.unmount()
    dom.window.close()
    rmSync(dir, { recursive: true, force: true })
    for (const key of globals) delete globalThis[key]
  }
})
