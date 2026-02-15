/**
 * Simplified E2EE using X25519 DH + NaCl secretbox.
 * Not full X3DH/Double Ratchet but provides end-to-end encryption.
 * Signal protocol: web/src/crypto/signal.ts (opt-in, fallback на MVP)
 */
import * as nacl from 'tweetnacl'
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

/** Encrypt attachment (file) with CEK+nonce. Returns ciphertext + fileKey + nonce. */
export async function encryptAttachment(plaintext: Uint8Array): Promise<{
  ciphertext: Uint8Array
  fileKey: string
  nonce: string
}> {
  const key = nacl.randomBytes(nacl.secretbox.keyLength)
  const nonce = nacl.randomBytes(nacl.secretbox.nonceLength)
  const ciphertext = nacl.secretbox(plaintext, nonce, key)
  return { ciphertext, fileKey: encodeBase64(key), nonce: encodeBase64(nonce) }
}

/** Decrypt attachment with file_key + nonce. */
export function decryptAttachment(
  ciphertext: Uint8Array,
  fileKeyB64: string,
  nonceB64: string
): Uint8Array {
  const key = decodeBase64(fileKeyB64)
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
