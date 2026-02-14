# Полная инструкция: Главный сервер (Hub)

Главный сервер — центральный узел федеративной сети. Две реплики API (server+server2), Traefik, Let's Encrypt, PostgreSQL, Redis, MinIO, Xray VPN.

---

## 1. Требования

| Требование | Минимум |
|------------|---------|
| ОС | Linux (Ubuntu 22+, Debian 12+, Alpine, CentOS) |
| RAM | 2 GB |
| Диск | 10 GB |
| Порты | 80, 443 (обязательно), 8082 (Traefik dashboard) |
| Домен | Свой домен или IP для nip.io |

---

## 2. Подготовка сервера

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
sudo ufw enable
```

---

## 3. Развёртывание

### 3.1 Одна команда (zero config)

```bash
curl -fsSL https://raw.githubusercontent.com/Grendgi/astralrelay.online/main/bootstrap.sh | sudo sh
```

С переменными: `INSTALL_MODE=main REPO_URL=https://... sudo -E ./bootstrap.sh`

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

При `MESH_ENABLED=1` `install.sh` генерирует CA в `deploy/main/mesh-ca/` и настраивает coordinator для выдачи mTLS-сертификатов при join. Self-host узлы получают сертификаты автоматически через `setup-mesh.sh`.

---

## 10. Архитектура

Traefik :80/:443 → / (web) и /api, /federation (server+server2). Балансировка между server и server2. PostgreSQL, Redis, MinIO, Xray — общая инфраструктура. Другие инстансы (self-host) подключаются по федерации. Mesh: coordinator :9443, backup-receiver :9100.
