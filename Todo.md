# Todo и Roadmap

Федеративный мессенджер + VPN (AstralRelay.online). Улучшения по приоритету.

---

## Что стало лучше (и это прям важно)

- **Web DX собран в единое целое:** в web/package.json — eslint + плагины + prettier; scripts/fmt.sh / scripts/lint.sh реально запускают web-форматирование и линт.
- **WebSocket больше не тащит токен в URL:** клиент передаёт ws_token через **Sec-WebSocket-Protocol** (`['bearer.' + ws_token]`), URL чистый (только `?as=...`). Сильный шаг по снижению утечек через логи/рефереры.
- **E2EE Strict/Compatibility:** в strict-режиме нет "тихого даунгрейда" — при проблемах с Signal отправка не уходит в fallback незаметно.
- **Реакция на смену ключей + UI:** предупреждение "ключ собеседника изменился", блокировка отправки в strict до подтверждения.
- **Multi-device:** управление устройствами в UI, endpoints `/auth/devices` и revoke, клиент использует.
- **Signal-интеграция мульти-девайснее:** детерминированный `uuidToSignalDeviceId()`, поддержка `device_id/signal_device_id` в бандле.
- **Endpoint статуса ключей** (`/auth/keys/status`) — база для автопополнения one-time prekeys и ротации signed prekey.
- **Чистка токенов** (expired/revoked), чтобы таблица не росла бесконечно.
- **JWT_SECRET fail-fast, iss/aud/typ, ревокация, install.sh идемпотентен, SSRF закрыт, README честный.**

---

## Что ещё стоит дожать (особенно для max E2EE)

### 1) Ключи нельзя держать в localStorage — ✅ Сделано

~~Сейчас ключи (включая **identitySecret**) в localStorage.~~ Ключи перенесены в **IndexedDB**.

**Осталось (опционально):** WebCrypto non-extractable + шифрование "ключом разблокировки" (passphrase → PBKDF2/Argon2).

### 2) One-time prekeys: нужна private-часть — ✅ Сделано

~~Сейчас генерация OTPK делает только публичные ключи.~~ OTPK как KeyPair (pub+priv), priv в IndexedDB; consumed удаляются; replenishment сохраняет priv.

### 3) Trust-модель identity keys — ✅ Сделано

Trusted identity keys в **IndexedDB**; при смене ключа — баннер и блок отправки в strict до подтверждения safety-number.

### 4) Автогигиена ключей — ✅ Сделано

Клиент по `/auth/keys/status`:
- rotation signed prekey раз в 7 дней;
- replenishment OTPK при запасе < 20;
- `next_one_time_key_id` от сервера.

### 5) Multi-device для получателя — ✅ Сделано

Endpoint `GET /keys/devices/{userID}`, клиент шифрует для каждого устройства получателя; relay доставляет per-device (`recipientAddr:deviceID`).

---

## План следующего PR (без GitHub Actions)

| # | Задача | Статус |
|---|--------|--------|
| 1 | Вынести E2EE секреты из localStorage: IndexedDB минимум; лучше — WebCrypto wrapping + unlock при входе | [x] |
| 2 | One-time prekeys полноценные (pub+priv) + локальное хранение priv + удаление consumed | [x] |
| 3 | Подключить `/auth/keys/status` в клиенте: автопополнение OTPK и ротация signed prekey по таймеру/при старте | [x] |
| 4 | Дожать trust-модель: хранить "trusted identity keys" персистентно; strict — блок до подтверждения safety-number | [x] |
| 5 | Multi-device для получателя: endpoint "список устройств + бандлы", шифровать per-device, доставлять per-device | [x] |
| 6 | E2EE для вложений: шифровать файл (CEK+nonce), на сервер — только ciphertext, CEK через Signal-сообщение | [x] |
| 7 | Укрепить фронт: CSP без inline, санация контента, минимум логов plaintext | [x] |

### Выполнено (предыдущий PR)

- Web DX (eslint, prettier, fmt/lint), ws_token через Sec-WebSocket-Protocol, E2EE Strict, уведомления о смене ключей, multi-device UI, /auth/keys/status, replenishment OTPK, чистка токенов, CSP.

### Выполнено (текущий PR)

- **E2EE секреты в IndexedDB** — ключи перенесены из localStorage в IndexedDB; миграция при загрузке.
- **OTPK pub+priv** — one-time prekeys как KeyPair, priv хранится локально; consumed удаляются; replenishment сохраняет priv.
- **Keys status подключён** — автопополнение OTPK при < 20; ротация signed prekey каждые 7 дней; identitySigningSecret для подписи.
- **Trust-модель** — trusted identity keys в IndexedDB; миграция из localStorage; strict блокирует отправку до подтверждения при смене ключа.
- **Multi-device получателя** — GET /keys/devices/{userID}, encryptDMForRecipient шифрует на каждое устройство; relay Sync выбирает ciphertext по recipientAddr:deviceID.
- **E2EE вложений** — файл шифруется CEK+nonce (NaCl secretbox), на сервер только ciphertext; file_key+nonce передаются в Signal-сообщении.
- **Фронт-укрепление** — `logError()` вместо console.error, контексты логов, `sanitizeForDisplay()` для отображаемого контента. **CSP без inline** — все inline стили вынесены в CSS (Chat.css, index.css), `style-src 'self'` без `'unsafe-inline'`.

### DX-бонус

Минимальный `web/eslint.config.js` (flat config), если мигрируешь с `.eslintrc.cjs`:

```js
// web/eslint.config.js
import tseslint from "typescript-eslint";
import reactHooks from "eslint-plugin-react-hooks";
import react from "eslint-plugin-react";

export default [
  ...tseslint.configs.recommended,
  {
    files: ["src/**/*.{ts,tsx}"],
    plugins: { react, "react-hooks": reactHooks },
    rules: {
      "react-hooks/rules-of-hooks": "error",
      "react-hooks/exhaustive-deps": "warn",
    },
  },
];
```

---

## Архив: выполненные задачи (1–10)

### 1. Установка и секреты

#### 1.1 Не перегенерировать секреты при повторном `install.sh`

**Проблема:** Скрипт безусловно делает `update_env "JWT_SECRET"`, `POSTGRES_PASSWORD`, `MINIO_ROOT_PASSWORD`, `DB_ENCRYPTION_KEY` и т.д. Повторный запуск на работающем инстансе может:
- инвалидировать JWT-сессии
- потерять доступ к БД/MinIO
- при DB encryption — сделать данные нерасшифровываемыми

**Решение:** Генерировать секрет **только если переменной нет** — `set_if_missing` вместо безусловного `update_env`.

- [x] Добавить `get_env`, `set_if_missing` в install.sh, install-selfhost.sh
- [x] Применить к JWT_SECRET, POSTGRES_PASSWORD, MINIO_ROOT_PASSWORD, DB_ENCRYPTION_KEY

#### 1.2 Безопасность bootstrap.sh (curl | sudo sh)

- [x] Поддержка pinned commit/tag (`BRANCH=v0.3.1`, `EXPECTED_SHA256`)
- [x] Режим «скачать → показать diff → подтверждение» (BOOTSTRAP_DIFF=1)
- [x] Логировать версию (commit hash) при установке

---

### 2. E2EE

#### 2.1 Signal / Double Ratchet

- [x] Постоянное хранилище для Signal store (IndexedDB вместо in-memory)
- [x] UI: верификация safety number / fingerprint (MVP)
- [x] Multi-device (deviceId ≠ 1)

#### 2.2 MVP-шифрование

- [x] Обновить README/доки: честно описать упрощённую схему, Signal — «опционально/экспериментально»
- [x] Документировать threat model (аутентификация шифротекста, MITM при подмене prekey bundle)

---

### 3. Федерация

#### 3.1 Blocklist URL — таймаут и валидация

**Проблема:** `http.Get` без `Timeout` и `context` — может повесить горутину.

- [x] Использовать `http.Client{Timeout: ...}` + `NewRequestWithContext`
- [x] Валидировать BlocklistURL (не file://, не внутренние адреса)

#### 3.2 Дополнительно

- [x] Отдельный лимит на discovery/.well-known + кеширование
- [x] Trust-on-first-contact: порог доверия (например 2 успешных транзакции) перед записью в allowlist

---

### 4. VPN

#### 4.1 Выбор WG-адреса без скана

**Проблема:** `getWireGuardConfig()` тянет все `client_address` и вычисляет `NextClientAddress(usedAddrs)`. При сотнях/тысячах пиров — дорого.

- [x] Выдавать адреса транзакционно в БД (sequence vpn_wg_addr_seq)
- [x] Хранить адрес как `inet/cidr` (Postgres), запросы по сети

#### 4.2 Ротация peer при повторной выдаче конфига

- [x] Перед обновлением сохранять старые значения и удалять старый peer из WG
- [x] Логировать ошибки xrayAPI, добавить retry/backoff

---

### 5. Keydir (хранилище ключей)

- [x] Серверная валидация signed prekey signature при `UpdateKeys()`
- [x] Квоты/лимиты на one-time prekeys на устройство (500/device)

---

### 6. CI/CD и наблюдаемость

#### CI (GitHub Actions)

- [ ] `go test ./...`, golangci-lint, govulncheck
- [ ] `npm ci`, `npm run build`, `npm audit`
- [ ] Сборка Docker образов + SBOM (syft) + скан (trivy)

#### Наблюдаемость

- [x] Метрики по VPN (выдача/ревок)
- [x] Латентность запросов к DB/Redis (db_ping_duration_seconds, redis_ping_duration_seconds)
- [x] Structured logs везде (у федерации уже есть)

---

### 7. Roadmap (история фаз)

#### Фаза 1 — Критичные исправления

| # | Задача | Файлы |
|---|--------|-------|
| 1 | ✓ Секреты: не перетирать при повторном install | install.sh, install-selfhost.sh |
| 2 | ✓ Blocklist URL: http.Client timeout | federation/security.go |

#### Фаза 2 — E2EE и федерация

| # | Задача | Файлы |
|---|--------|-------|
| 3 | ✓ Стратегия E2EE: обновить README/доки | README.md, docs/ |
| 4 | ✓ Signal: persistent store (IndexedDB) | web/src/crypto/signal-store.ts |
| 5 | ✓ Trust-on-first-contact: порог доверия | federation/security.go |

#### Фаза 3 — VPN и keydir

| # | Задача | Файлы |
|---|--------|-------|
| 6 | ✓ WG: выдача адресов транзакционно + ротация peer | vpn/service.go, wg_stats.go, migration 016 |
| 7 | ✓ xrayAPI: логирование, retry | vpn/xrayapi.go |
| 8 | ✓ Keydir: валидация signed prekey | keydir/keydir.go |

#### Фаза 4 — CI и наблюдаемость

| # | Задача | Файлы |
|---|--------|-------|
| 9 | GitHub Actions: тесты, линтеры, scan | .github/workflows/ |
| 10 | ✓ Метрики VPN (выдача/ревок) | api/vpn_metrics.go |

#### Фаза 5 — bootstrap и безопасность

| # | Задача | Файлы |
|---|--------|-------|
| 11 | ✓ bootstrap.sh: pinned version, log commit | bootstrap.sh |

---

### 8. План PR 2 (JWT, ws_token, Signal) — выполнен

*Без пункта про GitHub Actions — его пока не включаем.*

### 0) Зафиксировать уже сделанное (changelog/релиз-ноты)

Отразить в changelog — код менять не надо:

- [x] `install.sh` безопасно перезапускать (секреты через `set_if_missing`)
- [x] blocklist: timeout + лимит тела + фильтрация URL (SSRF/подвисания закрыты)
- [x] WireGuard: стабильная выдача адресов + удаление старого peer при смене pubkey
- [x] E2EE: Signal + IndexedDB store + fallback на MVP
- [x] Клиент реально шифрует через Signal (с fallback) и хранит "ciphertext для себя"

---

### 1) JWT_SECRET: fail-fast в проде

**Цель:** Убрать риск «случайно подняли прод без секрета».

- [x] Если `ENV=production` или `APP_ENV=prod` и `JWT_SECRET` пустой → panic/return error
- [x] В dev — оставить fallback (или автогенерация)

**Файлы:** `server/cmd/server/main.go`, (опц.) `.env.example` / docs: описать `APP_ENV`

---

### 2) JWT validation hardening

**Цель:** Защита от algorithm confusion и мусорных токенов.

- [x] `jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}))` (или актуальный алгоритм)
- [x] Опционально: проверять `iss`/`aud`, `token.Use`/`type` (access vs ws_token)

**Файлы:** файл с JWT-логикой (ValidateToken, middleware)

---

### 3) access_tokens: ревокация или убрать

Сейчас токены хешируются и пишутся в БД, но **не проверяются по БД** при запросах → ревокации по факту нет.

**Вариант A (рекомендуемый): включить реальную ревокацию**

- [x] Добавить в JWT claim `jti` (uuid)
- [x] В БД: `token_hash`, `jti`, `user_id`, `expires_at`, `revoked_at`
- [x] В ValidateToken/middleware: по `jti` проверять запись (существует, не revoked, не expired)
- [x] Logout endpoint: помечать токен revoked

**Вариант B: убрать таблицу `access_tokens`** — не делаем (оставляем ревокацию)

- [x] ~~Удалить сохранение токенов в БД~~ — оставляем Variant A

**Файлы:** `middleware.go`, auth service, миграции `access_tokens`

---

### 4) WebSocket auth: уйти от access_token в query string

**Цель:** Не светить «главный» токен в URL (логи/рефереры/прокси).

**Решение: короткоживущий `ws_token`**

- [x] Endpoint `POST /auth/ws-token`: принимает access token (Authorization), возвращает `ws_token` с TTL 60 с
- [x] WebSocket: ws_token через Sec-WebSocket-Protocol (bearer.TOKEN), не в query; fallback на query
- [x] JWT claims: `typ=ws`, отдельный expiry

**Файлы:** `web/src/api/client.ts`, WS handler + auth middleware, JWT/claims

**Доп.:** Traefik/nginx — query string не логировать (см. п. 5)

---

### 5) Traefik/прокси: query string не попадает в логи

**Цель:** Даже ws_token лучше не писать в access logs.

- [x] Traefik: описать в docs (отключить / RequestPath: drop)
- [x] Описать в docs «Production hardening» (SECURITY-HARDENING.md)

**Файлы:** `docker-compose`, traefik конфиги, `docs/SECURITY-HARDENING.md`

---

### 6) Signal: registrationId настоящий и персистентный

**Цель:** Убрать константу `0x1234`, подготовка к multi-device.

- [x] Генерировать `registrationId` один раз (random uint16 по требованиям libsignal)
- [x] Сохранять в IndexedDB (signal-store)
- [x] Не перезаписывать на каждом `initStore()`

**Файлы:** `web/src/crypto/signal-store.ts`, `web/src/crypto/signal.ts`

---

### 7) Signal: signed prekey по реальному key_id

**Цель:** Корректная ротация ключей и совместимость с keydir.

- [x] Протянуть `signed_prekey.key_id` из серверного ответа / локальной генерации
- [x] `storeSignedPreKey(key_id, ...)` вместо `storeSignedPreKey(1, ...)`
- [x] Аналогично для prekeys (key_id из массива)

**Файлы:** `web/src/crypto/signal.ts`, `StoredKeys`, keydir (сервер)

---

### 8) README/доки: выровнять обещания про E2EE

**Цель:** README не звучит как «полный Signal везде», если есть режимы (Signal vs MVP).

- [x] Чётко написать: «Signal Protocol (если доступен) + fallback MVP encryption»
- [x] Что где используется (личные сообщения, federation)
- [x] Как проверить/включить Signal режим, что хранится в IndexedDB

**Файлы:** `README.md`, (опц.) `docs/E2EE.md`

---

### 9) DX: автоформат/линт локально (без CI пока)

**Цель:** Проект легче поддерживать, код ровный.

- [x] Go: `gofmt`, `goimports` (scripts/fmt.sh)
- [x] Web: `eslint`, `prettier` (`npm run lint`, `npm run format`)
- [x] Shell: `shfmt`, `shellcheck` (scripts/fmt.sh, scripts/lint.sh)

**Файлы:** `package.json`, `scripts/`, `Makefile`

---

### 10) Makefile / justfile: единая точка входа

**Цель:** Одинаково удобно разработчику и контрибьютору.

- [x] `make dev` (compose)
- [x] `make up-main` / `make up-selfhost` / `make down`
- [x] `make fmt` / `make lint`
- [x] `make migrate`
- [x] `make clean`

**Файлы:** `Makefile` в корне
