#!/usr/bin/env sh
# Одна команда — полная установка с нуля
# Запуск: curl -fsSL https://raw.githubusercontent.com/Grendgi/astralrelay.online/main/bootstrap.sh | sudo sh
#
# Скачивает репозиторий и запускает install.sh в автоматическом режиме.
# Пользователь ничего не настраивает — всё определяется автоматически.

set -e

echo "=== Chat_VPN — установка в один клик ==="
echo ""

# Определение URL репозитория (можно переопределить)
REPO_URL="${REPO_URL:-https://github.com/Grendgi/astralrelay.online}"
BRANCH="${BRANCH:-main}"
INSTALL_DIR="${INSTALL_DIR:-/opt/Chat_VPN}"

echo "Скачивание репозитория..."
mkdir -p "$(dirname "$INSTALL_DIR")"
if command -v git >/dev/null 2>&1; then
  if [ -d "$INSTALL_DIR/.git" ]; then
    (cd "$INSTALL_DIR" && git pull -q)
  else
    rm -rf "$INSTALL_DIR"
    git clone -q --depth 1 -b "$BRANCH" "$REPO_URL" "$INSTALL_DIR"
  fi
else
  # Без git — через tarball
  rm -rf "$INSTALL_DIR"
  TMP_DIR=$(mktemp -d)
  curl -fsSL "$REPO_URL/archive/refs/heads/$BRANCH.tar.gz" | tar xz -C "$TMP_DIR"
  subdir=$(ls -1 "$TMP_DIR" | head -1)
  mv "$TMP_DIR/$subdir" "$INSTALL_DIR"
  rm -rf "$TMP_DIR"
fi

cd "$INSTALL_DIR" || exit 1
INSTALL_AUTO=1 INSTALL_MODE="${INSTALL_MODE:-selfhost}" ./install.sh

echo ""
echo "Установка завершена. Данные: $INSTALL_DIR"
