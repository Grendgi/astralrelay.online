/**
 * Trusted identity keys storage in IndexedDB (not localStorage).
 * Persists per-contact identity_key and trust state: seen, verified, changed.
 *
 * Trust states:
 * - seen: first contact (TOFU) or accepted new key after change
 * - verified: user confirmed safety number (manual or QR)
 * - changed: key differs from stored (computed at runtime)
 */
const DB_NAME = 'messenger-trusted-keys'
const DB_VERSION = 2
const STORE_NAME = 'data'
const KEY_NAME = 'identity_keys'

const LEGACY_STORAGE_KEY = 'e2ee_identity_keys'

export type TrustStatus = 'verified' | 'unverified' | 'changed'
export type VerifiedMethod = 'manual' | 'qr'

export interface TrustEntry {
  identityKey: string
  seenAt?: number
  verifiedAt?: number
  verifiedMethod?: VerifiedMethod
  changedAt?: number
}

export interface TrustState {
  status: TrustStatus
  identityKey: string
  previousKey?: string
  seenAt?: number
  verifiedAt?: number
  verifiedMethod?: VerifiedMethod
  changedAt?: number
}

type StoredMap = Record<string, TrustEntry | string>

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

function toTrustEntry(v: TrustEntry | string): TrustEntry {
  if (typeof v === 'string') return { identityKey: v }
  if (v && typeof v === 'object' && typeof (v as TrustEntry).identityKey === 'string') {
    return v as TrustEntry
  }
  return { identityKey: String(v) }
}

/** Storage key: recipient:device_id for multi-device, recipient for legacy. */
function storageKey(recipient: string, deviceId?: string): string {
  return deviceId ? `${recipient}:${deviceId}` : recipient
}

async function getMap(): Promise<StoredMap> {
  try {
    const db = await openDB()
    const raw = await new Promise<StoredMap | undefined>((resolve, reject) => {
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

async function setMap(map: StoredMap): Promise<void> {
  const db = await openDB()
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readwrite')
    const req = tx.objectStore(STORE_NAME).put(map, KEY_NAME)
    req.onerror = () => reject(req.error)
    req.onsuccess = () => resolve()
  })
  db.close()
}

/** Migrate from localStorage (legacy {recipient: identityKey} string values). */
async function migrateFromLocalStorage(): Promise<StoredMap> {
  try {
    const raw = localStorage.getItem(LEGACY_STORAGE_KEY)
    if (!raw) return {}
    const parsed = JSON.parse(raw)
    const map = typeof parsed === 'object' && parsed !== null ? parsed : {}
    if (Object.keys(map).length > 0) {
      const migrated: StoredMap = {}
      for (const [k, v] of Object.entries(map)) {
        if (typeof v === 'string') migrated[k] = { identityKey: v }
        else migrated[k] = toTrustEntry(v as TrustEntry)
      }
      await setMap(migrated)
      localStorage.removeItem(LEGACY_STORAGE_KEY)
    }
    return map as StoredMap
  } catch {
    return {}
  }
}

/** Ensure stored format is TrustEntry (migrate legacy string values). */
async function ensureTrustEntryFormat(): Promise<void> {
  const map = await getMap()
  let changed = false
  for (const [k, v] of Object.entries(map)) {
    if (typeof v === 'string') {
      ;(map as Record<string, TrustEntry>)[k] = { identityKey: v }
      changed = true
    }
  }
  if (changed) await setMap(map)
}

/** Get stored identity_key for a contact (or specific device when deviceId provided). */
export async function getStoredIdentityKey(recipient: string, deviceId?: string): Promise<string | null> {
  const map = await getMap()
  const key = storageKey(recipient, deviceId)
  const entry = map[key]
  if (!entry) return null
  return toTrustEntry(entry).identityKey
}

/** Store identity_key for a contact/device (TOFU or after accepting changed key). Clears verified when key changes. */
export async function setStoredIdentityKey(
  recipient: string,
  identityKey: string,
  deviceId?: string
): Promise<void> {
  const map = await getMap()
  const key = storageKey(recipient, deviceId)
  const prev = map[key]
  const prevEntry = prev ? toTrustEntry(prev) : null
  const now = Date.now()
  let entry: TrustEntry
  if (prevEntry?.identityKey === identityKey) {
    entry = { ...prevEntry, identityKey }
  } else if (prevEntry) {
    entry = { identityKey, seenAt: now, changedAt: now }
  } else {
    entry = { identityKey, seenAt: now }
  }
  map[key] = entry
  await setMap(map)
}

/** Clear verified status (don't trust). Keeps identity key; user can re-verify. */
export async function clearVerified(recipient: string, deviceId?: string): Promise<void> {
  const map = await getMap()
  const key = storageKey(recipient, deviceId)
  const raw = map[key]
  if (!raw) return
  const entry = toTrustEntry(raw)
  const { verifiedAt, verifiedMethod, ...rest } = entry
  map[key] = rest as TrustEntry
  await setMap(map)
}

/** Remove trust entry entirely (forget key). For full reset. */
export async function removeTrustEntry(recipient: string, deviceId?: string): Promise<void> {
  const map = await getMap()
  const key = storageKey(recipient, deviceId)
  delete map[key]
  await setMap(map)
}

/** Mark contact/device as verified (safety number confirmed). If identityKey provided and no entry exists, creates entry first. */
export async function markVerified(
  recipient: string,
  method: VerifiedMethod = 'manual',
  identityKey?: string,
  deviceId?: string
): Promise<void> {
  const map = await getMap()
  const key = storageKey(recipient, deviceId)
  let raw = map[key]
  if (!raw && identityKey) {
    map[key] = { identityKey }
    raw = map[key]
  }
  const entry = raw ? toTrustEntry(raw) : null
  if (!entry) return
  map[key] = {
    ...entry,
    verifiedAt: Date.now(),
    verifiedMethod: method,
  }
  await setMap(map)
}

/**
 * Get trust state for a contact (or specific device when deviceId provided).
 * Pass currentIdentityKey (from server) to detect 'changed'.
 */
export async function getTrustState(
  recipient: string,
  currentIdentityKey?: string,
  deviceId?: string
): Promise<TrustState | null> {
  await ensureTrustEntryFormat()
  const map = await getMap()
  const key = storageKey(recipient, deviceId)
  const raw = map[key]
  if (!raw) return null
  const entry = toTrustEntry(raw)
  const stored = entry.identityKey

  if (currentIdentityKey != null) {
    if (stored !== currentIdentityKey) {
      return {
        status: 'changed',
        identityKey: currentIdentityKey,
        previousKey: stored,
        seenAt: entry.seenAt,
        verifiedAt: entry.verifiedAt,
        verifiedMethod: entry.verifiedMethod,
        changedAt: entry.changedAt,
      }
    }
  }

  return {
    status: entry.verifiedAt ? 'verified' : 'unverified',
    identityKey: stored,
    seenAt: entry.seenAt,
    verifiedAt: entry.verifiedAt,
    verifiedMethod: entry.verifiedMethod,
    changedAt: entry.changedAt,
  }
}

/**
 * Check if identity_key has changed since last seen (for contact or specific device).
 * Returns { changed: true, previousKey } if different; { changed: false } otherwise.
 */
export async function checkIdentityKeyChange(
  recipient: string,
  currentIdentityKey: string,
  deviceId?: string
): Promise<{ changed: false } | { changed: true; previousKey: string }> {
  const stored = await getStoredIdentityKey(recipient, deviceId)
  if (!stored) return { changed: false }
  if (stored === currentIdentityKey) return { changed: false }
  return { changed: true, previousKey: stored }
}

/** Export trusted keys map for backup. */
export async function getTrustedKeysForBackup(): Promise<Record<string, TrustEntry>> {
  await ensureTrustEntryFormat()
  const map = await getMap()
  const out: Record<string, TrustEntry> = {}
  for (const [k, v] of Object.entries(map)) {
    if (v) out[k] = toTrustEntry(v)
  }
  return out
}

/** Restore trusted keys from backup. Merges with existing (backup overwrites). */
export async function restoreTrustedKeysFromBackup(data: Record<string, TrustEntry>): Promise<void> {
  if (!data || typeof data !== 'object') return
  const map = await getMap()
  for (const [k, v] of Object.entries(data)) {
    if (v && typeof v === 'object' && typeof (v as TrustEntry).identityKey === 'string') {
      map[k] = v as TrustEntry
    }
  }
  await setMap(map)
}

/** Ensure migration has run and return current map. Used at init. */
export async function initTrustedKeysStorage(): Promise<void> {
  await migrateFromLocalStorage()
  await ensureTrustEntryFormat()
}

/** Check if recipient has at least one verified device (multi-device). */
export async function getRecipientHasVerifiedDevice(
  recipient: string,
  deviceIds: string[]
): Promise<boolean> {
  await ensureTrustEntryFormat()
  if (!deviceIds.length) {
    const raw = (await getMap())[recipient]
    if (!raw) return false
    return !!toTrustEntry(raw).verifiedAt
  }
  const map = await getMap()
  for (const did of deviceIds) {
    const raw = map[storageKey(recipient, did)]
    if (raw && toTrustEntry(raw).verifiedAt) return true
  }
  return false
}
