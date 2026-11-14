import test from 'node:test'
import assert from 'node:assert/strict'
import { linearItem, executionRepository } from '../src/shared/linear.js'
import { itemReference, gitHubItem, itemSessions, itemKey, matchesWorkItem, canWorkOn } from '../src/shared/github.js'
import { hasWorkItems } from '../src/shared/workspace.js'
import { renderMarkdown } from '../src/shared/markdown.js'
import createDOMPurify from 'dompurify'
import { JSDOM } from 'jsdom'
const source = { id: 'linear-source', name: 'Engineering', repositoryID: 'repo' }
const issue = {
  id: 'uuid',
  identifier: 'ENG-1',
  title: 'Fix',
  description: '# Task',
  url: 'https://linear.app/a/issue/ENG-1',
  updatedAt: '2026-01-01',
  state: { name: 'In Progress', type: 'started' },
  labels: { nodes: [{ name: 'Bug', color: '#ffffff' }] },
}
test('Linear keeps native status, Markdown and identifiers with source filtering', () => {
  const item = linearItem(issue, source)
  assert.equal(item.nativeStatus, 'In Progress')
  assert.equal(item.body, '# Task')
  assert.equal(item.labels[0].color, 'ffffff')
  assert.ok(canWorkOn(item))
  assert.ok(matchesWorkItem(item, { source: source.id, search: 'eng-1' }))
  assert.equal(matchesWorkItem(item, { source: 'repo' }), false)
  assert.equal(canWorkOn(linearItem({ ...issue, state: { type: 'completed', name: 'Done' } }, source)), false)
  assert.ok(hasWorkItems({ repositories: [{ id: 'repo' }], linearSources: [source] }))
  const purifier = createDOMPurify(new JSDOM('').window)
  assert.doesNotMatch(
    renderMarkdown('<script>alert(1)</script><a href="javascript:alert(1)">Click</a>', purifier),
    /script|javascript:/,
  )
})
test('provider identity survives repository choices, duplicate sources and number collisions', () => {
  const item = linearItem(issue, source)
  const github = { id: 'uuid', repositoryID: 'repo', number: 1, html_url: 'https://github.com/a/b/issues/1' }
  const workspace = {
    sessions: [
      { id: 'linear', repositoryID: 'another-repo', workItem: itemReference(item) },
      { id: 'github', repositoryID: 'repo', workItem: itemReference(github) },
      { id: 'legacy', repositoryID: 'repo', gitHubItem: { number: 1 } },
    ],
  }
  assert.deepEqual(
    itemSessions(workspace, item).map((s) => s.id),
    ['linear'],
  )
  assert.deepEqual(
    itemSessions(workspace, { ...item, sourceID: 'another-source' }).map((s) => s.id),
    ['linear'],
  )
  assert.deepEqual(
    itemSessions(workspace, github).map((s) => s.id),
    ['github', 'legacy'],
  )
  assert.notEqual(itemKey(item), itemKey(github))
  assert.notEqual(itemKey(item), itemKey({ ...item, sourceID: 'another-source' }))
  assert.equal(gitHubItem(item).reference.provider, 'linear')
})
test('execution repository uses explicit choice then valid source default and asks for ambiguous work', () => {
  const workspace = { repositories: [{ id: 'repo' }, { id: 'other' }] }
  assert.equal(executionRepository(workspace, source), 'repo')
  assert.equal(executionRepository(workspace, source, 'other'), 'other')
  assert.equal(executionRepository(workspace, {}), '')
  assert.equal(executionRepository(workspace, { repositoryID: 'removed' }), '')
  assert.equal(executionRepository({ repositories: [{ id: 'only' }] }, {}), 'only')
})
