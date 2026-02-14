#!/bin/sh
set -e
export PGPASSWORD=repl_password

# Wait for primary to be ready
until pg_isready -h postgres -U messenger 2>/dev/null; do
  echo "Waiting for primary..."
  sleep 2
done

# If data dir is empty, do basebackup
if [ ! -f "$PGDATA/PG_VERSION" ]; then
  echo "Running pg_basebackup..."
  rm -rf "$PGDATA"/*
  pg_basebackup -h postgres -U replicator -D "$PGDATA" -P -R -X stream
fi

exec /usr/local/bin/docker-entrypoint.sh postgres
