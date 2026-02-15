/**
 * Trusted identity keys storage in IndexedDB (not localStorage).
 * Persists per-contact identity_key for key-change detection.
 */
const DB_NAME = 'messenger-trusted-keys'
const DB_VERSION = 1
const STORE_NAME = 'data'
const KEY_NAME = 'identity_keys'

const LEGACY_STORAGE_KEY = 'e2ee_identity_keys'

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

async function getMap(): Promise<Record<string, string>> {
  try {
    const db = await openDB()
    const raw = await new Promise<Record<string, string> | undefined>((resolve, reject) => {
      const tx = db.transaction(STORE_NAME, 'readonly')
      const req = tx.objectStore(STORE_NAME).get(KEY_NAME)
      req.onerror = () => reject(req.error)
      req.onsuccess = () => resolve(req.result)
    })
    db.close()
    return raw && typeof raw === 'object' ? raw : {}
  } catch {
    return {}
  }
}

async function setMap(map: Record<string, string>): Promise<void> {
  const db = await openDB()
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readwrite')
    const req = tx.objectStore(STORE_NAME).put(map, KEY_NAME)
    req.onerror = () => reject(req.error)
    req.onsuccess = () => resolve()
  })
  db.close()
}

/** Migrate from localStorage to IndexedDB. */
async function migrateFromLocalStorage(): Promise<Record<string, string>> {
  try {
    const raw = localStorage.getItem(LEGACY_STORAGE_KEY)
    if (!raw) return {}
    const parsed = JSON.parse(raw)
    const map = typeof parsed === 'object' && parsed !== null ? parsed : {}
    if (Object.keys(map).length > 0) {
      await setMap(map)
      localStorage.removeItem(LEGACY_STORAGE_KEY)
    }
    return map
  } catch {
    return {}
  }
}

/** Get stored identity_key for a contact. */
export async function getStoredIdentityKey(recipient: string): Promise<string | null> {
  const map = await getMap()
  return map[recipient] ?? null
}

/** Store identity_key for a contact (after verification or first contact). */
export async function setStoredIdentityKey(recipient: string, identityKey: string): Promise<void> {
  const map = await getMap()
  map[recipient] = identityKey
  await setMap(map)
}

/**
 * Check if identity_key has changed since last seen.
 * Returns { changed: true, previousKey } if different; { changed: false } otherwise.
 */
export async function checkIdentityKeyChange(
  recipient: string,
  currentIdentityKey: string
): Promise<{ changed: false } | { changed: true; previousKey: string }> {
  const stored = await getStoredIdentityKey(recipient)
  if (!stored) return { changed: false }
  if (stored === currentIdentityKey) return { changed: false }
  return { changed: true, previousKey: stored }
}

/** Ensure migration has run and return current map. Used at init. */
export async function initTrustedKeysStorage(): Promise<void> {
  await migrateFromLocalStorage()
}
