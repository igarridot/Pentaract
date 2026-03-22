# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Pentaract is a self-hosted file storage service that uses Telegram channels as distributed chunk storage. Go backend (Chi router, PostgreSQL via pgx), React frontend (Vite, MUI 7). Files are split into ~19.9 MB chunks, encrypted with AES-256-GCM, and stored as Telegram messages.

## Commands

```bash
# Development (live reload: Go + Vite)
make dev-up              # API on :8000, UI on :3000

# Production
make up                  # Docker Compose full stack
make down

# Tests
make test                # Go + UI tests in container
go test ./...            # Go tests locally
cd ui && pnpm test       # UI tests locally (node:test, not vitest)

# Single Go test
go test -run TestName ./internal/service/

# Build
make build               # Go binary in container
cd ui && pnpm run build  # Frontend build

# Backup
make backup-now          # Run a one-off DB backup immediately

# Other
make check               # go vet
make dev-shell           # Shell in dev container
make ui-install          # pnpm install in container
```

## Architecture

### Wire-up: `internal/server/server.go`

All dependency injection happens in `New()`:
Repositories → Telegram Client → WorkerScheduler → StorageManager → Services → Handlers → Chi Router

### Key data flow

**Upload**: Handler reads multipart stream → `io.Pipe` → `StorageManager.Upload` reads chunks from pipe → encrypts each chunk (AES-256-GCM) → uploads 10 chunks in parallel to Telegram → verifies each chunk by re-downloading and SHA-256 hashing → saves to DB.

**Download**: Handler → `StorageManager.DownloadToWriter` → lists chunks from DB → downloads in parallel (1 goroutine per worker) → decrypts → writes to `http.ResponseWriter` in order via `runOrderedJobs`.

**Progress tracking**: All long operations (upload/download/delete) support real-time SSE via `pollSSE()` helper in `progress_tracker.go`. Frontend subscribes before starting the operation.

### Packages

- `internal/telegram/` — Telegram Bot API client with rate-limit retry (`doWithRateLimitRetry`), transient error backoff
- `internal/service/` — Business logic. `StorageManager` is the core: `chunk_uploader.go`, `chunk_downloader.go`, `chunk_deleter.go`, `chunk_crypto.go`
- `internal/handler/` — HTTP handlers. Upload/download split into `upload_handler.go`, `download_handler.go`. SSE polling unified in `progress_tracker.go`
- `internal/repository/` — PostgreSQL queries. `files.go` has the complex path-based queries (ListDir, Search, CreateFileAnyway with dedup)

### Frontend

- `ui/src/api/` — API client with shared SSE subscription (`subscribeAuthSSE`)
- `ui/src/pages/Files/` — Main file browser, split into hooks: `useUploads`, `useDownloads`, `useDeleteOperation`, `useBulkOperations`, `useFileNavigation`
- `ui/src/components/ProgressCard.jsx` — Shared progress UI used by all 4 progress components

## Configuration

All via environment variables (see `.env.example`). Key ones:

- `SECRET_KEY` — Used for both JWT signing AND chunk encryption key derivation (PBKDF2, 600k iterations). Changing it breaks all existing encrypted files.
- `TELEGRAM_RATE_LIMIT` — Requests per minute per worker (default 18)
- `WORKERS` — DB connection pool size multiplier

## Testing patterns

**All new features and bug fixes must include tests.** Backend changes need Go tests; frontend changes need JS tests. Do not submit code without corresponding test coverage.

**Go handlers**: Mock service interfaces with function fields (`mockFilesService`), use `httptest`. See `internal/handler/files_handler_test.go`.

**Go services**: Fake repository interfaces. See `internal/service/files_test.go`.

**Frontend**: Node.js built-in `test` module (NOT vitest). Run with `pnpm test` which calls `node --test`.

## Key constants (`internal/service/constants.go`)

| Constant | Value | Purpose |
|----------|-------|---------|
| `UploadChunkSize` | ~19.9 MB | Plaintext chunk size before encryption |
| `UploadChunkParallelism` | 10 | Concurrent chunk uploads per file |
| `UploadChunkMaxAttempts` | 5 | Retries per chunk upload |
| `DownloadChunkMaxAttempts` | 3 | Retries per chunk download |
| `VerifyChunkParallelism` | 10 | Max parallel verification downloads |
| `DeleteParallelism` | 5 | Concurrent Telegram message deletions |
| `SSEPollingInterval` | 500ms | Progress event frequency |

## CI/CD

Push to `master` → Go tests + UI tests → auto-tag (semver patch bump) → multi-arch Docker image → Docker Hub (`norbega/pentaract:latest`).

## Gotchas

- `crypto.randomUUID()` not available in insecure HTTP contexts — `ui/src/common/operation_id.js` has a fallback using `crypto.getRandomValues()`
- No HTTP read/write timeouts on the server (large transfers can take hours) — per-request context cancellation instead
- Download auth via `?access_token=` query param (for iframe-based downloads) — only allowed on `/files/download/` and `/files/download_dir/` paths
- DB migrations run automatically on startup (`internal/startup/startup.go`)
- `ui/dist/` is gitignored — Docker builds it fresh; local dev uses Vite dev server
