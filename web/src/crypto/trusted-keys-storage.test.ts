/**
 * Unit tests for trusted-keys-storage: trust state machine (seen/verified/changed).
 * Run: npm test -- src/crypto/trusted-keys-storage.test.ts
 */
import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import {
  setStoredIdentityKey,
  getStoredIdentityKey,
  getTrustState,
  markVerified,
  clearVerified,
  removeTrustEntry,
  checkIdentityKeyChange,
  initTrustedKeysStorage,
} from './trusted-keys-storage'

const recipient = 'alice'
const identityKeyA = 'A'.repeat(44)
const identityKeyB = 'B'.repeat(44)

describe('trusted-keys trust state machine', () => {
  beforeEach(async () => {
    await initTrustedKeysStorage()
  })

  afterEach(async () => {
    await removeTrustEntry(recipient)
  })

  it('seen: first contact (TOFU) creates entry with seenAt', async () => {
    await setStoredIdentityKey(recipient, identityKeyA)
    const state = await getTrustState(recipient)
    expect(state).not.toBeNull()
    expect(state!.status).toBe('unverified')
    expect(state!.identityKey).toBe(identityKeyA)
    expect(state!.seenAt).toBeDefined()
    expect(state!.verifiedAt).toBeUndefined()
  })

  it('verified: markVerified sets verifiedAt, getTrustState returns status verified', async () => {
    await setStoredIdentityKey(recipient, identityKeyA)
    await markVerified(recipient, 'manual')
    const state = await getTrustState(recipient)
    expect(state!.status).toBe('verified')
    expect(state!.verifiedAt).toBeDefined()
    expect(state!.verifiedMethod).toBe('manual')
  })

  it('verified: markVerified with qr method', async () => {
    await setStoredIdentityKey(recipient, identityKeyA)
    await markVerified(recipient, 'qr')
    const state = await getTrustState(recipient)
    expect(state!.status).toBe('verified')
    expect(state!.verifiedMethod).toBe('qr')
  })

  it('changed: when currentIdentityKey differs, getTrustState returns status changed', async () => {
    await setStoredIdentityKey(recipient, identityKeyA)
    await markVerified(recipient, 'manual')
    const state = await getTrustState(recipient, identityKeyB)
    expect(state!.status).toBe('changed')
    expect(state!.identityKey).toBe(identityKeyB)
    expect(state!.previousKey).toBe(identityKeyA)
  })

  it('checkIdentityKeyChange: returns changed when key differs', async () => {
    await setStoredIdentityKey(recipient, identityKeyA)
    const r = await checkIdentityKeyChange(recipient, identityKeyB)
    expect(r.changed).toBe(true)
    expect('previousKey' in r && r.previousKey).toBe(identityKeyA)
  })

  it('checkIdentityKeyChange: returns not changed when key matches', async () => {
    await setStoredIdentityKey(recipient, identityKeyA)
    const r = await checkIdentityKeyChange(recipient, identityKeyA)
    expect(r.changed).toBe(false)
  })

  it('setStoredIdentityKey: same key keeps entry, different key sets changedAt and clears verified', async () => {
    await setStoredIdentityKey(recipient, identityKeyA)
    await markVerified(recipient, 'manual')
    await setStoredIdentityKey(recipient, identityKeyB)
    const state = await getTrustState(recipient, identityKeyB)
    expect(state!.identityKey).toBe(identityKeyB)
    expect(state!.changedAt).toBeDefined()
    expect(state!.status).toBe('unverified')
    const stored = await getStoredIdentityKey(recipient)
    expect(stored).toBe(identityKeyB)
  })

  it('clearVerified: removes verified status, keeps identity key', async () => {
    await setStoredIdentityKey(recipient, identityKeyA)
    await markVerified(recipient, 'manual')
    await clearVerified(recipient)
    const state = await getTrustState(recipient)
    expect(state!.status).toBe('unverified')
    expect(state!.identityKey).toBe(identityKeyA)
    expect(state!.verifiedAt).toBeUndefined()
  })

  it('removeTrustEntry: removes entry entirely', async () => {
    await setStoredIdentityKey(recipient, identityKeyA)
    await removeTrustEntry(recipient)
    const stored = await getStoredIdentityKey(recipient)
    expect(stored).toBeNull()
    const state = await getTrustState(recipient)
    expect(state).toBeNull()
  })

  it('markVerified with identityKey when no entry: creates entry first', async () => {
    await markVerified(recipient, 'manual', identityKeyA)
    const state = await getTrustState(recipient)
    expect(state!.status).toBe('verified')
    expect(state!.identityKey).toBe(identityKeyA)
  })

  it('markVerified without identityKey when no entry: no-op', async () => {
    await markVerified(recipient, 'manual')
    const state = await getTrustState(recipient)
    expect(state).toBeNull()
  })

  it('multi-device: storage key is recipient:device_id', async () => {
    const deviceId = 'dev1'
    await setStoredIdentityKey(recipient, identityKeyA, deviceId)
    await markVerified(recipient, 'qr', undefined, deviceId)
    const state = await getTrustState(recipient, identityKeyA, deviceId)
    expect(state!.status).toBe('verified')
    const stored = await getStoredIdentityKey(recipient, deviceId)
    expect(stored).toBe(identityKeyA)
  })
})
