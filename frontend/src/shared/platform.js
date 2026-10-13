// macOS uses its native title bar inset and ⌘ shortcuts. Other platforms keep the window
// manager's title bar and use Ctrl, with Ctrl+Shift inside terminals.
export const mac = /Mac/.test(globalThis.navigator?.platform || '')

// Shortcut labels are written in macOS notation, for example '⇧⌘B'.
export function shortcutLabel(text, isMac = mac) {
  if (isMac) return text
  const key = text.replace(/[⇧⌘]/g, '').replace('↩', 'Enter')
  return 'Ctrl+' + (text.includes('⇧') ? 'Shift+' : '') + key
}
