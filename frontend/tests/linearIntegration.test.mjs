import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs'
import { pathToFileURL } from 'node:url'
import { parse, compileScript } from '@vue/compiler-sfc'
import { JSDOM } from 'jsdom'

test('workspace Linear forms route credentials and sources through their own workspace', async () => {
  const dom = new JSDOM('<div id="root"></div>')
  const globals = ['window', 'document', 'Document', 'Element', 'HTMLElement', 'SVGElement']
  for (const key of globals) globalThis[key] = dom.window[key]
  const { descriptor } = parse(
    readFileSync(new URL('../src/components/LinearIntegration.vue', import.meta.url), 'utf8'),
  )
  const script = compileScript(descriptor, { id: 'linear-integration-test', inlineTemplate: true })
  const dir = mkdtempSync(new URL('../node_modules/.linear-integration-', import.meta.url))
  writeFileSync(
    dir + '/component.mjs',
    script.content
      .replace(
        /import \{ call, openURL \} from ['"]\.\.\/shared\/api['"]/,
        'const call = (...args) => globalThis.linearCall(...args); const openURL = () => {}',
      )
      .replace(/import Icon from ['"]\.\/Icon.vue['"]/, 'const Icon = { render: () => null }'),
  )
  const calls = []
  globalThis.linearCall = async (method, workspace, value) => {
    calls.push([method, workspace, value])
    if (method === 'LinearAccount') return {}
    if (method === 'ConnectLinear') return { id: 'account', name: 'Account' }
    if (method === 'LinearChoices') return value ? [] : [{ id: 'team', name: 'Team' }]
    if (method === 'SaveLinearSource') return { workspaces: [] }
  }
  let app
  try {
    const { createApp, h, nextTick } = await import('vue')
    const { default: View } = await import(pathToFileURL(dir + '/component.mjs').href)
    const workspace = { id: 'workspace-a', name: 'Client', repositories: [], linearSources: [] }
    app = createApp({ render: () => h(View, { workspace }) })
    app.mount('#root')
    const settle = async () => {
      await nextTick()
      await new Promise((r) => setTimeout(r, 0))
      await nextTick()
    }
    await settle()
    const input = document.querySelector('input[type="password"]')
    input.value = 'test-key'
    input.dispatchEvent(new dom.window.Event('input', { bubbles: true }))
    document.querySelector('form').dispatchEvent(new dom.window.Event('submit', { cancelable: true }))
    await settle()
    assert.ok(
      calls.some(([method, id, key]) => method === 'ConnectLinear' && id === workspace.id && key === 'test-key'),
    )
    const select = document.querySelector('select')
    select.value = 'team'
    select.dispatchEvent(new dom.window.Event('change', { bubbles: true }))
    await settle()
    document.querySelector('form').dispatchEvent(new dom.window.Event('submit', { cancelable: true }))
    await settle()
    assert.ok(
      calls.some(
        ([method, id, source]) => method === 'SaveLinearSource' && id === workspace.id && source.teamID === 'team',
      ),
    )
    Array.from(document.querySelectorAll('button'))
      .find((b) => b.textContent.includes('Disconnect'))
      .click()
    await settle()
    assert.ok(calls.some(([method, id]) => method === 'DisconnectLinear' && id === workspace.id))
    assert.ok(document.querySelector('input[type="password"]'))
    assert.ok(calls.every(([, id]) => id === workspace.id))
  } finally {
    app?.unmount()
    dom.window.close()
    rmSync(dir, { recursive: true, force: true })
    delete globalThis.linearCall
    for (const key of globals) delete globalThis[key]
  }
})
