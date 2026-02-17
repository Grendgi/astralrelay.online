#!/usr/bin/env sh
# Обновление установки: подтянуть код (git) и перезапустить контейнеры.
# Запускать из корня репозитория после git pull.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT"

if [ ! -f "./install.sh" ]; then
  echo "[ERR] Запускайте из корня репозитория (где есть install.sh)." >&2
  exit 1
fi

# Определить режим по существующему .env
MODE=""
[ -f "$ROOT/deploy/main/.env" ] && MODE="main"
[ -f "$ROOT/deploy/selfhost/.env" ] && MODE="selfhost"

if [ -z "$MODE" ]; then
  echo "[ERR] Не найден deploy/main/.env или deploy/selfhost/.env. Сначала выполните установку." >&2
  exit 1
fi

echo "Режим: $MODE. Подтягиваю код и перезапускаю..."
if [ -d ".git" ]; then
  git fetch origin main 2>/dev/null || true
  git pull --ff-only 2>/dev/null || true
fi

INSTALL_AUTO=1 INSTALL_MODE="$MODE" INSTALL_ACTION=update sh ./install.sh
