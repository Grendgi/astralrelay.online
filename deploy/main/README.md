# MAIN — Hub-сервер

Всё для main в этой директории. Не зависит от deploy/selfhost и deploy/dev.

## Структура

```
deploy/main/
├── docker-compose.yml   # Traefik, server, server2, postgres, web, xray
├── docker-compose.mesh.yml  # Coordinator, backup-receiver (при MESH_ENABLED)
├── mesh-ca/             # CA для mTLS (создаётся install.sh при mesh)
├── traefik/
│   ├── traefik.yml.tpl  # Шаблон (LETSENCRYPT_EMAIL)
│   └── traefik.yml      # Генерируется run.sh
├── postgres/
│   ├── 01-replicator.sql
│   ├── 02-pg-hba-append.sh
│   ├── replica-entrypoint.sh
│   └── Dockerfile.replica
├── .env.example
├── run.sh
└── README.md
```

## Запуск

```bash
# Из корня проекта
./deploy/main/run.sh
# или
./install.sh  # выбрать main
```

Создайте `.env` из `.env.example`. Обязательно: `SERVER_DOMAIN`, `JWT_SECRET`, `POSTGRES_PASSWORD`, `MINIO_ROOT_PASSWORD`.

## HA (реплика PostgreSQL)

```bash
docker compose -p main -f deploy/main/docker-compose.yml --env-file deploy/main/.env --profile ha up -d
```

## HTTPS, DNS, Push

- **HTTPS:** `LETSENCRYPT_EMAIL` в `.env` — run.sh генерирует traefik.yml
- **DNS:** A-запись `SERVER_DOMAIN` на IP (для nip.io уже резолвится)
- **Push:** VAPID в `.env`, см. [docs/PUSH-VAPID.md](../../docs/PUSH-VAPID.md)
