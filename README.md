# Pentaract

[![Tests](https://img.shields.io/github/actions/workflow/status/igarridot/Pentaract/tests.yml?style=plastic&logo=github)](https://github.com/igarridot/Pentaract/actions/workflows/tests.yml)
[<img alt="Dockerhub latest" src="https://img.shields.io/badge/dockerhub-latest-blue?logo=docker&style=plastic">](https://hub.docker.com/r/norbega/pentaract)
[<img alt="Any platform" src="https://img.shields.io/badge/platform-any-green?style=plastic&logo=linux&logoColor=white">](https://github.com/igarridot/Pentaract)

Pentaract is a self-hosted storage service that uses **Telegram channels as chunk storage**.
The API and UI provide file management, access control, progress tracking, and worker orchestration.

## Current Project Status

- Backend: Go 1.25 (`chi`, `pgx`)
- Frontend: React 19 + Vite + MUI 7
- Database: PostgreSQL

## Related projects

- [CLI](https://github.com/igarridot/pentaract-cli)
- [Kodi streaming addon](https://github.com/igarridot/Pentaract-kodi)

## What You Get

- Chunked upload/download to Telegram (20 MB-safe encrypted chunks)
- Chunk encryption with **AES-256-GCM** and per-chunk random nonce
- Encryption key derived from `SECRET_KEY` using **PBKDF2-HMAC-SHA256** (600000 iterations)
- Real-time progress via SSE (upload/download/delete), including upload verification
- Upload/download cancellation
- File browser: upload, download, move, create folders, search, preview
- Directory download as a streamed ZIP archive
- Multi-worker support (shared or storage-scoped bots)
- Worker-aware chunk recovery when a different bot needs to download an existing chunk
- Per-storage access control:
  - `read`: browse/download
  - `write`: browse/download/upload/move/create folder
  - `admin`: full management including delete and access grants
- Admin-only user management:
  - list managed users
  - update non-admin passwords
  - delete non-admin users
- Safer delete model with progress and optional `force delete` mode
- **Local filesystem upload**: browse and upload files from the container's local filesystem to Telegram storage via the Web UI

## Quick Start (Production-like)

### 1. Prepare env

```bash
cp .env.example .env
```

Set at least:

- `SECRET_KEY` (generate one: `openssl rand -hex 32`)
- `SUPERUSER_EMAIL`
- `SUPERUSER_PASS`
- `DATABASE_PASSWORD`

### 2. Start stack

```bash
make up
```

App URL: `http://localhost:8000`

### 3. Configure Telegram backend

1. Create a Telegram channel (private recommended).
2. Get channel ID (forward message to `@RawDataBot`).
3. Use the numeric ID without the `-100` prefix as `chat_id`.
4. Create one or more bots via `@BotFather`.
5. Add bots as channel admins (send + delete permissions).
6. In UI:
   - Create storage with `chat_id`
   - Create workers with bot tokens

Stop stack:

```bash
make down
```

## Permissions Model

- `read`: cannot upload or delete
- `write`: can upload/download/manage non-destructive file ops, **cannot delete**
- `admin`: required for delete operations and access changes

Delete behavior:

- Normal delete: removes Telegram chunks + DB metadata, retrying across available workers when needed
- `force delete`: removes DB metadata only, can leave Telegram orphan chunks

## Development Workflow

All heavy tasks can run inside containers.

```bash
# compile backend
make build

# static checks
make check

# backend + frontend tests
make test

# run dev API + UI with live code
make dev-up
```

Useful targets:

- `make dev-shell`
- `make ui-install`
- `make ui-build`
- `make mod-tidy`
- `make backup-now`

## Testing and CI

### Local

Run backend + frontend tests in containers:

```bash
make test
```

### GitHub Actions

- PR validation lives in `.github/workflows/tests.yml`
  - Go job: `go test ./...`
  - UI job: `pnpm test`
- Merge-to-`master` release automation lives in `.github/workflows/release.yml`
  - reruns Go and UI tests
  - creates or reuses the release tag
  - publishes the GitHub release
  - builds and pushes Docker images to Docker Hub as `<tag>` and `latest`
- `.github/workflows/docker-publish-tag.yml` remains available as a manual backfill workflow for publishing Docker images from an existing tag

## Configuration

From `.env`:

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8000` | API + UI port |
| `WORKERS` | `4` | app worker goroutines |
| `SUPERUSER_EMAIL` | required | bootstrap admin |
| `SUPERUSER_PASS` | required | bootstrap admin password |
| `ACCESS_TOKEN_EXPIRE_IN_SECS` | `1800` | JWT TTL |
| `SECRET_KEY` | required | JWT + chunk encryption secret |
| `TELEGRAM_API_BASE_URL` | `https://api.telegram.org` | Telegram API |
| `TELEGRAM_RATE_LIMIT` | `18` | per-worker requests/min guard |
| `DATABASE_USER` | `pentaract` | postgres user |
| `DATABASE_PASSWORD` | `pentaract` | postgres password |
| `DATABASE_NAME` | `pentaract` | postgres db |
| `DATABASE_HOST` | `db` | db host in compose |
| `DATABASE_PORT` | `5432` | db port |
| `LOCAL_UPLOAD_BASE_PATH` | _(empty)_ | Host directory to expose for local filesystem uploads. Mounted read-only at `/mnt/data` inside the container. |
| `BACKUP_RETENTION_DAYS` | `7` | Days to keep old database backups |
| `BACKUP_INTERVAL_SECONDS` | `86400` | Seconds between automatic backups |

## Local Filesystem Upload

Pentaract can upload files directly from the container's filesystem to Telegram storage, useful for migrating large datasets without streaming them through the browser.

### Setup

1. Set `LOCAL_UPLOAD_BASE_PATH` in `.env` to the host directory you want to expose (e.g., `/home/user/media`).
2. Uncomment the volume mount in `docker-compose.yml` — it bind-mounts `LOCAL_UPLOAD_BASE_PATH` read-only at `/mnt/data` inside the container.
3. In the Web UI, navigate to **Local Upload** in the sidebar, select a storage, browse the filesystem, and upload.

The feature is auto-detected: if `/mnt/data` exists inside the container, local uploads are enabled. Batch uploads (up to 100 items) are supported. Each upload gets its own SSE progress stream and can be cancelled individually. Path traversal outside `/mnt/data` is blocked server-side.

## Database Backup

An automated daily backup of the PostgreSQL database is included. Backups run inside a dedicated container, are compressed with gzip, and saved to a bind volume on the host.

### How it works

- The `db-backup` service starts with `make up` alongside the rest of the stack.
- It runs a backup immediately on startup and then every 24 hours.
- Each backup is saved as `pentaract_YYYYMMDD_HHMMSS.sql.gz` in `persistent_data/backups/`.
- Backups older than the retention period (default 7 days) are automatically deleted.
- The container is memory-limited (256 MB) to prevent OOM issues on the host.

### Configuration

| Variable | Default | Notes |
|---|---|---|
| `BACKUP_RETENTION_DAYS` | `7` | Number of days to keep old backups |
| `BACKUP_INTERVAL_SECONDS` | `86400` | Interval between backups (24h) |

### Manual backup

Run a one-off backup at any time:

```bash
make backup-now
```

### Restore

```bash
gunzip -c persistent_data/backups/pentaract_YYYYMMDD_HHMMSS.sql.gz \
  | docker exec -i $(docker ps -qf name=pentaract-db) \
    psql -U pentaract -d pentaract
```

## Persistence and Data

Persistent data lives in `persistent_data/`:

- `persistent_data/db` - PostgreSQL data
- `persistent_data/backups` - Database backups (`.sql.gz`)
- `persistent_data/go-mod-cache` - Go module cache (dev)
- `persistent_data/go-build-cache` - Go build cache (dev)

When `LOCAL_UPLOAD_BASE_PATH` is set, the host directory is bind-mounted read-only at `/mnt/data` inside the container for local uploads.

Compose uses bind mounts to `./persistent_data/*` (not named Docker volumes).
If you used older versions with named volumes, remove them once:

```bash
docker volume ls | grep pentaract
docker volume rm <old_volume_name>
```

Full local reset:

```bash
make down
rm -rf persistent_data/
```

## API Surface (high level)

Main route groups:

- `/api/auth/*`
- `/api/users/*`
- `/api/storages/*`
- `/api/storages/{storageID}/access*`
- `/api/storage_workers/*`
- `/api/storages/{storageID}/files/*`
- `/api/local_fs/*` (local filesystem browsing)
- `/api/*_progress` and cancel endpoints

See `internal/server/server.go` for exact route list.

## Project Layout

```text
cmd/pentaract/          application entry point
internal/
  config/               env config
  domain/               core models/errors
  handler/              HTTP handlers + middleware
  repository/           postgres access
  service/              business logic (storage/workers/permissions)
  server/               router + static UI serving
  startup/              db init + migrations + superuser bootstrap
  telegram/             Telegram Bot API client
  jwt/                  JWT logic
  password/             password hashing
ui/
  src/api/              frontend API + SSE clients
  src/common/           shared frontend utilities
  src/components/       reusable UI components
  src/pages/            route screens (Files, LocalUpload, etc.)
```

## Known Operational Constraints

- Telegram Bot API limits and channel permissions directly affect throughput.
- If workers are misconfigured (missing admin rights/token revoked), uploads/deletes fail.
- Upload completion includes a Telegram round-trip verification phase, so a file can still be "verifying" after its chunks finished uploading.
- `force delete` is irreversible and may leave backend remnants by design.
- Legacy plaintext chunks (from older versions) are still readable for backward compatibility.

## Acknowledgement

Inspired by the original [Pentaract](https://github.com/Dominux/Pentaract) by Dominux.
