# Запуск Main (hub) сервера

Подробно: [SETUP-MAIN.md](SETUP-MAIN.md)

## Быстрый старт

```bash
./install.sh   # выбрать main
# или вручную:
cp deploy/main/.env.example deploy/main/.env
# Заполните: SERVER_DOMAIN, JWT_SECRET, пароли, LETSENCRYPT_EMAIL
./deploy/main/run.sh
```

## Что поднимается

| Компонент       | Роль                                              |
|-----------------|---------------------------------------------------|
| Traefik         | Балансировщик :80, dashboard :8082                |
| server         | API (единственная реплика по умолчанию)           |
| server2        | Вторая реплика API (профиль ha, round-robin)      |
| web             | Веб-клиент                                        |
| postgres        | Primary с wal_level=replica                       |
| postgres-replica| Реплика для чтения (профиль ha)                   |

## Перед запуском

1. `.env`: `SERVER_DOMAIN`, `JWT_SECRET`, `DB_ENCRYPTION_KEY`, пароли postgres/minio.
2. DNS: A/AAAA запись на IP сервера.
3. HTTPS: Let's Encrypt настраивается в Traefik через LETSENCRYPT_EMAIL (см. [SETUP-MAIN.md](SETUP-MAIN.md)).

## Устранение 404/502

- Убедитесь, что подняты **main-web-1** и **main-server-1**: `docker ps`.
- Если **main-server-1** или **main-web-1** в состоянии Restarting/Exit: `docker logs main-server-1`, `docker logs main-web-1`.
- Проверьте, что DNS для `SERVER_DOMAIN` указывает на IP сервера.
- Перезапуск: из корня репозитория `docker compose -p main -f deploy/main/docker-compose.yml --env-file deploy/main/.env up -d --build`.

## Профиль ha (реплика PostgreSQL)

```bash
docker compose -p main -f deploy/main/docker-compose.yml --env-file deploy/main/.env --profile ha up -d
```

