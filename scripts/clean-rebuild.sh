#!/usr/bin/env sh
# Полная очистка и пересборка — удаляет ВСЕ данные (БД, файлы, Redis, ключи).
# После выполнения: пустая система, как после первой установки.
#
# Использование: ./scripts/clean-rebuild.sh [main|selfhost|dev] [-f|--force]
#   main     — deploy/main (по умолчанию)
#   selfhost — deploy/selfhost
#   dev      — deploy/dev
#   -f       — без подтверждения (для скриптов)

set -e

MODE="${1:-main}"
FORCE=""
[ "$2" = "-f" ] || [ "$2" = "--force" ] && FORCE=1
[ "$1" = "-f" ] || [ "$1" = "--force" ] && { FORCE=1; MODE="${2:-main}"; }
SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$SCRIPT_DIR"

case "$MODE" in
  main)
    PROJECT="main"
    COMPOSE_FILE="deploy/main/docker-compose.yml"
    ENV_FILE="deploy/main/.env"
    ;;
  selfhost)
    PROJECT="selfhost"
    COMPOSE_FILE="deploy/selfhost/docker-compose.yml"
    ENV_FILE="deploy/selfhost/.env"
    ;;
  dev)
    PROJECT="dev"
    COMPOSE_FILE="deploy/dev/docker-compose.yml"
    ENV_FILE="deploy/dev/.env"
    ;;
  *)
    echo "Режим: main | selfhost | dev"
    exit 1
    ;;
esac

if [ ! -f "$COMPOSE_FILE" ]; then
  echo "Файл не найден: $COMPOSE_FILE"
  exit 1
fi

echo "════════════════════════════════════════════"
echo "  ПОЛНАЯ ОЧИСТКА И ПЕРЕСБОРКА (mode: $MODE)"
echo "  Будут удалены: postgres, minio, redis,"
echo "  server_data, все данные пользователей."
echo "════════════════════════════════════════════"
echo ""
if [ -z "$FORCE" ]; then
  printf "Продолжить? (yes/no): "
  read -r CONFIRM
  if [ "$CONFIRM" != "yes" ]; then
    echo "Отменено."
    exit 0
  fi
fi

ENV_ARG=""
[ -f "$ENV_FILE" ] && ENV_ARG="--env-file $ENV_FILE"
COMPOSE_CMD="docker compose -p $PROJECT -f $COMPOSE_FILE $ENV_ARG"

echo ""
echo "[1/4] Остановка контейнеров..."
$COMPOSE_CMD down --remove-orphans

echo ""
echo "[2/4] Удаление volumes (БД, MinIO, Redis, server_data)..."
$COMPOSE_CMD down -v

echo ""
echo "[3/4] Удаление образов (пересборка с нуля)..."
$COMPOSE_CMD build --no-cache 2>/dev/null || true

echo ""
echo "[4/4] Запуск..."
$COMPOSE_CMD up -d --build

echo ""
echo "✓ Готово. Система пересобрана с нуля."
echo "  При первом запуске будут созданы новые миграции БД."
echo "  Браузер: очистите localStorage (F12 → Application → Clear site data) для полного сброса клиента."
echo ""
