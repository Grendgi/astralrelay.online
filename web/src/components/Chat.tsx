import { useState, useEffect, useCallback, useRef, type ReactNode } from 'react'
import './Chat.css'
import { api, ApiError, type SyncEvent } from '../api/client'
import type { AuthUser, StoredKeys } from '../hooks/useAuth'
import { setKeysInStorage } from '../crypto/key-storage'

const IMAGE_EXTS = /\.(jpe?g|png|gif|webp|bmp|svg)(\?|$)/i

function playNotificationSound(): void {
  try {
    const Ctx = window.AudioContext || (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext
    if (!Ctx) return
    const ctx = new Ctx()
    const osc = ctx.createOscillator()
    const gain = ctx.createGain()
    osc.connect(gain)
    gain.connect(ctx.destination)
    osc.frequency.value = 880
    osc.type = 'sine'
    gain.gain.setValueAtTime(0.15, ctx.currentTime)
    gain.gain.exponentialRampToValueAtTime(0.01, ctx.currentTime + 0.15)
    osc.start(ctx.currentTime)
    osc.stop(ctx.currentTime + 0.15)
  } catch {
    // ignore
  }
}
const VIDEO_EXTS = /\.(mp4|webm|ogg|mov)(\?|$)/i
const AUDIO_EXTS = /\.(mp3|wav|ogg|m4a|aac)(\?|$)/i
const PDF_EXTS = /\.pdf(\?|$)/i

function formatFileSize(n: number): string {
  if (n < 1024) return `${n} Б`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} КБ`
  return `${(n / 1024 / 1024).toFixed(1)} МБ`
}

function FilePreview({
  contentUri,
  filename,
  size,
  token,
  onDownload,
  lazy = true,
  fileKey,
  nonce,
  fileSha256,
  chunkSize,
  e2eeVersion,
}: {
  contentUri: string
  filename: string
  size?: number
  token: string
  onDownload: () => void
  lazy?: boolean
  fileKey?: string
  nonce?: string
  fileSha256?: string
  chunkSize?: number
  e2eeVersion?: number
}) {
  const [url, setUrl] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [inView, setInView] = useState(!lazy)
  const [lightboxOpen, setLightboxOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)
  const isImage = IMAGE_EXTS.test(filename)
  const isVideo = VIDEO_EXTS.test(filename)
  const isAudio = AUDIO_EXTS.test(filename)
  const isPdf = PDF_EXTS.test(filename)
  const canPreview = isImage || isVideo || isAudio || isPdf

  useEffect(() => {
    if (!lazy || !containerRef.current) return
    const el = containerRef.current
    const obs = new IntersectionObserver(
      ([entry]) => { if (entry?.isIntersecting) setInView(true) },
      { rootMargin: '100px', threshold: 0 }
    )
    obs.observe(el)
    return () => obs.disconnect()
  }, [lazy])

  useEffect(() => {
    if (!lightboxOpen) return
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setLightboxOpen(false) }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [lightboxOpen])

  useEffect(() => {
    if (!canPreview || !contentUri || (lazy && !inView)) {
      if (!canPreview || !contentUri) setLoading(false)
      return
    }
    let revoked = false
    setError(false)
    api.downloadFile(contentUri, token)
      .then(async (res) => {
        if (!res.ok) throw new Error('Download failed')
        const ct = res.headers.get('content-type') || ''
        if (ct.includes('text/html') || ct.includes('application/json')) {
          throw new Error('Received error page instead of file')
        }
        let blob: Blob = await res.blob()
        if (fileKey && (nonce || chunkSize)) {
          validateAttachmentVersion(e2eeVersion)
          const ciphertext = new Uint8Array(await blob.arrayBuffer())
          const plain = await decryptAttachment(ciphertext, fileKey, nonce ?? '', fileSha256, chunkSize)
          blob = new Blob([plain as BlobPart])
        }
        // Браузеру нужен корректный MIME для отображения img/video (octet-stream может не рендериться)
        const ext = filename.split('.').pop()?.toLowerCase()
        const mimeMap: Record<string, string> = {
          png: 'image/png', jpg: 'image/jpeg', jpeg: 'image/jpeg', gif: 'image/gif',
          webp: 'image/webp', bmp: 'image/bmp', svg: 'image/svg+xml',
          mp4: 'video/mp4', webm: 'video/webm', ogg: 'video/ogg', mov: 'video/quicktime',
          mp3: 'audio/mpeg', wav: 'audio/wav', m4a: 'audio/mp4', aac: 'audio/aac',
          pdf: 'application/pdf',
        }
        const mime = ext ? mimeMap[ext] : undefined
        const displayBlob = mime ? new Blob([await blob.arrayBuffer()], { type: mime }) : blob
        if (revoked) return
        setUrl(URL.createObjectURL(displayBlob))
        setLoading(false)
      })
      .catch(() => {
        if (!revoked) {
          setError(true)
          setLoading(false)
        }
      })
    return () => {
      revoked = true
      setUrl((u) => {
        if (u) URL.revokeObjectURL(u)
        return null
      })
    }
  }, [contentUri, token, canPreview, filename, lazy, inView, fileKey, nonce, fileSha256, chunkSize, e2eeVersion])

  const sizeStr = size != null ? formatFileSize(size) : null

  const wrap = (children: ReactNode) => (
    <div ref={containerRef} className="fp-wrap">
      {children}
    </div>
  )

  if (!canPreview || error) {
    return wrap(
      <button type="button" onClick={onDownload} className="fp-download-btn">
        📎 {filename}
        {sizeStr && <span className="fp-size">({sizeStr})</span>}
      </button>
    )
  }

  if (loading) {
    return wrap(
      <div className="fp-download-btn fp-download-btn--loading">
        ⏳ Загрузка {filename}…
      </div>
    )
  }

  if (isImage && url) {
    return wrap(
      <>
        <div className="fp-preview">
          <img
            src={url}
            alt={filename}
            className="fp-image"
            onClick={(e) => { e.stopPropagation(); setLightboxOpen(true) }}
            title="Клик — открыть в полном размере"
            onError={() => {
              setUrl((u) => { if (u) URL.revokeObjectURL(u); return null })
              setError(true)
            }}
          />
          <button type="button" onClick={onDownload} className="fp-download-btn">
            📎 {filename}
            {sizeStr && <span className="fp-size">({sizeStr})</span>}
          </button>
        </div>
        {lightboxOpen && (
          <div
            role="dialog"
            aria-modal="true"
            className="fp-lightbox-backdrop"
            onClick={() => setLightboxOpen(false)}
          >
            <img
              src={url}
              alt={filename}
              className="fp-lightbox-image"
              onClick={(e) => e.stopPropagation()}
            />
            <button
              type="button"
              onClick={() => setLightboxOpen(false)}
              className="fp-lightbox-close"
              aria-label="Закрыть"
            >
              ✕
            </button>
          </div>
        )}
      </>
    )
  }

  if (isVideo && url) {
    return wrap(
      <div className="fp-preview">
        <video src={url} controls className="fp-video" />
        <button type="button" onClick={onDownload} className="fp-download-btn">
          📎 {filename}
          {sizeStr && <span className="fp-size">({sizeStr})</span>}
        </button>
      </div>
    )
  }

  if (isAudio && url) {
    return wrap(
      <div className="fp-preview">
        <audio src={url} controls className="fp-audio" />
        <button type="button" onClick={onDownload} className="fp-download-btn">
          📎 {filename}
          {sizeStr && <span className="fp-size">({sizeStr})</span>}
        </button>
      </div>
    )
  }

  if (isPdf && url) {
    return wrap(
      <div className="fp-preview">
        <iframe src={url} title={filename} className="fp-pdf" />
        <button type="button" onClick={onDownload} className="fp-download-btn">
          📎 {filename}
          {sizeStr && <span className="fp-size">({sizeStr})</span>}
        </button>
      </div>
    )
  }

  return wrap(null)
}
import { encrypt, decrypt, isE2EEPayload, encryptAttachmentFromFile, decryptAttachment, E2EE_ATTACHMENT_VERSION, MAX_ATTACHMENT_SIZE, isValidBase64, validateAttachmentVersion } from '../crypto/e2ee'
import { computeSafetyNumber } from '../crypto/fingerprint'
import { safetyNumberToQRDataUrl } from '../crypto/fingerprint-qr'
import { signalEncrypt, signalDecrypt, isSignalCiphertext, uuidToSignalDeviceId, deleteSessionForRecipientDevice } from '../crypto/signal'
import {
  generateSenderKey,
  storeSenderKey,
  getSenderKey,
  deleteSenderKey,
  encryptWithSenderKey,
  decryptWithSenderKey,
  buildDistributionPayload,
  parseDistributionPayload,
  memberSetEquals,
} from '../crypto/sender-keys'
import { getE2EEMode, setE2EEMode, type E2EEMode } from '../crypto/e2ee-mode'
import { getStoredIdentityKey, setStoredIdentityKey, checkIdentityKeyChange, markVerified, clearVerified, getTrustedKeysForBackup, restoreTrustedKeysFromBackup, getRecipientHasVerifiedDevice, getTrustState } from '../crypto/trusted-keys'
import { createBackup, restoreBackup } from '../crypto/backup'
import { logError } from '../utils/safe-log'
import { sanitizeForDisplay, sanitizeFilename } from '../utils/sanitize'
import { generateOneTimePrekeys, generateSignedPrekey } from '../crypto/keys'
import { useTheme } from '../hooks/useTheme'

function decodeBase64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64)
  return Uint8Array.from(bin, (c) => c.charCodeAt(0))
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} Б`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} КБ`
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} МБ`
  return `${(n / 1024 / 1024 / 1024).toFixed(1)} ГБ`
}

function formatTime(ts: number): string {
  const d = new Date(ts * 1000)
  const now = new Date()
  const isToday = d.toDateString() === now.toDateString()
  return isToday ? d.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' }) : d.toLocaleDateString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
}

function formatTrustDate(ms?: number): string {
  if (ms == null) return '—'
  const d = new Date(ms)
  return d.toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' })
}

/** Нормализует recipient в MXID/room ID: если нет домена — добавляет домен текущего пользователя */
function normalizeRecipient(recipient: string, myUserID: string): string {
  const r = recipient.trim()
  if (!r) return r
  if (r.includes(':')) return r
  const domain = myUserID.includes(':') ? myUserID.slice(myUserID.indexOf(':') + 1) : 'localhost'
  // Комнаты: !uuid или @user
  if (r.startsWith('!')) return r + `:${domain}`
  return (r.startsWith('@') ? r : `@${r}`) + `:${domain}`
}

function isRoomAddr(addr: string): boolean {
  return addr.startsWith('!')
}

interface ChatProps {
  user: AuthUser
  token: string
  keys: StoredKeys | null
  onLogout: () => void
  addOtpks?: (entries: import('../hooks/useAuth').OneTimePrekeyEntry[]) => Promise<void>
  rotateSignedPrekey?: (spk: { key: string; signature: string; secret: string; key_id: number }) => Promise<void>
  lockKeysWithPassphrase?: (passphrase: string) => Promise<void>
}

export function Chat({ user, token, keys, onLogout, addOtpks, rotateSignedPrekey, lockKeysWithPassphrase }: ChatProps) {
  const { theme, toggleTheme } = useTheme()
  const [recipient, setRecipient] = useState(() => sessionStorage.getItem('chat_recipient') ?? '')
  const [message, setMessage] = useState('')
  const [events, setEvents] = useState<SyncEvent[]>([])
  const [cursor, setCursor] = useState('')
  const [uploading, setUploading] = useState(false)
  const [vpnProtocols, setVpnProtocols] = useState<Array<{ id: string; name: string; hint: string }>>([])
  const [vpnNodes, setVpnNodes] = useState<Array<{ id: string; name: string; region: string; is_default: boolean; ping_url?: string }>>([])
  const [vpnNodeLatencies, setVpnNodeLatencies] = useState<Record<string, number>>({})
  const [vpnMyConfigs, setVpnMyConfigs] = useState<Array<{ device_id: string; protocol: string; node_name?: string; created_at: string; expires_at?: string; is_expired: boolean; traffic_rx_bytes?: number; traffic_tx_bytes?: number; traffic_limit_bytes?: number }>>([])
  const [vpnLoading, setVpnLoading] = useState<string | null>(null)
  const [backupLoading, setBackupLoading] = useState(false)
  const [backupError, setBackupError] = useState<string | null>(null)
  const [lockPassphrase, setLockPassphrase] = useState('')
  const [lockPassphraseConfirm, setLockPassphraseConfirm] = useState('')
  const [lockError, setLockError] = useState('')
  const [lockLoading, setLockLoading] = useState(false)
  const [sendHint, setSendHint] = useState<string | null>(null)
  const [sendHintError, setSendHintError] = useState(false)
  const [sending, setSending] = useState(false)
  const [rooms, setRooms] = useState<Array<{ id: string; name: string; domain: string; address: string }>>([])
  const [roomsLoading, setRoomsLoading] = useState(false)
  const [createRoomName, setCreateRoomName] = useState('')
  const [createRoomLoading, setCreateRoomLoading] = useState(false)
  const [roomMembers, setRoomMembers] = useState<Array<{ user_id: number; username: string; domain: string; address: string; role: string }>>([])
  const [roomMembersVisible, setRoomMembersVisible] = useState(false)
  const [searchResults, setSearchResults] = useState<Array<{ user_id: string }>>([])
  const searchDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [inviteUsername, setInviteUsername] = useState('')
  const [inviteLoading, setInviteLoading] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const [mainView, setMainView] = useState<'chats' | 'vpn' | 'devices'>(() => (sessionStorage.getItem('main_view') as 'chats' | 'vpn' | 'devices') || 'chats')
  useEffect(() => {
    sessionStorage.setItem('main_view', mainView)
  }, [mainView])
  const [leaveLoading, setLeaveLoading] = useState(false)
  const [roomActionLoading, setRoomActionLoading] = useState<string | null>(null)
  const [devices, setDevices] = useState<Array<{ device_id: string; name?: string; created_at: string; is_current: boolean }>>([])
  const [devicesLoading, setDevicesLoading] = useState(false)
  const [devicesRevokeLoading, setDevicesRevokeLoading] = useState<string | null>(null)
  const [devicesRenameLoading, setDevicesRenameLoading] = useState<string | null>(null)
  const [soundEnabled, setSoundEnabled] = useState(() => {
    try {
      return localStorage.getItem('chat_sound_enabled') !== 'false'
    } catch { return true }
  })
  const [pushEnabled, setPushEnabled] = useState(false)
  const [pushLoading, setPushLoading] = useState(false)
  const [decryptedSignal, setDecryptedSignal] = useState<Record<string, string>>({})
  const [fingerprintModal, setFingerprintModal] = useState<{ recipient: string; safetyNumber: string; identityKey?: string; deviceId?: string; trustHistory?: { seenAt?: number; verifiedAt?: number; changedAt?: number; status?: string } } | null>(null)
  const [fingerprintQrUrl, setFingerprintQrUrl] = useState<string | null>(null)
  const [fingerprintLoading, setFingerprintLoading] = useState(false)
  const [fingerprintCopied, setFingerprintCopied] = useState(false)
  const [backupRestoredHint, setBackupRestoredHint] = useState(() => !!sessionStorage.getItem('backup_restored'))
  const [e2eeMode, setE2eeModeState] = useState<E2EEMode>(() => getE2EEMode())
  const setE2eeMode = (m: E2EEMode) => {
    setE2eeModeState(m)
    setE2EEMode(m)
  }
  const [identityKeyChanged, setIdentityKeyChanged] = useState<{ recipient: string; newKey: string; deviceId?: string } | null>(null)
  useEffect(() => {
    if ('serviceWorker' in navigator && 'PushManager' in window && Notification.permission === 'granted') {
      navigator.serviceWorker.ready.then((reg) => {
        reg.pushManager.getSubscription().then((sub) => {
          if (sub) setPushEnabled(true)
        })
      })
    }
  }, [])
  useEffect(() => {
    try {
      localStorage.setItem('chat_sound_enabled', String(soundEnabled))
    } catch {
      //
    }
  }, [soundEnabled])
  const restoreInputRef = useRef<HTMLInputElement>(null)
  const messagesEndRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!fingerprintModal) return
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setFingerprintModal(null) }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [fingerprintModal])

  useEffect(() => {
    if (!fingerprintModal || fingerprintModal.safetyNumber.includes('—')) {
      setFingerprintQrUrl(null)
      return
    }
    let cancelled = false
    safetyNumberToQRDataUrl(fingerprintModal.safetyNumber)
      .then((url) => { if (!cancelled) setFingerprintQrUrl(url) })
      .catch(() => { if (!cancelled) setFingerprintQrUrl(null) })
    return () => { cancelled = true }
  }, [fingerprintModal])

  const trustNewKey = useCallback(async () => {
    if (!identityKeyChanged) return
    await setStoredIdentityKey(identityKeyChanged.recipient, identityKeyChanged.newKey, identityKeyChanged.deviceId)
    setIdentityKeyChanged(null)
  }, [identityKeyChanged])

  const showFingerprint = useCallback(async () => {
    const r = normalizeRecipient(recipient, user.user_id)
    if (!r || isRoomAddr(r) || !keys) return
    setFingerprintLoading(true)
    setFingerprintModal(null)
    try {
      const bundle = await api.getKeys(r, token)
      const safetyNumber = await computeSafetyNumber(keys.identityKey, bundle.identity_key)
      const trust = await getTrustState(r, bundle.identity_key, bundle.device_id)
      const trustHistory = trust
        ? { seenAt: trust.seenAt, verifiedAt: trust.verifiedAt, changedAt: trust.changedAt, status: trust.status }
        : undefined
      setFingerprintModal({ recipient: r, safetyNumber, identityKey: bundle.identity_key, deviceId: bundle.device_id, trustHistory })
    } catch {
      setFingerprintModal({ recipient: r, safetyNumber: '— ключи недоступны —' })
    } finally {
      setFingerprintLoading(false)
    }
  }, [recipient, user.user_id, keys, token])

  // Check for identity key change when opening DM (уведомления при смене ключей)
  useEffect(() => {
    const r = normalizeRecipient(recipient, user.user_id)
    if (!r || isRoomAddr(r) || !keys || !token) {
      setIdentityKeyChanged(null)
      return
    }
    let cancelled = false
    api.getKeys(r, token)
      .then(async (bundle) => {
        if (cancelled) return
        const res = await checkIdentityKeyChange(r, bundle.identity_key, bundle.device_id)
        if (res.changed) {
          setIdentityKeyChanged({ recipient: r, newKey: bundle.identity_key, deviceId: bundle.device_id })
        } else {
          setIdentityKeyChanged(null)
          const stored = await getStoredIdentityKey(r, bundle.device_id)
          if (!stored) await setStoredIdentityKey(r, bundle.identity_key, bundle.device_id)
        }
      })
      .catch(() => {
        if (!cancelled) setIdentityKeyChanged(null)
      })
    return () => { cancelled = true }
  }, [recipient, user.user_id, keys, token])

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [events, recipient])

  // Асинхронная расшифровка Signal (sig1:) и Sender Key (sk1:) сообщений
  useEffect(() => {
    const ourKeys = keys ? { identityKey: keys.identityKey, identitySecret: keys.identitySecret, signedPrekey: keys.signedPrekey } : null
    events.forEach((ev) => {
      const ct = ev.ciphertext
      if (!ct) return
      if (ct.startsWith('sig1:')) {
        if (!ourKeys?.identitySecret || !ourKeys?.signedPrekey?.secret) return
        signalDecrypt(ct, ourKeys, ev.sender, ev.sender_device).then(
          (plain) => {
            const dist = parseDistributionPayload(plain)
            if (dist) {
              const bin = atob(dist.key)
              const arr = new Uint8Array(bin.length)
              for (let i = 0; i < bin.length; i++) arr[i] = bin.charCodeAt(i)
              storeSenderKey(dist.roomId, dist.senderId, dist.senderDeviceId, arr, dist.keyId).catch(() => {})
              plain = dist.body ?? '[key received]'
            }
            setDecryptedSignal((prev) => (prev[ev.event_id] ? prev : { ...prev, [ev.event_id]: plain }))
          },
          () => setDecryptedSignal((prev) => (prev[ev.event_id] ? prev : { ...prev, [ev.event_id]: '[decrypt failed]' }))
        )
      } else if (ct.startsWith('sk1:')) {
        const roomId = ev.recipient?.startsWith('!') ? ev.recipient.slice(1).split(':')[0] : ''
        if (!roomId || !ev.sender || !ev.sender_device) return
        getSenderKey(roomId, ev.sender, ev.sender_device)
          .then((sk) => {
            if (!sk) {
              setDecryptedSignal((prev) => (prev[ev.event_id] ? prev : { ...prev, [ev.event_id]: '[no sender key]' }))
              return
            }
            const plain = decryptWithSenderKey(ct, sk.key)
            setDecryptedSignal((prev) => (prev[ev.event_id] ? prev : { ...prev, [ev.event_id]: plain }))
          })
          .catch(() => setDecryptedSignal((prev) => (prev[ev.event_id] ? prev : { ...prev, [ev.event_id]: '[decrypt failed]' })))
      }
    })
  }, [events, keys])

  const sync = useCallback(async () => {
    try {
      const res = await api.sync(cursor, token)
      if (res.events.length) {
        // API returns ciphertext as base64; decode for sig1/sk1 (raw prefix+base64)
        const norm = (e: SyncEvent) => {
          let ct = e.ciphertext
          if (ct) {
            try {
              const d = atob(ct)
              if (d.startsWith('sig1:') || d.startsWith('sk1:')) ct = d
            } catch { /* keep as-is */ }
          }
          return { ...e, ciphertext: ct }
        }
        let newIncomingCount = 0
        setEvents((prev) => {
          const ids = new Set(prev.map((e) => e.event_id))
          const newEvents = res.events.filter((e) => !ids.has(e.event_id)).map(norm)
          newIncomingCount = newEvents.filter((e) => e.sender !== user.user_id).length
          return newEvents.length ? [...prev, ...newEvents] : prev
        })
        setCursor(res.next_cursor)
        if (newIncomingCount > 0 && soundEnabled) playNotificationSound()
      }
    } catch {
      // ignore
    }
  }, [token, cursor, user.user_id, soundEnabled])

  const syncRef = useRef(sync)
  syncRef.current = sync

  useEffect(() => {
    sync()
    const id = setInterval(sync, 5000)
    return () => clearInterval(id)
  }, [sync])

  useEffect(() => {
    if (mainView === 'devices' && token) {
      setDevicesLoading(true)
      api.listDevices(token)
        .then((r) => setDevices(r.devices))
        .catch(() => setDevices([]))
        .finally(() => setDevicesLoading(false))
    }
  }, [mainView, token])

  // Keys hygiene: replenish OTPK when < 20; rotate signed prekey when > 7 days old.
  // Lock serializes refreshes to avoid races; server has ON CONFLICT DO NOTHING for OTPK key_id.
  const REPLENISH_THRESHOLD = 20
  const REPLENISH_COUNT = 50
  const SIGNED_PREKEY_MAX_AGE_MS = 7 * 24 * 60 * 60 * 1000
  const keysRefreshLockRef = useRef<Promise<void>>(Promise.resolve())
  useEffect(() => {
    if (!keys || !token) return
    let cancelled = false
    const prevLock = keysRefreshLockRef.current
    const ourRefresh = (async () => {
      await prevLock
      if (cancelled) return
      const st = await api.getKeysStatus(token)
      if (cancelled) return
      const updates: Parameters<typeof api.updateKeys>[0] = {}
      let newPrekeys: import('../crypto/keys').OneTimePrekeyEntry[] = []
      if (st.unconsumed_prekeys < REPLENISH_THRESHOLD) {
        newPrekeys = generateOneTimePrekeys(REPLENISH_COUNT, st.next_one_time_key_id)
        updates.one_time_prekeys = newPrekeys.map((o) => ({ key: o.pub, key_id: o.key_id }))
      }
      let newSpk: { key: string; signature: string; secret: string; key_id: number } | undefined
      const spkUpdated = new Date(st.signed_prekey_updated_at).getTime()
      if (keys.identitySigningSecret && keys.identitySigningKey && Date.now() - spkUpdated > SIGNED_PREKEY_MAX_AGE_MS) {
        const nextId = (keys.signedPrekey.key_id ?? 1) + 1
        newSpk = generateSignedPrekey(keys.identitySigningSecret, nextId)
        updates.signed_prekey = { key: newSpk.key, signature: newSpk.signature, key_id: newSpk.key_id }
        updates.identity_signing_key = keys.identitySigningKey
      }
      if (Object.keys(updates).length > 0 && !cancelled) {
        await api.updateKeys(updates, token)
        if (cancelled) return
        if (newPrekeys.length) await addOtpks?.(newPrekeys)
        if (newSpk) await rotateSignedPrekey?.(newSpk)
      }
    })()
    keysRefreshLockRef.current = ourRefresh
    ourRefresh.catch(() => {}).finally(() => {
      if (keysRefreshLockRef.current === ourRefresh) keysRefreshLockRef.current = Promise.resolve()
    })
    return () => { cancelled = true }
  }, [keys, token, addOtpks, rotateSignedPrekey])

  useEffect(() => {
    if (!recipient) {
      sessionStorage.removeItem('chat_recipient')
      return
    }
    sessionStorage.setItem('chat_recipient', recipient)
  }, [recipient])

  // Поиск пользователей по имени (без указания домена)
  useEffect(() => {
    const q = recipient.trim()
    if (q.length < 2 || q.includes(':') || q.startsWith('!')) {
      setSearchResults([])
      return
    }
    if (searchDebounceRef.current) clearTimeout(searchDebounceRef.current)
    searchDebounceRef.current = setTimeout(() => {
      searchDebounceRef.current = null
      if (!token) return
      api.usersSearch(q, token).then(
        (r) => setSearchResults((r.users || []).filter((u) => u.user_id !== user?.user_id)),
        () => setSearchResults([]),
      )
    }, 300)
    return () => {
      if (searchDebounceRef.current) clearTimeout(searchDebounceRef.current)
    }
  }, [recipient, token, user?.user_id])

  // Непрочитанные: lastReadTs на чат, обновляем при открытии
  const lastReadKey = (addr: string) => `chat_last_read_${addr}`
  const getLastRead = useCallback((addr: string) => {
    try {
      const v = localStorage.getItem(lastReadKey(addr))
      return v ? parseInt(v, 10) : 0
    } catch { return 0 }
  }, [])
  const setLastRead = useCallback((addr: string, ts: number) => {
    try {
      localStorage.setItem(lastReadKey(addr), String(ts))
    } catch {}
  }, [])


  const getUnreadCount = useCallback((addr: string) => {
    const lastTs = getLastRead(addr)
    return events.filter((e) => {
      if (isRoomAddr(addr)) return e.recipient === addr && e.sender !== user.user_id && e.timestamp > lastTs
      const myAddr = user.user_id
      const isInChat = (e.sender === myAddr && e.recipient === addr) || (e.sender === addr && e.recipient === myAddr)
      return isInChat && e.sender !== user.user_id && e.timestamp > lastTs
    }).length
  }, [events, user.user_id, getLastRead])

  const [wsConnected, setWsConnected] = useState(false)

  useEffect(() => {
    if (!user.user_id) return
    const cleanup = api.streamWebSocket(
      user.user_id,
      token,
      (msg?: { type?: string; sender?: string; typing?: boolean; room?: string; event_id?: string; status?: string; read_at?: string }) => {
        if (msg?.type === 'typing') {
          if (msg.typing) {
            setTypingFrom(msg.sender ?? '')
            setTypingRoom(msg.room ?? '')
          } else {
            setTypingFrom('')
            setTypingRoom('')
          }
        } else if (msg?.type === 'delivery' && msg.event_id) {
          setEvents((prev) =>
            prev.map((e) => (e.event_id === msg.event_id ? { ...e, status: msg.status ?? 'delivered' } : e))
          )
        } else if (msg?.type === 'read' && msg.event_id) {
          setEvents((prev) =>
            prev.map((e) => (e.event_id === msg.event_id ? { ...e, read_at: msg.read_at ?? '' } : e))
          )
        } else {
          syncRef.current()
        }
      },
      setWsConnected,
    )
    return cleanup
  }, [user.user_id, token])

  const [typingFrom, setTypingFrom] = useState('')
  const [typingRoom, setTypingRoom] = useState('')

  useEffect(() => {
    if (!typingFrom && !typingRoom) return
    const t = setTimeout(() => {
      setTypingFrom('')
      setTypingRoom('')
    }, 3000)
    return () => clearTimeout(t)
  }, [typingFrom, typingRoom])

  const sendTypingRef = useRef<ReturnType<typeof setTimeout>>()
  const currentRecipientForTyping = recipient.trim() ? normalizeRecipient(recipient, user.user_id) : ''
  useEffect(() => {
    if (!currentRecipientForTyping) return
    if (message.trim()) {
      api.sendTyping(currentRecipientForTyping, true, token).catch(() => {})
    }
    if (sendTypingRef.current) clearTimeout(sendTypingRef.current)
    sendTypingRef.current = setTimeout(() => {
      api.sendTyping(currentRecipientForTyping, false, token).catch(() => {})
      sendTypingRef.current = undefined
    }, 2000)
    return () => {
      if (sendTypingRef.current) clearTimeout(sendTypingRef.current)
      api.sendTyping(currentRecipientForTyping, false, token).catch(() => {})
    }
  }, [message, currentRecipientForTyping, token, recipient])

  useEffect(() => {
    api.vpnProtocols(token).then((r) => setVpnProtocols(r.protocols || [])).catch(() => {})
    api.vpnNodes(token).then((r) => {
      const nodes = r.nodes || []
      setVpnNodes(nodes)
      if (nodes.length > 1) {
        setSelectedNodeId((prev) => prev || (nodes.find((n) => n.is_default) || nodes[0])?.id || '')
      }
      // Ping nodes for latency (client-side RTT)
      const latencies: Record<string, number> = {}
      Promise.all(
        nodes
          .filter((n) => n.ping_url)
          .map(async (n) => {
            const start = performance.now()
            try {
              const ctrl = new AbortController()
              const t = setTimeout(() => ctrl.abort(), 5000)
              await fetch(n.ping_url!, { method: 'HEAD', signal: ctrl.signal })
              clearTimeout(t)
              latencies[n.id] = Math.round(performance.now() - start)
            } catch {
              latencies[n.id] = 99999
            }
          })
      ).then(() => {
        setVpnNodeLatencies(latencies)
        // Auto-select fastest node when we have latencies
        const withLatency = nodes.filter((n) => latencies[n.id] != null && latencies[n.id] < 99999)
        if (withLatency.length > 0) {
          const fastest = withLatency.sort((a, b) => (latencies[a.id] ?? 99999) - (latencies[b.id] ?? 99999))[0]
          setSelectedNodeId(fastest.id)
        }
      })
    }).catch(() => {})
  }, [token])

  useEffect(() => {
    api.vpnMyConfigs(token).then((r) => setVpnMyConfigs(r.configs || [])).catch(() => {})
  }, [token])

  const fetchRooms = useCallback(() => {
    setRoomsLoading(true)
    api.roomsList(token)
      .then((r) => setRooms(r.rooms || []))
      .catch(() => {})
      .finally(() => setRoomsLoading(false))
  }, [token])

  useEffect(() => {
    fetchRooms()
  }, [fetchRooms])

  const handleCreateRoom = async () => {
    if (!createRoomName.trim()) return
    setCreateRoomLoading(true)
    try {
      const room = await api.roomsCreate(createRoomName.trim(), token)
      setRooms((prev) => [{ id: room.id, name: room.name, domain: room.domain, address: room.address }, ...prev])
      setRecipient(room.address)
      setCreateRoomName('')
    } catch (err) {
      logError('CreateRoom', err)
    } finally {
      setCreateRoomLoading(false)
    }
  }

  const currentRoom = recipient.trim()
    ? rooms.find((r) => r.address === normalizeRecipient(recipient, user.user_id))
    : null

  // Недавние DM из событий: уникальные recipient (не комната, не я), по последнему сообщению
  const recentDMs = (() => {
    const seen = new Set<string>()
    const list: { addr: string; lastTs: number }[] = []
    for (let i = events.length - 1; i >= 0; i--) {
      const ev = events[i]
      if (isRoomAddr(ev.recipient)) continue
      if (ev.recipient === user.user_id) continue
      if (ev.sender === user.user_id && ev.recipient === user.user_id) continue
      const other = ev.sender === user.user_id ? ev.recipient : ev.sender
      if (seen.has(other)) continue
      seen.add(other)
      list.push({ addr: other, lastTs: ev.timestamp })
    }
    return list.sort((a, b) => b.lastTs - a.lastTs).slice(0, 15)
  })()

  // Список личных чатов (DM только) — комнаты в отдельной секции «Комнаты»
  const chatList = recentDMs.map(({ addr, lastTs }) => ({
    addr,
    label: addr.replace(/^@([^:]+).*/, '$1'),
    lastTs,
  }))

  const fetchRoomMembers = useCallback(() => {
    if (!currentRoom) return
    api.roomsMembers(currentRoom.id, token)
      .then((r) => setRoomMembers(r.members || []))
      .catch(() => {})
  }, [currentRoom?.id, token])

  useEffect(() => {
    if (currentRoom && roomMembersVisible) {
      fetchRoomMembers()
    }
  }, [currentRoom?.id, roomMembersVisible, fetchRoomMembers])

  const handleLeaveRoom = async () => {
    if (!currentRoom) return
    if (!confirm('Выйти из комнаты?')) return
    setLeaveLoading(true)
    try {
      await api.roomsLeave(currentRoom.id, token)
      setRooms((prev) => prev.filter((r) => r.id !== currentRoom.id))
      setRecipient('')
      setRoomMembersVisible(false)
    } catch (err) {
      logError('LeaveRoom', err)
    } finally {
      setLeaveLoading(false)
    }
  }

  const handleInvite = async () => {
    if (!currentRoom || !inviteUsername.trim()) return
    setInviteLoading(true)
    try {
      await api.roomsInvite(currentRoom.id, { username: inviteUsername.trim() }, token)
      setInviteUsername('')
      await deleteSenderKey(currentRoom.id, user.user_id, user.device_id)
      fetchRoomMembers()
    } catch (err) {
      logError('RoomInvite', err)
    } finally {
      setInviteLoading(false)
    }
  }

  const myRole = (currentRoom && roomMembers.find((m) => m.address === user.user_id)?.role) ?? ''

  const handleTransferCreator = async (targetUsername: string) => {
    if (!currentRoom) return
    setRoomActionLoading('transfer')
    try {
      await api.roomsTransferCreator(currentRoom.id, { username: targetUsername }, token)
      fetchRoomMembers()
    } catch (err) {
      logError('RoomTransfer', err)
    } finally {
      setRoomActionLoading(null)
    }
  }

  const handleRemoveMember = async (targetUsername: string) => {
    if (!currentRoom) return
    if (!confirm(`Исключить ${targetUsername} из комнаты?`)) return
    setRoomActionLoading('remove')
    try {
      await api.roomsRemoveMember(currentRoom.id, { username: targetUsername }, token)
      await deleteSenderKey(currentRoom.id, user.user_id, user.device_id)
      fetchRoomMembers()
    } catch (err) {
      logError('RoomRemove', err)
    } finally {
      setRoomActionLoading(null)
    }
  }

  const currentRecipient = recipient.trim() ? normalizeRecipient(recipient, user.user_id) : ''
  const filteredEvents = currentRecipient
    ? events.filter((ev) => {
        if (isRoomAddr(currentRecipient)) {
          return ev.recipient === currentRecipient
        }
        // DM: показываем сообщения в обе стороны (от меня к нему и от него ко мне)
        const myAddr = user.user_id
        return (ev.sender === myAddr && ev.recipient === currentRecipient) || (ev.sender === currentRecipient && ev.recipient === myAddr)
      })
    : []

  useEffect(() => {
    if (!currentRecipient) return
    const latestTs = filteredEvents.length > 0
      ? Math.max(...filteredEvents.map((e) => e.timestamp))
      : Math.floor(Date.now() / 1000)
    setLastRead(currentRecipient, latestTs)
  }, [currentRecipient, filteredEvents, setLastRead])

  // Read receipts: отправляем при просмотре чата (входящие сообщения)
  const readReceiptSentRef = useRef<Set<string>>(new Set())
  useEffect(() => {
    if (!currentRecipient || !token) return
    const incomingIds = filteredEvents
      .filter((e) => e.sender !== user.user_id)
      .map((e) => e.event_id)
      .filter((id) => !readReceiptSentRef.current.has(id))
    if (incomingIds.length === 0) return
    const t = setTimeout(() => {
      api.sendReadReceipts(incomingIds, token).catch(() => {})
      incomingIds.forEach((id) => readReceiptSentRef.current.add(id))
    }, 500)
    return () => clearTimeout(t)
  }, [currentRecipient, filteredEvents, token, user.user_id])

  const refreshVpnMyConfigs = () => {
    api.vpnMyConfigs(token).then((r) => setVpnMyConfigs(r.configs || [])).catch(() => {})
  }

  const vpnRevoke = async (c: { device_id: string; protocol: string }) => {
    try {
      await api.vpnRevoke({ protocol: c.protocol, device_id: c.device_id }, token)
      refreshVpnMyConfigs()
    } catch (err) {
      logError('VpnRevoke', err)
    }
  }

  const handleCreateBackup = async () => {
    if (!keys) {
      setBackupError('Нет ключей для бэкапа')
      return
    }
    const password = prompt('Введите пароль для шифрования бэкапа (запомните его):')
    if (!password) return
    setBackupLoading(true)
    setBackupError(null)
    try {
      const { salt } = await api.backupPrepare(token)
      const trustedKeys = await getTrustedKeysForBackup()
      const payload = {
        identityKey: keys.identityKey,
        identitySecret: keys.identitySecret,
        ...(keys.identitySigningKey && { identitySigningKey: keys.identitySigningKey }),
        ...(keys.identitySigningSecret && { identitySigningSecret: keys.identitySigningSecret }),
        signedPrekey: { ...keys.signedPrekey, created_at: Date.now() },
        oneTimePrekeys: keys.oneTimePrekeys,
        device_id: user.device_id,
        trustedKeys,
        schemaVersion: 2,
      }
      const blobB64 = await createBackup(payload, password, salt)
      await api.keysSync(token, { salt, blob: blobB64 })
      const bin = atob(blobB64)
      const bytes = new Uint8Array(bin.length)
      for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
      const blob = new Blob([bytes], { type: 'application/octet-stream' })
      const a = document.createElement('a')
      a.href = URL.createObjectURL(blob)
      a.download = 'messenger-backup.dat'
      a.click()
      URL.revokeObjectURL(a.href)
    } catch (err) {
      setBackupError(err instanceof Error ? err.message : 'Ошибка создания бэкапа')
    } finally {
      setBackupLoading(false)
    }
  }

  const handleRestoreBackup = async () => {
    const file = restoreInputRef.current?.files?.[0]
    if (!file) return
    const password = prompt('Введите пароль бэкапа:')
    if (!password) return
    setBackupLoading(true)
    setBackupError(null)
    try {
      const buf = await file.arrayBuffer()
      const bytes = new Uint8Array(buf)
      const CHUNK = 0x8000
      let bin = ''
      for (let i = 0; i < bytes.length; i += CHUNK) {
        bin += String.fromCharCode.apply(null, bytes.subarray(i, i + CHUNK) as unknown as number[])
      }
      const blobB64 = btoa(bin)
      const { salt } = await api.backupGetSalt(token)
      const payload = await restoreBackup(blobB64, password, salt)
      const otpk = Array.isArray(payload.oneTimePrekeys) && payload.oneTimePrekeys.length > 0
        ? (payload.oneTimePrekeys as unknown[]).filter((o: unknown): o is { key_id: number; pub: string; priv: string } =>
            Boolean(o && typeof o === 'object' && 'key_id' in o && 'pub' in o && 'priv' in o)
          )
        : []
      await setKeysInStorage({
        identityKey: payload.identityKey,
        identitySecret: payload.identitySecret,
        identitySigningKey: payload.identitySigningKey,
        ...(payload.identitySigningSecret && { identitySigningSecret: payload.identitySigningSecret }),
        signedPrekey: payload.signedPrekey,
        oneTimePrekeys: otpk,
      })
      if (payload.trustedKeys && Object.keys(payload.trustedKeys).length > 0) {
        await restoreTrustedKeysFromBackup(payload.trustedKeys)
      }
      sessionStorage.setItem('backup_restored', '1')
      window.location.reload()
    } catch (err) {
      setBackupError(err instanceof Error ? err.message : 'Ошибка восстановления')
    } finally {
      setBackupLoading(false)
    }
  }

  const [selectedNodeId, setSelectedNodeId] = useState<string>('')
  const downloadVpnConfig = async (protocol: string, nodeId?: string) => {
    setVpnLoading(protocol)
    try {
      await api.vpnConfig(protocol, token, undefined, nodeId || selectedNodeId || undefined)
      refreshVpnMyConfigs()
    } catch (err) {
      logError('VpnConfig', err)
    } finally {
      setVpnLoading(null)
    }
  }

  const encryptDM = async (
    plaintext: string,
    bundle: Parameters<typeof signalEncrypt>[1],
    ourKeys: StoredKeys,
    addr: string,
    deviceId?: number
  ): Promise<string> => {
    if (e2eeMode === 'strict') {
      return signalEncrypt(plaintext, bundle, ourKeys, addr, deviceId)
    }
    return signalEncrypt(plaintext, bundle, ourKeys, addr, deviceId).catch(() => encrypt(plaintext, bundle))
  }

  const lastKnownDevicesRef = useRef<Record<string, string[]>>({})
  /** Encrypt DM for recipient (all devices) + self. Returns ciphertexts map and revokedCount if any device was revoked. */
  const encryptDMForRecipient = useCallback(
    async (payload: string, recipientAddr: string): Promise<{ ciphertexts: Record<string, string>; revokedCount?: number }> => {
      if (!keys) return { ciphertexts: {} }
      const selfBundle = {
        identity_key: keys.identityKey,
        signed_prekey: { key: keys.signedPrekey.key, signature: keys.signedPrekey.signature, key_id: keys.signedPrekey.key_id ?? 1 },
      }
      const ciphertexts: Record<string, string> = {}
      const selfKey = `${user.user_id}:${user.device_id}`
      let revokedCount = 0
      try {
        const { devices } = await api.getRecipientDevices(recipientAddr, token)
        const currentIds = (devices ?? []).map((d) => d.device_id)
        const prevIds = lastKnownDevicesRef.current[recipientAddr]
        if (prevIds?.length && currentIds.length < prevIds.length && currentIds.every((id) => prevIds.includes(id))) {
          revokedCount += prevIds.length - currentIds.length
        }
        lastKnownDevicesRef.current[recipientAddr] = currentIds
        if (devices?.length) {
          for (const d of devices) {
            try {
              const bundle = await api.getKeys(recipientAddr, token, d.device_id)
              const ct = await encryptDM(payload, bundle, keys, recipientAddr, uuidToSignalDeviceId(d.device_id))
              ciphertexts[`${recipientAddr}:${d.device_id}`] = ct
            } catch (e) {
              if (e instanceof ApiError && e.status === 404) {
                await deleteSessionForRecipientDevice(recipientAddr, d.device_id)
                revokedCount++
              }
            }
          }
        }
        if (Object.keys(ciphertexts).length === 0) {
          const bundle = await api.getKeys(recipientAddr, token)
          const ct = await encryptDM(payload, bundle, keys, recipientAddr)
          ciphertexts[recipientAddr] = ct
        }
      } catch {
        const bundle = await api.getKeys(recipientAddr, token)
        const ct = await encryptDM(payload, bundle, keys, recipientAddr)
        ciphertexts[recipientAddr] = ct
      }
      const ctSelf = await encryptDM(payload, selfBundle, keys, user.user_id, uuidToSignalDeviceId(user.device_id))
      ciphertexts[selfKey] = ctSelf
      return { ciphertexts, revokedCount: revokedCount > 0 ? revokedCount : undefined }
    },
    [keys, token, user.user_id, user.device_id, e2eeMode]
  )

  const encryptForRoom = useCallback(
    async (roomAddr: string, payload: string): Promise<Record<string, string> | null> => {
      if (!keys) return null
      const roomId = roomAddr.startsWith('!') ? roomAddr.slice(1).split(':')[0] : ''
      if (!roomId) return null
      const { members } = await api.roomsMembers(roomId, token).catch(() => ({ members: [] }))
      const memberList = members ?? []
      if (memberList.length === 0) return null

      const senderId = user.user_id
      const senderDeviceId = user.device_id
      const currentAddrs = memberList.map((m) => m.address)
      let sk = await getSenderKey(roomId, senderId, senderDeviceId)

      // Rekey when group composition changed (add/remove member)
      if (sk && !memberSetEquals(sk.memberAddrs, currentAddrs)) {
        await deleteSenderKey(roomId, senderId, senderDeviceId)
        sk = null
      }

      if (!sk) {
        // First message or after rekey: create sender key, distribute via Signal DM
        const key = generateSenderKey()
        const keyId = `sk-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`
        await storeSenderKey(roomId, senderId, senderDeviceId, key, keyId, currentAddrs)
        const distPayload = buildDistributionPayload(roomId, senderId, senderDeviceId, keyId, key, payload)
        const ciphertexts: Record<string, string> = {}
        for (const m of memberList) {
          try {
            const { ciphertexts: ctMap } = await encryptDMForRecipient(distPayload, m.address)
            Object.assign(ciphertexts, ctMap)
          } catch {
            // skip member without keys
          }
        }
        return Object.keys(ciphertexts).length > 0 ? ciphertexts : null
      }

      // Subsequent messages: encrypt once with sender key, same ct for all
      const sk1 = encryptWithSenderKey(payload, sk.key)
      const ciphertexts: Record<string, string> = {}
      for (const m of memberList) {
        ciphertexts[m.address] = sk1
      }
      return ciphertexts
    },
    [keys, token, user.user_id, user.device_id, encryptDMForRecipient]
  )

  const sendFile = async (file: File) => {
    if (!recipient) return
    const r = normalizeRecipient(recipient, user.user_id)
    if (identityKeyChanged && identityKeyChanged.recipient === r) {
      setSendHint('Ключ собеседника изменился — подтвердите новый ключ перед отправкой')
      setSendHintError(true)
      return
    }
    if (keys && !isRoomAddr(r) && e2eeMode === 'strict') {
      let deviceIds: string[] = []
      try {
        const { devices } = await api.getRecipientDevices(r, token)
        deviceIds = (devices ?? []).map((d) => d.device_id)
      } catch { /* fallback to legacy */ }
      const hasVerified = await getRecipientHasVerifiedDevice(r, deviceIds)
      if (!hasVerified) {
        setSendHint('Strict: подтвердите Safety Number перед отправкой')
        setSendHintError(true)
        return
      }
    }
    setSendHint(null)
    setSendHintError(false)
    if (file.size > MAX_ATTACHMENT_SIZE) {
      setSendHint(`Файл слишком большой (макс. ${MAX_ATTACHMENT_SIZE / 1024 / 1024} МБ)`)
      setSendHintError(true)
      return
    }
    try {
      const { ciphertext, fileKey, nonce, sha256, chunkSize } = await encryptAttachmentFromFile(file)
      const cipherBlob = new Blob([ciphertext as BlobPart])
      const { content_uri } = await api.uploadFile(cipherBlob, token)
      const payload = JSON.stringify({
        message_type: 'file',
        e2ee_version: E2EE_ATTACHMENT_VERSION,
        content_uri,
        filename: sanitizeFilename(file.name),
        size: file.size,
        file_key: fileKey,
        nonce,
        file_sha256: sha256,
        ...(chunkSize != null && { chunk_size: chunkSize }),
      })
      const isRoom = isRoomAddr(r)
      let content: { ciphertext?: string; ciphertexts?: Record<string, string>; session_id: string }
      if (keys && isRoom) {
        const ciphertexts = await encryptForRoom(r, payload)
        content = ciphertexts ? { ciphertexts, session_id: 'sess_mvp' } : { ciphertext: btoa(unescape(encodeURIComponent(payload))), session_id: 'sess_mvp' }
      } else if (keys && !isRoom) {
        try {
          const { ciphertexts: ctMap, revokedCount } = await encryptDMForRecipient(payload, r)
          content = { ciphertexts: ctMap, session_id: 'sess_mvp' }
          if (revokedCount && revokedCount > 0 && e2eeMode === 'strict') {
            setSendHint('У получателя отозвано устройство. Сообщение доставлено на оставшиеся.')
            setSendHintError(false)
          }
        } catch (e) {
          if (e instanceof ApiError && e.status === 404) {
            setSendHint('Пользователь не найден')
            setSendHintError(true)
            return
          }
          if (e2eeMode === 'strict' && e instanceof Error) {
            setSendHint(`Signal недоступен: ${e.message}. Режим Strict — отключите для fallback.`)
            setSendHintError(true)
            return
          }
          setSendHint('Ключи получателя недоступны — сообщение без E2EE')
          setSendHintError(false)
          content = { ciphertext: btoa(unescape(encodeURIComponent(payload))), session_id: 'sess_mvp' }
        }
      } else {
        content = { ciphertext: btoa(unescape(encodeURIComponent(payload))), session_id: 'sess_mvp' }
      }
      const ts = Math.floor(Date.now() / 1000)
      const res = await api.sendMessage(
        {
          type: 'm.room.encrypted',
          sender: user.user_id,
          recipient: r,
          device_id: user.device_id,
          timestamp: ts,
          content,
        },
        token,
        crypto.randomUUID()
      )
      // Оптимистичное отображение: префикс opt: чтобы не путать с E2EE
      setEvents((prev) => {
        const rest = prev.filter((e) => e.event_id !== res.event_id)
        return [...rest, { event_id: res.event_id, type: 'm.room.encrypted', sender: user.user_id, recipient: r, timestamp: ts, ciphertext: 'opt:' + btoa(unescape(encodeURIComponent(payload))), session_id: 'sess_mvp' }]
      })
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) {
        setSendHint('Пользователь не найден')
        setSendHintError(true)
      } else {
        logError('Send/file', e)
        setSendHint('Не удалось отправить файл. Проверьте соединение и попробуйте снова.')
        setSendHintError(true)
      }
    }
  }

  const sendFiles = async (files: FileList | null) => {
    if (!files?.length || !recipient) return
    setUploading(true)
    try {
      for (let i = 0; i < files.length; i++) {
        await sendFile(files[i]!)
      }
    } finally {
      setUploading(false)
    }
  }

  const sendMessage = async () => {
    if (!recipient || !message.trim()) return
    const r = normalizeRecipient(recipient, user.user_id)
    if (identityKeyChanged && identityKeyChanged.recipient === r) {
      setSendHint('Ключ собеседника изменился — подтвердите новый ключ перед отправкой')
      setSendHintError(true)
      return
    }
    if (keys && !isRoomAddr(r) && e2eeMode === 'strict') {
      let deviceIds: string[] = []
      try {
        const { devices } = await api.getRecipientDevices(r, token)
        deviceIds = (devices ?? []).map((d) => d.device_id)
      } catch { /* fallback to legacy */ }
      const hasVerified = await getRecipientHasVerifiedDevice(r, deviceIds)
      if (!hasVerified) {
        setSendHint('Strict: подтвердите Safety Number перед отправкой')
        setSendHintError(true)
        return
      }
    }
    setSendHint(null)
    setSendHintError(false)
    setSending(true)
    try {
      const plaintext = message.trim()
      const isRoom = isRoomAddr(r)
      let content: { ciphertext?: string; ciphertexts?: Record<string, string>; session_id: string }
      if (keys && isRoom) {
        const ciphertexts = await encryptForRoom(r, plaintext)
        content = ciphertexts ? { ciphertexts, session_id: 'sess_mvp' } : { ciphertext: btoa(unescape(encodeURIComponent(plaintext))), session_id: 'sess_mvp' }
      } else if (keys && !isRoom) {
        try {
          const { ciphertexts: ctMap, revokedCount } = await encryptDMForRecipient(plaintext, r)
          content = { ciphertexts: ctMap, session_id: 'sess_mvp' }
          if (revokedCount && revokedCount > 0 && e2eeMode === 'strict') {
            setSendHint('У получателя отозвано устройство. Сообщение доставлено на оставшиеся.')
            setSendHintError(false)
          }
        } catch (e) {
          if (e instanceof ApiError && e.status === 404) {
            setSendHint('Пользователь не найден')
            setSendHintError(true)
            return
          }
          if (e2eeMode === 'strict' && e instanceof Error) {
            setSendHint(`Signal недоступен: ${e.message}. Режим Strict — отключите для fallback.`)
            setSendHintError(true)
            return
          }
          setSendHint('Ключи получателя недоступны — сообщение без E2EE')
          setSendHintError(false)
          content = { ciphertext: btoa(unescape(encodeURIComponent(plaintext))), session_id: 'sess_mvp' }
        }
      } else {
        content = { ciphertext: btoa(unescape(encodeURIComponent(plaintext))), session_id: 'sess_mvp' }
      }
      const ts = Math.floor(Date.now() / 1000)
      const res = await api.sendMessage(
        {
          type: 'm.room.encrypted',
          sender: user.user_id,
          recipient: r,
          device_id: user.device_id,
          timestamp: ts,
          content,
        },
        token,
        crypto.randomUUID()
      )
      setMessage('')
      // Оптимистичное отображение: префикс opt: чтобы не путать с E2EE
      const optimisticCipher = 'opt:' + btoa(unescape(encodeURIComponent(plaintext)))
      setEvents((prev) => {
        const rest = prev.filter((e) => e.event_id !== res.event_id)
        return [...rest, { event_id: res.event_id, type: 'm.room.encrypted', sender: user.user_id, recipient: r, timestamp: ts, ciphertext: optimisticCipher, session_id: 'sess_mvp' }]
      })
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) {
        setSendHint('Пользователь не найден')
        setSendHintError(true)
      } else {
        logError('Send/file', e)
        setSendHint('Не удалось отправить. Проверьте соединение и попробуйте снова.')
        setSendHintError(true)
      }
    } finally {
      setSending(false)
    }
  }

  type ParsedMessage =
    | { type: 'text'; text: string }
    | { type: 'file'; filename: string; content_uri: string; size?: number; file_key?: string; nonce?: string; file_sha256?: string; chunk_size?: number; e2ee_version?: number }

  const parseMessage = (ciphertext: string, eventId?: string): ParsedMessage => {
    if (!ciphertext) return { type: 'text', text: '[encrypted]' }
    let plaintext: string
    // Signal (sig1:) и Sender Key (sk1:) — асинхронная расшифровка, результат в decryptedSignal
    if ((ciphertext.startsWith('sig1:') || ciphertext.startsWith('sk1:')) && eventId && decryptedSignal[eventId]) {
      plaintext = decryptedSignal[eventId]
    } else if (ciphertext.startsWith('sig1:') || ciphertext.startsWith('sk1:')) {
      return { type: 'text', text: '[decrypting...]' }
    } else if (ciphertext.startsWith('opt:')) {
      const b64 = ciphertext.slice(4)
      if (!isValidBase64(b64)) return { type: 'text', text: '[encrypted]' }
      try {
        plaintext = decodeURIComponent(escape(atob(b64)))
      } catch {
        return { type: 'text', text: '[encrypted]' }
      }
    } else if (keys?.identitySecret && keys?.signedPrekey?.secret && isE2EEPayload(ciphertext)) {
      try {
        plaintext = decrypt(
          ciphertext,
          decodeBase64ToBytes(keys.identitySecret),
          decodeBase64ToBytes(keys.signedPrekey.secret)
        )
        const dist = parseDistributionPayload(plaintext)
        if (dist) {
          const bin = atob(dist.key)
          const arr = new Uint8Array(bin.length)
          for (let i = 0; i < bin.length; i++) arr[i] = bin.charCodeAt(i)
          storeSenderKey(dist.roomId, dist.senderId, dist.senderDeviceId, arr, dist.keyId).catch(() => {})
          plaintext = dist.body ?? '[key received]'
        }
      } catch {
        return { type: 'text', text: '[decrypt failed]' }
      }
    } else {
      try {
        plaintext = decodeURIComponent(escape(atob(ciphertext)))
      } catch {
        return { type: 'text', text: '[encrypted]' }
      }
    }
    try {
      const parsed = JSON.parse(plaintext)
      if (parsed?.message_type === 'file' && parsed?.content_uri) {
        return {
          type: 'file',
          filename: sanitizeFilename(parsed.filename || 'file'),
          content_uri: parsed.content_uri,
          size: parsed.size,
          file_key: parsed.file_key,
          nonce: parsed.nonce,
          file_sha256: parsed.file_sha256,
          chunk_size: parsed.chunk_size,
          e2ee_version: parsed.e2ee_version,
        }
      }
    } catch {
      /* plain text */
    }
    return { type: 'text', text: plaintext }
  }

  const searchFilteredEvents =
    searchQuery.trim() === ''
      ? filteredEvents
      : (() => {
          const q = searchQuery.trim().toLowerCase()
          return filteredEvents.filter((ev) => {
            const parsed = parseMessage(ev.ciphertext, ev.event_id)
            if (parsed.type === 'text') return parsed.text.toLowerCase().includes(q)
            return parsed.filename.toLowerCase().includes(q)
          })
        })()

  const enablePush = useCallback(async () => {
    if (!('serviceWorker' in navigator) || !('PushManager' in window)) return
    setPushLoading(true)
    try {
      const reg = await navigator.serviceWorker.register('/sw.js')
      await reg.update()
      let permission = Notification.permission
      if (permission === 'default') {
        permission = await Notification.requestPermission()
      }
      if (permission !== 'granted') {
        setPushLoading(false)
        return
      }
      const { public_key } = await api.getVapidPublic(token)
      const keyBytes = Uint8Array.from(atob(public_key.replace(/-/g, '+').replace(/_/g, '/')), (c) => c.charCodeAt(0))
      const sub = await reg.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: keyBytes,
      })
      await api.pushSubscribe(sub.toJSON(), token)
      setPushEnabled(true)
    } catch (e) {
      logError('Push subscribe', e)
    } finally {
      setPushLoading(false)
    }
  }, [token])

  const downloadFileMessage = async (contentUri: string, filename: string, fileKey?: string, nonce?: string, fileSha256?: string, chunkSize?: number, e2eeVersion?: number) => {
    try {
      validateAttachmentVersion(e2eeVersion)
      const res = await api.downloadFile(contentUri, token)
      let blob: Blob = await res.blob()
      if (fileKey && (nonce || chunkSize)) {
        const ciphertext = new Uint8Array(await blob.arrayBuffer())
        const plain = await decryptAttachment(ciphertext, fileKey, nonce ?? '', fileSha256, chunkSize)
        blob = new Blob([plain as BlobPart])
      }
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = sanitizeFilename(filename)
      a.click()
      URL.revokeObjectURL(url)
    } catch (err) {
      logError('FileDownload', err)
    }
  }

  return (
    <div className="chat-layout">
      <header>
        <div className="chat-header-left">
          <span className="chat-header-user">{user.user_id}</span>
          <span
            className={`chat-ws-dot ${wsConnected ? 'chat-ws-dot--connected' : ''}`}
            title={wsConnected ? 'Подключено' : 'Нет соединения — переподключение…'}
          />
          {!wsConnected && (
            <span className="chat-ws-reconnecting">Переподключение…</span>
          )}
          <span className={`chat-e2ee-status ${keys ? 'chat-e2ee-status--loaded' : ''}`} title={keys ? 'E2EE ключи загружены' : 'E2EE недоступен — войдите заново или восстановите ключи'}>
            {keys ? '🔒' : '⚠️'}
          </span>
        </div>
        <div className="chat-header-actions">
          <div className="chat-tab-btns">
            <button
              type="button"
              onClick={() => setMainView('chats')}
              className={`chat-tab-btn ${mainView === 'chats' ? 'chat-tab-btn--active' : ''}`}
            >
              Чаты
            </button>
            <button
              type="button"
              onClick={() => setMainView('vpn')}
              className={`chat-tab-btn ${mainView === 'vpn' ? 'chat-tab-btn--active' : ''}`}
            >
              VPN
            </button>
            <button
              type="button"
              onClick={() => setMainView('devices')}
              className={`chat-tab-btn ${mainView === 'devices' ? 'chat-tab-btn--active' : ''}`}
            >
              Устройства
            </button>
          </div>
          {'PushManager' in window && (
            <button
              type="button"
              onClick={enablePush}
              disabled={pushLoading || pushEnabled}
              className="chat-theme-btn"
              title={pushEnabled ? 'Push включены' : 'Включить push-уведомления'}
            >
              {pushLoading ? '…' : pushEnabled ? '📲' : '📱'}
            </button>
          )}
          <button
            onClick={() => setSoundEnabled((v) => !v)}
            className="chat-theme-btn"
            title={soundEnabled ? 'Звук вкл — выключить' : 'Звук выкл — включить'}
          >
            {soundEnabled ? '🔔' : '🔕'}
          </button>
          <button onClick={toggleTheme} className="chat-theme-btn" title={theme === 'dark' ? 'Светлая тема' : 'Тёмная тема'}>
            {theme === 'dark' ? '☀️' : '🌙'}
          </button>
          <button onClick={onLogout} className="chat-logout">
            Выход
          </button>
        </div>
      </header>

      {backupRestoredHint && (
        <div className="chat-backup-restored-hint">
          <span>Вы восстановили ключи. Рекомендуется повторно проверить Safety Number у важных контактов.</span>
          <button
            type="button"
            onClick={() => {
              sessionStorage.removeItem('backup_restored')
              setBackupRestoredHint(false)
            }}
            className="chat-vpn-btn chat-btn-sm"
          >
            Понятно
          </button>
        </div>
      )}

      <main className="chat-main">
        {mainView === 'devices' ? (
          <>
            <div className="chat-sidebar-narrow">
              <div className="chat-sidebar-section">
                <button type="button" onClick={() => setMainView('chats')} className="chat-vpn-btn chat-vpn-btn-full">
                  ← Чаты
                </button>
              </div>
            </div>
            <div className="chat-content-area">
              <h2>Мои устройства</h2>
              <p className="chat-vpn-proto-hint chat-mb-16">Устройства, с которых вы входили в аккаунт. Удаление отзовёт сессию — устройство потребует повторного входа.</p>
              {devicesLoading ? (
                <p className="chat-vpn-proto-hint">Загрузка…</p>
              ) : devices.length === 0 ? (
                <p className="chat-vpn-proto-hint">Нет устройств</p>
              ) : (
                <div className="chat-devices-section">
                  {devices.map((d) => (
                    <div
                      key={d.device_id}
                      className={`chat-device-card ${d.is_current ? 'chat-device-card--current' : ''}`}
                    >
                      <div>
                        <span className="chat-device-id">{d.name?.trim() || `${d.device_id.slice(0, 8)}…${d.device_id.slice(-4)}`}</span>
                        {d.name?.trim() && <span className="chat-device-id-suffix"> ({d.device_id.slice(0, 8)}…)</span>}
                        {d.is_current && <span className="chat-device-current-badge">текущее</span>}
                        <p className="chat-device-created">Создано: {d.created_at}</p>
                      </div>
                      <div className="chat-device-actions">
                        <button
                          type="button"
                          className="chat-vpn-btn"
                          onClick={async () => {
                            const newName = window.prompt('Название устройства', d.name?.trim() || '')
                            if (newName == null) return
                            setDevicesRenameLoading(d.device_id)
                            try {
                              await api.renameDevice(d.device_id, newName.trim(), token)
                              setDevices((prev) => prev.map((x) => x.device_id === d.device_id ? { ...x, name: newName.trim() } : x))
                            } catch (err) {
                              logError('DeviceRename', err)
                            } finally {
                              setDevicesRenameLoading(null)
                            }
                          }}
                          disabled={devicesRenameLoading === d.device_id}
                        >
                          {devicesRenameLoading === d.device_id ? '…' : 'Переименовать'}
                        </button>
                        <button
                          type="button"
                          className="chat-vpn-btn-danger"
                        onClick={async () => {
                          if (d.is_current && !window.confirm('Удалить текущее устройство? Вы выйдете из аккаунта.')) return
                          setDevicesRevokeLoading(d.device_id)
                          try {
                            await api.revokeDevice(d.device_id, token)
                            setDevices((prev) => prev.filter((x) => x.device_id !== d.device_id))
                            if (d.is_current) onLogout()
                          } catch (err) {
                            logError('DeviceRevoke', err)
                          } finally {
                            setDevicesRevokeLoading(null)
                          }
                        }}
                        disabled={devicesRevokeLoading === d.device_id}
                      >
                        {devicesRevokeLoading === d.device_id ? '…' : 'Удалить'}
                      </button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </>
        ) : mainView === 'vpn' ? (
          <>
            <div className="chat-sidebar-narrow">
              <div className="chat-sidebar-section">
                <button
                  type="button"
                  onClick={() => setMainView('chats')}
                  className="chat-vpn-btn chat-vpn-btn-full"
                >
                  ← Чаты
                </button>
              </div>
            </div>
            <div className="chat-content-area">
              <h2 className="vpn-h2">VPN</h2>
              <p className="chat-vpn-hint chat-mb-20">Скачайте конфиг и подключитесь. Только ваши конфиги.</p>
              {vpnNodes.length > 1 && (
                <div className="chat-mb-16">
                  <label className="chat-select-label">Нода</label>
                  <select
                    value={selectedNodeId}
                    onChange={(e) => setSelectedNodeId(e.target.value)}
                    className="chat-recipient-input chat-select-node"
                  >
                    {[...vpnNodes]
                      .sort((a, b) => {
                        const la = vpnNodeLatencies[a.id] ?? 99999
                        const lb = vpnNodeLatencies[b.id] ?? 99999
                        return la - lb
                      })
                      .map((n) => {
                        const ms = vpnNodeLatencies[n.id]
                        const validLatencies = Object.values(vpnNodeLatencies).filter((v) => v < 99999)
                        const minMs = validLatencies.length ? Math.min(...validLatencies) : 99999
                        const isRecommended = ms != null && ms < 99999 && ms === minMs
                        return (
                          <option key={n.id} value={n.id}>
                            {n.name}
                            {n.region ? ` (${n.region})` : ''}
                            {ms != null && ms < 99999 ? ` · ${ms} мс` : ''}
                            {isRecommended ? ' ★ Рекомендуется' : n.is_default ? ' ★' : ''}
                          </option>
                        )
                      })}
                  </select>
                </div>
              )}
              <div className="chat-mb-24">
                <h3 className="chat-h3">Протоколы</h3>
                {vpnProtocols.length === 0 ? (
                  <p className="chat-vpn-proto-hint">Загрузка…</p>
                ) : (
                  <div className="chat-protocols-list">
                    {vpnProtocols.map((p) => (
                      <div key={p.id} className="chat-vpn-protocol">
                        <div>
                          <strong>{p.name}</strong>
                          <p className="chat-vpn-proto-hint">{p.hint}</p>
                        </div>
                        <button className="chat-vpn-btn" onClick={() => downloadVpnConfig(p.id)} disabled={vpnLoading === p.id}>
                          {vpnLoading === p.id ? '…' : 'Скачать'}
                        </button>
                      </div>
                    ))}
                  </div>
                )}
              </div>
              <div className="chat-vpn-admin-section">
                <h3 className="chat-h3">Мои конфиги</h3>
                <p className="chat-vpn-hint">Только ваши выданные конфиги</p>
                {vpnMyConfigs.length === 0 ? (
                  <p className="chat-vpn-proto-hint">Нет конфигов. Скачайте протокол выше.</p>
                ) : (
                  vpnMyConfigs.map((c, i) => (
                    <div key={`${c.device_id}-${c.protocol}-${i}`} className="chat-vpn-admin-row">
                      <div>
                        <strong>{c.protocol}</strong>{c.node_name ? ` · ${c.node_name}` : ''}
                        <p className="chat-vpn-proto-hint">
                          {new Date(c.created_at).toLocaleDateString()}
                          {c.expires_at && ` · истекает ${new Date(c.expires_at).toLocaleDateString()}`}
                          {c.is_expired && ' (истёк)'}
                          {(() => {
                            const used = (c.traffic_rx_bytes ?? 0) + (c.traffic_tx_bytes ?? 0)
                            const limit = c.traffic_limit_bytes ?? 0
                            if (used > 0 || limit > 0) return limit > 0 ? ` · трафик ${formatBytes(used)} / ${formatBytes(limit)}` : ` · трафик ${formatBytes(used)}`
                            return null
                          })()}
                        </p>
                      </div>
                      <div className="chat-vpn-admin-actions">
                        <button className="chat-vpn-btn" onClick={() => downloadVpnConfig(c.protocol)} disabled={vpnLoading === c.protocol} title="Скачать снова">↻</button>
                        <button className="chat-vpn-btn-danger" onClick={() => vpnRevoke(c)} title="Отозвать">✕</button>
                      </div>
                    </div>
                  ))
                )}
              </div>
              <div className="chat-vpn-admin-section">
                <h3 className="chat-h3">Резервная копия ключей</h3>
                <p className="chat-vpn-hint">Без сервера расшифровать нельзя</p>
                {backupError && <p className="chat-vpn-proto-hint chat-error-text">{backupError}</p>}
                <div className="chat-backup-actions">
                  <button className="chat-vpn-btn" onClick={handleCreateBackup} disabled={backupLoading || !keys} title="Создать зашифрованный бэкап">
                    {backupLoading ? '…' : 'Создать бэкап'}
                  </button>
                  <div>
                    <input ref={restoreInputRef} type="file" accept=".dat" className="chat-input-hidden" onChange={(e) => { if (e.target.files?.[0]) handleRestoreBackup(); e.target.value = '' }} />
                    <button className="chat-vpn-btn" onClick={() => restoreInputRef.current?.click()} disabled={backupLoading}>Восстановить из бэкапа</button>
                  </div>
                  {lockKeysWithPassphrase && keys && (
                    <div className="chat-mt-10">
                      <p className="chat-vpn-hint chat-mb-8">Защитить ключи паролем</p>
                      <input type="password" placeholder="Пароль" value={lockPassphrase} onChange={(e) => { setLockPassphrase(e.target.value); setLockError('') }} className="chat-recipient-input chat-recipient-input-sm" />
                      <input type="password" placeholder="Подтверждение" value={lockPassphraseConfirm} onChange={(e) => { setLockPassphraseConfirm(e.target.value); setLockError('') }} className="chat-recipient-input chat-recipient-input-sm chat-mt-8" />
                      {lockError && <p className="chat-vpn-proto-hint chat-error-text">{lockError}</p>}
                      <button className="chat-vpn-btn chat-mt-8" disabled={lockLoading || !lockPassphrase || lockPassphrase !== lockPassphraseConfirm} onClick={async () => { if (!lockPassphrase || lockPassphrase !== lockPassphraseConfirm) return; setLockError(''); setLockLoading(true); try { await lockKeysWithPassphrase(lockPassphrase) } catch (err) { setLockError(err instanceof Error ? err.message : 'Ошибка') } finally { setLockLoading(false) } }}>{lockLoading ? '…' : 'Защитить паролем'}</button>
                    </div>
                  )}
                </div>
              </div>
            </div>
          </>
        ) : (
        <>
        <div className="chat-sidebar">
          <div className="chat-sidebar-section">
            <h3 className="chat-sidebar-title">Чаты</h3>
            <div className="chat-chat-list">
              {chatList.length === 0 ? (
                <p className="chat-vpn-proto-hint">Нет чатов. Начните новый ниже.</p>
              ) : (
                chatList.map(({ addr, label }) => {
                  const unread = getUnreadCount(addr)
                  return (
                    <button
                      key={addr}
                      type="button"
                      onClick={() => setRecipient(addr)}
                      className={`chat-room-item ${recipient === addr ? 'chat-room-item--active' : ''}`}
                    >
                      <span className="chat-room-item-label">{label}</span>
                      {unread > 0 && (
                        <span className="chat-unread-badge">{unread > 99 ? '99+' : unread}</span>
                      )}
                    </button>
                  )
                })
              )}
            </div>
            <div className="chat-mt-10 chat-search-wrap">
              <input
                type="text"
                placeholder="Найти пользователя (имя с любого сервера)"
                value={recipient}
                onChange={(e) => { setRecipient(e.target.value); setSendHint(null) }}
                className="chat-recipient-input chat-recipient-input-sm"
              />
              {searchResults.length > 0 && (
                <ul className="chat-search-dropdown">
                  {searchResults.map((u) => (
                    <li key={u.user_id}>
                      <button
                        type="button"
                        className="chat-room-item chat-room-item--full"
                        onClick={() => { setRecipient(u.user_id); setSearchResults([]) }}
                      >
                        {u.user_id}
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </div>
          <div className="chat-sidebar-section">
            <h4 className="chat-vpn-title">Комнаты</h4>
            <div className="chat-room-actions">
              <input
                type="text"
                placeholder="Название комнаты"
                value={createRoomName}
                onChange={(e) => setCreateRoomName(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && handleCreateRoom()}
                className="chat-recipient-input chat-recipient-input-flex"
              />
              <button
                className="chat-vpn-btn"
                onClick={handleCreateRoom}
                disabled={createRoomLoading || !createRoomName.trim()}
                title="Создать комнату"
              >
                {createRoomLoading ? '…' : '+'}
              </button>
            </div>
            {roomsLoading ? (
              <p className="chat-vpn-proto-hint">Загрузка…</p>
            ) : rooms.length === 0 ? (
              <p className="chat-vpn-proto-hint">Нет комнат. Создайте комнату выше.</p>
            ) : (
              <div className="chat-chat-list">
                {rooms.map((room) => {
                  const unread = getUnreadCount(room.address)
                  return (
                  <button
                    key={room.id}
                    type="button"
                    onClick={() => setRecipient(room.address)}
                    className={`chat-room-item ${recipient === room.address ? 'chat-room-item--active' : ''}`}
                  >
                    <span className="chat-room-item-label">{room.name}</span>
                    {unread > 0 && (
                      <span className="chat-unread-badge">{unread > 99 ? '99+' : unread}</span>
                    )}
                  </button>
                )
                })}
              </div>
            )}
          </div>
          {vpnProtocols.length > 0 && (
            <div className="chat-vpn-section">
              <div className="chat-sidebar-section">
              <h4 className="chat-vpn-title">VPN</h4>
              <p className="chat-vpn-hint">Не работает? Попробуй другой протокол</p>
              {vpnNodes.length > 1 && (
                <div className="chat-mb-8">
                  <label className="chat-select-label chat-select-label-sm">Нода: </label>
                  <select
                    value={selectedNodeId}
                    onChange={(e) => setSelectedNodeId(e.target.value)}
                    className="chat-recipient-input chat-recipient-input-xs"
                  >
                    {[...vpnNodes]
                      .sort((a, b) => {
                        const la = vpnNodeLatencies[a.id] ?? 99999
                        const lb = vpnNodeLatencies[b.id] ?? 99999
                        return la - lb
                      })
                      .map((n) => {
                        const ms = vpnNodeLatencies[n.id]
                        const validLatencies = Object.values(vpnNodeLatencies).filter((v) => v < 99999)
                        const minMs = validLatencies.length ? Math.min(...validLatencies) : 99999
                        const isRecommended = ms != null && ms < 99999 && ms === minMs
                        return (
                          <option key={n.id} value={n.id}>
                            {n.name}
                            {n.region ? ` (${n.region})` : ''}
                            {ms != null && ms < 99999 ? ` · ${ms} мс` : ''}
                            {isRecommended ? ' ★ Рекомендуется' : n.is_default ? ' ★' : ''}
                          </option>
                        )
                      })}
                  </select>
                </div>
              )}
              {vpnProtocols.map((p) => (
                <div key={p.id} className="chat-vpn-protocol">
                  <div>
                    <strong>{p.name}</strong>
                    <p className="chat-vpn-proto-hint">{p.hint}</p>
                  </div>
                  <button
                    className="chat-vpn-btn"
                    onClick={() => downloadVpnConfig(p.id)}
                    disabled={vpnLoading === p.id}
                  >
                    {vpnLoading === p.id ? '…' : 'Скачать'}
                  </button>
                </div>
              ))}
              </div>
            </div>
          )}
          {vpnProtocols.length > 0 && (
            <div className="chat-vpn-admin-section">
              <h4 className="chat-vpn-title">Мои конфиги</h4>
              <p className="chat-vpn-hint">Только ваши выданные конфиги</p>
              {vpnMyConfigs.length === 0 ? (
                <p className="chat-vpn-proto-hint">Нет конфигов. Скачайте протокол выше.</p>
              ) : (
                vpnMyConfigs.map((c, i) => (
                  <div key={`${c.device_id}-${c.protocol}-${i}`} className="chat-vpn-admin-row">
                    <div>
                      <strong>{c.protocol}</strong>{c.node_name ? ` · ${c.node_name}` : ''}
                      <p className="chat-vpn-proto-hint">
                        {new Date(c.created_at).toLocaleDateString()}
                        {c.expires_at && ` · истекает ${new Date(c.expires_at).toLocaleDateString()}`}
                        {c.is_expired && ' (истёк)'}
                        {(() => {
                          const used = (c.traffic_rx_bytes ?? 0) + (c.traffic_tx_bytes ?? 0)
                          if (used > 0 || (c.traffic_limit_bytes ?? 0) > 0) {
                            const limit = c.traffic_limit_bytes ?? 0
                            return limit > 0 ? ` · трафик ${formatBytes(used)} / ${formatBytes(limit)}` : ` · трафик ${formatBytes(used)}`
                          }
                          return null
                        })()}
                      </p>
                    </div>
                    <div className="chat-vpn-admin-actions">
                      <button className="chat-vpn-btn" onClick={() => downloadVpnConfig(c.protocol)} disabled={vpnLoading === c.protocol} title="Скачать снова">
                        ↻
                      </button>
                      <button className="chat-vpn-btn-danger" onClick={() => vpnRevoke(c)} title="Отозвать">
                        ✕
                      </button>
                    </div>
                  </div>
                ))
              )}
            </div>
          )}
          <div className="chat-vpn-admin-section">
            <h4 className="chat-vpn-title">Резервная копия ключей</h4>
            <p className="chat-vpn-hint">Без сервера расшифровать нельзя</p>
            {backupError && <p className="chat-vpn-proto-hint chat-error-text">{backupError}</p>}
            <div className="chat-backup-actions">
              <button
                className="chat-vpn-btn"
                onClick={handleCreateBackup}
                disabled={backupLoading || !keys}
                title="Создать зашифрованный бэкап"
              >
                {backupLoading ? '…' : 'Создать бэкап'}
              </button>
              <div>
                <input
                  ref={restoreInputRef}
                  type="file"
                  accept=".dat"
                  className="chat-input-hidden"
                  onChange={(e) => {
                    if (e.target.files?.[0]) handleRestoreBackup()
                    e.target.value = ''
                  }}
                />
                <button
                  className="chat-vpn-btn"
                  onClick={() => restoreInputRef.current?.click()}
                  disabled={backupLoading}
                >
                  Восстановить из бэкапа
                </button>
              </div>
              {lockKeysWithPassphrase && keys && (
                <div className="chat-mt-10">
                  <p className="chat-vpn-hint chat-mb-8">Защитить ключи паролем (опционально)</p>
                  <input
                    type="password"
                    placeholder="Пароль"
                    value={lockPassphrase}
                    onChange={(e) => { setLockPassphrase(e.target.value); setLockError('') }}
                    className="chat-recipient-input chat-recipient-input-sm"
                  />
                  <input
                    type="password"
                    placeholder="Подтверждение"
                    value={lockPassphraseConfirm}
                    onChange={(e) => { setLockPassphraseConfirm(e.target.value); setLockError('') }}
                    className="chat-recipient-input chat-recipient-input-sm chat-mt-8"
                  />
                  {lockError && <p className="chat-vpn-proto-hint chat-error-text">{lockError}</p>}
                  <button
                    className="chat-vpn-btn chat-mt-8"
                    disabled={lockLoading || !lockPassphrase || lockPassphrase !== lockPassphraseConfirm}
                    onClick={async () => {
                      if (!lockPassphrase || lockPassphrase !== lockPassphraseConfirm) return
                      setLockError('')
                      setLockLoading(true)
                      try {
                        await lockKeysWithPassphrase(lockPassphrase)
                      } catch (err) {
                        setLockError(err instanceof Error ? err.message : 'Ошибка')
                      } finally {
                        setLockLoading(false)
                      }
                    }}
                  >
                    {lockLoading ? '…' : 'Защитить паролем'}
                  </button>
                </div>
              )}
            </div>
          </div>
        </div>

        <div className="chat-chat">
          <div className="chat-recipient-bar">
            {recipient.trim() ? (
              <div className="chat-recipient-flex">
                <p className="chat-with chat-with-no-margin">
                  {isRoomAddr(normalizeRecipient(recipient, user.user_id))
                    ? `Комната: ${rooms.find((r) => r.address === normalizeRecipient(recipient, user.user_id))?.name ?? recipient}`
                    : `Чат с ${normalizeRecipient(recipient, user.user_id)}`}
                </p>
                {!isRoomAddr(normalizeRecipient(recipient, user.user_id)) && keys && (
                  <>
                    <button
                      type="button"
                      onClick={() => setE2eeMode(e2eeMode === 'strict' ? 'compatibility' : 'strict')}
                      className={`chat-theme-btn chat-e2ee-mode-btn ${e2eeMode === 'strict' ? 'chat-e2ee-mode-btn--strict' : ''}`}
                      title={e2eeMode === 'strict' ? 'Strict: только Signal, без fallback на MVP. Нажмите для Compatibility.' : 'Compatibility: при ошибке Signal — fallback на MVP. Нажмите для Strict.'}
                    >
                      E2EE: {e2eeMode === 'strict' ? 'Strict' : 'Compat'}
                    </button>
                    <button
                      type="button"
                      onClick={showFingerprint}
                      disabled={fingerprintLoading}
                      className="chat-theme-btn chat-e2ee-mode-btn"
                      title="Проверить ключ (Safety number)"
                    >
                      {fingerprintLoading ? '…' : '🔐 Отпечаток'}
                    </button>
                  </>
                )}
                {typingFrom && ((typingRoom && currentRecipient === typingRoom) || (!typingRoom && currentRecipient === typingFrom)) && (
                  <span className="chat-typing">{typingFrom.replace(/^@([^:]+).*/, '$1')} печатает…</span>
                )}
                {currentRecipient && (
                  <input
                    type="text"
                    placeholder="Поиск в чате…"
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    className="chat-recipient-input chat-recipient-input-search"
                  />
                )}
                {currentRoom && (
                  <>
                    <button
                      type="button"
                      onClick={() => setRoomMembersVisible((v) => !v)}
                      className="chat-vpn-btn chat-btn-sm"
                    >
                      {roomMembersVisible ? 'Скрыть участников' : 'Участники'}
                    </button>
                    <button
                      type="button"
                      onClick={handleLeaveRoom}
                      disabled={leaveLoading}
                      className="chat-vpn-btn-danger chat-btn-sm"
                    >
                      {leaveLoading ? '…' : 'Выйти'}
                    </button>
                  </>
                )}
              </div>
            ) : (
              <p className="chat-select-hint">Выберите чат в списке слева</p>
            )}
            {currentRoom && roomMembersVisible && (
              <div className="chat-room-members-panel">
                <h4 className="chat-panel chat-panel-h4">Участники</h4>
                <ul className="chat-member-list">
                  {roomMembers.map((m) => {
                    const isMe = m.address === user.user_id
                    const canTransfer = myRole === 'creator' && m.role !== 'creator' && !isMe
                    const canKick = (myRole === 'creator' || myRole === 'admin') && !isMe && m.role !== 'creator' && (myRole === 'creator' || m.role === 'member')
                    return (
                      <li key={m.user_id} className="chat-member-item">
                        <span>
                          {m.address} {m.role !== 'member' && <span className="chat-member-role">({m.role})</span>}
                        </span>
                        {(canTransfer || canKick) && (
                          <span className="chat-member-actions-inline">
                            {canTransfer && (
                              <button
                                type="button"
                                onClick={() => handleTransferCreator(m.username)}
                                disabled={!!roomActionLoading}
                                className="chat-vpn-btn chat-btn-xs"
                                title="Передать создание"
                              >
                                {roomActionLoading === 'transfer' ? '…' : 'Создатель'}
                              </button>
                            )}
                            {canKick && (
                              <button
                                type="button"
                                onClick={() => handleRemoveMember(m.username)}
                                disabled={!!roomActionLoading}
                                className="chat-vpn-btn-danger chat-btn-xs"
                                title="Исключить"
                              >
                                {roomActionLoading === 'remove' ? '…' : 'Исключить'}
                              </button>
                            )}
                          </span>
                        )}
                      </li>
                    )
                  })}
                </ul>
                <div className="chat-member-actions">
                  <input
                    type="text"
                    placeholder="Username для приглашения"
                    value={inviteUsername}
                    onChange={(e) => setInviteUsername(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && handleInvite()}
                    className="chat-recipient-input chat-recipient-input-flex"
                  />
                  <button className="chat-vpn-btn" onClick={handleInvite} disabled={inviteLoading || !inviteUsername.trim()}>
                    {inviteLoading ? '…' : 'Пригласить'}
                  </button>
                </div>
              </div>
            )}
            {identityKeyChanged && identityKeyChanged.recipient === normalizeRecipient(recipient, user.user_id) && (
              <div className={`chat-identity-warning ${e2eeMode === 'compatibility' ? 'chat-identity-warning--critical' : ''}`}>
                <p>
                  ⚠️ Ключ собеседника изменился. Возможна атака или переустановка.
                </p>
                <div className="chat-identity-actions">
                  <button type="button" onClick={trustNewKey} className="chat-vpn-btn chat-btn-md">
                    Подтвердить новый ключ
                  </button>
                  <button type="button" onClick={showFingerprint} disabled={fingerprintLoading} className="chat-vpn-btn chat-btn-md">
                    {fingerprintLoading ? '…' : 'Проверить отпечаток'}
                  </button>
                </div>
              </div>
            )}
            {sendHint && (
              <p className={`chat-input-hint chat-input-hint-dynamic ${sendHintError ? 'chat-input-hint-error' : 'chat-input-hint-accent'}`}>
                {sendHint}
              </p>
            )}
          </div>

          <div className="chat-messages messages-scroll">
            {filteredEvents.length === 0 && (
              <div className="chat-empty-state">
                <span className="chat-empty-icon">💬</span>
                <p className="chat-empty-text">
                  {recipient.trim()
                    ? isRoomAddr(normalizeRecipient(recipient, user.user_id))
                      ? 'Нет сообщений в комнате'
                      : 'Введите получателя и начните диалог'
                    : 'Выберите чат или введите получателя'}
                </p>
                <p className="chat-empty-hint">Сообщения зашифрованы сквозным шифрованием</p>
              </div>
            )}
            {filteredEvents.length > 0 && searchFilteredEvents.length === 0 && searchQuery.trim() && (
              <div className="chat-empty-state">
                <span className="chat-empty-icon">🔍</span>
                <p className="chat-empty-text">Ничего не найдено по «{searchQuery.trim()}»</p>
              </div>
            )}
            {searchFilteredEvents.map((ev) => {
              const isOwn = ev.sender === user.user_id
              const parsed = ev.ciphertext ? parseMessage(ev.ciphertext, ev.event_id) : { type: 'text' as const, text: '[encrypted]' }
              const encrypted = ev.ciphertext ? (isE2EEPayload(ev.ciphertext) || isSignalCiphertext(ev.ciphertext) || ev.ciphertext.startsWith('sk1:')) : false
              // Наши сообщения из sync зашифрованы для получателя — не показываем [decrypt failed]
              const displayText = isOwn && parsed.type === 'text' && parsed.text === '[decrypt failed]'
                ? 'Ваше сообщение'
                : parsed.type === 'text' ? sanitizeForDisplay(parsed.text) : ''
              return (
                <div key={ev.event_id} className={`chat-bubble-wrap ${isOwn ? 'chat-bubble-wrap--own' : ''}`}>
                  <div className={`msg-bubble chat-bubble ${isOwn ? 'chat-bubble--own' : 'chat-bubble--in'}`}>
                    {!isOwn && <span className="chat-sender">{ev.sender}</span>}
                    {parsed.type === 'file' ? (
                      <FilePreview
                        contentUri={parsed.content_uri}
                        filename={parsed.filename}
                        size={parsed.size}
                        token={token}
                        onDownload={() => downloadFileMessage(parsed.content_uri, parsed.filename, parsed.file_key, parsed.nonce, parsed.file_sha256, parsed.chunk_size, parsed.e2ee_version)}
                        fileKey={parsed.file_key}
                        nonce={parsed.nonce}
                        fileSha256={parsed.file_sha256}
                        chunkSize={parsed.chunk_size}
                        e2eeVersion={parsed.e2ee_version}
                      />
                    ) : (
                      <span className="chat-msg-text">{displayText}</span>
                    )}
                    <div className="chat-bubble-meta">
                      {encrypted && <span className="chat-e2ee-badge" title="Шифрование E2EE">🔒</span>}
                      {isOwn && (
                        <span className="chat-delivery-badge" title={ev.read_at ? 'Прочитано' : ev.status === 'delivered' ? 'Доставлено' : 'Отправлено'}>
                          {ev.read_at ? '✓✓' : ev.status === 'delivered' ? '✓' : '⋯'}
                        </span>
                      )}
                      <span className="chat-timestamp">{formatTime(ev.timestamp)}</span>
                    </div>
                  </div>
                </div>
              )
            })}
            <div ref={messagesEndRef} />
          </div>

          <div className="chat-input-wrap">
            <div className="chat-input-bar">
              <label className="chat-file-label">
              <input
                type="file"
                multiple
                className="chat-input-hidden"
                onChange={(e) => {
                  if (e.target.files?.length) sendFiles(e.target.files)
                  e.target.value = ''
                }}
                disabled={uploading || !recipient || (!!recipient.trim() && !recipient.includes(':') && !recipient.startsWith('!'))}
              />
              {uploading ? '⏳' : '📎'}
            </label>
            <textarea
              placeholder="Сообщение..."
              value={message}
              onChange={(e) => {
                setMessage(e.target.value)
                if (sendHint) {
                  setSendHint(null)
                  setSendHintError(false)
                }
              }}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault()
                  sendMessage()
                }
              }}
              rows={1}
              className="chat-message-input"
            />
            <button
              onClick={sendMessage}
              disabled={uploading || !recipient || sending || (!!recipient.trim() && !recipient.includes(':') && !recipient.startsWith('!'))}
              className="chat-send-btn btn-primary"
            >
              {sending ? '…' : 'Отправить →'}
            </button>
            </div>
            <p className="chat-input-hint">Enter — отправить, Shift+Enter — новая строка</p>
          </div>
        </div>
        </>
        )}
      {fingerprintModal && (() => {
        const close = () => { setFingerprintModal(null); setFingerprintCopied(false); setFingerprintQrUrl(null) }
        const handleCopy = async () => {
          try {
            await navigator.clipboard.writeText(fingerprintModal.safetyNumber)
            setFingerprintCopied(true)
            setTimeout(() => setFingerprintCopied(false), 2000)
          } catch {
            // clipboard API denied
          }
        }
        return (
        <div
          role="dialog"
          aria-modal="true"
          aria-labelledby="fingerprint-title"
          className="chat-fingerprint-backdrop"
          onClick={close}
        >
          <div
            className="chat-fingerprint-dialog"
            onClick={(e) => e.stopPropagation()}
          >
            <h3 id="fingerprint-title" className="chat-fingerprint-title">Отпечаток ключа (Safety number)</h3>
            <p className="chat-fingerprint-desc">
              Сравните с {fingerprintModal.recipient} лично. Если совпадает — канал защищён от MITM.
            </p>
            {fingerprintQrUrl && (
              <div className="chat-fingerprint-qr" aria-hidden="true">
                <img src={fingerprintQrUrl} alt="" width={180} height={180} />
              </div>
            )}
            <pre className="chat-fingerprint-pre">
              {fingerprintModal.safetyNumber}
            </pre>
            {fingerprintModal.trustHistory && (fingerprintModal.trustHistory.seenAt != null || fingerprintModal.trustHistory.verifiedAt != null || fingerprintModal.trustHistory.changedAt != null) && (
              <div className="chat-fingerprint-history">
                <p className="chat-fingerprint-history-title">
                  {fingerprintModal.trustHistory.status === 'changed' ? 'История предыдущего ключа' : 'История ключа'}
                </p>
                {fingerprintModal.trustHistory.seenAt != null && (
                  <p className="chat-fingerprint-history-item">Впервые увиден: {formatTrustDate(fingerprintModal.trustHistory.seenAt)}</p>
                )}
                {fingerprintModal.trustHistory.verifiedAt != null && (
                  <p className="chat-fingerprint-history-item">Подтверждён: {formatTrustDate(fingerprintModal.trustHistory.verifiedAt)}</p>
                )}
                {fingerprintModal.trustHistory.changedAt != null && (
                  <p className="chat-fingerprint-history-item">Изменён (принят): {formatTrustDate(fingerprintModal.trustHistory.changedAt)}</p>
                )}
              </div>
            )}
            <div className="chat-fingerprint-actions">
              <button
                type="button"
                onClick={handleCopy}
                className="chat-vpn-btn"
                title="Скопировать в буфер"
              >
                {fingerprintCopied ? 'Скопировано' : 'Копировать'}
              </button>
              <button
                type="button"
                onClick={async () => {
                  await markVerified(fingerprintModal.recipient, 'manual', fingerprintModal.identityKey, fingerprintModal.deviceId)
                  close()
                }}
                className="chat-vpn-btn"
              >
                Подтвердить
              </button>
              {fingerprintModal.trustHistory?.status === 'verified' && (
                <button
                  type="button"
                  onClick={async () => {
                    if (!window.confirm('Снять доверие с этого ключа? В Strict режиме отправка будет заблокирована до повторного подтверждения.')) return
                    await clearVerified(fingerprintModal.recipient, fingerprintModal.deviceId)
                    close()
                  }}
                  className="chat-vpn-btn chat-vpn-btn-danger"
                  title="Снять доверие / сбросить"
                >
                  Не доверять
                </button>
              )}
              <button
                type="button"
                onClick={close}
                className="chat-vpn-btn chat-fingerprint-close"
              >
                Закрыть
              </button>
            </div>
          </div>
        </div>
        )
      })()}
      </main>
    </div>
  )
}
