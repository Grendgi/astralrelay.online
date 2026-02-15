import { useState, useEffect, useCallback, useRef, type ReactNode } from 'react'
import { api, ApiError, type SyncEvent } from '../api/client'
import type { AuthUser, StoredKeys } from '../hooks/useAuth'

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
}: {
  contentUri: string
  filename: string
  size?: number
  token: string
  onDownload: () => void
  lazy?: boolean
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
        const blob = await res.blob()
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
  }, [contentUri, token, canPreview, filename, lazy, inView])

  const sizeStr = size != null ? formatFileSize(size) : null

  const wrap = (children: ReactNode) => (
    <div ref={containerRef} style={{ display: 'block', minHeight: 1 }}>
      {children}
    </div>
  )

  if (!canPreview || error) {
    return wrap(
      <button type="button" onClick={onDownload} style={filePreviewStyles.downloadBtn}>
        📎 {filename}
        {sizeStr && <span style={{ opacity: 0.8, fontSize: '0.9em', marginLeft: 4 }}>({sizeStr})</span>}
      </button>
    )
  }

  if (loading) {
    return wrap(
      <div style={{ ...filePreviewStyles.downloadBtn, cursor: 'default', textDecoration: 'none' }}>
        ⏳ Загрузка {filename}…
      </div>
    )
  }

  if (isImage && url) {
    return wrap(
      <>
        <div style={filePreviewStyles.preview}>
          <img
            src={url}
            alt={filename}
            style={filePreviewStyles.image}
            onClick={(e) => { e.stopPropagation(); setLightboxOpen(true) }}
            title="Клик — открыть в полном размере"
            onError={() => {
              setUrl((u) => { if (u) URL.revokeObjectURL(u); return null })
              setError(true)
            }}
          />
          <button type="button" onClick={onDownload} style={filePreviewStyles.downloadBtn}>
            📎 {filename}
            {sizeStr && <span style={{ opacity: 0.8, fontSize: '0.9em', marginLeft: 4 }}>({sizeStr})</span>}
          </button>
        </div>
        {lightboxOpen && (
          <div
            role="dialog"
            aria-modal="true"
            style={filePreviewStyles.lightboxBackdrop}
            onClick={() => setLightboxOpen(false)}
          >
            <img
              src={url}
              alt={filename}
              style={filePreviewStyles.lightboxImage}
              onClick={(e) => e.stopPropagation()}
            />
            <button
              type="button"
              onClick={() => setLightboxOpen(false)}
              style={filePreviewStyles.lightboxClose}
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
      <div style={filePreviewStyles.preview}>
        <video src={url} controls style={filePreviewStyles.video} />
        <button type="button" onClick={onDownload} style={filePreviewStyles.downloadBtn}>
          📎 {filename}
          {sizeStr && <span style={{ opacity: 0.8, fontSize: '0.9em', marginLeft: 4 }}>({sizeStr})</span>}
        </button>
      </div>
    )
  }

  if (isAudio && url) {
    return wrap(
      <div style={filePreviewStyles.preview}>
        <audio src={url} controls style={filePreviewStyles.audio} />
        <button type="button" onClick={onDownload} style={filePreviewStyles.downloadBtn}>
          📎 {filename}
          {sizeStr && <span style={{ opacity: 0.8, fontSize: '0.9em', marginLeft: 4 }}>({sizeStr})</span>}
        </button>
      </div>
    )
  }

  if (isPdf && url) {
    return wrap(
      <div style={filePreviewStyles.preview}>
        <iframe src={url} title={filename} style={filePreviewStyles.pdf} />
        <button type="button" onClick={onDownload} style={filePreviewStyles.downloadBtn}>
          📎 {filename}
          {sizeStr && <span style={{ opacity: 0.8, fontSize: '0.9em', marginLeft: 4 }}>({sizeStr})</span>}
        </button>
      </div>
    )
  }

  return wrap(null)
}

const filePreviewStyles: Record<string, React.CSSProperties> = {
  preview: {
    display: 'flex',
    flexDirection: 'column',
    gap: 8,
    maxWidth: 'min(400px, 90vw)',
  },
  image: {
    maxWidth: '100%',
    maxHeight: 320,
    borderRadius: 8,
    cursor: 'pointer',
  },
  video: {
    maxWidth: '100%',
    maxHeight: 320,
    borderRadius: 8,
  },
  audio: {
    width: '100%',
    maxWidth: 320,
  },
  pdf: {
    width: '100%',
    height: 400,
    border: '1px solid var(--border)',
    borderRadius: 8,
  },
  downloadBtn: {
    display: 'inline-flex',
    alignItems: 'center',
    background: 'none',
    border: 'none',
    color: 'inherit',
    cursor: 'pointer',
    padding: 0,
    font: 'inherit',
    textDecoration: 'underline',
    fontSize: 13,
  },
  lightboxBackdrop: {
    position: 'fixed',
    inset: 0,
    zIndex: 10000,
    background: 'rgba(0,0,0,0.85)',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    cursor: 'pointer',
  },
  lightboxImage: {
    maxWidth: '95vw',
    maxHeight: '95vh',
    objectFit: 'contain',
    cursor: 'default',
    borderRadius: 8,
  },
  lightboxClose: {
    position: 'fixed',
    top: 16,
    right: 16,
    width: 40,
    height: 40,
    borderRadius: '50%',
    border: 'none',
    background: 'rgba(255,255,255,0.2)',
    color: '#fff',
    fontSize: 18,
    cursor: 'pointer',
  },
}
import { encrypt, decrypt, isE2EEPayload } from '../crypto/e2ee'
import { computeSafetyNumber } from '../crypto/fingerprint'
import { signalEncrypt, signalDecrypt, isSignalCiphertext, uuidToSignalDeviceId } from '../crypto/signal'
import { createBackup, restoreBackup } from '../crypto/backup'
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
}

export function Chat({ user, token, keys, onLogout }: ChatProps) {
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
  const [sendHint, setSendHint] = useState<string | null>(null)
  const [sendHintError, setSendHintError] = useState(false)
  const [sending, setSending] = useState(false)
  const [rooms, setRooms] = useState<Array<{ id: string; name: string; domain: string; address: string }>>([])
  const [roomsLoading, setRoomsLoading] = useState(false)
  const [createRoomName, setCreateRoomName] = useState('')
  const [createRoomLoading, setCreateRoomLoading] = useState(false)
  const [roomMembers, setRoomMembers] = useState<Array<{ user_id: number; username: string; domain: string; address: string; role: string }>>([])
  const [roomMembersVisible, setRoomMembersVisible] = useState(false)
  const [inviteUsername, setInviteUsername] = useState('')
  const [inviteLoading, setInviteLoading] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const [mainView, setMainView] = useState<'chats' | 'vpn'>(() => (sessionStorage.getItem('main_view') as 'chats' | 'vpn') || 'chats')
  useEffect(() => {
    sessionStorage.setItem('main_view', mainView)
  }, [mainView])
  const [leaveLoading, setLeaveLoading] = useState(false)
  const [roomActionLoading, setRoomActionLoading] = useState<string | null>(null)
  const [soundEnabled, setSoundEnabled] = useState(() => {
    try {
      return localStorage.getItem('chat_sound_enabled') !== 'false'
    } catch { return true }
  })
  const [pushEnabled, setPushEnabled] = useState(false)
  const [pushLoading, setPushLoading] = useState(false)
  const [decryptedSignal, setDecryptedSignal] = useState<Record<string, string>>({})
  const [fingerprintModal, setFingerprintModal] = useState<{ recipient: string; safetyNumber: string } | null>(null)
  const [fingerprintLoading, setFingerprintLoading] = useState(false)
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

  const showFingerprint = useCallback(async () => {
    const r = normalizeRecipient(recipient, user.user_id)
    if (!r || isRoomAddr(r) || !keys) return
    setFingerprintLoading(true)
    setFingerprintModal(null)
    try {
      const bundle = await api.getKeys(r, token)
      const safetyNumber = await computeSafetyNumber(keys.identityKey, bundle.identity_key)
      setFingerprintModal({ recipient: r, safetyNumber })
    } catch {
      setFingerprintModal({ recipient: r, safetyNumber: '— ключи недоступны —' })
    } finally {
      setFingerprintLoading(false)
    }
  }, [recipient, user.user_id, keys, token])

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [events, recipient])

  // Асинхронная расшифровка Signal (sig1:) сообщений
  useEffect(() => {
    if (!keys?.identitySecret || !keys?.signedPrekey?.secret) return
    const ourKeys = { identityKey: keys.identityKey, identitySecret: keys.identitySecret, signedPrekey: keys.signedPrekey }
    events
      .filter((ev) => ev.ciphertext?.startsWith('sig1:'))
      .forEach((ev) => {
        signalDecrypt(ev.ciphertext!, ourKeys, ev.sender, ev.sender_device).then(
          (plain) => setDecryptedSignal((prev) => (prev[ev.event_id] ? prev : { ...prev, [ev.event_id]: plain })),
          () => setDecryptedSignal((prev) => (prev[ev.event_id] ? prev : { ...prev, [ev.event_id]: '[decrypt failed]' }))
        )
      })
  }, [events, keys])

  const sync = useCallback(async () => {
    try {
      const res = await api.sync(cursor, token)
      if (res.events.length) {
        let newIncomingCount = 0
        setEvents((prev) => {
          const ids = new Set(prev.map((e) => e.event_id))
          const newEvents = res.events.filter((e) => !ids.has(e.event_id))
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
    if (!recipient) {
      sessionStorage.removeItem('chat_recipient')
      return
    }
    sessionStorage.setItem('chat_recipient', recipient)
  }, [recipient])

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
      console.error(err)
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
      console.error(err)
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
      fetchRoomMembers()
    } catch (err) {
      console.error(err)
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
      console.error(err)
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
      fetchRoomMembers()
    } catch (err) {
      console.error(err)
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
      console.error(err)
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
      const payload = {
        identityKey: keys.identityKey,
        identitySecret: keys.identitySecret,
        ...(keys.identitySigningKey && { identitySigningKey: keys.identitySigningKey }),
        signedPrekey: keys.signedPrekey,
        oneTimePrekeys: keys.oneTimePrekeys,
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
      localStorage.setItem('keys', JSON.stringify({
        identityKey: payload.identityKey,
        identitySecret: payload.identitySecret,
        signedPrekey: payload.signedPrekey,
        oneTimePrekeys: payload.oneTimePrekeys ?? [],
      }))
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
      console.error(err)
    } finally {
      setVpnLoading(null)
    }
  }

  const encryptForRoom = async (roomAddr: string, payload: string): Promise<Record<string, string> | null> => {
    if (!keys) return null
    const roomId = roomAddr.startsWith('!') ? roomAddr.slice(1).split(':')[0] : ''
    if (!roomId) return null
    const { members } = await api.roomsMembers(roomId, token).catch(() => ({ members: [] }))
    const ciphertexts: Record<string, string> = {}
    for (const m of members || []) {
      try {
        const bundle = await api.getKeys(m.address, token)
        ciphertexts[m.address] = encrypt(payload, bundle)
      } catch {
        // skip member without keys
      }
    }
    return Object.keys(ciphertexts).length > 0 ? ciphertexts : null
  }

  const sendFile = async (file: File) => {
    if (!recipient) return
    const r = normalizeRecipient(recipient, user.user_id)
    setSendHint(null)
    setSendHintError(false)
    try {
      const { content_uri } = await api.uploadFile(file, token)
      const payload = JSON.stringify({
        message_type: 'file',
        content_uri,
        filename: file.name,
        size: file.size,
      })
      const isRoom = isRoomAddr(r)
      let content: { ciphertext?: string; ciphertexts?: Record<string, string>; session_id: string }
      if (keys && isRoom) {
        const ciphertexts = await encryptForRoom(r, payload)
        content = ciphertexts ? { ciphertexts, session_id: 'sess_mvp' } : { ciphertext: btoa(unescape(encodeURIComponent(payload))), session_id: 'sess_mvp' }
      } else if (keys && !isRoom) {
        try {
          const bundle = await api.getKeys(r, token)
          const ourKeys = { identityKey: keys.identityKey, identitySecret: keys.identitySecret, signedPrekey: keys.signedPrekey }
          const ctRecipient = await signalEncrypt(payload, bundle, ourKeys, r).catch(() => encrypt(payload, bundle))
          const selfBundle = { identity_key: keys.identityKey, signed_prekey: { key: keys.signedPrekey.key, signature: keys.signedPrekey.signature, key_id: keys.signedPrekey.key_id ?? 1 } }
          const ctSelf = await signalEncrypt(payload, selfBundle, ourKeys, user.user_id, uuidToSignalDeviceId(user.device_id)).catch(() => encrypt(payload, selfBundle))
          content = { ciphertexts: { [r]: ctRecipient, [user.user_id]: ctSelf }, session_id: 'sess_mvp' }
        } catch (e) {
          if (e instanceof ApiError && e.status === 404) {
            setSendHint('Пользователь не найден')
            setSendHintError(true)
            return
          }
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
        console.error(e)
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
          const bundle = await api.getKeys(r, token)
          const ourKeys = { identityKey: keys.identityKey, identitySecret: keys.identitySecret, signedPrekey: keys.signedPrekey }
          const ctRecipient = await signalEncrypt(plaintext, bundle, ourKeys, r).catch(() => encrypt(plaintext, bundle))
          const selfBundle = { identity_key: keys.identityKey, signed_prekey: { key: keys.signedPrekey.key, signature: keys.signedPrekey.signature, key_id: keys.signedPrekey.key_id ?? 1 } }
          const ctSelf = await signalEncrypt(plaintext, selfBundle, ourKeys, user.user_id, uuidToSignalDeviceId(user.device_id)).catch(() => encrypt(plaintext, selfBundle))
          content = { ciphertexts: { [r]: ctRecipient, [user.user_id]: ctSelf }, session_id: 'sess_mvp' }
        } catch (e) {
          if (e instanceof ApiError && e.status === 404) {
            setSendHint('Пользователь не найден')
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
        console.error(e)
        setSendHint('Не удалось отправить. Проверьте соединение и попробуйте снова.')
        setSendHintError(true)
      }
    } finally {
      setSending(false)
    }
  }

  type ParsedMessage = { type: 'text'; text: string } | { type: 'file'; filename: string; content_uri: string; size?: number }

  const parseMessage = (ciphertext: string, eventId?: string): ParsedMessage => {
    if (!ciphertext) return { type: 'text', text: '[encrypted]' }
    let plaintext: string
    // Signal (sig1:) — асинхронная расшифровка, результат в decryptedSignal
    if (ciphertext.startsWith('sig1:') && eventId && decryptedSignal[eventId]) {
      plaintext = decryptedSignal[eventId]
    } else if (ciphertext.startsWith('sig1:')) {
      return { type: 'text', text: '[decrypting...]' }
    } else if (ciphertext.startsWith('opt:')) {
      try {
        plaintext = decodeURIComponent(escape(atob(ciphertext.slice(4))))
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
        return { type: 'file', filename: parsed.filename || 'file', content_uri: parsed.content_uri, size: parsed.size }
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
      console.error('Push subscribe:', e)
    } finally {
      setPushLoading(false)
    }
  }, [token])

  const downloadFileMessage = async (contentUri: string, filename: string) => {
    try {
      const res = await api.downloadFile(contentUri, token)
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = filename
      a.click()
      URL.revokeObjectURL(url)
    } catch (err) {
      console.error(err)
    }
  }

  return (
    <div className="chat-layout" style={styles.layout}>
      <header style={styles.header}>
        <div style={styles.headerLeft}>
          <span style={styles.headerUser}>{user.user_id}</span>
          <span
            style={{
              display: 'inline-block',
              marginLeft: 6,
              width: 8,
              height: 8,
              borderRadius: '50%',
              backgroundColor: wsConnected ? 'var(--success, #22c55e)' : 'var(--muted, #94a3b8)',
              flexShrink: 0,
            }}
            title={wsConnected ? 'Подключено' : 'Нет соединения — переподключение…'}
          />
          {!wsConnected && (
            <span style={{ marginLeft: 6, fontSize: 12, color: 'var(--text-muted, #94a3b8)' }}>Переподключение…</span>
          )}
          <span style={{ ...styles.e2eeStatus, opacity: keys ? 1 : 0.6, marginLeft: 6 }} title={keys ? 'E2EE ключи загружены' : 'E2EE недоступен — войдите заново или восстановите ключи'}>
            {keys ? '🔒' : '⚠️'}
          </span>
        </div>
        <div style={styles.headerActions}>
          <div style={{ display: 'flex', gap: 4, marginRight: 8 }}>
            <button
              type="button"
              onClick={() => setMainView('chats')}
              style={{
                ...styles.tabBtn,
                ...(mainView === 'chats' ? styles.tabBtnActive : {}),
              }}
            >
              Чаты
            </button>
            <button
              type="button"
              onClick={() => setMainView('vpn')}
              style={{
                ...styles.tabBtn,
                ...(mainView === 'vpn' ? styles.tabBtnActive : {}),
              }}
            >
              VPN
            </button>
          </div>
          {'PushManager' in window && (
            <button
              type="button"
              onClick={enablePush}
              disabled={pushLoading || pushEnabled}
              style={styles.themeBtn}
              title={pushEnabled ? 'Push включены' : 'Включить push-уведомления'}
            >
              {pushLoading ? '…' : pushEnabled ? '📲' : '📱'}
            </button>
          )}
          <button
            onClick={() => setSoundEnabled((v) => !v)}
            style={styles.themeBtn}
            title={soundEnabled ? 'Звук вкл — выключить' : 'Звук выкл — включить'}
          >
            {soundEnabled ? '🔔' : '🔕'}
          </button>
          <button onClick={toggleTheme} style={styles.themeBtn} title={theme === 'dark' ? 'Светлая тема' : 'Тёмная тема'}>
            {theme === 'dark' ? '☀️' : '🌙'}
          </button>
          <button onClick={onLogout} style={styles.logout}>
            Выход
          </button>
        </div>
      </header>

      <main style={styles.main} className="chat-main">
        {mainView === 'vpn' ? (
          <>
            <div style={{ ...styles.sidebar, width: 120, minWidth: 120 }}>
              <div style={styles.sidebarSection}>
                <button
                  type="button"
                  onClick={() => setMainView('chats')}
                  style={{ ...styles.vpnBtn, width: '100%', marginTop: 8 }}
                >
                  ← Чаты
                </button>
              </div>
            </div>
            <div style={{ flex: 1, overflow: 'auto', padding: 24 }}>
              <h2 style={{ margin: '0 0 20px', fontSize: 20 }}>VPN</h2>
              <p style={{ ...styles.vpnHint, marginBottom: 20 }}>Скачайте конфиг и подключитесь. Только ваши конфиги.</p>
              {vpnNodes.length > 1 && (
                <div style={{ marginBottom: 16 }}>
                  <label style={{ fontSize: 13, color: 'var(--text-muted)', display: 'block', marginBottom: 6 }}>Нода</label>
                  <select
                    value={selectedNodeId}
                    onChange={(e) => setSelectedNodeId(e.target.value)}
                    style={{ ...styles.recipientInput, padding: '8px 12px', maxWidth: 320 }}
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
              <div style={{ marginBottom: 24 }}>
                <h3 style={{ margin: '0 0 12px', fontSize: 16 }}>Протоколы</h3>
                {vpnProtocols.length === 0 ? (
                  <p style={styles.vpnProtoHint}>Загрузка…</p>
                ) : (
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                    {vpnProtocols.map((p) => (
                      <div key={p.id} style={styles.vpnProtocol}>
                        <div>
                          <strong>{p.name}</strong>
                          <p style={styles.vpnProtoHint}>{p.hint}</p>
                        </div>
                        <button style={styles.vpnBtn} onClick={() => downloadVpnConfig(p.id)} disabled={vpnLoading === p.id}>
                          {vpnLoading === p.id ? '…' : 'Скачать'}
                        </button>
                      </div>
                    ))}
                  </div>
                )}
              </div>
              <div style={styles.vpnAdminSection}>
                <h3 style={{ margin: '0 0 12px', fontSize: 16 }}>Мои конфиги</h3>
                <p style={styles.vpnHint}>Только ваши выданные конфиги</p>
                {vpnMyConfigs.length === 0 ? (
                  <p style={styles.vpnProtoHint}>Нет конфигов. Скачайте протокол выше.</p>
                ) : (
                  vpnMyConfigs.map((c, i) => (
                    <div key={`${c.device_id}-${c.protocol}-${i}`} style={styles.vpnAdminRow}>
                      <div>
                        <strong>{c.protocol}</strong>{c.node_name ? ` · ${c.node_name}` : ''}
                        <p style={styles.vpnProtoHint}>
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
                      <div style={styles.vpnAdminActions}>
                        <button style={styles.vpnBtn} onClick={() => downloadVpnConfig(c.protocol)} disabled={vpnLoading === c.protocol} title="Скачать снова">↻</button>
                        <button style={styles.vpnBtnDanger} onClick={() => vpnRevoke(c)} title="Отозвать">✕</button>
                      </div>
                    </div>
                  ))
                )}
              </div>
              <div style={styles.vpnAdminSection}>
                <h3 style={{ margin: '0 0 12px', fontSize: 16 }}>Резервная копия ключей</h3>
                <p style={styles.vpnHint}>Без сервера расшифровать нельзя</p>
                {backupError && <p style={{ ...styles.vpnProtoHint, color: 'var(--error)' }}>{backupError}</p>}
                <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                  <button style={styles.vpnBtn} onClick={handleCreateBackup} disabled={backupLoading || !keys} title="Создать зашифрованный бэкап">
                    {backupLoading ? '…' : 'Создать бэкап'}
                  </button>
                  <div>
                    <input ref={restoreInputRef} type="file" accept=".dat" style={{ display: 'none' }} onChange={(e) => { if (e.target.files?.[0]) handleRestoreBackup(); e.target.value = '' }} />
                    <button style={styles.vpnBtn} onClick={() => restoreInputRef.current?.click()} disabled={backupLoading}>Восстановить из бэкапа</button>
                  </div>
                </div>
              </div>
            </div>
          </>
        ) : (
        <>
        <div style={styles.sidebar} className="chat-sidebar">
          <div style={styles.sidebarSection}>
            <h3 style={styles.sidebarTitle}>Чаты</h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4, maxHeight: 200, overflowY: 'auto' }}>
              {chatList.length === 0 ? (
                <p style={styles.vpnProtoHint}>Нет чатов. Начните новый ниже.</p>
              ) : (
                chatList.map(({ addr, label }) => {
                  const unread = getUnreadCount(addr)
                  return (
                    <button
                      key={addr}
                      type="button"
                      onClick={() => setRecipient(addr)}
                      style={{
                        ...styles.roomItem,
                        background: recipient === addr ? 'var(--accent-subtle)' : 'transparent',
                        borderColor: recipient === addr ? 'var(--accent)' : 'transparent',
                      }}
                    >
                      <span style={{ fontWeight: 500, flex: 1, textAlign: 'left' }}>{label}</span>
                      {unread > 0 && (
                        <span style={styles.unreadBadge}>{unread > 99 ? '99+' : unread}</span>
                      )}
                    </button>
                  )
                })
              )}
            </div>
            <div style={{ marginTop: 10 }}>
              <input
                type="text"
                placeholder="Новый чат: пользователь или @user:domain"
                value={recipient}
                onChange={(e) => { setRecipient(e.target.value); setSendHint(null) }}
                style={{ ...styles.recipientInput, padding: '8px 10px', fontSize: 13 }}
              />
            </div>
          </div>
          <div style={styles.sidebarSection}>
            <h4 style={styles.vpnTitle}>Комнаты</h4>
            <div style={{ display: 'flex', gap: 8, marginBottom: 8 }}>
              <input
                type="text"
                placeholder="Название комнаты"
                value={createRoomName}
                onChange={(e) => setCreateRoomName(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && handleCreateRoom()}
                style={{ ...styles.recipientInput, flex: 1, padding: '6px 10px' }}
              />
              <button
                style={styles.vpnBtn}
                onClick={handleCreateRoom}
                disabled={createRoomLoading || !createRoomName.trim()}
                title="Создать комнату"
              >
                {createRoomLoading ? '…' : '+'}
              </button>
            </div>
            {roomsLoading ? (
              <p style={styles.vpnProtoHint}>Загрузка…</p>
            ) : rooms.length === 0 ? (
              <p style={styles.vpnProtoHint}>Нет комнат. Создайте комнату выше.</p>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                {rooms.map((room) => {
                  const unread = getUnreadCount(room.address)
                  return (
                  <button
                    key={room.id}
                    type="button"
                    onClick={() => setRecipient(room.address)}
                    style={{
                      ...styles.roomItem,
                      background: recipient === room.address ? 'var(--accent-subtle)' : 'transparent',
                      borderColor: recipient === room.address ? 'var(--accent)' : 'transparent',
                    }}
                  >
                    <span style={{ fontWeight: 500, flex: 1, textAlign: 'left' }}>{room.name}</span>
                    {unread > 0 && (
                      <span style={styles.unreadBadge}>{unread > 99 ? '99+' : unread}</span>
                    )}
                  </button>
                )
                })}
              </div>
            )}
          </div>
          {vpnProtocols.length > 0 && (
            <div style={styles.vpnSection}>
              <div style={styles.sidebarSection}>
              <h4 style={styles.vpnTitle}>VPN</h4>
              <p style={styles.vpnHint}>Не работает? Попробуй другой протокол</p>
              {vpnNodes.length > 1 && (
                <div style={{ marginBottom: 8 }}>
                  <label style={{ fontSize: 12, color: 'var(--text-muted)' }}>Нода: </label>
                  <select
                    value={selectedNodeId}
                    onChange={(e) => setSelectedNodeId(e.target.value)}
                    style={{ ...styles.recipientInput, padding: 4, marginTop: 4 }}
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
                <div key={p.id} style={styles.vpnProtocol}>
                  <div>
                    <strong>{p.name}</strong>
                    <p style={styles.vpnProtoHint}>{p.hint}</p>
                  </div>
                  <button
                    style={styles.vpnBtn}
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
            <div style={styles.vpnAdminSection}>
              <h4 style={styles.vpnTitle}>Мои конфиги</h4>
              <p style={styles.vpnHint}>Только ваши выданные конфиги</p>
              {vpnMyConfigs.length === 0 ? (
                <p style={styles.vpnProtoHint}>Нет конфигов. Скачайте протокол выше.</p>
              ) : (
                vpnMyConfigs.map((c, i) => (
                  <div key={`${c.device_id}-${c.protocol}-${i}`} style={styles.vpnAdminRow}>
                    <div>
                      <strong>{c.protocol}</strong>{c.node_name ? ` · ${c.node_name}` : ''}
                      <p style={styles.vpnProtoHint}>
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
                    <div style={styles.vpnAdminActions}>
                      <button style={styles.vpnBtn} onClick={() => downloadVpnConfig(c.protocol)} disabled={vpnLoading === c.protocol} title="Скачать снова">
                        ↻
                      </button>
                      <button style={styles.vpnBtnDanger} onClick={() => vpnRevoke(c)} title="Отозвать">
                        ✕
                      </button>
                    </div>
                  </div>
                ))
              )}
            </div>
          )}
          <div style={styles.vpnAdminSection}>
            <h4 style={styles.vpnTitle}>Резервная копия ключей</h4>
            <p style={styles.vpnHint}>Без сервера расшифровать нельзя</p>
            {backupError && <p style={{ ...styles.vpnProtoHint, color: 'var(--error)' }}>{backupError}</p>}
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              <button
                style={styles.vpnBtn}
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
                  style={{ display: 'none' }}
                  onChange={(e) => {
                    if (e.target.files?.[0]) handleRestoreBackup()
                    e.target.value = ''
                  }}
                />
                <button
                  style={styles.vpnBtn}
                  onClick={() => restoreInputRef.current?.click()}
                  disabled={backupLoading}
                >
                  Восстановить из бэкапа
                </button>
              </div>
            </div>
          </div>
        </div>

        <div style={styles.chat}>
          <div style={styles.recipientBar}>
            {recipient.trim() ? (
              <div style={{ display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap', padding: '4px 0' }}>
                <p style={{ ...styles.chatWith, margin: 0 }}>
                  {isRoomAddr(normalizeRecipient(recipient, user.user_id))
                    ? `Комната: ${rooms.find((r) => r.address === normalizeRecipient(recipient, user.user_id))?.name ?? recipient}`
                    : `Чат с ${normalizeRecipient(recipient, user.user_id)}`}
                </p>
                {!isRoomAddr(normalizeRecipient(recipient, user.user_id)) && keys && (
                  <button
                    type="button"
                    onClick={showFingerprint}
                    disabled={fingerprintLoading}
                    style={{ ...styles.themeBtn, fontSize: 12, padding: '4px 8px' }}
                    title="Проверить ключ (Safety number)"
                  >
                    {fingerprintLoading ? '…' : '🔐 Отпечаток'}
                  </button>
                )}
                {typingFrom && ((typingRoom && currentRecipient === typingRoom) || (!typingRoom && currentRecipient === typingFrom)) && (
                  <span style={{ color: 'var(--text-muted)', fontSize: 13 }}>{typingFrom.replace(/^@([^:]+).*/, '$1')} печатает…</span>
                )}
                {currentRecipient && (
                  <input
                    type="text"
                    placeholder="Поиск в чате…"
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    style={{ ...styles.recipientInput, maxWidth: 180, padding: '6px 10px', fontSize: 13 }}
                  />
                )}
                {currentRoom && (
                  <>
                    <button
                      type="button"
                      onClick={() => setRoomMembersVisible((v) => !v)}
                      style={{ ...styles.vpnBtn, padding: '4px 10px', fontSize: 12 }}
                    >
                      {roomMembersVisible ? 'Скрыть участников' : 'Участники'}
                    </button>
                    <button
                      type="button"
                      onClick={handleLeaveRoom}
                      disabled={leaveLoading}
                      style={{ ...styles.vpnBtnDanger, padding: '4px 10px', fontSize: 12 }}
                    >
                      {leaveLoading ? '…' : 'Выйти'}
                    </button>
                  </>
                )}
              </div>
            ) : (
              <p style={{ margin: 0, color: 'var(--text-muted)', fontSize: 14 }}>Выберите чат в списке слева</p>
            )}
            {currentRoom && roomMembersVisible && (
              <div style={styles.roomMembersPanel}>
                <h4 style={{ margin: '0 0 8px', fontSize: 13 }}>Участники</h4>
                <ul style={{ margin: '0 0 12px', paddingLeft: 0, fontSize: 13, listStyle: 'none' }}>
                  {roomMembers.map((m) => {
                    const isMe = m.address === user.user_id
                    const canTransfer = myRole === 'creator' && m.role !== 'creator' && !isMe
                    const canKick = (myRole === 'creator' || myRole === 'admin') && !isMe && m.role !== 'creator' && (myRole === 'creator' || m.role === 'member')
                    return (
                      <li key={m.user_id} style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
                        <span>
                          {m.address} {m.role !== 'member' && <span style={{ color: 'var(--text-muted)', fontSize: 11 }}>({m.role})</span>}
                        </span>
                        {(canTransfer || canKick) && (
                          <span style={{ display: 'flex', gap: 4 }}>
                            {canTransfer && (
                              <button
                                type="button"
                                onClick={() => handleTransferCreator(m.username)}
                                disabled={!!roomActionLoading}
                                style={{ ...styles.vpnBtn, padding: '2px 6px', fontSize: 11 }}
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
                                style={{ ...styles.vpnBtnDanger, padding: '2px 6px', fontSize: 11 }}
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
                <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                  <input
                    type="text"
                    placeholder="Username для приглашения"
                    value={inviteUsername}
                    onChange={(e) => setInviteUsername(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && handleInvite()}
                    style={{ ...styles.recipientInput, flex: 1, padding: '6px 10px', fontSize: 13 }}
                  />
                  <button style={styles.vpnBtn} onClick={handleInvite} disabled={inviteLoading || !inviteUsername.trim()}>
                    {inviteLoading ? '…' : 'Пригласить'}
                  </button>
                </div>
              </div>
            )}
            {sendHint && (
              <p style={{ ...styles.inputHint, margin: '6px 0 0', color: sendHintError ? 'var(--error, #ef4444)' : 'var(--accent)' }}>
                {sendHint}
              </p>
            )}
          </div>

          <div style={styles.messages} className="messages-scroll">
            {filteredEvents.length === 0 && (
              <div style={styles.emptyState}>
                <span style={styles.emptyIcon}>💬</span>
                <p style={styles.emptyText}>
                  {recipient.trim()
                    ? isRoomAddr(normalizeRecipient(recipient, user.user_id))
                      ? 'Нет сообщений в комнате'
                      : 'Введите получателя и начните диалог'
                    : 'Выберите чат или введите получателя'}
                </p>
                <p style={styles.emptyHint}>Сообщения зашифрованы сквозным шифрованием</p>
              </div>
            )}
            {filteredEvents.length > 0 && searchFilteredEvents.length === 0 && searchQuery.trim() && (
              <div style={styles.emptyState}>
                <span style={styles.emptyIcon}>🔍</span>
                <p style={styles.emptyText}>Ничего не найдено по «{searchQuery.trim()}»</p>
              </div>
            )}
            {searchFilteredEvents.map((ev) => {
              const isOwn = ev.sender === user.user_id
              const parsed = ev.ciphertext ? parseMessage(ev.ciphertext, ev.event_id) : { type: 'text' as const, text: '[encrypted]' }
              const encrypted = ev.ciphertext ? (isE2EEPayload(ev.ciphertext) || isSignalCiphertext(ev.ciphertext)) : false
              // Наши сообщения из sync зашифрованы для получателя — не показываем [decrypt failed]
              const displayText = isOwn && parsed.type === 'text' && parsed.text === '[decrypt failed]'
                ? 'Ваше сообщение'
                : parsed.type === 'text' ? parsed.text : ''
              return (
                <div key={ev.event_id} style={{ ...styles.bubbleWrap, justifyContent: isOwn ? 'flex-end' : 'flex-start' }}>
                  <div className="msg-bubble" style={{ ...styles.bubble, ...(isOwn ? styles.bubbleOwn : styles.bubbleIn) }}>
                    {!isOwn && <span style={styles.sender}>{ev.sender}</span>}
                    {parsed.type === 'file' ? (
                      <FilePreview
                        contentUri={parsed.content_uri}
                        filename={parsed.filename}
                        size={parsed.size}
                        token={token}
                        onDownload={() => downloadFileMessage(parsed.content_uri, parsed.filename)}
                      />
                    ) : (
                      <span style={styles.msgText}>{displayText}</span>
                    )}
                    <div style={styles.bubbleMeta}>
                      {encrypted && <span style={styles.e2eeBadge} title="Шифрование E2EE">🔒</span>}
                      {isOwn && (
                        <span style={styles.deliveryBadge} title={ev.read_at ? 'Прочитано' : ev.status === 'delivered' ? 'Доставлено' : 'Отправлено'}>
                          {ev.read_at ? '✓✓' : ev.status === 'delivered' ? '✓' : '⋯'}
                        </span>
                      )}
                      <span style={styles.timestamp}>{formatTime(ev.timestamp)}</span>
                    </div>
                  </div>
                </div>
              )
            })}
            <div ref={messagesEndRef} />
          </div>

          <div style={styles.inputWrap}>
            <div style={styles.inputBar}>
              <label style={styles.fileLabel}>
              <input
                type="file"
                multiple
                style={{ display: 'none' }}
                onChange={(e) => {
                  if (e.target.files?.length) sendFiles(e.target.files)
                  e.target.value = ''
                }}
                disabled={uploading || !recipient}
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
              style={{ ...styles.messageInput, minHeight: 44, resize: 'vertical' as const }}
            />
            <button
              onClick={sendMessage}
              disabled={uploading || !recipient || sending}
              style={styles.sendBtn}
              className="btn-primary"
            >
              {sending ? '…' : 'Отправить →'}
            </button>
            </div>
            <p style={styles.inputHint}>Enter — отправить, Shift+Enter — новая строка</p>
          </div>
        </div>
        </>
        )}
      {fingerprintModal && (() => {
        const close = () => setFingerprintModal(null)
        return (
        <div
          role="dialog"
          aria-modal="true"
          aria-labelledby="fingerprint-title"
          style={{
            position: 'fixed',
            inset: 0,
            zIndex: 1000,
            background: 'rgba(0,0,0,0.6)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
          }}
          onClick={close}
        >
          <div
            style={{
              background: 'var(--surface)',
              borderRadius: 12,
              padding: 24,
              maxWidth: 400,
              boxShadow: '0 8px 32px rgba(0,0,0,0.3)',
            }}
            onClick={(e) => e.stopPropagation()}
          >
            <h3 id="fingerprint-title" style={{ margin: '0 0 12px', fontSize: 18 }}>Отпечаток ключа (Safety number)</h3>
            <p style={{ margin: '0 0 12px', fontSize: 13, color: 'var(--text-muted)' }}>
              Сравните с {fingerprintModal.recipient} лично. Если совпадает — канал защищён от MITM.
            </p>
            <pre
              style={{
                margin: 0,
                padding: 12,
                background: 'var(--bg)',
                borderRadius: 8,
                fontSize: 13,
                letterSpacing: 0.5,
                wordBreak: 'break-all',
                whiteSpace: 'pre-wrap',
              }}
            >
              {fingerprintModal.safetyNumber}
            </pre>
            <button
              type="button"
              onClick={close}
              style={{ ...styles.vpnBtn, marginTop: 16 }}
            >
              Закрыть
            </button>
          </div>
        </div>
        )
      })()}
      </main>
    </div>
  )
}

const styles: Record<string, React.CSSProperties> = {
  layout: {
    display: 'flex',
    flexDirection: 'column',
    height: '100vh',
  },
  header: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    padding: '12px 24px',
    borderBottom: '1px solid var(--border)',
    background: 'var(--surface)',
  },
  headerLeft: {
    display: 'flex',
    alignItems: 'center',
    gap: 8,
  },
  headerUser: {
    fontSize: 14,
    fontWeight: 500,
    color: 'var(--text)',
  },
  e2eeStatus: {
    fontSize: 14,
  },
  headerActions: {
    display: 'flex',
    alignItems: 'center',
    gap: 12,
  },
  tabBtn: {
    padding: '6px 14px',
    borderRadius: 8,
    border: '1px solid var(--border)',
    background: 'var(--bg)',
    color: 'var(--text)',
    fontSize: 13,
    cursor: 'pointer',
  },
  tabBtnActive: {
    background: 'var(--accent-subtle)',
    borderColor: 'var(--accent)',
    color: 'var(--accent)',
  },
  themeBtn: {
    background: 'none',
    border: '1px solid var(--border)',
    color: 'var(--text)',
    padding: '8px 12px',
    borderRadius: 8,
    cursor: 'pointer',
    fontSize: 18,
  },
  logout: {
    background: 'none',
    border: '1px solid var(--border)',
    color: 'var(--text)',
    padding: '8px 16px',
    borderRadius: 8,
    cursor: 'pointer',
    fontSize: 14,
  },
  main: {
    display: 'flex',
    flex: 1,
    overflow: 'hidden',
  },
  sidebar: {
    width: 260,
    borderRight: '1px solid var(--border)',
    padding: 20,
    background: 'var(--surface)',
    overflowY: 'auto',
  },
  sidebarSection: {
    marginBottom: 20,
  },
  sidebarTitle: {
    margin: '0 0 8px',
    fontSize: 15,
    fontWeight: 600,
  },
  hint: {
    fontSize: 12,
    color: 'var(--text-muted)',
  },
  vpnSection: {
    marginTop: 24,
    paddingTop: 16,
    borderTop: '1px solid var(--border)',
  },
  vpnTitle: { margin: '0 0 8px', fontSize: 14 },
  vpnHint: { fontSize: 11, color: 'var(--text-muted)', margin: '0 0 12px' },
  vpnProtocol: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'flex-start',
    gap: 8,
    marginBottom: 12,
  },
  vpnProtoHint: { fontSize: 11, color: 'var(--text-muted)', margin: '4px 0 0' },
  vpnBtn: {
    flexShrink: 0,
    padding: '6px 12px',
    borderRadius: 6,
    border: '1px solid var(--border)',
    background: 'var(--surface)',
    color: 'var(--text)',
    cursor: 'pointer',
    fontSize: 12,
  },
  vpnBtnDanger: {
    flexShrink: 0,
    padding: '6px 12px',
    borderRadius: 6,
    border: '1px solid var(--accent)',
    background: 'transparent',
    color: 'var(--accent)',
    cursor: 'pointer',
    fontSize: 12,
  },
  vpnAdminSection: {
    marginTop: 24,
    paddingTop: 16,
    borderTop: '1px solid var(--border)',
  },
  vpnAdminRow: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'flex-start',
    gap: 8,
    marginBottom: 12,
  },
  vpnAdminActions: { display: 'flex', gap: 4 },
  roomItem: {
    display: 'flex',
    alignItems: 'center',
    gap: 8,
    textAlign: 'left',
    padding: '8px 12px',
    borderRadius: 8,
    border: '1px solid transparent',
    background: 'transparent',
    color: 'var(--text)',
    cursor: 'pointer',
    fontSize: 13,
  },
  unreadBadge: {
    minWidth: 20,
    padding: '2px 6px',
    borderRadius: 10,
    background: 'var(--accent)',
    color: 'white',
    fontSize: 11,
    fontWeight: 600,
  },
  deliveryBadge: {
    opacity: 0.8,
    fontSize: 12,
    marginRight: 2,
  },
  roomMembersPanel: {
    marginTop: 12,
    padding: 12,
    background: 'var(--bg)',
    borderRadius: 8,
    border: '1px solid var(--border)',
  },
  chat: {
    flex: 1,
    display: 'flex',
    flexDirection: 'column',
  },
  recipientBar: {
    padding: 12,
    borderBottom: '1px solid var(--border)',
    background: 'var(--surface)',
  },
  recipientRow: { width: '100%' },
  chatWith: {
    margin: '8px 0 0',
    fontSize: 13,
    color: 'var(--accent)',
    fontWeight: 500,
  },
  recipientInput: {
    width: '100%',
    padding: 10,
    borderRadius: 8,
    border: '1px solid var(--border)',
    background: 'var(--bg)',
    color: 'var(--text)',
  },
  messages: {
    flex: 1,
    overflow: 'auto',
    padding: 16,
    display: 'flex',
    flexDirection: 'column',
    gap: 8,
    background: 'var(--bg-chat)',
  },
  bubbleWrap: {
    display: 'flex',
    width: '100%',
  },
  bubble: {
    padding: '10px 14px',
    borderRadius: 16,
    maxWidth: '75%',
    wordBreak: 'break-word',
  },
  bubbleOwn: {
    background: 'var(--bubble-own)',
    color: 'var(--bubble-own-text)',
    borderBottomRightRadius: 4,
  },
  bubbleIn: {
    background: 'var(--bubble-in)',
    border: '1px solid var(--bubble-in-border)',
    borderBottomLeftRadius: 4,
  },
  emptyState: {
    flex: 1,
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 12,
    padding: 32,
    color: 'var(--text-muted)',
  },
  emptyIcon: { fontSize: 48, opacity: 0.5 },
  emptyText: { margin: 0, fontSize: 16, fontWeight: 500, color: 'var(--text)' },
  emptyHint: { margin: 0, fontSize: 13 },
  sender: {
    display: 'block',
    fontSize: 12,
    color: 'var(--accent)',
    marginBottom: 4,
    fontWeight: 500,
  },
  msgText: { fontSize: 15, lineHeight: 1.5, display: 'block' },
  msgFile: {
    fontSize: 14,
    display: 'block',
    padding: '8px 12px',
    background: 'rgba(0,0,0,0.1)',
    borderRadius: 8,
    border: '1px dashed var(--border)',
  },
  fileLink: {
    display: 'inline-flex',
    alignItems: 'center',
    background: 'none',
    border: 'none',
    color: 'inherit',
    cursor: 'pointer',
    padding: 0,
    font: 'inherit',
    textDecoration: 'underline',
  },
  bubbleMeta: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'flex-end',
    gap: 6,
    marginTop: 4,
  },
  e2eeBadge: { fontSize: 11, opacity: 0.9 },
  timestamp: { fontSize: 11, opacity: 0.7 },
  inputWrap: {
    padding: 16,
    paddingTop: 12,
    borderTop: '1px solid var(--border)',
    background: 'var(--surface)',
  },
  inputBar: {
    display: 'flex',
    gap: 8,
    alignItems: 'center',
  },
  inputHint: {
    margin: '6px 0 0',
    fontSize: 11,
    color: 'var(--text-muted)',
  },
  fileLabel: {
    cursor: 'pointer',
    padding: 8,
    fontSize: 18,
  },
  messageInput: {
    flex: 1,
    padding: 12,
    borderRadius: 8,
    border: '1px solid var(--border)',
    background: 'var(--bg)',
    color: 'var(--text)',
  },
  sendBtn: {
    padding: '12px 24px',
    borderRadius: 8,
    background: 'var(--accent)',
    color: 'white',
    border: 'none',
    cursor: 'pointer',
    fontWeight: 600,
  },
}
