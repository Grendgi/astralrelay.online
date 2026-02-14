#!/usr/bin/env sh
# Полная настройка сервера для Docker
# Запуск: curl -fsSL https://raw.githubusercontent.com/Grendgi/astralrelay.online/main/deploy/setup-server.sh | sh
# или: ./deploy/setup-server.sh
#
# Устанавливает Docker и Docker Compose на чистый Linux (Ubuntu/Debian/CentOS/Alpine).
# Требует root или sudo.

set -e

echo "=== Настройка сервера для Docker ==="

# Проверка root
if [ "$(id -u)" -ne 0 ]; then
  echo "Требуются права root."
  if command -v sudo >/dev/null 2>&1; then
    echo "Запустите: sudo $0 $*"
  else
    echo "Войдите как root и запустите снова."
  fi
  exit 1
fi

# Определение дистрибутива
detect_os() {
  if [ -f /etc/os-release ]; then
    . /etc/os-release
    echo "$ID"
  elif [ -f /etc/alpine-release ]; then
    echo "alpine"
  else
    echo "unknown"
  fi
}

install_wireguard_ubuntu_debian() {
  if command -v wg >/dev/null 2>&1; then return 0; fi
  echo "Установка WireGuard..."
  apt-get update -qq
  apt-get install -y -qq wireguard-tools 2>/dev/null || true
}

install_docker_ubuntu_debian() {
  echo "Установка Docker (Ubuntu/Debian)..."
  apt-get update -qq
  apt-get install -y -qq ca-certificates curl
  install -m 0755 -d /etc/apt/keyrings
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
  chmod a+r /etc/apt/keyrings/docker.gpg
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
    > /etc/apt/sources.list.d/docker.list
  apt-get update -qq
  apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
}

install_docker_debian() {
  echo "Установка Docker (Debian)..."
  apt-get update -qq
  apt-get install -y -qq ca-certificates curl
  install -m 0755 -d /etc/apt/keyrings
  curl -fsSL https://download.docker.com/linux/debian/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
  chmod a+r /etc/apt/keyrings/docker.gpg
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/debian $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
    > /etc/apt/sources.list.d/docker.list
  apt-get update -qq
  apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
}

install_wireguard_alpine() {
  if command -v wg >/dev/null 2>&1; then return 0; fi
  apk add --no-cache wireguard-tools 2>/dev/null || true
}

install_docker_alpine() {
  echo "Установка Docker (Alpine)..."
  apk add --no-cache docker docker-cli-compose
  install_wireguard_alpine
  rc-update add docker boot 2>/dev/null || true
  service docker start 2>/dev/null || true
}

install_docker_centos_rhel() {
  echo "Установка Docker (CentOS/RHEL/Rocky)..."
  yum install -y yum-utils
  yum-config-manager -y --add-repo https://download.docker.com/linux/centos/docker-ce.repo
  yum install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
  systemctl enable --now docker
}

# Node.js для генерации VAPID без Docker (опционально)
install_node_ubuntu_debian() {
  if command -v node >/dev/null 2>&1; then return 0; fi
  echo "Установка Node.js..."
  apt-get install -y -qq nodejs npm 2>/dev/null || true
  if ! command -v npx >/dev/null 2>&1 && command -v node >/dev/null 2>&1; then
    npm install -g npx 2>/dev/null || true
  fi
}

# Основная логика
OS=$(detect_os)
echo "Обнаружен: $OS"

if command -v docker >/dev/null 2>&1 && (docker compose version >/dev/null 2>&1 || docker-compose version >/dev/null 2>&1); then
  echo "Docker и Docker Compose уже установлены."
  docker --version
  docker compose version
  echo ""
  echo "Проверка: docker run --rm hello-world"
  docker run --rm hello-world
  echo ""
  echo "Готово. Можно разворачивать мессенджер."
  exit 0
fi

case "$OS" in
  ubuntu)
    install_docker_ubuntu_debian
    install_wireguard_ubuntu_debian
    ;;
  debian)
    install_docker_debian
    install_wireguard_ubuntu_debian
    ;;
  alpine)
    install_docker_alpine
    ;;
  centos|rhel|rocky|fedora)
    install_docker_centos_rhel
    ;;
  *)
    echo "Неизвестный дистрибутив: $OS"
    echo ""
    echo "Установите Docker вручную:"
    echo "  https://docs.docker.com/engine/install/"
    echo ""
    echo "Или используйте официальный скрипт:"
    echo "  curl -fsSL https://get.docker.com | sh"
    exit 1
    ;;
esac

# Проверка
if ! command -v docker >/dev/null 2>&1; then
  echo "Ошибка: Docker не установлен."
  exit 1
fi

# Запуск Docker (для не-systemd)
if ! systemctl is-active --quiet docker 2>/dev/null; then
  systemctl start docker 2>/dev/null || true
  service docker start 2>/dev/null || true
fi

echo ""
echo "Проверка установки..."
docker --version
docker compose version 2>/dev/null || docker-compose --version
docker run --rm hello-world

# Node.js (для VAPID без Docker) — Ubuntu/Debian
if [ "${INSTALL_NODE:-1}" != "0" ] && { [ "$OS" = "ubuntu" ] || [ "$OS" = "debian" ]; }; then
  install_node_ubuntu_debian 2>/dev/null || true
fi

echo ""
echo "=== Готово ==="
echo "Docker и Docker Compose установлены."
echo ""
echo "Добавить пользователя в группу docker (опционально):"
echo "  sudo usermod -aG docker \$USER"
echo "  # затем перелогиниться"
echo ""
echo "Следующий шаг — развёртывание мессенджера:"
echo "  ./deploy/selfhost/run.sh   # свой инстанс"
echo "  ./deploy/main/run.sh       # hub-сервер"
