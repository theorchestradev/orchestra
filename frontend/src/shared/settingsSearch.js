import { shortcuts } from './shortcuts.js'
import { statusBarGroups } from './statusBar.js'

// Search destinations for controls in Settings. Keep labels aligned with the visible controls.
export const settingsSearchItems = [
  ['General', 'Startup View', 'startup-view'],
  ['General', 'Dismiss Errors Manually', 'manualErrorDismiss'],
  ['General', 'Keep Sessions Running After Quitting', 'keepSessionsRunning'],
  ['General', 'Keep Computer Awake While Sessions Run', 'keepAwake'],
  ['General', 'GitHub Auto Refresh', 'github-refresh'],
  ['General', 'Check for Updates Automatically', 'updateChecks'],
  ['General', 'Include Beta Versions', 'betaUpdates'],
  ['Agents', 'Default Agent', 'default-agent'],
  ['Agents', 'Orchestra Skill: Install or Update, Copy Command', 'agent-skill-heading'],
  ['Agents', 'Deliver Inbox Requests Automatically', 'deliverRequests'],
  ['Agents', 'Let Agents Start Sessions', 'agentsStartSessions'],
  ['Agents', 'Play Sound When an Agent Needs You', 'attentionSound'],
  ['Source Control', 'Keep Local Main Up to Date', 'keepLocalMainUpdated'],
  ['Appearance', 'Theme', 'theme'],
  ['Appearance', 'Rounded Viewport', 'rounded-viewport'],
  ['Appearance', 'Translucent Window', 'translucent'],
  ['Appearance', 'Opacity', 'window-opacity'],
  ['Appearance', 'Arc Mode', 'arc-mode'],
  ['Sidebar', 'Show Work Items and Reviews counts', 'show-work-item-counts'],
  ['Sidebar', 'Show Search in Sidebar', 'show-search'],
  ['Sidebar', 'Show Automations in Sidebar', 'show-automations'],
  ['Status Bar', 'Show Status Bar', 'show-status-bar'],
  ['Terminal', 'Font', 'font-family'],
  ['Terminal', 'Font Size', 'font-size'],
  ['Terminal', 'Horizontal Padding', 'padding-x'],
  ['Terminal', 'Vertical Padding', 'padding-y'],
  ['Terminal', 'Font Smoothing', 'smoothing'],
  ['Terminal', 'Show Open in Button', 'open-in-button'],
  ['Privacy', 'Share anonymous usage data', 'telemetry-enabled'],
  ['Usage', 'Show Usage in Status Bar', 'usage-status'],
  ['General', 'Confirm Before Quitting', 'confirmQuit'],
  ['General', 'Confirm Before Closing a Session', 'confirmSessionClose'],
  ['Appearance', 'Mode: System, Light, Dark', 'settings-mode'],
  ['Appearance', 'Duplicate as Custom Theme', 'duplicate-theme'],
  ['General', 'Check for Updates', 'check-updates'],
  ['Status Bar', 'Restore Default Layout', 'restore-status-layout'],
  ['Profiles', 'Profiles, accounts, login and skills', ''],
  ['Usage', 'Codex, Claude Code and OpenRouter usage limits', ''],
].map(([section, label, target]) => ({ section, label, target }))
settingsSearchItems.push(
  ...statusBarGroups.flatMap((group) =>
    group.items.map(([id, label]) => ({ section: 'Status Bar', label, target: 'status-item-' + id })),
  ),
  ...shortcuts.map((item) => ({ section: 'Shortcuts', label: item.name, target: 'shortcut-' + item.id })),
)

export function searchSettings(query) {
  const words = query.trim().toLowerCase().split(/\s+/).filter(Boolean)
  return words.length
    ? settingsSearchItems.filter((item) =>
        words.every((word) => (item.section + ' ' + item.label).toLowerCase().includes(word)),
      )
    : []
}
