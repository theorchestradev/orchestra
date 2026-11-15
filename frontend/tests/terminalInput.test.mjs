import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { JSDOM } from 'jsdom'

test('a focused terminal under Settings does not consume Escape before Settings closes', () => {
  const source = readFileSync(new URL('../src/components/Terminal.vue', import.meta.url), 'utf8')
  const body = source.match(/terminal\.attachCustomKeyEventHandler\(\(e\) => \{([\s\S]*?)\n  \}\)/)[1]
  const props = { visible: false }
  let terminalKeys = 0
  const handler = new Function('props', 'mac', 'terminalKeeps', `return (e) => {${body}}`)(props, true, () => {
    terminalKeys++
    return true
  })
  const dom = new JSDOM('<textarea></textarea>')
  const textarea = dom.window.document.querySelector('textarea')
  let writes = 0
  let closed = false
  textarea.addEventListener('keydown', (e) => {
    if (handler(e)) writes++
  })
  dom.window.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      closed = true
      props.visible = true
    }
  })
  textarea.focus()
  textarea.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
  assert.equal(closed, true)
  assert.equal(writes, 0)
  assert.equal(terminalKeys, 0)
  textarea.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
  assert.equal(writes, 1)
  dom.window.close()
  const app = readFileSync(new URL('../src/App.vue', import.meta.url), 'utf8')
  assert.match(app, /:visible="modal !== 'settings' && workspace\?\.id === p\.id && currentDestination === sess\.id"/)
})
