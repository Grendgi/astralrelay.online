/**
 * In-memory store for @privacyresearch/libsignal-protocol-typescript.
 * Implements StorageType interface. Sessions are lost on page reload.
 */
type KeyPair = { pubKey: ArrayBuffer; privKey: ArrayBuffer }
type StoreValue = KeyPair | ArrayBuffer | string | number | undefined

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
    return typeof rid === 'number' ? rid : 0x1234
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
