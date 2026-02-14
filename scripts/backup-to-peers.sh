#!/usr/bin/env sh
# Отправка бэкапа (pg_dump + медиа) на backup peers по VPN
# Запуск: cron или systemd timer, например каждые 6 часов
# Требует: BACKUP_PEERS=10.100.0.2,10.100.0.3 BACKUP_SECRET=xxx в .env

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$SCRIPT_DIR"

ENV_FILE="${BACKUP_ENV:-}"
[ -z "$ENV_FILE" ] && [ -f "deploy/main/.env" ] && ENV_FILE="deploy/main/.env"
[ -z "$ENV_FILE" ] && [ -f "deploy/selfhost/.env" ] && ENV_FILE="deploy/selfhost/.env"

[ -f "$ENV_FILE" ] && export $(grep -v '^#' "$ENV_FILE" | grep -v '^$' | xargs)

BACKUP_PEERS="${MESH_BACKUP_PEERS:-$BACKUP_PEERS}"
BACKUP_SECRET="${MESH_BACKUP_SECRET:-$BACKUP_SECRET}"
DOMAIN="${SERVER_DOMAIN:-unknown}"
POSTGRES_USER="${POSTGRES_USER:-messenger}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD}"
POSTGRES_DB="${POSTGRES_DB:-messenger}"

[ -z "$BACKUP_PEERS" ] && echo "BACKUP_PEERS не задан" && exit 0
[ -z "$BACKUP_SECRET" ] && echo "BACKUP_SECRET не задан" && exit 0

TMP_DIR=$(mktemp -d)
trap "rm -rf $TMP_DIR" EXIT

DUMP_FILE="$TMP_DIR/dump_$(date +%Y%m%d-%H%M%S).sql.gz"

# pg_dump
export PGPASSWORD="$POSTGRES_PASSWORD"
if docker ps --format '{{.Names}}' | grep -q 'postgres'; then
  CONT=$(docker ps --format '{{.Names}}' | grep postgres | head -1)
  docker exec "$CONT" pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB" 2>/dev/null | gzip > "$DUMP_FILE"
else
  pg_dump -U "$POSTGRES_USER" -h localhost "$POSTGRES_DB" 2>/dev/null | gzip > "$DUMP_FILE"
fi
unset PGPASSWORD

[ ! -s "$DUMP_FILE" ] && echo "pg_dump не создал файл" && exit 1

# Отправка на каждый peer
for peer in $(echo "$BACKUP_PEERS" | tr ',' ' '); do
  peer=$(echo "$peer" | tr -d ' ')
  [ -z "$peer" ] && continue
  URL="http://${peer}:9100/backup/$DOMAIN"
  if curl -sf -X POST -H "X-Backup-Token: $BACKUP_SECRET" \
     -H "X-Backup-Filename: $(basename $DUMP_FILE)" \
     --data-binary "@$DUMP_FILE" \
     --connect-timeout 10 "$URL" >/dev/null; then
    echo "Backup sent to $peer"
  else
    echo "Failed to send backup to $peer"
  fi
done
