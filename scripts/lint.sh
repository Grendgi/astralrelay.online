#!/usr/bin/env sh
# Lint: Go (go vet), Web (eslint), Shell (shellcheck)
set -e
cd "$(dirname "$0")/.."

err=0

echo "==> Go: go vet"
go vet ./server/... || err=1

if command -v shellcheck >/dev/null 2>&1; then
  echo "==> Shell: shellcheck"
  for f in install.sh install-selfhost.sh bootstrap.sh; do
    shellcheck -x "$f" 2>/dev/null || err=1
  done
else
  echo "==> shellcheck not found (skip)"
fi

if [ -d web/node_modules ]; then
  echo "==> Web: npm run lint"
  (cd web && npm run lint) || err=1
else
  echo "==> Web: run 'make deps' first (skip lint)"
fi

[ $err -eq 0 ] && echo "==> Lint OK" || exit 1
