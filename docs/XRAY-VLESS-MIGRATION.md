# Xray: миграция на VLESS + XTLS Vision

## Обзор

VMess и Trojan в Xray помечены как deprecated. Рекомендуется использовать **VLESS + XTLS Vision** (flow: `xtls-rprx-vision`).

## Что изменилось

1. **VLESS inbound** — переключён с TLS на XTLS (`streamSettings.security: "xtls"`)
2. **Flow** — при добавлении пользователя через API устанавливается `flow: "xtls-rprx-vision"`
3. **URL для клиентов** — `BuildVLESSURL` добавляет `security=xtls` и `flow=xtls-rprx-vision`
4. **Порядок протоколов** — VLESS отображается первым в панели VPN

## Совместимость

- VMess и Trojan по-прежнему доступны (для обратной совместимости)
- Новые пользователи должны выбирать VLESS
- Клиенты: v2rayN, Nekoray, v2rayNG (проверьте поддержку XTLS Vision)

## Проверка

```bash
# Получить VLESS-конфиг для устройства
curl -H "Authorization: Bearer TOKEN" "https://DOMAIN/api/v1/vpn/config/vless"
# vless://uuid@host:10444?type=tcp&security=xtls&flow=xtls-rprx-vision&sni=...&...
```
