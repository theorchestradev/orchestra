// IDs are persisted; missing preferences keep every existing indicator visible.
export const statusBarGroups = [
  {
    name: 'Left',
    items: [
      ['workspace', 'Workspace'],
      ['repository', 'Repository'],
      ['session', 'Session'],
      ['profile', 'Account profile'],
      ['activity', 'Session activity'],
      ['branch', 'Branch'],
      ['exitMessage', 'Exit message'],
      ['view', 'Current view'],
    ],
  },
  {
    name: 'Right',
    items: [
      ['working', 'Working sessions'],
      ['waiting', 'Waiting sessions'],
      ['running', 'Running sessions'],
      ['update', 'Available update'],
      ['inbox', 'Inbox'],
      ['usage', 'Agent usage'],
    ],
  },
]
export const statusBarItemVisible = (preferences, id) =>
  !(id === 'usage' && preferences.hideUsageStatus) && !(preferences.hiddenStatusBarItems || []).includes(id)
export function statusBarItemChange(preferences, id) {
  const hidden = new Set(preferences.hiddenStatusBarItems || [])
  if (statusBarItemVisible(preferences, id)) hidden.add(id)
  else hidden.delete(id)
  return { ...preferences, hiddenStatusBarItems: [...hidden], ...(id === 'usage' ? { hideUsageStatus: false } : {}) }
}

// A saved layout contains both sides, including hidden items so showing them keeps their position.
export function statusBarLayout(preferences) {
  const known = new Map(statusBarGroups.flatMap((group) => group.items))
  const seen = new Set()
  const saved = preferences.statusBarLayout
  const groups = statusBarGroups.map((group, side) => ({
    name: group.name,
    items: (saved?.length === 2 && Array.isArray(saved[side]) ? saved[side] : group.items.map(([id]) => id))
      .filter((id) => {
        if (!known.has(id) || seen.has(id)) return false
        seen.add(id)
        return true
      })
      .map((id) => [id, known.get(id)]),
  }))
  // New indicators join their default side when loading a layout from an older version.
  statusBarGroups.forEach((group, side) => {
    for (const item of group.items) if (!seen.has(item[0])) groups[side].items.push(item)
  })
  return groups
}
export function moveStatusBarItem(preferences, id, side, index) {
  const layout = statusBarLayout(preferences).map((group) => group.items.map(([key]) => key))
  if (!layout.some((items) => items.includes(id)) || (side !== 0 && side !== 1)) return preferences
  for (const items of layout) {
    const previous = items.indexOf(id)
    if (previous !== -1) items.splice(previous, 1)
  }
  layout[side].splice(Math.max(0, Math.min(index, layout[side].length)), 0, id)
  return { ...preferences, statusBarLayout: layout }
}
