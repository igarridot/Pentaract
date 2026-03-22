.PHONY: up down build check test ui-install ui-build dev-shell dev-up backup-now

COMPOSE := docker compose --project-name pentaract

# Production
up:
	$(COMPOSE) up -d --build --force-recreate --remove-orphans

down:
	$(COMPOSE) down

# Dev: compile Go inside container
build:
	$(COMPOSE) run --rm dev sh -c "go build ./cmd/... ./internal/..."

check:
	$(COMPOSE) run --rm dev sh -c "go vet ./cmd/... ./internal/..."

test:
	$(COMPOSE) --profile dev run --rm dev sh -c "go test ./cmd/... ./internal/... && cd ui && npm run test"

# Dev: install and build UI inside container
ui-install:
	$(COMPOSE) run --rm dev sh -c "cd ui && pnpm install"

ui-build:
	$(COMPOSE) run --rm dev sh -c "cd ui && VITE_API_BASE=/api pnpm run build"

# Dev: open a shell inside the dev container
dev-shell:
	$(COMPOSE) run --rm dev sh

# Dev: start DB in background and run API + UI in development mode
dev-up:
	$(COMPOSE) up -d init-perms db
	$(COMPOSE) --profile dev run --rm --service-ports dev sh -c "set -e; cd ui; [ -d node_modules ] || pnpm install; pnpm dev --host 0.0.0.0 --port 3000 & UI_PID=$$!; trap 'kill $$UI_PID' EXIT; cd /app; go run ./cmd/pentaract"

# Dev: download Go modules inside container
mod-tidy:
	$(COMPOSE) run --rm dev go mod tidy

# Backup: run a one-off database backup immediately
backup-now:
	$(COMPOSE) run --rm db-backup /usr/local/bin/db-backup.sh
