# Traefik — шаблон (email из LETSENCRYPT_EMAIL)
# Генерация: envsubst < traefik.yml.tpl > traefik.yml
#
# Access logs: по умолчанию отключены. Если включить — RequestPath: drop (см. docs/SECURITY-HARDENING.md)

api:
  dashboard: true
  insecure: true

entryPoints:
  web:
    address: ":80"
  websecure:
    address: ":443"

certificatesResolvers:
  letsencrypt:
    acme:
      email: "${LETSENCRYPT_EMAIL:-changeme@example.com}"
      storage: "/letsencrypt/acme.json"
      httpChallenge:
        entryPoint: web

providers:
  docker:
    endpoint: "unix:///var/run/docker.sock"
    exposedByDefault: false
