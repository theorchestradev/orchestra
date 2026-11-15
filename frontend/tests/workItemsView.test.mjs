import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs'
import { pathToFileURL } from 'node:url'
import { parse, compileScript } from '@vue/compiler-sfc'
import { JSDOM } from 'jsdom'

test('mixed-source view isolates failures and renders read-only items by their capabilities', async () => {
  const dom = new JSDOM('<div class="app-shell"><div id="root"></div></div>')
  for (const key of ['window', 'document', 'Element', 'HTMLElement', 'SVGElement']) globalThis[key] = dom.window[key]
  const { descriptor } = parse(readFileSync(new URL('../src/components/WorkItems.vue', import.meta.url), 'utf8'))
  const script = compileScript(descriptor, { id: 'work-items-view', inlineTemplate: true })
  const dir = mkdtempSync(new URL('../node_modules/.work-items-view-', import.meta.url))
  let code = script.content
    .replace(
      /import \{ call, copy, openURL \} from ['"]\.\.\/shared\/api['"]/,
      'const call = (...args) => globalThis.viewCall(...args); const copy = async () => {}; const openURL = () => {}',
    )
    .replace(/import \{ relativeDate \} from ['"]\.\.\/shared\/appearance['"]/, 'const relativeDate = () => "now"')
    .replace(/import \{ shortcutLabel \} from ['"]\.\.\/shared\/platform['"]/, 'const shortcutLabel = x => x')
    .replace(/import \{ refreshDelay \} from ['"]\.\.\/shared\/pullRequest['"]/, 'const refreshDelay = () => 60000')
    .replace(
      /from ['"]\.\.\/shared\/(workItems|workspace|markdown)['"]/g,
      (_, name) => `from '${new URL('../src/shared/' + name + '.js', import.meta.url).href}'`,
    )
    .replace(
      /import (\w+) from ['"]\.\/(\w+)\.vue['"]/g,
      (_, name) => `const ${name} = { name: '${name}', render: () => null }`,
    )
  writeFileSync(dir + '/component.mjs', code)
  let app
  try {
    const { createApp, h, nextTick } = await import('vue')
    // A Linear issue as the Store normalizes it: no edits, comments or checks.
    const issue = {
      provider: 'linear',
      sourceID: 'source',
      id: 'uuid',
      identifier: 'ENG-123',
      title: 'Linear issue',
      url: 'https://linear.app/a/issue/ENG-123',
      state: 'open',
      status: 'Under Review',
      assignees: [],
      labels: [],
      can: { chooseRepository: true },
    }
    const calls = []
    globalThis.viewCall = async (method, ...args) => {
      calls.push([method, ...args])
      if (method === 'WorkItems' && args[1] === 'github:issues:repo') throw Error('GitHub unavailable')
      if (method === 'WorkItems') return [issue]
      if (method === 'WorkItemDetail')
        return { ...issue, body: '# Safe description\n<script>alert(1)</script>', comments: [], prompt: 'Work' }
      throw Error('Unexpected method: ' + method)
    }
    const { default: View } = await import(pathToFileURL(dir + '/component.mjs').href)
    const workspace = {
      id: 'workspace',
      repositories: [{ id: 'repo', name: 'App', gitHub: 'a/b' }],
      linearSources: [{ id: 'source', name: 'Team', teamID: 'team' }],
      sources: [
        { id: 'github:issues:repo', provider: 'github', kind: 'workItems', name: 'a/b' },
        { id: 'source', provider: 'linear', kind: 'workItems', name: 'Team' },
      ],
      sessions: [],
    }
    app = createApp({ render: () => h(View, { workspace, visible: true, profiles: [] }) })
    app.mount('#root')
    const settle = async () => {
      await nextTick()
      await new Promise((r) => setTimeout(r, 0))
      await nextTick()
    }
    await settle()
    assert.match(document.querySelector('.github-row').textContent, /ENG-123/)
    assert.match(document.querySelector('[role="alert"]').textContent, /a\/b: Error: GitHub unavailable/)
    assert.match(document.querySelector('.status-badge').textContent, /Under Review/)
    document.querySelector('.item-open').click()
    await settle()
    assert.match(document.querySelector('.description-body').textContent, /Safe description/)
    assert.equal(document.querySelector('.description-body script'), null)
    assert.equal(document.querySelector('.description-edit'), null)
    assert.equal(document.querySelector('.metadata-trigger[aria-label="Status"]').disabled, true)
    assert.equal(document.querySelector('.metadata-trigger[aria-label="Assignees"]').disabled, true)
    assert.equal(document.querySelector('.metadata-trigger[aria-label="Labels"]').disabled, true)
    assert.equal(document.querySelector('.pr-checks'), null)
    assert.equal(document.querySelector('.discussion h4'), null)
    assert.ok(
      calls.some(([method, , source, id]) => method === 'WorkItemDetail' && source === 'source' && id === 'uuid'),
    )
    assert.equal(
      calls.some(([method]) => method === 'ReviewStatus'),
      false,
    )
  } finally {
    app?.unmount()
    rmSync(dir, { recursive: true, force: true })
  }
})
