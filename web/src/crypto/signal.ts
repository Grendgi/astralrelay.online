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
  signedPrekey: { key: string; signature: string; secret: string }
}

/** Convert our PrekeyBundle to libsignal DeviceType. */
function bundleToDeviceType(bundle: PrekeyBundle) {
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
    deviceId: 1,
    identityKey: b64ToBuf(bundle.identity_key),
    signedPreKey,
    preKeyId: undefined,
    preKeyPublic: undefined,
    oneTimePreKeyId: oneTimePreKeys[0]?.keyId,
    oneTimePreKeyPublic: oneTimePreKeys[0]?.publicKey,
  }
}

import { InMemorySignalStore } from './signal-store'

/** Populate store with our keys. */
function initStore(store: InMemorySignalStore, keys: StoredKeys): void {
  const identityKeyPair = {
    pubKey: b64ToBuf(keys.identityKey),
    privKey: b64ToBuf(keys.identitySecret),
  }
  store.put('identityKey', identityKeyPair)
  store.put('registrationId', 0x1234)

  const signedPreKeyPair = {
    pubKey: b64ToBuf(keys.signedPrekey.key),
    privKey: b64ToBuf(keys.signedPrekey.secret),
  }
  store.put('25519KeysignedKey1', signedPreKeyPair)
}

/**
 * Encrypt with Signal protocol. Returns "sig1:" + base64 or throws (fallback to MVP).
 */
export async function signalEncrypt(
  plaintext: string,
  recipientBundle: PrekeyBundle,
  ourKeys: StoredKeys,
  recipientAddr: string,
  deviceId: number = 1
): Promise<string> {
  const lib = await import('@privacyresearch/libsignal-protocol-typescript')
  const store = new InMemorySignalStore()
  initStore(store, ourKeys)

  const address = new lib.SignalProtocolAddress(recipientAddr, deviceId)
  const bundle = bundleToDeviceType(recipientBundle)

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
 */
export async function signalDecrypt(
  ciphertext: string,
  ourKeys: StoredKeys,
  senderAddr: string,
  deviceId: number = 1
): Promise<string> {
  if (!ciphertext.startsWith(SIGNAL_PREFIX)) throw new Error('Not Signal format')

  const lib = await import('@privacyresearch/libsignal-protocol-typescript')
  const store = new InMemorySignalStore()
  initStore(store, ourKeys)

  const address = new lib.SignalProtocolAddress(senderAddr, deviceId)
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
