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
cd ui && pnpm lint       # ESLint (rules-of-hooks + exhaustive-deps); CI runs it

# Single Go test
go test -run TestName ./internal/service/

# Build
make build               # Go binary in container
cd ui && pnpm run build  # Frontend build

# Backup
make backup-now          # Run a one-off DB backup immediately
make backup-list         # List available backups
make backup-restore BACKUP=<file>  # Restore from a backup

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
- `internal/service/` — Business logic. `StorageManager` is a facade over `ChunkUploader` (`chunk_uploader.go`), `ChunkDownloader` (`chunk_downloader.go`) and `ChunkDeleter` (`chunk_deleter.go`), which share a `chunkTransport` (scheduler, Telegram client, `ChunkCipher`). Build it with `NewStorageManager` (or `newStorageManager(storageDeps{...})` in tests)
- `internal/handler/` — HTTP handlers. `FilesHandler` embeds `UploadHandler` (`upload_handler.go`, `local_fs.go`) and `DownloadHandler` (`download_handler.go`) and adds tree/search/move/delete. SSE polling unified in `progress_tracker.go`. `DeleteTrackers` (delete progress registry) is injected into both the files and storages handlers. Inline previews (`?inline=1`) of any type are served through the chunk range path and never tracked
- `internal/repository/` — PostgreSQL queries. `files.go` has the complex path-based queries (ListDir, Search, CreateFileAnyway with dedup)

### Frontend

- `ui/src/api/` — API client with shared SSE subscription (`subscribeAuthSSE`)
- `ui/src/pages/Files/` — Main file browser, split into hooks: `useUploads`, `useDownloads`, `useDeleteOperation`, `useBulkOperations`, `useFileNavigation`, `useFileSelection`. `useUploadConflicts` (conflict dialog + directory cache) is shared with `pages/LocalUpload`; `common/use_delete_progress.js` is shared with `pages/Storages`
- `common/use_api_action.js` — `run(action, { success, onSuccess, onError })` replaces the try/await/addAlert boilerplate in pages. API failures are `ApiError` with a `status` (`api/request.js`)
- `common/completion_registry.js` — awaitable terminal status per transfer; bulk uploads and bulk downloads use it to run one file at a time
- Dialogs that edit an entity (`EditWorkerDialog`, `GrantAccess`, `RenameFolderDialog`) initialise state from props and are reset by the parent through `key`, not by effects
- Shared components: `Panel` (bordered surface), `PathBreadcrumbs` (Root > a > b), `ProgressCard`
- Pure, node-tested modules hold the logic hooks and components lean on: `common/progress.js`, `common/api_action.js`, `pages/Files/operations.js`, `pages/Files/upload_conflicts.js`, `pages/LocalUpload/local_upload_paths.js`
- Storage paths inside URLs go through `encodePath` in `ui/src/api/index.js` (per-segment `encodeURIComponent`); the server decodes with `url.PathUnescape`
- `ui/src/components/ProgressCard.jsx` — Shared progress UI used by all 4 progress components

## Configuration

All via environment variables (see `.env.example`). Key ones:

- `SECRET_KEY` — JWT signing secret. Also derives the chunk encryption key (PBKDF2, 600k iterations) unless `ENCRYPTION_KEY` is set. Changing it breaks files encrypted with it.
- `ENCRYPTION_KEY` — Optional dedicated chunk encryption secret. `ChunkCipher` keeps `SECRET_KEY` as a decrypt-only fallback so files uploaded before the switch stay readable (`NewChunkCipherWithFallback`).
- `TELEGRAM_RATE_LIMIT` — Requests per minute per worker (default 18)
- `DB_MAX_CONNS` — Postgres pool size (default 32). The legacy `WORKERS` variable is still read as `WORKERS * 8`.

## Testing patterns

**All new features and bug fixes must include tests.** Backend changes need Go tests; frontend changes need JS tests. Do not submit code without corresponding test coverage.

**Go handlers**: Mock service interfaces with function fields (`mockFilesService`), use `httptest`. See `internal/handler/files_handler_test.go`.

**Go services**: Fake repository interfaces. See `internal/service/files_test.go`; `StorageManager` takes `chunksRepository`/`storageGetter` interfaces so upload tests use in-memory fakes (`chunk_uploader_flow_test.go`).

**Frontend**: Node.js built-in `test` module (NOT vitest). Run with `pnpm test` which calls `node --test`.

## Key constants (`internal/service/constants.go`)

| Constant | Value | Purpose |
|----------|-------|---------|
| `UploadChunkSize` | ~19.9 MB | Plaintext chunk size before encryption |
| `UploadChunkParallelism` | 10 | Concurrent chunk uploads per file |
| `UploadChunkMaxAttempts` | 5 | Retries per chunk upload |
| `DownloadChunkMaxAttempts` | 3 | Retries per chunk download |
| `PipelineVerifyParallelism` | 5 | Concurrent verifications during upload pipeline |
| `VerifyCBFailureThreshold` | 3 | Consecutive transient failures to trip circuit breaker |
| `VerifyCBCooldownDuration` | 30s | Pause before retrying after circuit breaker trips |
| `VerifyCBMaxRetryRounds` | 3 | Max cooldown+retry rounds for failed verifications |
| `DeleteParallelism` | 5 | Concurrent Telegram message deletions |
| `SSEPollingInterval` | 500ms | Progress event frequency |
| `DownloadInterruptedGracePeriod` | 2m | How long a download whose connection the browser dropped waits for the browser to re-request it |

## CI/CD

Push to `master` → Go tests + UI tests → auto-tag (semver patch bump) → multi-arch Docker image → Docker Hub (`norbega/pentaract:latest`).

## Gotchas

- `crypto.randomUUID()` not available in insecure HTTP contexts — `ui/src/common/operation_id.js` has a fallback using `crypto.getRandomValues()`
- No HTTP read/write timeouts on the server (large transfers can take hours) — per-request context cancellation instead
- Download auth via `?access_token=` query param (for iframe-based downloads) — only allowed on `/files/download/` and `/files/download_dir/` paths; the access log redacts it
- Progress SSE subscriptions stop on 401/403/404 (`SSE_FATAL_STATUSES` in `ui/src/api/index.js`) and keep reconnecting on other failures
- Over plain HTTP, Chrome blocks downloads of file types outside its safe list (e.g. `.funscript`, `.zip`, `.pdf`) as "insecure" and closes the connection; it re-requests the same URL when the user allows the file. `failTracker` in `download_handler.go` keeps such downloads in an `interrupted` state (SSE status `interrupted`) instead of failing them, so the resumed request continues the same progress card
- The upload handler answers only after the multipart body has been consumed. If a handler starts writing while the request body is unread, `net/http` discards the rest of a body under 256 KB (`maxPostHandlerReadBytes`), which silently truncated every small browser upload to the ~3 KB the multipart parser had buffered. `runTrackedUpload` wraps source read failures in `domain.ErrUploadInterrupted` and `uploadChunks` aborts on any read error other than `io.EOF`/`io.ErrUnexpectedEOF`, so a cut-short source fails the upload instead of persisting a truncated file
- DB migrations run automatically on startup (`internal/startup/startup.go`)
- `ui/dist/` is gitignored — Docker builds it fresh; local dev uses Vite dev server
