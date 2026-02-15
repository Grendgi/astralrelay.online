/**
 * Store last known identity_key per contact for key-change detection.
 * When identity_key changes — possible MITM or reinstall; user should verify.
 */
const STORAGE_KEY = 'e2ee_identity_keys'

function load(): Record<string, string> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return {}
    const parsed = JSON.parse(raw)
    return typeof parsed === 'object' && parsed !== null ? parsed : {}
  } catch {
    return {}
  }
}

function save(map: Record<string, string>): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(map))
  } catch {
    // ignore
  }
}

/** Get stored identity_key for a contact. */
export function getStoredIdentityKey(recipient: string): string | null {
  const map = load()
  return map[recipient] ?? null
}

/** Store identity_key for a contact (after verification or first contact). */
export function setStoredIdentityKey(recipient: string, identityKey: string): void {
  const map = load()
  map[recipient] = identityKey
  save(map)
}

/**
 * Check if identity_key has changed since last seen.
 * Returns { changed: true, previousKey } if different; { changed: false } otherwise.
 */
export function checkIdentityKeyChange(recipient: string, currentIdentityKey: string): { changed: false } | { changed: true; previousKey: string } {
  const stored = getStoredIdentityKey(recipient)
  if (!stored) return { changed: false }
  if (stored === currentIdentityKey) return { changed: false }
  return { changed: true, previousKey: stored }
}
