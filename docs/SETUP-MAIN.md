# Полная инструкция: Главный сервер (Hub)

Главный сервер — центральный узел федеративной сети. Две реплики API (server+server2), Traefik, Let's Encrypt, PostgreSQL, Redis, MinIO, Xray VPN.

---

## 1. Требования

| Требование | Минимум |
|------------|---------|
| ОС | Linux (Ubuntu 22+, Debian 12+, Alpine, CentOS) |
| RAM | 2 GB |
| Диск | 10 GB |
| Порты | 80, 443 (обязательно), 8082 (Traefik dashboard), **9443** (coordinator для mesh) |
| Домен | Свой домен или IP для nip.io |

**При установке через `bootstrap.sh` или `install.sh`** Docker и UFW настраиваются автоматически (порты 22, 80, 443, 8082, 9443). Ручная подготовка нужна только при развёртывании без этих скриптов.

---

## 2. Подготовка сервера (при ручной установке)

### 2.1 Установка Docker

```bash
sudo ./deploy/setup-server.sh
```

Или: https://docs.docker.com/engine/install/

### 2.2 Фаервол

```bash
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw allow 8082/tcp
sudo ufw allow 9443/tcp   # coordinator (mesh)
sudo ufw enable
```

---

## 3. Развёртывание

### 3.1 Одна команда (рекомендуется)

```bash
curl -fsSL https://raw.githubusercontent.com/Grendgi/astralrelay.online/main/bootstrap.sh | sudo sh
```

В мастере выбрать **1** (main), ввести домен. Docker, UFW, секреты — автоматически. С переменными: `INSTALL_MODE=main REPO_URL=https://... sudo -E ./bootstrap.sh`

### 3.2 Интерактивная установка

```bash
git clone https://github.com/Grendgi/astralrelay.online.git
cd astralrelay.online
sudo ./install.sh
```

Выберите режим `1` (main), домен или IP, email для Let's Encrypt.

### 3.3 Ручная настройка

```bash
cp deploy/main/.env.example deploy/main/.env
nano deploy/main/.env
```

Заполните: SERVER_DOMAIN, JWT_SECRET, POSTGRES_PASSWORD, MINIO_ROOT_PASSWORD, LETSENCRYPT_EMAIL.

```bash
./deploy/main/run.sh
```

---

## 4. DNS

A-запись: SERVER_DOMAIN → IP сервера. Для nip.io DNS уже настроен.

---

## 5. Проверка

- Сайт: https://YOUR_DOMAIN
- Health: https://YOUR_DOMAIN/health
- Federation: https://YOUR_DOMAIN/.well-known/federation
- Smoke: `./scripts/smoke-test.sh https://YOUR_DOMAIN`

---

## 6. HA (реплика PostgreSQL)

```bash
docker compose -p main -f deploy/main/docker-compose.yml --env-file deploy/main/.env --profile ha up -d
```

---

## 7. Обновление

```bash
git pull
./deploy/main/run.sh
```

---

## 8. Push-уведомления

```bash
docker run --rm node:20-alpine npx web-push generate-vapid-keys
```

Добавьте в .env: PUSH_VAPID_PUBLIC_KEY, PUSH_VAPID_PRIVATE_KEY. Перезапустите.

---

## 9. Mesh и mTLS (опционально)

**Coordinator** всегда поднимается на main (порт **9443**, `docker-compose.mesh.yml`). Токен для подключения selfhost: `curl -s http://YOUR_DOMAIN:9443/v1/token`. Если домен за **Cloudflare** (оранжевое облако), порт 9443 снаружи недоступен — используйте запись с **DNS only** (серое облако) или отдельную A-запись на IP. Подробнее: [RUN-MAIN.md](RUN-MAIN.md).

При настройке mTLS `install.sh` может генерировать CA в `deploy/main/mesh-ca/`; coordinator выдаёт mTLS-сертификаты при join. Self-host получают сертификаты через `scripts/setup-mesh.sh`.

---

## 10. Архитектура

Traefik :80/:443 → / (web) и /api, /federation (server+server2). Coordinator :9443 (отдельный контейнер, не за Traefik). Балансировка между server и server2. PostgreSQL, Redis, MinIO, Xray — общая инфраструктура. Другие инстансы (self-host) подключаются по федерации. Mesh: coordinator :9443, backup-receiver :9100 (при mesh).
