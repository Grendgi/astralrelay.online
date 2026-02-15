# Защита федерации

Меры защиты федеративных S2S-запросов: rate limiting, allowlist/blocklist, валидация, main_only режим.

---

## Переменные окружения

| Переменная | По умолчанию | Описание |
|------------|--------------|----------|
| `FEDERATION_RATE_LIMIT` | 100 | Запросов в минуту на один домен |
| `FEDERATION_MAX_BODY_SIZE` | 1048576 | Макс. размер body (1MB) |
| `FEDERATION_ALLOWLIST_MODE` | auto | `auto`, `manual`, `open` |
| `FEDERATION_ALLOWLIST_PATH` | — | Файл allowlist (один домен на строку) |
| `FEDERATION_ALLOWLIST_TRUST_THRESHOLD` | 1 | auto: минимум успешных транзакций перед добавлением в allowlist |
| `FEDERATION_BLOCKLIST_PATH` | — | Локальный файл blocklist |
| `FEDERATION_BLOCKLIST_URL` | — | URL для загрузки blocklist (JSON-массив или текст) |
| `FEDERATION_BLOCKLIST_RELOAD_HOURS` | 6 | Интервал обновления blocklist из URL |
| `FEDERATION_MODE` | open | `open`, `main_only` |
| `FEDERATION_MAIN_DOMAIN` | — | Для main_only: домен главного хаба |
| `FEDERATION_MTLS_CLIENT_CERT` | — | Путь к клиентскому сертификату (PEM) |
| `FEDERATION_MTLS_CLIENT_KEY` | — | Путь к приватному ключу (PEM) |
| `FEDERATION_ALERT_WEBHOOK_URL` | — | Webhook URL: POST при rate limit и blocklist |

**Кеширование discovery:** запросы `/.well-known/federation` к удалённым доменам кешируются на 5 минут. Это снижает нагрузку при повторных транзакциях на тот же домен.

---

## Режимы allowlist

- **auto** — trust-on-first-contact: домен добавляется при первой успешной транзакции (подпись + discovery)
- **manual** — только домены из allowlist-файла
- **open** — без ограничений (как раньше)

---

## Режим main_only

Для self-host: общение **только** через главный хаб.

- **Отправка**: все транзакции идут на Main, Main пересылает получателю
- **Приём**: принимаются транзакции **только** от Main

Self-host не принимает прямые запросы от других self-host и не шлёт им напрямую.

`install-selfhost.sh` по умолчанию устанавливает `FEDERATION_MODE=main_only` и `FEDERATION_MAIN_DOMAIN=astralrelay.online`.

---

## Лимиты валидации

| Поле | Лимит |
|------|-------|
| Body | 1 MB |
| Events в транзакции | 100 |
| Event ID | 128 символов |
| User ID | 256 символов |
| Ciphertext (base64) | 128 KB |
| Timestamp | ±5 мин от текущего времени |

---

## Таймауты

- HTTP timeout для inbound transaction: 10 сек
- Timestamp в заголовках: окно ±5 мин (уже было в VerifyRequest)

---

## Main как ретранслятор

Главный хаб автоматически пересылает транзакции, если `req.Destination != наш_домен`. Транзакция отправляется на целевой домен с сохранением Origin (источник) в теле.

---

## Логирование и метрики

### Structured logging (JSON)

Каждый федеративный запрос логируется:

```json
{"type":"federation","method":"POST","path":"/federation/v1/transaction","domain":"peer.example.org","status":200,"duration":45}
```

### Prometheus metrics

Эндпоинт: `GET /metrics` (порт API, например `:8080/metrics`)

| Метрика | Описание |
|---------|----------|
| `federation_requests_total{domain,path,status}` | Всего федеративных запросов |
| `federation_request_duration_seconds{path}` | Гистограмма латентности |
| `federation_blocklist_hits_total{domain}` | Срабатывания блоклиста |

> Для сбора метрик: добавьте Prometheus scrape target на `http://server:8080/metrics`

### Webhook-алерты (опционально)

При задании `FEDERATION_ALERT_WEBHOOK_URL` сервер отправляет POST с JSON при:

- **rate_limit** — превышение rate limit (X-Server-Origin)
- **blocklist** — запрос от домена из blocklist

Тело: `{"event":"rate_limit|blocklist","domain":"peer.example.org"}`

---

### Изоляция

Федеративные handler'ы обёрнуты:
- `federationRecover` — перехват panic с логом стека
- `federationLogger` — структурированное логирование

---

## mTLS (опционально)

Для усиления защиты федеративных соединений можно использовать mTLS (клиентские сертификаты).

### Outbound (self-host / Main отправляет запросы)

Когда заданы `FEDERATION_MTLS_CLIENT_CERT` и `FEDERATION_MTLS_CLIENT_KEY`, сервер использует клиентский сертификат при всех федеративных HTTPS-запросах.

```env
FEDERATION_MTLS_CLIENT_CERT=/path/to/client-cert.pem
FEDERATION_MTLS_CLIENT_KEY=/path/to/client-key.pem
```

### Inbound (Main принимает от self-host с mTLS)

Проверку клиентского сертификата выполняет Traefik. Пример конфигурации:

```yaml
# traefik/dynamic/federation-mtls.yaml
tls:
  options:
    federationMtls:
      clientAuth:
        caFiles:
          - /path/to/ca.pem
        clientAuthType: RequireAndVerifyClientCert
```

Для роутеров федерации: `tls.options: federationMtls`

### Получение сертификатов

- **Coordinator при mesh join**: endpoint `POST /v1/cert` выдаёт клиентский сертификат для домена, зарегистрированного в mesh.
  - Переменные coordinator: `COORDINATOR_CA_CERT`, `COORDINATOR_CA_KEY` (пути к CA).
  - Скрипт `setup-mesh.sh` при наличии `JOIN_TOKEN` автоматически запрашивает сертификат и сохраняет в `MTLS_DIR` (по умолчанию `/etc/messenger/federation`).
  - Добавьте в `.env` после mesh join: `FEDERATION_MTLS_CLIENT_CERT=...`, `FEDERATION_MTLS_CLIENT_KEY=...`
- **Ручная генерация**: `openssl req -new -x509 -nodes -days 365 ...`

---

## WAF (опционально)

Для дополнительной защиты на уровне протокола (SQLi, XSS и т.п.) см. [docs/WAF.md](WAF.md).
