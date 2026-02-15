import * as nacl from 'tweetnacl'
import { decodeBase64, decodeUTF8, encodeBase64 } from 'tweetnacl-util'

/**
 * Generate identity key, signed prekey, and one-time prekeys.
 * For MVP we use NaCl box (X25519) for DH and ed25519 for signing.
 * Full X3DH/Double Ratchet would need libsignal - this is a placeholder.
 */
export async function generateKeys() {
  const identityKeyPair = nacl.box.keyPair()
  const signedPrekeyPair = nacl.box.keyPair()
  const sigKeyPair = nacl.sign.keyPair()
  const message = decodeUTF8(encodeBase64(signedPrekeyPair.publicKey))
  const signature = nacl.sign.detached(message, sigKeyPair.secretKey)

  const oneTimePrekeys: Array<{ key_id: number; pub: string; priv: string }> = []
  for (let i = 0; i < 10; i++) {
    const kp = nacl.box.keyPair()
    oneTimePrekeys.push({
      key_id: i + 1,
      pub: encodeBase64(kp.publicKey),
      priv: encodeBase64(kp.secretKey),
    })
  }

  return {
    identityKey: encodeBase64(identityKeyPair.publicKey),
    identitySecret: encodeBase64(identityKeyPair.secretKey),
    identitySigningKey: encodeBase64(sigKeyPair.publicKey),
    identitySigningSecret: encodeBase64(sigKeyPair.secretKey),
    signedPrekey: {
      key: encodeBase64(signedPrekeyPair.publicKey),
      signature: encodeBase64(signature),
      secret: encodeBase64(signedPrekeyPair.secretKey),
      key_id: 1,
    },
    oneTimePrekeys,
  }
}

/** One-time prekey with pub+priv for local storage and Signal protocol. */
export interface OneTimePrekeyEntry {
  key_id: number
  pub: string
  priv: string
}

/** Generate a new signed prekey for rotation. Uses identity signing secret to sign. */
export function generateSignedPrekey(
  identitySigningSecretB64: string,
  nextKeyId: number
): { key: string; signature: string; secret: string; key_id: number } {
  const signedPrekeyPair = nacl.box.keyPair()
  const message = decodeUTF8(encodeBase64(signedPrekeyPair.publicKey))
  const signature = nacl.sign.detached(message, decodeBase64(identitySigningSecretB64))
  return {
    key: encodeBase64(signedPrekeyPair.publicKey),
    signature: encodeBase64(signature),
    secret: encodeBase64(signedPrekeyPair.secretKey),
    key_id: nextKeyId,
  }
}

/** Generate one-time prekeys for replenishment (count, starting key_id). */
export function generateOneTimePrekeys(count: number, startKeyId: number): OneTimePrekeyEntry[] {
  const out: OneTimePrekeyEntry[] = []
  for (let i = 0; i < count; i++) {
    const kp = nacl.box.keyPair()
    out.push({
      key_id: startKeyId + i,
      pub: encodeBase64(kp.publicKey),
      priv: encodeBase64(kp.secretKey),
    })
  }
  return out
}
