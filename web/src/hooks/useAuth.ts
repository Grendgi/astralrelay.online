import { useState, useCallback } from 'react'
import { api } from '../api/client'
import { generateKeys } from '../crypto/keys'
import { createBackup } from '../crypto/backup'

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

export interface StoredKeys {
  identityKey: string
  identitySecret: string
  identitySigningKey?: string
  signedPrekey: { key: string; signature: string; secret: string }
  oneTimePrekeys: string[]
}

export function useAuth() {
  const [user, setUser] = useState<AuthUser | null>(() => {
    const u = localStorage.getItem('user')
    return u ? JSON.parse(u) : null
  })
  const [token, setToken] = useState<string | null>(() =>
    localStorage.getItem('token')
  )
  const keys = user ? ((): StoredKeys | null => {
    try {
      const k = localStorage.getItem('keys')
      return k ? JSON.parse(k) : null
    } catch {
      return null
    }
  })() : null

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
          oneTimePrekeys: keys.oneTimePrekeys,
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
            key_id: 1,
          },
          one_time_prekeys: keys.oneTimePrekeys.slice(0, 5).map((k, i) => ({
            key: k,
            key_id: i + 1,
          })),
        },
        keys_backup: { salt: saltB64, blob: keysBackupBlob },
      })
      localStorage.setItem('user', JSON.stringify({ user_id: res.user_id, device_id: res.device_id }))
      localStorage.setItem('device_id', res.device_id)
      localStorage.setItem('token', res.access_token)
      localStorage.setItem('keys', JSON.stringify({
        identityKey: keys.identityKey,
        identitySecret: keys.identitySecret,
        identitySigningKey: keys.identitySigningKey,
        signedPrekey: keys.signedPrekey,
        oneTimePrekeys: keys.oneTimePrekeys,
      }))
      setUser({ user_id: res.user_id, device_id: res.device_id })
      setToken(res.access_token)
    },
    []
  )

  const login = useCallback(
    async (username: string, password: string) => {
      const storedDeviceId = localStorage.getItem('device_id')
      const storedKeys = (() => {
        try {
          const k = localStorage.getItem('keys')
          return k ? JSON.parse(k) : null
        } catch {
          return null
        }
      })()
      const isNewDevice = !storedDeviceId
      const needKeysRestore = !storedKeys
      const devId = storedDeviceId || randomUUID()
      let keys: { identityKey: string; identitySecret: string; signedPrekey: { key: string; signature: string; secret: string }; oneTimePrekeys: string[] } | null = null
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
          signed_prekey: { key: keys.signedPrekey.key, signature: keys.signedPrekey.signature, key_id: 1 },
          one_time_prekeys: keys.oneTimePrekeys.slice(0, 5).map((k, i) => ({ key: k, key_id: i + 1 })),
        }
      }
      const res = await api.login(body)
      localStorage.setItem('user', JSON.stringify({ user_id: res.user_id, device_id: res.device_id }))
      localStorage.setItem('device_id', res.device_id)
      localStorage.setItem('token', res.access_token)
      if (keys) {
        localStorage.setItem('keys', JSON.stringify(keys))
      } else if (res.keys_backup && needKeysRestore) {
        try {
          const { restoreBackup } = await import('../crypto/backup')
          const restored = await restoreBackup(res.keys_backup.blob, password, res.keys_backup.salt)
          localStorage.setItem('keys', JSON.stringify({
            identityKey: restored.identityKey,
            identitySecret: restored.identitySecret,
            identitySigningKey: restored.identitySigningKey,
            signedPrekey: restored.signedPrekey,
            oneTimePrekeys: restored.oneTimePrekeys ?? [],
          }))
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

  const logout = useCallback(() => {
    localStorage.removeItem('user')
    localStorage.removeItem('token')
    localStorage.removeItem('device_id')
    localStorage.removeItem('keys')
    setUser(null)
    setToken(null)
  }, [])

  return { user, token, keys, register, login, logout }
}
