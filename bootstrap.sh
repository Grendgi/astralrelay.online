#!/usr/bin/env sh
# Одна команда — полная установка с нуля
# Запуск: curl -fsSL https://raw.githubusercontent.com/Grendgi/astralrelay.online/main/bootstrap.sh | sudo sh
#
# Переменные: REPO_URL, BRANCH, EXPECTED_SHA256, BOOTSTRAP_DIFF=1 (скачать → diff → подтверждение)

set -e

echo "=== astralrelay.online — установка в один клик ==="
echo ""

REPO_URL="${REPO_URL:-https://github.com/Grendgi/astralrelay.online}"
BRANCH="${BRANCH:-main}"
EXPECTED_SHA256="${EXPECTED_SHA256:-}"
BOOTSTRAP_DIFF="${BOOTSTRAP_DIFF:-0}"
INSTALL_DIR="${INSTALL_DIR:-/opt/astralrelay.online}"
STAGING_DIR=""

cleanup_staging() {
  if [ -n "$STAGING_DIR" ] && [ -d "$STAGING_DIR" ]; then
    rm -rf "$STAGING_DIR"
  fi
}
trap cleanup_staging EXIT

do_download() {
  local dest="$1"
  mkdir -p "$(dirname "$dest")"
  if command -v git >/dev/null 2>&1; then
    if [ -d "$dest/.git" ]; then
      (cd "$dest" && git fetch -q origin "$BRANCH" && git checkout -q "$BRANCH" 2>/dev/null || git pull -q)
    else
      rm -rf "$dest"
      git clone -q --depth 1 -b "$BRANCH" "$REPO_URL" "$dest"
    fi
  else
    rm -rf "$dest"
    TMP_DIR=$(mktemp -d)
    curl -fsSL "$REPO_URL/archive/$BRANCH.tar.gz" | tar xz -C "$TMP_DIR"
    subdir=$(ls -1 "$TMP_DIR" | head -1)
    mv "$TMP_DIR/$subdir" "$dest"
    rm -rf "$TMP_DIR"
  fi
}

if [ "$BOOTSTRAP_DIFF" = "1" ] || [ "$BOOTSTRAP_DIFF" = "true" ]; then
  echo "Режим diff: скачивание во временный каталог..."
  STAGING_DIR=$(mktemp -d)
  do_download "$STAGING_DIR"
  if [ -d "$INSTALL_DIR" ] && [ -f "$INSTALL_DIR/install.sh" ]; then
    echo ""
    echo "--- diff install.sh (новое vs текущее) ---"
    diff -u "$INSTALL_DIR/install.sh" "$STAGING_DIR/install.sh" 2>/dev/null || true
    if [ -f "$INSTALL_DIR/install-selfhost.sh" ] && [ -f "$STAGING_DIR/install-selfhost.sh" ]; then
      echo "--- diff install-selfhost.sh ---"
      diff -u "$INSTALL_DIR/install-selfhost.sh" "$STAGING_DIR/install-selfhost.sh" 2>/dev/null || true
    fi
    echo "--- конец diff ---"
    echo ""
    printf "Применить и запустить установку? [y/N] "
    read -r confirm </dev/tty 2>/dev/null || true
    case "$confirm" in
      [yY][eE][sS]|[yY]) ;;
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
COMMIT_HASH=""
if [ -d ".git" ]; then
  COMMIT_HASH=$(git rev-parse HEAD 2>/dev/null || true)
fi
if [ -n "$COMMIT_HASH" ]; then
  echo "Версия: $COMMIT_HASH"
fi
if [ -n "$EXPECTED_SHA256" ] && [ -n "$COMMIT_HASH" ]; then
  case "$COMMIT_HASH" in
    $EXPECTED_SHA256*) ;;
    *)
      echo "Ошибка: ожидался commit $EXPECTED_SHA256, получен $COMMIT_HASH"
      exit 1
      ;;
  esac
fi

INSTALL_AUTO=1 INSTALL_MODE="${INSTALL_MODE:-selfhost}" ./install.sh

echo ""
echo "Установка завершена. Данные: $INSTALL_DIR${COMMIT_HASH:+ (commit $COMMIT_HASH)}"
