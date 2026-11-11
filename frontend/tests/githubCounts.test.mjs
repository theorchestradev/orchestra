import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs'
import { pathToFileURL } from 'node:url'
import { parse, compileScript } from '@vue/compiler-sfc'
import { JSDOM } from 'jsdom'

test('GitHub counts track loaded, filtered lists and invalidate failed or changed sources', async () => {
  const dom = new JSDOM('<div class="app-shell"><div id="root"></div></div>')
  for (const key of ['window', 'document', 'Element', 'HTMLElement', 'SVGElement']) globalThis[key] = dom.window[key]
  let resolve
  globalThis.countCall = () =>
    new Promise((r) => {
      resolve = r
    })
  const dir = mkdtempSync(new URL('../node_modules/.github-counts-', import.meta.url))
  const script = compileScript(
    parse(readFileSync(new URL('../src/components/GitHub.vue', import.meta.url), 'utf8')).descriptor,
    {
      id: 'github-counts',
      inlineTemplate: true,
    },
  )
    .content.replace(
      /import \{ call, copy, openURL \} from ['"]\.\.\/shared\/api['"]/,
      'const call = (...args) => globalThis.countCall(...args), copy = () => {}, openURL = () => {}',
    )
    .replace(/import (\w+) from ['"]\.\/[^'"]+\.vue['"]/g, 'const $1 = { render: () => null }')
    .replace(/(['"])\.\.\/shared\/(\w+)\1/g, (_, q, name) =>
      JSON.stringify(new URL('../src/shared/' + name + '.js', import.meta.url).href),
    )
  writeFileSync(dir + '/github.mjs', script)
  let app
  try {
    const { createApp, h, ref, nextTick } = await import('vue')
    const { default: GitHub } = await import(pathToFileURL(dir + '/github.mjs').href)
    const workspace = ref({ id: 'w', repositories: [{ id: 'r', name: 'Repo', gitHub: 'o/r' }], sessions: [] })
    const counts = []
    app = createApp({
      render: () => h(GitHub, { workspace: workspace.value, profiles: [], onCount: (n) => counts.push(n) }),
    })
    app.mount('#root')
    assert.equal(counts.at(-1), null)
    const flush = async () => {
      await new Promise((r) => setTimeout(r, 0))
      await nextTick()
    }
    resolve([])
    await flush()
    assert.equal(counts.at(-1), 0)
    workspace.value.repositories = [{ id: 'other', name: 'Other', gitHub: 'o/other' }]
    await nextTick()
    assert.equal(counts.at(-1), null)
    resolve([
      { number: 1, title: 'Issue', updated_at: '2026-01-01' },
      { number: 2, title: 'PR', pull_request: {}, updated_at: '2026-01-01' },
    ])
    await flush()
    assert.equal(counts.at(-1), 1)
    const search = document.querySelector('input[placeholder="Search by title or number"]')
    search.value = 'missing'
    search.dispatchEvent(new dom.window.Event('input', { bubbles: true }))
    await nextTick()
    assert.equal(counts.at(-1), 0)
    globalThis.countCall = async () => {
      throw new Error('Unavailable')
    }
    workspace.value.repositories = [{ id: 'failed', name: 'Failed', gitHub: 'o/failed' }]
    await flush()
    assert.equal(counts.at(-1), null)
  } finally {
    app?.unmount()
    rmSync(dir, { recursive: true, force: true })
    dom.window.close()
    delete globalThis.countCall
  }
})
