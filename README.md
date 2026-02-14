# Федеративный E2EE-мессенджер + VPN

Чаты с end-to-end шифрованием, федерация, VPN (Xray). Децентрализованная сеть — главный сервер + self-host узлы.

## Быстрый старт

```bash
git clone <repo> && cd Chat_VPN
./install.sh
```

Режим (main/selfhost), домен или IP. При вводе IP → `IP.nip.io`. Секреты генерируются автоматически.

## Инструкции

| Роль | Документ |
|------|----------|
| **Главный сервер (Hub)** | [docs/SETUP-MAIN.md](docs/SETUP-MAIN.md) |
| **Self-host** (расширение сети) | [docs/SETUP-SELFHOST.md](docs/SETUP-SELFHOST.md) |
| Dev | [docs/RUN-DEV.md](docs/RUN-DEV.md) |

## Структура

```
├── deploy/main/      # Hub: Traefik, server+server2
├── deploy/selfhost/  # Self-host: nip.io или домен
├── deploy/dev/       # Разработка
├── install.sh
├── server/, web/, xray/
└── docs/
```
