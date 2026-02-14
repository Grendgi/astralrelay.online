# Push-уведомления и VAPID

## Обзор

Push использует Web Push API и требует пару ключей VAPID (Voluntary Application Server Identification).

## Генерация VAPID-ключей

### С Node.js

```bash
npx web-push generate-vapid-keys
```

### Без Node.js (через Docker)

```bash
docker run --rm node:20-alpine npx web-push generate-vapid-keys
```

Пример вывода:

```
Public Key:
BAg8wG6Fu12dhA2_TMqZzXGAuAwHy5AdfSbAP4jlOyf4G03sowOfNGo_O1i4-AedcnEXI5ni_CXYOuGZTf-CItI

Private Key:
gMx_jtMbaVQcQEdspBwbnAmwLcYAOBXgUAHsyhZv-ZU
```

## Настройка

1. Добавьте ключи в `.env`:

   ```
   PUSH_VAPID_PUBLIC_KEY=BAg8wG6Fu12dhA2_...
   PUSH_VAPID_PRIVATE_KEY=gMx_jtMbaVQcQEdspBwbnAmwLcYAOBXgUAHsyhZv-ZU
   ```

2. Перезапустите server.

3. В веб-клиенте появится кнопка включения push (📱).

## Требования

- **HTTPS** или **localhost** — браузеры не разрешают push по HTTP (кроме localhost)
- Поддерживаемые браузеры: Chrome, Firefox, Edge, Safari 16+

## Проверка

```bash
# Без ключей push отключён
curl -s -H "Authorization: Bearer TOKEN" https://YOUR_DOMAIN/api/v1/push/vapid-public
# {"public_key":""} или 401

# С ключами — возвращается публичный ключ
# {"public_key":"BAg8wG6Fu12dhA2_..."}
```

## Безопасность

- **Приватный ключ** храните только на сервере, не добавляйте в репозиторий
- Используйте отдельные ключи для dev и production
