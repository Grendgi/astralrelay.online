#!/usr/bin/env sh
# Настройка WireGuard mesh — регистрация в coordinator
# COORDINATOR_URL=https://coord.domain:9443 JOIN_TOKEN=xxx DOMAIN=hub.example.com ./scripts/setup-mesh.sh
# Выводит в stdout: MESH_VPN_ADDR=... MESH_BACKUP_PEERS=... MESH_BACKUP_SECRET=...

set -e

COORDINATOR_URL="${COORDINATOR_URL:-}"
JOIN_TOKEN="${JOIN_TOKEN:-}"
DOMAIN="${DOMAIN:-}"
PUBLIC_IP="${PUBLIC_IP:-}"
USE_SUBDOMAIN="${USE_SUBDOMAIN:-}"
MAIN_DOMAIN="${MAIN_DOMAIN:-}"
WG_PORT="${WG_PORT:-51820}"
WG_DIR="${WG_DIR:-/etc/wireguard}"
MTLS_DIR="${MTLS_DIR:-/etc/messenger/federation}"
# Путь внутри контейнера (если MTLS_DIR — host path в проекте)
MTLS_CONTAINER_PATH="${MTLS_CONTAINER_PATH:-}"

[ -z "$COORDINATOR_URL" ] && echo "COORDINATOR_URL обязателен" && exit 1
[ -z "$DOMAIN" ] && echo "DOMAIN обязателен" && exit 1

# Параметры для GET (token опционален для первого узла)
URL="$COORDINATOR_URL/v1/config?endpoint=${PUBLIC_IP}:${WG_PORT}&domain=$(echo "$DOMAIN" | sed 's/:/%3A/g')&public_key=PK"
# public_key подставим после генерации

# Определить внешний IP
if [ -z "$PUBLIC_IP" ]; then
  PUBLIC_IP=$(curl -s --max-time 5 -4 icanhazip.com 2>/dev/null || curl -s --max-time 5 -4 ifconfig.me 2>/dev/null || true)
fi
[ -z "$PUBLIC_IP" ] && echo "Не удалось определить PUBLIC_IP" && exit 1

mkdir -p "$WG_DIR"
if [ ! -f "$WG_DIR/privatekey" ]; then
  wg genkey | tee "$WG_DIR/privatekey" | wg pubkey > "$WG_DIR/publickey"
  chmod 600 "$WG_DIR/privatekey" "$WG_DIR/publickey"
fi
PRIVKEY=$(cat "$WG_DIR/privatekey")
PUBKEY=$(cat "$WG_DIR/publickey")

# Регистрация через GET /v1/config
ENDPOINT="${PUBLIC_IP}:${WG_PORT}"
FULL_URL="${COORDINATOR_URL}/v1/config?public_key=$(echo "$PUBKEY" | sed 's/+/%2B/g; s/\//%2F/g')&endpoint=$(echo "$ENDPOINT" | sed 's/:/%3A/g')&domain=$(echo "$DOMAIN" | sed 's/:/%3A/g; s/\./%2E/g')"
[ -n "$JOIN_TOKEN" ] && FULL_URL="${FULL_URL}&token=$(echo "$JOIN_TOKEN" | sed 's/+/%2B/g')"
[ "$USE_SUBDOMAIN" = "1" ] && [ -n "$MAIN_DOMAIN" ] && FULL_URL="${FULL_URL}&use_subdomain=1&main_domain=$(echo "$MAIN_DOMAIN" | sed 's/\./%2E/g')"

RESP_HEADERS=$(mktemp)
RESP_BODY=$(mktemp)
trap "rm -f $RESP_HEADERS $RESP_BODY" EXIT

if ! curl -sk -D "$RESP_HEADERS" -o "$RESP_BODY" "$FULL_URL" 2>/dev/null; then
  echo "Ошибка соединения с coordinator"
  exit 1
fi

VPN_IP=$(grep -i "x-vpn-ip:" "$RESP_HEADERS" | cut -d: -f2- | tr -d ' \r')
BACKUP_PEERS=$(grep -i "x-backup-peers:" "$RESP_HEADERS" | cut -d: -f2- | tr -d ' \r')
BACKUP_SECRET=$(grep -i "x-backup-secret:" "$RESP_HEADERS" | cut -d: -f2- | tr -d ' \r')
SUBDOMAIN=$(grep -i "x-subdomain:" "$RESP_HEADERS" | cut -d: -f2- | tr -d ' \r')
SERVER_DOMAIN_SUB=$(grep -i "x-server-domain:" "$RESP_HEADERS" | cut -d: -f2- | tr -d ' \r')

if [ -z "$VPN_IP" ]; then
  echo "Coordinator не вернул VPN IP. Ответ:" && cat "$RESP_BODY"
  exit 1
fi

# Подставить private key в конфиг
sed "s|<PRIVATE_KEY>|$PRIVKEY|" "$RESP_BODY" > "$WG_DIR/wg0.conf"
chmod 600 "$WG_DIR/wg0.conf"

# Запуск WireGuard
if command -v wg-quick >/dev/null 2>&1; then
  wg-quick down wg0 2>/dev/null || true
  wg-quick up wg0 2>/dev/null || true
  systemctl enable wg-quick@wg0 2>/dev/null || true
fi

echo "MESH_VPN_ADDR=$VPN_IP"
echo "MESH_BACKUP_PEERS=$BACKUP_PEERS"
echo "MESH_BACKUP_SECRET=$BACKUP_SECRET"
[ -n "$SUBDOMAIN" ] && echo "MESH_SUBDOMAIN=$SUBDOMAIN"
[ -n "$SERVER_DOMAIN_SUB" ] && echo "SERVER_DOMAIN=$SERVER_DOMAIN_SUB" && echo "MESH_SUBDOMAIN_MODE=1"

# mTLS: fetch federation client cert from coordinator (optional)
if [ -n "$JOIN_TOKEN" ]; then
  CERT_URL="${COORDINATOR_URL}/v1/cert"
  CERT_RESP=$(mktemp)
  trap "rm -f $RESP_HEADERS $RESP_BODY $CERT_RESP" EXIT 2>/dev/null || true
  if curl -sk -X POST -H "Content-Type: application/json" \
     -d "{\"token\":\"$(echo "$JOIN_TOKEN" | sed 's/"/\\"/g')\",\"domain\":\"$(echo "$DOMAIN" | sed 's/"/\\"/g')\"}" \
     "$CERT_URL" -o "$CERT_RESP" 2>/dev/null; then
    if command -v jq >/dev/null 2>&1; then
      CERT_PEM=$(jq -r '.cert_pem // empty' "$CERT_RESP")
      KEY_PEM=$(jq -r '.key_pem // empty' "$CERT_RESP")
    else
      CERT_PEM=$(python3 -c "import json,sys; d=json.load(open('$CERT_RESP')); print(d.get('cert_pem',''))" 2>/dev/null || true)
      KEY_PEM=$(python3 -c "import json,sys; d=json.load(open('$CERT_RESP')); print(d.get('key_pem',''))" 2>/dev/null || true)
    fi
    if [ -n "$CERT_PEM" ] && [ -n "$KEY_PEM" ]; then
      mkdir -p "$MTLS_DIR"
      echo "$CERT_PEM" > "$MTLS_DIR/client-cert.pem"
      echo "$KEY_PEM" > "$MTLS_DIR/client-key.pem"
      chmod 600 "$MTLS_DIR/client-cert.pem" "$MTLS_DIR/client-key.pem"
      OUT_PATH="${MTLS_CONTAINER_PATH:-$MTLS_DIR}"
      echo "FEDERATION_MTLS_CLIENT_CERT=$OUT_PATH/client-cert.pem"
      echo "FEDERATION_MTLS_CLIENT_KEY=$OUT_PATH/client-key.pem"
    fi
  fi
fi