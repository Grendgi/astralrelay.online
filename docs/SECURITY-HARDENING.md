# Усиление безопасности

Цель: максимальная защищённость и устойчивость к взлому. План действий и рекомендации.

## Текущее состояние

| Компонент | Реализация | Риски |
|-----------|------------|-------|
| E2EE | MVP: X25519 + NaCl secretbox | Нет Double Ratchet, нет forward secrecy |
| Ключи | Хранение в БД (зашифровано при DB_ENCRYPTION_KEY) | Зависимость от секрета |
| Аутентификация | JWT | Стандартно, токены имеют срок жизни |
| Транспорт | HTTPS | Обязательно в продакшене |
| Данные | PostgreSQL, MinIO | Шифрование at-rest — на уровне ОС/диска |

## Roadmap: максимальная защита

### 1. Signal Protocol (libsignal) — высокий приоритет

**Задача:** переход на полноценный Signal protocol (X3DH + Double Ratchet).

**Пакет:** `@privacyresearch/libsignal-protocol-typescript` — работает в браузере.

**Эффект:**
- Forward secrecy — компрометация ключей не раскрывает старые сообщения
- Лучшая стойкость к атакам
- Соответствие лучшим практикам E2EE

**План:** см. [LIBSIGNAL-INTEGRATION.md](./LIBSIGNAL-INTEGRATION.md)

### 2. Секреты и хранение

- **JWT_SECRET** — минимум 256 бит, ротация при компрометации
- **DB_ENCRYPTION_KEY** — обязательно в продакшене, `openssl rand -base64 32`
- **Переменные окружения** — не коммитить `.env`, использовать секретное хранилище (Docker secrets, Vault) при возможности

### 3. Сеть и транспорт ✓ (частично)

- **Только HTTPS** в продакшене
- **HSTS** ✓ — включено в deploy/main (Traefik: stsSeconds=31536000, stsIncludeSubdomains, stsPreload)
- **Security headers** ✓ — X-Content-Type-Options: nosniff, X-Frame-Options: DENY, Referrer-Policy, Content-Security-Policy
- **Ограничение запросов** — rate limiting на API
- **Федерация** ✓ — rate limit по домену, blocklist/allowlist, mTLS, webhook-алерты. См. [FEDERATION-SECURITY.md](./FEDERATION-SECURITY.md), [WAF.md](./WAF.md)

### 4. Полная пересборка (чистый сброс)

При компрометации или для сброса на чистое состояние:

1. Выполнить `./scripts/clean-rebuild.sh main -f`
2. Сменить все секреты в `.env`
3. Перезапустить
4. Пользователям — очистить localStorage в браузере и перерегистрироваться

Подробнее: [CLEAN-REBUILD.md](./CLEAN-REBUILD.md)

### 5. Аудит и логи

- Не логировать пароли, токены, содержимое сообщений
- Логировать только метаданные (IP, путь, статус) при необходимости расследования

### 6. Traefik: query string не в access logs

WebSocket передаёт `ws_token` через Sec-WebSocket-Protocol (subprotocol `bearer.TOKEN`), не в query. Раньше был query — если логируют path, токен мог попасть в логи.

**Вариант A (рекомендуется):** не включать access logs. По умолчанию при отсутствии секции `accessLog` в traefik.yml access logs могут не писаться.

**Вариант B:** если нужны access logs — исключить поле с query:

```yaml
accessLog:
  format: json
  fields:
    names:
      RequestPath: drop   # путь+query — не логируем (содержит ws_token)
```

**Вариант C:** для роута `/api/v1/messages/stream` отключить логи через `observability.accessLogs=false` в динамической конфигурации (требует отдельного роутера).

### 8. Content-Security-Policy (CSP) — защита от XSS

CSP ограничивает источники скриптов, стилей и других ресурсов, снижая риск XSS и утечки E2EE-данных.

**Реализовано:**
- CSP задаётся в Traefik (secure-headers) и в `index.html` (meta fallback)
- `script-src 'self'` — без inline-скриптов (тема инициализируется в main.tsx)
- `style-src 'self' https://fonts.googleapis.com` — без inline styles (все стили в CSS), Google Fonts
- `connect-src 'self' ws: wss:` — API и WebSocket
- `img-src 'self' data: blob:` — изображения, превью файлов
- `frame-ancestors 'none'` — запрет встраивания в iframe

**Рекомендации:** не логировать plaintext в `console.*`, санитизировать пользовательский контент.

## Чеклист перед продакшеном

- [ ] JWT_SECRET, пароли БД и MinIO — сгенерированы и уникальны
- [ ] DB_ENCRYPTION_KEY задан
- [ ] HTTPS включён (Let's Encrypt)
- [ ] `.env` не в репозитории
- [ ] Redis включён (REDIS_DISABLED=false) для HA
- [ ] Push VAPID (если нужны уведомления)
