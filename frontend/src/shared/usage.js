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
export function resetLabel(epoch, now = Date.now()) {
  if (!epoch) return 'Reset time unavailable'
  const minutes = Math.ceil((epoch * 1000 - now) / 60000)
  if (minutes <= 0) return 'Reset time passed · refresh to update'
  if (minutes < 60) return `Resets in ${minutes}m`
  const hours = Math.floor(minutes / 60),
    rest = minutes % 60
  if (hours < 24) return `Resets in ${hours}h${rest ? ' ' + rest + 'm' : ''}`
  return `Resets in ${Math.floor(hours / 24)}d${hours % 24 ? ' ' + (hours % 24) + 'h' : ''}`
}
