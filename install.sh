#!/usr/bin/env sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# -----------------------------
# helpers
# -----------------------------
have_root() { [ "$(id -u)" -eq 0 ]; }
have_sudo() { command -v sudo >/dev/null 2>&1; }

run_root() {
  if have_root; then
    sh -c "$*"
  elif have_sudo; then
    sudo sh -c "$*"
  else
    echo "Нужны права root или sudo."
    exit 1
  fi
}

is_tty() { [ -t 0 ] && [ -t 1 ]; }

say() { printf "%s\n" "$*"; }
warn() { printf "[WARN] %s\n" "$*" >&2; }
die() { printf "[ERR] %s\n" "$*" >&2; exit 1; }

# -----------------------------
# args / env
# -----------------------------
WIZARD=0
MODE="${INSTALL_MODE:-}"            # main|selfhost
DOMAIN="${INSTALL_DOMAIN:-}"
ADDRESS_MODE="${INSTALL_ADDRESS_MODE:-}"   # standalone|subdomain
MAIN_DOMAIN="${MAIN_DOMAIN:-}"      # домен main
COORDINATOR_URL="${INSTALL_COORDINATOR_URL:-}"
JOIN_TOKEN="${INSTALL_JOIN_TOKEN:-}"
ALLOW_DOCKER_VAPID="${ALLOW_DOCKER_VAPID:-0}"

ACTION="${INSTALL_ACTION:-install}" # install|update

while [ $# -gt 0 ]; do
  case "$1" in
    --wizard) WIZARD=1 ;;
    --mode) MODE="$2"; shift ;;
    --domain) DOMAIN="$2"; shift ;;
    --address-mode) ADDRESS_MODE="$2"; shift ;;
    --main-domain) MAIN_DOMAIN="$2"; shift ;;
    --coordinator-url) COORDINATOR_URL="$2"; shift ;;
    --join-token) JOIN_TOKEN="$2"; shift ;;
    --allow-docker-vapid) ALLOW_DOCKER_VAPID=1 ;;
    --action) ACTION="$2"; shift ;;
    --auto) INSTALL_AUTO=1 ;;
    -h|--help)
      cat <<EOF
Usage: sh install.sh [--wizard] [--mode main|selfhost] [--domain example.com]
                    [--address-mode standalone|subdomain]
                    [--main-domain astralrelay.online]
                    [--coordinator-url https://...:9443] [--join-token ...]
                    [--action install|update]
Env:
  INSTALL_AUTO=1 INSTALL_MODE=... INSTALL_DOMAIN=... INSTALL_ADDRESS_MODE=...
  MAIN_DOMAIN=... INSTALL_COORDINATOR_URL=... INSTALL_JOIN_TOKEN=...
  ALLOW_DOCKER_VAPID=1
EOF
      exit 0
      ;;
    *) die "Неизвестный аргумент: $1" ;;
  esac
  shift
done

# Если нет TTY или явно INSTALL_AUTO — авто режим
if [ -n "${INSTALL_AUTO:-}" ] || ! is_tty; then
  INSTALL_AUTO=1
fi

# -----------------------------
# wizard UI
# -----------------------------
prompt_num() {
  # $1 label, $2 default
  label="$1"; def="$2"
  printf "%s [%s]: " "$label" "$def"
  read -r ans
  [ -z "$ans" ] && ans="$def"
  printf "%s" "$ans"
}

prompt_text() {
  label="$1"; def="$2"
  printf "%s [%s]: " "$label" "$def"
  read -r ans
  [ -z "$ans" ] && ans="$def"
  printf "%s" "$ans"
}

prompt_yesno() {
  label="$1"; def="$2" # y|n
  if [ "$def" = "y" ]; then
    printf "%s [Y/n]: " "$label"
  else
    printf "%s [y/N]: " "$label"
  fi
  read -r ans
  ans="$(printf "%s" "$ans" | tr '[:upper:]' '[:lower:]')"
  if [ -z "$ans" ]; then
    ans="$def"
  fi
  case "$ans" in y|yes) printf "y" ;; *) printf "n" ;; esac
}

# -----------------------------
# docker/wg install (без pulls)
# -----------------------------
ensure_wireguard() {
  command -v wg >/dev/null 2>&1 && return 0
  if command -v apt-get >/dev/null 2>&1; then
    say "Установка WireGuard..."
    run_root "apt-get update -y >/dev/null 2>&1 || true; apt-get install -y wireguard-tools >/dev/null 2>&1 || true"
  fi
}

install_docker_debian_ubuntu() {
  say "Установка Docker (Ubuntu/Debian через get.docker.com)..."
  run_root "apt-get update -y >/dev/null 2>&1 || true; apt-get install -y ca-certificates curl gnupg >/dev/null 2>&1 || true"
  run_root "curl -fsSL https://get.docker.com | sh"
  if ! docker compose version >/dev/null 2>&1; then
    run_root "apt-get update -y >/dev/null 2>&1 || true; apt-get install -y docker-compose-plugin >/dev/null 2>&1 || true"
  fi
  if command -v systemctl >/dev/null 2>&1; then
    run_root "systemctl enable --now docker >/dev/null 2>&1 || true"
  fi
}

ensure_docker() {
  if command -v docker >/dev/null 2>&1; then
    docker info >/dev/null 2>&1 || true
    # Если docker есть — ок
    return 0
  fi

  if command -v apt-get >/dev/null 2>&1; then
    install_docker_debian_ubuntu
    ensure_wireguard
  elif [ -f "$SCRIPT_DIR/deploy/setup-server.sh" ]; then
    say "Установка Docker через deploy/setup-server.sh..."
    run_root "sh '$SCRIPT_DIR/deploy/setup-server.sh'"
  else
    die "Docker не найден и автоматическая установка недоступна. Установите Docker вручную."
  fi
}

COMPOSE="docker compose"
pick_compose() {
  docker compose version >/dev/null 2>&1 && { COMPOSE="docker compose"; return; }
  command -v docker-compose >/dev/null 2>&1 && { COMPOSE="docker-compose"; return; }
  COMPOSE="docker compose"
}

# -----------------------------
# env helpers
# -----------------------------
update_env() {
  key="$1"; val="$2"; file="$3"
  if grep -q "^${key}=" "$file" 2>/dev/null; then
    sed -i.bak "s|^${key}=.*|${key}=${val}|" "$file" 2>/dev/null || sed -i "s|^${key}=.*|${key}=${val}|" "$file"
    rm -f "$file.bak" 2>/dev/null || true
  else
    printf "%s=%s\n" "$key" "$val" >> "$file"
  fi
}

get_env() {
  key="$1"; file="$2"
  grep -E "^$key=" "$file" 2>/dev/null | head -n1 | cut -d= -f2- | sed 's/^"//;s/"$//'
}

set_if_missing() {
  key="$1"; val="$2"; file="$3"
  cur="$(get_env "$key" "$file")"
  [ -n "$cur" ] && return 0
  update_env "$key" "$val" "$file"
}

rand_base64() {
  n="${1:-32}"
  openssl rand -base64 "$n" 2>/dev/null || head -c 32 /dev/urandom | base64 2>/dev/null || echo "change-me"
}

detect_domain_default() {
  ip="$(curl -s --max-time 10 -4 icanhazip.com 2>/dev/null || curl -s --max-time 10 -4 ifconfig.me 2>/dev/null || echo "")"
  if [ -z "$ip" ]; then
    echo "localhost"
    return
  fi
  if echo "$ip" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$'; then
    echo "$ip.nip.io"
    return
  fi
  echo "localhost"
}

# -----------------------------
# mesh token helper
# -----------------------------
fetch_join_token() {
  url="$1"
  curl -fsS --max-time 10 --proto '=https,http' "$url/v1/token" 2>/dev/null | \
    sed -n 's/.*"token":"\([^"]*\)".*/\1/p'
}

# -----------------------------
# wizard logic
# -----------------------------
run_wizard() {
  say ""
  say "=== Мастер установки (wizard) ==="
  say "Выбираешь только главное — остальное скрипт делает сам."
  say ""

  # Авто-детект текущего режима по .env
  default_mode="selfhost"
  [ -f "$SCRIPT_DIR/deploy/main/.env" ] && default_mode="main"
  [ -f "$SCRIPT_DIR/deploy/selfhost/.env" ] && default_mode="selfhost"

  say "1) Установить/обновить MAIN (hub)"
  say "2) Установить/обновить SELFHOST (узел)"
  say "3) Просто обновить/перезапустить текущую установку (без вопросов)"
  choice="$(prompt_num "Выбор" "2")"

  case "$choice" in
    1) MODE="main"; ACTION="install" ;;
    2) MODE="selfhost"; ACTION="install" ;;
    3) MODE="$default_mode"; ACTION="update" ;;
    *) MODE="selfhost"; ACTION="install" ;;
  esac

  env_file="$SCRIPT_DIR/deploy/$MODE/.env"
  cur_domain="$(get_env SERVER_DOMAIN "$env_file")"
  [ -z "$cur_domain" ] && cur_domain="$(detect_domain_default)"

  if [ "$ACTION" = "update" ]; then
    say ""
    say "Режим update: буду использовать текущие настройки из $env_file"
    return 0
  fi

  DOMAIN="$(prompt_text "Домен (или оставь пустым для авто)" "$cur_domain")"

  if [ "$MODE" = "selfhost" ]; then
    # Mesh
    cur_main="$(get_env FEDERATION_MAIN_DOMAIN "$env_file")"
    [ -z "$cur_main" ] && cur_main="astralrelay.online"

    use_mesh="$(prompt_yesno "Подключить mesh (main domain)?" "y")"
    if [ "$use_mesh" = "y" ]; then
      MAIN_DOMAIN="$(prompt_text "Домен main" "$cur_main")"
      # address mode
      say ""
      say "Как публиковать selfhost:"
      say "1) standalone (свой домен/https/traefik)"
      say "2) subdomain  (через main)"
      am="$(prompt_num "Выбор" "1")"
      case "$am" in
        2) ADDRESS_MODE="subdomain" ;;
        *) ADDRESS_MODE="standalone" ;;
      esac

      # join token
      COORDINATOR_URL="https://$MAIN_DOMAIN:9443"
      got="$(fetch_join_token "$COORDINATOR_URL" || true)"
      if [ -z "$got" ]; then
        warn "Не получилось получить join token по HTTPS, попробую HTTP..."
        COORDINATOR_URL="http://$MAIN_DOMAIN:9443"
        got="$(fetch_join_token "$COORDINATOR_URL" || true)"
      fi
      if [ -n "$got" ]; then
        JOIN_TOKEN="$got"
        say "Join token получен автоматически."
      else
        warn "Join token не получен автоматически."
        JOIN_TOKEN="$(prompt_text "Вставь JOIN_TOKEN (или оставь пустым и задашь позже)" "")"
      fi
    else
      MAIN_DOMAIN=""
      ADDRESS_MODE="standalone"
      COORDINATOR_URL=""
      JOIN_TOKEN=""
    fi
  fi

  allow_docker_vapid="$(prompt_yesno "Разрешить VAPID через docker, если нет node/npx? (может упереться в rate-limit)" "n")"
  [ "$allow_docker_vapid" = "y" ] && ALLOW_DOCKER_VAPID=1 || ALLOW_DOCKER_VAPID=0

  say ""
  say "=== Итог ==="
  say "MODE:          $MODE"
  say "ACTION:        $ACTION"
  say "DOMAIN:        $DOMAIN"
  [ "$MODE" = "selfhost" ] && say "MAIN_DOMAIN:    ${MAIN_DOMAIN:-<no mesh>}"
  [ "$MODE" = "selfhost" ] && say "ADDRESS_MODE:   ${ADDRESS_MODE:-standalone}"
  [ "$MODE" = "selfhost" ] && [ -n "$COORDINATOR_URL" ] && say "COORDINATOR:    $COORDINATOR_URL"
  [ "$MODE" = "selfhost" ] && [ -n "$JOIN_TOKEN" ] && say "JOIN_TOKEN:     (set)"
  say "ALLOW_DOCKER_VAPID: $ALLOW_DOCKER_VAPID"
  say ""

  ok="$(prompt_yesno "Продолжить?" "y")"
  [ "$ok" = "y" ] || { say "Отменено."; exit 0; }
}

# Запускаем wizard если нужно
if [ "$WIZARD" = "1" ] || { is_tty && [ "${INSTALL_AUTO:-}" != "1" ] && [ -z "$MODE" ]; }; then
  run_wizard
  INSTALL_AUTO=1
fi

# -----------------------------
# main install/update flow
# -----------------------------
[ -z "$MODE" ] && MODE="selfhost"
[ -z "$ADDRESS_MODE" ] && ADDRESS_MODE="standalone"

ensure_docker
pick_compose

say ""
say "=== Установка ==="
say "MODE=$MODE  ACTION=$ACTION"
say ""

ENV_DIR="$SCRIPT_DIR/deploy/$MODE"
ENV_FILE="$ENV_DIR/.env"
mkdir -p "$ENV_DIR"

# безопасные права на секреты
umask 077
if [ ! -f "$ENV_FILE" ]; then
  [ -f "$ENV_DIR/.env.example" ] && cp "$ENV_DIR/.env.example" "$ENV_FILE" || : > "$ENV_FILE"
fi
chmod 600 "$ENV_FILE" 2>/dev/null || true

# домен: если не указан — берём из ENV (если есть), иначе авто
if [ -z "$DOMAIN" ]; then
  existing="$(get_env SERVER_DOMAIN "$ENV_FILE")"
  if [ -n "$existing" ]; then
    DOMAIN="$existing"
  else
    DOMAIN="$(detect_domain_default)"
  fi
fi

# Важно: не переписываем домен случайно — пишем только если новый != текущего или текущего нет
cur_domain="$(get_env SERVER_DOMAIN "$ENV_FILE")"
if [ -z "$cur_domain" ] || [ "$DOMAIN" != "$cur_domain" ]; then
  update_env "SERVER_DOMAIN" "$DOMAIN" "$ENV_FILE"
fi

# secrets idempotent
set_if_missing "JWT_SECRET" "$(rand_base64 32)" "$ENV_FILE"
set_if_missing "POSTGRES_PASSWORD" "$(rand_base64 24)" "$ENV_FILE"
set_if_missing "MINIO_ROOT_PASSWORD" "$(rand_base64 24)" "$ENV_FILE"
set_if_missing "DB_ENCRYPTION_KEY" "$(rand_base64 32)" "$ENV_FILE"

# LetsEncrypt email (если домен не localhost)
if [ "$DOMAIN" != "localhost" ] && ! grep -q '^LETSENCRYPT_EMAIL=.' "$ENV_FILE" 2>/dev/null; then
  update_env "LETSENCRYPT_EMAIL" "admin@$DOMAIN" "$ENV_FILE"
fi

# selfhost federation hints
if [ "$MODE" = "selfhost" ]; then
  update_env "FEDERATION_MODE" "main_only" "$ENV_FILE"

  if [ -n "$MAIN_DOMAIN" ]; then
    update_env "FEDERATION_MAIN_DOMAIN" "$MAIN_DOMAIN" "$ENV_FILE"
  fi

  # address mode
  if [ "$ADDRESS_MODE" = "subdomain" ] && [ -n "$MAIN_DOMAIN" ]; then
    update_env "MESH_SUBDOMAIN_MODE" "1" "$ENV_FILE"
  else
    # не трогаем, если уже стоит; но можно и сбросить
    :
  fi
fi

# VAPID generation (без docker по умолчанию)
gen_vapid() {
  if command -v npx >/dev/null 2>&1; then
    npx -y web-push generate-vapid-keys 2>/dev/null
    return 0
  fi
  if [ "$ALLOW_DOCKER_VAPID" = "1" ] || [ "$ALLOW_DOCKER_VAPID" = "true" ]; then
    docker run --rm node:20-alpine npx -y web-push generate-vapid-keys 2>/dev/null
    return 0
  fi
  return 1
}

if ! grep -q '^PUSH_VAPID_PUBLIC_KEY=.' "$ENV_FILE" 2>/dev/null; then
  say "Генерация VAPID..."
  out="$(gen_vapid 2>/dev/null || true)"
  if [ -n "$out" ]; then
    pub="$(echo "$out" | awk '/Public Key:/{getline; gsub(/[ \r]/,""); print; exit}')"
    priv="$(echo "$out" | awk '/Private Key:/{getline; gsub(/[ \r]/,""); print; exit}')"
    [ -n "$pub" ] && update_env "PUSH_VAPID_PUBLIC_KEY" "$pub" "$ENV_FILE"
    [ -n "$priv" ] && update_env "PUSH_VAPID_PRIVATE_KEY" "$priv" "$ENV_FILE"
  else
    warn "VAPID не сгенерирован (нет npx, docker-fallback выключен). Это не блокирует запуск."
  fi
fi

# coordinator url / token (если mesh)
if [ "$MODE" = "selfhost" ] && [ -n "$MAIN_DOMAIN" ]; then
  if [ -z "$COORDINATOR_URL" ]; then
    COORDINATOR_URL="https://$MAIN_DOMAIN:9443"
  fi
  if [ -z "$JOIN_TOKEN" ]; then
    JOIN_TOKEN="$(fetch_join_token "$COORDINATOR_URL" || true)"
    if [ -z "$JOIN_TOKEN" ] && echo "$COORDINATOR_URL" | grep -q '^https://'; then
      warn "Не получилось по HTTPS, пробую HTTP..."
      COORDINATOR_URL="$(echo "$COORDINATOR_URL" | sed 's|^https://|http://|')"
      JOIN_TOKEN="$(fetch_join_token "$COORDINATOR_URL" || true)"
    fi
  fi
fi

# -----------------------------
# compose up
# -----------------------------
compose_run() {
  # $1 project, $2 compose files string, $3 env file
  proj="$1"; files="$2"; envf="$3"

  tmp="$(mktemp)"
  set +e
  # shellcheck disable=SC2086
  $COMPOSE -p "$proj" $files --env-file "$envf" up -d --build 2>&1 | tee "$tmp"
  rc="${PIPESTATUS:-$?}"
  set -e

  if [ "$rc" -ne 0 ]; then
    if grep -qi "pull rate limit" "$tmp"; then
      warn "Docker Hub rate limit. Решение: sudo docker login (или использовать registry mirror)."
    fi
    rm -f "$tmp" 2>/dev/null || true
    exit "$rc"
  fi

  rm -f "$tmp" 2>/dev/null || true
}

say ""
say "Запуск контейнеров..."

if [ "$MODE" = "main" ]; then
  files="-f deploy/main/docker-compose.yml"
  [ -f "deploy/main/docker-compose.mesh.yml" ] && files="$files -f deploy/main/docker-compose.mesh.yml"

  compose_run "main" "$files" "$ENV_FILE"

  say ""
  say "=== MAIN готов ==="
  say "URL: https://$DOMAIN"
  exit 0
fi

# selfhost:
files="-f deploy/selfhost/docker-compose.yml"

# если subdomain режим — использовать отдельный compose
if grep -q '^MESH_SUBDOMAIN_MODE=1' "$ENV_FILE" 2>/dev/null && [ -f "deploy/selfhost/docker-compose.subdomain.yml" ]; then
  files="$files -f deploy/selfhost/docker-compose.subdomain.yml"
else
  # standalone https (traefik) если есть
  if [ "$DOMAIN" != "localhost" ] && [ -f "deploy/selfhost/docker-compose.traefik.yml" ]; then
    files="$files -f deploy/selfhost/docker-compose.traefik.yml"
  fi
fi

# mesh compose если есть
[ -f "deploy/selfhost/docker-compose.mesh.yml" ] && files="$files -f deploy/selfhost/docker-compose.mesh.yml"

compose_run "selfhost" "$files" "$ENV_FILE"

say ""
say "=== SELFHOST готов ==="
if grep -q '^MESH_SUBDOMAIN_MODE=1' "$ENV_FILE" 2>/dev/null; then
  say "URL: https://$(get_env SERVER_DOMAIN "$ENV_FILE")"
elif [ "$DOMAIN" != "localhost" ]; then
  say "URL: https://$DOMAIN"
else
  say "LOCAL: Web http://localhost:3000  API http://localhost:8080"
fi
