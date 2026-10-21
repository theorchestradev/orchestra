import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs'
import { pathToFileURL } from 'node:url'
import { parse, compileScript } from '@vue/compiler-sfc'
import { JSDOM } from 'jsdom'

test('Files expands folders, navigates by keyboard and opens selected files in the central view', async () => {
  const dom = new JSDOM('<div id="root"></div>')
  for (const key of ['window', 'document', 'Element', 'HTMLElement', 'SVGElement']) globalThis[key] = dom.window[key]
  Object.defineProperty(dom.window.HTMLElement.prototype, 'offsetParent', {
    get() {
      return this.parentElement
    },
  })
  const pending = [],
    opened = []
  globalThis.filesCall = async (method, id, path) => {
    assert.equal(method, 'SessionFiles')
    if (id === 'one' && path === 'slow-folder') return new Promise((resolve) => pending.push(resolve))
    if (path === 'src') return { entries: [{ name: 'main.go', path: 'src/main.go', directory: false }] }
    if (path) return { content: id + ':' + path }
    return {
      entries: [
        { name: 'src', path: 'src', directory: true },
        { name: 'slow-folder', path: 'slow-folder', directory: true },
      ],
    }
  }
  const { descriptor } = parse(readFileSync(new URL('../src/components/SessionFiles.vue', import.meta.url), 'utf8'))
  const script = compileScript(descriptor, { id: 'files-test', inlineTemplate: true })
  const dir = mkdtempSync(new URL('../node_modules/.files-test-', import.meta.url))
  writeFileSync(
    dir + '/files.mjs',
    script.content
      .replace(/import \{ call \} from ['"]\.\.\/shared\/api['"]/, 'const call = globalThis.filesCall')
      .replace(/import Icon from ['"]\.\/Icon.vue['"]/, 'const Icon = { render: () => null }'),
  )
  let app
  try {
    const { default: Files } = await import(pathToFileURL(dir + '/files.mjs').href)
    const { createApp, h, ref } = await import('vue')
    const session = ref({ id: 'one' })
    const flush = () => new Promise((resolve) => setImmediate(resolve))
    app = createApp({ render: () => h(Files, { session: session.value, onOpen: (path) => opened.push(path) }) })
    app.mount('#root')
    await flush()
    const rows = () => [...document.querySelectorAll('.session-file-row')]
    assert.equal(rows().length, 2)
    rows()[0].click()
    await flush()
    assert.equal(rows().length, 3)
    rows()[0].focus()
    rows()[0].dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }))
    assert.equal(document.activeElement, rows()[1])
    rows()[1].click()
    await flush()
    assert.deepEqual(opened, ['src/main.go'])
    assert.equal(document.querySelector('.session-file-preview'), null)
    rows()[2].click()
    await flush()
    session.value = { id: 'two' }
    await flush()
    pending[0]({ entries: [{ name: 'stale', path: 'slow-folder/stale' }] })
    await flush()
    assert.equal(document.querySelector('.session-file-preview'), null)
    assert.equal(rows().length, 2)
  } finally {
    app?.unmount()
    dom.window.close()
    rmSync(dir, { recursive: true, force: true })
    delete globalThis.filesCall
    for (const key of ['window', 'document', 'Element', 'HTMLElement', 'SVGElement']) delete globalThis[key]
  }
})
