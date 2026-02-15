/**
 * Sender Keys for group/room E2EE.
 * One sender key per (roomId, senderAddr, senderDeviceId). Distribute via Signal DM.
 * Format: sk1: = prefix + base64(nonce + secretbox(plaintext, nonce, senderKey))
 */
import * as nacl from 'tweetnacl'
import { decodeUTF8, encodeUTF8 } from 'tweetnacl-util'

const DB_NAME = 'signal-keystore'
const DB_VERSION = 1
const STORE_NAME = 'kv'
const SENDER_KEY_PREFIX = 'sk:'

interface StoredSenderKey {
  key: string // base64
  keyId: string
  createdAt: number
  memberAddrs?: string[] // sorted, for rekey detection when group changes
}

function openDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION)
    req.onerror = () => reject(req.error)
    req.onsuccess = () => resolve(req.result)
  })
}

function storageKey(roomId: string, senderAddr: string, senderDeviceId: string): string {
  return `${SENDER_KEY_PREFIX}${roomId}:${senderAddr}:${senderDeviceId}`
}

/** Generate random 32-byte sender key. */
export function generateSenderKey(): Uint8Array {
  return nacl.randomBytes(nacl.secretbox.keyLength)
}

/** Store sender key with member set (for rekey detection). */
export async function storeSenderKey(
  roomId: string,
  senderAddr: string,
  senderDeviceId: string,
  key: Uint8Array,
  keyId: string,
  memberAddrs?: string[]
): Promise<void> {
  const db = await openDB()
  const keyB64 = btoa(String.fromCharCode(...key))
  const entry: StoredSenderKey = { key: keyB64, keyId, createdAt: Date.now(), memberAddrs }
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readwrite')
    const store = tx.objectStore(STORE_NAME)
    store.put(entry, storageKey(roomId, senderAddr, senderDeviceId))
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
}

/** Load sender key by (roomId, senderAddr, senderDeviceId). */
export async function getSenderKey(
  roomId: string,
  senderAddr: string,
  senderDeviceId: string
): Promise<{ key: Uint8Array; keyId: string; memberAddrs?: string[] } | null> {
  const db = await openDB()
  const entry = await new Promise<StoredSenderKey | undefined>((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readonly')
    const store = tx.objectStore(STORE_NAME)
    const req = store.get(storageKey(roomId, senderAddr, senderDeviceId))
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
  if (!entry?.key) return null
  const bin = atob(entry.key)
  const arr = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) arr[i] = bin.charCodeAt(i)
  return { key: arr, keyId: entry.keyId, memberAddrs: entry.memberAddrs }
}

/** Delete our sender key for room (forces rekey on next send). */
export async function deleteSenderKey(
  roomId: string,
  senderAddr: string,
  senderDeviceId: string
): Promise<void> {
  const db = await openDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readwrite')
    const store = tx.objectStore(STORE_NAME)
    store.delete(storageKey(roomId, senderAddr, senderDeviceId))
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
}

/** Compare two member sets (sorted). True if identical. */
export function memberSetEquals(a: string[] | undefined, b: string[]): boolean {
  if (!a || a.length !== b.length) return false
  const sa = [...a].sort()
  const sb = [...b].sort()
  return sa.every((v, i) => v === sb[i])
}

/** Encrypt plaintext with sender key. Returns "sk1:" + base64(nonce + ciphertext). */
export function encryptWithSenderKey(plaintext: string, key: Uint8Array): string {
  const nonce = nacl.randomBytes(nacl.secretbox.nonceLength)
  const msg = decodeUTF8(plaintext)
  const box = nacl.secretbox(msg, nonce, key)
  const combined = new Uint8Array(nonce.length + box.length)
  combined.set(nonce, 0)
  combined.set(box, nonce.length)
  const b64 = btoa(String.fromCharCode(...combined))
  return `sk1:${b64}`
}

/** Decrypt sk1 payload. Expects "sk1:" + base64(nonce + ciphertext). */
export function decryptWithSenderKey(sk1Payload: string, key: Uint8Array): string {
  if (!sk1Payload.startsWith('sk1:')) throw new Error('Invalid sk1 format')
  const b64 = sk1Payload.slice(4)
  const bin = atob(b64)
  const arr = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) arr[i] = bin.charCodeAt(i)
  const nonce = arr.slice(0, nacl.secretbox.nonceLength)
  const box = arr.slice(nacl.secretbox.nonceLength)
  const plain = nacl.secretbox.open(box, nonce, key)
  if (!plain) throw new Error('sk1 decryption failed')
  return encodeUTF8(plain)
}

/** Distribution payload: encrypted via Signal DM, contains sender key + optional first message body. */
export interface SenderKeyDistributionPayload {
  type: 'sk-dist'
  roomId: string
  senderId: string
  senderDeviceId: string
  keyId: string
  key: string // base64
  body?: string // first message JSON
}

export function buildDistributionPayload(
  roomId: string,
  senderId: string,
  senderDeviceId: string,
  keyId: string,
  key: Uint8Array,
  body?: string
): string {
  const keyB64 = btoa(String.fromCharCode(...key))
  const payload: SenderKeyDistributionPayload = {
    type: 'sk-dist',
    roomId,
    senderId,
    senderDeviceId,
    keyId,
    key: keyB64,
  }
  if (body != null) payload.body = body
  return JSON.stringify(payload)
}

export function parseDistributionPayload(plaintext: string): SenderKeyDistributionPayload | null {
  try {
    const parsed = JSON.parse(plaintext) as SenderKeyDistributionPayload
    if (parsed?.type === 'sk-dist' && parsed.roomId && parsed.senderId && parsed.senderDeviceId && parsed.keyId && parsed.key) {
      return parsed
    }
  } catch {
    /* not a distribution */
  }
  return null
}
