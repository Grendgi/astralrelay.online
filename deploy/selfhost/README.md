# SELF-HOST — Свой инстанс

Всё для selfhost в этой директории. Не зависит от deploy/main и deploy/dev.

Один сервер, федерация, VPN. Без своего домена — nip.io по IP.

## Структура

```
deploy/selfhost/
├── docker-compose.yml
├── docker-compose.traefik.yml   # Overlay при домене/nip.io
├── docker-compose.mesh.yml      # Overlay для mesh (backup-receiver)
├── federation/                  # mTLS-сертификаты (создаётся setup-mesh.sh)
├── traefik/traefik.yml.tpl
├── .env.example, run.sh
└── README.md
```

## Быстрый старт

```bash
./install.sh   # выбрать selfhost, ввести IP или домен
# или
./deploy/selfhost/run.sh   # после создания .env
```

## Режимы

| SERVER_DOMAIN | Трафик | HTTPS |
|---------------|--------|-------|
| localhost | Прямые порты 3000, 8080 | нет |
| IP.nip.io или домен | Traefik 80/443 | Let's Encrypt |

При домене/nip.io `run.sh` поднимает Traefik и HTTPS автоматически.

## Порты

| Сервис | localhost | с Traefik |
|--------|-----------|-----------|
| web | 3000 | 80/443 |
| server | 8080 | 80/443 (/api) |
| xray | 10443–10445 | 10443–10445 |
| traefik | — | 80, 443, 8082 |

## nip.io

```bash
# В .env или при install.sh введите IP:
SERVER_DOMAIN=1.2.3.4.nip.io
LETSENCRYPT_EMAIL=you@example.com
```

DNS уже настроен, Let's Encrypt выдаст сертификат.

## Mesh и mTLS

При mesh (домен главного в install) `setup-mesh.sh` регистрирует узел в coordinator. С `JOIN_TOKEN` скрипт запрашивает mTLS-сертификат и сохраняет в `federation/`. Сервер монтирует `./federation:/etc/messenger/federation`.
