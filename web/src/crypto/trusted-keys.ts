/**
 * Store trusted identity_key per contact for key-change detection.
 * Trust states: seen (TOFU), verified (safety number confirmed), changed (key differs).
 * Persisted in IndexedDB (not localStorage) for consistency with E2EE keys.
 */
export {
  getStoredIdentityKey,
  setStoredIdentityKey,
  checkIdentityKeyChange,
  initTrustedKeysStorage,
  markVerified,
  getTrustState,
  getTrustedKeysForBackup,
  restoreTrustedKeysFromBackup,
} from './trusted-keys-storage'
export type { TrustState, TrustStatus, TrustEntry, VerifiedMethod } from './trusted-keys-storage'