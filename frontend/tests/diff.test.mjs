import test from 'node:test'
import assert from 'node:assert/strict'
import { parsePatch, splitPath, clampDiffWidth, diffPlaceholder } from '../src/shared/diff.js'
import { diffTokens, terminalTheme } from '../src/shared/appearance.js'

test('patches parse into hunks with old and new line numbers', () => {
  const hunks = parsePatch(
    '@@ -1,3 +1,3 @@ heading\n one\n-two\n+2\n three\n@@ -10 +10,2 @@\n ten\n+eleven\n\\ No newline at end of file\n',
  )
  assert.equal(hunks.length, 2)
  assert.equal(hunks[0].header, '@@ -1,3 +1,3 @@ heading')
  assert.deepEqual(
    hunks[0].lines.map((l) => [l.kind, l.text, l.old, l.new]),
    [
      ['context', 'one', 1, 1],
      ['del', 'two', 2, null],
      ['add', '2', null, 2],
      ['context', 'three', 3, 3],
    ],
  )
  assert.deepEqual(
    hunks[1].lines.map((l) => [l.kind, l.old, l.new]),
    [
      ['context', 10, 10],
      ['add', null, 11],
      ['note', null, null],
    ],
  )
  assert.deepEqual(parsePatch(''), [])
  assert.equal(parsePatch('@@ -1 +1 @@\n \n').at(0).lines[0].text, '')
})

test('binary, truncated and renamed files explain why there is no patch', () => {
  assert.equal(diffPlaceholder({ binary: true }), 'Binary file not shown.')
  assert.equal(diffPlaceholder({ truncated: true }), 'This file is too large to show.')
  assert.equal(diffPlaceholder({ status: 'R' }), 'Renamed without content changes.')
})

test('paths split into folder and file name, and widths stay in range', () => {
  assert.deepEqual(splitPath('frontend/src/App.vue'), { folder: 'frontend/src/', name: 'App.vue' })
  assert.deepEqual(splitPath('README.md'), { folder: '', name: 'README.md' })
  assert.equal(clampDiffWidth(100), 280)
  assert.equal(clampDiffWidth(5000), 900)
  assert.equal(clampDiffWidth(440.4), 440)
})

test('diff colors follow each theme and appearance', () => {
  for (const preferences of [
    { theme: 'system', appearance: 'dark' },
    { theme: 'dracula', appearance: 'dark' },
    { theme: 'github', appearance: 'light' },
  ]) {
    const terminal = terminalTheme(preferences)
    assert.deepEqual(diffTokens(preferences), { '--diff-added': terminal.green, '--diff-removed': terminal.red })
  }
  assert.notEqual(
    diffTokens({ theme: 'dracula', appearance: 'dark' })['--diff-added'],
    diffTokens({ theme: 'system', appearance: 'light' })['--diff-added'],
  )
})
