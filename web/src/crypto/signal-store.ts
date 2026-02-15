/**
 * Signal protocol stores for @privacyresearch/libsignal-protocol-typescript.
 * - InMemorySignalStore: ephemeral, sessions lost on reload
 * - IndexedDBSignalStore: persistent, survives reloads — preferred for maximum security
 */
type KeyPair = { pubKey: ArrayBuffer; privKey: ArrayBuffer }
type StoreValue = KeyPair | ArrayBuffer | string | number | undefined

const DB_NAME = 'signal-keystore'
const DB_VERSION = 1
const STORE_NAME = 'kv'

/** Helper to serialize values for IndexedDB (handles ArrayBuffer in objects). */
function serialize(v: StoreValue): unknown {
  if (v === undefined) return undefined
  if (v instanceof ArrayBuffer) return v
  if (typeof v === 'string' || typeof v === 'number') return v
  if (v && typeof v === 'object' && 'pubKey' in v && 'privKey' in v) {
    return { pubKey: (v as KeyPair).pubKey, privKey: (v as KeyPair).privKey }
  }
  return v
}

/** Helper to deserialize (IndexedDB returns as-is for ArrayBuffer). */
function deserialize(v: unknown): StoreValue {
  if (v === undefined || v === null) return undefined
  if (v instanceof ArrayBuffer) return v
  if (typeof v === 'string' || typeof v === 'number') return v
  if (v && typeof v === 'object' && 'pubKey' in v && 'privKey' in v) {
    return { pubKey: (v as KeyPair).pubKey, privKey: (v as KeyPair).privKey } as KeyPair
  }
  return v as StoreValue
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

async function getFromDB(db: IDBDatabase, key: string): Promise<StoreValue> {
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readonly')
    const store = tx.objectStore(STORE_NAME)
    const req = store.get(key)
    req.onerror = () => reject(req.error)
    req.onsuccess = () => resolve(deserialize(req.result))
  })
}

async function putToDB(db: IDBDatabase, key: string, value: StoreValue): Promise<void> {
  if (value === undefined) return
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readwrite')
    const store = tx.objectStore(STORE_NAME)
    const req = store.put(serialize(value), key)
    req.onerror = () => reject(req.error)
    req.onsuccess = () => resolve()
  })
}

async function removeFromDB(db: IDBDatabase, key: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readwrite')
    const store = tx.objectStore(STORE_NAME)
    const req = store.delete(key)
    req.onerror = () => reject(req.error)
    req.onsuccess = () => resolve()
  })
}

/** In-memory store — sessions lost on page reload. */
export class InMemorySignalStore {
  private _store: Record<string, StoreValue> = {}

  get(key: string, defaultValue?: StoreValue): StoreValue {
    if (key in this._store) return this._store[key]
    return defaultValue
  }

  put(key: string, value: StoreValue): void {
    if (value != null) this._store[key] = value
  }

  remove(key: string): void {
    delete this._store[key]
  }

  async getIdentityKeyPair(): Promise<KeyPair | undefined> {
    const kp = this.get('identityKey')
    if (kp && typeof kp === 'object' && 'pubKey' in kp) return kp as KeyPair
    return undefined
  }

  async getLocalRegistrationId(): Promise<number> {
    const rid = this.get('registrationId')
    return typeof rid === 'number' ? rid : 0
  }

  async isTrustedIdentity(): Promise<boolean> {
    return true
  }

  async loadPreKey(keyId: number | string): Promise<KeyPair | undefined> {
    const k = this.get('25519KeypreKey' + keyId)
    if (k && typeof k === 'object' && 'pubKey' in k) return k as KeyPair
    return undefined
  }

  async loadSignedPreKey(keyId: number | string): Promise<KeyPair | undefined> {
    const k = this.get('25519KeysignedKey' + keyId)
    if (k && typeof k === 'object' && 'pubKey' in k) return k as KeyPair
    return undefined
  }

  async loadSession(identifier: string): Promise<string | undefined> {
    const s = this.get('session' + identifier)
    if (typeof s === 'string') return s
    return undefined
  }

  async saveIdentity(): Promise<boolean> {
    return false
  }

  async storeIdentityKeyPair(kp: KeyPair): Promise<void> {
    this.put('identityKey', kp)
  }

  async storeLocalRegistrationId(id: number): Promise<void> {
    this.put('registrationId', id)
  }

  async storePreKey(keyId: number | string, keyPair: KeyPair): Promise<void> {
    this.put('25519KeypreKey' + keyId, keyPair)
  }

  async storeSignedPreKey(keyId: number | string, keyPair: KeyPair): Promise<void> {
    this.put('25519KeysignedKey' + keyId, keyPair)
  }

  async storeSession(identifier: string, record: string): Promise<void> {
    this.put('session' + identifier, record)
  }

  async removePreKey(keyId: number | string): Promise<void> {
    this.remove('25519KeypreKey' + keyId)
  }

  async removeSignedPreKey(keyId: number | string): Promise<void> {
    this.remove('25519KeysignedKey' + keyId)
  }

  async loadIdentityKey(): Promise<ArrayBuffer | undefined> {
    const kp = await this.getIdentityKeyPair()
    return kp?.pubKey
  }
}

export type SignalStore = InMemorySignalStore | IndexedDBSignalStore

/** IndexedDB-backed persistent store — sessions survive reloads. Use for maximum security. */
export class IndexedDBSignalStore {
  private _db: IDBDatabase | null = null

  private async db(): Promise<IDBDatabase> {
    if (!this._db) this._db = await openDB()
    return this._db
  }

  async get(key: string, defaultValue?: StoreValue): Promise<StoreValue> {
    const v = await getFromDB(await this.db(), key)
    return v !== undefined ? v : defaultValue
  }

  async put(key: string, value: StoreValue): Promise<void> {
    await putToDB(await this.db(), key, value)
  }

  async remove(key: string): Promise<void> {
    await removeFromDB(await this.db(), key)
  }

  async getIdentityKeyPair(): Promise<KeyPair | undefined> {
    const kp = await this.get('identityKey')
    if (kp && typeof kp === 'object' && 'pubKey' in kp) return kp as KeyPair
    return undefined
  }

  async getLocalRegistrationId(): Promise<number> {
    const rid = await this.get('registrationId')
    return typeof rid === 'number' ? rid : 0
  }

  async isTrustedIdentity(): Promise<boolean> {
    return true
  }

  async loadPreKey(keyId: number | string): Promise<KeyPair | undefined> {
    const k = await this.get('25519KeypreKey' + keyId)
    if (k && typeof k === 'object' && 'pubKey' in k) return k as KeyPair
    return undefined
  }

  async loadSignedPreKey(keyId: number | string): Promise<KeyPair | undefined> {
    const k = await this.get('25519KeysignedKey' + keyId)
    if (k && typeof k === 'object' && 'pubKey' in k) return k as KeyPair
    return undefined
  }

  async loadSession(identifier: string): Promise<string | undefined> {
    const s = await this.get('session' + identifier)
    if (typeof s === 'string') return s
    return undefined
  }

  async saveIdentity(): Promise<boolean> {
    return false
  }

  async storeIdentityKeyPair(kp: KeyPair): Promise<void> {
    await this.put('identityKey', kp)
  }

  async storeLocalRegistrationId(id: number): Promise<void> {
    await this.put('registrationId', id)
  }

  async storePreKey(keyId: number | string, keyPair: KeyPair): Promise<void> {
    await this.put('25519KeypreKey' + keyId, keyPair)
  }

  async storeSignedPreKey(keyId: number | string, keyPair: KeyPair): Promise<void> {
    await this.put('25519KeysignedKey' + keyId, keyPair)
  }

  async storeSession(identifier: string, record: string): Promise<void> {
    await this.put('session' + identifier, record)
  }

  async removePreKey(keyId: number | string): Promise<void> {
    await this.remove('25519KeypreKey' + keyId)
  }

  async removeSignedPreKey(keyId: number | string): Promise<void> {
    await this.remove('25519KeysignedKey' + keyId)
  }

  async loadIdentityKey(): Promise<ArrayBuffer | undefined> {
    const kp = await this.getIdentityKeyPair()
    return kp?.pubKey
  }
}
