import test from 'node:test'
import assert from 'node:assert/strict'
import {
  canWorkOn,
  conflictTask,
  descriptionSource,
  effortLabel,
  gitHubItem,
  isPermissionError,
  itemKey,
  itemSessions,
  matchesWorkItem,
  modelEfforts,
  pullConflict,
  sessionsLabel,
  workItemAssignees,
  workItems,
} from '../src/shared/github.js'

test('links sessions to their issue or pull request', () => {
  const issue = { number: 22 },
    pull = { number: 23, pull_request: {} }
  assert.deepEqual(gitHubItem(issue), { number: 22, pullRequest: false })
  assert.deepEqual(gitHubItem(pull), { number: 23, pullRequest: true })
  const project = {
    sessions: [
      { id: 'a', name: '#22 Fix', gitHubItem: { number: 22 } },
      { id: 'b', name: 'Plain' },
      { id: 'c', name: '#23 Review', gitHubItem: { number: 23, pullRequest: true } },
      { id: 'd', name: 'Renamed', gitHubItem: { number: 22 } },
    ],
  }
  assert.deepEqual(
    itemSessions(project, issue).map((s) => s.id),
    ['a', 'd'],
  )
  assert.deepEqual(
    itemSessions(project, pull).map((s) => s.id),
    ['c'],
  )
  assert.deepEqual(itemSessions(project, { number: 23 }), [])
  assert.deepEqual(itemSessions(undefined, issue), [])
  assert.equal(sessionsLabel(itemSessions(project, pull)), '1 session: #23 Review')
  assert.equal(sessionsLabel(itemSessions(project, issue)), '2 sessions')
})

test('Work Items keeps items of different repositories apart', () => {
  const app = { id: 'app', name: 'app', gitHub: 'o/app' },
    site = { id: 'site', name: 'site', gitHub: 'o/site' }
  const items = workItems([
    { repository: app, items: [{ number: 1, title: 'Old', updated_at: '2026-01-01T00:00:00Z', assignees: [] }] },
    {
      repository: site,
      items: [
        { number: 1, title: 'New', updated_at: '2026-02-01T00:00:00Z', assignees: [{ login: 'zoe' }] },
        {
          number: 2,
          title: 'Pull',
          pull_request: {},
          updated_at: '2026-01-15T00:00:00Z',
          assignees: [{ login: 'al' }],
        },
      ],
    },
  ])
  assert.deepEqual(items.map(itemKey), ['site#1', 'site#2', 'app#1'])
  assert.equal(items[0].repositoryGitHub, 'o/site')
  assert.deepEqual(workItemAssignees(items), ['al', 'zoe'])
  const shown = (filters) => items.filter((i) => matchesWorkItem(i, filters)).map(itemKey)
  assert.deepEqual(shown({}), ['site#1', 'site#2', 'app#1'])
  assert.deepEqual(shown({ source: 'app' }), ['app#1'])
  assert.deepEqual(shown({ assignee: 'zoe' }), ['site#1'])
  assert.deepEqual(shown({ assignee: 'al' }), ['site#2'])
  assert.deepEqual(shown({ assignee: '-' }), ['app#1'])
  assert.deepEqual(shown({ search: 'old' }), ['app#1'])
  // The same number in another repository belongs to other sessions.
  const project = {
    sessions: [
      { id: 'a', repositoryID: 'app', gitHubItem: { number: 1 } },
      { id: 'b', repositoryID: 'site', gitHubItem: { number: 1 } },
    ],
  }
  assert.deepEqual(
    itemSessions(project, items[0]).map((s) => s.id),
    ['b'],
  )
})

test('offers Work on and Review only for open items', () => {
  assert.equal(canWorkOn({ state: 'open' }), true)
  assert.equal(canWorkOn({ state: 'closed' }), false)
  assert.equal(canWorkOn({ state: 'open', draft: true, pull_request: {} }), true)
  assert.equal(canWorkOn({ state: 'closed', pull_request: { merged_at: '2026-01-01T00:00:00Z' } }), false)
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

test('reports only confirmed conflicts on open pull requests', () => {
  const pr = {
    number: 50,
    title: 'PR tab',
    state: 'open',
    html_url: 'https://github.com/o/r/pull/50',
    mergeable: false,
    mergeable_state: 'dirty',
    maintainer_can_modify: false,
    head: { ref: 'feature', repo: { full_name: 'o/r' } },
    base: { ref: 'main', repo: { full_name: 'o/r' } },
  }
  assert.deepEqual(pullConflict(pr), {
    number: 50,
    title: 'PR tab',
    url: 'https://github.com/o/r/pull/50',
    base: 'main',
    head: 'feature',
    forkWithoutEdits: false,
  })
  assert.equal(pullConflict({ ...pr, mergeable: true, mergeable_state: 'clean' }), null)
  // GitHub is still computing mergeability.
  assert.equal(pullConflict({ ...pr, mergeable: null, mergeable_state: 'unknown' }), null)
  assert.equal(pullConflict({ ...pr, mergeable: undefined }), null)
  assert.equal(pullConflict({ ...pr, state: 'closed' }), null)
  assert.equal(pullConflict({ number: 3, state: 'open', mergeable: false }), null)
  assert.equal(pullConflict(undefined), null)
  const fork = { ...pr, head: { ref: 'feature', repo: { full_name: 'someone/r' } } }
  assert.equal(pullConflict(fork).forkWithoutEdits, true)
  assert.equal(pullConflict({ ...fork, maintainer_can_modify: true }).forkWithoutEdits, false)
  assert.equal(pullConflict({ ...pr, head: { ref: 'feature', repo: null } }).forkWithoutEdits, true)
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
