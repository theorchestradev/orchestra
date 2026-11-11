import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs'
import { pathToFileURL } from 'node:url'
import { parse, compileScript } from '@vue/compiler-sfc'
import { JSDOM } from 'jsdom'

test('Automations creates weekly and one-time schedules and edits reactive saved items', async () => {
  const dom = new JSDOM('<div class="app-shell"><div id="root"></div></div>')
  for (const key of ['window', 'document', 'Document', 'Element', 'HTMLElement', 'SVGElement'])
    globalThis[key] = dom.window[key]
  const calls = []
  globalThis.automationCall = async (method, value) => {
    calls.push({ method, value })
    if (method === 'RunAutomation') return state.value
    if (method === 'ClearAutomationHistory') {
      state.value.automations[0].runs = []
      return state.value
    }
    return { ...state.value, automations: [{ ...value, id: 'saved' }] }
  }
  const { descriptor } = parse(readFileSync(new URL('../src/components/Automations.vue', import.meta.url), 'utf8'))
  const script = compileScript(descriptor, { id: 'automations-test', inlineTemplate: true })
  const dir = mkdtempSync(new URL('../node_modules/.automations-test-', import.meta.url))
  writeFileSync(
    dir + '/component.mjs',
    script.content
      .replace(/import \{ call \} from ['"]\.\.\/shared\/api['"]/, 'const call = globalThis.automationCall')
      .replace(/import Icon from ['"]\.\/Icon.vue['"]/, 'const Icon = { render: () => null }')
      .replaceAll("'../shared/menu'", JSON.stringify(new URL('../src/shared/menu.js', import.meta.url).href))
      .replaceAll(
        "'../shared/appearance'",
        JSON.stringify(new URL('../src/shared/appearance.js', import.meta.url).href),
      )
      .replaceAll("'../shared/workspace'", JSON.stringify(new URL('../src/shared/workspace.js', import.meta.url).href)),
  )
  let app, state
  try {
    const { default: Automations } = await import(pathToFileURL(dir + '/component.mjs').href)
    const { createApp, h, ref } = await import('vue')
    state = ref({
      preferences: { defaultAgent: 'codex' },
      selectedWorkspaceID: 'workspace',
      workspaces: [{ id: 'workspace', name: 'Example', repositories: [{ id: 'repo', name: 'Repository' }] }],
    })
    app = createApp({
      render: () =>
        h(Automations, {
          state: state.value,
          onState: (value) => (state.value = value),
        }),
    })
    app.mount('#root')
    const flush = () => new Promise((resolve) => setImmediate(resolve))
    const back = async () => {
      document.querySelector('.automation-breadcrumb button').click()
      await flush()
    }
    const button = (name) => [...document.querySelectorAll('button')].find((b) => b.textContent.trim() === name)
    assert.equal(document.querySelector('[aria-label="Search automations"]'), null)
    const templates = [...document.querySelectorAll('.automation-template')]
    assert.equal(templates.length, 6)
    for (const template of templates) {
      const name = template.querySelector('.automation-template-name').textContent
      const currentTemplate = [...document.querySelectorAll('.automation-template')].find(
        (el) => el.querySelector('.automation-template-name').textContent === name,
      )
      currentTemplate.click()
      await flush()
      assert.equal(
        document.querySelector('form input:not([type])').value,
        template.querySelector('.automation-template-name').textContent,
      )
      assert.ok(document.querySelector('textarea').value.length > 50)
      assert.equal(document.querySelector('.automation-agent-picker select').value, 'codex')
      assert.equal(document.querySelector('.automation-context-fields select').value, 'workspace')
      assert.equal(calls.length, 0, 'Choosing a template must not save or run it')
      document.querySelector('.automation-breadcrumb button').click()
      await flush()
    }
    button('New Automation').click()
    await flush()
    assert.equal(document.querySelector('[role="dialog"]'), null)
    assert.equal(document.querySelector('[aria-current="page"]').textContent, 'New Automation')
    const fill = (selector, value) => {
      const el = document.querySelector(selector)
      el.value = value
      el.dispatchEvent(new dom.window.Event('input', { bubbles: true }))
    }
    for (const select of document.querySelectorAll('form select')) {
      assert.ok(select.closest('.theme-select'), 'Automation dropdowns use the shared Settings control')
      assert.ok(select.parentElement.querySelector('select'), 'Keep native selects for keyboard interaction')
    }
    assert.deepEqual(
      [...document.querySelectorAll('.automation-step > legend')].map((el) => el.textContent),
      ['Trigger', 'Action'],
    )
    const enabled = document.querySelector('[role="switch"][aria-label="Enabled"]')
    assert.equal(enabled.getAttribute('aria-checked'), 'true')
    assert.equal(enabled.type, 'button')
    enabled.click()
    await flush()
    assert.equal(enabled.getAttribute('aria-checked'), 'false')
    assert.equal(calls.length, 0, 'Toggling the draft must not submit the form')
    fill('input:not([type])', 'Review')
    fill('textarea', 'Review the code')
    assert.equal(
      document.querySelector('.automation-context-fields label:last-child select').textContent.trim(),
      'Repository',
    )
    document.querySelector('form').dispatchEvent(new dom.window.Event('submit', { bubbles: true, cancelable: true }))
    await flush()
    assert.equal(calls[0].method, 'SaveAutomation')
    assert.deepEqual(calls[0].value.days, [1, 2, 3, 4, 5])
    assert.equal(calls[0].value.repositoryID, 'repo')
    assert.equal(calls[0].value.enabled, false)
    assert.ok(document.querySelector('form'), 'Saving stays on the details page')
    assert.equal(document.querySelector('[aria-current="page"]').textContent, 'Review')
    assert.equal(document.querySelector('[role="tab"][aria-selected="true"]').textContent.trim(), 'Configuration')
    await back()
    state.value.automations[0].enabled = true
    await flush()
    assert.equal(document.querySelector('.automation-schedule').textContent.trim(), 'Weekdays · 09:00')
    document.querySelector('.automation-actions button').click()
    await flush()
    button('Edit').click()
    await flush()
    assert.equal(document.querySelector('textarea').value, 'Review the code')
    const schedule = document.querySelector('[aria-label="Schedule"]')
    schedule.value = 'once'
    schedule.dispatchEvent(new dom.window.Event('change', { bubbles: true }))
    await flush()
    fill('input[type="date"]', '2027-01-01')
    document.querySelector('form').dispatchEvent(new dom.window.Event('submit', { bubbles: true, cancelable: true }))
    await flush()
    assert.deepEqual(calls[1].value.days, [])
    assert.equal(calls[1].value.date, '2027-01-01')
    await back()
    assert.equal(document.querySelector('.automation-schedule').textContent.trim(), '2027-01-01 · 09:00')
    assert.equal(document.querySelector('h1').textContent, 'Automations')
    assert.equal(document.querySelectorAll('.automation-row').length, 1)
    fill('input[type="search"]', 'no such automation')
    await flush()
    assert.equal(document.querySelectorAll('.automation-row').length, 0)
    assert.match(document.querySelector('.automation-empty').textContent, /No matching automations/)
    button('Clear Filters').click()
    await flush()
    fill('input[type="search"]', 'Repository')
    await flush()
    assert.equal(document.querySelectorAll('.automation-row').length, 1)
    const filter = document.querySelector('[aria-label="Filter automations"]')
    filter.value = 'disabled'
    filter.dispatchEvent(new dom.window.Event('change', { bubbles: true }))
    await flush()
    assert.equal(document.querySelectorAll('.automation-row').length, 0)
    button('Clear Filters').click()
    await flush()
    assert.equal(document.querySelectorAll('.automation-row').length, 1)
    assert.equal(document.querySelector('form'), null)
    assert.equal(document.querySelector('[role="status"]').textContent, 'Automation saved.')
    document.querySelector('.automation-actions button').click()
    await flush()
    button('Edit').click()
    await flush()
    const daily = document.querySelector('[aria-label="Schedule"]')
    daily.value = 'daily'
    daily.dispatchEvent(new dom.window.Event('change', { bubbles: true }))
    await flush()
    assert.equal(document.querySelector('input[type="date"]'), null)
    assert.equal(document.querySelector('.weekdays'), null)
    document.querySelector('form').dispatchEvent(new dom.window.Event('submit', { bubbles: true, cancelable: true }))
    await flush()
    assert.deepEqual(calls[2].value.days, [0, 1, 2, 3, 4, 5, 6])
    await back()
    assert.equal(document.querySelector('.automation-schedule').textContent.trim(), 'Every day · 09:00')
    const trigger = document.querySelector('.automation-actions button')
    trigger.click()
    await flush()
    assert.equal(trigger.getAttribute('aria-expanded'), 'true')
    state.value = JSON.parse(JSON.stringify(state.value))
    state.value.automations[0].name = 'Review updated elsewhere'
    await flush()
    assert.ok(document.querySelector('[role="menu"]'), 'Fresh snapshots must not dismiss an open menu')
    const menu = document.querySelector('[role="menu"]')
    assert.deepEqual(
      [...menu.querySelectorAll('[role="menuitem"]')].map((el) => el.textContent.trim()),
      ['View Runs', 'Edit', 'Run Now', 'Delete'],
    )
    assert.ok(menu.querySelector('[role="separator"]'))
    menu.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'End', bubbles: true }))
    assert.equal(document.activeElement.textContent.trim(), 'Delete')
    menu.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await flush()
    assert.equal(document.querySelector('[role="menu"]'), null)
    assert.equal(document.activeElement, trigger)
    trigger.click()
    await flush()
    button('Run Now').click()
    await flush()
    assert.deepEqual(calls[3], { method: 'RunAutomation', value: 'saved' })
    assert.equal(document.querySelector('[role="status"]').textContent, 'Automation started.')
    assert.equal(document.querySelector('[role="menu"]'), null)
    trigger.click()
    await flush()
    button('Edit').click()
    await flush()
    assert.equal(document.querySelector('form input:not([type])').value, 'Review updated elsewhere')
    document.querySelector('.automation-breadcrumb button').click()
    await flush()
    trigger.click()
    await flush()
    document.body.dispatchEvent(new dom.window.Event('pointerdown', { bubbles: true }))
    await flush()
    assert.equal(document.querySelector('[role="menu"]'), null)

    state.value.automations[0].runs = [
      { id: 'old', startedAt: '2026-11-01T09:00:00Z', trigger: 'legacy', status: 'started', sessionID: 'deleted' },
      {
        id: 'new',
        startedAt: '2026-11-02T09:00:00Z',
        trigger: 'manual',
        status: 'failed',
        name: 'Original name',
        agent: 'codex',
        prompt: '<script>original prompt</script>',
        error: 'Missing workspace',
      },
    ]
    await flush()
    document.querySelector('.automation-title-button').click()
    await flush()
    assert.equal(document.querySelector('[role="dialog"]'), null)
    assert.equal(document.querySelector('[role="tab"][aria-selected="true"]').textContent.trim(), 'Configuration')
    fill('textarea', 'Unsaved configuration')
    button('Runs').click()
    await flush()
    assert.equal(document.querySelector('[role="tab"][aria-selected="true"]').textContent.trim(), 'Runs')
    button('Configuration').click()
    await flush()
    assert.equal(
      document.querySelector('textarea').value,
      'Unsaved configuration',
      'Changing tabs preserves unsaved configuration',
    )
    document
      .querySelector('[role="tablist"]')
      .dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true, cancelable: true }))
    await flush()
    assert.equal(document.activeElement.id, 'automation-tab-runs')
    const runs = document.querySelectorAll('.automation-runs li')
    assert.equal(runs.length, 2)
    assert.match(runs[0].textContent, /Launch failed/)
    assert.equal(runs[0].querySelector('pre').textContent, '<script>original prompt</script>')
    assert.equal(runs[0].querySelector('script'), null)
    assert.match(runs[1].textContent, /Session deleted/)
    assert.match(runs[1].textContent, /original configuration unavailable/)
    button('Clear History…').click()
    await flush()
    assert.match(document.querySelector('.automation-history').textContent, /Sessions will be kept/)
    button('Clear History').click()
    await flush()
    assert.equal(calls.at(-1).method, 'ClearAutomationHistory')
    assert.match(document.querySelector('.automation-history').textContent, /No runs recorded yet/)
    await back()

    state.value = { ...state.value, workspaces: [], automations: [] }
    await flush()
    assert.equal(button('New Automation').disabled, true)
    assert.ok([...document.querySelectorAll('.automation-template')].every((button) => button.disabled))
    assert.match(document.querySelector('.automation-empty').textContent, /Add a workspace/)
  } finally {
    app?.unmount()
    dom.window.close()
    rmSync(dir, { recursive: true, force: true })
    delete globalThis.automationCall
    for (const key of ['window', 'document', 'Document', 'Element', 'HTMLElement', 'SVGElement']) delete globalThis[key]
  }
})
