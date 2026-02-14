# Полная инструкция: Self-Host (свой инстанс)

Развёртывание своего инстанса расширяет федеративную сеть. Без домена — используйте nip.io по IP. Чат, VPN, федерация с главным сервером и другими self-host.

---

## 1. Зачем разворачивать self-host?

- **Децентрализация** — нет единой точки отказа
- **Приватность** — свои данные на своём сервере
- **VPN** — Xray (VLESS) для обхода блокировок
- **Расширение сети** — каждый инстанс укрепляет федерацию

---

## 2. Требования

| Требование | Минимум |
|------------|---------|
| ОС | Linux (Ubuntu, Debian, Alpine) |
| RAM | 1 GB |
| Диск | 5 GB |
| Сеть | Статический IP или nip.io |
| Порты | 80, 443 (при домене/nip.io) или 3000, 8080 (локально) |

**Без своего домена:** nip.io — введите IP, получите `1.2.3.4.nip.io`. Let's Encrypt выдаст сертификат.

---

## 3. Подготовка

### 3.1 Docker

```bash
sudo ./deploy/setup-server.sh
```

Или: https://docs.docker.com/engine/install/

### 3.2 Порты

**С доменом/nip.io (Traefik):**
```bash
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable
```

**Только локально (localhost):**
```bash
sudo ufw allow 3000/tcp
sudo ufw allow 8080/tcp
sudo ufw enable
```

---

## 4. Развёртывание

### 4.1 Только self-host (рекомендуется)

Скрипт `install-selfhost.sh` — без выбора main/selfhost, домен главного по умолчанию `astralrelay.online`:

```bash
git clone https://github.com/Grendgi/astralrelay.online.git
cd astralrelay.online
sudo ./install-selfhost.sh
```

### 4.2 Одна команда (zero config)

Пользователь ничего не настраивает — всё автоматически:

```bash
curl -fsSL https://raw.githubusercontent.com/Grendgi/astralrelay.online/main/bootstrap.sh | sudo sh
```

Или из репозитория в авто-режиме:

```bash
git clone https://github.com/Grendgi/astralrelay.online.git && cd astralrelay.online
INSTALL_AUTO=1 sudo ./install.sh
```

**Автоматически:** Docker, IP→nip.io, все секреты, VAPID, Traefik. Без вопросов.

### 4.3 Универсальный install.sh

```bash
sudo ./install.sh
```

Выберите режим (main/selfhost), домен/IP, email. Секреты генерируются автоматически.

### 4.4 Ручная настройка

```bash
cp deploy/selfhost/.env.example deploy/selfhost/.env
nano deploy/selfhost/.env
```

**Обязательно:**

| Переменная | Пример | Описание |
|------------|--------|----------|
| `SERVER_DOMAIN` | `1.2.3.4.nip.io` | Домен или IP.nip.io |
| `JWT_SECRET` | (openssl rand -base64 32) | Секрет JWT |
| `POSTGRES_PASSWORD` | — | Пароль БД |
| `MINIO_ROOT_PASSWORD` | — | Пароль MinIO |

**Для HTTPS (nip.io или домен):**
```
LETSENCRYPT_EMAIL=you@example.com
```

**Для VPN (Xray):**
```
VPN_XRAY_ENABLED=true
VPN_XRAY_ENDPOINT=1.2.3.4.nip.io:10444
```

Запуск:
```bash
./deploy/selfhost/run.sh
```

---

## 5. Режимы работы

| SERVER_DOMAIN | Доступ | HTTPS |
|---------------|--------|-------|
| `localhost` | http://localhost:3000, :8080 | Нет |
| `1.2.3.4.nip.io` | https://1.2.3.4.nip.io | Let's Encrypt |
| `mydomain.com` | https://mydomain.com | Let's Encrypt |

При домене/nip.io `run.sh` автоматически поднимает Traefik.

---

## 6. За NAT (Cloudflare Tunnel)

Если порты 80/443 закрыты (за роутером):

```bash
docker compose -p selfhost -f deploy/selfhost/docker-compose.yml -f deploy/selfhost/docker-compose.tunnel.yml --env-file deploy/selfhost/.env up -d
docker compose -p selfhost logs -f cloudflared
```

URL вида `xxx.trycloudflare.com` появится в логах. Задайте в `.env`:
```
SERVER_DOMAIN=xxx.trycloudflare.com
```
Лимит quick tunnel: ~200 одновременных запросов. Для стабильного URL — Cloudflare аккаунт и named tunnel.

---

## 7. Проверка

```bash
./scripts/smoke-test.sh https://YOUR_DOMAIN
# или
./scripts/smoke-test.sh http://localhost:8080
```

- **Federation:** https://YOUR_DOMAIN/.well-known/federation
- **Чат:** зарегистрируйтесь и напишите пользователю с другого инстанса

---

## 8. Обновление

```bash
git pull
./deploy/selfhost/run.sh
```

---

## 9. Федерация

Ваш инстанс автоматически участвует в федерации. Другие серверы находят вас по:
```
https://YOUR_DOMAIN/.well-known/federation
```

Пользователи с главного сервера и других self-host могут общаться с `@user:YOUR_DOMAIN`.

**main_only** (по умолчанию): общение только через главный хаб. **mTLS**: при mesh join `setup-mesh.sh` получает клиентский сертификат и записывает `FEDERATION_MTLS_CLIENT_CERT/KEY` в `.env`. Защита федерации: [FEDERATION-SECURITY.md](FEDERATION-SECURITY.md).

---

## 10. VPN (Xray)

В `.env`:
```
VPN_XRAY_ENABLED=true
VPN_XRAY_ENDPOINT=1.2.3.4.nip.io:10444
VPN_XRAY_API_ADDR=xray:10085
```

Порты 10443 (VMess), 10444 (VLESS), 10445 (Trojan) должны быть открыты, если Xray снаружи. При Traefik: Xray на отдельных портах, не через 80/443.

---

## 11. Безотказность системы

- **Главный сервер** — 2 реплики API, балансировка
- **Self-host** — независимые узлы, при падении одного остальные работают
- **Федерация** — p2p между доменами, нет центрального роутера
- **VPN** — распределён по инстансам, обход блокировок

Чем больше self-host — тем устойчивее сеть.
