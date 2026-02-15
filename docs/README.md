# Документация проекта

Федеративный E2EE-мессенджер с pluggable VPN модулем.

## Структура документации

### Развёртывание

| Документ | Описание |
|----------|----------|
| [RUN-MAIN](./RUN-MAIN.md) | **Быстрый старт Main** — bootstrap/install, порты, Cloudflare, mesh |
| [RUN-SELFHOST](./RUN-SELFHOST.md) | **Быстрый старт Self-host** — bootstrap/install, mesh, JOIN_TOKEN |
| [SETUP-MAIN](./SETUP-MAIN.md) | **Главный сервер** — полная инструкция |
| [SETUP-SELFHOST](./SETUP-SELFHOST.md) | **Self-host** — расширение сети, nip.io |
| [SETUP-SERVER](../deploy/SETUP-SERVER.md) | Docker с нуля |
| [RUN-DEV](./RUN-DEV.md) | Режим разработки |
| [bootstrap.sh](../bootstrap.sh) | Установка в один клик: `curl ... \| sudo sh` |
| [install-selfhost.sh](../install-selfhost.sh) | Только self-host (без выбора main/selfhost) |

### Архитектура и развёртывание

| Документ | Описание |
|----------|----------|
| [Архитектура](./architecture.md) | Обзор системы, компоненты, потоки данных |
| [Глоссарий](./glossary.md) | Термины, идентификаторы, обозначения |
| [Privacy-by-Design](./privacy.md) | Принципы минимизации метаданных и логов |
| [HA и репликация](./ha-replication.md) | Репликация БД/S3, отказоустойчивость |
| [VPN Mesh и репликация](./MESH-AND-BACKUP.md) | Единая VPN-сеть, coordinator, бэкапы |
| [Subdomain главного](./SUBDOMAIN-MODE.md) | Self-host как subdomain (будет с Cloudflare API) |
| [Self-Hosting](./SELF-HOSTING.md) | Обзор ролей main/selfhost, nip.io, HA |
| [Автоматизация развёртывания](./DEPLOY-AUTOMATION.md) | Цепочка install, nip.io, масштабирование |

### Протокол

| Документ | Описание |
|----------|----------|
| [Форматы сообщений](./protocol/01-message-formats.md) | Envelope, типы событий (text, file) |
| [Модель ключей](./protocol/02-key-model.md) | Identity keys, prekeys, жизненный цикл, X3DH/Double Ratchet |
| [Интеграция libsignal](./LIBSIGNAL-INTEGRATION.md) | План перехода на полноценный Signal protocol |
| [E2EE Threat Model](./E2EE-THREAT-MODEL.md) | Угрозы, MITM, аутентификация шифротекста |
| [Усиление безопасности](./SECURITY-HARDENING.md) | Roadmap и чеклист защиты |
| [Push / VAPID](./PUSH-VAPID.md) | Генерация ключей, настройка push |
| [Федерация (setup)](./FEDERATION-SETUP.md) | Связь между инстансами |
| [Защита федерации](./FEDERATION-SECURITY.md) | Rate limit, blocklist, mTLS, webhook, WAF |
| [Тестирование](./TESTING.md) | Smoke и E2E |
| [Полная пересборка](./CLEAN-REBUILD.md) | Очистка БД, volumes, чистый сброс |

### API

| Документ | Описание |
|----------|----------|
| [C2S API](./api/c2s-api.md) | Клиент ↔ Сервер |
| [S2S API](./api/s2s-api.md) | Федерация (сервер ↔ сервер) |

## Порядок чтения

1. **Архитектура** — общее понимание системы
2. **Глоссарий** — соглашения по именованию
3. **Протокол** — форматы данных и криптография
4. **API** — интерфейсы взаимодействия
5. **Privacy** — ограничения и настройки по умолчанию

## MVP Scope

- **Чаты 1:1** + **Файлы**
- 1 устройство на аккаунт (упрощение для MVP)
- E2EE: упрощённая схема (X25519 DH + NaCl secretbox); опционально — Signal Protocol (`sig1:`)
- Федерация по модели email/Matrix
- VPN — отдельный модуль, плагинная архитектура

> Подробнее об E2EE: [LIBSIGNAL-INTEGRATION.md](LIBSIGNAL-INTEGRATION.md)

## Версионирование

Документация соответствует **версии протокола 0.1** (MVP). При изменениях API или форматов — обновлять номер версии и вести changelog.
