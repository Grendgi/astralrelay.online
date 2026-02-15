# E2EE: Signal Protocol и fallback MVP

## Режимы шифрования

| Режим | Описание | Где используется |
|-------|----------|------------------|
| **Signal Protocol** | X3DH + Double Ratchet (libsignal). Forward secrecy, persistent сессии. | Личные DM 1:1, если пакет `@privacyresearch/libsignal-protocol-typescript` доступен |
| **MVP** | X25519 DH + NaCl secretbox. Упрощённая схема без Double Ratchet. | Fallback при отсутствии libsignal или ошибке Signal; комнаты (per-member encryption — MVP или Signal) |
| **Федерация** | Сервер передаёт ciphertext как есть. Шифрование — на клиенте (Signal или MVP). | Межсерверная доставка сообщений |

## Как проверить Signal режим

1. Откройте DevTools → Application → IndexedDB → `signal-keystore`
2. При отправке Signal-сообщения в `kv` появляются записи: `identityKey`, `registrationId`, `25519KeysignedKey*`, `session*`
3. Сообщения с префиксом `sig1:` — шифрованы через Signal. Без префикса — MVP base64

## Что хранится в IndexedDB

- **`identityKey`** — пара ключей (public/private) для identity
- **`registrationId`** — уникальный ID устройства (uint16), генерируется один раз
- **`25519KeysignedKey{id}`** — signed prekey (по key_id)
- **`25519KeypreKey{id}`** — one-time prekeys (если использовались)
- **`session{addr}`** — сессии Double Ratchet с каждым собеседником

Данные сохраняются между перезагрузками страницы. Очистка IndexedDB → потребуется повторный X3DH handshake с собеседниками.

## Связанные документы

- [LIBSIGNAL-INTEGRATION.md](./LIBSIGNAL-INTEGRATION.md) — план и текущее состояние интеграции
- [E2EE-THREAT-MODEL.md](./E2EE-THREAT-MODEL.md) — угрозы и MITM
- [protocol/02-key-model.md](./protocol/02-key-model.md) — модель ключей
