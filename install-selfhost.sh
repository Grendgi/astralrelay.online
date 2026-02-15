#!/usr/bin/env sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

if [ ! -f "./install.sh" ]; then
  echo "[ERR] install.sh не найден. Запускайте скрипт из корня репозитория." >&2
  exit 1
fi

# По умолчанию запускаем wizard с уже выбранным selfhost
if [ -t 0 ] && [ "${INSTALL_AUTO:-}" != "1" ]; then
  sh ./install.sh --wizard --mode selfhost
else
  INSTALL_AUTO=1 INSTALL_MODE=selfhost sh ./install.sh
fi
