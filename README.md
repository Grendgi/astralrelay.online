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
| 🕸️ **Mesh** | WireGuard mesh между серверами, бэкапы БД на peer-узлы |

---

## 🚀 Быстрый старт

### Вариант 1: Одна команда (zero config)

```bash
curl -fsSL https://raw.githubusercontent.com/Grendgi/astralrelay.online/main/bootstrap.sh | sudo sh
```

Docker, секреты, IP→nip.io, VAPID — всё автоматически. Без вопросов. Для фиксации версии: `BRANCH=v0.3.1` или `EXPECTED_SHA256=...`.

---

### Вариант 2: Только self-host (рекомендуется)

Для своего инстанса — домен главного по умолчанию `astralrelay.online`:

```bash
git clone https://github.com/Grendgi/astralrelay.online.git
cd astralrelay.online
sudo ./install-selfhost.sh
```

---

### Вариант 3: Main или self-host

Универсальный скрипт — выбор режима (hub или свой инстанс):

```bash
git clone https://github.com/Grendgi/astralrelay.online.git
cd astralrelay.online
sudo ./install.sh
```

Интерактивно: домен/IP, email для Let's Encrypt, mesh. Секреты генерируются автоматически.

---

### Автоматический режим (CI / без TTY)

```bash
INSTALL_AUTO=1 INSTALL_MODE=selfhost sudo ./install.sh
```

---

## 📐 Архитектура

```
                    [astralrelay.online] — главный хаб
                               │
               ┌───────────────┼───────────────┐
               │               │               │
          [Traefik]       [server 1]      [server 2]
               │               │               │
               └───────────────┼───────────────┘
                               │
                     [PostgreSQL] [Redis] [MinIO]
                               │
               ────────────────┼─────────────── федерация
                               │
    [self-host A]      [self-host B]      [self-host C]
     nip.io/домен       nip.io/домен       subdomain*
```

\* Subdomain главного (1-2-3-4.astralrelay.online)

---

## 🛠 Технологии

| Компонент | Стек |
|-----------|------|
| **Backend** | Go 1.23, Chi, JWT, PostgreSQL, Redis |
| **Frontend** | React 18, TypeScript, Vite |
| **E2EE** | Signal Protocol (X3DH + Double Ratchet) или MVP (X25519 + NaCl) |
| **VPN** | Xray-core (VLESS, VMess, Trojan) |
| **Инфра** | Docker, Traefik, Let's Encrypt |

---

## 📚 Документация

| Раздел | Описание |
|--------|----------|
| [Главный сервер (Hub)](docs/SETUP-MAIN.md) | Traefik, HA (server+server2), Let's Encrypt |
| [Self-host](docs/SETUP-SELFHOST.md) | Свой инстанс, nip.io, Cloudflare Tunnel |
| [VPN Mesh + бэкапы](docs/MESH-AND-BACKUP.md) | Coordinator, WireGuard mesh, mTLS, pg_dump |
| [Subdomain главного](docs/SUBDOMAIN-MODE.md) | 1-2-3-4.astralrelay.online |
| [Self-Hosting обзор](docs/SELF-HOSTING.md) | Роли, nip.io, HA, федерация |
| [Защита федерации](docs/FEDERATION-SECURITY.md) | Rate limit, blocklist, mTLS, webhook-алерты |
| [E2EE: Signal vs MVP](docs/E2EE.md) | Режимы шифрования, проверка Signal, IndexedDB |
| [Усиление безопасности](docs/SECURITY-HARDENING.md) | JWT (iss/aud/typ, HS256), ws_token, ревокация, Traefik |
| [WAF](docs/WAF.md) | Traefik/CrowdSec/ModSecurity |
| [Dev-режим](docs/RUN-DEV.md) | Локальная разработка |
| [Архитектура](docs/architecture.md) | Схема, компоненты, потоки |
| [Полная документация](docs/README.md) | Протокол, API, глоссарий |

---

## 📁 Структура проекта

```
astralrelay.online/
├── deploy/
│   ├── main/           # Hub: Traefik, server+server2, HA
│   ├── selfhost/       # Self-host: nip.io, домен, mesh
│   └── dev/            # Разработка
├── server/             # Go backend (API, федерация, VPN)
├── web/                # React frontend
├── xray/               # Xray VPN конфигурация
├── mesh/               # Coordinator, backup-receiver (Go)
├── scripts/            # setup-mesh, backup-to-peers, smoke-test, clean-rebuild, fmt, lint
├── install.sh          # Универсальная установка (main/selfhost)
├── install-selfhost.sh # Только self-host
└── docs/               # Документация
```

---

## 🔧 Ручной запуск

```bash
# Подготовка сервера (Docker)
sudo ./deploy/setup-server.sh

# Main hub
./deploy/main/run.sh

# Self-host
./deploy/selfhost/run.sh

# Dev (Makefile)
make dev
```

Или напрямую: `./deploy/dev/run.sh`. Полезные команды: `make fmt`, `make lint`, `make migrate`, `make build`, `make clean`.

---

## 🧪 Проверка

```bash
./scripts/smoke-test.sh https://YOUR_DOMAIN
```

---

## 📋 Требования

| Роль | RAM | Диск | Порты |
|------|-----|------|-------|
| **Main** | 2 GB | 10 GB | 80, 443, 8082 |
| **Self-host** | 1 GB | 5 GB | 80, 443 или 3000, 8080 |

ОС: Linux (Ubuntu, Debian, Alpine, CentOS)

---

## 🤝 Участие в сети

- **Главный хаб** — основной домен, точка входа для пользователей
- **Self-host** — ваш инстанс расширяет федерацию, свои данные, свой контроль

Подробнее: [docs/SELF-HOSTING.md](docs/SELF-HOSTING.md)

---

<p align="center">
  <sub>Децентрализованная сеть без единой точки отказа</sub>
</p>
