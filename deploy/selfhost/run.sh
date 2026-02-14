#!/usr/bin/env sh
# Self-host — свой инстанс
# Запуск из корня проекта: ./deploy/selfhost/run.sh
# С SERVER_DOMAIN=*.nip.io или доменом — Traefik + HTTPS

set -e
cd "$(dirname "$0")/../.."

ENV_FILE="deploy/selfhost/.env"
if [ ! -f "$ENV_FILE" ]; then
  echo "Запустите: ./install-selfhost.sh или ./install.sh"
  exit 1
fi

export $(grep -v '^#' "$ENV_FILE" | xargs) 2>/dev/null || true

COMPOSE_FILES="-f deploy/selfhost/docker-compose.yml"
# Subdomain mode — gateway вместо Traefik
if grep -q '^MESH_SUBDOMAIN_MODE=1' "$ENV_FILE" 2>/dev/null; then
  COMPOSE_FILES="$COMPOSE_FILES -f deploy/selfhost/docker-compose.subdomain.yml"
elif [ -n "$SERVER_DOMAIN" ] && [ "$SERVER_DOMAIN" != "localhost" ]; then
  mkdir -p deploy/selfhost/traefik
  if [ -f "deploy/selfhost/traefik/traefik.yml.tpl" ]; then
    email=$(grep '^LETSENCRYPT_EMAIL=' "$ENV_FILE" 2>/dev/null | cut -d= -f2- | tr -d '"')
    [ -z "$email" ] && email="changeme@example.com"
    export LETSENCRYPT_EMAIL="$email"
    envsubst '${LETSENCRYPT_EMAIL}' < deploy/selfhost/traefik/traefik.yml.tpl > deploy/selfhost/traefik/traefik.yml
  fi
  COMPOSE_FILES="$COMPOSE_FILES -f deploy/selfhost/docker-compose.traefik.yml"
fi
# Mesh
grep -q '^MESH_VPN_ADDR=' "$ENV_FILE" 2>/dev/null && COMPOSE_FILES="$COMPOSE_FILES -f deploy/selfhost/docker-compose.mesh.yml"

docker compose -p selfhost $COMPOSE_FILES --env-file "$ENV_FILE" up -d --build "$@"

echo ""
if grep -q '^MESH_SUBDOMAIN_MODE=1' "$ENV_FILE" 2>/dev/null; then
  DOM=$(grep '^SERVER_DOMAIN=' "$ENV_FILE" 2>/dev/null | cut -d= -f2- | tr -d '"')
  echo "Self-host: https://${DOM:-subdomain} (через main)"
elif [ -n "$SERVER_DOMAIN" ] && [ "$SERVER_DOMAIN" != "localhost" ]; then
  echo "Self-host: https://$SERVER_DOMAIN"
else
  echo "Self-host: http://localhost:3000 (Web), http://localhost:8080 (API)"
  echo "Для федерации: SERVER_DOMAIN=YOUR_IP.nip.io в .env"
fi
