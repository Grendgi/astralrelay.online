# C2S API — Клиент ↔ Сервер

Версия протокола: **0.1**

Базовый URL: `https://{domain}/api/v1`

## Общие соглашения

- **Content-Type**: `application/json` для всех JSON-запросов
- **Аутентификация**: `Authorization: Bearer <access_token>`
- **Версия протокола**: `X-Protocol-Version: 0.1`
- **Идемпотентность**: POST с `X-Idempotency-Key: <uuid>` для повторных попыток

---

## 1. Auth / Accounts

### 1.1 Регистрация

```
POST /auth/register
```

**Request (без auth):**

```json
{
  "username": "alice",
  "device_id": "550e8400-e29b-41d4-a716-446655440000",
  "keys": {
    "identity_key": "base64_public",
    "signed_prekey": {
      "key": "base64_public",
      "signature": "base64_sig",
      "key_id": 1
    },
    "one_time_prekeys": [
      {"key": "base64_public", "key_id": 1},
      {"key": "base64_public", "key_id": 2}
    ]
  }
}
```

**Response 201:**

```json
{
  "user_id": "@alice:example.org",
  "device_id": "550e8400-e29b-41d4-a716-446655440000",
  "access_token": "eyJ...",
  "expires_in": 86400
}
```

**Ошибки:**

| Code | Причина |
|------|---------|
| 400 | Неверный формат, username занят |
| 429 | Rate limit |
| 503 | Invite-only: нужен invite code |

**Опциональные поля (конфиг инстанса):**

- `invite_code` — при включённой invite-only регистрации
- `captcha_response` — при CAPTCHA
- `pow_solution` — при proof-of-work

---

### 1.2 Логин (получение нового токена)

```
POST /auth/login
```

**Request:**

```json
{
  "user_id": "@alice:example.org",
  "device_id": "550e8400-e29b-41d4-a716-446655440000",
  "signature": "base64_ed25519_signature",
  "timestamp": 1707811200
}
```

Подпись: `Ed25519(timestamp || device_id)` приватным identity key.

**Response 200:**

```json
{
  "access_token": "eyJ...",
  "expires_in": 86400
}
```

---

### 1.3 Обновление ключей устройства

```
PUT /auth/keys
Authorization: Bearer <token>
```

**Request:**

```json
{
  "signed_prekey": {
    "key": "base64_public",
    "signature": "base64_sig",
    "key_id": 2
  },
  "one_time_prekeys": [
    {"key": "base64_public", "key_id": 101}
  ]
}
```

Используется для ротации signed prekey и пополнения one-time prekeys.

**Response 200:** `{}`

---

## 2. Key Directory

### 2.1 Запрос prekey bundle

**По user_id (MVP 1:1 — возвращает bundle первого устройства):**

```
GET /keys/bundle/{user_id}
Authorization: Bearer <token>
```

**По user_id и device_id:**

```
GET /keys/bundle/{user_id}/{device_id}
Authorization: Bearer <token>
```

Или для локального пользователя (без auth при запросе своего домена):

```
GET /keys/bundle/@bob:example.org/550e8400-...
```

**Response 200:**

```json
{
  "identity_key": "base64_public",
  "signed_prekey": {
    "key": "base64_public",
    "signature": "base64_sig",
    "key_id": 1
  },
  "one_time_prekey": {
    "key": "base64_public",
    "key_id": 42
  }
}
```

`one_time_prekey` может отсутствовать, если закончились.

**Response 404:** пользователь или устройство не найдены.

**Для удалённого пользователя** (`@bob:other.org`): клиент обращается к своему серверу, сервер запрашивает bundle через S2S и возвращает клиенту. Endpoint тот же.

---

## 3. Message Relay

### 3.1 Отправка сообщения

```
POST /messages/send
Authorization: Bearer <token>
X-Idempotency-Key: <uuid>  (опционально)
```

**Request:**

См. [01-message-formats](./../protocol/01-message-formats.md) — envelope.

```json
{
  "type": "m.room.encrypted",
  "sender": "@alice:example.org",
  "recipient": "@bob:other.org",
  "device_id": "550e8400-...",
  "timestamp": 1707811200,
  "content": {
    "ciphertext": "base64...",
    "session_id": "sess_..."
  }
}
```

**Response 202:**

```json
{
  "event_id": "evt_abc123",
  "status": "queued"
}
```

**Ошибки:**

| Code | Причина |
|------|---------|
| 400 | Неверный формат |
| 404 | Recipient не найден |
| 429 | Rate limit |

---

### 3.2 Sync (получение сообщений)

**Long-polling:**

```
GET /messages/sync?since={cursor}&timeout=30000
Authorization: Bearer <token>
```

- `since` — cursor последнего полученного события (пусто при первом запросе)
- `timeout` — макс. время ожидания в мс (30 сек)

**Response 200:**

```json
{
  "events": [
    {
      "event_id": "evt_...",
      "type": "m.room.encrypted",
      "sender": "@bob:other.org",
      "recipient": "@alice:example.org",
      "timestamp": 1707811205,
      "content": {
        "ciphertext": "base64...",
        "session_id": "sess_..."
      }
    }
  ],
  "next_cursor": "cursor_xyz"
}
```

Клиент сохраняет `next_cursor` и передаёт в `since` при следующем запросе.

**WebSocket (альтернатива):**

```
WS /messages/stream
Authorization: Bearer <token>
```

При подключении сервер отправляет события в реальном времени. Формат события — как элемент `events` выше. Клиент может комбинировать: WebSocket для real-time + периодический sync для восстановления.

---

### 3.3 Подтверждение доставки (опционально)

```
POST /messages/ack
Authorization: Bearer <token>
```

```json
{
  "event_ids": ["evt_1", "evt_2"]
}
```

Сервер помечает события как delivered. Не обязательно для MVP.

---

## 4. Media

### 4.1 Загрузка файла

```
POST /media/upload
Authorization: Bearer <token>
Content-Type: application/octet-stream
X-Content-Length: 1024000
```

**Request body:** raw bytes зашифрованного blob'а.

**Response 201:**

```json
{
  "content_uri": "blob:sha256:a1b2c3d4e5f6..."
}
```

Content-URI = хэш зашифрованного содержимого (content-addressable).

**Ошибки:**

| Code | Причина |
|------|---------|
| 413 | Превышен лимит размера файла |
| 429 | Rate limit |

---

### 4.2 Скачивание файла

```
GET /media/{content_uri}
Authorization: Bearer <token>
```

Пример: `GET /media/blob:sha256:a1b2c3d4...`

**Response 200:**

- `Content-Type: application/octet-stream`
- Body: зашифрованный blob

**Response 404:** файл не найден.

**Примечание:** для файлов на другом инстансе сервер может проксировать запрос через S2S (см. S2S API).

---

## 5. Health / Discovery

### 5.1 Health check

```
GET /health
```

**Response 200:** `{"status": "ok"}`

---

### 5.2 Well-known (серверные ключи, версия протокола)

```
GET /.well-known/protocol
```

**Response 200:**

```json
{
  "version": "0.1",
  "server_key": "base64_ed25519_public",
  "capabilities": {
    "media": true,
    "federation": true
  }
}
```

---

## 6. Rate Limits (рекомендуемые)

| Endpoint | Лимит |
|----------|-------|
| POST /auth/register | 5 / min / IP |
| POST /auth/login | 10 / min / IP |
| POST /messages/send | 60 / min / user |
| GET /messages/sync | 120 / min / user |
| POST /media/upload | 20 / min / user |
| GET /keys/bundle | 100 / min / user |

Значения настраиваются админом инстанса.
