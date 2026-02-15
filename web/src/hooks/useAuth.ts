import { useState, useCallback, useEffect } from 'react'
import { api } from '../api/client'
import { generateKeys, generateSignedPrekey } from '../crypto/keys'
import { createBackup } from '../crypto/backup'
import { getKeysFromStorage, setKeysInStorage, clearKeysFromStorage, migrateKeysFromLocalStorage, mergeOtpksToStorage, updateSignedPrekeyInStorage, isKeysEncrypted, unlockKeysWithPassphrase, setKeysWithPassphrase } from '../crypto/key-storage'
import { initTrustedKeysStorage } from '../crypto/trusted-keys-storage'

function randomUUID(): string {
  if (typeof crypto !== 'undefined' && crypto.randomUUID) {
    return crypto.randomUUID()
  }
  // Fallback для старых браузеров и HTTP (без secure context)
  const bytes = new Uint8Array(16)
  ;(crypto as Crypto).getRandomValues(bytes)
  bytes[6] = (bytes[6]! & 0x0f) | 0x40
  bytes[8] = (bytes[8]! & 0x3f) | 0x80
  const hex = [...bytes].map((b) => b.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}

export interface AuthUser {
  user_id: string
  device_id: string
}

export interface OneTimePrekeyEntry {
  key_id: number
  pub: string
  priv: string
}

export interface StoredKeys {
  identityKey: string
  identitySecret: string
  identitySigningKey?: string
  identitySigningSecret?: string
  signedPrekey: { key: string; signature: string; secret: string; key_id?: number }
  oneTimePrekeys: OneTimePrekeyEntry[]
}

export function useAuth() {
  const [user, setUser] = useState<AuthUser | null>(() => {
    const u = localStorage.getItem('user')
    return u ? JSON.parse(u) : null
  })
  const [token, setToken] = useState<string | null>(() =>
    localStorage.getItem('token')
  )
  const [keys, setKeys] = useState<StoredKeys | null>(null)
  const [keysLocked, setKeysLocked] = useState(false)

  useEffect(() => {
    if (!user) {
      setKeys(null)
      setKeysLocked(false)
      return
    }
    let cancelled = false
    ;(async () => {
      await initTrustedKeysStorage()
      const migrated = await migrateKeysFromLocalStorage()
      const k = migrated ?? (await getKeysFromStorage())
      if (!cancelled) {
        setKeys(k)
        setKeysLocked(k == null && (await isKeysEncrypted()))
      }
    })()
    return () => { cancelled = true }
  }, [user])

  const register = useCallback(
    async (username: string, password: string) => {
      const keys = await generateKeys()
      const deviceId = randomUUID()
      const salt = crypto.getRandomValues(new Uint8Array(32))
      const saltB64 = btoa(String.fromCharCode(...salt))
      const keysBackupBlob = await createBackup(
        {
          identityKey: keys.identityKey,
          identitySecret: keys.identitySecret,
          signedPrekey: keys.signedPrekey,
          oneTimePrekeys: keys.oneTimePrekeys.map((o) => o.pub),
        },
        password,
        saltB64
      )
      const res = await api.register({
        username,
        password,
        device_id: deviceId,
        keys: {
          identity_key: keys.identityKey,
          signed_prekey: {
            key: keys.signedPrekey.key,
            signature: keys.signedPrekey.signature,
            key_id: keys.signedPrekey.key_id ?? 1,
          },
          one_time_prekeys: keys.oneTimePrekeys.slice(0, 5).map((o) => ({
            key: o.pub,
            key_id: o.key_id,
          })),
        },
        keys_backup: { salt: saltB64, blob: keysBackupBlob },
      })
      localStorage.setItem('user', JSON.stringify({ user_id: res.user_id, device_id: res.device_id }))
      localStorage.setItem('device_id', res.device_id)
      localStorage.setItem('token', res.access_token)
      const stored: StoredKeys = {
        identityKey: keys.identityKey,
        identitySecret: keys.identitySecret,
        identitySigningKey: keys.identitySigningKey,
        ...('identitySigningSecret' in keys && { identitySigningSecret: (keys as { identitySigningSecret: string }).identitySigningSecret }),
        signedPrekey: keys.signedPrekey,
        oneTimePrekeys: keys.oneTimePrekeys,
      }
      await setKeysInStorage(stored)
      setUser({ user_id: res.user_id, device_id: res.device_id })
      setToken(res.access_token)
      setKeys(stored)
    },
    []
  )

  const login = useCallback(
    async (username: string, password: string) => {
      const storedDeviceId = localStorage.getItem('device_id')
      const migrated = await migrateKeysFromLocalStorage()
      const storedKeys = migrated ?? (await getKeysFromStorage())
      const isNewDevice = !storedDeviceId
      const needKeysRestore = !storedKeys
      const devId = storedDeviceId || randomUUID()
      let keys: { identityKey: string; identitySecret: string; signedPrekey: { key: string; signature: string; secret: string; key_id?: number }; oneTimePrekeys: OneTimePrekeyEntry[] } | null = null
      if (isNewDevice && !needKeysRestore) {
        keys = (await generateKeys()) as any
      }
      const body: Parameters<typeof api.login>[0] = {
        username,
        password,
        device_id: devId,
        request_keys_restore: needKeysRestore,
      }
      if (keys) {
        body.keys = {
          identity_key: keys.identityKey,
          signed_prekey: { key: keys.signedPrekey.key, signature: keys.signedPrekey.signature, key_id: keys.signedPrekey.key_id ?? 1 },
          one_time_prekeys: keys.oneTimePrekeys.slice(0, 5).map((o) => ({ key: o.pub, key_id: o.key_id })),
        }
      }
      const res = await api.login(body)
      localStorage.setItem('user', JSON.stringify({ user_id: res.user_id, device_id: res.device_id }))
      localStorage.setItem('device_id', res.device_id)
      localStorage.setItem('token', res.access_token)
      if (keys) {
        const stored: StoredKeys = {
          ...keys,
          oneTimePrekeys: keys.oneTimePrekeys,
          ...('identitySigningSecret' in keys && { identitySigningSecret: (keys as { identitySigningSecret: string }).identitySigningSecret }),
        }
        await setKeysInStorage(stored)
        setKeys(stored)
      } else if (res.keys_backup && needKeysRestore) {
        try {
          const { restoreBackup } = await import('../crypto/backup')
          const restored = await restoreBackup(res.keys_backup.blob, password, res.keys_backup.salt)
          const stored: StoredKeys = {
            identityKey: restored.identityKey,
            identitySecret: restored.identitySecret,
            identitySigningKey: restored.identitySigningKey,
            ...(restored.identitySigningSecret && { identitySigningSecret: restored.identitySigningSecret }),
            signedPrekey: restored.signedPrekey,
            oneTimePrekeys: [],
          }
          await setKeysInStorage(stored)
          window.location.reload()
          return
        } catch {
          // не удалось восстановить — остаёмся без ключей
        }
      }
      setUser({ user_id: res.user_id, device_id: res.device_id })
      setToken(res.access_token)
    },
    []
  )

  const refreshKeys = useCallback(async () => {
    if (!user) return
    const k = await getKeysFromStorage()
    setKeys(k)
    setKeysLocked(k == null && (await isKeysEncrypted()))
  }, [user])

  const unlockKeys = useCallback(async (passphrase: string) => {
    const k = await unlockKeysWithPassphrase(passphrase)
    setKeys(k)
    setKeysLocked(false)
  }, [])

  const lockKeysWithPassphrase = useCallback(async (passphrase: string) => {
    const k = await getKeysFromStorage()
    if (!k) return
    await setKeysWithPassphrase(k, passphrase)
    setKeys(null)
    setKeysLocked(true)
  }, [])

  const addOtpks = useCallback(async (entries: OneTimePrekeyEntry[]) => {
    await mergeOtpksToStorage(entries)
    const k = await getKeysFromStorage()
    setKeys(k)
  }, [])

  const rotateSignedPrekey = useCallback(
    async (newSignedPrekey: { key: string; signature: string; secret: string; key_id: number }) => {
      await updateSignedPrekeyInStorage(newSignedPrekey)
      const k = await getKeysFromStorage()
      setKeys(k)
    },
    []
  )

  const logout = useCallback(() => {
    const t = token
    if (t) {
      api.logout(t).catch(() => {}) // fire-and-forget: revoke on server
    }
    localStorage.removeItem('user')
    localStorage.removeItem('token')
    localStorage.removeItem('device_id')
    clearKeysFromStorage().catch(() => {})
    setUser(null)
    setToken(null)
  }, [token])

  return { user, token, keys, keysLocked, register, login, logout, refreshKeys, addOtpks, rotateSignedPrekey, unlockKeys, lockKeysWithPassphrase }
}
