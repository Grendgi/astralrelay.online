#!/usr/bin/env bash
# Smoke test: проверка основных endpoints работающего инстанса.
# Использование: ./scripts/smoke-test.sh [BASE_URL]
# Пример: ./scripts/smoke-test.sh https://example.org
#         ./scripts/smoke-test.sh http://localhost:8080

set -e
BASE_URL="${1:-http://localhost:8080}"
BASE_URL="${BASE_URL%/}"

echo "=== Smoke test: $BASE_URL ==="

# 1. Health
echo -n "GET /health ... "
resp=$(curl -sf "$BASE_URL/health")
if echo "$resp" | grep -q '"status":"ok"'; then
  echo "OK"
else
  echo "FAIL: $resp"
  exit 1
fi

# 2. Federation discovery
echo -n "GET /.well-known/federation ... "
resp=$(curl -sf "$BASE_URL/.well-known/federation")
if echo "$resp" | grep -q 'federation_endpoint' && echo "$resp" | grep -q 'server_key'; then
  echo "OK"
else
  echo "FAIL: $resp"
  exit 1
fi

# 3. Login (API принимает запросы)
echo -n "POST /api/v1/auth/login (invalid) ... "
status=$(curl -sf -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/api/v1/auth/login" \
  -H "Content-Type: application/json" -d '{"username":"__smoke__","password":"x"}')
if [ "$status" = "401" ] || [ "$status" = "400" ]; then
  echo "OK (expected $status)"
else
  echo "unexpected status $status"
fi

# 4. VPN protocols (401 без токена)
echo -n "GET /api/v1/vpn/protocols (no auth) ... "
status=$(curl -sf -o /dev/null -w "%{http_code}" "$BASE_URL/api/v1/vpn/protocols")
if [ "$status" = "401" ]; then
  echo "OK (401 без токена)"
else
  echo "status $status"
fi

# 5. Push VAPID (401 без токена)
echo -n "GET /api/v1/push/vapid-public (no auth) ... "
status=$(curl -sf -o /dev/null -w "%{http_code}" "$BASE_URL/api/v1/push/vapid-public")
if [ "$status" = "401" ]; then
  echo "OK (401 без токена)"
else
  echo "status $status"
fi

echo "=== Smoke test passed ==="
