/**
 * E2EE keys storage in IndexedDB (not localStorage) to reduce XSS impact.
 * XSS can still read IndexedDB, but it's harder than localStorage and
 * allows future wrapping with WebCrypto non-extractable.
 */
import type { StoredKeys, OneTimePrekeyEntry } from '../hooks/useAuth'

const DB_NAME = 'messenger-key-storage'
const DB_VERSION = 1
const STORE_NAME = 'keys'
const KEY_NAME = 'e2ee_keys'

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

export async function getKeysFromStorage(): Promise<StoredKeys | null> {
  try {
    const db = await openDB()
    const raw = await new Promise<StoredKeys | undefined>((resolve, reject) => {
      const tx = db.transaction(STORE_NAME, 'readonly')
      const req = tx.objectStore(STORE_NAME).get(KEY_NAME)
      req.onerror = () => reject(req.error)
      req.onsuccess = () => resolve(req.result)
    })
    db.close()
    if (!raw) return null
    return normalizeStoredKeys(raw)
  } catch {
    return null
  }
}

export async function setKeysInStorage(keys: StoredKeys): Promise<void> {
  const db = await openDB()
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readwrite')
    const req = tx.objectStore(STORE_NAME).put(keys, KEY_NAME)
    req.onerror = () => reject(req.error)
    req.onsuccess = () => resolve()
  })
  db.close()
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
