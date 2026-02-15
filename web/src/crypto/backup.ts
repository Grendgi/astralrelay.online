/**
 * E2EE backup: encrypt keys with KDF(password, server_salt).
 * AEAD (secretbox = XSalsa20-Poly1305) + explicit integrity hash.
 * Decryption requires server to provide salt.
 *
 * Schema versions:
 * - v1: identityKey, identitySecret, signedPrekey, oneTimePrekeys: string[] (pub only)
 * - v2: full Strict — identitySigningSecret, oneTimePrekeys as {key_id,pub,priv}, trustedKeys, registrationId, device_id
 * - v2+integrity: payload.integrity = SHA256(JSON) for explicit verification
 */
import * as nacl from 'tweetnacl'
import { decodeBase64, encodeBase64, decodeUTF8, encodeUTF8 } from 'tweetnacl-util'
import type { TrustEntry } from './trusted-keys-storage'

const PBKDF2_ITERATIONS = 100_000
const NONCE_LENGTH = nacl.secretbox.nonceLength
const VERSION_LEGACY = 1
const VERSION_FULL = 2

export interface OneTimePrekeyEntry {
  key_id: number
  pub: string
  priv: string
}

export interface BackupPayload {
  identityKey: string
  identitySecret: string
  identitySigningKey?: string
  identitySigningSecret?: string
  signedPrekey: { key: string; signature: string; secret: string; key_id?: number; created_at?: number }
  oneTimePrekeys?: OneTimePrekeyEntry[] | string[]
  registrationId?: number
  device_id?: string
  trustedKeys?: Record<string, TrustEntry>
  schemaVersion?: number
  /** SHA256 of payload (without this field) — integrity verification */
  integrity?: string
}

/** Legacy v1 payload (oneTimePrekeys = string[]). */
interface BackupPayloadV1 {
  identityKey: string
  identitySecret: string
  identitySigningKey?: string
  identitySigningSecret?: string
  signedPrekey: { key: string; signature: string; secret: string; key_id?: number }
  oneTimePrekeys?: string[]
}

async function sha256Base64(data: Uint8Array): Promise<string> {
  const hash = await crypto.subtle.digest('SHA-256', data as BufferSource)
  const arr = new Uint8Array(hash)
  let bin = ''
  for (let i = 0; i < arr.length; i++) bin += String.fromCharCode(arr[i]!)
  return btoa(bin)
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

/** Normalize to full BackupPayload (v2). */
function toFullPayload(p: BackupPayload | BackupPayloadV1): BackupPayload {
  const spk = p.signedPrekey
  const otpk = p.oneTimePrekeys
  const full: BackupPayload = {
    identityKey: p.identityKey,
    identitySecret: p.identitySecret,
    identitySigningKey: p.identitySigningKey,
    identitySigningSecret: p.identitySigningSecret,
    signedPrekey: {
      key: spk.key,
      signature: spk.signature,
      secret: spk.secret,
      key_id: spk.key_id ?? 1,
      created_at: (spk as { created_at?: number }).created_at,
    },
    schemaVersion: 2,
  }
  if (Array.isArray(otpk) && otpk.length > 0) {
    const first = otpk[0]
    if (first && typeof first === 'object' && 'key_id' in first && 'pub' in first && 'priv' in first) {
      full.oneTimePrekeys = otpk as OneTimePrekeyEntry[]
    } else {
      full.oneTimePrekeys = []
    }
  } else {
    full.oneTimePrekeys = []
  }
  if ('registrationId' in p && typeof (p as BackupPayload).registrationId === 'number') {
    full.registrationId = (p as BackupPayload).registrationId
  }
  if ('device_id' in p && typeof (p as BackupPayload).device_id === 'string') {
    full.device_id = (p as BackupPayload).device_id
  }
  if ((p as BackupPayload).trustedKeys && typeof (p as BackupPayload).trustedKeys === 'object') {
    full.trustedKeys = (p as BackupPayload).trustedKeys
  } else {
    full.trustedKeys = {}
  }
  return full
}

export async function createBackup(
  payload: BackupPayload,
  password: string,
  saltB64: string
): Promise<string> {
  const full = payload.schemaVersion === 2 ? payload : toFullPayload(payload)
  const { integrity: _drop, ...payloadNoIntegrity } = full
  const plainNoIntegrity = JSON.stringify(payloadNoIntegrity)
  const integrity = await sha256Base64(new TextEncoder().encode(plainNoIntegrity))
  const fullWithIntegrity = { ...full, integrity }
  const plain = JSON.stringify(fullWithIntegrity)
  const salt = decodeBase64(saltB64)
  const key = await deriveKey(password, salt)
  const nonce = nacl.randomBytes(NONCE_LENGTH)
  const ciphertext = nacl.secretbox(decodeUTF8(plain), nonce, key)
  const blob = new Uint8Array(1 + NONCE_LENGTH + ciphertext.length)
  blob[0] = VERSION_FULL
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
  const version = blob[0]
  if (version !== VERSION_LEGACY && version !== VERSION_FULL) {
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
  const parsed = JSON.parse(encodeUTF8(plain)) as BackupPayload | BackupPayloadV1
  const storedIntegrity = (parsed as BackupPayload).integrity
  if (typeof storedIntegrity === 'string') {
    const { integrity: _d, ...rest } = parsed as BackupPayload
    const plainNoIntegrity = JSON.stringify(rest)
    const computed = await sha256Base64(new TextEncoder().encode(plainNoIntegrity))
    if (computed !== storedIntegrity) {
      throw new Error('Backup integrity check failed (tampered or corrupted)')
    }
  }
  return toFullPayload(parsed)
}
