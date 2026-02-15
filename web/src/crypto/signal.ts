/**
 * Signal Protocol integration via @privacyresearch/libsignal-protocol-typescript.
 * Uses dynamic import — falls back to MVP e2ee if the package is unavailable.
 *
 * Format: ciphertext prefixed with "sig1:" = Signal protocol; else = MVP.
 */
import type { PrekeyBundle } from './e2ee'

const SIGNAL_PREFIX = 'sig1:'

function b64ToBuf(b64: string): ArrayBuffer {
  const bin = atob(b64)
  const arr = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) arr[i] = bin.charCodeAt(i)
  return arr.buffer.slice(arr.byteOffset, arr.byteOffset + arr.byteLength)
}

function bufToB64(buf: ArrayBuffer): string {
  return btoa(String.fromCharCode(...new Uint8Array(buf)))
}

export interface StoredKeys {
  identityKey: string
  identitySecret: string
  signedPrekey: { key: string; signature: string; secret: string; key_id?: number }
  oneTimePrekeys?: Array<{ key_id: number; pub: string; priv: string }>
}

/** Derive Signal device ID from backend device UUID. Must match server's UUIDToSignalDeviceID. */
export function uuidToSignalDeviceId(uuid: string): number {
  if (!uuid || uuid.length < 8) return 1
  const hex = uuid.replace(/-/g, '').slice(0, 8)
  const n = parseInt(hex, 16)
  if (isNaN(n)) return 1
  return Math.max(1, (n % 16383) + 1)
}

/** Resolve Signal device ID from bundle (signal_device_id or derive from device_id). */
function bundleSignalDeviceId(bundle: PrekeyBundle & { device_id?: string; signal_device_id?: number }): number {
  if (typeof bundle.signal_device_id === 'number' && bundle.signal_device_id >= 1) return bundle.signal_device_id
  if (bundle.device_id) return uuidToSignalDeviceId(bundle.device_id)
  return 1
}

/** Convert our PrekeyBundle to libsignal DeviceType. */
function bundleToDeviceType(
  bundle: PrekeyBundle & { device_id?: string; signal_device_id?: number },
  signalDeviceId: number
) {
  const signedPreKey = {
    keyId: bundle.signed_prekey.key_id,
    publicKey: b64ToBuf(bundle.signed_prekey.key),
    signature: b64ToBuf(bundle.signed_prekey.signature),
  }
  const oneTimePreKeys: Array<{ keyId: number; publicKey: ArrayBuffer }> = []
  if (bundle.one_time_prekey) {
    oneTimePreKeys.push({
      keyId: bundle.one_time_prekey.key_id,
      publicKey: b64ToBuf(bundle.one_time_prekey.key),
    })
  }
  return {
    registrationId: 0,
    deviceId: signalDeviceId,
    identityKey: b64ToBuf(bundle.identity_key),
    signedPreKey,
    preKeyId: undefined,
    preKeyPublic: undefined,
    oneTimePreKeyId: oneTimePreKeys[0]?.keyId,
    oneTimePreKeyPublic: oneTimePreKeys[0]?.publicKey,
  }
}

import { IndexedDBSignalStore } from './signal-store'
import { removeOtpkFromStorage } from './key-storage'

type SignalStoreLike = Pick<IndexedDBSignalStore, 'get' | 'put' | 'remove' | 'getIdentityKeyPair' | 'getLocalRegistrationId' | 'isTrustedIdentity' | 'loadPreKey' | 'loadSignedPreKey' | 'loadSession' | 'saveIdentity' | 'storeIdentityKeyPair' | 'storeLocalRegistrationId' | 'storePreKey' | 'storeSignedPreKey' | 'storeSession' | 'removePreKey' | 'removeSignedPreKey' | 'loadIdentityKey'>

function createStoreWithOtpkSync(base: IndexedDBSignalStore): SignalStoreLike {
  return {
    get: (k, d) => base.get(k, d),
    put: (k, v) => base.put(k, v),
    remove: (k) => base.remove(k),
    getIdentityKeyPair: () => base.getIdentityKeyPair(),
    getLocalRegistrationId: () => base.getLocalRegistrationId(),
    isTrustedIdentity: () => base.isTrustedIdentity(),
    loadPreKey: (id) => base.loadPreKey(id),
    loadSignedPreKey: (id) => base.loadSignedPreKey(id),
    loadSession: (id) => base.loadSession(id),
    saveIdentity: () => base.saveIdentity(),
    storeIdentityKeyPair: (kp) => base.storeIdentityKeyPair(kp),
    storeLocalRegistrationId: (id) => base.storeLocalRegistrationId(id),
    storePreKey: (id, kp) => base.storePreKey(id, kp),
    storeSignedPreKey: (id, kp) => base.storeSignedPreKey(id, kp),
    storeSession: (id, r) => base.storeSession(id, r),
    removePreKey: async (keyId) => {
      await removeOtpkFromStorage(typeof keyId === 'string' ? parseInt(keyId, 10) : keyId)
      await base.removePreKey(keyId)
    },
    removeSignedPreKey: (id) => base.removeSignedPreKey(id),
    loadIdentityKey: () => base.loadIdentityKey(),
  }
}

/** Get or create persistent registrationId (uint16). Never overwrite existing. */
async function getOrCreateRegistrationId(store: IndexedDBSignalStore): Promise<number> {
  const existing = await store.getLocalRegistrationId()
  if (existing > 0 && existing <= 0xffff && existing !== 0x1234) return existing
  const rid = Math.floor(Math.random() * 0xffff) + 1
  await store.storeLocalRegistrationId(rid)
  return rid
}

/** Populate persistent store with our keys. */
async function initStore(store: IndexedDBSignalStore, keys: StoredKeys): Promise<void> {
  const identityKeyPair = {
    pubKey: b64ToBuf(keys.identityKey),
    privKey: b64ToBuf(keys.identitySecret),
  }
  await store.storeIdentityKeyPair(identityKeyPair)
  await getOrCreateRegistrationId(store)

  const signedPreKeyPair = {
    pubKey: b64ToBuf(keys.signedPrekey.key),
    privKey: b64ToBuf(keys.signedPrekey.secret),
  }
  const spkId = keys.signedPrekey.key_id ?? 1
  await store.storeSignedPreKey(spkId, signedPreKeyPair)

  for (const otpk of keys.oneTimePrekeys ?? []) {
    if (otpk.priv) {
      await store.storePreKey(otpk.key_id, {
        pubKey: b64ToBuf(otpk.pub),
        privKey: b64ToBuf(otpk.priv),
      })
    }
  }
}

/**
 * Encrypt with Signal protocol. Returns "sig1:" + base64 or throws (fallback to MVP).
 * recipientBundle may include device_id/signal_device_id for multi-device.
 */
export async function signalEncrypt(
  plaintext: string,
  recipientBundle: PrekeyBundle & { device_id?: string; signal_device_id?: number },
  ourKeys: StoredKeys,
  recipientAddr: string,
  deviceId?: number
): Promise<string> {
  const lib = await import('@privacyresearch/libsignal-protocol-typescript')
  const base = new IndexedDBSignalStore()
  await initStore(base, ourKeys)
  const store = createStoreWithOtpkSync(base)

  const sigDeviceId = deviceId ?? bundleSignalDeviceId(recipientBundle)
  const address = new lib.SignalProtocolAddress(recipientAddr, sigDeviceId)
  const bundle = bundleToDeviceType(recipientBundle, sigDeviceId)

  const sessionBuilder = new lib.SessionBuilder(store, address)
  await sessionBuilder.processPreKey(bundle)

  const sessionCipher = new lib.SessionCipher(store, address)
  const plaintextBuf = new TextEncoder().encode(plaintext).buffer
  const ciphertext = await sessionCipher.encrypt(plaintextBuf)

  const body = ciphertext.body
  if (!body) throw new Error('Signal encrypt: no body')
  const serialized = typeof body === 'string' ? body : bufToB64(body as ArrayBuffer)
  return SIGNAL_PREFIX + serialized
}

/**
 * Decrypt Signal protocol message. Throws if not Signal format or decrypt fails.
 * senderDeviceId: use ev.sender_device (UUID) with uuidToSignalDeviceId, or pass number.
 */
export async function signalDecrypt(
  ciphertext: string,
  ourKeys: StoredKeys,
  senderAddr: string,
  senderDeviceId?: number | string
): Promise<string> {
  if (!ciphertext.startsWith(SIGNAL_PREFIX)) throw new Error('Not Signal format')

  const lib = await import('@privacyresearch/libsignal-protocol-typescript')
  const base = new IndexedDBSignalStore()
  await initStore(base, ourKeys)
  const store = createStoreWithOtpkSync(base)

  const sigDeviceId =
    typeof senderDeviceId === 'number'
      ? senderDeviceId
      : typeof senderDeviceId === 'string'
        ? uuidToSignalDeviceId(senderDeviceId)
        : 1
  const address = new lib.SignalProtocolAddress(senderAddr, sigDeviceId)
  const sessionCipher = new lib.SessionCipher(store, address)

  const raw = ciphertext.slice(SIGNAL_PREFIX.length)
  try {
    const plain = await sessionCipher.decryptPreKeyWhisperMessage(raw, 'binary')
    return new TextDecoder().decode(plain)
  } catch {
    try {
      const plain = await sessionCipher.decryptWhisperMessage(raw, 'binary')
      return new TextDecoder().decode(plain)
    } catch {
      throw new Error('Signal decrypt failed')
    }
  }
}

export function isSignalCiphertext(s: string): boolean {
  return s.startsWith(SIGNAL_PREFIX)
}
