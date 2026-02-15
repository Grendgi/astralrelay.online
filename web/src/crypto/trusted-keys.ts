/**
 * Store trusted identity_key per contact for key-change detection.
 * Persisted in IndexedDB (not localStorage) for consistency with E2EE keys.
 * When identity_key changes — possible MITM or reinstall; user should verify.
 */
export {
  getStoredIdentityKey,
  setStoredIdentityKey,
  checkIdentityKeyChange,
  initTrustedKeysStorage,
} from './trusted-keys-storage'
