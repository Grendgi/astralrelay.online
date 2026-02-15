/**
 * E2EE backup: encrypt keys with KDF(password, server_salt).
 * Decryption requires server to provide salt.
 */
import * as nacl from 'tweetnacl'
import { decodeBase64, encodeBase64, decodeUTF8, encodeUTF8 } from 'tweetnacl-util'

const PBKDF2_ITERATIONS = 100_000
const NONCE_LENGTH = nacl.secretbox.nonceLength
const VERSION = 1

export interface BackupPayload {
  identityKey: string
  identitySecret: string
  identitySigningKey?: string // Ed25519 public, base64 — для проверки signed prekey
  signedPrekey: { key: string; signature: string; secret: string }
  oneTimePrekeys?: string[] // optional, secrets not stored; on restore we regenerate
}

async function deriveKey(password: string, salt: Uint8Array): Promise<Uint8Array> {
  const enc = new TextEncoder()
  const keyMaterial = await crypto.subtle.importKey(
    'raw',
    enc.encode(password),
    'PBKDF2',
    false,
    ['deriveBits']
  )
  const bits = await crypto.subtle.deriveBits(
    {
      name: 'PBKDF2',
      salt,
      iterations: PBKDF2_ITERATIONS,
      hash: 'SHA-256',
    },
    keyMaterial,
    256
  )
  return new Uint8Array(bits)
}

export async function createBackup(
  payload: BackupPayload,
  password: string,
  saltB64: string
): Promise<string> {
  const salt = decodeBase64(saltB64)
  const key = await deriveKey(password, salt)
  const nonce = nacl.randomBytes(NONCE_LENGTH)
  const plain = JSON.stringify(payload)
  const ciphertext = nacl.secretbox(decodeUTF8(plain), nonce, key)
  const blob = new Uint8Array(1 + NONCE_LENGTH + ciphertext.length)
  blob[0] = VERSION
  blob.set(nonce, 1)
  blob.set(ciphertext, 1 + NONCE_LENGTH)
  return encodeBase64(blob)
}

export async function restoreBackup(
  blobB64: string,
  password: string,
  saltB64: string
): Promise<BackupPayload> {
  const blob = decodeBase64(blobB64)
  if (blob[0] !== VERSION) {
    throw new Error('Unsupported backup version')
  }
  const salt = decodeBase64(saltB64)
  const key = await deriveKey(password, salt)
  const nonce = blob.slice(1, 1 + NONCE_LENGTH)
  const ciphertext = blob.slice(1 + NONCE_LENGTH)
  const plain = nacl.secretbox.open(ciphertext, nonce, key)
  if (!plain) {
    throw new Error('Wrong password or corrupted backup')
  }
  return JSON.parse(encodeUTF8(plain)) as BackupPayload
}
