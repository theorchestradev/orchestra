import data from './emojiData.js'
import keywordData from './emojiKeywords.js'

const icons = ['😀', '👋', '🐶', '🍔', '✈️', '⚽', '💡', '🔣', '🏁']

export const emojiGroups = data.map(([name, list], i) => ({
  name,
  icon: icons[i],
  emojis: list.split('|').map((item) => {
    const space = item.indexOf(' ')
    return { emoji: item.slice(0, space), name: item.slice(space + 1) }
  }),
}))

// Lowercase, without accents or punctuation, so “cote” finds Côte d’Ivoire and “womans” finds woman’s hat.
const words = (text) =>
  text
    .toLowerCase()
    .normalize('NFD')
    .replace(/\p{M}/gu, '')
    .replace(/[’']/g, '')
    .split(/[^a-z0-9#*]+/)
    .filter(Boolean)

const keywords = new Map(
  keywordData.split('|').map((item) => {
    const space = item.indexOf(' ')
    return [item.slice(0, space), item.slice(space + 1).split(' ')]
  }),
)

const all = emojiGroups.flatMap((g) =>
  g.emojis.map((e) => {
    const name = words(e.name)
    return { emoji: e.emoji, name: e.name, nameWords: name, words: name.concat(keywords.get(e.emoji) || []) }
  }),
)

const matches = (list, terms) => terms.every((t) => list.some((w) => w.startsWith(t)))

// Every word typed must start a word of the emoji's name or CLDR keywords, so “cat” finds 🐈 but not
// 🏏 cricket game, and “smile” or “happy” find 😀. Matches on the name come first.
export function searchEmoji(query) {
  const terms = words(query)
  if (!terms.length) return []
  const found = all.filter((e) => matches(e.words, terms))
  return [
    ...found.filter((e) => matches(e.nameWords, terms)),
    ...found.filter((e) => !matches(e.nameWords, terms)),
  ].map(({ emoji, name }) => ({ emoji, name }))
}

// Index of the group holding an emoji, so the picker opens on the current choice.
export const emojiGroupIndex = (emoji) =>
  Math.max(
    0,
    emojiGroups.findIndex((g) => g.emojis.some((e) => e.emoji === emoji)),
  )
