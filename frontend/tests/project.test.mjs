import test from 'node:test'
import assert from 'node:assert/strict'
import { projectTitle, spaceSymbol } from '../src/shared/project.js'
import { emojiGroupIndex, emojiGroups, searchEmoji } from '../src/shared/emoji.js'

test('project titles show the emoji before the name', () => {
  assert.equal(projectTitle({ name: 'Orchestra', emoji: '🎻' }), '🎻 Orchestra')
  assert.equal(projectTitle({ name: 'Orchestra' }), 'Orchestra')
  assert.equal(projectTitle(null), '')
})

test('Arc Mode spaces prefer the emoji over the initial', () => {
  assert.equal(spaceSymbol({ name: 'site', emoji: '👨‍💻' }), '👨‍💻')
  assert.equal(spaceSymbol({ name: ' site' }), 'S')
  assert.equal(spaceSymbol({ name: '' }), '•')
})

test('the emoji picker groups and searches emoji by name', () => {
  assert.equal(emojiGroups.length, 9)
  assert.ok(emojiGroups.every((g) => g.icon && g.emojis.length))
  assert.deepEqual(emojiGroups[0].emojis[0], { emoji: '😀', name: 'grinning face' })
  const cats = searchEmoji('cat face').map((e) => e.emoji)
  assert.ok(cats.includes('🐱'))
  assert.ok(!searchEmoji('cat').some((e) => e.name === 'cricket game'))
  assert.equal(searchEmoji('  ').length, 0)
  assert.ok(searchEmoji('Brazil').some((e) => e.emoji === '🇧🇷'))
  // Typing a full word keeps what its prefix found, through CLDR keywords, accents and apostrophes.
  assert.ok(searchEmoji('smil').some((e) => e.emoji === '😄'))
  assert.ok(searchEmoji('smile').some((e) => e.emoji === '😄'))
  assert.ok(searchEmoji('happy').some((e) => e.emoji === '😀'))
  assert.equal(searchEmoji('tada')[0].emoji, '🎉')
  assert.ok(searchEmoji('cote').some((e) => e.emoji === '🇨🇮'))
  assert.ok(searchEmoji('womans hat').some((e) => e.emoji === '👒'))
  assert.equal(emojiGroups[emojiGroupIndex('🇧🇷')].name, 'Flags')
  assert.equal(emojiGroupIndex('x'), 0)
})
