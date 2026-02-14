# VPN API — самообслуживание VPN

Любой авторизованный пользователь управляет своими конфигами. Админов нет.

Базовый URL: `https://{domain}/api/v1`

---

## Эндпоинты

### 1. Список протоколов

```
GET /vpn/protocols
```

**Response 200:**

```json
{
  "protocols": [
    {"id": "wireguard", "name": "WireGuard", "hint": "Быстро, рекомендуется..."},
    {"id": "openvpn-tcp443", "name": "OpenVPN (TCP 443)", "hint": "Если WireGuard заблокирован..."},
    {"id": "vmess", "name": "VMess", "hint": "Xray, обходит DPI..."},
    {"id": "vless", "name": "VLESS", "hint": "Xray VLESS, легче VMess."},
    {"id": "trojan", "name": "Trojan", "hint": "Xray Trojan, маскируется под HTTPS."}
  ]
}
```

Xray (vmess, vless, trojan) появляется при `VPN_XRAY_ENABLED=true` и заданном `VPN_XRAY_ENDPOINT`.

### 2. Список нод

```
GET /vpn/nodes
```

Каждая нода может содержать `ping_url` (вычисляется из endpoint) — для клиентского пинга и сортировки по задержке.

**Response 200:**

```json
{
  "nodes": [
    {
      "id": "uuid",
      "name": "Default",
      "region": "",
      "wireguard_endpoint": "vpn.example.org:51820",
      "wireguard_server_pubkey": "base64...",
      "openvpn_endpoint": "vpn.example.org:443",
      "is_default": true,
      "ping_url": "https://vpn.example.org/"
    }
  ]
}
```

### 3. Мои конфиги

```
GET /vpn/my-configs
```

Список VPN-конфигов текущего пользователя (все его устройства). Только свои.

**Response 200:**

```json
{
  "configs": [
    {
      "device_id": "uuid",
      "protocol": "wireguard",
      "node_name": "Default",
      "created_at": "2025-02-13T12:00:00Z",
      "expires_at": "2025-03-15T12:00:00Z",
      "is_expired": false,
      "traffic_rx_bytes": 1048576,
      "traffic_tx_bytes": 524288
    }
  ]
}
```

### 4. Скачать конфиг

```
GET /vpn/config/{protocol}?node_id={uuid}&format=json
```

| Параметр | Описание |
|----------|----------|
| protocol | wireguard, openvpn-tcp443, vmess, vless, trojan |
| node_id | (опционально) UUID ноды |
| format | (опционально) json — вернуть JSON |

**Response 200:** бинарный конфиг или JSON.

### 5. Отозвать конфиг

```
POST /vpn/revoke?protocol={proto}&device_id={uuid}
```

Отзывает свой конфиг. device_id опционален — по умолчанию текущее устройство из JWT.

**Response 200:** `{"status": "ok"}`

---

## Ноды из env

Для multi-node без админки — задайте `VPN_NODES_JSON` (JSON array):

```json
[
  {"name":"Amsterdam","region":"EU","wireguard_endpoint":"ams.vpn:51820","wireguard_server_pubkey":"base64...","openvpn_endpoint":"ams.vpn:443","xray_endpoint":"ams.vpn:443","is_default":false},
  {"name":"Tokyo","region":"APAC","wireguard_endpoint":"tok.vpn:51820","wireguard_server_pubkey":"base64...","xray_endpoint":"tok.vpn:443","is_default":false}
]
```

При старте ноды upsert'ятся по `name`. Default-нода из миграции (с пустыми endpoint = config) остаётся; можно переопределить через JSON.

**Xray** включается переменными:

- `VPN_XRAY_ENABLED=true`
- `VPN_XRAY_ENDPOINT=host:port` — точка входа (VMess по умолчанию)
- `VPN_XRAY_API_ADDR=xray:10085` — gRPC API для AddUser/RemoveUser (пусто = без API, конфиги всё равно выдаются)
- `VPN_XRAY_VMESS_PORT`, `VPN_XRAY_VLESS_PORT`, `VPN_XRAY_TROJAN_PORT` — порты в URL (0 = из Endpoint). Для direct Xray: 10443, 10444, 10445

---

## Отказоустойчивость (HA)

- Состояние VPN (vpn_peers, vpn_nodes) хранится в PostgreSQL
- При нескольких репликах server (docker-compose.ha.yml) все инстансы используют общую БД
- Нет in-memory состояния — любой инстанс обрабатывает любой запрос
- При падении одного сервера трафик переходит на другие за балансировщиком (Traefik)
