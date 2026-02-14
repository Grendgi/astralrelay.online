# S2S API — Федерация (сервер ↔ сервер)

Версия протокола: **0.1**

## 1. Обзор

- Каждый homeserver общается с другими по HTTPS
- Аутентификация: подпись запросов **Server Signing Key** (Ed25519)
- Discovery: `GET https://{domain}/.well-known/federation` → URL federation endpoint

---

## 2. Discovery

### 2.1 Federation endpoint

```
GET https://example.org/.well-known/federation
```

**Response 200:**

```json
{
  "federation_endpoint": "https://example.org/federation",
  "server_key": "base64_ed25519_public",
  "version": "0.1"
}
```

---

## 3. Аутентификация S2S

Каждый исходящий запрос подписывается:

**Заголовки:**

```
X-Server-Origin: example.org
X-Server-Destination: other.org
X-Server-Timestamp: 1707811200
X-Server-Signature: base64_ed25519_signature
```

**Signed payload:**

```
{origin}\n{destination}\n{timestamp}\n{method}\n{path}\n{body_hash}
```

- `body_hash` = SHA256(body), hex-encoded; для GET — пустая строка
- Подпись: Ed25519(signed_payload) приватным ключом origin-сервера
- Получатель проверяет подпись по публичному ключу из `/.well-known/federation`

**Временное окно:** timestamp в пределах ±5 минут.

---

## 4. Endpoints

Базовый путь: `https://{domain}/federation/v1`

### 4.1 Запрос prekey bundle

Сервер A запрашивает у сервера B ключи пользователя `@bob:b.org`.

```
GET /federation/v1/keys/bundle/@bob:b.org/{device_id}
X-Server-Origin: a.org
X-Server-Destination: b.org
...
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

**Response 404:** пользователь/устройство не найдены.

**Потребление one-time prekey:** при наличии `one_time_prekey` в ответе, сервер B помечает его consumed. Это делается при успешной доставке ответа (идемпотентность по device_id+key_id при повторных запросах — возвращать тот же bundle до фактического consume при отправке транзакции, или сразу consume при выдаче — на выбор реализации; важно не отдавать один и тот же one-time prekey двум разным инициаторам).

---

### 4.2 Доставка транзакции (push)

Сервер A отправляет события на сервер B (получатели — пользователи B).

```
POST /federation/v1/transaction
X-Server-Origin: a.org
X-Server-Destination: b.org
Content-Type: application/json
```

**Request body:**

```json
{
  "transaction_id": "txn_uuid_...",
  "origin": "a.org",
  "destination": "b.org",
  "events": [
    {
      "event_id": "evt_...",
      "type": "m.room.encrypted",
      "sender": "@alice:a.org",
      "recipient": "@bob:b.org",
      "timestamp": 1707811200,
      "content": {
        "ciphertext": "base64...",
        "session_id": "sess_..."
      }
    }
  ]
}
```

**Response 200:**

```json
{
  "accepted": ["evt_1", "evt_2"],
  "rejected": []
}
```

Или при частичном отказе:

```json
{
  "accepted": ["evt_1"],
  "rejected": [
    {
      "event_id": "evt_2",
      "reason": "recipient_not_found"
    }
  ]
}
```

**Ошибки:**

| Code | Причина |
|------|---------|
| 400 | Неверный формат |
| 401 | Неверная подпись |
| 403 | Домен в blocklist |
| 429 | Rate limit |

**Идемпотентность:** получатель проверяет `transaction_id` + `event_id`; дубликаты игнорирует.

---

### 4.3 Запрос media (проксирование)

Сервер A запрашивает у сервера B blob для `content_uri`, принадлежащий B.

```
GET /federation/v1/media/blob:sha256:a1b2c3d4...
X-Server-Origin: a.org
X-Server-Destination: b.org
```

**Response 200:**

- `Content-Type: application/octet-stream`
- Body: зашифрованный blob

**Response 404:** blob не найден.

---

### 4.4 Backfill / запрос истории (опционально, post-MVP)

Для восстановления после офлайна можно добавить:

```
GET /federation/v1/events?user=@bob:b.org&since=cursor&limit=100
```

Возвращает события, адресованные `@bob:b.org`, начиная с cursor. В MVP можно не реализовывать — достаточно sync через C2S.

---

## 5. Маршрутизация транзакций

1. Клиент отправляет сообщение на свой homeserver (C2S)
2. Homeserver определяет домен получателя (`recipient` → domain)
3. Если домен = свой — кладёт в локальную очередь
4. Если домен другой — формирует транзакцию и отправляет `POST /federation/v1/transaction` на destination
5. Получатель (другой homeserver) кладёт события в очередь своих пользователей
6. Клиент получателя забирает через sync

**Несколько получателей в одной транзакции:** разрешено. Группировать по домену — одна транзакция на домен.

---

## 6. Trust / Blocklist

- По умолчанию: любой домен может отправлять транзакции
- Админ может настроить:
  - **Allowlist** — только перечисленные домены
  - **Blocklist** — запрещённые домены
- Проверка выполняется при приёме `POST /federation/v1/transaction`

---

## 7. Retry и надёжность

- При временной ошибке (5xx, timeout) — повтор с exponential backoff
- Максимум попыток: 5
- Мёртвые буквы: после исчерпания попыток — сохранить в DLQ для ручного разбора (опционально)
- `transaction_id` — для дедупликации при повторах
