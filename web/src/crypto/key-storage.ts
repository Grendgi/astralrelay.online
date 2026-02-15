/**
 * E2EE keys storage in IndexedDB (not localStorage) to reduce XSS impact.
 * Optional passphrase wrapping: keys can be stored encrypted with PBKDF2+NaCl.
 */
import * as nacl from 'tweetnacl'
import { decodeBase64, encodeBase64, decodeUTF8, encodeUTF8 } from 'tweetnacl-util'
import type { StoredKeys, OneTimePrekeyEntry } from '../hooks/useAuth'

const DB_NAME = 'messenger-key-storage'
const DB_VERSION = 1
const STORE_NAME = 'keys'
const KEY_NAME = 'e2ee_keys'

const PBKDF2_ITERATIONS = 100_000
const NONCE_LENGTH = nacl.secretbox.nonceLength
const ENCRYPTED_VERSION = 2

async function deriveKey(password: string, salt: Uint8Array): Promise<Uint8Array> {
  const enc = new TextEncoder()
  const keyMaterial = await crypto.subtle.importKey('raw', enc.encode(password), 'PBKDF2', false, ['deriveBits'])
  const bits = await crypto.subtle.deriveBits(
    { name: 'PBKDF2', salt, iterations: PBKDF2_ITERATIONS, hash: 'SHA-256' },
    keyMaterial,
    256
  )
  return new Uint8Array(bits)
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
  return typeof raw === 'object' && raw !== null && 'v' in raw && (raw as { v: number }).v === ENCRYPTED_VERSION
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

export async function setKeysInStorage(keys: StoredKeys): Promise<void> {
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

/** Encrypt keys with passphrase and store. Replaces existing keys. */
export async function setKeysWithPassphrase(keys: StoredKeys, passphrase: string): Promise<void> {
  const salt = crypto.getRandomValues(new Uint8Array(32))
  const key = await deriveKey(passphrase, salt)
  const nonce = nacl.randomBytes(NONCE_LENGTH)
  const plain = JSON.stringify(keys)
  const ciphertext = nacl.secretbox(decodeUTF8(plain), nonce, key)
  const blob = new Uint8Array(1 + NONCE_LENGTH + ciphertext.length)
  blob[0] = ENCRYPTED_VERSION
  blob.set(nonce, 1)
  blob.set(ciphertext, 1 + NONCE_LENGTH)
  await putRawToStorage({ v: ENCRYPTED_VERSION, salt: encodeBase64(salt), blob: encodeBase64(blob) })
}

/** Decrypt keys with passphrase. On success, overwrites storage with plain keys (unlocked). */
export async function unlockKeysWithPassphrase(passphrase: string): Promise<StoredKeys> {
  const raw = await getRawFromStorage()
  if (!raw || !isEncryptedBlob(raw)) throw new Error('Keys are not encrypted')
  const blob = decodeBase64(raw.blob)
  if (blob[0] !== ENCRYPTED_VERSION) throw new Error('Unsupported format')
  const salt = decodeBase64(raw.salt)
  const key = await deriveKey(passphrase, salt)
  const nonce = blob.slice(1, 1 + NONCE_LENGTH)
  const ciphertext = blob.slice(1 + NONCE_LENGTH)
  const plain = nacl.secretbox.open(ciphertext, nonce, key)
  if (!plain) throw new Error('Wrong passphrase')
  const keys = normalizeStoredKeys(JSON.parse(encodeUTF8(plain)) as StoredKeys)
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
  await setKeysInStorage({ ...keys, signedPrekey })
}

/** Merge new one-time prekeys (from replenishment) into stored keys. */
export async function mergeOtpksToStorage(entries: Array<{ key_id: number; pub: string; priv: string }>): Promise<void> {
  const keys = await getKeysFromStorage()
  if (!keys) return
  const existingIds = new Set(keys.oneTimePrekeys.map((o) => o.key_id))
  const toAdd = entries.filter((e) => !existingIds.has(e.key_id))
  if (toAdd.length === 0) return
  await setKeysInStorage({
    ...keys,
    oneTimePrekeys: [...keys.oneTimePrekeys, ...toAdd],
  })
}

/** Remove one-time prekey when consumed by Signal protocol. */
export async function removeOtpkFromStorage(keyId: number): Promise<void> {
  const keys = await getKeysFromStorage()
  if (!keys?.oneTimePrekeys?.length) return
  const filtered = keys.oneTimePrekeys.filter((o) => o.key_id !== keyId)
  if (filtered.length === keys.oneTimePrekeys.length) return
  await setKeysInStorage({ ...keys, oneTimePrekeys: filtered })
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
