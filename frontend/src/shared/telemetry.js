import { call } from './api'

export const privacyURL = 'https://theorchestra.dev/docs/privacy'

// Fixed values only; Go applies consent, environment gates, rate limits and the allowlist again.
export function track(event, properties) {
  call('Track', event, properties).catch(() => {})
}
