/**
 * Simplified E2EE using X25519 DH + NaCl secretbox.
 * Not full X3DH/Double Ratchet but provides end-to-end encryption.
 * Signal protocol: web/src/crypto/signal.ts (opt-in, fallback на MVP)
 */
import * as nacl from 'tweetnacl'

/** E2EE attachment envelope version: 1=file_key+nonce, 2=+file_sha256 */
export const E2EE_ATTACHMENT_VERSION = 2
import { decodeBase64, encodeBase64, decodeUTF8, encodeUTF8 } from 'tweetnacl-util'

export interface PrekeyBundle {
  identity_key: string
  signed_prekey: { key: string; signature: string; key_id: number }
  one_time_prekey?: { key: string; key_id: number }
}

function decodeKey(b64: string): Uint8Array {
  const bin = atob(b64)
  const arr = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) arr[i] = bin.charCodeAt(i)
  return arr
}

function hash(...parts: Uint8Array[]): Uint8Array {
  const combined = new Uint8Array(parts.reduce((s, p) => s + p.length, 0))
  let offset = 0
  for (const p of parts) {
    combined.set(p, offset)
    offset += p.length
  }
  return nacl.hash(combined).slice(0, nacl.secretbox.keyLength)
}

export function encrypt(plaintext: string, bundle: PrekeyBundle): string {
  const ephemeral = nacl.box.keyPair()
  const identityPub = decodeKey(bundle.identity_key)
  const signedPrekeyPub = decodeKey(bundle.signed_prekey.key)

  // MVP: identity + signed_prekey only. One-time prekeys require recipient to store private keys.
  const parts: Uint8Array[] = [
    nacl.box.before(identityPub, ephemeral.secretKey),
    nacl.box.before(signedPrekeyPub, ephemeral.secretKey),
  ]

  const shared = hash(...parts)
  const nonce = nacl.randomBytes(nacl.secretbox.nonceLength)
  const msg = decodeUTF8(plaintext)
  const ciphertext = nacl.secretbox(msg, nonce, shared)

  const payload = new Uint8Array(
    ephemeral.publicKey.length + nonce.length + ciphertext.length
  )
  payload.set(ephemeral.publicKey, 0)
  payload.set(nonce, ephemeral.publicKey.length)
  payload.set(ciphertext, ephemeral.publicKey.length + nonce.length)

  return encodeBase64(payload)
}

export function decrypt(ciphertextB64: string, identitySecret: Uint8Array, signedPrekeySecret: Uint8Array, oneTimePrekeySecret?: Uint8Array): string {
  const payload = decodeBase64(ciphertextB64)
  const ephemPub = payload.slice(0, nacl.box.publicKeyLength)
  const nonce = payload.slice(
    nacl.box.publicKeyLength,
    nacl.box.publicKeyLength + nacl.secretbox.nonceLength
  )
  const box = payload.slice(nacl.box.publicKeyLength + nacl.secretbox.nonceLength)

  const parts: Uint8Array[] = [
    nacl.box.before(ephemPub, identitySecret),
    nacl.box.before(ephemPub, signedPrekeySecret),
  ]
  if (oneTimePrekeySecret) {
    parts.push(nacl.box.before(ephemPub, oneTimePrekeySecret))
  }

  const shared = hash(...parts)
  const plain = nacl.secretbox.open(box, nonce, shared)
  if (!plain) throw new Error('Decryption failed')
  return encodeUTF8(plain)
}

/** Compute SHA-256 of bytes, return base64. */
export async function sha256Base64(data: Uint8Array): Promise<string> {
  const hash = await crypto.subtle.digest('SHA-256', data as BufferSource)
  const arr = new Uint8Array(hash)
  let bin = ''
  for (let i = 0; i < arr.length; i++) bin += String.fromCharCode(arr[i]!)
  return btoa(bin)
}

/** Default chunk size for large files (1MB). Files larger use chunked encryption. */
export const DEFAULT_CHUNK_SIZE = 1024 * 1024

/** Build deterministic nonce for chunk index (unique per key+chunk). */
function chunkNonce(chunkIndex: number): Uint8Array {
  const nonce = new Uint8Array(nacl.secretbox.nonceLength)
  const view = new DataView(nonce.buffer)
  view.setUint32(nonce.length - 4, chunkIndex, false)
  return nonce
}

/** Encrypt attachment (file) with CEK+nonce. For large files, uses chunked encryption. */
export async function encryptAttachment(
  plaintext: Uint8Array,
  options?: { chunkSize?: number }
): Promise<{
  ciphertext: Uint8Array
  fileKey: string
  nonce: string
  sha256: string
  chunkSize?: number
}> {
  const chunkSize = options?.chunkSize ?? DEFAULT_CHUNK_SIZE
  const key = nacl.randomBytes(nacl.secretbox.keyLength)

  if (plaintext.length <= chunkSize) {
    const nonce = nacl.randomBytes(nacl.secretbox.nonceLength)
    const ciphertext = nacl.secretbox(plaintext, nonce, key)
    const sha256 = await sha256Base64(ciphertext)
    return { ciphertext, fileKey: encodeBase64(key), nonce: encodeBase64(nonce), sha256 }
  }

  const chunks: Uint8Array[] = []
  for (let i = 0; i < plaintext.length; i += chunkSize) {
    const chunk = plaintext.subarray(i, Math.min(i + chunkSize, plaintext.length))
    const idx = Math.floor(i / chunkSize)
    const nonce = chunkNonce(idx)
    const ct = nacl.secretbox(chunk, nonce, key)
    chunks.push(ct)
  }
  const ciphertext = new Uint8Array(chunks.reduce((s, c) => s + c.length, 0))
  let offset = 0
  for (const c of chunks) {
    ciphertext.set(c, offset)
    offset += c.length
  }
  const sha256 = await sha256Base64(ciphertext)
  return { ciphertext, fileKey: encodeBase64(key), nonce: '', sha256, chunkSize }
}

/** Verify ciphertext hash before decrypt. Throws if mismatch. */
export async function verifyAttachmentHash(ciphertext: Uint8Array, expectedSha256: string): Promise<void> {
  const actual = await sha256Base64(ciphertext)
  if (actual !== expectedSha256) {
    throw new Error('Attachment integrity check failed (hash mismatch)')
  }
}

/** Decrypt attachment with file_key + nonce (or chunked with chunk_size). Optionally verify sha256 first. */
export async function decryptAttachment(
  ciphertext: Uint8Array,
  fileKeyB64: string,
  nonceB64: string,
  sha256B64?: string,
  chunkSize?: number
): Promise<Uint8Array> {
  if (sha256B64) {
    await verifyAttachmentHash(ciphertext, sha256B64)
  }
  const key = decodeBase64(fileKeyB64)

  if (chunkSize != null && chunkSize > 0) {
    const plainParts: Uint8Array[] = []
    let offset = 0
    let idx = 0
    while (offset < ciphertext.length) {
      const overhead = nacl.secretbox.overheadLength
      const ctLen = Math.min(chunkSize + overhead, ciphertext.length - offset)
      const ct = ciphertext.subarray(offset, offset + ctLen)
      const nonce = chunkNonce(idx)
      const plain = nacl.secretbox.open(ct, nonce, key)
      if (!plain) throw new Error('Attachment decryption failed')
      plainParts.push(plain)
      offset += ctLen
      idx++
    }
    const total = plainParts.reduce((s, p) => s + p.length, 0)
    const out = new Uint8Array(total)
    let o = 0
    for (const p of plainParts) {
      out.set(p, o)
      o += p.length
    }
    return out
  }

  const nonce = decodeBase64(nonceB64)
  const plain = nacl.secretbox.open(ciphertext, nonce, key)
  if (!plain) throw new Error('Attachment decryption failed')
  return plain
}

/** Detect if ciphertext is our E2EE format vs legacy base64 */
export function isE2EEPayload(s: string): boolean {
  try {
    const raw = decodeBase64(s)
    return raw.length >= nacl.box.publicKeyLength + nacl.secretbox.nonceLength + nacl.secretbox.overheadLength
  } catch {
    return false
  }
}
