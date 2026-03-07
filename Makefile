.PHONY: up down build check ui-install ui-build dev-shell

# Production
up:
	docker compose up -d --build --force-recreate --remove-orphans

down:
	docker compose down

# Dev: compile Go inside container
build:
	docker compose run --rm dev go build ./...

check:
	docker compose run --rm dev go vet ./...

# Dev: install and build UI inside container
ui-install:
	docker compose run --rm dev sh -c "cd ui && pnpm install"

ui-build:
	docker compose run --rm dev sh -c "cd ui && VITE_API_BASE=/api pnpm run build"

# Dev: open a shell inside the dev container
dev-shell:
	docker compose run --rm dev sh

# Dev: download Go modules inside container
mod-tidy:
	docker compose run --rm dev go mod tidy
