# Запуск Main (hub) сервера

Подробно: [SETUP-MAIN.md](SETUP-MAIN.md)

## Быстрый старт

Достаточно одного скрипта (Docker, UFW, порты настраиваются автоматически):

```bash
curl -fsSL https://raw.githubusercontent.com/Grendgi/astralrelay.online/main/bootstrap.sh | sudo sh
# в мастере выбрать 1 (main), ввести домен
```

Или из клона репозитория: `sudo ./install.sh` → выбрать main. Скрипты в `deploy/` вручную вызывать не нужно.

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
4. **Mesh (подключение selfhost-узлов):** Coordinator слушает порт **9443** (отдельный контейнер, не Traefik). В `docker-compose.mesh.yml` порт уже проброшен (`9443:9443`). Нужно разрешить входящий **9443/TCP** в фаерволе сервера (ufw, iptables) и в security group облака. Токен: `curl -s http://localhost:9443/v1/token` на MAIN или `curl -s http://ВАШ_ДОМЕН:9443/v1/token` снаружи.  
   **Если домен за Cloudflare:** порт 9443 через прокси (оранжевое облако) не поддерживается. Включите для записи **DNS only** (серое облако) или создайте отдельную A-запись на IP сервера с серым облаком (например `mesh.astralrelay.online`), иначе selfhost не получит токен автоматически.

## Устранение 404/502

- Убедитесь, что подняты **main-web-1** и **main-server-1**: `docker ps`.
- Если **main-server-1** или **main-web-1** в состоянии Restarting/Exit: `docker logs main-server-1`, `docker logs main-web-1`.
- Проверьте, что DNS для `SERVER_DOMAIN` указывает на IP сервера.
- Перезапуск: снова `sudo ./install.sh` и выбрать 3 (обновить), или из корня репозитория `docker compose -p main -f deploy/main/docker-compose.yml -f deploy/main/docker-compose.mesh.yml --env-file deploy/main/.env up -d --build`.

### main-server падает: «invalid port» после host

Пароль PostgreSQL содержит символы `/` или `+`, из‑за чего ломается URL в `DATABASE_URL`. Замените пароль на безопасный (только буквы/цифры, без `+` и `/`):

1. Задайте новый пароль в `.env`: `POSTGRES_PASSWORD=новый_пароль_только_буквы_цифры`
2. Обновите пароль в Postgres и перезапустите server:
   ```bash
   docker exec -it main-postgres-1 psql -U messenger -d messenger -c "ALTER USER messenger PASSWORD 'новый_пароль_только_буквы_цифры';"
   docker restart main-server-1
   ```
   (подставьте тот же пароль, что в шаге 1)

## Профиль ha (реплика PostgreSQL)

```bash
docker compose -p main -f deploy/main/docker-compose.yml --env-file deploy/main/.env --profile ha up -d
```

