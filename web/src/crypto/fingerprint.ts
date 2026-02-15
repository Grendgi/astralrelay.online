/**
 * Safety number / fingerprint for E2EE verification.
 * Users compare fingerprints out-of-band to detect MITM.
 */

function b64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64)
  const arr = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) arr[i] = bin.charCodeAt(i)
  return arr
}

/** SHA-256 digest (SubtleCrypto). */
async function sha256(data: Uint8Array): Promise<Uint8Array> {
  const hash = await crypto.subtle.digest('SHA-256', data)
  return new Uint8Array(hash)
}

/**
 * Compute safety number between two identity public keys (base64).
 * Deterministic: fingerprint(A,B) === fingerprint(B,A).
 * Returns 60 digits in groups of 5, e.g. "12345 12345 12345 ..."
 */
export async function computeSafetyNumber(ourIdentityPubB64: string, theirIdentityPubB64: string): Promise<string> {
  const ours = b64ToBytes(ourIdentityPubB64)
  const theirs = b64ToBytes(theirIdentityPubB64)
  // Normalize order so fingerprint(A,B) === fingerprint(B,A)
  let a: Uint8Array
  let b: Uint8Array
  if (ours.length !== theirs.length) {
    a = ours.length < theirs.length ? ours : theirs
    b = ours.length < theirs.length ? theirs : ours
  } else {
    let cmp = 0
    for (let i = 0; i < ours.length && cmp === 0; i++) cmp = ours[i]! - theirs[i]!
    a = cmp <= 0 ? ours : theirs
    b = cmp <= 0 ? theirs : ours
  }
  const combined = new Uint8Array(a.length + b.length)
  combined.set(a, 0)
  combined.set(b, a.length)
  const hash = await sha256(combined)
  // Encode as numeric string (base 10, 5 digits per group)
  let num = 0n
  for (let i = 0; i < hash.length; i++) num = (num << 8n) | BigInt(hash[i])
  const str = num.toString().padStart(60, '0').slice(-60)
  const parts: string[] = []
  for (let i = 0; i < 60; i += 5) parts.push(str.slice(i, i + 5))
  return parts.join(' ')
}
