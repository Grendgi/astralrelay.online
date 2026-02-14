# WAF для федерации

Рекомендации по интеграции Web Application Firewall для защиты федеративных эндпоинтов.

---

## Варианты

### 1. Traefik + ModSecurity Plugin

Traefik v3 поддерживает плагины. ModSecurity можно добавить как middleware.

**Требования:** Traefik Plugin (Yaegi) или внешний ModSecurity-proxy.

**Пример** — ModSecurity как отдельный контейнер перед Traefik:
- Nginx с ModSecurity в режиме reverse proxy
- Traefik → nginx-modsec → server

### 2. CrowdSec + Traefik Bouncer

[CrowdSec](https://www.crowdsec.net/) — поведенческий WAF и антибот. Есть bouncer для Traefik.

```yaml
# docker-compose — добавить crowdsec + bouncer
services:
  crowdsec:
    image: crowdsecurity/crowdsec:latest
    volumes:
      - ./crowdsec:/etc/crowdsec
  crowdsec-bouncer-traefik:
    image: crowdsecurity/cs-traefik-bouncer:latest
    environment:
      CROWDSEC_BOUNCER_API_KEY: ${CROWDSEC_API_KEY}
    depends_on:
      - crowdsec
```

**Плюсы:** Лёгкая интеграция, автоматические блокировки по сценариям.

### 3. Nginx + ModSecurity

Если федерация идёт через Nginx (не Traefik):

```nginx
# nginx.conf
modsecurity on;
modsecurity_rules_file /etc/nginx/modsecurity/main.conf;

location /federation/ {
    modsecurity_rules_file /etc/nginx/modsec/federation.conf;
    proxy_pass http://server:8080;
}
```

**OWASP Core Rule Set (CRS):** Рекомендуется для базовой защиты.

### 4. Traefik ForwardAuth + внешний WAF

Трафик к федерации проверяется внешним сервисом:

```yaml
# traefik dynamic
http:
  middlewares:
    waf-auth:
      forwardAuth:
        address: "http://waf-service:8080/check"
        trustForwardHeader: true
  routers:
    api:
      middlewares: [secure-headers, waf-auth]
```

---

## Рекомендации

- **Main hub:** CrowdSec bouncer или ModSecurity на федеративных путях (`/federation`, `/.well-known/federation`).
- **Self-host:** При наличии собственного reverse proxy — добавить ModSecurity/CrowdSec.
- **Правила:** Ограничить payload size (уже есть в приложении), отключить агрессивные CRS-правила для JSON API, чтобы не блокировать легитимные транзакции.

---

## Ограничения приложения

Сервер уже обеспечивает:
- Rate limiting по домену
- Blocklist / allowlist
- Валидация размера body (1 MB)
- Схема транзакций

WAF дополняет защиту на уровне протокола (SQLi, XSS в заголовках, нестандартные атаки).
