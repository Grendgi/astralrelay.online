# Subdomain главного домена

Self-host узлы получают адрес `SUBDOMAIN.MAIN_DOMAIN` (например `1-2-3-4.astralrelay.online`). Трафик идёт через Traefik на главном сервере.

---

## Требования

- **Main** — astralrelay.online (или ваш домен). Coordinator (порт 9443) всегда поднимается на main вместе с Traefik.
- **DNS** — wildcard `*.astralrelay.online` → IP главного сервера
- **Let's Encrypt** — LETSENCRYPT_EMAIL на main для выдачи сертификатов поддоменам

---

## Установка self-host в subdomain режиме

Токен запрашивается автоматически — пользователю ничего копировать не нужно.

```bash
INSTALL_ADDRESS_MODE=subdomain MAIN_DOMAIN=astralrelay.online sudo ./install.sh
```

Режим: 2 (selfhost). Получите адрес `X-Y-Z-W.astralrelay.online` по вашему IP.

---

## Как это работает

1. Self-host регистрируется в coordinator с `use_subdomain=1`, `main_domain=astralrelay.online`
2. Coordinator создаёт маршрут в Traefik (File provider): `Host(1-2-3-4.astralrelay.online)` → `http://10.100.0.x:9080`
3. Self-host поднимает **gateway** (nginx на :9080), проксирующий на server и web
4. Main Traefik принимает запросы на `*.astralrelay.online`, маршрутизирует по VPN на gateway self-host

---

## Режимы адреса

| Режим | Адрес | Трафик |
|-------|-------|--------|
| **subdomain** | 1-2-3-4.astralrelay.online | Через main Traefik |
| **standalone** | 1.2.3.4.nip.io, свой домен, Cloudflare Tunnel | Напрямую на self-host |
