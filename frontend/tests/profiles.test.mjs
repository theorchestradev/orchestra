import test from 'node:test'
import assert from 'node:assert/strict'
import { findProfile, profileName, sessionProfile } from '../src/shared/profiles.js'

const profiles = [
  { id: 'work', name: 'Work', emoji: '💼' },
  { id: 'side', name: 'side workspaces' },
]

test('an empty profile ID is Default', () => {
  assert.equal(findProfile(profiles, ''), null)
  assert.equal(profileName(profiles, ''), 'Default')
  assert.equal(profileName(profiles, 'work'), 'Work')
  assert.equal(profileName(profiles, 'gone'), 'Removed profile')
})

test('sessions show their profile only when it differs from the workspace', () => {
  assert.equal(sessionProfile(profiles, { profileID: 'work' }, { profileID: 'work' }), null)
  assert.equal(sessionProfile(profiles, {}, {}), null)
  assert.deepEqual(sessionProfile(profiles, {}, { profileID: 'work' }), {
    id: 'work',
    name: 'Work',
    symbol: '💼',
    missing: false,
  })
  assert.equal(sessionProfile(profiles, { profileID: 'work' }, { profileID: 'side' }).symbol, 'S')
  assert.deepEqual(sessionProfile(profiles, { profileID: 'work' }, {}), {
    id: '',
    name: 'Default',
    symbol: 'D',
    missing: false,
  })
  assert.equal(sessionProfile(profiles, {}, { profileID: 'gone' }).missing, true)
})
