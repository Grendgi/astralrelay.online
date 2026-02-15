/**
 * Unit tests for E2EE attachments: encrypt/decrypt, sha256 integrity, chunked.
 * Run: npm test -- src/crypto/e2ee-attachment.test.ts
 */
import { describe, it, expect } from 'vitest'
import {
  encryptAttachment,
  decryptAttachment,
  verifyAttachmentHash,
  sha256Base64,
  DEFAULT_CHUNK_SIZE,
} from './e2ee'

describe('attachment encrypt/decrypt', () => {
  it('small payload: encrypt → decrypt roundtrip', async () => {
    const plain = new TextEncoder().encode('Hello E2EE attachment')
    const { ciphertext, fileKey, nonce, sha256 } = await encryptAttachment(plain)
    expect(ciphertext.length).toBeGreaterThan(plain.length)
    expect(nonce).toBeTruthy()
    expect(sha256).toBeTruthy()

    const decrypted = await decryptAttachment(ciphertext, fileKey, nonce, sha256)
    expect(new TextDecoder().decode(decrypted)).toBe('Hello E2EE attachment')
  })

  it('decrypt verifies sha256 and throws on mismatch', async () => {
    const plain = new Uint8Array([1, 2, 3])
    const { ciphertext, fileKey, nonce, sha256 } = await encryptAttachment(plain)
    const badSha = sha256.slice(0, -2) + 'xx'
    await expect(decryptAttachment(ciphertext, fileKey, nonce, badSha)).rejects.toThrow(/integrity|hash/)
  })

  it('verifyAttachmentHash passes on correct hash', async () => {
    const data = new Uint8Array([1, 2, 3])
    const hash = await sha256Base64(data)
    await expect(verifyAttachmentHash(data, hash)).resolves.toBeUndefined()
  })

  it('verifyAttachmentHash throws on wrong hash', async () => {
    const data = new Uint8Array([1, 2, 3])
    await expect(verifyAttachmentHash(data, 'wrong')).rejects.toThrow(/integrity|mismatch/)
  })
})

describe('chunked attachment', () => {
  it('large payload uses chunked encryption', async () => {
    const chunkSize = 64 * 1024
    const plain = new Uint8Array(chunkSize * 2 + 100)
    crypto.getRandomValues(plain)

    const { ciphertext, fileKey, nonce, sha256, chunkSize: outChunkSize } = await encryptAttachment(plain, {
      chunkSize,
    })
    expect(nonce).toBe('')
    expect(outChunkSize).toBe(chunkSize)

    const decrypted = await decryptAttachment(ciphertext, fileKey, nonce, sha256, chunkSize)
    expect(decrypted.length).toBe(plain.length)
    expect(decrypted).toEqual(plain)
  })

  it('chunked decrypt throws on wrong key', async () => {
    const plain = new Uint8Array(DEFAULT_CHUNK_SIZE + 100)
    crypto.getRandomValues(plain)
    const { ciphertext, nonce, sha256, chunkSize } = await encryptAttachment(plain)

    const wrongKey = 'a'.repeat(44)
    await expect(decryptAttachment(ciphertext, wrongKey, nonce ?? '', sha256, chunkSize)).rejects.toThrow(
      /decryption failed|decrypt/
    )
  })
})
