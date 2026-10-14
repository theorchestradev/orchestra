import { isTerminal } from './appearance.js'

// Sessions that finished a turn or asked for permission since the previous workspace update.
// Startup, restores and first launches go from no state or "unknown" to "waiting", so they never count.
export function sessionsNeedingYou(previous, next, skip = () => false) {
  const before = new Map((previous || []).flatMap((p) => p.sessions || []).map((s) => [s.id, s.activity]))
  return (next || [])
    .flatMap((p) => p.sessions || [])
    .filter((s) => !isTerminal(s) && s.activity === 'waiting' && before.get(s.id) === 'working' && !skip(s))
}

// One sound covers every session that stops within the same second.
export function attentionSound(play, now = () => Date.now()) {
  let last = -Infinity
  return () => {
    if (now() - last < 1000) return false
    last = now()
    play()
    return true
  }
}
