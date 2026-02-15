const API_BASE = '/api/v1'

export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public code?: string
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

async function request<T>(
  path: string,
  options: RequestInit & { token?: string; idempotencyKey?: string } = {}
): Promise<T> {
  const { token, idempotencyKey, ...init } = options
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    'X-Protocol-Version': '0.1',
    ...(init.headers as Record<string, string>),
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }
  if (idempotencyKey) {
    headers['X-Idempotency-Key'] = idempotencyKey
  }

  const res = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers,
  })

  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    throw new ApiError(
      data.message || res.statusText,
      res.status,
      data.error
    )
  }
  return data as T
}

export const api = {
  register: (body: RegisterRequest) =>
    request<RegisterResponse>('/auth/register', {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  login: (body: LoginRequest) =>
    request<LoginResponse>('/auth/login', {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  updateKeys: (body: { identity_signing_key?: string; signed_prekey?: { key: string; signature: string; key_id: number }; one_time_prekeys?: Array<{ key: string; key_id: number }> }, token: string) =>
    request<Record<string, never>>('/auth/keys', {
      method: 'PUT',
      body: JSON.stringify(body),
      token,
    }),

  getKeys: (userID: string, token: string, deviceID?: string) =>
    request<PrekeyBundle & { device_id?: string; signal_device_id?: number }>(
      deviceID
        ? `/keys/bundle/${encodeURIComponent(userID)}/${encodeURIComponent(deviceID)}`
        : `/keys/bundle/${encodeURIComponent(userID)}`,
      { token }
    ),

  sendMessage: (body: SendMessageRequest, token: string, idempotencyKey?: string) =>
    request<SendMessageResponse>('/messages/send', {
      method: 'POST',
      body: JSON.stringify(body),
      token,
      idempotencyKey,
    }),

  sync: async (since: string, token: string) => {
    for (let i = 0; i < 3; i++) {
      try {
        return await request<SyncResponse>(`/messages/sync?since=${encodeURIComponent(since)}`, { token })
      } catch (e) {
        const ok = e instanceof ApiError && [404, 502, 503].includes(e.status) && i < 2
        if (!ok) throw e
        await new Promise(r => setTimeout(r, 1000 * (i + 1)))
      }
    }
    throw new ApiError('Sync failed after retries', 503)
  },

  uploadFile: (file: File, token: string) => {
    const headers: Record<string, string> = {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/octet-stream',
      'X-Protocol-Version': '0.1',
    }
    return fetch(`${API_BASE}/media/upload`, {
      method: 'POST',
      headers,
      body: file,
    }).then(async (res) => {
      const data = await res.json().catch(() => ({}))
      if (!res.ok) throw new ApiError(data.message || res.statusText, res.status, data.error)
      return data as { content_uri: string }
    })
  },

  downloadFile: (contentUri: string, token: string) =>
    fetch(`${API_BASE}/media/${encodeURIComponent(contentUri)}`, {
      headers: { 'Authorization': `Bearer ${token}` },
      credentials: 'include',
    }),

  vpnNodes: (token: string) =>
    request<{ nodes: Array<{ id: string; name: string; region: string; wireguard_endpoint: string; wireguard_server_pubkey: string; openvpn_endpoint: string; is_default: boolean; ping_url?: string }> }>('/vpn/nodes', { token }),

  vpnMyConfigs: (token: string) =>
    request<{ configs: Array<{ device_id: string; protocol: string; node_name?: string; created_at: string; expires_at?: string; is_expired: boolean; traffic_rx_bytes?: number; traffic_tx_bytes?: number }> }>('/vpn/my-configs', { token }),

  vpnRevoke: (params: { protocol: string; device_id?: string }, token: string) =>
    request<{ status: string }>(
      `/vpn/revoke?protocol=${encodeURIComponent(params.protocol)}${params.device_id ? `&device_id=${encodeURIComponent(params.device_id)}` : ''}`,
      { method: 'POST', token }
    ),

  vpnProtocols: (token: string) =>
    request<{ protocols: Array<{ id: string; name: string; hint: string }> }>('/vpn/protocols', { token }),

  roomsList: (token: string) =>
    request<{ rooms: Array<{ id: string; name: string; domain: string; address: string }> }>('/rooms', { token }),

  roomsCreate: (name: string, token: string) =>
    request<{ id: string; name: string; domain: string; address: string; creator_id: number }>('/rooms', {
      method: 'POST',
      body: JSON.stringify({ name }),
      token,
    }),

  roomsGet: (roomID: string, token: string) =>
    request<{ id: string; name: string; domain: string; address: string }>(`/rooms/${encodeURIComponent(roomID)}`, { token }),

  roomsInvite: (roomID: string, params: { user_id?: number; username?: string }, token: string) =>
    request<{ status: string }>(`/rooms/${encodeURIComponent(roomID)}/invite`, {
      method: 'POST',
      body: JSON.stringify(params),
      token,
    }),

  roomsLeave: (roomID: string, token: string) =>
    request<{ status: string }>(`/rooms/${encodeURIComponent(roomID)}/leave`, {
      method: 'POST',
      token,
    }),

  roomsMembers: (roomID: string, token: string) =>
    request<{ members: Array<{ user_id: number; username: string; domain: string; address: string; role: string; joined_at: string }> }>(
      `/rooms/${encodeURIComponent(roomID)}/members`,
      { token }
    ),

  roomsTransferCreator: (roomID: string, params: { user_id?: number; username?: string }, token: string) =>
    request<{ status: string }>(`/rooms/${encodeURIComponent(roomID)}/transfer`, {
      method: 'POST',
      body: JSON.stringify(params),
      token,
    }),

  roomsRemoveMember: (roomID: string, params: { user_id?: number; username?: string }, token: string) =>
    request<{ status: string }>(`/rooms/${encodeURIComponent(roomID)}/remove`, {
      method: 'POST',
      body: JSON.stringify(params),
      token,
    }),

  vpnConfig: async (protocol: string, token: string, format?: 'file' | 'json', nodeId?: string) => {
    let url = `${API_BASE}/vpn/config/${encodeURIComponent(protocol)}`
    const params = new URLSearchParams()
    if (format === 'json') params.set('format', 'json')
    if (nodeId) params.set('node_id', nodeId)
    if (params.toString()) url += '?' + params.toString()
    const res = await fetch(url, { headers: { Authorization: `Bearer ${token}` } })
    if (!res.ok) {
      const data = await res.json().catch(() => ({}))
      throw new ApiError(data.message || res.statusText, res.status, data.error)
    }
    if (format === 'json') {
      return res.json() as Promise<{ config: string; filename: string }>
    }
    const blob = await res.blob()
    const disposition = res.headers.get('Content-Disposition')
    const filename = disposition?.match(/filename=(.+)/)?.[1]?.replace(/"/g, '') || `${protocol}.conf`
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = filename
    a.click()
    URL.revokeObjectURL(a.href)
    return undefined
  },

  backupPrepare: (token: string) =>
    request<{ salt: string }>('/backup/prepare', { method: 'POST', token }),

  backupGetSalt: (token: string) =>
    request<{ salt: string }>('/backup/salt', { token }),

  keysSync: (token: string, keysBackup: { salt: string; blob: string }) =>
    request<{ status: string }>('/keys/sync', {
      method: 'POST',
      body: JSON.stringify(keysBackup),
      token,
    }),

  getVapidPublic: (token: string) =>
    request<{ public_key: string }>('/push/vapid-public', { token }),
  pushSubscribe: (subscription: PushSubscriptionJSON, token: string) =>
    request<{ status: string }>('/push/subscribe', {
      method: 'POST',
      body: JSON.stringify({
        endpoint: subscription.endpoint,
        keys: subscription.keys,
      }),
      token,
    }),
  pushUnsubscribe: (endpoint: string, token: string) =>
    request<{ status: string }>('/push/unsubscribe', {
      method: 'POST',
      body: JSON.stringify({ endpoint }),
      token,
    }),
  sendReadReceipts: (eventIds: string[], token: string) =>
    request<{ status: string }>('/messages/read', {
      method: 'POST',
      body: JSON.stringify({ event_ids: eventIds }),
      token,
    }),
  sendTyping: (recipient: string, typing: boolean, token: string) =>
    request<{ status: string }>('/messages/typing', {
      method: 'POST',
      body: JSON.stringify({ recipient, typing }),
      token,
    }),

  streamWebSocket: (
    recipient: string,
    token: string,
    onMessage: (data?: { type?: string; sender?: string; typing?: boolean; event_id?: string }) => void,
    onConnectionChange?: (connected: boolean) => void,
  ) => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const url = `${protocol}//${window.location.host}${API_BASE}/messages/stream?as=${encodeURIComponent(recipient)}&access_token=${encodeURIComponent(token)}`
    let ws: WebSocket
    let retries = 0
    const maxRetries = 5
    let stopped = false
    const connect = () => {
      if (stopped) return
      ws = new WebSocket(url)
      ws.onopen = () => {
        retries = 0
        onConnectionChange?.(true)
      }
      ws.onmessage = (e) => {
        try {
          const d = JSON.parse(e.data as string) as { type?: string; sender?: string; typing?: boolean; event_id?: string }
          onMessage(d)
        } catch {
          onMessage()
        }
      }
      ws.onclose = () => {
        onConnectionChange?.(false)
        if (!stopped && retries < maxRetries) setTimeout(connect, 2000 * ++retries)
      }
    }
    connect()
    return () => { stopped = true; ws?.close() }
  },
}

export interface RegisterRequest {
  username: string
  password: string
  device_id: string
  keys: {
    identity_key: string
    signed_prekey: {
      key: string
      signature: string
      key_id: number
    }
    one_time_prekeys: Array<{ key: string; key_id: number }>
  }
  keys_backup?: { salt: string; blob: string }
}

export interface LoginKeys {
  identity_key: string
  signed_prekey: { key: string; signature: string; key_id: number }
  one_time_prekeys: Array<{ key: string; key_id: number }>
}

export interface RegisterResponse {
  user_id: string
  device_id: string
  access_token: string
  expires_in: number
}

export interface LoginRequest {
  username: string
  password: string
  device_id: string
  request_keys_restore?: boolean
  keys?: LoginKeys
}

export interface LoginResponse {
  user_id: string
  device_id: string
  access_token: string
  expires_in: number
  keys_backup?: { salt: string; blob: string }
}

export interface PrekeyBundle {
  identity_key: string
  signed_prekey: { key: string; signature: string; key_id: number }
  one_time_prekey?: { key: string; key_id: number }
}

export interface SendMessageRequest {
  type: string
  sender: string
  recipient: string
  device_id: string
  timestamp: number
  content: { ciphertext?: string; ciphertexts?: Record<string, string>; session_id: string }
}

export interface SendMessageResponse {
  event_id: string
  status: string
}

export interface SyncEvent {
  event_id: string
  type: string
  sender: string
  recipient: string
  sender_device?: string // UUID for Signal multi-device
  timestamp: number
  ciphertext: string
  session_id: string
  status?: string // queued | delivered
  read_at?: string // ISO timestamp when recipient read
}

export interface SyncResponse {
  events: SyncEvent[]
  next_cursor: string
}
