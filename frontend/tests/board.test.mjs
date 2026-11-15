import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { JSDOM } from 'jsdom'
import { parse } from '@vue/compiler-sfc'
import { compile } from '@vue/compiler-dom'

test('Board places sessions by activity and pull request state, opens cards and keeps shells out', async () => {
  const dom = new JSDOM('<div id="root"></div>')
  const globals = ['window', 'document', 'Element', 'HTMLElement', 'SVGElement']
  for (const key of globals) globalThis[key] = dom.window[key]
  const Vue = await import('vue')
  const appearance = await import('../src/shared/appearance.js')
  const { repositoryLabel, sessionRepository } = await import('../src/shared/workspace.js')
  const { descriptor } = parse(readFileSync(new URL('../src/components/Board.vue', import.meta.url), 'utf8'))
  const sessions = [
    {
      id: 'a',
      name: 'Agent with a very long descriptive session name',
      agent: 'codex',
      repositoryID: 'r',
      worktreeBranch: 'feat/long',
      hasLaunched: true,
      running: true,
      activity: 'working',
    },
    {
      id: 'b',
      name: 'Waiting agent',
      agent: 'claude',
      repositoryID: 'r',
      hasLaunched: true,
      running: true,
      activity: 'waiting',
    },
    { id: 'c', name: 'Merged agent', agent: 'pi', repositoryID: 'r', restored: true, hasLaunched: true },
    { id: 'd', name: 'New agent', agent: 'codex' },
    { id: 'shell', name: 'Terminal', agent: 'terminal', running: true, activity: 'working' },
  ]
  const workspace = {
    id: 'w',
    name: 'Workspace',
    repositories: [{ id: 'r', name: 'repo', gitHub: 'o/repo' }],
    sessions,
  }
  const props = Vue.reactive({ workspaces: [workspace], currentWorkspaceId: 'w', appearance: 'dark' })
  const pullRequests = Vue.reactive(new Map())
  const loaded = []
  const states = { b: { state: 'open', branch: 'feat/waiting' }, c: { state: 'merged', branch: 'feat/merged' } }
  const loadPullRequest = async (id) => {
    loaded.push(id)
    if (states[id]) pullRequests.set(id, states[id])
  }
  const source = descriptor.scriptSetup.content.replace(/^import .*$/gm, '')
  const setup = new Function(
    'computed',
    'onMounted',
    'ref',
    'defineProps',
    'defineEmits',
    'agents',
    'agentIcon',
    'activityLabel',
    'isTerminal',
    'repositoryLabel',
    'sessionRepository',
    'loadPullRequest',
    'pullRequests',
    source +
      '\nreturn { workspaceFilter, createWorkspace, columns, count, branch, place, agentName, status, agentIcon }',
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
          Vue.onMounted,
          Vue.ref,
          () => props,
          () => () => {},
          appearance.agents,
          appearance.agentIcon,
          appearance.activityLabel,
          appearance.isTerminal,
          repositoryLabel,
          sessionRepository,
          loadPullRequest,
          pullRequests,
        ),
        ...Vue.toRefs(props),
      }),
      render,
    },
    { onSelect: (card) => events.push(['select', card.session]), onCreate: () => events.push(['create']) },
  )
  try {
    app.mount('#root')
    await new Promise((resolve) => setTimeout(resolve))
    await Vue.nextTick()
    const column = (id) => dom.window.document.querySelector('#board-' + id).closest('section')
    const cardCount = (id) => column(id).querySelectorAll('.board-card').length
    assert.deepEqual(loaded, ['a', 'b', 'c'])
    assert.deepEqual(['todo', 'progress', 'needs', 'review', 'done'].map(cardCount), [1, 1, 1, 0, 1])
    assert.match(dom.window.document.body.textContent, /4 sessions/)
    assert.match(column('progress').textContent, /feat\/long/)
    assert.match(column('progress').textContent, /Codex · Agent working/)
    assert.match(column('needs').textContent, /feat\/waiting/)
    assert.match(column('done').textContent, /Pi · Ready to restart/)
    pullRequests.set('a', { state: 'draft' })
    props.appearance = 'light'
    await Vue.nextTick()
    assert.equal(cardCount('progress'), 1)
    sessions[0].activity = 'waiting'
    sessions[1].activity = 'working'
    props.workspaces = [{ ...workspace }]
    await Vue.nextTick()
    assert.deepEqual(['progress', 'needs', 'review'].map(cardCount), [1, 1, 0])
    sessions[1].running = false
    props.workspaces = [{ ...workspace }]
    await Vue.nextTick()
    assert.deepEqual(['progress', 'needs', 'review'].map(cardCount), [0, 1, 1])
    const card = column('needs').querySelector('.board-card')
    assert.match(card.querySelector('img').src, /CodexIcon.svg/)
    card.click()
    assert.equal(events[0][0], 'select')
    assert.equal(events[0][1].id, 'a')
    dom.window.document.querySelector('.board-header button').click()
    assert.equal(events[1][0], 'create')
    workspace.sessions = []
    props.workspaces = [{ ...workspace }]
    await Vue.nextTick()
    assert.equal(dom.window.document.querySelectorAll('.board-empty').length, 5)
  } finally {
    app.unmount()
    dom.window.close()
    for (const key of globals) delete globalThis[key]
  }
})
