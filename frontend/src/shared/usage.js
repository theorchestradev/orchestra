export function remainingLabel(value) {
  return new Intl.NumberFormat('en', { maximumFractionDigits: 1 }).format(value) + '%'
}
export function moneyLabel(value, precision = 2) {
  return value == null
    ? 'Unavailable'
    : new Intl.NumberFormat('en-US', {
        style: 'currency',
        currency: 'USD',
        minimumFractionDigits: 2,
        maximumFractionDigits: precision,
      }).format(value)
}
export function countLabel(value) {
  return new Intl.NumberFormat('en-US').format(value)
}
// The time left until a reset, such as "4h 25m"; empty when unknown or already passed.
export function durationLabel(epoch, now = Date.now()) {
  if (!epoch) return ''
  const minutes = Math.ceil((epoch * 1000 - now) / 60000)
  if (minutes <= 0) return ''
  if (minutes < 60) return `${minutes}m`
  const hours = Math.floor(minutes / 60),
    rest = minutes % 60
  if (hours < 24) return `${hours}h${rest ? ' ' + rest + 'm' : ''}`
  return `${Math.floor(hours / 24)}d${hours % 24 ? ' ' + (hours % 24) + 'h' : ''}`
}
export function resetLabel(epoch, now = Date.now()) {
  if (!epoch) return 'Reset time unavailable'
  const duration = durationLabel(epoch, now)
  return duration ? `Resets in ${duration}` : 'Reset time passed · refresh to update'
}
// Windows report the allowance left; warning levels follow how much is used.
export const usedPercent = (window) => Math.min(100, Math.max(0, 100 - window.remaining))
export const usageLevel = (used) => (used >= 95 ? 'critical' : used >= 80 ? 'warning' : '')
// A window whose reset has passed no longer limits the account until the next reading.
const current = (window, now) => !window.resetsAt || window.resetsAt * 1000 > now
// The window closest to its limit decides when an agent runs out.
export function constrainedWindow(provider, now = Date.now()) {
  return (provider?.windows || [])
    .filter((w) => current(w, now))
    .reduce((low, w) => (!low || w.remaining < low.remaining ? w : low), null)
}
// The status bar names the session's agent, or else the agent closest to its limit.
export function usageSummary(providers, agent, now = Date.now()) {
  const readings = (providers || [])
    .map((provider) => ({ agent: provider.agent, window: constrainedWindow(provider, now) }))
    .filter((r) => r.window)
  return (
    readings.find((r) => r.agent === agent) ||
    readings.reduce((low, r) => (!low || r.window.remaining < low.window.remaining ? r : low), null)
  )
}
