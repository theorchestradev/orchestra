// Helpers for the Changes pane. Patches come from SessionDiff and start at the first hunk.
export const statusLabels = { A: 'Added', M: 'Modified', D: 'Deleted', R: 'Renamed' }
export const minDiffWidth = 280,
  maxDiffWidth = 900

// Hunks with their lines. Context and removed lines carry old numbers; context and added lines new ones.
export function parsePatch(patch = '') {
  const hunks = []
  let hunk,
    oldLine = 0,
    newLine = 0
  for (const text of patch.split('\n')) {
    const header = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(text)
    if (header) {
      oldLine = Number(header[1])
      newLine = Number(header[2])
      hunk = { header: text, lines: [] }
      hunks.push(hunk)
    } else if (hunk && text) {
      const kind = { '+': 'add', '-': 'del', '\\': 'note' }[text[0]] || 'context'
      hunk.lines.push({
        kind,
        text: kind === 'note' ? text : text.slice(1),
        old: kind === 'context' || kind === 'del' ? oldLine++ : null,
        new: kind === 'context' || kind === 'add' ? newLine++ : null,
      })
    }
  }
  return hunks
}

// The folder part shrinks first so long paths keep their file name.
export function splitPath(path) {
  const index = path.lastIndexOf('/') + 1
  return { folder: path.slice(0, index), name: path.slice(index) }
}

export function clampDiffWidth(width) {
  return Math.round(Math.max(minDiffWidth, Math.min(maxDiffWidth, width)))
}

export function diffPlaceholder(file) {
  if (file.binary) return 'Binary file not shown.'
  if (file.truncated) return 'This file is too large to show.'
  if (file.status === 'R' && !file.patch) return 'Renamed without content changes.'
  return 'No content changes.'
}
