/**
 * E2EE keys storage in IndexedDB (not localStorage) to reduce XSS impact.
 * Passphrase wrapping: v2 = PBKDF2+NaCl; v3 = PBKDF2+AES-GCM with non-extractable wrapping key.
 */
import * as nacl from 'tweetnacl'
import { decodeBase64, encodeBase64, decodeUTF8, encodeUTF8 } from 'tweetnacl-util'
import type { StoredKeys, OneTimePrekeyEntry } from '../hooks/useAuth'

const DB_NAME = 'messenger-key-storage'
const DB_VERSION = 1
const STORE_NAME = 'keys'
const KEY_NAME = 'e2ee_keys'

const PBKDF2_ITERATIONS = 100_000
const NACL_NONCE_LENGTH = nacl.secretbox.nonceLength
const ENCRYPTED_VERSION = 2
const ENCRYPTED_VERSION_V3 = 3 // AES-GCM + non-extractable wrapping key
const AES_GCM_IV_LENGTH = 12

async function deriveKeyRaw(password: string, salt: Uint8Array): Promise<Uint8Array> {
  const enc = new TextEncoder()
  const keyMaterial = await crypto.subtle.importKey('raw', enc.encode(password), 'PBKDF2', false, ['deriveBits'])
  const bits = await crypto.subtle.deriveBits(
    { name: 'PBKDF2', salt, iterations: PBKDF2_ITERATIONS, hash: 'SHA-256' },
    keyMaterial,
    256
  )
  return new Uint8Array(bits)
}

/** Derive wrapping key as non-extractable CryptoKey — never exported. */
async function deriveWrappingKey(password: string, salt: Uint8Array): Promise<CryptoKey> {
  const bits = await deriveKeyRaw(password, salt)
  return crypto.subtle.importKey('raw', bits, 'AES-GCM', false, ['encrypt', 'decrypt'])
}

function openDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION)
    req.onerror = () => reject(req.error)
    req.onsuccess = () => resolve(req.result)
    req.onupgradeneeded = (e) => {
      const db = (e.target as IDBOpenDBRequest).result
      if (!db.objectStoreNames.contains(STORE_NAME)) {
        db.createObjectStore(STORE_NAME)
      }
    }
  })
}

/** Normalize legacy oneTimePrekeys: string[] to OneTimePrekeyEntry[] (legacy has no privs). */
function normalizeStoredKeys(raw: StoredKeys & { oneTimePrekeys?: unknown }): StoredKeys {
  const otpk = raw.oneTimePrekeys
  if (Array.isArray(otpk) && otpk.length > 0) {
    const first = otpk[0]
    if (first && typeof first === 'object' && 'key_id' in first && 'pub' in first && 'priv' in first) {
      return raw as StoredKeys
    }
  }
  return { ...raw, oneTimePrekeys: [] }
}

type StoredRaw = StoredKeys | { v: number; salt: string; blob: string }

function isEncryptedBlobV2(raw: StoredRaw): raw is { v: number; salt: string; blob: string } {
  return typeof raw === 'object' && raw !== null && 'v' in raw && (raw as { v: number }).v === ENCRYPTED_VERSION
}

function isEncryptedBlobV3(raw: StoredRaw): raw is { v: number; salt: string; blob: string } {
  return typeof raw === 'object' && raw !== null && 'v' in raw && (raw as { v: number }).v === ENCRYPTED_VERSION_V3
}

export async function getKeysFromStorage(): Promise<StoredKeys | null> {
  try {
    const raw = await getRawFromStorage()
    if (!raw) return null
    if (isEncryptedBlob(raw)) return null
    return normalizeStoredKeys(raw as StoredKeys)
  } catch {
    return null
  }
}

/** Returns true if keys are stored encrypted (need passphrase to unlock). */
export async function isKeysEncrypted(): Promise<boolean> {
  try {
    const raw = await getRawFromStorage()
    return raw != null && isEncryptedBlob(raw)
  } catch {
    return false
  }
}

function isEncryptedBlob(raw: StoredRaw): raw is { v: number; salt: string; blob: string } {
  return isEncryptedBlobV2(raw) || isEncryptedBlobV3(raw)
}

async function getRawFromStorage(): Promise<StoredRaw | undefined> {
  const db = await openDB()
  const raw = await new Promise<StoredRaw | undefined>((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readonly')
    const req = tx.objectStore(STORE_NAME).get(KEY_NAME)
    req.onerror = () => reject(req.error)
    req.onsuccess = () => resolve(req.result)
  })
  db.close()
  return raw
}

export async function setKeysInStorage(keys: StoredKeys | null | undefined): Promise<void> {
  if (!keys || typeof keys !== 'object') return
  if (!keys.identityKey || !keys.identitySecret || !keys.signedPrekey) return
  await putRawToStorage(keys)
}

async function putRawToStorage(data: StoredKeys | { v: number; salt: string; blob: string }): Promise<void> {
  const db = await openDB()
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readwrite')
    const req = tx.objectStore(STORE_NAME).put(data, KEY_NAME)
    req.onerror = () => reject(req.error)
    req.onsuccess = () => resolve()
  })
  db.close()
}

/** Encrypt keys with passphrase and store. Uses v3 (AES-GCM + non-extractable wrapping key). */
export async function setKeysWithPassphrase(keys: StoredKeys, passphrase: string): Promise<void> {
  const salt = crypto.getRandomValues(new Uint8Array(32))
  const wrappingKey = await deriveWrappingKey(passphrase, salt)
  const iv = crypto.getRandomValues(new Uint8Array(AES_GCM_IV_LENGTH))
  const plain = new TextEncoder().encode(JSON.stringify(keys))
  const ciphertext = await crypto.subtle.encrypt(
    { name: 'AES-GCM', iv, tagLength: 128 },
    wrappingKey,
    plain
  )
  const blob = new Uint8Array(1 + iv.length + ciphertext.byteLength)
  blob[0] = ENCRYPTED_VERSION_V3
  blob.set(iv, 1)
  blob.set(new Uint8Array(ciphertext), 1 + iv.length)
  await putRawToStorage({ v: ENCRYPTED_VERSION_V3, salt: encodeBase64(salt), blob: encodeBase64(blob) })
}

/** Decrypt keys with passphrase. On success, overwrites storage with plain keys (unlocked). */
export async function unlockKeysWithPassphrase(passphrase: string): Promise<StoredKeys> {
  const raw = await getRawFromStorage()
  if (!raw || !isEncryptedBlob(raw)) throw new Error('Keys are not encrypted')
  const blob = decodeBase64(raw.blob)
  const salt = decodeBase64(raw.salt)
  let plain: Uint8Array

  if (isEncryptedBlobV3(raw)) {
    const wrappingKey = await deriveWrappingKey(passphrase, salt)
    const iv = blob.slice(1, 1 + AES_GCM_IV_LENGTH)
    const ciphertext = blob.slice(1 + AES_GCM_IV_LENGTH)
    try {
      const decrypted = await crypto.subtle.decrypt(
        { name: 'AES-GCM', iv, tagLength: 128 },
        wrappingKey,
        ciphertext
      )
      plain = new Uint8Array(decrypted)
    } catch {
      throw new Error('Wrong passphrase')
    }
  } else {
    if (blob[0] !== ENCRYPTED_VERSION) throw new Error('Unsupported format')
    const key = await deriveKeyRaw(passphrase, salt)
    const nonce = blob.slice(1, 1 + NACL_NONCE_LENGTH)
    const ciphertext = blob.slice(1 + NACL_NONCE_LENGTH)
    const opened = nacl.secretbox.open(ciphertext, nonce, key)
    if (!opened) throw new Error('Wrong passphrase')
    plain = opened
  }

  const keys = normalizeStoredKeys(JSON.parse(new TextDecoder().decode(plain)) as StoredKeys)
  await putRawToStorage(keys)
  return keys
}

export async function clearKeysFromStorage(): Promise<void> {
  try {
    const db = await openDB()
    await new Promise<void>((resolve, reject) => {
      const tx = db.transaction(STORE_NAME, 'readwrite')
      const req = tx.objectStore(STORE_NAME).delete(KEY_NAME)
      req.onerror = () => reject(req.error)
      req.onsuccess = () => resolve()
    })
    db.close()
  } catch {
    // ignore
  }
}

/** Update signed prekey (after rotation). */
export async function updateSignedPrekeyInStorage(
  signedPrekey: { key: string; signature: string; secret: string; key_id: number }
): Promise<void> {
  const keys = await getKeysFromStorage()
  if (!keys) return
  const current = keys.signedPrekey
  if (current.key_id === signedPrekey.key_id && current.key === signedPrekey.key) return
  await putRawToStorage({ ...keys, signedPrekey })
}

/** Merge new one-time prekeys (from replenishment) into stored keys. */
export async function mergeOtpksToStorage(entries: Array<{ key_id: number; pub: string; priv: string }>): Promise<void> {
  if (!entries?.length) return
  const keys = await getKeysFromStorage()
  if (!keys) return
  const existingIds = new Set(keys.oneTimePrekeys.map((o) => o.key_id))
  const toAdd = entries.filter((e) => !existingIds.has(e.key_id))
  if (toAdd.length === 0) return
  await putRawToStorage({
    ...keys,
    oneTimePrekeys: [...keys.oneTimePrekeys, ...toAdd],
  })
}

/** Remove one-time prekey when consumed by Signal protocol. */
export async function removeOtpkFromStorage(keyId: number): Promise<void> {
  const keys = await getKeysFromStorage()
  if (!keys || !keys.oneTimePrekeys?.length) return
  const filtered = keys.oneTimePrekeys.filter((o) => o.key_id !== keyId)
  if (filtered.length === keys.oneTimePrekeys.length) return
  await putRawToStorage({ ...keys, oneTimePrekeys: filtered })
}

/** Migrate keys from localStorage to IndexedDB, then remove from localStorage. */
export async function migrateKeysFromLocalStorage(): Promise<StoredKeys | null> {
  try {
    const raw = localStorage.getItem('keys')
    if (!raw) return null
    const keys = JSON.parse(raw) as StoredKeys & { oneTimePrekeys?: unknown }
    if (!keys?.identityKey || !keys?.identitySecret || !keys?.signedPrekey) return null
    const normalized = normalizeStoredKeys(keys)
    await setKeysInStorage(normalized)
    localStorage.removeItem('keys')
    return normalized
  } catch {
    return null
  }
}
