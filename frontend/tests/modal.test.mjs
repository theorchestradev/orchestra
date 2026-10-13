import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs'
import { pathToFileURL } from 'node:url'
import { parse, compileScript, compileTemplate } from '@vue/compiler-sfc'
import { JSDOM } from 'jsdom'

test('modal Tab navigation reaches buttons, skips disabled controls, and wraps in both directions', async () => {
  const dom = new JSDOM('<div id="root"></div>')
  for (const key of ['window', 'document', 'Element', 'HTMLElement', 'SVGElement']) globalThis[key] = dom.window[key]
  // jsdom has no layout; these mounted controls are visible in the native dialog.
  Object.defineProperty(dom.window.HTMLElement.prototype, 'offsetParent', {
    get() {
      return this.parentElement
    },
  })
  const { descriptor } = parse(readFileSync(new URL('../src/components/Modal.vue', import.meta.url), 'utf8'))
  const script = compileScript(descriptor, { id: 'modal-test' })
  const template = compileTemplate({
    source: descriptor.template.content,
    filename: 'Modal.vue',
    id: 'modal-test',
    compilerOptions: { bindingMetadata: script.bindings },
  })
  assert.deepEqual(template.errors, [])
  const dir = mkdtempSync(new URL('../node_modules/.modal-test-', import.meta.url))
  const module = dir + '/modal.mjs'
  writeFileSync(
    module,
    script.content.replace('export default', 'const component =') +
      '\n' +
      template.code +
      '\ncomponent.render = render; export default component',
  )
  let app
  try {
    const { default: Modal } = await import(pathToFileURL(module).href)
    const { createApp, h, nextTick } = await import('vue')
    app = createApp({
      render: () =>
        h(
          Modal,
          { label: 'Privacy' },
          {
            default: () => [
              h('button', { id: 'first' }, 'Privacy'),
              h('button', { disabled: true }, 'Disabled'),
              h('button', { id: 'switch', role: 'switch' }, 'Share usage'),
              h('button', { id: 'last' }, 'Privacy docs'),
            ],
          },
        ),
    })
    app.mount('#root')
    await nextTick()
    const tab = (shiftKey = false) =>
      document.dispatchEvent(
        new dom.window.KeyboardEvent('keydown', { key: 'Tab', shiftKey, bubbles: true, cancelable: true }),
      )
    assert.equal(document.activeElement.getAttribute('role'), 'dialog')
    tab()
    assert.equal(document.activeElement.id, 'first')
    tab()
    assert.equal(document.activeElement.id, 'switch')
    tab()
    assert.equal(document.activeElement.id, 'last')
    tab()
    assert.equal(document.activeElement.id, 'first')
    tab(true)
    assert.equal(document.activeElement.id, 'last')
  } finally {
    app?.unmount()
    dom.window.close()
    rmSync(dir, { recursive: true, force: true })
    for (const key of ['window', 'document', 'Element', 'HTMLElement', 'SVGElement']) delete globalThis[key]
  }
})
