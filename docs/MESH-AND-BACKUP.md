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

```bash
MESH_ENABLED=1 sudo ./install.sh
# Режим: 1 (main)
```

Поднимаются: coordinator :9443, backup-receiver :9100, WireGuard. После запуска получите JOIN_TOKEN:

```bash
curl -s http://YOUR_DOMAIN:9443/v1/token
# {"token":"xxx"}
```

### Второй и последующие узлы

Токен запрашивается автоматически — вручную ничего получать не нужно.

```bash
INSTALL_COORDINATOR_URL=http://first-hub.domain:9443 sudo ./install.sh
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

При `COORDINATOR_CA_CERT` и `COORDINATOR_CA_KEY` coordinator выдаёт клиентские сертификаты для федерации. `setup-mesh.sh` при наличии `JOIN_TOKEN` автоматически запрашивает сертификат и добавляет `FEDERATION_MTLS_CLIENT_CERT/KEY` в `.env`. Подробнее: [FEDERATION-SECURITY.md](FEDERATION-SECURITY.md).
