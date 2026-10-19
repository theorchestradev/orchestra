import test from 'node:test'
import assert from 'node:assert/strict'
import { shellWord, shellPaths } from '../src/shared/fileDrop.js'

test('plain paths are typed as they are', () => {
  assert.equal(shellWord('/Users/me/code/orchestra/main.go'), '/Users/me/code/orchestra/main.go')
  assert.equal(shellWord('/tmp/a-b_c+d=e,f:g@h%i'), '/tmp/a-b_c+d=e,f:g@h%i')
})

test('other paths are single-quoted', () => {
  assert.equal(shellWord('/Users/me/My Files/a.txt'), "'/Users/me/My Files/a.txt'")
  assert.equal(shellWord("/tmp/it's"), "'/tmp/it'\\''s'")
  assert.equal(shellWord('/tmp/$HOME `x` *'), "'/tmp/$HOME `x` *'")
  assert.equal(shellWord('/tmp/a\nb'), "'/tmp/a\nb'")
  assert.equal(shellWord('/tmp/café'), "'/tmp/café'")
})

test('dropped paths are separated and followed by a space', () => {
  assert.equal(shellPaths(['/a', '/b c']), "/a '/b c' ")
  assert.equal(shellPaths([]), '')
})
