# VPN Mesh и репликация БД

Единая VPN-сеть между серверами + автоматическая отправка бэкапов на peer-узлы.

---

## 1. Архитектура

- **Coordinator** — на первом Main-хабе. Выдаёт WireGuard-конфиги новым узлам.
- **WireGuard mesh** — подсеть `10.100.0.0/16`, каждый узел — `10.100.0.x`.
- **Backup-receiver** — принимает pg_dump от peers по VPN.
- **backup-to-peers.sh** — cron-скрипт: pg_dump → отправка на BACKUP_PEERS.

---

## 2. Автоматическая установка

### Первый узел (Main, coordinator)

На main **coordinator всегда поднимается** (порт **9443**) вместе с основным compose (`docker-compose.mesh.yml`). Отдельно включать mesh не нужно.

```bash
curl -fsSL https://raw.githubusercontent.com/Grendgi/astralrelay.online/main/bootstrap.sh | sudo sh
# или: sudo ./install.sh → режим 1 (main)
```

После запуска токен для подключения selfhost-узлов:

```bash
curl -s http://YOUR_DOMAIN:9443/v1/token
# {"token":"xxx"}
```

**Cloudflare:** если домен main за прокси (оранжевое облако), порт 9443 снаружи недоступен. Нужна запись с **DNS only** (серое облако) или отдельная A-запись на IP main — иначе selfhost не получит токен автоматически. См. [RUN-MAIN.md](RUN-MAIN.md).

### Второй и последующие узлы (selfhost)

При установке selfhost в мастере можно выбрать «Подключить mesh» и ввести домен main — токен запрашивается автоматически с `http://ДОМЕН_MAIN:9443/v1/token`. Если токен не подтянулся (например, 9443 закрыт Cloudflare), получите его вручную на main и вставьте в мастере или добавьте в `deploy/selfhost/.env`: `COORDINATOR_URL=http://ДОМЕН_MAIN:9443`, `MESH_JOIN_TOKEN=...`, затем при необходимости запустите `scripts/setup-mesh.sh`. Подробнее: [RUN-SELFHOST.md](RUN-SELFHOST.md).

```bash
# Установка selfhost с указанием coordinator (если нужен нестандартный URL)
COORDINATOR_URL=http://first-hub.domain:9443 sudo ./install.sh
# Режим: 2 (selfhost)
```

**Subdomain главного домена** — трафик через main Traefik:
```bash
INSTALL_ADDRESS_MODE=subdomain MAIN_DOMAIN=astralrelay.online sudo ./install.sh
```
Получите `1-2-3-4.astralrelay.online` (по IP). Не нужен nip.io/свой домен.

Режим: 2 (selfhost) или 1 (main). Скрипт регистрируется, получает WireGuard-конфиг, при subdomain — маршрут в Traefik main.

---

## 3. Cron для бэкапов

Добавьте в crontab (каждые 6 часов):

```bash
0 */6 * * * /path/to/Chat_VPN/scripts/backup-to-peers.sh
```

Или через systemd timer — см. `scripts/backup-to-peers.sh`.

---

## 4. Восстановление из бэкапа

При падении узла A:

1. Данные A лежат в `backup_data/from-A.domain/` на узлах B, C.
2. Восстановление:
   ```bash
   gunzip -c latest.dump.gz | psql -U messenger -d messenger
   ```
3. Поднять новый сервер с тем же `SERVER_DOMAIN`, применить дамп.

---

## 5. API Coordinator

| Endpoint | Описание |
|----------|----------|
| GET /v1/token | Получить токен для join (создаёт при первом вызове) |
| GET /v1/config?public_key=...&endpoint=...&domain=...&token=... | Регистрация, возвращает wg0.conf |
| POST /v1/join | JSON-регистрация (альтернатива) |
| POST /v1/cert | mTLS: выдача клиентского сертификата (token, domain). Требует COORDINATOR_CA_CERT/KEY |
| GET /v1/peers?token=... | Список узлов (для sync) |
| GET /health | Healthcheck |

### mTLS при mesh join

При `COORDINATOR_CA_CERT` и `COORDINATOR_CA_KEY` coordinator выдаёт клиентские сертификаты для федерации. `scripts/setup-mesh.sh` при наличии `MESH_JOIN_TOKEN` (или `JOIN_TOKEN`) и `COORDINATOR_URL` автоматически запрашивает сертификат и добавляет `FEDERATION_MTLS_CLIENT_CERT/KEY` в `.env`. Подробнее: [FEDERATION-SECURITY.md](FEDERATION-SECURITY.md).
