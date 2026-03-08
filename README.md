# Pentaract

[![Tests](https://img.shields.io/github/actions/workflow/status/igarridot/Pentaract/tests.yml?style=plastic&logo=github)](https://github.com/igarridot/Pentaract/actions/workflows/tests.yml)

Pentaract is a cloud storage system that uses **Telegram as the storage backend**.
Files are split into **20 MB chunks**, uploaded through Telegram bot workers, and reassembled on download/stream.

Stack:
- **Backend:** Go 1.24 (Chi, pgx)
- **Frontend:** React 18 + Vite + MUI 5
- **Database:** PostgreSQL 15
- **Architectures:** `amd64`, `arm64`

## Project Status

Current implementation includes:
- Chunked upload/download with worker scheduling and Telegram rate-limit handling.
- Real-time progress (SSE) for upload, download, and delete.
- **Multiple concurrent uploads/downloads in UI**, each with its own progress card.
- Per-operation cancellation (upload/download) with backend cancellation endpoints.
- File and directory deletion with Telegram-first cleanup and progress reporting.
- Media preview in UI:
  - Images: inline preview.
  - Videos: inline playback with byte-range support for seeking and progressive buffering.
- Download as single file or full directory ZIP.
- CI workflow running Go and UI tests.

Notes:
- MKV playback depends on browser codec support. Container support alone does not guarantee playback for all MKV files.

## Credits

This project is based on the original [Pentaract](https://github.com/Dominux/Pentaract) idea.
The current codebase was rewritten, but the core concept comes from the original author.

## Prerequisites

- Docker
- Docker Compose

## Quick Start

```bash
# 1) Create env file
cp .env.example .env
```

Edit `.env` at minimum:

| Variable | Required change |
|---|---|
| `SECRET_KEY` | Replace `XXX` with a random value (`openssl rand -hex 32`) |
| `SUPERUSER_EMAIL` | Initial admin email |
| `SUPERUSER_PASS` | Initial admin password |
| `DATABASE_PASSWORD` | Non-default password |

```bash
# 2) Build and start
make up
```

Then:
1. Create a Telegram channel (private recommended).
2. Forward a channel message to [@RawDataBot](https://t.me/RawDataBot) and get `message.forward_origin.chat.id`.
3. Remove `-100` prefix and use the remaining value as `chat_id` in Pentaract.
4. Create Telegram bots in [@BotFather](https://t.me/BotFather).
5. Add bots as channel admins (send/delete permissions as needed).
6. In Pentaract UI:
   - Create a Storage with that `chat_id`.
   - Add one or more Workers with bot tokens.

Default URL: `http://localhost:8000`

Stop services:

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

High-level flow:
1. User uploads file.
2. Backend splits into 20 MB chunks.
3. Workers upload chunks to Telegram.
4. DB stores metadata (`files`, `file_chunks`, access, workers).
5. Download/preview reassembles from Telegram chunks.

## Persistent Data

Persistent data is stored in `persistent_data/`:

| Path | Purpose |
|---|---|
| `persistent_data/db/` | PostgreSQL data |
| `persistent_data/go-mod-cache/` | Go module cache (dev) |
| `persistent_data/go-build-cache/` | Go build cache (dev) |

On Linux, an `init-perms` one-shot service prepares writable permissions for bind-mounted paths.

Reset all local data:

```bash
make down
rm -rf persistent_data/
```

## Configuration

Environment variables (main ones):

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8000` | API/UI port |
| `WORKERS` | `4` | App worker count |
| `SECRET_KEY` | required | JWT signing secret |
| `SUPERUSER_EMAIL` | required | Initial admin user |
| `SUPERUSER_PASS` | required | Initial admin password |
| `ACCESS_TOKEN_EXPIRE_IN_SECS` | `1800` | JWT TTL |
| `TELEGRAM_API_BASE_URL` | `https://api.telegram.org` | Telegram API base |
| `TELEGRAM_RATE_LIMIT` | `18` | Requests/minute per bot worker |
| `DATABASE_USER` | `pentaract` | DB user |
| `DATABASE_PASSWORD` | `pentaract` | DB password |
| `DATABASE_NAME` | `pentaract` | DB name |
| `DATABASE_HOST` | `db` | DB host |
| `DATABASE_PORT` | `5432` | DB port |

## Development

### Make targets

| Command | Description |
|---|---|
| `make up` | Build and start stack |
| `make down` | Stop and remove containers |
| `make build` | Run `go build ./...` in dev container |
| `make check` | Run `go vet ./...` in dev container |
| `make test` | Run backend + UI tests in dev container |
| `make mod-tidy` | Run `go mod tidy` in dev container |
| `make ui-install` | Install UI deps in dev container |
| `make ui-build` | Build UI bundle in dev container |
| `make dev-shell` | Open shell in dev container |

### Typical workflow

```bash
cp .env.example .env
make mod-tidy
make ui-install
make up
```

Follow backend logs:

```bash
docker compose logs -f pentaract
```

## Testing

### Recommended (containerized)

```bash
make test
```

Runs inside `dev` container:
- `go test ./...`
- `cd ui && npm run test`

### Local (optional)

```bash
go test ./...
cd ui && npm run test
```

## CI

GitHub Actions workflow: `.github/workflows/tests.yml`

Jobs:
- **Go Tests**: `go test ./...`
- **UI Tests**: `pnpm install --frozen-lockfile && pnpm test`

## Feature Overview

### File operations
- Upload file
- Download file
- Download directory as ZIP
- Create folder
- Move file/folder
- Delete file/folder
- Search within path

### Progress and cancellation
- Upload progress SSE: bytes, chunks, worker status
- Download progress SSE: bytes, chunks, worker status
- Delete progress SSE: chunk deletion progress
- Cancel upload and download in-flight
- Multiple concurrent uploads/downloads shown independently in UI

### Media preview
- Image preview inline
- Video preview inline (range-based)
- Seek support through byte ranges
- Progressive buffering behavior driven by browser range requests

### Access model
- Access levels: read (`r`), write (`w`), admin (`a`)
- Per-storage grants/revokes

### Worker model
- Multiple bot workers
- Optional worker-storage assignment
- Scheduler-aware rate limit waiting status (`active` / `waiting_rate_limit`)

## API Endpoints

All endpoints except login/register require `Authorization: Bearer <token>`.

### Auth & users

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

### Storage workers

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

### Progress and cancellation

| Method | Path |
|---|---|
| `GET` | `/api/upload_progress?upload_id=` |
| `GET` | `/api/download_progress?download_id=` |
| `GET` | `/api/delete_progress?delete_id=` |
| `POST` | `/api/upload_cancel/{uploadID}` |
| `POST` | `/api/download_cancel/{downloadID}` |

## Project Layout

```text
cmd/pentaract/main.go
internal/
  config/
  domain/
  repository/
  service/
  handler/
  telegram/
  server/
  startup/
  password/
  jwt/
ui/
  src/
    api/
    common/
    components/
    layouts/
    pages/
```

## License

Based on the original [Pentaract](https://github.com/Dominux/Pentaract) by Dominux.
