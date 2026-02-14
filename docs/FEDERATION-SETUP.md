# Федерация: связь между инстансами

## Обзор

Федерация связывает независимые инстансы мессенджера. Пользователи разных доменов могут обмениваться сообщениями.

**Модель:** как email — каждый домен независим, связь идёт по HTTPS между серверами.

## Discovery

Каждый инстанс публикует endpoint федерации:

```
GET https://YOUR_DOMAIN/.well-known/federation
```

**Ответ:**
```json
{
  "federation_endpoint": "https://YOUR_DOMAIN/federation/v1",
  "server_key": "base64_ed25519_public_key",
  "version": "0.1"
}
```

Связь между инстансами **автоматическая**: при отправке сообщения `@bob:friend.org` сервер отправителя сам находит `friend.org` по DNS, запрашивает `/.well-known/federation` и отправляет транзакцию. Ручная привязка не требуется.

## Требования

1. **Публичный HTTPS** — оба инстанса должны быть доступны по `https://domain`
2. **DNS** — A/AAAA записи домена на IP сервера
3. **Порты** — 80, 443 открыты для входящих

## Проверка

```bash
# Проверить discovery своего инстанса
curl -s https://YOUR_DOMAIN/.well-known/federation | jq

# Проверить discovery удалённого инстанса
curl -s https://FRIEND_DOMAIN/.well-known/federation | jq
```

## Маршрутизация трафика

| Путь | Описание |
|------|----------|
| `/.well-known/federation` | Discovery, возвращает federation_endpoint и server_key |
| `/federation/v1/*` | S2S API (keys, transaction, media) |
| `/api/*` | C2S API (клиент ↔ сервер) |

В `docker-compose` Traefik проксирует `/api`, `/.well-known`, `/federation` на server.

## Подробности

- [S2S API](./api/s2s-api.md) — endpoints и аутентификация
- [Защита федерации](./FEDERATION-SECURITY.md) — rate limit, blocklist, mTLS, webhook
- [HA и репликация](./ha-replication.md) — несколько серверов на один домен
