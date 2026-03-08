# Pentaract

Cloud storage system that uses **Telegram as the storage backend**. Files are split into 20 MB chunks, uploaded to Telegram channels via bot workers, and reassembled on download.

Built with **Go 1.24** (Chi, pgx) + **React 18** (Material UI 5) + **PostgreSQL 15**. Supports **amd64** and **arm64** architectures.

## Credits

Before continuing, please note that this project is based on the original [Pentaract](https://github.com/Dominux/Pentaract) idea. I have rewrote the full code as the original seems abandoned, but the core concept is derived from the original. All credit for the initial idea and implementation goes to the original author.

## Prerequisites

Docker and Docker Compose

## Quick start

```bash
# 1. Create your environment file
cp .env.example .env
```

Edit `.env` and set at minimum:

| Variable | What to change |
|----------|---------------|
| `SECRET_KEY` | Replace `XXX` with a random string (`openssl rand -hex 32`) |
| `SUPERUSER_EMAIL` | Admin account email |
| `SUPERUSER_PASS` | Admin account password |
| `DATABASE_PASSWORD` | Something other than the default |

```bash
# 2. Build and start
make up
```

1. Create a Telegram channel (private recommended)
2. Send a message in the channel and forward it to [@RawDataBot](https://t.me/RawDataBot) to get the channel's `message.forward_origin.chat.id`. Remove heading "-100"
3. Create one or more Telegram bots via [@BotFather](https://t.me/BotFather) and save their tokens
4. Add the bots as administrators of the channel (they need permission to post messages)
5. In Pentaract:
   - Go to **Storages** and create a storage with the channel's `chat_id`
   - Go to **Workers** and create a worker with each bot token
6. Upload files through the **Files** browser

The application will be available at **http://localhost:8000** (or whatever `PORT` you configured).

To stop:

```bash
make down
```

---

## How it works

```
Browser  -->  Go HTTP server  -->  PostgreSQL (metadata)
                    |
                    v
              Telegram Bot API (file storage)
```

1. Users create **Storages**, each linked to a Telegram channel via its `chat_id`
2. Users register **Storage Workers** (Telegram bots) and optionally bind them to storages
3. On **upload**, files are split into 20 MB chunks and uploaded to the Telegram channel in parallel using available bot workers
4. On **download**, chunks are fetched from Telegram in parallel and reassembled in order
5. A **rate limiter** tracks bot usage per minute and selects the least-loaded available worker

### What `make up` does

1. Builds a multi-stage Docker image (`Dockerfile`):
   - Stage 1: Cross-compiles the Go binary for the target architecture (`golang:1.24-alpine`)
   - Stage 2: Builds the React frontend (`node:22-slim` + Vite)
   - Stage 3: Copies both into a minimal `scratch` image
2. Starts PostgreSQL 15 with a health check
3. Starts the Pentaract server once the database is healthy
4. The server automatically creates the database schema and the superuser on first boot

### Persistent data

All persistent data is stored under `persistent_data/` in the project root:

| Directory | Contents |
|-----------|----------|
| `persistent_data/db/` | PostgreSQL database files |
| `persistent_data/go-mod-cache/` | Go module cache (dev only) |
| `persistent_data/go-build-cache/` | Go build cache (dev only) |

On Linux, compose runs a one-shot `init-perms` service before PostgreSQL that prepares these directories and sets writable permissions to avoid UID/GID mismatches.

To reset all data:

```bash
make down
rm -rf persistent_data/
```

### Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8000` | Port exposed to the host |
| `WORKERS` | `4` | Max concurrent requests (throttle) |
| `SECRET_KEY` | *required* | Secret for JWT signing (use `openssl rand -hex 32`) |
| `SUPERUSER_EMAIL` | *required* | Email for the initial admin account |
| `SUPERUSER_PASS` | *required* | Password for the initial admin account |
| `ACCESS_TOKEN_EXPIRE_IN_SECS` | `1800` | JWT token lifetime in seconds |
| `TELEGRAM_API_BASE_URL` | `https://api.telegram.org` | Telegram Bot API base URL |
| `TELEGRAM_RATE_LIMIT` | `18` | Max requests per minute per bot worker |
| `DATABASE_USER` | `pentaract` | PostgreSQL user |
| `DATABASE_PASSWORD` | `pentaract` | PostgreSQL password |
| `DATABASE_NAME` | `pentaract` | PostgreSQL database name |
| `DATABASE_HOST` | `db` | PostgreSQL host (use `db` for the compose service) |
| `DATABASE_PORT` | `5432` | PostgreSQL port |

---

## Development

The project includes a `dev` service that provides a containerized Go + Node toolchain. Source code is mounted as a volume, so edits on the host are reflected immediately.

Build caches are persisted under `persistent_data/` so subsequent builds are fast.

### Make targets

| Command | Description |
|---------|-------------|
| `make up` | Build and start the full production stack |
| `make down` | Stop and remove all containers |
| `make build` | Compile Go backend inside a container |
| `make check` | Run `go vet` inside a container |
| `make mod-tidy` | Run `go mod tidy` inside a container |
| `make ui-install` | Install frontend npm dependencies inside a container |
| `make ui-build` | Build the production frontend bundle inside a container |
| `make dev-shell` | Open an interactive shell in the dev container |

### Workflow

```bash
# 1. Create your .env
cp .env.example .env
# Edit SECRET_KEY at minimum

# 2. Download dependencies
make mod-tidy
make ui-install

# 3. Build and run
make up

# 4. Check logs
docker compose logs -f pentaract
```

### Frontend hot-reload

```bash
# Start backend + database
make up

# Start Vite dev server in a container
docker compose run --rm -p 3000:3000 dev sh -c "cd ui && pnpm run dev"
```

The Vite dev server proxies `/api` requests to the backend.

---

## Features

### File management
- **Upload** files of any size with real-time progress tracking (SSE) and cancellation support
- **Download** individual files or entire directories as ZIP archives
- **Create folders**, **move** files/folders, and **delete** items
- **Search** files by name within any directory
- **Duplicate handling** — uploading a file with an existing name automatically appends a numeric suffix

### Storage workers
- Register multiple Telegram bots as workers for parallel chunk transfers
- Workers can be assigned to specific storages or left available for all
- Built-in rate limiter respects Telegram's per-bot API limits

### Access control
- Three access levels: **read** (r), **write** (w), and **admin** (a)
- Admins can grant and revoke access per storage
- Storage owners automatically get admin access

### UI
- Minimalist design with Inter font, frosted glass effects, and pill-shaped buttons
- **Dark mode** with three options: Light, Dark, and Auto (follows system preference), persisted to localStorage
- Responsive layout with collapsible sidebar

### Multi-architecture
The Docker image supports both `linux/amd64` and `linux/arm64`. The Go binary is cross-compiled natively for the target architecture.

```bash
docker buildx build --platform linux/amd64,linux/arm64 -t pentaract .
```

---

## Project structure

```
cmd/pentaract/main.go          Entry point
internal/
  config/                      Environment configuration
  domain/                      Models and error types
  repository/                  Database queries (raw SQL + pgx)
  service/                     Business logic
  handler/                     HTTP handlers and middleware
  telegram/                    Telegram Bot API client
  server/                      Router, CORS, static file serving
  startup/                     Database creation, migrations, superuser seed
  password/                    bcrypt hashing
  jwt/                         JWT generation and validation
ui/                            React frontend (Vite + MUI 5)
  src/
    api/                       API client (fetch-based)
    common/                    Auth guard, theme context, utilities
    components/                Reusable UI components
    layouts/                   Page layouts
    pages/                     Route pages
```

## API endpoints

All endpoints except login and register require an `Authorization: Bearer <token>` header.

### Auth & Users

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/auth/login` | Login, returns JWT |
| POST | `/api/users` | Register new user |

### Storages

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/storages` | List storages |
| POST | `/api/storages` | Create storage |
| GET | `/api/storages/:id` | Get storage details |
| DELETE | `/api/storages/:id` | Delete storage |

### Access control

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/storages/:id/access` | List access entries |
| POST | `/api/storages/:id/access` | Grant access |
| DELETE | `/api/storages/:id/access` | Revoke access |

### Storage workers

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/storage_workers` | List workers |
| POST | `/api/storage_workers` | Create worker |
| PUT | `/api/storage_workers/:id` | Update worker |
| DELETE | `/api/storage_workers/:id` | Delete worker |
| GET | `/api/storage_workers/has_workers?storage_id=` | Check if storage has workers |

### Files

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/storages/:id/files/create_folder` | Create folder |
| POST | `/api/storages/:id/files/move` | Move file or folder |
| POST | `/api/storages/:id/files/upload` | Upload file (multipart) |
| GET | `/api/storages/:id/files/tree/*` | Browse directory |
| GET | `/api/storages/:id/files/download/*` | Download file |
| GET | `/api/storages/:id/files/download_dir/*` | Download directory as ZIP |
| GET | `/api/storages/:id/files/search/*` | Search files |
| DELETE | `/api/storages/:id/files/*` | Delete file or folder |
| GET | `/api/upload_progress?upload_id=` | SSE stream with upload progress |
| POST | `/api/upload_cancel/:id` | Cancel an in-flight upload |

---

## License

Based on the original [Pentaract](https://github.com/Dominux/Pentaract) by Dominux.
