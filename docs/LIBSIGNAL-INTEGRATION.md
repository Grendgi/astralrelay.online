# Интеграция libsignal (Signal Protocol)

Текущий MVP E2EE (`web/src/crypto/e2ee.ts`) использует X25519 DH + NaCl secretbox — упрощённая схема без полного X3DH и Double Ratchet.

## Цель

Переход на полноценный Signal Protocol для:
- Forward secrecy (Double Ratchet)
- Лучшая защита при компрометации ключей
- Совместимость с экосистемой Signal

## Ограничения

- **@signalapp/libsignal-client** — Node.js, node-gyp. В браузере не работает.
- **Рекомендуемый пакет для браузера:** `@privacyresearch/libsignal-protocol-typescript` — чистый TypeScript, работает в браузере и Node. Реализует X3DH + Double Ratchet.

## План интеграции

1. ✓ **Установлен** `@privacyresearch/libsignal-protocol-typescript`. Интеграция в `web/src/crypto/signal.ts`, `signal-store.ts`. При отправке — попытка Signal, при ошибке — fallback на MVP. При получении — асинхронная расшифровка sig1: сообщений.

2. ✓ **Persistent store (IndexedDB)** — `IndexedDBSignalStore` в `signal-store.ts`. Сессии Signal сохраняются между перезагрузками страницы, Double Ratchet работает корректно.

3. ✓ **Safety number (отпечаток ключа)** — `web/src/crypto/fingerprint.ts`. Кнопка «Отпечаток» в DM позволяет сравнить fingerprint с собеседником для проверки на MITM.

4. **Слой совместимости** — создать `web/src/crypto/signal.ts`:
   - `convertBundle(our: PrekeyBundle): SignalPreKeyBundle`
   - `encryptWithSignal(plaintext, bundle, sessionStore): ciphertext`
   - `decryptWithSignal(ciphertext, ourKeys, sessionStore): plaintext`
   - Session store: IndexedDB (по умолчанию для max security)

3. **Миграция** — по флагу или версии протокола:
   - Новые сессии — Signal
   - Старые — текущий MVP (fallback)
   - Сообщения между разными версиями — либо MVP, либо предупреждение

4. **Сервер** — API `/keys/bundle` уже отдаёт `identity_key`, `signed_prekey`, `one_time_prekey`. Проверить соответствие формату libsignal (public key bytes, key_id).

5. **Тесты** — шифрование/расшифровка между двумя клиентами, session persistence.

## Текущий формат ключей

- `identity_key`, `signed_prekey.key`, `one_time_prekey.key` — base64 (32 bytes X25519 public key)
- `signed_prekey.signature` — base64 (64 bytes Ed25519)
- `signed_prekey.key_id` — number

## Референсы

- [@privacyresearch/libsignal-protocol-typescript](https://github.com/privacyresearchgroup/libsignal-protocol-typescript) — браузер + Node
- [Signal Protocol (X3DH, Double Ratchet)](https://signal.org/docs/)
- [SECURITY-HARDENING.md](./SECURITY-HARDENING.md) — общий план усиления защиты
