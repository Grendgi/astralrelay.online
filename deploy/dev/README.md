# DEV — Разработка и тестирование

Всё для dev в этой директории. Не зависит от deploy/main и deploy/selfhost.

Локальная разработка. Публикует порты postgres, redis, minio, xray.

## Запуск

```bash
# Из корня проекта
./deploy/dev/run.sh
```

Или:

```bash
cp deploy/dev/.env.example deploy/dev/.env
# отредактируйте .env при необходимости
docker compose -f deploy/dev/docker-compose.yml up -d
```

## Порты

| Сервис   | Порт       |
|----------|------------|
| web      | 3000       |
| server   | 8080       |
| postgres | 5432       |
| redis    | 6379       |
| minio    | 9000, 9001 |
| xray     | 10443–10445 |

**Не используйте в продакшене** — порты БД доступны снаружи.
