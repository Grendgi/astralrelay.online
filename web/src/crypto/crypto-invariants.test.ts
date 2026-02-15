/**
 * Mini crypto invariants — catch regressions across modules.
 * Run: npm test -- src/crypto/crypto-invariants.test.ts
 */
import { describe, it, expect } from 'vitest'
import * as nacl from 'tweetnacl'
import { encrypt, decrypt, PrekeyBundle } from './e2ee'
import { isValidBase64, validateAttachmentVersion, E2EE_ATTACHMENT_VERSION_MIN, E2EE_ATTACHMENT_VERSION_MAX } from './e2ee'
import { encryptAttachment, decryptAttachment } from './e2ee'

function generateMockBundle(): { bundle: PrekeyBundle; identitySecret: Uint8Array; signedPrekeySecret: Uint8Array } {
  const identity = nacl.box.keyPair()
  const signed = nacl.box.keyPair()
  const signKp = nacl.sign.keyPair()
  const encode = (b: Uint8Array) => btoa(String.fromCharCode(...b))
  return {
    bundle: {
      identity_key: encode(identity.publicKey),
      signed_prekey: {
        key: encode(signed.publicKey),
        signature: encode(nacl.sign.detached(signed.publicKey, signKp.secretKey)),
        key_id: 1,
      },
    },
    identitySecret: identity.secretKey,
    signedPrekeySecret: signed.secretKey,
  }
}

describe('crypto invariants', () => {
  it('MVP encrypt/decrypt roundtrip', () => {
    const { bundle, identitySecret, signedPrekeySecret } = generateMockBundle()
    const plain = 'Hello E2EE world'
    const ct = encrypt(plain, bundle)
    expect(typeof ct).toBe('string')
    expect(ct.length).toBeGreaterThan(0)
    const dec = decrypt(ct, identitySecret, signedPrekeySecret)
    expect(dec).toBe(plain)
  })

  it('attachment encrypt/decrypt roundtrip', async () => {
    const plain = new Uint8Array([1, 2, 3, 4, 5])
    const { ciphertext, fileKey, nonce, sha256 } = await encryptAttachment(plain)
    const dec = await decryptAttachment(ciphertext, fileKey, nonce, sha256)
    expect(dec).toEqual(plain)
  })

  it('isValidBase64: valid strings pass', () => {
    expect(isValidBase64('SGVsbG8=')).toBe(true)
    expect(isValidBase64('YWJjZA==')).toBe(true)
  })

  it('validateAttachmentVersion: bounds', () => {
    expect(() => validateAttachmentVersion(E2EE_ATTACHMENT_VERSION_MIN)).not.toThrow()
    expect(() => validateAttachmentVersion(E2EE_ATTACHMENT_VERSION_MAX)).not.toThrow()
    expect(() => validateAttachmentVersion(E2EE_ATTACHMENT_VERSION_MAX + 1)).toThrow()
  })

  it('nacl constants: key and nonce lengths', () => {
    expect(nacl.secretbox.keyLength).toBe(32)
    expect(nacl.secretbox.nonceLength).toBe(24)
    expect(nacl.box.publicKeyLength).toBe(32)
  })
})
