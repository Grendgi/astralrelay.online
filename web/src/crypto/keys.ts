import * as nacl from 'tweetnacl'
import { decodeUTF8, encodeBase64 } from 'tweetnacl-util'

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

  const oneTimePrekeys: string[] = []
  for (let i = 0; i < 10; i++) {
    const kp = nacl.box.keyPair()
    oneTimePrekeys.push(encodeBase64(kp.publicKey))
  }

  return {
    identityKey: encodeBase64(identityKeyPair.publicKey),
    identitySecret: encodeBase64(identityKeyPair.secretKey),
    identitySigningKey: encodeBase64(sigKeyPair.publicKey),
    signedPrekey: {
      key: encodeBase64(signedPrekeyPair.publicKey),
      signature: encodeBase64(signature),
      secret: encodeBase64(signedPrekeyPair.secretKey),
      key_id: 1,
    },
    oneTimePrekeys,
  }
}

/** Generate one-time prekeys for replenishment (count, starting key_id). */
export function generateOneTimePrekeys(count: number, startKeyId: number): Array<{ key: string; key_id: number }> {
  const out: Array<{ key: string; key_id: number }> = []
  for (let i = 0; i < count; i++) {
    const kp = nacl.box.keyPair()
    out.push({ key: encodeBase64(kp.publicKey), key_id: startKeyId + i })
  }
  return out
}
