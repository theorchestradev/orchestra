import test from 'node:test'
import assert from 'node:assert/strict'
import {
  activityCounts,
  checksSummary,
  failureContext,
  groupChecks,
  pullRequestActivity,
  refreshDelay,
  ringSegments,
} from '../src/shared/pullRequest.js'

const check = (name, status, extra = {}) => ({ name, status, ...extra })

test('checks are grouped worst first and summarized', () => {
  const checks = [
    check('build', 'success'),
    check('lint', 'failure'),
    check('docs', 'skipped'),
    check('tests', 'pending'),
    check('format', 'success'),
  ]
  assert.deepEqual(
    groupChecks(checks).map((group) => [group.status, group.title, group.checks.map((c) => c.name)]),
    [
      ['failure', '1 failing check', ['lint']],
      ['pending', '1 in progress check', ['tests']],
      ['success', '2 successful checks', ['build', 'format']],
      ['skipped', '1 skipped check', ['docs']],
    ],
  )
  const summary = checksSummary(checks)
  assert.equal(summary.outcome, 'failure')
  assert.equal(summary.headline, 'Some checks were not successful')
  assert.equal(summary.detail, '1 failing, 1 in progress, 2 successful, 1 skipped checks')
  assert.equal(checksSummary([check('a', 'success'), check('b', 'skipped')]).headline, 'All checks have passed')
  assert.equal(checksSummary([check('a', 'success')]).detail, '1 successful check')
  assert.equal(checksSummary([check('a', 'pending')]).outcome, 'pending')
  assert.equal(checksSummary([]).headline, 'No checks')
  const ring = ringSegments(checks)
  assert.deepEqual(
    ring.map((s) => s.status),
    ['failure', 'pending', 'success', 'skipped'],
  )
  assert.equal(ring.at(-1).start + ring.at(-1).size, 1)
})

test('polling is faster while checks run', () => {
  assert.equal(refreshDelay({ checks: [check('a', 'pending')] }), 20000)
  assert.equal(refreshDelay({ checks: [check('a', 'success')] }), 120000)
  assert.equal(refreshDelay(undefined), 120000)
})

test('activity keeps meaningful reviews and comments in time order', () => {
  const items = pullRequestActivity({
    reviews: [
      { id: 1, state: 'APPROVED', body: '', submitted_at: '2026-10-21T12:00:00Z', user: { login: 'a' } },
      { id: 2, state: 'COMMENTED', body: '  ', submitted_at: '2026-10-21T10:00:00Z' },
      { id: 3, state: 'PENDING', body: 'draft', submitted_at: '2026-10-21T10:00:00Z' },
      { id: 4, state: 'CHANGES_REQUESTED', body: 'Fix it', submitted_at: '2026-10-21T09:00:00Z' },
    ],
    comments: [
      { id: 5, body: 'Thanks', created_at: '2026-10-21T11:00:00Z' },
      { id: 6, body: '', created_at: '2026-10-21T11:00:00Z' },
    ],
  })
  assert.deepEqual(
    items.map((item) => [item.id, item.label]),
    [
      ['review-4', 'Requested changes'],
      ['comment-5', 'Commented'],
      ['review-1', 'Approved'],
    ],
  )
  assert.deepEqual(activityCounts(items), { approved: 1, changes: 1, comments: 1 })
  assert.deepEqual(pullRequestActivity(undefined), [])
})

test('failure context names the pull request and each failed check', () => {
  const lint = check('lint', 'failure', { workflow: 'CI', url: 'https://example.invalid/lint' })
  const build = check('build', 'failure', { jobID: 9 })
  const text = failureContext(
    { number: 7, title: 'Fix', url: 'https://github.com/o/r/pull/7', branch: 'feature' },
    [lint, build],
    new Map([[build, 'Check: build\nLog:\n```\nerror\n```']]),
  )
  assert.equal(
    text,
    'Pull request #7: Fix\nhttps://github.com/o/r/pull/7\nBranch: feature\n\n---\n\n' +
      'Check: CI / lint\nStatus: failure\nURL: https://example.invalid/lint\n\n---\n\n' +
      'Check: build\nLog:\n```\nerror\n```',
  )
})
