import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs'
import { pathToFileURL } from 'node:url'
import { parse, compileScript } from '@vue/compiler-sfc'
import { JSDOM } from 'jsdom'

test('Settings lists workspaces and opens the selected workspace without selecting an application workspace', async () => {
  const dom = new JSDOM('<div class="app-shell"><div id="root"></div></div>')
  for (const key of ['window', 'document', 'Element', 'HTMLElement', 'SVGElement']) globalThis[key] = dom.window[key]
  dom.window.HTMLElement.prototype.scrollTo = () => {}
  const { descriptor } = parse(readFileSync(new URL('../src/components/Settings.vue', import.meta.url), 'utf8'))
  const script = compileScript(descriptor, { id: 'workspace-settings', inlineTemplate: true })
  const dir = mkdtempSync(new URL('../node_modules/.workspace-settings-', import.meta.url))
  const code = script.content
    .replace(/import \{ privacyURL \} from ['"]\.\.\/shared\/telemetry['"]/, 'const privacyURL = "https://example.com"')
    .replace(/import (\w+) from ['"]\.\/(\w+)\.vue['"]/g, (_, name) => `const ${name} = { render: () => null }`)
    .replace(
      /import \{ call, openURL \} from ['"]\.\.\/shared\/api['"]/,
      'const call = async () => []; const openURL = () => {}',
    )
    .replace(
      /from ['"]\.\.\/shared\/(\w+)['"]/g,
      (_, name) => `from '${new URL('../src/shared/' + name + '.js', import.meta.url).href}'`,
    )
  writeFileSync(dir + '/component.mjs', code)
  let app
  try {
    const { createApp, h, ref, nextTick } = await import('vue')
    const { default: Settings } = await import(pathToFileURL(dir + '/component.mjs').href)
    const section = ref('General')
    const selected = []
    const workspaces = [
      { id: 'one', name: 'First', emoji: '🎵' },
      { id: 'two', name: 'Long workspace name' },
    ]
    app = createApp({
      render: () =>
        h(
          Settings,
          {
            preferences: {},
            workspaces,
            profiles: [],
            workspaceIcons: { one: 'data:image/png;base64,first', two: 'data:image/png;base64,second' },
            initialSection: section.value,
            onWorkspaceSettings: (workspace) => {
              selected.push(workspace.id)
              section.value = 'workspace:' + workspace.id
            },
          },
          { workspace: () => h('p', { id: 'workspace-form' }, section.value) },
        ),
    })
    app.mount('#root')
    const button = (name) =>
      [...document.querySelectorAll('.settings-rail button')].find((b) => b.textContent.includes(name))
    assert.equal(button('First').querySelector('img'), null)
    assert.ok(button('Long workspace').querySelector('img'))
    button('First').click()
    await nextTick()
    assert.deepEqual(selected, ['one'])
    assert.equal(document.querySelector('h2').textContent.trim(), 'First')
    assert.equal(button('First').getAttribute('aria-pressed'), 'true')
    assert.equal(document.querySelector('.usage-status-setting'), null)
    button('Long workspace').click()
    await nextTick()
    assert.deepEqual(selected, ['one', 'two'])
    assert.equal(document.querySelector('#workspace-form').textContent, 'workspace:two')
    assert.equal(button('First').getAttribute('aria-pressed'), 'false')
    button('General').click()
    await nextTick()
    assert.equal(document.querySelector('#workspace-form'), null)
    button('Long workspace').click()
    await nextTick()
    assert.equal(document.querySelector('#workspace-form').textContent, 'workspace:two')
  } finally {
    app?.unmount()
    rmSync(dir, { recursive: true, force: true })
    dom.window.close()
  }
})
