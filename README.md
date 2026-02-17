<p align="center">
  <img src="https://img.shields.io/badge/AstralRelay-Federated%20E2EE%20%2B%20VPN-blue?style=for-the-badge" alt="AstralRelay" />
</p>

<h1 align="center">AstralRelay.online</h1>
<p align="center">
  <strong>Федеративный E2EE-мессенджер с интегрированным VPN</strong>
</p>
<p align="center">
  Чаты с end-to-end шифрованием • Федерация как email • Xray (VLESS) для обхода блокировок
</p>

---

## ✨ Возможности

| Возможность | Описание |
|-------------|----------|
| 🔐 **E2EE** | **Signal Protocol** (если доступен) + **fallback MVP**; persistent store (IndexedDB), safety number для MITM. Личные DM — Signal/MVP, федерация — передача ciphertext |
| 🌐 **Федерация** | Децентрализованная сеть: главный хаб + self-host узлы, обмен сообщениями между доменами |
| 🔒 **VPN** | Xray (VLESS, VMess, Trojan) — пользователь скачивает конфиг в один клик |
| 📱 **Push** | Web Push уведомления (VAPID) |
| 🚀 **nip.io** | Домен по IP без покупки — `1.2.3.4.nip.io` → Let's Encrypt |
| 🕸️ **Mesh** | WireGuard mesh между серверами, coordinator (:9443), бэкапы БД на peer-узлы |

---

## 🚀 Быстрый старт

Установка в один шаг: **Docker**, **UFW** (порты), секреты и nip.io настраиваются автоматически. Скрипты в `deploy/` вручную вызывать не нужно.

### Одна команда (main или selfhost)

```bash
curl -fsSL https://raw.githubusercontent.com/Grendgi/astralrelay.online/main/bootstrap.sh | sudo sh
```

Откроется мастер: **1** — главный хаб (main), **2** — свой узел (selfhost), **3** — обновить текущую установку. Домен, mesh, VAPID — по подсказкам.

Фиксация версии: `BRANCH=v0.3.1` или `EXPECTED_COMMIT=abc123`.

### Только self-host

Если нужен только свой инстанс (без выбора main/selfhost в мастере):

```bash
git clone https://github.com/Grendgi/astralrelay.online.git
cd astralrelay.online
sudo ./install-selfhost.sh
```

### Универсальный install.sh (из клона)

```bash
git clone https://github.com/Grendgi/astralrelay.online.git
cd astralrelay.online
sudo ./install.sh
```

Тот же мастер, что и в bootstrap: выбор main/selfhost, домен, подключение к mesh (coordinator на main), standalone/subdomain.

### Без TTY (CI / автоматический режим)

```bash
INSTALL_AUTO=1 INSTALL_MODE=selfhost sudo ./install.sh
# или INSTALL_MODE=main для главного хаба
```

### Обновление на своих серверах

Повторный запуск bootstrap → в мастере **3** (обновить текущую установку), либо после `git pull`: `sudo ./scripts/update.sh`. Подробнее: [docs/DOWNLOAD-AND-UPDATE.md](docs/DOWNLOAD-AND-UPDATE.md).

---

## 📐 Архитектура

```
                    [astralrelay.online] — главный хаб
                               │
        ┌──────────────────────┼──────────────────────┐
        │                      │                      │
   [Traefik :80/443]    [Coordinator :9443]    [server] [server2]
        │                      │                      │
        └──────────────────────┼──────────────────────┘
                               │
                     [PostgreSQL] [Redis] [MinIO] [Xray]
                               │
               ────────────────┼─────────────── федерация
                               │
    [self-host A]      [self-host B]      [self-host C]
     nip.io/домен       nip.io/домен       subdomain*
```

\* Subdomain главного: например `1-2-3-4.astralrelay.online`.

**Cloudflare:** если домен main за прокси (оранжевое облако), порт **9443** (coordinator) снаружи недоступен. Нужна запись с **DNS only** (серое облако) или отдельная A-запись на IP сервера — иначе selfhost не получит join token автоматически. Подробнее: [RUN-MAIN](docs/RUN-MAIN.md), [RUN-SELFHOST](docs/RUN-SELFHOST.md).

---

## 🛠 Технологии

| Компонент | Стек |
|-----------|------|
| **Backend** | Go 1.23, Chi, JWT, PostgreSQL, Redis |
| **Frontend** | React 18, TypeScript, Vite |
| **E2EE** | Signal Protocol (X3DH + Double Ratchet) или MVP (X25519 + NaCl) |
| **VPN** | Xray-core (VLESS, VMess, Trojan) |
| **Инфра** | Docker, Traefik, Let's Encrypt, UFW |

---

## 📚 Документация

| Раздел | Описание |
|--------|----------|
| [Запуск Main (hub)](docs/RUN-MAIN.md) | Быстрый старт main, порты, 404/502, Cloudflare, mesh |
| [Запуск Self-host](docs/RUN-SELFHOST.md) | Быстрый старт selfhost, mesh, JOIN_TOKEN, Cloudflare |
| [Главный сервер (детально)](docs/SETUP-MAIN.md) | Traefik, HA, Let's Encrypt |
| [Self-host (детально)](docs/SETUP-SELFHOST.md) | nip.io, Cloudflare Tunnel |
| [VPN Mesh + бэкапы](docs/MESH-AND-BACKUP.md) | Coordinator, WireGuard mesh, mTLS, pg_dump |
| [Subdomain главного](docs/SUBDOMAIN-MODE.md) | 1-2-3-4.astralrelay.online |
| [Self-Hosting обзор](docs/SELF-HOSTING.md) | Роли, nip.io, HA, федерация |
| [Защита федерации](docs/FEDERATION-SECURITY.md) | Rate limit, blocklist, mTLS, webhook-алерты |
| [E2EE: Signal vs MVP](docs/E2EE.md) | Режимы шифрования, IndexedDB |
| [Усиление безопасности](docs/SECURITY-HARDENING.md) | JWT, ws_token, ревокация, Traefik |
| [WAF](docs/WAF.md) | Traefik/CrowdSec/ModSecurity |
| [Dev-режим](docs/RUN-DEV.md) | Локальная разработка |
| [Архитектура](docs/architecture.md) | Схема, компоненты, потоки |
| [Полная документация](docs/README.md) | Протокол, API, глоссарий |

---

## 📁 Структура проекта

```
astralrelay.online/
├── bootstrap.sh         # Установка в один клик (curl | sh)
├── install.sh           # Универсальный мастер: main / selfhost / update
├── install-selfhost.sh  # Только self-host
├── deploy/
│   ├── main/            # Hub: Traefik, server, coordinator (:9443), mesh
│   ├── selfhost/        # Self-host: nip.io, домен, mesh
│   └── dev/             # Разработка
├── server/              # Go backend (API, федерация, VPN)
├── web/                 # React frontend
├── xray/                # Xray VPN конфигурация
├── mesh/                # Coordinator, backup-receiver (Go)
├── scripts/             # setup-mesh, backup-to-peers, smoke-test, clean-rebuild, fmt, lint
└── docs/                # Документация
```

---

## 🔧 Перезапуск и ручной запуск

Для первичной установки достаточно `bootstrap.sh` или `install.sh`. Скрипты в `deploy/` нужны для перезапуска или ручного управления:

```bash
# Main (из корня репозитория)
./deploy/main/run.sh

# Self-host
./deploy/selfhost/run.sh

# Dev
make dev
# или ./deploy/dev/run.sh
```

Подготовка сервера (только Docker, без установки приложения): `sudo ./deploy/setup-server.sh`.  
Полезные команды: `make fmt`, `make lint`, `make migrate`, `make build`, `make clean`.

---

## 🧪 Проверка

```bash
./scripts/smoke-test.sh https://YOUR_DOMAIN
```

---

## 📋 Требования

| Роль | RAM | Диск | Порты (открываются UFW автоматически) |
|------|-----|------|--------------------------------------|
| **Main** | 2 GB | 10 GB | 22, 80, 443, 8082 (Traefik), **9443** (coordinator) |
| **Self-host** | 1 GB | 5 GB | 22, 80, 443, 3000, 8080, 51820/udp, 9100 |

ОС: Linux (рекомендуется Ubuntu/Debian — для автоустановки Docker и UFW).

**Docker Hub:** при нескольких установках подряд возможен rate limit для неавторизованных pull. Решение: `sudo docker login` или повторить установку через несколько часов.

---

## 🤝 Участие в сети

- **Главный хаб (main)** — основной домен, точка входа, coordinator для mesh.
- **Self-host** — ваш инстанс расширяет федерацию, свои данные, свой контроль.

Подробнее: [docs/SELF-HOSTING.md](docs/SELF-HOSTING.md)

---

<p align="center">
  <sub>Децентрализованная сеть без единой точки отказа</sub>
</p>
