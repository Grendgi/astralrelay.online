# Форматы сообщений и событий

Версия протокола: **0.1**

## 1. Envelope (обёртка сообщения)

Сервер видит только envelope. Содержимое — шифртекст, расшифровывается на клиенте.

### 1.1 C2S — отправка (клиент → сервер)

```json
{
  "type": "m.room.encrypted",
  "sender": "@alice:example.org",
  "recipient": "@bob:other.org",
  "device_id": "550e8400-e29b-41d4-a716-446655440000",
  "timestamp": 1707811200,
  "content": {
    "ciphertext": "base64_encoded_ciphertext",
    "session_id": "opaque_session_identifier"
  }
}
```

| Поле | Тип | Обязательно | Описание |
|------|-----|-------------|----------|
| `type` | string | да | Всегда `m.room.encrypted` для E2EE |
| `sender` | string | да | User ID отправителя |
| `recipient` | string | да | User ID получателя |
| `device_id` | string | да | Device ID отправителя |
| `timestamp` | int64 | да | Unix timestamp (client-side) |
| `content.ciphertext` | string | да | Шифртекст (Double Ratchet output) |
| `content.session_id` | string | да | Идентификатор сессии для маршрутизации |

### 1.2 Внутреннее содержимое ciphertext (на клиенте)

После расшифровки клиент получает **plaintext payload**:

```json
{
  "message_type": "text",
  "body": "Привет!",
  "timestamp": 1707811200
}
```

или для файла:

```json
{
  "message_type": "file",
  "content_uri": "blob:sha256:a1b2c3d4...",
  "file_key": "base64_encoded_key",
  "nonce": "base64_nonce",
  "hash": "sha256_hash_of_plaintext",
  "size": 1024000,
  "filename": "document.pdf"
}
```

| message_type | Описание |
|--------------|----------|
| `text` | Текстовое сообщение |
| `file` | Ссылка на зашифрованный файл + ключ |

### 1.3 Sync / доставка (сервер → клиент)

При получении сообщения клиент получает:

```json
{
  "event_id": "evt_abc123...",
  "type": "m.room.encrypted",
  "sender": "@bob:other.org",
  "recipient": "@alice:example.org",
  "timestamp": 1707811205,
  "content": {
    "ciphertext": "base64_encoded_ciphertext",
    "session_id": "opaque_session_identifier"
  }
}
```

`event_id` — уникальный идентификатор на сервере (для idempotency, deduplication).

---

## 2. Типы событий

### 2.1 Сообщения (внутри E2EE)

| `message_type` | Поля | Описание |
|----------------|------|----------|
| `text` | `body`, `timestamp` | Текстовое сообщение |
| `file` | `content_uri`, `file_key`, `nonce`, `hash`, `size`, `filename` | Файл |

### 2.2 Системные (C2S, не E2EE)

| `type` | Назначение |
|--------|------------|
| `m.key.upload` | Публикация identity key + prekeys |
| `m.key.request` | Запрос prekey bundle |
| `m.key.bundle` | Ответ с prekey bundle |

---

## 3. Content URI для файлов

Формат: `blob:sha256:{hex_digest}`

- `sha256` — хэш **зашифрованного** blob'а (content-addressable)
- Сервер хранит blob по хэшу; дубликаты не создают копии
- Клиент передаёт URI и ключ только внутри E2EE-сообщения

Пример: `blob:sha256:a1b2c3d4e5f6...`

---

## 4. Формат ciphertext (Double Ratchet)

Структура пакета на уровне протокола (реализационные детали в [02-key-model](./02-key-model.md)):

```
[header: 1+ bytes][ciphertext: variable]
```

Header содержит минимально необходимое для ratchet (например, DH public key, message number). Конкретный wire format — в спецификации крипто-библиотеки (Signal protocol / libsignal).

---

## 5. Размеры и лимиты (рекомендуемые)

| Параметр | Значение | Примечание |
|----------|----------|------------|
| Max message body (text) | 64 KB | Без файлов |
| Max file size | 100 MB | Настраиваемо админом |
| Max filename length | 255 bytes | UTF-8 |
| TTL в очереди | 7 дней | По умолчанию |
| Batch sync | до 100 событий | За один запрос |

---

## 6. Версионирование протокола

В заголовках запросов:

```
X-Protocol-Version: 0.1
```

Сервер может отклонять неподдерживаемые версии (HTTP 426 Upgrade Required).
