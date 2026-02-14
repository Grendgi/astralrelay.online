# Тестирование

## Smoke-тесты

Быстрая проверка работающего инстанса (health, federation, API).

```bash
# Против локального dev-сервера (порт 8080)
./scripts/smoke-test.sh http://localhost:8080

# Против production
./scripts/smoke-test.sh https://your-domain.org
```

Проверяются:
- `GET /health` — статус сервера
- `GET /.well-known/federation` — discovery федерации
- `POST /api/v1/auth/login` — API принимает запросы
- `GET /api/v1/vpn/protocols` — VPN endpoint (401 без токена)
- `GET /api/v1/push/vapid-public` — Push endpoint (401 без токена)

## E2E (Playwright)

Для полного E2E чата, push и VPN можно использовать Playwright.

```bash
cd web
npm init playwright@latest  # при первом запуске
npx playwright test
```

Рекомендуется создать `web/e2e/` с тестами:
- `chat.spec.ts` — регистрация, отправка сообщения, получение
- `push.spec.ts` — подписка на push, проверка уведомления
- `vpn.spec.ts` — получение конфига VPN (при включённом Xray)

## Запуск против Docker

```bash
cd deploy/main
docker compose up -d
../../scripts/smoke-test.sh http://localhost  # из корня: ./scripts/smoke-test.sh
```
