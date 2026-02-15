#!/usr/bin/env sh
# Установка только Self-host (main один, его ставят отдельно)
#
# Запуск: sudo sh ./install-selfhost.sh
# Домен главного по умолчанию: astralrelay.online
#
# Переменные:
#   INSTALL_AUTO=1
#   INSTALL_DOMAIN=...                  (иначе IP.nip.io)
#   MAIN_DOMAIN / MAIN_DOMAIN_DEFAULT   (для mesh)
#   INSTALL_COORDINATOR_URL=...
#   INSTALL_JOIN_TOKEN=...
#   ALLOW_DOCKER_VAPID=1

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

MAIN_DOMAIN_DEFAULT="${MAIN_DOMAIN_DEFAULT:-astralrelay.online}"

# Авто-режим: нет TTY или явно INSTALL_AUTO
if [ -n "${INSTALL_AUTO:-}" ] || ! [ -t 0 ]; then
  INSTALL_AUTO=1
fi

have_root() { [ "$(id -u)" -eq 0 ]; }
have_sudo() { command -v sudo >/dev/null 2>&1; }

run_root() {
  if have_root; then
    sh -c "$*"
  elif have_sudo; then
    sudo sh -c "$*"
  else
    echo "Нужны права root или sudo: sudo $0"
    exit 1
  fi
}

ensure_envsubst() {
  if command -v envsubst >/dev/null 2>&1; then
    return 0
  fi
  if command -v apt-get >/dev/null 2>&1; then
    echo "Установка envsubst (gettext-base)..."
    run_root "apt-get update -y >/dev/null 2>&1 || true; apt-get install -y gettext-base >/dev/null 2>&1 || true"
  fi
  command -v envsubst >/dev/null 2>&1 || return 1
  return 0
}

install_docker_debian_ubuntu() {
  echo "Установка Docker (Ubuntu/Debian через get.docker.com)..."
  run_root "apt-get update -y >/dev/null 2>&1 || true; apt-get install -y ca-certificates curl gnupg >/dev/null 2>&1 || true"
  run_root "curl -fsSL https://get.docker.com | sh"
  if ! docker compose version >/dev/null 2>&1; then
    run_root "apt-get update -y >/dev/null 2>&1 || true; apt-get install -y docker-compose-plugin >/dev/null 2>&1 || true"
  fi
  if command -v systemctl >/dev/null 2>&1; then
    run_root "systemctl enable --now docker >/dev/null 2>&1 || true"
  fi
}

ensure_wireguard() {
  if command -v wg >/dev/null 2>&1; then
    return 0
  fi
  if command -v apt-get >/dev/null 2>&1; then
    echo "Установка WireGuard..."
    run_root "apt-get update -y >/dev/null 2>&1 || true; apt-get install -y wireguard-tools >/dev/null 2>&1 || true"
  fi
}

ensure_docker() {
  if command -v docker >/dev/null 2>&1 && (docker compose version >/dev/null 2>&1 || docker-compose version >/dev/null 2>&1); then
    return 0
  fi

  if command -v apt-get >/dev/null 2>&1; then
    install_docker_debian_ubuntu
    ensure_wireguard
  elif [ -f "$SCRIPT_DIR/deploy/setup-server.sh" ]; then
    echo "Установка Docker через deploy/setup-server.sh..."
    if have_root; then
      sh "$SCRIPT_DIR/deploy/setup-server.sh"
    elif have_sudo; then
      sudo sh "$SCRIPT_DIR/deploy/setup-server.sh"
    else
      echo "Запустите от root или с sudo: sudo $0"
      exit 1
    fi
  else
    echo "Docker не найден. Установите: curl -fsSL https://get.docker.com | sh"
    exit 1
  fi

  command -v docker >/dev/null 2>&1 || { echo "Docker не установлен"; exit 1; }
}

echo ""
echo "=== Chat_VPN — установка Self-host ==="
ensure_docker
echo "[OK] Docker"
echo ""

COMPOSE="docker compose"
docker compose version >/dev/null 2>&1 || COMPOSE="docker-compose"

# === 2. Определение домена ===
MODE="selfhost"
DOMAIN="${INSTALL_DOMAIN:-}"

if [ -z "$DOMAIN" ]; then
  echo "Определение внешнего IP..."
  DOMAIN="$(curl -s --max-time 10 -4 icanhazip.com 2>/dev/null || \
           curl -s --max-time 10 -4 ifconfig.me 2>/dev/null || \
           curl -s --max-time 10 -4 ipv4.icanhazip.com 2>/dev/null || \
           echo "")"
  if [ -z "$DOMAIN" ]; then
    DOMAIN="localhost"
    echo "IP не определён, используется localhost"
  elif echo "$DOMAIN" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$'; then
    DOMAIN="$DOMAIN.nip.io"
    echo "Домен: $DOMAIN"
  fi
fi

# Интерактивные настройки
if [ "$INSTALL_AUTO" != "1" ]; then
  echo "[1/5] Настройка"
  echo ""
  echo "─────────────────────────────────────────"
  echo "  Mesh-сеть: VPN между узлами + бэкапы на другие серверы"
  echo "  Введите домен главного сервера"
  echo "  или Enter чтобы пропустить (без mesh)"
  echo "─────────────────────────────────────────"
  printf "Домен главного [%s, Enter=пропустить]: " "$MAIN_DOMAIN_DEFAULT"
  read -r mesh_domain
  if [ -n "$mesh_domain" ]; then
    MAIN_DOMAIN_INTERACTIVE="$mesh_domain"
  else
    MAIN_DOMAIN_INTERACTIVE=""
  fi

  echo ""
  printf "Домен или IP для вашего узла [default %s]: " "$DOMAIN"
  read -r d
  [ -n "$d" ] && DOMAIN="$d"
  echo ""
else
  echo "[1/5] Авто-режим (IP.nip.io)"
  # По умолчанию selfhost в авто-режиме подключается к MAIN_DOMAIN_DEFAULT.
  MAIN_DOMAIN_INTERACTIVE="${MAIN_DOMAIN_DEFAULT}"
fi

# nip.io для голого IP
if echo "$DOMAIN" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$'; then
  DOMAIN="$DOMAIN.nip.io"
fi

# === 3. Подготовка .env ===
echo "[2/5] Генерация секретов"
ENV_DIR="deploy/selfhost"
ENV_FILE="$ENV_DIR/.env"
mkdir -p "$ENV_DIR"

# Безопасные права на секреты
umask 077

if [ ! -f "$ENV_FILE" ]; then
  [ -f "$ENV_DIR/.env.example" ] && cp "$ENV_DIR/.env.example" "$ENV_FILE" || : > "$ENV_FILE"
fi
chmod 600 "$ENV_FILE" 2>/dev/null || true

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

rand_base64() {
  n="${1:-32}"
  openssl rand -base64 "$n" 2>/dev/null || \
    head -c "$((n*3/4))" /dev/urandom | base64 2>/dev/null || \
    echo "change-me"
}

update_env "SERVER_DOMAIN" "$DOMAIN"

# Self-host общается только через главный
update_env "FEDERATION_MODE" "main_only"

# Секреты — генерируем только если ещё не заданы
set_if_missing "JWT_SECRET" "$(rand_base64 32)"
set_if_missing "POSTGRES_PASSWORD" "$(rand_base64 24)"
set_if_missing "MINIO_ROOT_PASSWORD" "$(rand_base64 24)"
KEY="$(rand_base64 32)"
[ -n "$KEY" ] && set_if_missing "DB_ENCRYPTION_KEY" "$KEY"

if [ "$DOMAIN" != "localhost" ] && ! grep -q '^LETSENCRYPT_EMAIL=.' "$ENV_FILE" 2>/dev/null; then
  if [ "$INSTALL_AUTO" = "1" ]; then
    update_env "LETSENCRYPT_EMAIL" "admin@$DOMAIN"
  else
    printf "Email для Let's Encrypt [admin@%s]: " "$DOMAIN"
    read -r ACME_EMAIL
    update_env "LETSENCRYPT_EMAIL" "${ACME_EMAIL:-admin@$DOMAIN}"
  fi
fi

gen_vapid() {
  if command -v npx >/dev/null 2>&1; then
    npx -y web-push generate-vapid-keys 2>/dev/null
    return 0
  fi
  if [ "${ALLOW_DOCKER_VAPID:-0}" = "1" ] || [ "${ALLOW_DOCKER_VAPID:-0}" = "true" ]; then
    docker run --rm node:20-alpine npx -y web-push generate-vapid-keys 2>/dev/null
    return 0
  fi
  return 1
}

if ! grep -q '^PUSH_VAPID_PUBLIC_KEY=.' "$ENV_FILE" 2>/dev/null; then
  echo "Генерация VAPID для push..."
  VAPID_OUT="$(gen_vapid 2>/dev/null || true)"
  if [ -n "$VAPID_OUT" ]; then
    PUB="$(echo "$VAPID_OUT" | awk '/Public Key:/{getline; gsub(/[ \r]/,""); print; exit}')"
    PRIV="$(echo "$VAPID_OUT" | awk '/Private Key:/{getline; gsub(/[ \r]/,""); print; exit}')"
    [ -n "$PUB" ] && update_env "PUSH_VAPID_PUBLIC_KEY" "$PUB"
    [ -n "$PRIV" ] && update_env "PUSH_VAPID_PRIVATE_KEY" "$PRIV"
  else
    echo "[WARN] VAPID не сгенерирован (нет npx; docker-fallback выключен)."
    echo "       Это не критично для запуска. Для push-уведомлений:"
    echo "       - установи node+npm, либо"
    echo "       - запусти с ALLOW_DOCKER_VAPID=1 (может упереться в Docker Hub лимит), либо"
    echo "       - задай PUSH_VAPID_PUBLIC_KEY / PUSH_VAPID_PRIVATE_KEY вручную в $ENV_FILE"
  fi
fi

echo "[OK] Секреты"
echo ""

# === 4. Traefik config ===
gen_traefik() {
  tpl="$1"
  out="$2"
  [ ! -f "$tpl" ] && return 0

  email="$(grep '^LETSENCRYPT_EMAIL=' "$ENV_FILE" 2>/dev/null | cut -d= -f2- | tr -d '"')"
  [ -z "$email" ] && email="admin@$DOMAIN"
  LETSENCRYPT_EMAIL="$email"

  if ensure_envsubst >/dev/null 2>&1; then
    export LETSENCRYPT_EMAIL
    envsubst '${LETSENCRYPT_EMAIL}' < "$tpl" > "$out"
  else
    esc_email="$(printf "%s" "$LETSENCRYPT_EMAIL" | sed 's/[\/&]/\\&/g')"
    sed "s|\${LETSENCRYPT_EMAIL}|$esc_email|g" "$tpl" > "$out"
  fi
}

# === 5. Mesh ===
echo "[3/5] Mesh и адрес"
COORDINATOR_URL="${INSTALL_COORDINATOR_URL:-}"
JOIN_TOKEN="${INSTALL_JOIN_TOKEN:-}"
MAIN_DOMAIN="${MAIN_DOMAIN:-${MAIN_DOMAIN_INTERACTIVE:-}}"

MESH_DOM="${MAIN_DOMAIN_INTERACTIVE:-$MAIN_DOMAIN}"
if [ -n "$MESH_DOM" ]; then
  update_env "FEDERATION_MAIN_DOMAIN" "$MESH_DOM"
else
  # Если mesh отключён явно — не трогаем FEDERATION_MAIN_DOMAIN
  :
fi

if [ -n "$MESH_DOM" ] && [ -z "$COORDINATOR_URL" ]; then
  if [ "$MESH_DOM" = "localhost" ] || echo "$MESH_DOM" | grep -qE '^(127\.|10\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[0-1])\.)'; then
    COORDINATOR_URL="http://$MESH_DOM:9443"
  else
    COORDINATOR_URL="https://$MESH_DOM:9443"
  fi
fi

MESH_ENABLED=""
[ -n "$COORDINATOR_URL" ] && MESH_ENABLED=1

fetch_join_token() {
  url="$1"
  curl -fsS --max-time 10 --proto '=https,http' "$url/v1/token" 2>/dev/null | \
    sed -n 's/.*"token":"\([^"]*\)".*/\1/p'
}

if [ -n "$COORDINATOR_URL" ] && [ -z "$JOIN_TOKEN" ]; then
  JOIN_TOKEN="$(fetch_join_token "$COORDINATOR_URL" || true)"
  if [ -z "$JOIN_TOKEN" ] && echo "$COORDINATOR_URL" | grep -q '^https://'; then
    echo "[WARN] Не удалось получить JOIN_TOKEN по HTTPS, пробую HTTP..."
    COORDINATOR_URL="$(echo "$COORDINATOR_URL" | sed 's|^https://|http://|')"
    JOIN_TOKEN="$(fetch_join_token "$COORDINATOR_URL" || true)"
  fi
  if [ -z "$JOIN_TOKEN" ] && [ -n "$COORDINATOR_URL" ]; then
    echo "[WARN] JOIN_TOKEN не получен автоматически. Можно задать INSTALL_JOIN_TOKEN=... или проверить доступность coordinator."
  fi
fi

echo "[OK] Mesh"
echo ""

# === 6. Запуск ===
echo "[4/5] Запуск контейнеров"
COMPOSE_EXTRA=""
[ "$MESH_ENABLED" = "1" ] && COMPOSE_EXTRA="-f deploy/selfhost/docker-compose.mesh.yml"

if [ -n "$COORDINATOR_URL" ] && [ "$DOMAIN" != "localhost" ] && command -v wg >/dev/null 2>&1; then
  echo "[5/5] Регистрация в mesh..."
  MTLS_HOST_DIR="$SCRIPT_DIR/deploy/selfhost/federation"
  MESH_OUT="$(DOMAIN="$DOMAIN" COORDINATOR_URL="$COORDINATOR_URL" JOIN_TOKEN="$JOIN_TOKEN" \
    USE_SUBDOMAIN="" MAIN_DOMAIN="$MAIN_DOMAIN" \
    MTLS_DIR="$MTLS_HOST_DIR" MTLS_CONTAINER_PATH="/etc/messenger/federation" \
    "$SCRIPT_DIR/scripts/setup-mesh.sh" 2>/dev/null || true)"
  if [ -n "$MESH_OUT" ]; then
    echo "$MESH_OUT" | while read -r line; do
      key="${line%%=*}"; val="${line#*=}"
      [ -n "$key" ] && [ -n "$val" ] && update_env "$key" "$val"
    done
  fi
fi

SUBDOMAIN_MODE=""
grep -q '^MESH_SUBDOMAIN_MODE=1' "$ENV_FILE" 2>/dev/null && SUBDOMAIN_MODE=1
SUBDOMAIN_FINAL="$(grep '^SERVER_DOMAIN=' "$ENV_FILE" 2>/dev/null | cut -d= -f2- | tr -d '"' || true)"

COMPOSE_FILES="-f deploy/selfhost/docker-compose.yml"

if [ "$SUBDOMAIN_MODE" = "1" ] && [ -n "$SUBDOMAIN_FINAL" ]; then
  COMPOSE_FILES="$COMPOSE_FILES -f deploy/selfhost/docker-compose.subdomain.yml"
  echo "Запуск Self-host (subdomain: https://$SUBDOMAIN_FINAL)..."
elif [ "$DOMAIN" != "localhost" ]; then
  mkdir -p deploy/selfhost/traefik
  gen_traefik "deploy/selfhost/traefik/traefik.yml.tpl" "deploy/selfhost/traefik/traefik.yml"
  COMPOSE_FILES="$COMPOSE_FILES -f deploy/selfhost/docker-compose.traefik.yml"
  echo "Запуск Self-host (HTTPS для $DOMAIN)..."
else
  echo "Запуск Self-host (локально)..."
fi

$COMPOSE -p selfhost $COMPOSE_FILES $COMPOSE_EXTRA --env-file "$ENV_FILE" up -d --build

echo ""
echo "=== Self-host готов ==="
if [ -n "$SUBDOMAIN_FINAL" ] && [ "$SUBDOMAIN_FINAL" != "$DOMAIN" ]; then
  echo "  Сайт: https://$SUBDOMAIN_FINAL (через main)"
elif [ "$DOMAIN" != "localhost" ]; then
  echo "  Сайт: https://$DOMAIN"
else
  echo "  Web: http://localhost:3000  API: http://localhost:8080"
fi

if [ "$MESH_ENABLED" = "1" ] && grep -q "MESH_BACKUP_PEERS=" "$ENV_FILE" 2>/dev/null; then
  CRON_CMD="0 */6 * * * $SCRIPT_DIR/scripts/backup-to-peers.sh"
  if command -v crontab >/dev/null 2>&1; then
    (crontab -l 2>/dev/null | grep -v backup-to-peers; echo "$CRON_CMD") | crontab - 2>/dev/null || true
  fi
fi

echo ""
echo "Документация: docs/SELF-HOSTING.md docs/MESH-AND-BACKUP.md"
