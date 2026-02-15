# AstralRelay — единая точка входа для разработки и сборки

.PHONY: dev up down fmt lint deps migrate clean build

# Install web dependencies (required for make lint / make build)
deps:
	cd web && npm install

# Dev: поднять compose (миграции выполняются при старте server)
dev:
	@./deploy/dev/run.sh
	@echo "Dev: http://localhost:3000 (web), http://localhost:8080 (API)"
# Main hub
up-main:
	./deploy/main/run.sh

# Self-host
up-selfhost:
	./deploy/selfhost/run.sh

# Down (dev)
down:
	docker compose -p dev -f deploy/dev/docker-compose.yml down 2>/dev/null || true

# Format: Go + Shell
fmt:
	@chmod +x scripts/fmt.sh 2>/dev/null || true
	./scripts/fmt.sh

# Lint: Go + Web + Shell
lint:
	@chmod +x scripts/lint.sh 2>/dev/null || true
	./scripts/lint.sh

# Migrate (требует DATABASE_URL или deploy/dev/.env)
migrate:
	@if [ -f deploy/dev/.env ]; then set -a; . deploy/dev/.env; set +a; fi; \
	export DATABASE_URL=$${DATABASE_URL:-postgres://messenger:messenger_dev@localhost:5432/messenger?sslmode=disable}; \
	cd server && go run ./cmd/migrate

# Clean: остановить compose, удалить volumes
clean:
	docker compose -p dev -f deploy/dev/docker-compose.yml down -v 2>/dev/null || true
	rm -rf web/node_modules/.vite 2>/dev/null || true
	@echo "clean done"

# Build server + web
build:
	cd server && go build -o /dev/null ./...
	cd web && npm run build
