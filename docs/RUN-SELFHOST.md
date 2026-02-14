# Запуск Self-host сервера

Подробно: [SETUP-SELFHOST.md](SETUP-SELFHOST.md)

## Быстрый старт

```bash
sudo ./install-selfhost.sh   # только self-host (рекомендуется)
# или:
sudo ./install.sh            # выбрать selfhost, ввести IP или домен
# вручную:
cp deploy/selfhost/.env.example deploy/selfhost/.env
# Заполните: SERVER_DOMAIN, JWT_SECRET, пароли
./deploy/selfhost/run.sh
```

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

## Cloudflare Tunnel (без открытия портов)

```bash
docker compose -p selfhost -f deploy/selfhost/docker-compose.yml -f deploy/selfhost/docker-compose.tunnel.yml --env-file deploy/selfhost/.env up -d
docker compose -p selfhost logs -f cloudflared
```

## Скрипт только для self-host

```bash
sudo ./install-selfhost.sh   # без выбора main/selfhost
```
