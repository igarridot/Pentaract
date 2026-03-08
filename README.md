# Pentaract

[![Tests](https://img.shields.io/github/actions/workflow/status/igarridot/Pentaract/tests.yml?style=plastic&logo=github)](https://github.com/igarridot/Pentaract/actions/workflows/tests.yml)
[<img alt="Dockerhub latest" src="https://img.shields.io/badge/dockerhub-latest-blue?logo=docker&style=plastic">](https://hub.docker.com/r/norbega/pentaract)
[<img alt="Docker Image Size (tag)" src="https://img.shields.io/docker/image-size/norbega/pentaract/latest?style=plastic&logo=docker&color=gold">](https://hub.docker.com/r/thedominux/pentaract/tags?page=1&name=latest)
[<img alt="Any platform" src="https://img.shields.io/badge/platform-any-green?style=plastic&logo=linux&logoColor=white">](https://github.com/igarridot/Pentaract)

Pentaract is a self-hosted cloud storage that uses **Telegram as the storage backend**.
Files are split into **20 MB chunks**, uploaded through Telegram bot workers, and reassembled on download.

- **Backend:** Go 1.24 (Chi router, pgx)
- **Frontend:** React 18 + Vite + Material UI 5
- **Database:** PostgreSQL 15
- **Platforms:** `linux/amd64`, `linux/arm64`

## Features

- **Chunked upload/download** with parallel worker scheduling and Telegram rate-limit handling.
- **Real-time progress** (SSE) for uploads, downloads, and deletes.
- **Multiple concurrent operations** in UI, each with its own progress indicator.
- **Per-operation cancellation** for uploads and downloads.
- **Media preview** in the browser:
  - Images: inline preview.
  - Videos (MP4, WebM, OGG, MOV, M4V): inline playback with byte-range seeking.
- **File management:** upload, download, move, rename, delete, create folders, search.
- **Directory download** as ZIP.
- **Access control:** read / write / admin per storage, with user grants.
- **Multi-worker model:** multiple Telegram bots, optional per-storage assignment, rate-limit aware scheduling.

## Support

If you would like to support this project

**BTC**: `bc1qgx8f76qy3eekfhtsr9eauqvjt30utvts4r8n4h`

**ETH**: `0x032C9ABEb3055ae5E0e58df94a7309823e70eBcB`

## Prerequisites

- Docker and Docker Compose

## Quick Start

```bash
cp .env.example .env
```

Edit `.env` — at minimum change:

| Variable | Required change |
|---|---|
| `SECRET_KEY` | Replace with a random value (`openssl rand -hex 32`) |
| `SUPERUSER_EMAIL` | Initial admin email |
| `SUPERUSER_PASS` | Initial admin password |
| `DATABASE_PASSWORD` | Non-default password |

```bash
make up
```

Then configure Telegram:

1. Create a Telegram channel (private recommended).
2. Forward a channel message to [@RawDataBot](https://t.me/RawDataBot) and note `message.forward_origin.chat.id`.
3. Remove the `-100` prefix — use the remaining number as `chat_id` in Pentaract.
4. Create one or more Telegram bots via [@BotFather](https://t.me/BotFather).
5. Add bots as channel admins (with send and delete permissions).
6. In the Pentaract UI (`http://localhost:8000`):
   - Create a **Storage** with the channel `chat_id`.
   - Add **Workers** with the bot tokens.

Stop:

```bash
make down
```

## Architecture

```text
Browser  -->  Go API  -->  PostgreSQL (metadata)
                   |
                   v
            Telegram Bot API (chunk storage)
```

1. User uploads a file.
2. Backend splits it into 20 MB chunks.
3. Bot workers upload chunks to Telegram in parallel.
4. PostgreSQL stores file metadata and chunk references.
5. On download/preview, chunks are fetched from Telegram and reassembled.

## Configuration

All settings are via environment variables (see `.env.example`):

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8000` | HTTP port |
| `WORKERS` | `4` | Application worker goroutines |
| `SECRET_KEY` | *required* | JWT signing key |
| `SUPERUSER_EMAIL` | *required* | Initial admin email |
| `SUPERUSER_PASS` | *required* | Initial admin password |
| `ACCESS_TOKEN_EXPIRE_IN_SECS` | `31536000` | JWT token TTL in seconds |
| `TELEGRAM_API_BASE_URL` | `https://api.telegram.org` | Telegram API base URL |
| `TELEGRAM_RATE_LIMIT` | `18` | Max requests/minute per bot |
| `DATABASE_USER` | `pentaract` | PostgreSQL user |
| `DATABASE_PASSWORD` | `pentaract` | PostgreSQL password |
| `DATABASE_NAME` | `pentaract` | PostgreSQL database name |
| `DATABASE_HOST` | `db` | PostgreSQL host |
| `DATABASE_PORT` | `5432` | PostgreSQL port |

## Development

### Make targets

| Command | Description |
|---|---|
| `make up` | Build and start the production stack |
| `make down` | Stop and remove containers |
| `make build` | Compile Go inside dev container |
| `make check` | Run `go vet` inside dev container |
| `make test` | Run Go + UI tests inside dev container |
| `make mod-tidy` | Run `go mod tidy` inside dev container |
| `make ui-install` | Install UI dependencies inside dev container |
| `make ui-build` | Build UI bundle inside dev container |
| `make dev-shell` | Open a shell inside the dev container |

### Typical workflow

```bash
cp .env.example .env
make mod-tidy
make ui-install
make up
```

Follow logs:

```bash
docker compose logs -f pentaract
```

## Testing

```bash
# Containerized (recommended)
make test

# Local
go test ./...
cd ui && pnpm test
```

CI runs automatically via GitHub Actions (`.github/workflows/tests.yml`).

Dependency updates are automated with Updatecli (`.github/workflows/update-dependencies.yml`), covering Go modules, npm packages, Docker images, and GitHub Actions versions.

## Persistent Data

Data is stored in `persistent_data/`:

| Path | Purpose |
|---|---|
| `persistent_data/db/` | PostgreSQL data |
| `persistent_data/go-mod-cache/` | Go module cache (dev) |
| `persistent_data/go-build-cache/` | Go build cache (dev) |

Reset all local data:

```bash
make down
rm -rf persistent_data/
```

## API

All endpoints except login and register require `Authorization: Bearer <token>`.

### Auth & Users

| Method | Path |
|---|---|
| `POST` | `/api/auth/login` |
| `POST` | `/api/users` |

### Storages

| Method | Path |
|---|---|
| `GET` | `/api/storages` |
| `POST` | `/api/storages` |
| `GET` | `/api/storages/{storageID}` |
| `DELETE` | `/api/storages/{storageID}` |

### Access

| Method | Path |
|---|---|
| `GET` | `/api/storages/{storageID}/access` |
| `POST` | `/api/storages/{storageID}/access` |
| `DELETE` | `/api/storages/{storageID}/access` |

### Storage Workers

| Method | Path |
|---|---|
| `GET` | `/api/storage_workers` |
| `POST` | `/api/storage_workers` |
| `PUT` | `/api/storage_workers/{workerID}` |
| `DELETE` | `/api/storage_workers/{workerID}` |
| `GET` | `/api/storage_workers/has_workers?storage_id=` |

### Files

| Method | Path |
|---|---|
| `POST` | `/api/storages/{storageID}/files/create_folder` |
| `POST` | `/api/storages/{storageID}/files/move` |
| `POST` | `/api/storages/{storageID}/files/upload` |
| `GET` | `/api/storages/{storageID}/files/tree/*` |
| `GET` | `/api/storages/{storageID}/files/download/*` |
| `GET` | `/api/storages/{storageID}/files/download_dir/*` |
| `GET` | `/api/storages/{storageID}/files/search/*` |
| `DELETE` | `/api/storages/{storageID}/files/*` |

### Progress & Cancellation

| Method | Path |
|---|---|
| `GET` | `/api/upload_progress?upload_id=` |
| `GET` | `/api/download_progress?download_id=` |
| `GET` | `/api/delete_progress?delete_id=` |
| `POST` | `/api/upload_cancel/{uploadID}` |
| `POST` | `/api/download_cancel/{downloadID}` |

## Project Layout

```text
cmd/pentaract/          Entry point
internal/
  config/               Environment-based configuration
  domain/               Models and error types
  handler/              HTTP handlers and middleware
  repository/           PostgreSQL queries
  service/              Business logic and storage manager
  server/               Router setup and static file serving
  startup/              DB creation, migrations, superuser
  telegram/             Telegram Bot API client
  jwt/                  JWT generation and validation
  password/             Bcrypt hashing
ui/
  src/
    api/                API client and SSE helpers
    common/             Shared utilities (auth, theme, etc.)
    components/         Reusable UI components
    layouts/            Page layouts
    pages/              Route pages
```

## Kudos

Inspired on the [Pentaract](https://github.com/Dominux/Pentaract) by Dominux.
To support him:

**BTC**: `18mquj59AcB4y4VBevdn5HekG5y7gvPYGk`

**TON**: `UQDoGRgUIEDA30cko8k-icnI8S5i8QIq2jFvqswNvVUc9F2U`

**USDT TON**: `UQDoGRgUIEDA30cko8k-icnI8S5i8QIq2jFvqswNvVUc9F2U`
