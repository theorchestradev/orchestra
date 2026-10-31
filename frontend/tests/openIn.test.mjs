import test from 'node:test'
import assert from 'node:assert/strict'
import { sessionPath, worktreeName, lastApp, appGroups } from '../src/shared/openIn.js'

test('worktree labels use their branch or folder name, and repository folders have no label', () => {
  assert.equal(worktreeName({ worktreeDirectory: '/trees/abc', worktreeBranch: 'feature/files' }), 'feature/files')
  assert.equal(worktreeName({ worktreeDirectory: '/trees/abc/' }), 'abc')
  assert.equal(worktreeName({ worktreeBranch: 'main' }), '')
  assert.equal(worktreeName(), '')
})

test('sessions open their worktree, or their repository folder without one', () => {
  const workspace = {
    repositories: [
      { id: 'app', directory: '/code/orchestra' },
      { id: 'site', directory: '/code/site' },
    ],
  }
  assert.equal(
    sessionPath(workspace, { repositoryID: 'site', worktreeDirectory: '/home/me/.orchestra/worktrees/site/1' }),
    '/home/me/.orchestra/worktrees/site/1',
  )
  assert.equal(sessionPath(workspace, { repositoryID: 'app' }), '/code/orchestra')
  assert.equal(sessionPath(workspace, { repositoryID: 'site' }), '/code/site')
  assert.equal(sessionPath(workspace, { repositoryID: 'removed' }), '')
})

test('the last app is the default only while it is installed', () => {
  const apps = [{ id: 'zed', name: 'Zed' }]
  assert.equal(lastApp(apps, 'zed'), apps[0])
  assert.equal(lastApp(apps, 'vscode'), null)
  assert.equal(lastApp(apps, ''), null)
  assert.equal(lastApp(undefined, 'zed'), null)
})

test('menus group editors, terminals and Git tools, leaving the file manager to Copy Path', () => {
  const apps = [
    { id: 'zed', kind: 'editor' },
    { id: 'fork', kind: 'git' },
    { id: 'vscode', kind: 'editor' },
    { id: 'files', kind: 'files' },
  ]
  assert.deepEqual(
    appGroups(apps).map((g) => [g.label, g.apps.map((a) => a.id)]),
    [
      ['Editors', ['zed', 'vscode']],
      ['Git Tools', ['fork']],
    ],
  )
  assert.deepEqual(appGroups([]), [])
})
