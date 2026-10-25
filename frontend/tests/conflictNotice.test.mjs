import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs'
import { pathToFileURL } from 'node:url'
import { parse, compileScript } from '@vue/compiler-sfc'
import { JSDOM } from 'jsdom'

test('conflict notice names the base branch and starts Resolve conflicts', async () => {
  const dom = new JSDOM('<div id="root"></div>')
  for (const key of ['window', 'document', 'Element', 'HTMLElement', 'SVGElement']) globalThis[key] = dom.window[key]
  const { descriptor } = parse(readFileSync(new URL('../src/components/ConflictNotice.vue', import.meta.url), 'utf8'))
  const script = compileScript(descriptor, { id: 'conflict-notice-test', inlineTemplate: true })
  const dir = mkdtempSync(new URL('../node_modules/.conflict-notice-test-', import.meta.url))
  writeFileSync(
    dir + '/conflictNotice.mjs',
    script.content
      .replace(/import Icon from ['"]\.\/Icon.vue['"]/, 'const Icon = { render: () => null }')
      .replace(/import WorkOnButton from ['"]\.\/WorkOnButton.vue['"]/, 'const WorkOnButton = globalThis.workOnStub'),
  )
  let app
  try {
    const { createApp, h, ref } = await import('vue')
    const buttons = []
    globalThis.workOnStub = {
      props: ['label', 'defaultAgent', 'profiles', 'profileId', 'visible'],
      emits: ['work'],
      setup: (props, { emit }) => {
        buttons.push(props)
        return () => h('button', { class: 'work-stub', onClick: () => emit('work', { agent: 'codex' }) }, props.label)
      },
    }
    const { default: ConflictNotice } = await import(pathToFileURL(dir + '/conflictNotice.mjs').href)
    const conflict = ref({ base: 'main', head: 'feature', forkWithoutEdits: false }),
      readOnly = ref(false),
      emitted = []
    app = createApp({
      render: () =>
        h(ConflictNotice, {
          conflict: conflict.value,
          readOnly: readOnly.value,
          defaultAgent: 'claude',
          profiles: [],
          profileId: 'p1',
          visible: true,
          onResolve: (choice) => emitted.push(choice),
        }),
    })
    app.mount('#root')
    const notice = () => document.querySelector('.conflict-notice')
    assert.equal(notice().getAttribute('aria-label'), 'Merge conflicts')
    assert.match(notice().textContent, /This branch has conflicts with main that must be resolved\./)
    assert.doesNotMatch(notice().textContent, /push may fail/)
    assert.equal(buttons[0].label, 'Resolve conflicts')
    assert.equal(buttons[0].defaultAgent, 'claude')
    assert.equal(buttons[0].profileId, 'p1')
    document.querySelector('.work-stub').click()
    assert.deepEqual(emitted, [{ agent: 'codex' }])

    readOnly.value = true
    app._instance.update()
    await Promise.resolve()
    assert.match(notice().textContent, /You may not have push access/)
    conflict.value = { ...conflict.value, forkWithoutEdits: true }
    app._instance.update()
    await Promise.resolve()
    assert.match(notice().textContent, /fork that doesn’t allow edits by maintainers/)
  } finally {
    app?.unmount()
    rmSync(dir, { recursive: true, force: true })
  }
})
