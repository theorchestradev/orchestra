import test from 'node:test'
import assert from 'node:assert/strict'
import createDOMPurify from 'dompurify'
import { JSDOM } from 'jsdom'
import {
  canWorkOn,
  conflictTask,
  descriptionSource,
  effortLabel,
  executionRepository,
  isPermissionError,
  itemKey,
  itemReference,
  itemSessions,
  matchesWorkItem,
  modelEfforts,
  providerIcon,
  providerName,
  sessionsLabel,
  sortWorkItems,
  workItemAssignees,
} from '../src/shared/workItems.js'
import { renderMarkdown } from '../src/shared/markdown.js'

// Items as the Store normalizes them: GitHub IDs name the repository and number.
const gitHubItem = (repositoryID, number, fields = {}) => ({
  provider: 'github',
  sourceID: `github:issues:${repositoryID}`,
  repositoryID,
  id: `${repositoryID}#${number}`,
  number,
  identifier: `#${number}`,
  title: `Item ${number}`,
  url: `https://github.com/o/${repositoryID}/issues/${number}`,
  assignees: [],
  ...fields,
})
const linearItem = {
  provider: 'linear',
  sourceID: 'linear:engineering',
  repositoryID: 'app',
  id: 'uuid',
  identifier: 'ENG-1',
  title: 'Fix',
  url: 'https://linear.app/a/issue/ENG-1',
  assignees: [],
}

test('links sessions to their item by provider identity', () => {
  const issue = gitHubItem('app', 22),
    pull = gitHubItem('app', 23, { sourceID: 'github:pulls:app', review: true })
  assert.deepEqual(itemReference(pull), {
    provider: 'github',
    id: 'app#23',
    url: 'https://github.com/o/app/issues/23',
    identifier: '#23',
    number: 23,
    review: true,
  })
  assert.deepEqual(itemReference(linearItem), {
    provider: 'linear',
    id: 'uuid',
    url: 'https://linear.app/a/issue/ENG-1',
    identifier: 'ENG-1',
  })
  const workspace = {
    sessions: [
      { id: 'a', name: '#22 Fix', workItem: itemReference(issue) },
      { id: 'b', name: 'Plain' },
      { id: 'c', name: '#23 Review', workItem: itemReference(pull) },
      { id: 'd', name: 'Renamed', workItem: itemReference(issue) },
      // The same number in another repository, and a Linear issue worked on elsewhere.
      { id: 'e', workItem: itemReference(gitHubItem('site', 22)) },
      { id: 'f', repositoryID: 'site', workItem: itemReference(linearItem) },
    ],
  }
  const ids = (item) => itemSessions(workspace, item).map((s) => s.id)
  assert.deepEqual(ids(issue), ['a', 'd'])
  assert.deepEqual(ids(pull), ['c'])
  assert.deepEqual(ids(linearItem), ['f'])
  assert.deepEqual(ids({ ...linearItem, sourceID: 'linear:other' }), ['f'])
  assert.deepEqual(itemSessions(undefined, issue), [])
  assert.equal(sessionsLabel(itemSessions(workspace, pull)), '1 session: #23 Review')
  assert.equal(sessionsLabel(itemSessions(workspace, issue)), '2 sessions')
  assert.notEqual(itemKey(issue), itemKey(gitHubItem('site', 22)))
  assert.notEqual(itemKey(linearItem), itemKey({ ...linearItem, sourceID: 'linear:other' }))
})

test('Work Items filters by source, assignee and search', () => {
  const items = [
    gitHubItem('site', 1, { title: 'New', assignees: [{ login: 'zoe' }] }),
    gitHubItem('site', 2, { title: 'Pull', assignees: [{ login: 'al' }] }),
    gitHubItem('app', 1, { title: 'Old' }),
    linearItem,
  ]
  assert.deepEqual(workItemAssignees(items), ['al', 'zoe'])
  const shown = (filters) => items.filter((i) => matchesWorkItem(i, filters)).map(itemKey)
  assert.equal(shown({}).length, 4)
  assert.deepEqual(shown({ source: 'github:issues:app' }), ['github:issues:app:app#1'])
  assert.deepEqual(shown({ assignee: 'zoe' }), ['github:issues:site:site#1'])
  assert.deepEqual(shown({ assignee: '-' }), ['github:issues:app:app#1', 'linear:engineering:uuid'])
  assert.deepEqual(shown({ search: 'old' }), ['github:issues:app:app#1'])
  assert.deepEqual(shown({ search: 'eng-1' }), ['linear:engineering:uuid'])
})

test('names providers and falls back for unknown ones', () => {
  assert.equal(providerName('github'), 'GitHub')
  assert.equal(providerIcon('linear'), 'linear')
  assert.equal(providerIcon('gitlab'), 'circle.dotted')
  assert.equal(providerName('gitlab'), 'gitlab')
})

test('execution repository uses the explicit choice, then the item repository, and asks when ambiguous', () => {
  const workspace = { repositories: [{ id: 'app' }, { id: 'other' }] }
  assert.equal(executionRepository(workspace, linearItem), 'app')
  assert.equal(executionRepository(workspace, linearItem, 'other'), 'other')
  assert.equal(executionRepository(workspace, {}), '')
  assert.equal(executionRepository(workspace, { repositoryID: 'removed' }), '')
  assert.equal(executionRepository({ repositories: [{ id: 'only' }] }, {}), 'only')
})

test('renders Markdown descriptions without scripts', () => {
  const purifier = createDOMPurify(new JSDOM('').window)
  assert.doesNotMatch(
    renderMarkdown('<script>alert(1)</script><a href="javascript:alert(1)">Click</a>', purifier),
    /script|javascript:/,
  )
})

test('offers Work on and Review only for open items', () => {
  assert.equal(canWorkOn({ state: 'open' }), true)
  assert.equal(canWorkOn({ state: 'closed' }), false)
  assert.equal(canWorkOn({ state: 'open', status: 'Draft', review: true }), true)
  assert.equal(canWorkOn({ state: 'closed', status: 'Merged', review: true }), false)
  assert.equal(canWorkOn(undefined), false)
})

test('offers efforts the chosen model accepts', () => {
  const models = [
    { id: 'a', efforts: ['low', 'medium', 'high', 'xhigh'] },
    { id: 'b', efforts: ['low', 'high'] },
    { id: 'c', efforts: [] },
  ]
  assert.deepEqual(modelEfforts(models, 'a'), ['low', 'medium', 'high', 'xhigh'])
  assert.deepEqual(modelEfforts(models, 'c'), [])
  assert.deepEqual(modelEfforts(models, 'missing'), [])
  assert.deepEqual(modelEfforts(models, ''), ['low', 'high'])
  assert.deepEqual(modelEfforts([{ id: 'x', efforts: [] }], ''), [])
  assert.deepEqual(modelEfforts([], ''), [])
  assert.equal(effortLabel('xhigh'), 'Extra high')
  assert.equal(effortLabel('max'), 'Max')
})

test('compares description drafts with the normalized Markdown source', () => {
  assert.equal(descriptionSource({ body: 'a\r\n\r\n- [ ] b\rc' }), 'a\n\n- [ ] b\nc')
  assert.equal(descriptionSource({ body: null }), '')
  assert.equal(descriptionSource(undefined), '')
})

test('recognizes GitHub permission errors', () => {
  assert.ok(isPermissionError('gh: Resource not accessible by integration (HTTP 403)'))
  assert.ok(isPermissionError('gh: Not Found (HTTP 404)'))
  assert.ok(!isPermissionError('gh: request timed out'))
  assert.ok(!isPermissionError('The description changed on GitHub since you started editing.'))
})

test('asks the agent to merge the base branch and push to the head', () => {
  const task = conflictTask({
    number: 50,
    title: 'PR tab',
    url: 'https://github.com/o/r/pull/50',
    base: 'main',
    head: 'feature',
  })
  assert.equal(task.name, '#50 Resolve conflicts: PR tab')
  for (const part of [
    'Resolve the merge conflicts on this GitHub pull request: https://github.com/o/r/pull/50.',
    'has conflicts with main',
    'gh pr checkout https://github.com/o/r/pull/50.',
    'another worktree already has feature checked out',
    'HEAD:feature',
    "Fetch main and merge it into the branch. Merge, don't rebase, so the push isn't a force-push.",
    'keeping the intent of both sides, then run the project',
    'Commit the merge and push it to the pull request',
    'report which files you resolved and how',
    'If the push is refused',
    "doesn't allow edits by maintainers, stop and explain",
  ])
    assert.ok(task.prompt.includes(part), part)
})

test('sortWorkItems sorts by each column and keeps unassigned items last', () => {
  const items = [
    { identifier: 'TECH-9', title: 'beta', status: 'Todo', updatedAt: '2026-01-02T00:00:00Z', assignees: [] },
    {
      identifier: 'TECH-10',
      title: 'Alpha',
      status: 'Backlog',
      updatedAt: '2026-01-03T00:00:00Z',
      assignees: [{ login: 'zoe' }],
    },
    {
      identifier: 'TECH-2',
      title: 'gamma',
      status: 'In Progress',
      updatedAt: '2026-01-01T00:00:00Z',
      assignees: [{ login: 'amy' }],
    },
  ]
  const ids = (sort) => sortWorkItems(items, sort).map((i) => i.identifier)
  assert.deepEqual(ids(), ['TECH-10', 'TECH-9', 'TECH-2'])
  assert.deepEqual(ids({ column: 'updated', direction: 'asc' }), ['TECH-2', 'TECH-9', 'TECH-10'])
  assert.deepEqual(ids({ column: 'id' }), ['TECH-2', 'TECH-9', 'TECH-10'])
  assert.deepEqual(ids({ column: 'id', direction: 'desc' }), ['TECH-10', 'TECH-9', 'TECH-2'])
  assert.deepEqual(ids({ column: 'title' }), ['TECH-10', 'TECH-9', 'TECH-2'])
  assert.deepEqual(ids({ column: 'status' }), ['TECH-10', 'TECH-2', 'TECH-9'])
  assert.deepEqual(ids({ column: 'assignees' }), ['TECH-2', 'TECH-10', 'TECH-9'])
  assert.deepEqual(ids({ column: 'assignees', direction: 'desc' }), ['TECH-10', 'TECH-2', 'TECH-9'])
  assert.deepEqual(
    items.map((i) => i.identifier),
    ['TECH-9', 'TECH-10', 'TECH-2'],
  )
})
