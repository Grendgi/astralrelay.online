#!/usr/bin/env sh
# Format: Go (gofmt, goimports) and shell (shfmt)
set -e
cd "$(dirname "$0")/.."

echo "==> Go: gofmt"
gofmt -s -w ./server/...

if command -v goimports >/dev/null 2>&1; then
  echo "==> Go: goimports"
  goimports -w -local github.com/messenger ./server/
else
  echo "==> goimports not found (go install golang.org/x/tools/cmd/goimports@latest)"
fi

if [ -d web/node_modules ] && [ -f web/package.json ]; then
  if grep -q '"format"' web/package.json 2>/dev/null; then
    echo "==> Web: npm run format"
    (cd web && npm run format) 2>/dev/null || true
  fi
fi

if command -v shfmt >/dev/null 2>&1; then
  echo "==> Shell: shfmt"
  for f in install.sh install-selfhost.sh bootstrap.sh scripts/*.sh; do [ -f "$f" ] && shfmt -w -i 2 "$f"; done 2>/dev/null || true
else
  echo "==> shfmt not found (install from https://github.com/mvdan/sh)"
fi

echo "==> Done"
