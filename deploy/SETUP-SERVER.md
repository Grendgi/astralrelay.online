# Настройка сервера для Docker

Скрипт и инструкции для подготовки **чистого** Linux-сервера без Docker.

---

## Автоматическая установка

```bash
# Скачать и запустить (требует root/sudo)
curl -fsSL https://raw.githubusercontent.com/Grendgi/astralrelay.online/main/deploy/setup-server.sh | sh
```

Или из клонированного репозитория:

```bash
sudo ./deploy/setup-server.sh
```

### Поддерживаемые дистрибутивы

| ОС | Пакеты |
|----|--------|
| Ubuntu | docker-ce, docker-compose-plugin |
| Debian | docker-ce, docker-compose-plugin |
| CentOS / RHEL / Rocky / Fedora | docker-ce, docker-compose-plugin |
| Alpine | docker, docker-cli-compose |

---

## Ручная установка

### Ubuntu 22.04 / 24.04

```bash
sudo apt-get update
sudo apt-get install -y ca-certificates curl
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc

echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
```

### Debian 12

```bash
sudo apt-get update
sudo apt-get install -y ca-certificates curl
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc

echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
```

### Alpine

```bash
sudo apk add docker docker-cli-compose
sudo rc-update add docker boot
sudo service docker start
```

### CentOS / Rocky / RHEL

```bash
sudo yum install -y yum-utils
sudo yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
sudo yum install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo systemctl enable --now docker
```

### Универсальный скрипт (официальный)

```bash
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
# Перелогиниться
```

---

## После установки

### Проверка

```bash
docker --version
docker compose version
docker run --rm hello-world
```

### Права пользователя (без sudo)

```bash
sudo usermod -aG docker $USER
# Выйти из сессии и войти снова
```

### Открытие портов (для продакшена)

| Порт | Назначение |
|------|------------|
| 80 | HTTP (Main с Traefik) |
| 443 | HTTPS (при настройке SSL) |
| 3000 | Web (selfhost) |
| 8080 | API (selfhost) |

Пример для ufw (Ubuntu):

```bash
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw allow 3000/tcp
sudo ufw allow 8080/tcp
sudo ufw enable
```

---

## Полный цикл: от нуля до работающего мессенджера

```bash
# 1. Установка Docker
sudo ./deploy/setup-server.sh

# 2. Клонирование (или скачивание)
git clone https://github.com/Grendgi/astralrelay.online.git
cd astralrelay.online

# 3. Self-host
cp deploy/selfhost/.env.example deploy/selfhost/.env
nano deploy/selfhost/.env   # JWT_SECRET, пароли

./deploy/selfhost/run.sh

# Готово: http://YOUR_IP:3000
```
