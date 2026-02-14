# Self-Hosting — развёртывание своего инстанса

Любой может развернуть свой инстанс мессенджера и подключить его к федеративной сети. Участие человека минимально: скачать проект и запустить.

---

## Быстрый старт

**Только self-host** (рекомендуется, main один):
```bash
git clone https://github.com/Grendgi/astralrelay.online.git
cd astralrelay.online
sudo ./install-selfhost.sh
```

**Main или self-host** (универсальный скрипт):
```bash
sudo ./install.sh
```

**install.sh** спрашивает:
1. Режим: main (hub) или selfhost
2. Домен/IP: введите IP → будет `1.2.3.4.nip.io`, или свой домен, или localhost
3. Email для Let's Encrypt (при домене/nip.io)

Генерирует секреты, создаёт `.env`, запускает. С nip.io — HTTPS и федерация без покупки домена.

---

## Что происходит при первом запуске

| Действие | Автоматически |
|----------|---------------|
| Создание `.env` | из `deploy/main/.env.example` или `deploy/selfhost/.env.example` |
| Генерация JWT_SECRET | openssl rand |
| Ключи федерации (Ed25519) | при первом запросе |
| Миграции БД | при старте server |
| Создание bucket в MinIO | при первом upload |

---

## Конфигурация (опционально)

Перед запуском или после редактируйте `.env`:

| Переменная | По умолчанию | Описание |
|------------|--------------|----------|
| `SERVER_DOMAIN` | localhost | Ваш домен (обязательно для федерации) |
| `JWT_SECRET` | генерируется | Секрет для JWT (32+ байт) |
| `POSTGRES_PASSWORD` | messenger_dev | Пароль БД |
| `MINIO_ROOT_PASSWORD` | minioadmin | Пароль MinIO |

Для продакшена обязательно задать:
- `SERVER_DOMAIN` — домен, на который указывает DNS
- `JWT_SECRET` — случайная строка (или сгенерируется install.sh)
- Надёжные пароли для PostgreSQL и MinIO

---

## Безопасность и анонимность

По умолчанию пользователи не имеют доступа к БД и внутренним сервисам:

| Компонент | Доступ | Описание |
|-----------|--------|----------|
| PostgreSQL, Redis, MinIO | Только внутри Docker-сети | Порты не публикуются на хост. К ним подключается только server. |
| Xray gRPC API (10085) | Только server | Добавление/удаление VPN-пользователей — без доступа снаружи. |
| Web (3000), API (8080) | Публичные | Единственная точка входа для пользователей. |

**Шифрование в БД:** Задайте `DB_ENCRYPTION_KEY` (base64, 32 байта). Чувствительные поля (password_hash, ключи устройств, prekeys, token_hash, backup salt) шифруются AES-256-GCM перед записью. Без ключа сервера прочитать данные нельзя. `install.sh` генерирует ключ при первом запуске.

**Рекомендации для анонимности:**
- Логи: метод, путь, статус, латентность — без IP и user_id (реализовано).
- В продакшене: сильные пароли PostgreSQL/MinIO, `.env` вне git.
- Для отладки БД: `./deploy/dev/run.sh`. Не используйте dev-режим в продакшене.

---

## Федерация

Федерация всегда включена. Другие инстансы найдут вас по `https://YOUR_DOMAIN/.well-known/federation`.

### С собственным доменом

1. `SERVER_DOMAIN=your-domain.com` в `.env`
2. DNS: A/AAAA запись → IP сервера
3. HTTPS: nginx/caddy/traefik с Let's Encrypt
4. Порты: 80, 443 (прокси на 3000, 8080)

### Без покупки домена

Запустите `sudo ./install.sh` — скрипт спросит:
- **Режим адреса:** 1) Свой адрес (nip.io, IP, Cloudflare) — по умолчанию  2) Subdomain главного
- **Mesh:** домен главного или Enter

**Свой адрес** — nip.io, sslip.io, Cloudflare Tunnel, внешний IP (рекомендуется):

#### nip.io / sslip.io — по IP (VPS, статический IP)

`YOUR_IP.nip.io` резолвится в ваш IP. Let's Encrypt выдаёт сертификат. **install.sh** автоматически подставляет `.nip.io` при вводе IP.

```bash
# В install.sh введите IP (напр. 1.2.3.4) — получится 1.2.3.4.nip.io
# Или в .env:
SERVER_DOMAIN=1.2.3.4.nip.io
LETSENCRYPT_EMAIL=you@example.com
```

Self-host с Traefik поднимает HTTPS автоматически. Достаточно статического IP, свой домен не нужен.

#### Cloudflare Tunnel — за NAT, без открытия портов

```bash
docker compose -p selfhost -f deploy/selfhost/docker-compose.yml -f deploy/selfhost/docker-compose.tunnel.yml --env-file deploy/selfhost/.env up -d
docker compose -p selfhost logs -f cloudflared  # URL вида xxx.trycloudflare.com
```

Задайте `SERVER_DOMAIN=xxx.trycloudflare.com` в `.env` (URL меняется при перезапуске tunnel). Для стабильного адреса — Cloudflare аккаунт и named tunnel.

#### Прочие варианты

| Способ | Когда | Замечание |
|--------|-------|-----------|
| **DuckDNS** | Динамический IP | Бесплатный поддомен, клиент обновляет IP. |
| **Tailscale** | Mesh-сеть | `*.ts.net` — только между узлами Tailscale. |

---

## Без git (curl)

Если git недоступен:

```bash
curl -fsSL https://github.com/Grendgi/astralrelay.online/archive/main.tar.gz | tar xz
cd astralrelay.online-main
./install.sh
```

---

## Обновление

```bash
git pull
./deploy/selfhost/run.sh   # или ./deploy/main/run.sh для main
```

---

## HA (основной хаб)

Для отказоустойчивого основного домена: [SETUP-MAIN.md](SETUP-MAIN.md)

```bash
docker compose -p main -f deploy/main/docker-compose.yml --env-file deploy/main/.env --profile ha up -d
```

| Компонент | Роль |
|-----------|------|
| Traefik | Балансировщик :80/:443: /api, /federation → server; / → web |
| server + server2 | Две реплики API за Traefik (round-robin) |
| postgres-replica | Реплика PostgreSQL для чтения (профиль ha) |

Для продакшена: добавьте HTTPS (Let's Encrypt) в Traefik, см. [SETUP-MAIN.md](SETUP-MAIN.md).

---

## Роли: хаб и самохостинг

- **Хаб** — основной домен, через него входят пользователи без своего сервера. HA для отказоустойчивости.
- **Самохостинг** — ваш инстанс, расширяет сеть, федерация с хабом и другими.

Подробнее: [SETUP-MAIN.md](SETUP-MAIN.md), [SETUP-SELFHOST.md](SETUP-SELFHOST.md)

---

## VPN (опционально)

Самообслуживание VPN — каждый пользователь управляет своими конфигами (WireGuard, OpenVPN). Нет админов.

### Включение

1. В `.env`:
   - `VPN_ENABLED=true`
   - `VPN_WIREGUARD_SERVER_PUBLIC_KEY=` — публичный ключ WG-сервера (base64)

2. WireGuard-сервер развёрнут отдельно. Панель выдаёт конфиги; трафик обрабатывает WG.

3. Для статистики трафика: `VPN_WIREGUARD_STATS_INTERFACE=wg0` (требует `wg` на хосте).

4. Multi-node: `VPN_NODES_JSON='[{"name":"Amsterdam","region":"EU","wireguard_endpoint":"...","wireguard_server_pubkey":"...","xray_endpoint":"..."}]'`

### Xray (VMess/VLESS/Trojan)

Docker Compose поднимает контейнер Xray. Включите:

- `VPN_XRAY_ENABLED=true`
- `VPN_XRAY_ENDPOINT=host:10443` (или ваш домен:порт)
- `VPN_XRAY_API_ADDR=xray:10085` — панель добавляет пользователей в Xray при выдаче конфига

Порты Xray: 10443 (VMess), 10444 (VLESS), 10445 (Trojan) — публичные для клиентов. gRPC API (10085) доступен только внутри Docker-сети. Для разных портов в URL: `VPN_XRAY_VMESS_PORT=10443` и т.д.

### Самообслуживание

Пользователь в веб-клиенте:
- скачивает конфиг по протоколу;
- видит только свои конфиги (раздел «Мои конфиги»);
- отзывает свои конфиги.

### HA

VPN хранит состояние в PostgreSQL. При нескольких репликах server все используют общую БД — отказоустойчиво.

API: [docs/api/vpn-api.md](api/vpn-api.md)

---

## Требования

- Docker и Docker Compose
- 512 MB RAM минимум, 1 GB рекомендуется
- Порты, публикуемые по умолчанию: 3000 (web), 8080 (API), 10443–10445 (Xray VPN). PostgreSQL, Redis, MinIO — только внутри Docker.
