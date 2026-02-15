#!/usr/bin/env sh
set -e

echo "=== astralrelay.online — установка в один клик ==="
echo ""

REPO_URL="${REPO_URL:-https://github.com/Grendgi/astralrelay.online}"
BRANCH="${BRANCH:-main}"

# Backward compat: раньше EXPECTED_SHA256 использовался как префикс commit
EXPECTED_COMMIT="${EXPECTED_COMMIT:-${EXPECTED_SHA256:-}}"

BOOTSTRAP_DIFF="${BOOTSTRAP_DIFF:-0}"
INSTALL_DIR="${INSTALL_DIR:-/opt/astralrelay.online}"
STAGING_DIR=""

cleanup() {
  [ -n "$STAGING_DIR" ] && [ -d "$STAGING_DIR" ] && rm -rf "$STAGING_DIR" || true
}
trap cleanup EXIT

do_download() {
  dest="$1"
  mkdir -p "$(dirname "$dest")"

  if command -v git >/dev/null 2>&1; then
    if [ -d "$dest/.git" ]; then
      (cd "$dest" && git fetch -q origin "$BRANCH" && git reset -q --hard "origin/$BRANCH")
    else
      rm -rf "$dest"
      git clone -q --depth 1 -b "$BRANCH" "$REPO_URL" "$dest"
    fi
  else
    rm -rf "$dest"
    tmp="$(mktemp -d)"
    curl -fsSL "$REPO_URL/archive/$BRANCH.tar.gz" | tar xz -C "$tmp"
    subdir="$(ls -1 "$tmp" | head -1)"
    mv "$tmp/$subdir" "$dest"
    rm -rf "$tmp"
  fi
}

if [ "$BOOTSTRAP_DIFF" = "1" ] || [ "$BOOTSTRAP_DIFF" = "true" ]; then
  echo "Режим diff: скачивание во временный каталог..."
  STAGING_DIR="$(mktemp -d)"
  do_download "$STAGING_DIR"

  if [ -d "$INSTALL_DIR" ] && [ -f "$INSTALL_DIR/install.sh" ]; then
    echo ""
    echo "--- diff install.sh (новое vs текущее) ---"
    diff -u "$INSTALL_DIR/install.sh" "$STAGING_DIR/install.sh" 2>/dev/null || true
    echo "--- diff install-selfhost.sh ---"
    diff -u "$INSTALL_DIR/install-selfhost.sh" "$STAGING_DIR/install-selfhost.sh" 2>/dev/null || true
    echo "--- diff bootstrap.sh ---"
    diff -u "$INSTALL_DIR/bootstrap.sh" "$STAGING_DIR/bootstrap.sh" 2>/dev/null || true
    echo "--- конец diff ---"
    echo ""
    printf "Применить изменения и запустить установку? [y/N] "
    read -r confirm </dev/tty 2>/dev/null || confirm=""
    case "$confirm" in
      [yY]|[yY][eE][sS]) ;;
      *) echo "Отменено."; exit 0 ;;
    esac
  fi

  rm -rf "$INSTALL_DIR"
  mv "$STAGING_DIR" "$INSTALL_DIR"
  STAGING_DIR=""
else
  echo "Скачивание репозитория (branch/tag: $BRANCH)..."
  do_download "$INSTALL_DIR"
fi

cd "$INSTALL_DIR" || exit 1

COMMIT=""
if [ -d ".git" ]; then
  COMMIT="$(git rev-parse HEAD 2>/dev/null || true)"
fi
[ -n "$COMMIT" ] && echo "Версия: $COMMIT"

if [ -n "$EXPECTED_COMMIT" ] && [ -n "$COMMIT" ]; then
  case "$COMMIT" in
    "$EXPECTED_COMMIT"*) ;;
    *)
      echo "Ошибка: ожидался commit prefix $EXPECTED_COMMIT, получен $COMMIT"
      exit 1
      ;;
  esac
fi

# Если есть TTY и пользователь не просит авто-режим — запускаем wizard
if [ -t 0 ] && [ "${INSTALL_AUTO:-}" != "1" ]; then
  sh ./install.sh --wizard
else
  INSTALL_AUTO=1 INSTALL_MODE="${INSTALL_MODE:-selfhost}" sh ./install.sh
fi
