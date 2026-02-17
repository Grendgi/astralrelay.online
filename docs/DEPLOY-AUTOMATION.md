# Автоматизация установки и масштабирования

## Обзор

- **bootstrap.sh** — установка в один клик: `curl -fsSL .../bootstrap.sh | sudo sh`. Мастер: main (1), selfhost (2), обновление (3). Рекомендуемый способ первичной установки.
- **install.sh** — универсальный мастер (main / selfhost / update). Вызывается из bootstrap или из клона репозитория. Настраивает Docker, UFW (порты по роли), секреты, coordinator на main.
- **install-selfhost.sh** — только self-host (без выбора main/selfhost), домен главного по умолчанию `astralrelay.online`.
- **scripts/update.sh** — обновление при установке через git: подтянуть код и перезапустить (вызов `install.sh --action update` с автоопределением main/selfhost). Подробнее: [DOWNLOAD-AND-UPDATE.md](DOWNLOAD-AND-UPDATE.md).

Поддерживается nip.io/sslip.io — домен по IP без покупки.

## Цепочка установки

```
bootstrap.sh  или  install.sh
    │
    ├─ Режим: 1 main | 2 selfhost | 3 update
    ├─ Домен: свой домен | IP (→ IP.nip.io) | localhost
    ├─ LETSENCRYPT_EMAIL (при домене/nip.io)
    ├─ Mesh: подключиться к coordinator (selfhost) — COORDINATOR_URL, MESH_JOIN_TOKEN в .env
    │
    ├─ Docker: при необходимости устанавливается
    ├─ UFW: порты по роли (main: 80, 443, 8082, 9443; selfhost: 80, 443, 3000, 8080, 51820/udp, 9100 при mesh)
    ├─ Генерирует: JWT_SECRET, POSTGRES_PASSWORD, MINIO_ROOT_PASSWORD, DB_ENCRYPTION_KEY (url-safe при необходимости)
    ├─ Создаёт: deploy/{main|selfhost}/.env
    ├─ Traefik: deploy/{main|selfhost}/traefik/traefik.yml (из шаблона)
    │
    └─ Запуск: docker compose up -d --build
```

## Main (hub)

- Traefik на 80/443, dashboard 8082
- **Coordinator** на **9443** (всегда, через docker-compose.mesh.yml) — токен для mesh: `curl http://DOMAIN:9443/v1/token`
- server + server2 (балансировка)
- Let's Encrypt при заданном LETSENCRYPT_EMAIL
- Профиль `ha` для postgres-replica: `docker compose --profile ha up -d`

## Self-host

- **localhost** — прямые порты 3000 (web), 8080 (API)
- **nip.io / домен** — Traefik + HTTPS, порты 80/443

Файлы:
- `deploy/selfhost/docker-compose.yml` — база
- `deploy/selfhost/docker-compose.traefik.yml` — overlay при домене

## nip.io

Введённый IP (напр. `1.2.3.4`) автоматически превращается в `1.2.3.4.nip.io`. Нужен только статический IP и порты 80/443.

## Масштабирование

| Цель | Действие |
|------|----------|
| + реплика API | Main уже имеет server+server2 |
| + реплика PostgreSQL | `docker compose --profile ha up -d` (deploy/main) |
| Новый инстанс | Клонировать репо, `./install.sh` (selfhost) |
| Переход main → HA | Добавить профиль ha в run.sh или вручную |

## Неинтерактивный режим (без TTY)

```bash
INSTALL_AUTO=1 INSTALL_MODE=selfhost sudo ./install.sh
# или INSTALL_MODE=main для главного хаба
```

Можно задать `SERVER_DOMAIN`, `COORDINATOR_URL` и др. Секреты генерируются автоматически, если не заданы в `.env`.
