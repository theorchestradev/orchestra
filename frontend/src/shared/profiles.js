import { spaceSymbol } from './workspace.js'

// Agents whose CLI reads its config folder from a variable, so a profile can choose the account.
export const profileAgents = ['codex', 'claude']

// An empty profile ID is Default, which uses each CLI's own config folder.
export const findProfile = (profiles, id) => (id ? (profiles || []).find((p) => p.id === id) : null)

export function profileName(profiles, id) {
  if (!id) return 'Default'
  return findProfile(profiles, id)?.name || 'Removed profile'
}

// The badge shows the profile's emoji, or else its name's first letter.
export const profileSymbol = (profile) => (profile ? spaceSymbol(profile) : '!')

// Sessions name their profile in the sidebar only when it differs from the workspace's.
export function sessionProfile(profiles, workspace, session) {
  const id = session?.profileID || ''
  if (!session || id === (workspace?.profileID || '')) return null
  const profile = findProfile(profiles, id)
  return {
    id,
    name: profileName(profiles, id),
    symbol: profileSymbol(id ? profile : { name: 'Default' }),
    missing: !!id && !profile,
  }
}
