// reconcileScratchpad decides what an open Scratchpad editor shows when the stored
// Scratchpad changes. local is the editor's text, base the stored text and revision
// the editor last matched, and submitted the texts the editor sent but has not seen
// confirmed. It returns null when incoming is not newer than base.
export function reconcileScratchpad(local, base, incoming, submitted = []) {
  if (incoming.revision <= base.revision) return null
  if (incoming.text === local || submitted.includes(incoming.text))
    return { text: local, base: incoming, conflict: false }
  if (local === base.text) return { text: incoming.text, base: incoming, conflict: false }
  // Text added at the end, such as orchestra scratchpad append, merges after local edits.
  if (incoming.text.startsWith(base.text))
    return { text: local + incoming.text.slice(base.text.length), base: incoming, conflict: false }
  return { text: local, base: incoming, conflict: true }
}
