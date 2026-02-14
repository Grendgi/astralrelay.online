# Конфигурации развёртывания

## Автоматическая установка (рекомендуется)

**Только self-host** (main один, домен по умолчанию astralrelay.online):
```bash
./install-selfhost.sh
```

**Main или self-host** (универсальный):
```bash
./install.sh
```

Интерактивно спросит: режим (main/selfhost), домен или IP. При вводе IP — подставит `IP.nip.io`. Генерирует секреты, создаёт `.env`, запускает.

**Main** — hub с Traefik, HA (server+server2).  
**Self-host** — свой инстанс; с nip.io/доменом — Traefik + HTTPS.

## Настройка сервера (Docker не установлен)

```bash
sudo ./deploy/setup-server.sh
```

Подробнее: [SETUP-SERVER.md](SETUP-SERVER.md)

---

## Пресеты (каждый полностью в своей директории)

| Директория | Назначение | Содержимое |
|------------|------------|------------|
| [main/](main/) | Hub — Traefik, HA, Let's Encrypt | docker-compose, traefik/, postgres/, run.sh, .env.example |
| [selfhost/](selfhost/) | Свой инстанс — nip.io или домен | docker-compose, traefik/, run.sh, .env.example |
| [dev/](dev/) | Разработка — порты БД открыты | docker-compose, run.sh, .env.example |

Каждый пресет самодостаточен, можно дорабатывать независимо.

## Запуск

Из корня проекта:

```bash
./install.sh              # Первый запуск (создаёт .env)
./deploy/main/run.sh      # Main hub
./deploy/selfhost/run.sh  # Self-host
./deploy/dev/run.sh       # Разработка
```

Self-host с `SERVER_DOMAIN=*.nip.io` или своим доменом — автоматически поднимает Traefik и HTTPS.
