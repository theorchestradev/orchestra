import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs'
import { pathToFileURL } from 'node:url'
import { parse, compileScript } from '@vue/compiler-sfc'
import { JSDOM } from 'jsdom'

test('central file view highlights source, ignores stale reads and handles missing files', async () => {
  const dom = new JSDOM('<div id="root"></div>')
  for (const key of ['window', 'document', 'Element', 'HTMLElement', 'SVGElement']) globalThis[key] = dom.window[key]
  let resolveOld,
    closed = false
  globalThis.fileViewCall = async (method, id, path) => {
    assert.equal(method, 'SessionFiles')
    if (path === 'old.go')
      return new Promise((resolve) => {
        resolveOld = resolve
      })
    if (path === 'missing.go') throw Error('File no longer exists')
    assert.equal(id, 'session-two')
    return { content: 'package main\nfunc main() {}\n' }
  }
  const { descriptor } = parse(readFileSync(new URL('../src/components/FileView.vue', import.meta.url), 'utf8'))
  const script = compileScript(descriptor, { id: 'file-view-test', inlineTemplate: true })
  const dir = mkdtempSync(new URL('../node_modules/.file-view-test-', import.meta.url))
  writeFileSync(
    dir + '/view.mjs',
    script.content
      .replace(/import \{ call \} from ['"]\.\.\/shared\/api['"]/, 'const call = globalThis.fileViewCall')
      .replace("'../shared/syntax.js'", JSON.stringify(new URL('../src/shared/syntax.js', import.meta.url).href))
      .replace("'../shared/openIn.js'", JSON.stringify(new URL('../src/shared/openIn.js', import.meta.url).href))
      .replace(/import Icon from ['"]\.\/Icon.vue['"]/, 'const Icon = { render: () => null }'),
  )
  let app
  try {
    const { default: View } = await import(pathToFileURL(dir + '/view.mjs').href)
    const { createApp, h, ref } = await import('vue')
    const file = ref({ id: 'old', sessionID: 'session-one', path: 'old.go' })
    app = createApp({
      render: () =>
        h(View, {
          file: file.value,
          session: { name: 'Agent' },
          onClose: () => {
            closed = true
          },
        }),
    })
    const flush = () => new Promise((resolve) => setImmediate(resolve))
    app.mount('#root')
    await flush()
    file.value = { id: 'new', sessionID: 'session-two', path: 'main.go' }
    await flush()
    resolveOld({ content: 'old content' })
    await flush()
    assert.equal(document.querySelector('pre').textContent, 'package main\nfunc main() {}\n')
    assert.ok(document.querySelector('code .hljs-keyword'))
    assert.deepEqual(
      [...document.querySelectorAll('.file-line-numbers span')].map((line) => line.textContent),
      ['1', '2', '3'],
    )
    assert.equal(document.querySelector('.file-line-numbers').getAttribute('aria-hidden'), 'true')
    assert.equal(document.querySelector('[aria-label="Refresh file"]'), null)
    assert.equal(document.querySelector('h2').textContent, 'main.go')
    document.querySelector('[aria-label="Close file"]').click()
    assert.equal(closed, true)
    file.value = { id: 'missing', sessionID: 'session-two', path: 'missing.go' }
    await flush()
    assert.match(document.querySelector('[role="alert"]').textContent, /File no longer exists/)
    assert.equal(document.querySelector('pre'), null)
  } finally {
    app?.unmount()
    dom.window.close()
    rmSync(dir, { recursive: true, force: true })
    delete globalThis.fileViewCall
    for (const key of ['window', 'document', 'Element', 'HTMLElement', 'SVGElement']) delete globalThis[key]
  }
})
