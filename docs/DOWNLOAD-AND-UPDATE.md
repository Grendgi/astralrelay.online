# Скачивание и обновление на своих серверах

Как пользователи могут впервые установить проект и как обновлять его до новых версий.

---

## Способы первой установки

| Способ | Когда использовать | Команда |
|--------|--------------------|--------|
| **Bootstrap (одна команда)** | Рекомендуется: не нужен заранее установленный git | `curl -fsSL https://raw.githubusercontent.com/Grendgi/astralrelay.online/main/bootstrap.sh \| sudo sh` |
| **Git clone + install** | Уже есть git, нужен полный репозиторий (теги, история) | `git clone https://github.com/Grendgi/astralrelay.online.git && cd astralrelay.online && sudo ./install.sh` |
| **Selfhost только** | Только свой узел, без выбора main/selfhost в мастере | `git clone ... && cd astralrelay.online && sudo ./install-selfhost.sh` |

Bootstrap скачивает код в `INSTALL_DIR` (по умолчанию `/opt/astralrelay.online`), затем запускает мастер: **1** — main, **2** — selfhost, **3** — только обновить текущую установку.

Фиксация версии при установке:
- `BRANCH=v0.3.1` — установить конкретный тег/ветку.
- `EXPECTED_COMMIT=abc123` — проверка префикса коммита (опционально).

---

## Обновление до новой версии

Есть два сценария: **обновить код и перезапустить** и **только перезапустить** (без нового кода).

### 1. Обновить код и перезапустить

**Если ставили через bootstrap** (каталог в `/opt/astralrelay.online` или другой `INSTALL_DIR`):

Снова запустите bootstrap и в мастере выберите **3** (обновить текущую установку). Скрипт подтянет свежий код и перезапустит контейнеры с текущим `.env`:

```bash
curl -fsSL https://raw.githubusercontent.com/Grendgi/astralrelay.online/main/bootstrap.sh | sudo sh
# В мастере выбрать: 3) Просто обновить/перезапустить текущую установку
```

Без интерактива (если уже настроен main или selfhost):

```bash
INSTALL_AUTO=1 INSTALL_ACTION=update INSTALL_DIR=/opt/astralrelay.online \
  sh -c "$(curl -fsSL https://raw.githubusercontent.com/Grendgi/astralrelay.online/main/bootstrap.sh)"
```

**Если ставили через git clone**:

```bash
cd /path/to/astralrelay.online
git fetch origin main
git checkout main
git pull --ff-only
sudo ./scripts/update.sh
# или вручную: sudo ./install.sh --action update
```

Скрипт `scripts/update.sh` делает то же самое (pull + определение режима main/selfhost по `.env` + `install.sh --action update`).

### 2. Только перезапустить (без нового кода)

Когда нужно перезапустить сервисы без обновления репозитория:

```bash
cd /opt/astralrelay.online   # или ваш каталог
sudo ./install.sh --action update
```

Или через docker compose напрямую (main):

```bash
cd /opt/astralrelay.online
docker compose -f deploy/main/docker-compose.yml --env-file deploy/main/.env up -d --build
```

---

## Краткая шпаргалка

| Цель | Действие |
|------|----------|
| Первая установка | `curl -fsSL .../bootstrap.sh \| sudo sh` или `git clone` + `./install.sh` |
| Обновить всё (код + контейнеры) | Снова `bootstrap` → пункт 3, либо `git pull` + `./scripts/update.sh` |
| Только перезапустить | `./install.sh --action update` или `docker compose up -d --build` |
| Установить конкретный тег | `BRANCH=v0.3.1` перед bootstrap или `git checkout v0.3.1` перед install |

---

## Что сохраняется при обновлении

- **deploy/main/.env** и **deploy/selfhost/.env** — не перезаписываются (секреты, домен, mesh).
- **traefik/traefik.yml** — генерируется заново из шаблона при запуске install (подставляется LETSENCRYPT_EMAIL из .env).
- Данные в томах Docker (postgres_data, minio_data, server_data и т.д.) не трогаются.
- Миграции БД выполняются при старте сервера (при наличии новых в server/internal/db/migrations).

Перед крупным обновлением разумно сделать бэкап БД и при необходимости проверить [CHANGELOG](CHANGELOG.md).

---

## Неинтерактивный режим (CI / скрипты)

```bash
# Установка main
INSTALL_AUTO=1 INSTALL_MODE=main sudo ./install.sh

# Установка selfhost
INSTALL_AUTO=1 INSTALL_MODE=selfhost sudo ./install.sh

# Обновление (код уже подтянут через git или bootstrap)
INSTALL_AUTO=1 INSTALL_ACTION=update sudo ./install.sh
```

Режим (main/selfhost) при `ACTION=update` определяется по наличию `deploy/main/.env` или `deploy/selfhost/.env`.
