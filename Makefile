.PHONY: up down certs data-dirs build check test ui-install ui-build dev-shell dev-up mod-tidy backup-now backup-restore backup-list

COMPOSE := docker compose --project-name pentaract

# Bind-mounted directories, created as the current user: Docker would
# create missing ones as root, and the dev container (UID 1000) could not
# write its Go caches.
DATA_DIRS := persistent_data/db persistent_data/backups persistent_data/go-mod-cache persistent_data/go-build-cache

# Production
up: certs data-dirs
	$(COMPOSE) up -d --build --force-recreate --remove-orphans

# Self-signed certificate for the nginx HTTPS front, kept if one exists.
# Add the names you browse with: make certs CERT_HOSTS="pentaract.lan 192.168.1.10"
certs:
	sh scripts/gen-self-signed-cert.sh persistent_data/certs $(CERT_HOSTS)

data-dirs:
	mkdir -p $(DATA_DIRS)

down:
	$(COMPOSE) down

# Dev: compile Go inside container
build: data-dirs
	$(COMPOSE) run --rm dev sh -c "go build ./cmd/... ./internal/..."

check: data-dirs
	$(COMPOSE) run --rm dev sh -c "go vet ./cmd/... ./internal/..."

test: data-dirs
	$(COMPOSE) --profile dev run --rm dev sh -c "go test ./cmd/... ./internal/... && cd ui && npm run test"

# Dev: install and build UI inside container
ui-install: data-dirs
	$(COMPOSE) run --rm dev sh -c "cd ui && pnpm install"

ui-build: data-dirs
	$(COMPOSE) run --rm dev sh -c "cd ui && VITE_API_BASE=/api pnpm run build"

# Dev: open a shell inside the dev container
dev-shell: data-dirs
	$(COMPOSE) run --rm dev sh

# Dev: start DB in background and run API + UI in development mode
dev-up: data-dirs
	$(COMPOSE) up -d db
	$(COMPOSE) --profile dev run --rm --service-ports dev sh -c "set -e; cd ui; [ -d node_modules ] || pnpm install; pnpm dev --host 0.0.0.0 --port 3000 & UI_PID=$$!; trap 'kill $$UI_PID' EXIT; cd /app; go run ./cmd/pentaract"

# Dev: download Go modules inside container
mod-tidy: data-dirs
	$(COMPOSE) run --rm dev go mod tidy

# Backup: run a one-off database backup immediately
backup-now: data-dirs
	$(COMPOSE) run --rm db-backup /usr/local/bin/db-backup.sh

# Backup: list available backups
backup-list: data-dirs
	$(COMPOSE) run --rm db-backup /usr/local/bin/db-restore.sh --list

# Backup: restore from a backup file (usage: make backup-restore BACKUP=<filename-or-path>)
backup-restore: data-dirs
	@if [ -z "$(BACKUP)" ]; then echo "Usage: make backup-restore BACKUP=<filename-or-path>"; echo "Run 'make backup-list' to see available backups."; exit 1; fi
	@echo "WARNING: This will stop pentaract and replace the current database with $(BACKUP)."
	@echo "Press Ctrl+C to cancel, or Enter to continue..." && read _
	$(COMPOSE) stop pentaract
	$(COMPOSE) run --rm db-backup /usr/local/bin/db-restore.sh $(BACKUP)
	$(COMPOSE) start pentaract
	@echo "Restore complete. Pentaract restarted."
