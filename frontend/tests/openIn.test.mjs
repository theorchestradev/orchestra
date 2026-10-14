import test from 'node:test'
import assert from 'node:assert/strict'
import { sessionPath, lastApp, appGroups } from '../src/shared/openIn.js'

test('sessions open their worktree, or the project folder without one', () => {
  const project = { directory: '/code/orchestra' }
  assert.equal(
    sessionPath(project, { worktreeDirectory: '/home/me/.orchestra/worktrees/orchestra/1' }),
    '/home/me/.orchestra/worktrees/orchestra/1',
  )
  assert.equal(sessionPath(project, {}), '/code/orchestra')
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
