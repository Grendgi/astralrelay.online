/**
 * Unit tests for key-storage: "no write on no-op" invariants.
 * Run: npx vitest run src/crypto/key-storage.test.ts (requires vitest)
 * Or: npm test -- --run src/crypto/key-storage.test.ts
 */
import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import {
  setKeysInStorage,
  getKeysFromStorage,
  clearKeysFromStorage,
  updateSignedPrekeyInStorage,
  mergeOtpksToStorage,
  removeOtpkFromStorage,
} from './key-storage'

const mockKeys = {
  identityKey: 'a'.repeat(44),
  identitySecret: 'b'.repeat(44),
  signedPrekey: {
    key: 'c'.repeat(44),
    signature: 'd'.repeat(88),
    secret: 'e'.repeat(44),
    key_id: 1,
  },
  oneTimePrekeys: [
    { key_id: 1, pub: 'f'.repeat(44), priv: 'g'.repeat(44) },
    { key_id: 2, pub: 'h'.repeat(44), priv: 'i'.repeat(44) },
  ],
}

describe('key-storage no-op invariants', () => {
  beforeEach(async () => {
    await clearKeysFromStorage()
    await setKeysInStorage(mockKeys)
  })

  afterEach(async () => {
    await clearKeysFromStorage()
  })

  it('updateSignedPrekeyInStorage: does not write when same signed prekey', async () => {
    const before = JSON.stringify(await getKeysFromStorage())
    await updateSignedPrekeyInStorage(mockKeys.signedPrekey)
    const after = JSON.stringify(await getKeysFromStorage())
    expect(after).toBe(before)
  })

  it('mergeOtpksToStorage: does not write when all OTPK already exist', async () => {
    const before = JSON.stringify(await getKeysFromStorage())
    await mergeOtpksToStorage(mockKeys.oneTimePrekeys)
    const after = JSON.stringify(await getKeysFromStorage())
    expect(after).toBe(before)
  })

  it('removeOtpkFromStorage: does not write when key_id not found', async () => {
    const before = JSON.stringify(await getKeysFromStorage())
    await removeOtpkFromStorage(999)
    const after = JSON.stringify(await getKeysFromStorage())
    expect(after).toBe(before)
  })

  it('updateSignedPrekeyInStorage: writes when signed prekey differs', async () => {
    const newSpk = { ...mockKeys.signedPrekey, key_id: 2, key: 'x'.repeat(44) }
    await updateSignedPrekeyInStorage(newSpk)
    const keys = await getKeysFromStorage()
    expect(keys?.signedPrekey.key_id).toBe(2)
  })

  it('mergeOtpksToStorage: writes when new OTPK added', async () => {
    await mergeOtpksToStorage([{ key_id: 10, pub: 'n'.repeat(44), priv: 'm'.repeat(44) }])
    const keys = await getKeysFromStorage()
    expect(keys?.oneTimePrekeys.find((o) => o.key_id === 10)).toBeDefined()
  })

  it('removeOtpkFromStorage: writes when key removed', async () => {
    await removeOtpkFromStorage(1)
    const keys = await getKeysFromStorage()
    expect(keys?.oneTimePrekeys.find((o) => o.key_id === 1)).toBeUndefined()
  })
})
