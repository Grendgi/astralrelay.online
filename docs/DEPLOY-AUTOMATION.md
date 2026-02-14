# Автоматизация установки и масштабирования

## Обзор

- **install-selfhost.sh** — только self-host (main один, домен astralrelay.online по умолчанию)
- **install.sh** — main hub или self-host. Поддерживается nip.io/sslip.io — домен по IP без покупки

## Цепочка установки

```
./install.sh
    │
    ├─ Режим: main (hub) | selfhost
    ├─ Домен: свой домен | IP (→ IP.nip.io) | localhost
    ├─ LETSENCRYPT_EMAIL (при домене/nip.io)
    │
    ├─ Генерирует: JWT_SECRET, POSTGRES_PASSWORD, MINIO_ROOT_PASSWORD, DB_ENCRYPTION_KEY
    ├─ Создаёт: deploy/{main|selfhost}/.env
    ├─ Traefik: deploy/{main|selfhost}/traefik/traefik.yml (из шаблона)
    │
    └─ Запуск: docker compose up -d --build
```

## Main (hub)

- Traefik на 80/443
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

## Неинтерактивный режим

```bash
INSTALL_MODE=selfhost SERVER_DOMAIN=1.2.3.4.nip.io ./install.sh
```

Секреты и LETSENCRYPT_EMAIL нужно задать в .env до запуска или они будут сгенерированы/пусты.
