import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { JSDOM } from 'jsdom'
import { parse } from '@vue/compiler-sfc'
import { compile } from '@vue/compiler-dom'

test('Board follows live agent activity, opens cards and keeps shells out', async () => {
  const dom = new JSDOM('<div id="root"></div>')
  const globals = ['window', 'document', 'Element', 'HTMLElement', 'SVGElement']
  for (const key of globals) globalThis[key] = dom.window[key]
  const Vue = await import('vue')
  const appearance = await import('../src/shared/appearance.js')
  const { repositoryLabel } = await import('../src/shared/workspace.js')
  const { descriptor } = parse(readFileSync(new URL('../src/components/Board.vue', import.meta.url), 'utf8'))
  const sessions = [
    {
      id: 'a',
      name: 'Agent with a very long descriptive session name',
      agent: 'codex',
      running: true,
      activity: 'working',
    },
    { id: 'b', name: 'Waiting agent', agent: 'claude', running: true, activity: 'waiting' },
    {
      id: 'c',
      name: 'Restored agent',
      agentSessionID: 'conversation',
      agent: 'pi',
      restored: true,
      hasLaunched: true,
      activity: 'working',
    },
    { id: 'd', name: 'Ended agent', agent: 'codex', hasLaunched: true, activity: 'ended' },
    { id: 'e', name: 'Unknown agent', agent: 'claude', running: true, activity: 'future-status' },
    { id: 'shell', name: 'Terminal', agent: 'terminal', running: true, activity: 'working' },
  ]
  const props = Vue.reactive({ workspace: { name: 'Workspace', sessions }, appearance: 'dark' })
  const source = descriptor.scriptSetup.content.replace(/^import .*$/gm, '')
  const setup = new Function(
    'computed',
    'defineProps',
    'defineEmits',
    'agents',
    'agentIcon',
    'activityLabel',
    'isTerminal',
    'repositoryLabel',
    source + '\nreturn { columns, count, detail, status, agents, agentIcon }',
  )
  const render = new Function('Vue', compile(descriptor.template.content, { prefixIdentifiers: true }).code)(Vue)
  const events = []
  const app = Vue.createApp(
    {
      emits: ['select', 'create'],
      components: { Icon: { render: () => null } },
      setup: () => ({
        ...setup(
          Vue.computed,
          () => props,
          () => () => {},
          appearance.agents,
          appearance.agentIcon,
          appearance.activityLabel,
          appearance.isTerminal,
          repositoryLabel,
        ),
        ...Vue.toRefs(props),
      }),
      render,
    },
    { onSelect: (session) => events.push(['select', session]), onCreate: () => events.push(['create']) },
  )
  try {
    app.mount('#root')
    const column = (id) => dom.window.document.querySelector('#board-' + id).closest('section')
    const cardCount = (id) => column(id).querySelectorAll('.board-card').length
    assert.deepEqual(['waiting', 'working', 'inactive', 'unknown'].map(cardCount), [1, 1, 2, 1])
    assert.match(dom.window.document.body.textContent, /5 agents/)
    assert.match(column('inactive').textContent, /Ready to resume/)
    props.workspace.sessions[0].activity = 'waiting'
    props.appearance = 'light'
    await Vue.nextTick()
    assert.deepEqual(['waiting', 'working'].map(cardCount), [2, 0])
    const card = column('waiting').querySelector('.board-card')
    assert.match(card.querySelector('img').src, /CodexIcon.svg/)
    card.click()
    assert.equal(events[0][0], 'select')
    assert.equal(events[0][1].id, 'a')
    dom.window.document.querySelector('.board-header button').click()
    assert.equal(events[1][0], 'create')
    props.workspace.sessions = []
    await Vue.nextTick()
    assert.equal(dom.window.document.querySelectorAll('.board-empty').length, 4)
  } finally {
    app.unmount()
    dom.window.close()
    for (const key of globals) delete globalThis[key]
  }
})
