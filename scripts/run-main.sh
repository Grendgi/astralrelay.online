#!/usr/bin/env sh
# Main — hub-сервер (делегирует в deploy/main)
set -e
cd "$(dirname "$0")/.."
exec ./deploy/main/run.sh "$@"
