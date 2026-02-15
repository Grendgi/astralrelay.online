# Changelog документации

Все значимые изменения протокола и API документируются здесь.

Формат основан на [Keep a Changelog](https://keepachangelog.com/ru/1.0.0/).

---

## [0.2.1] — 2025-02-16

### Изменено

- **Развёртывание:** единый вход через `bootstrap.sh` (curl \| sudo sh); RUN-MAIN.md и RUN-SELFHOST.md вынесены в быстрый старт; в индекс добавлены ссылки на них.
- **Coordinator:** на main всегда поднимается (порт 9443), без MESH_ENABLED; учтено в SETUP-MAIN, MESH-AND-BACKUP, architecture.
- **UFW:** установка и открытие портов (22, 80, 443, 8082, 9443 для main; 3000, 8080, 51820/udp, 9100 для selfhost при mesh) описаны как автоматические в install.sh/bootstrap.
- **Cloudflare:** в RUN-MAIN, RUN-SELFHOST, SETUP-MAIN, MESH-AND-BACKUP добавлено пояснение: для доступа к 9443 нужна запись DNS only (серое облако) или A на IP.
- **Selfhost mesh:** COORDINATOR_URL, MESH_JOIN_TOKEN в .env; опциональный scripts/setup-mesh.sh; обновлены SETUP-SELFHOST, RUN-SELFHOST, SELF-HOSTING.
- **DEPLOY-AUTOMATION:** bootstrap, UFW, INSTALL_AUTO=1, coordinator всегда.
- **CLEAN-REBUILD, TESTING:** учтён docker-compose.mesh.yml для main. architecture: схема с Coordinator, ссылки на RUN-MAIN/RUN-SELFHOST.

---

## [0.2] — 2025-02-14

### Добавлено

- Защита федерации: rate limit, allowlist/blocklist, main_only режим
- mTLS: выдача клиентских сертификатов в coordinator при mesh join (`POST /v1/cert`)
- Webhook-алерты при rate limit и blocklist (`FEDERATION_ALERT_WEBHOOK_URL`)
- DB-пользователь для федерации (миграция 015, `DATABASE_FEDERATION_URL`)
- Prometheus-метрики, JSON-логирование, federationRecover
- WAF: документация по Traefik/CrowdSec/ModSecurity
- Обновление install.sh и install-selfhost.sh: авто mTLS при mesh join

---

## [0.1] — 2025-02-13

### Добавлено

- Архитектура системы (architecture.md)
- Глоссарий (glossary.md)
- Спецификация форматов сообщений (01-message-formats.md)
- Модель ключей X3DH + Double Ratchet (02-key-model.md)
- C2S API: Auth, Key Directory, Message Relay, Media
- S2S API: Federation, транзакции, media proxy
- Privacy-by-Design (privacy.md)

### MVP Scope

- Чаты 1:1 + Файлы
- 1 устройство на аккаунт
- Федерация по модели email/Matrix
- E2EE без ключей на сервере

### VPN Panel (2025-02)

- Самообслуживание: пользователь скачивает и отзывает свои конфиги
- Только свои конфиги (изоляция по user_id)
- Multi-node, выбор ноды
- HA: состояние в PostgreSQL, отказоустойчиво при нескольких репликах
