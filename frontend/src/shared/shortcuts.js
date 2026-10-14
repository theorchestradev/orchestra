import { mac, shortcutLabel } from './platform.js'

// Every keyboard shortcut, in macOS notation. Settings → Shortcuts and the command bar read
// this list. `app` entries are matched by `appCommand` and run by App.vue as commands of the
// same id. Off macOS, Ctrl+B, Ctrl+K and Ctrl+N belong to the terminal, so `terminal` gives
// the form used inside terminals there, and null means the shortcut does not work in one.
// `inTerminal` entries only apply inside terminals.
export const shortcuts = [
  { id: 'command', group: 'General', name: 'Command Bar', keys: '⌘K', terminal: '⇧⌘K', app: true },
  { id: 'session', group: 'General', name: 'New Session', keys: '⌘N', terminal: null, app: true },
  { id: 'project', group: 'General', name: 'New Project', keys: '⇧⌘N', app: true },
  { id: 'settings', group: 'General', name: 'Settings', keys: '⌘,', app: true },
  { id: 'dismiss', group: 'General', name: 'Close Dialog or Menu', keys: 'Esc' },
  { id: 'previous', group: 'Navigation', name: 'Previous Item', keys: '⇧⌘[', app: true },
  { id: 'next', group: 'Navigation', name: 'Next Item', keys: '⇧⌘]', app: true },
  { id: 'contextMenu', group: 'Navigation', name: 'Open Context Menu', keys: '⇧F10' },
  { id: 'sidebar', group: 'View', name: 'Toggle Sidebar', keys: '⌘B', terminal: null, app: true },
  { id: 'diff', group: 'View', name: 'Toggle Changes', keys: '⇧⌘B', app: true },
  { id: 'copy', group: 'Terminal', name: 'Copy', keys: '⌘C', terminal: '⇧⌘C', inTerminal: true },
  { id: 'paste', group: 'Terminal', name: 'Paste', keys: '⌘V', terminal: '⇧⌘V', inTerminal: true },
  { id: 'scratchpadMode', group: 'Scratchpad', name: 'Switch Between Edit and Read', keys: '⌘E' },
  { id: 'githubBack', group: 'Issues & Pull Requests', name: 'Back to the List', keys: '⌘[' },
  { id: 'saveDescription', group: 'Issues & Pull Requests', name: 'Save Description', keys: '⌘↩' },
  { id: 'cancelEdit', group: 'Issues & Pull Requests', name: 'Cancel Editing', keys: 'Esc' },
  { id: 'sendRequest', group: 'Messages', name: 'Send Request', keys: '⌘↩' },
]

export function shortcut(id) {
  return shortcuts.find((s) => s.id === id)?.keys
}

const keyNames = { '[': 'Left Bracket', ']': 'Right Bracket', ',': 'Comma', Esc: 'Escape' }

// Key labels for the current platform, each with a name a screen reader can read.
export function shortcutKeys(entry, isMac = mac) {
  const key = (text, note = '') => {
    const base = text.replace(/[⇧⌘]/g, '')
    const shift = text.includes('⇧') && 'Shift',
      command = text.includes('⌘') && (isMac ? 'Command' : 'Control')
    const name = [
      ...(isMac ? [shift, command] : [command, shift]),
      keyNames[base] || (base === '↩' ? (isMac ? 'Return' : 'Enter') : base),
    ]
    return { label: shortcutLabel(text, isMac), name: name.filter(Boolean).join(' '), note }
  }
  if (isMac || entry.terminal === undefined) return [key(entry.keys)]
  if (entry.inTerminal) return [key(entry.terminal)]
  if (entry.terminal === null) return [key(entry.keys, 'Outside terminals')]
  return [key(entry.keys), key(entry.terminal, 'In a terminal')]
}

// The app command for a key event, matching the macOS menu in main.go. Both ⌘ and Ctrl work.
export function appCommand(event) {
  if (!event.metaKey && !event.ctrlKey) return null
  if (event.shiftKey && ['BracketLeft', 'BracketRight'].includes(event.code))
    return event.code === 'BracketRight' ? 'next' : 'previous'
  if (event.shiftKey && event.code === 'KeyB') return 'diff'
  const key = event.key.toLowerCase()
  if (key === 'b' && !event.shiftKey) return 'sidebar'
  if (key === 'k') return 'command'
  if (key === ',') return 'settings'
  if (key === 'n') return event.shiftKey ? 'project' : 'session'
  return null
}

// Whether a terminal keeps a key event instead of leaving it to App.vue. Off macOS, Ctrl+Shift+C
// and Ctrl+V are copy and paste, which Terminal.vue handles before this.
export function terminalKeeps(e, isMac = mac) {
  if (isMac)
    return !(
      e.metaKey &&
      (['n', 'k', ',', 'b'].includes(e.key.toLowerCase()) ||
        (e.shiftKey && ['KeyB', 'BracketLeft', 'BracketRight'].includes(e.code)))
    )
  if (!e.ctrlKey) return true
  if (!e.shiftKey) return e.key !== ','
  return !['KeyN', 'KeyK', 'KeyB', 'BracketLeft', 'BracketRight'].includes(e.code)
}
