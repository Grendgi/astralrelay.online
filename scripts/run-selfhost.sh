#!/usr/bin/env sh
# Self-host — делегирует в deploy/selfhost
set -e
cd "$(dirname "$0")/.."
exec ./deploy/selfhost/run.sh "$@"
