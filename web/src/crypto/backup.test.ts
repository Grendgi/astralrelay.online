/**
 * Unit tests for backup: roundtrip create/restore, integrity, migration.
 * Run: npm test -- src/crypto/backup.test.ts
 */
import { describe, it, expect } from 'vitest'
import { createBackup, restoreBackup, type BackupPayload } from './backup'

const mockPayload: BackupPayload = {
  identityKey: 'a'.repeat(44),
  identitySecret: 'b'.repeat(44),
  identitySigningKey: 'c'.repeat(44),
  identitySigningSecret: 'd'.repeat(44),
  signedPrekey: {
    key: 'e'.repeat(44),
    signature: 'f'.repeat(88),
    secret: 'g'.repeat(44),
    key_id: 1,
  },
  oneTimePrekeys: [
    { key_id: 1, pub: 'h'.repeat(44), priv: 'i'.repeat(44) },
    { key_id: 2, pub: 'j'.repeat(44), priv: 'k'.repeat(44) },
  ],
  schemaVersion: 2,
  trustedKeys: {
    alice: { identityKey: 'x'.repeat(44), seenAt: 1000, verifiedAt: 2000 },
  },
}

const password = 'test-password-123'
const saltB64 = 'dGVzdC1zYWx0LWJhc2U2NA==' // btoa('test-salt-base64')

describe('backup roundtrip', () => {
  it('createBackup + restoreBackup preserves payload', async () => {
    const blob = await createBackup(mockPayload, password, saltB64)
    expect(blob).toBeTruthy()
    const restored = await restoreBackup(blob, password, saltB64)
    expect(restored.identityKey).toBe(mockPayload.identityKey)
    expect(restored.identitySecret).toBe(mockPayload.identitySecret)
    expect(restored.identitySigningKey).toBe(mockPayload.identitySigningKey)
    expect(restored.identitySigningSecret).toBe(mockPayload.identitySigningSecret)
    expect(restored.signedPrekey.key_id).toBe(mockPayload.signedPrekey.key_id)
    expect(restored.oneTimePrekeys?.length).toBe(mockPayload.oneTimePrekeys?.length)
    expect(Object.keys(restored.trustedKeys ?? {}).length).toBe(Object.keys(mockPayload.trustedKeys ?? {}).length)
  })

  it('restoreBackup rejects wrong password', async () => {
    const blob = await createBackup(mockPayload, password, saltB64)
    await expect(restoreBackup(blob, 'wrong-password', saltB64)).rejects.toThrow(/Wrong password|corrupted/)
  })

  it('restoreBackup verifies integrity', async () => {
    const blob = await createBackup(mockPayload, password, saltB64)
    const decoded = atob(blob)
    const tampered = decoded.slice(0, -2) + 'xx'
    await expect(restoreBackup(btoa(tampered), password, saltB64)).rejects.toThrow()
  })

  it('handles payload without trustedKeys', async () => {
    const minimal: BackupPayload = {
      ...mockPayload,
      trustedKeys: undefined,
    }
    const blob = await createBackup(minimal, password, saltB64)
    const restored = await restoreBackup(blob, password, saltB64)
    expect(restored.trustedKeys).toEqual({})
  })
})
