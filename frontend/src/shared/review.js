// Comments on the Changes pane. A comment anchors to lines on one side of a file's diff:
// "new" numbers added and context lines, "old" numbers removed ones. Its excerpt is the
// text of those lines, so a later diff can tell whether they changed.
const maxExcerptBytes = 4000

export const lineSide = (line) => (line.new != null ? 'new' : 'old')
export const lineNumber = (line) => line.new ?? line.old
export const commentable = (line) => line.kind !== 'note'

// The commentable lines of hunks on one side, from start to end.
export function linesInRange(hunks, side, start, end) {
  const lines = []
  for (const hunk of hunks)
    for (const line of hunk.lines) {
      if (!commentable(line) || lineSide(line) !== side) continue
      const n = lineNumber(line)
      if (n >= start && n <= end) lines.push(line)
    }
  return lines
}

export function excerpt(hunks, side, start, end) {
  let text = linesInRange(hunks, side, start, end)
    .map((l) => l.text)
    .join('\n')
  const bytes = new TextEncoder().encode(text)
  if (bytes.length > maxExcerptBytes) text = new TextDecoder().decode(bytes.slice(0, maxExcerptBytes)).replace(/�$/, '')
  return text
}

// A comment is outdated once its file left the diff or its lines no longer match the excerpt.
export function outdated(comment, file) {
  if (!file?.hunks.length) return true
  const current = excerpt(file.hunks, comment.side, comment.start, comment.end)
  return !current || current !== (comment.excerpt || current)
}

export function rangeLabel({ side, start, end }) {
  return (start === end ? `Line ${start}` : `Lines ${start}–${end}`) + (side === 'old' ? ' (removed)' : '')
}

// The reviews sent to a session that the Changes pane shows, newest first. Cancelled
// reviews without any reply are left to Messages.
export function sessionReviews(runs, sessionID) {
  return runs
    .filter(
      (r) =>
        r.kind === 'review' &&
        r.toSessionID === sessionID &&
        !r.dismissed &&
        !(r.status === 'cancelled' && !(r.comments || []).some((c) => c.reply)),
    )
    .toReversed()
}

export function reviewStatus(run) {
  return (
    {
      queued: 'Waiting to be sent',
      running: 'Waiting for replies',
      completed: 'Replied',
      cancelled: 'Stopped waiting',
      interrupted: 'Interrupted',
      needsAttention: 'Needs attention',
    }[run.status] || run.status
  )
}
