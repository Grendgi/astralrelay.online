/**
 * E2EE mode: Strict = no silent fallback to MVP (Signal only or error);
 * Compatibility = allow fallback to MVP when Signal fails.
 */
export type E2EEMode = 'strict' | 'compatibility'

const STORAGE_KEY = 'e2ee_mode'

export function getE2EEMode(): E2EEMode {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v === 'strict' || v === 'compatibility') return v
  } catch {
    // ignore
  }
  return 'compatibility'
}

export function setE2EEMode(mode: E2EEMode): void {
  try {
    localStorage.setItem(STORAGE_KEY, mode)
  } catch {
    // ignore
  }
}
