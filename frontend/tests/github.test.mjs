import test from 'node:test'
import assert from 'node:assert/strict'
import {
  canWorkOn,
  descriptionSource,
  effortLabel,
  gitHubItem,
  isPermissionError,
  itemSessions,
  modelEfforts,
  sessionsLabel,
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
