import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs'
import { pathToFileURL } from 'node:url'
import { parse, compileScript } from '@vue/compiler-sfc'
import { JSDOM } from 'jsdom'

test('launch choices preserve manual IDs and scope catalogs to the chosen profile', async () => {
  const dom = new JSDOM('<div class="app-shell"><div id="root"></div></div>')
  for (const key of ['window', 'document', 'Element', 'HTMLElement', 'SVGElement']) globalThis[key] = dom.window[key]
  globalThis.innerWidth = 900
  globalThis.innerHeight = 650
  const { descriptor } = parse(readFileSync(new URL('../src/components/WorkOnButton.vue', import.meta.url), 'utf8'))
  const script = compileScript(descriptor, { id: 'work-on-test', inlineTemplate: true })
  const dir = mkdtempSync(new URL('../node_modules/.work-on-test-', import.meta.url))
  writeFileSync(
    dir + '/component.mjs',
    script.content
      .replace(
        /import \{ call \} from ['"]\.\.\/shared\/api['"]/,
        'const call = (...args) => globalThis.launchCall(...args)',
      )
      .replace(/import \{ agents \} from ['"]\.\.\/shared\/appearance['"]/, 'const agents = globalThis.launchAgents')
      .replace(
        /import \{ modelEfforts, effortLabel \} from ['"]\.\.\/shared\/github['"]/,
        'const modelEfforts = (models, id) => models.find(m => m.id === id)?.efforts || []; const effortLabel = e => e',
      )
      .replace(
        /import \{ findProfile \} from ['"]\.\.\/shared\/profiles['"]/,
        'const findProfile = (profiles, id) => profiles.find(p => p.id === id)',
      )
      .replace(/import Icon from ['"]\.\/Icon.vue['"]/, 'const Icon = { render: () => null }'),
  )
  let app
  try {
    const { createApp, h, nextTick } = await import('vue')
    const calls = [],
      emitted = []
    globalThis.launchAgents = [
      { id: 'copilot', name: 'Copilot' },
      { id: 'cursor', name: 'Cursor' },
      { id: 'aider', name: 'Aider', initialPrompt: false },
    ]
    globalThis.launchCall = async (...args) => {
      calls.push(args)
      return args[1] === 'copilot'
        ? { models: [], customModel: true, efforts: ['low', 'high'] }
        : { models: [{ id: args[2] ? 'work-model' : 'personal-model', name: 'Model', efforts: [] }], customModel: true }
    }
    const { default: WorkOnButton } = await import(pathToFileURL(dir + '/component.mjs').href)
    app = createApp({
      render: () =>
        h(WorkOnButton, {
          label: 'Work on',
          defaultAgent: 'copilot',
          visible: true,
          profiles: [{ id: 'work', name: 'Work' }],
          onWork: (choice) => emitted.push(choice),
        }),
    })
    app.mount('#root')
    const settle = async () => {
      await nextTick()
      await new Promise((resolve) => setTimeout(resolve, 0))
      await nextTick()
    }
    const choose = async (select, value) => {
      select.value = value
      select.dispatchEvent(new dom.window.Event('change', { bubbles: true }))
      await settle()
    }
    document.querySelector('.split-toggle').click()
    await settle()
    const input = document.querySelector('input')
    assert.equal(input.getAttribute('autocapitalize'), 'off')
    const model = 'provider/My-Model[effort=high]'
    input.value = model
    input.dispatchEvent(new dom.window.Event('input', { bubbles: true }))
    await choose(document.querySelectorAll('select')[2], 'high')
    document.querySelector('form').dispatchEvent(new dom.window.Event('submit', { bubbles: true, cancelable: true }))
    await settle()
    assert.deepEqual(emitted, [{ agent: 'copilot', model, effort: 'high', profileID: '' }])
    document.querySelector('.split-toggle').click()
    await settle()
    assert.equal(document.querySelector('option[value="aider"]').disabled, true)
    await choose(document.querySelectorAll('select')[1], 'cursor')
    assert.ok(document.querySelector('option[value="personal-model"]'))
    await choose(document.querySelectorAll('select')[0], 'work')
    assert.ok(document.querySelector('option[value="work-model"]'))
    assert.equal(document.querySelector('option[value="personal-model"]'), null)
    assert.deepEqual(calls, [
      ['AgentLaunchOptions', 'copilot', ''],
      ['AgentLaunchOptions', 'cursor', ''],
      ['AgentLaunchOptions', 'cursor', 'work'],
    ])
  } finally {
    app?.unmount()
    rmSync(dir, { recursive: true, force: true })
  }
})
