#!/usr/bin/env sh
# Полностью автоматическая установка — один запуск, zero конфиг
#
# Одна команда: curl -fsSL .../install.sh | sudo sh
# (из репозитория: git clone https://github.com/Grendgi/astralrelay.online.git && cd astralrelay.online && sudo ./install.sh)
#
# Режимы:
#   INSTALL_AUTO=1              — без вопросов, всё авто
#   INSTALL_MODE=main|selfhost  — hub или свой инстанс
#   INSTALL_DOMAIN=...          — домен (иначе IP.nip.io)
#   MESH_ENABLED=1, COORDINATOR_URL, MAIN_DOMAIN — для mesh (токен запрашивается сам)
#   INSTALL_ADDRESS_MODE=subdomain|standalone — subdomain или свой адрес (по умолчанию standalone)

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# Авто-режим: нет TTY или явно INSTALL_AUTO
if [ -n "${INSTALL_AUTO:-}" ] || ! [ -t 0 ]; then
  INSTALL_AUTO=1
fi

# === 1. Проверка/установка Docker ===
ensure_docker() {
  if command -v docker >/dev/null 2>&1 && (docker compose version >/dev/null 2>&1 || docker-compose version >/dev/null 2>&1); then
    return 0
  fi
  if [ -f "$SCRIPT_DIR/deploy/setup-server.sh" ]; then
    echo "Установка Docker..."
    if [ "$(id -u)" -eq 0 ]; then
      sh "$SCRIPT_DIR/deploy/setup-server.sh"
    elif command -v sudo >/dev/null 2>&1; then
      sudo sh "$SCRIPT_DIR/deploy/setup-server.sh"
    else
      echo "Запустите от root или с sudo: sudo $0 $*"
      exit 1
    fi
  else
    echo "Docker не найден. Установите: curl -fsSL https://get.docker.com | sh"
    exit 1
  fi
}

echo ""
echo "=== Chat_VPN — установка ==="
ensure_docker
echo "[OK] Docker"
echo ""

COMPOSE="docker compose"
docker compose version >/dev/null 2>&1 || COMPOSE="docker-compose"

# === 2. Определение режима и домена (без промптов в AUTO) ===
MODE="${INSTALL_MODE:-selfhost}"
DOMAIN="${INSTALL_DOMAIN:-}"

if [ -z "$DOMAIN" ]; then
  echo "Определение внешнего IP..."
  DOMAIN=$(curl -s --max-time 10 -4 icanhazip.com 2>/dev/null || \
           curl -s --max-time 10 -4 ifconfig.me 2>/dev/null || \
           curl -s --max-time 10 -4 ipv4.icanhazip.com 2>/dev/null || \
           echo "")
  if [ -z "$DOMAIN" ]; then
    DOMAIN="localhost"
    echo "IP не определён, используется localhost"
  elif echo "$DOMAIN" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$'; then
    DOMAIN="$DOMAIN.nip.io"
    echo "Домен: $DOMAIN"
  fi
fi

if [ "$INSTALL_AUTO" != "1" ]; then
  echo "[1/6] Режим установки"
  echo ""
  echo "─────────────────────────────────────────"
  echo "  1) main   — главный hub-сервер"
  echo "  2) selfhost — свой узел (расширяет сеть)"
  echo "─────────────────────────────────────────"
  printf "Выбор [1/2, default 2]: "
  read -r ans
  case "${ans:-2}" in 1|main) MODE=main ;; *) MODE=selfhost ;; esac

  if [ "$MODE" = "selfhost" ]; then
    echo ""
    echo "─────────────────────────────────────────"
    echo "  Режим адреса (как к вам будут подключаться):"
    echo "  1) Свой адрес — nip.io, sslip.io, Cloudflare Tunnel или внешний IP (рекомендуется)"
    echo "  2) Subdomain главного — через домен main (будет доступно с Cloudflare API)"
    echo "─────────────────────────────────────────"
    printf "Выбор [1/2, default 1]: "
    read -r addr
    case "${addr:-1}" in 2) ADDRESS_MODE_INTERACTIVE=subdomain ;; *) ADDRESS_MODE_INTERACTIVE=standalone ;; esac

    echo ""
    echo "─────────────────────────────────────────"
    echo "  Mesh-сеть: VPN между узлами + бэкапы на другие серверы"
    echo "  Введите домен главного сервера (например astralrelay.online)"
    echo "  или Enter чтобы пропустить (без mesh)"
    echo "─────────────────────────────────────────"
    printf "Домен главного []: "
    read -r mesh_domain
    [ -n "$mesh_domain" ] && MAIN_DOMAIN_INTERACTIVE="$mesh_domain"
  fi

  if [ -z "${INSTALL_DOMAIN:-}" ]; then
    echo ""
    printf "Домен или IP для вашего узла [default $DOMAIN]: "
    read -r d
    [ -n "$d" ] && DOMAIN="$d"
  fi
  echo ""
else
  echo "[1/6] Авто-режим (main/selfhost, IP.nip.io)"
fi

# nip.io для голого IP
if echo "$DOMAIN" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$'; then
  DOMAIN="$DOMAIN.nip.io"
fi

# === 3. Подготовка .env ===
echo "[2/6] Генерация секретов"
ENV_DIR="deploy/$MODE"
ENV_FILE="$ENV_DIR/.env"
mkdir -p "$ENV_DIR"

if [ ! -f "$ENV_FILE" ]; then
  [ -f "$ENV_DIR/.env.example" ] && cp "$ENV_DIR/.env.example" "$ENV_FILE" || touch "$ENV_FILE"
fi

update_env() {
  key="$1"; val="$2"
  if grep -q "^${key}=" "$ENV_FILE" 2>/dev/null; then
    sed -i.bak "s|^${key}=.*|${key}=${val}|" "$ENV_FILE" 2>/dev/null || sed -i "s|^${key}=.*|${key}=${val}|" "$ENV_FILE"
    rm -f "$ENV_FILE.bak"
  else
    echo "${key}=${val}" >> "$ENV_FILE"
  fi
}

get_env() {
  grep -E "^$1=" "$ENV_FILE" 2>/dev/null | head -n1 | cut -d= -f2- | sed 's/^"//;s/"$//'
}

set_if_missing() {
  key="$1"; val="$2"
  cur="$(get_env "$key")"
  [ -n "$cur" ] && return 0
  update_env "$key" "$val"
}

rand_base64() { n="${1:-32}"; openssl rand -base64 "$n" 2>/dev/null || head -c "$((n*3/4))" /dev/urandom | base64 2>/dev/null || echo "change-me"; }

# Секреты — генерируем только если ещё не заданы (безопасно при повторном install)
update_env "SERVER_DOMAIN" "$DOMAIN"
set_if_missing "JWT_SECRET" "$(rand_base64 32)"
set_if_missing "POSTGRES_PASSWORD" "$(rand_base64 24)"
set_if_missing "MINIO_ROOT_PASSWORD" "$(rand_base64 24)"
KEY=$(rand_base64 32)
[ -n "$KEY" ] && set_if_missing "DB_ENCRYPTION_KEY" "$KEY"

# LETSENCRYPT_EMAIL — для nip.io/домена
if [ "$DOMAIN" != "localhost" ] && ! grep -q '^LETSENCRYPT_EMAIL=.' "$ENV_FILE" 2>/dev/null; then
  if [ "$INSTALL_AUTO" = "1" ]; then
    update_env "LETSENCRYPT_EMAIL" "admin@$DOMAIN"
  else
    printf "Email для Let's Encrypt [admin@%s]: " "$DOMAIN"
    read -r ACME_EMAIL
    update_env "LETSENCRYPT_EMAIL" "${ACME_EMAIL:-admin@$DOMAIN}"
  fi
fi

# VAPID — генерируем локально или через Docker (без перезапуска)
gen_vapid() {
  if command -v npx >/dev/null 2>&1; then
    npx -y web-push generate-vapid-keys 2>/dev/null
  else
    docker run --rm node:20-alpine npx -y web-push generate-vapid-keys 2>/dev/null
  fi
}

if ! grep -q '^PUSH_VAPID_PUBLIC_KEY=.' "$ENV_FILE" 2>/dev/null; then
  echo "Генерация VAPID для push..."
  VAPID_OUT=$(gen_vapid 2>/dev/null) || true
  if [ -n "$VAPID_OUT" ]; then
    PUB=$(echo "$VAPID_OUT" | awk '/Public Key:/{getline; gsub(/[ \r]/,""); print; exit}')
    PRIV=$(echo "$VAPID_OUT" | awk '/Private Key:/{getline; gsub(/[ \r]/,""); print; exit}')
    [ -n "$PUB" ] && update_env "PUSH_VAPID_PUBLIC_KEY" "$PUB"
    [ -n "$PRIV" ] && update_env "PUSH_VAPID_PRIVATE_KEY" "$PRIV"
  fi
fi

echo "[OK] Секреты"
echo ""
# === 4. Traefik config ===
gen_traefik() {
  local tpl="$1" out="$2"
  [ ! -f "$tpl" ] && return
  email=$(grep '^LETSENCRYPT_EMAIL=' "$ENV_FILE" 2>/dev/null | cut -d= -f2- | tr -d '"')
  [ -z "$email" ] && email="admin@$DOMAIN"
  export LETSENCRYPT_EMAIL="$email"
  envsubst '${LETSENCRYPT_EMAIL}' < "$tpl" > "$out"
}

# === 5. Mesh и режим адреса ===
echo "[3/6] Mesh и адрес"
MESH_ENABLED="${MESH_ENABLED:-}"
COORDINATOR_URL="${INSTALL_COORDINATOR_URL:-}"
JOIN_TOKEN="${INSTALL_JOIN_TOKEN:-}"
ADDRESS_MODE="${INSTALL_ADDRESS_MODE:-${ADDRESS_MODE_INTERACTIVE:-standalone}}"
MAIN_DOMAIN="${MAIN_DOMAIN:-${MAIN_DOMAIN_INTERACTIVE:-}}"

# Coordinator URL из домена mesh
MESH_DOM="${MAIN_DOMAIN_INTERACTIVE:-$MAIN_DOMAIN}"
if [ -n "$MESH_DOM" ] && [ -z "$COORDINATOR_URL" ]; then
  COORDINATOR_URL="http://$MESH_DOM:9443"
  [ -z "$MAIN_DOMAIN" ] && [ "$ADDRESS_MODE" = "subdomain" ] && MAIN_DOMAIN="$MESH_DOM"
fi
if [ -n "$COORDINATOR_URL" ]; then
  MESH_ENABLED=1
fi

# Токен — если не задан, запрашиваем у coordinator (без участия пользователя)
if [ -n "$COORDINATOR_URL" ] && [ -z "$JOIN_TOKEN" ]; then
  JOIN_TOKEN=$(curl -sf --max-time 10 "$COORDINATOR_URL/v1/token" 2>/dev/null | sed -n 's/.*"token":"\([^"]*\)".*/\1/p') || true
fi

USE_SUBDOMAIN=""
[ "$ADDRESS_MODE" = "subdomain" ] && [ -n "$MAIN_DOMAIN" ] && USE_SUBDOMAIN=1

echo "[OK] Mesh"
echo ""
# === 6. CA для coordinator (main + mesh) ===
gen_mesh_ca() {
  local dir="$1"
  mkdir -p "$dir"
  [ -f "$dir/ca-cert.pem" ] && [ -f "$dir/ca-key.pem" ] && return 0
  echo "Генерация CA для mTLS coordinator..."
  openssl ecparam -genkey -name prime256v1 -out "$dir/ca-key.pem" 2>/dev/null || return 1
  openssl req -new -x509 -key "$dir/ca-key.pem" -out "$dir/ca-cert.pem" -days 3650 \
    -subj "/CN=Mesh Coordinator CA" 2>/dev/null || return 1
}

# === 7. Запуск ===
echo "[4/6] Запуск контейнеров"
COMPOSE_EXTRA=""
[ "$MESH_ENABLED" = "1" ] && COMPOSE_EXTRA="-f deploy/$MODE/docker-compose.mesh.yml"

if [ "$MODE" = "main" ]; then
  if [ "$MESH_ENABLED" = "1" ]; then
    gen_mesh_ca "$SCRIPT_DIR/deploy/main/mesh-ca" || true
  fi
  gen_traefik "deploy/main/traefik/traefik.yml.tpl" "deploy/main/traefik/traefik.yml"
  echo ""
  echo "Запуск Main (Traefik, HA)..."
  $COMPOSE -p main -f deploy/main/docker-compose.yml $COMPOSE_EXTRA --env-file "$ENV_FILE" up -d --build
  echo "[5/6] Регистрация в mesh (первый узел)"
  # Первый узел mesh — регистрация в своём coordinator
  if [ "$MESH_ENABLED" = "1" ] && [ -z "$COORDINATOR_URL" ] && command -v wg >/dev/null 2>&1; then
    echo "Ожидание coordinator..."
    sleep 5
    COORD_URL="http://$DOMAIN:9443"
    [ "$DOMAIN" = "localhost" ] && COORD_URL="http://127.0.0.1:9443"
    MESH_OUT=$(DOMAIN="$DOMAIN" COORDINATOR_URL="$COORD_URL" PUBLIC_IP="${INSTALL_PUBLIC_IP:-}" \
      "$SCRIPT_DIR/scripts/setup-mesh.sh" 2>/dev/null) || true
    if [ -n "$MESH_OUT" ]; then
      echo "$MESH_OUT" | while read -r line; do
        key="${line%%=*}"; val="${line#*=}"
        [ -n "$key" ] && [ -n "$val" ] && update_env "$key" "$val"
      done
      $COMPOSE -p main -f deploy/main/docker-compose.yml $COMPOSE_EXTRA --env-file "$ENV_FILE" up -d backup-receiver 2>/dev/null || true
    fi
  fi
  echo "[6/6] Готово"
  echo ""
  echo "=== Main готов ==="
  echo "  Сайт:    https://$DOMAIN (или http при первом запуске)"
  echo "  Traefik: http://localhost:8082"
  if [ "$MESH_ENABLED" = "1" ]; then
    echo "  Mesh:    Coordinator :9443, Backup :9100"
    echo "  Join:    MAIN_DOMAIN=$DOMAIN INSTALL_ADDRESS_MODE=subdomain sudo ./install.sh"
  fi
else
  # Self-host
  if [ -n "$COORDINATOR_URL" ] && [ "$DOMAIN" != "localhost" ] && command -v wg >/dev/null 2>&1; then
    echo "[5/6] Регистрация в mesh..."
    MTLS_HOST_DIR="$SCRIPT_DIR/deploy/selfhost/federation"
    MESH_OUT=$(DOMAIN="$DOMAIN" COORDINATOR_URL="$COORDINATOR_URL" JOIN_TOKEN="$JOIN_TOKEN" \
      USE_SUBDOMAIN="$USE_SUBDOMAIN" MAIN_DOMAIN="$MAIN_DOMAIN" \
      MTLS_DIR="$MTLS_HOST_DIR" MTLS_CONTAINER_PATH="/etc/messenger/federation" \
      "$SCRIPT_DIR/scripts/setup-mesh.sh" 2>/dev/null) || true
    if [ -n "$MESH_OUT" ]; then
      echo "$MESH_OUT" | while read -r line; do
        key="${line%%=*}"; val="${line#*=}"
        [ -n "$key" ] && [ -n "$val" ] && update_env "$key" "$val"
      done
    fi
  fi
  SUBDOMAIN_MODE=""
  grep -q '^MESH_SUBDOMAIN_MODE=1' "$ENV_FILE" 2>/dev/null && SUBDOMAIN_MODE=1
  SUBDOMAIN_FINAL=$(grep '^SERVER_DOMAIN=' "$ENV_FILE" 2>/dev/null | cut -d= -f2- | tr -d '"' || true)
  COMPOSE_FILES="-f deploy/selfhost/docker-compose.yml"
  if [ "$SUBDOMAIN_MODE" = "1" ] && [ -n "$SUBDOMAIN_FINAL" ]; then
    # Subdomain mode — gateway вместо Traefik
    COMPOSE_FILES="$COMPOSE_FILES -f deploy/selfhost/docker-compose.subdomain.yml"
    echo ""
    echo "Запуск Self-host (subdomain: https://$SUBDOMAIN_FINAL)..."
  elif [ "$DOMAIN" != "localhost" ]; then
    mkdir -p deploy/selfhost/traefik
    gen_traefik "deploy/selfhost/traefik/traefik.yml.tpl" "deploy/selfhost/traefik/traefik.yml"
    COMPOSE_FILES="$COMPOSE_FILES -f deploy/selfhost/docker-compose.traefik.yml"
    echo ""
    echo "Запуск Self-host (HTTPS для $DOMAIN)..."
  else
    echo ""
    echo "Запуск Self-host (локально)..."
  fi
  $COMPOSE -p selfhost $COMPOSE_FILES $COMPOSE_EXTRA --env-file "$ENV_FILE" up -d --build
  echo "[6/6] Готово"
  echo ""
  echo "=== Self-host готов ==="
  if [ -n "$SUBDOMAIN_FINAL" ] && [ "$SUBDOMAIN_FINAL" != "$DOMAIN" ]; then
    echo "  Сайт: https://$SUBDOMAIN_FINAL (через main)"
  elif [ "$DOMAIN" != "localhost" ]; then
    echo "  Сайт: https://$DOMAIN"
  else
    echo "  Web: http://localhost:3000  API: http://localhost:8080"
  fi
fi

# Cron для бэкапов (если mesh)
if [ "$MESH_ENABLED" = "1" ] && grep -q "MESH_BACKUP_PEERS=" "$ENV_FILE" 2>/dev/null; then
  CRON_CMD="0 */6 * * * $SCRIPT_DIR/scripts/backup-to-peers.sh"
  if command -v crontab >/dev/null 2>&1; then
    (crontab -l 2>/dev/null | grep -v backup-to-peers; echo "$CRON_CMD") | crontab - 2>/dev/null || true
  fi
fi

echo ""
echo "Документация: docs/SELF-HOSTING.md docs/MESH-AND-BACKUP.md"
