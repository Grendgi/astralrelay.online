# Запуск Self-host сервера

Подробно: [SETUP-SELFHOST.md](SETUP-SELFHOST.md)

## Быстрый старт

Достаточно одного скрипта (Docker, UFW, порты настраиваются автоматически):

```bash
curl -fsSL https://raw.githubusercontent.com/Grendgi/astralrelay.online/main/bootstrap.sh | sudo sh
# в мастере выбрать 2 (selfhost)
```

Или только selfhost: `sudo ./install-selfhost.sh`. Скрипты в `deploy/` вручную вызывать не нужно.

## Что поднимается

| Сервис   | Порт   | Назначение   |
|----------|--------|--------------|
| web      | 3000   | Веб-клиент   |
| server   | 8080   | API          |
| postgres | —      | Только внутри Docker |
| redis    | —      | Только внутри Docker |
| minio    | —      | Только внутри Docker |

Порты БД не публикуются (анонимность).

## Федерация

1. В `.env`: `SERVER_DOMAIN=ваш-домен.example`
2. DNS: A/AAAA на IP сервера.
3. HTTPS: nginx/caddy/traefik с Let's Encrypt.

Без своего домена: `YOUR_IP.nip.io` или `YOUR_IP.sslip.io` (см. [SELF-HOSTING.md](SELF-HOSTING.md)).

## Подключение к mesh (main)

**Cloudflare:** если домен main за прокси (оранжевое облако), порт 9443 до coordinator недоступен. В панели Cloudflare переведите запись на **DNS only** (серое облако) или заведите отдельную A-запись (например `mesh.домен`) с серым облаком на IP main.

Если в мастере вы выбрали «Подключить mesh» и **JOIN_TOKEN не подтянулся автоматически**:

1. На **MAIN**-сервере откройте порт **9443/TCP** (фаервол / security group).
2. Получите токен: на MAIN выполните `curl -s http://localhost:9443/v1/token` или с другой машины `curl -s http://ДОМЕН_MAIN:9443/v1/token`.
3. Вставьте токен в мастере при установке selfhost или позже: добавьте в `deploy/selfhost/.env` строки `COORDINATOR_URL=http://ДОМЕН_MAIN:9443` и `MESH_JOIN_TOKEN=полученный_токен`, затем снова запустите `./install.sh` (выбор 2, вставьте токен) — скрипт зарегистрирует узел в mesh. Либо вручную: `JOIN_TOKEN=... COORDINATOR_URL=... DOMAIN=... ./scripts/setup-mesh.sh`, добавьте вывод (MESH_VPN_ADDR и др.) в .env и перезапустите `./deploy/selfhost/run.sh`.

## Cloudflare Tunnel (без открытия портов)

```bash
docker compose -p selfhost -f deploy/selfhost/docker-compose.yml -f deploy/selfhost/docker-compose.tunnel.yml --env-file deploy/selfhost/.env up -d
docker compose -p selfhost logs -f cloudflared
```

## Скрипт только для self-host

```bash
sudo ./install-selfhost.sh   # без выбора main/selfhost
```
