#!/usr/bin/env sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# По умолчанию запускаем wizard с уже выбранным selfhost
if [ -t 0 ] && [ "${INSTALL_AUTO:-}" != "1" ]; then
  sh ./install.sh --wizard --mode selfhost
else
  INSTALL_AUTO=1 INSTALL_MODE=selfhost sh ./install.sh
fi
