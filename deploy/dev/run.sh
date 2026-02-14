#!/usr/bin/env sh
# Dev — локальная разработка
# Запуск из корня проекта: ./deploy/dev/run.sh

set -e
cd "$(dirname "$0")/../.."

if [ ! -f deploy/dev/.env ]; then
  cp deploy/dev/.env.example deploy/dev/.env 2>/dev/null || true
fi

ENV_FILE=""
[ -f deploy/dev/.env ] && ENV_FILE="--env-file deploy/dev/.env"

docker compose -p dev -f deploy/dev/docker-compose.yml $ENV_FILE up -d --build "$@"

echo ""
echo "Dev готов."
echo "  Web:    http://localhost:3000"
echo "  API:    http://localhost:8080"
echo "  Postgres: localhost:5432"
echo "  MinIO:  http://localhost:9001"
