#!/usr/bin/env sh
# Dev — делегирует в deploy/dev
set -e
cd "$(dirname "$0")/.."
exec ./deploy/dev/run.sh "$@"
