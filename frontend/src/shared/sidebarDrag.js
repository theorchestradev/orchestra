// Move a complete sidebar item and open a matching gap between its siblings.
export function sidebarDragLayout(rows, from, delta) {
  const source = rows[from],
    height = source.bottom - source.top
  const y = Math.max(rows[0].top - source.top, Math.min(delta, rows.at(-1).bottom - source.bottom))
  const positions = rows.map((row, i) => (i < from ? row.top - source.top : i > from ? row.bottom - source.bottom : 0))
  let index = 0
  while (index < rows.length - 1 && y > (positions[index] + positions[index + 1]) / 2) index++
  const gap =
    rows.length < 2
      ? 0
      : from < rows.length - 1
        ? rows[from + 1].top - source.bottom
        : source.top - rows[from - 1].bottom
  const offsets = rows.map((_, i) =>
    i >= index && i < from ? height + gap : i > from && i <= index ? -height - gap : 0,
  )
  const settleY = positions[index]
  return { y, index, offsets, settleY }
}
