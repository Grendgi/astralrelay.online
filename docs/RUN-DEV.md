# Запуск в режиме Dev

Режим разработки и тестирования. Публикует порты postgres, redis, minio для отладки.

## Быстрый старт

```bash
./deploy/dev/run.sh
```

## Что поднимается

| Сервис   | Порт          | Назначение                    |
|----------|---------------|-------------------------------|
| web      | 3000          | Веб-клиент                    |
| server   | 8080          | API                           |
| postgres | 5432          | БД (публикуется для отладки)  |
| redis    | 6379          | Кеш (публикуется)             |
| minio    | 9000, 9001    | S3 (публикуется)              |
| xray     | 10443–10445   | VPN                           |

## Локальная разработка (сервер на хосте)

```bash
# Поднять только инфраструктуру
docker compose -p dev -f deploy/dev/docker-compose.yml --env-file deploy/dev/.env up -d postgres redis minio

# Сервер и веб локально
cd server && go run ./cmd/server
cd web    && npm install && npm run dev
```

## Важно

- **Не используйте в продакшене** — порты БД доступны снаружи.
- Для продакшена: [RUN-MAIN.md](RUN-MAIN.md) или [RUN-SELFHOST.md](RUN-SELFHOST.md).

