#!/usr/bin/env sh
# Main — hub-сервер с Traefik, HA
# Запуск из корня проекта: ./deploy/main/run.sh

set -e
cd "$(dirname "$0")/../.."

ENV_FILE="deploy/main/.env"
if [ ! -f "$ENV_FILE" ]; then
  echo "Запустите сначала: ./install.sh (режим main)"
  echo "Или создайте .env: cp deploy/main/.env.example deploy/main/.env"
  exit 1
fi

# Генерация traefik.yml из шаблона
if [ -f "deploy/main/traefik/traefik.yml.tpl" ]; then
  email=$(grep '^LETSENCRYPT_EMAIL=' "$ENV_FILE" 2>/dev/null | cut -d= -f2- | tr -d '"')
  [ -z "$email" ] && email="changeme@example.com"
  export LETSENCRYPT_EMAIL="$email"
  envsubst '${LETSENCRYPT_EMAIL}' < deploy/main/traefik/traefik.yml.tpl > deploy/main/traefik/traefik.yml
fi

# Coordinator (:9443) — всегда на main, чтобы selfhost-узлы могли получить JOIN_TOKEN
COMPOSE_EXTRA="-f deploy/main/docker-compose.mesh.yml"
docker compose -f deploy/main/docker-compose.yml $COMPOSE_EXTRA --env-file "$ENV_FILE" up -d --build "$@"

export $(grep -v '^#' "$ENV_FILE" | xargs) 2>/dev/null || true
echo ""
echo "Main готов."
echo "  Сайт:    https://${SERVER_DOMAIN:-localhost}"
echo "  Traefik: http://localhost:8082"
echo "  Mesh:    Coordinator :9443, Backup :9100 (откройте 9443 в фаерволе для selfhost)"
echo ""
echo "DNS: A-запись *.${SERVER_DOMAIN:-localhost} на IP сервера (для subdomains)"
