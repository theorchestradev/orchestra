import { ref, shallowRef } from 'vue'
import { mac } from './platform.js'
import themes from './themes.json' with { type: 'json' }
import terminalColors from './terminalColors.json' with { type: 'json' }
export const themeNames = Object.entries(themes).map(([id, { name }]) => ({ id, name }))
// Themes from ~/.orchestra/themes, validated by the Go side. IDs start with `custom:`, so they
// never replace a built-in theme.
export const customThemes = shallowRef([])
export function setCustomThemes(list) {
  customThemes.value = Array.isArray(list) ? list : []
}
// Built-in themes first, then custom themes marked with `custom: true`.
export function themeOptions() {
  return [...themeNames, ...customThemes.value.map(({ id, name }) => ({ id, name, custom: true }))]
}
const isCustomTheme = (id) => typeof id === 'string' && id.startsWith('custom:')
function themeFor(id) {
  return (isCustomTheme(id) ? customThemes.value.find((t) => t.id === id) : themes[id]) || themes.system
}
// A selected custom theme that is no longer available falls back to System.
export function missingCustomTheme(id) {
  return isCustomTheme(id) && !customThemes.value.some((t) => t.id === id)
}
// System follows prefers-color-scheme, which on macOS tracks the OS while the window appearance is
// unset. WebKitGTK does not follow the desktop setting reliably, so System shows Dark elsewhere.
const systemQuery = mac ? globalThis.matchMedia?.('(prefers-color-scheme: dark)') : null
export const systemAppearance = ref(systemQuery && !systemQuery.matches ? 'light' : 'dark')
systemQuery?.addEventListener?.('change', (e) => (systemAppearance.value = e.matches ? 'dark' : 'light'))
// The appearance shown for the "system", "dark" or "light" preference. Views receive this value,
// so only Settings knows about System.
export function effectiveAppearance(appearance, system = systemAppearance.value) {
  return appearance === 'system' ? system : appearance === 'light' ? 'light' : 'dark'
}
export const tokens = ['canvas', 'sidebar', 'surface', 'text', 'muted', 'rule', 'accent', 'selection']
// Default terminal colors for themes without `ansi`, and for every theme in light mode.
const { dark: darkANSI, light: lightANSI } = terminalColors
// Colors for Duplicate as Custom Theme: System uses the native palettes in use, and a theme
// without `ansi` writes the default dark terminal colors so the copy can edit them.
export function themeSource(id) {
  const theme = themeFor(id)
  return { name: theme.name, light: theme.light, dark: theme.dark, ansi: theme.ansi || darkANSI }
}
export function palette(preferences) {
  const theme = themeFor(preferences.theme)
  return Object.fromEntries(
    tokens.map((key, i) => [key, theme[preferences.appearance === 'light' ? 'light' : 'dark'][i]]),
  )
}
// Terminals put the chosen family first, so a missing font falls back to System Monospace.
export const systemMonospace = 'ui-monospace, SFMono-Regular, Menlo, Monaco, monospace'
export const terminalFontFamily = (family) => (family ? `"${family}", ${systemMonospace}` : systemMonospace)
// A translucent window lets the vibrancy show through the terminal; the block cursor keeps the
// opaque canvas behind its character.
export function terminalTheme(preferences, translucent = false) {
  const p = palette(preferences),
    theme = themeFor(preferences.theme)
  const ansi = preferences.appearance === 'light' ? lightANSI : theme.ansi || darkANSI
  return {
    ...Object.fromEntries(
      [
        'black',
        'red',
        'green',
        'yellow',
        'blue',
        'magenta',
        'cyan',
        'white',
        'brightBlack',
        'brightRed',
        'brightGreen',
        'brightYellow',
        'brightBlue',
        'brightMagenta',
        'brightCyan',
        'brightWhite',
      ].map((key, i) => [key, ansi[i]]),
    ),
    background: translucent ? '#00000000' : p.canvas,
    foreground: p.text,
    cursor: p.text,
    cursorAccent: p.canvas,
    selectionBackground: p.selection,
  }
}
// Added and removed lines use the theme's terminal green and red, so every theme and mode keeps
// its own diff colors. Line and gutter backgrounds mix these with `canvas` in style.css.
// Window Opacity applies to the content area. The sidebar, Changes pane and status bar keep the
// default 62:78 ratio, so they stay more transparent than the content. The range matches Store.
export const windowOpacityRange = { min: 40, max: 95, default: 78 }
export function translucencyTokens(opacity) {
  const { min, max } = windowOpacityRange,
    canvas = Math.min(max, Math.max(min, Math.round(Number(opacity) || windowOpacityRange.default)))
  return { '--canvas-opacity': canvas + '%', '--sidebar-opacity': Math.round((canvas * 62) / 78) + '%' }
}
export function diffTokens(preferences) {
  const { green, red } = terminalTheme(preferences)
  return { '--diff-added': green, '--diff-removed': red }
}
export const agents = [
  { id: 'codex', name: 'Codex', icon: 'CodexIcon', dark: true },
  { id: 'claude', name: 'Claude Code', icon: 'ClaudeIcon' },
  { id: 'pi', name: 'Pi', icon: 'PiIcon' },
  { id: 'vibe', name: 'Mistral Vibe', icon: 'MistralIcon' },
  { id: 'opencode', name: 'OpenCode', icon: 'OpenCodeIcon', dark: true },
  { id: 'antigravity', name: 'Antigravity CLI', icon: 'AntigravityIcon' },
  { id: 'grok', name: 'Grok Build', icon: 'GrokIcon', dark: true },
]
// Plain terminal sessions run the login shell; they are not agents and stay out of `agents`.
export const isTerminal = (session) => session?.agent === 'terminal'
export function agentIcon(id, appearance) {
  const agent = agents.find((a) => a.id === id) || agents[0]
  return `/agents/${agent.icon}${agent.dark && appearance === 'dark' ? '-dark' : ''}.svg`
}
export function destination(project) {
  return project?.showsScratchpad
    ? 'scratchpad'
    : project?.showsGitHub
      ? 'github'
      : project?.selectedSessionID || 'empty'
}
// Arc spaces show only the active project, including a previously collapsed project.
export function visibleSidebarProjects(projects, selectedID, arcMode) {
  if (!arcMode) return projects
  const selected = projects.find((p) => p.id === selectedID) || projects[0]
  return selected ? [selected] : []
}
// A native horizontal scroll snaps to the closest space; rubber-band offsets stay bounded.
export function arcScrollDelta(event, width) {
  if (event.ctrlKey || (!event.shiftKey && Math.abs(event.deltaX) <= Math.abs(event.deltaY))) return 0
  return (
    (event.deltaX || (event.shiftKey ? event.deltaY : 0)) *
    (event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? width : 1)
  )
}
// Reorderable rows under Issues & PRs: sessions with the Scratchpad at its saved position.
export function projectRows(p) {
  const rows = [...(p.sessions || [])]
  if (p.hasScratchpad)
    rows.splice(Math.max(0, Math.min(p.scratchpadIndex || 0, rows.length)), 0, { id: 'scratchpad', scratchpad: true })
  return rows
}
export function sidebarItems(projects) {
  return (projects || [])
    .filter((p) => p.isExpanded)
    .flatMap((p) =>
      [...(p.repository ? ['github'] : []), ...projectRows(p).map((s) => s.id)].map((destination) => ({
        project: p,
        destination,
      })),
    )
}
export function adjacentItem(projects, projectID, current, step) {
  const items = sidebarItems(projects)
  if (!items.length) return null
  const index = items.findIndex((i) => i.project.id === projectID && i.destination === current)
  if (index < 0) return items[step > 0 ? 0 : items.length - 1]
  return items[(index + step + items.length) % items.length]
}
// Final index for moving id before or after target, or -1 when the order would not change.
export function dropIndex(ids, id, target, after) {
  const from = ids.indexOf(id),
    over = ids.indexOf(target)
  if (from < 0 || over < 0) return -1
  const index = over + (after ? 1 : 0) - (from < over + (after ? 1 : 0) ? 1 : 0)
  return index === from ? -1 : index
}
export function runLabel(status) {
  return (
    { queued: 'In inbox', running: 'Awaiting reply', needsAttention: 'Needs attention' }[status] ||
    status[0].toUpperCase() + status.slice(1)
  )
}
export function relativeDate(value) {
  const time = typeof value === 'number' ? (value + 978307200) * 1000 : Date.parse(value)
  const diff = Math.round((time - Date.now()) / 1000)
  if (!Number.isFinite(diff)) return ''
  const formatter = new Intl.RelativeTimeFormat('en', { numeric: 'auto' })
  for (const [unit, n] of [
    ['year', 31536000],
    ['month', 2592000],
    ['day', 86400],
    ['hour', 3600],
    ['minute', 60],
  ])
    if (Math.abs(diff) >= n) return formatter.format(Math.round(diff / n), unit)
  return formatter.format(diff, 'second')
}

export function setSystemPalettes(values) {
  for (const mode of ['dark', 'light']) if (values?.[mode]?.length === 8) themes.system[mode] = values[mode]
}

// Foreground for primary controls, shared across every theme.
export function accentForeground(color) {
  const rgb = color
    .replace('#', '')
    .match(/../g)
    ?.slice(0, 3)
    .map((x) => {
      const c = parseInt(x, 16) / 255
      return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
    }) || [0, 0, 0]
  return rgb[0] * 0.2126 + rgb[1] * 0.7152 + rgb[2] * 0.0722 > 0.45 ? '#000000' : '#FFFFFF'
}

export function startupDestination(project, preferences) {
  if (!project) return null
  if (preferences.startupView === 'scratchpad') return 'scratchpad'
  if (preferences.startupView === 'github' && project.repository) return 'github'
  return null
}
export function activityLabel(activity) {
  return (
    { waiting: 'Waiting for input', working: 'Agent working', ended: 'Session ended' }[activity] || 'Status unavailable'
  )
}
export function statusSummary(projects, runs) {
  const sessions = (projects || []).flatMap((p) => p.sessions || []).filter((s) => s.running)
  const count = (status) => (runs || []).filter((r) => r.status === status).length
  return {
    running: sessions.length,
    working: sessions.filter((s) => s.activity === 'working').length,
    waiting: sessions.filter((s) => s.activity === 'waiting').length,
    queued: count('queued'),
    attention: count('needsAttention'),
  }
}
