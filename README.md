# Pentaract

Cloud storage system that uses **Telegram as the storage backend**. Files are split into 20 MB chunks, uploaded to Telegram channels via bot workers, and reassembled on download.

Built with **Go 1.24** (Chi, pgx) + **React 18** (Material UI 5) + **PostgreSQL 15**.

## Prerequisites

- Docker and Docker Compose

Nothing else needs to be installed on the host machine. All compilation, dependency management, and execution happens inside containers.

## Quick start (production)

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

The application will be available at **http://localhost:8000** (or whatever `PORT` you configured).

To stop:

```bash
make down
```

### What `make up` does

1. Builds a multi-stage Docker image (`Dockerfile`):
   - Stage 1: Compiles the Go binary (`golang:1.24-alpine`)
   - Stage 2: Builds the React frontend (`node:21-slim` + Vite)
   - Stage 3: Copies both into a minimal `scratch` image
2. Starts PostgreSQL 15 with a health check
3. Starts the Pentaract server once the database is healthy
4. The server automatically creates the database schema and the superuser on first boot

### Production environment variables

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

The project includes a `dev` service that provides a containerized Go + Node toolchain. Your source code is mounted as a volume, so edits on the host are reflected immediately. **No local Go or Node installation required.**

### Available make targets

| Command | Description |
|---------|-------------|
| `make build` | Compile Go backend inside a container |
| `make check` | Run `go vet` inside a container |
| `make mod-tidy` | Run `go mod tidy` inside a container |
| `make ui-install` | Install frontend npm dependencies inside a container |
| `make ui-build` | Build the production frontend bundle inside a container |
| `make dev-shell` | Open an interactive shell in the dev container |
| `make up` | Build and start the full production stack |
| `make down` | Stop and remove all containers |

### Development workflow

```bash
# 1. Create your .env (same as production)
cp .env.example .env
# Edit SECRET_KEY at minimum

# 2. First time: download dependencies
make mod-tidy
make ui-install

# 3. Verify everything compiles
make build

# 4. Build the frontend
make ui-build

# 5. Run the full stack in production mode to test
make up

# 6. Check logs
docker compose logs -f pentaract
```

### Interactive development shell

If you need to run arbitrary commands (tests, debugging, installing new packages):

```bash
make dev-shell
```

This drops you into a container with Go 1.24, Node, and pnpm available. Your project directory is mounted at `/app`. Examples of what you can do inside:

```bash
# Run Go tests
go test ./...

# Add a new Go dependency
go get github.com/some/package
go mod tidy

# Run the Vite dev server (for frontend hot-reload)
cd ui && pnpm run dev

# Run a one-off Go build
go build -o pentaract ./cmd/pentaract
```

### Frontend development with hot-reload

For frontend iteration with hot module replacement:

```bash
# 1. Start the backend + database in production mode
make up

# 2. Open a dev shell and start the Vite dev server
make dev-shell
cd ui && pnpm run dev
```

The Vite dev server runs on port 3000 inside the container and proxies API requests to `http://localhost:8000`. To access it from the host, add a port mapping to the `dev` service or use `docker compose run --rm -p 3000:3000 dev sh -c "cd ui && pnpm run dev"`.

---

## How it works

### Architecture

```
Browser  -->  Go HTTP server  -->  PostgreSQL (metadata)
                    |
                    v
              Telegram Bot API (file storage)
```

1. Users create **Storages**, each linked to a Telegram channel via its `chat_id`
2. Users register **Storage Workers** (Telegram bots created via @BotFather) and bind them to storages
3. On **upload**, files are split into 20 MB chunks and uploaded to the Telegram channel in parallel using available bot workers
4. On **download**, chunks are fetched from Telegram in parallel and reassembled in order
5. A **rate limiter** tracks bot usage per minute and selects the least-loaded worker under the configured limit

### Setting up Telegram storage

1. Create a Telegram channel (private recommended)
2. Get the channel's numeric ID (you can use bots like `@userinfobot` or the Telegram API)
3. Create one or more Telegram bots via [@BotFather](https://t.me/BotFather) and save their tokens
4. Add the bots as administrators of the channel (they need permission to post messages)
5. In Pentaract:
   - Go to **Storage Workers** and create a worker with each bot token, binding it to a storage
   - Go to **Storages** and create a storage with the channel's `chat_id`
6. Upload files through the **Files** browser

### Project structure

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
```

### API endpoints

```
POST   /api/auth/login                              Login
POST   /api/users                                   Register

GET    /api/storages                                 List storages
POST   /api/storages                                 Create storage
GET    /api/storages/:id                             Get storage
DELETE /api/storages/:id                             Delete storage

GET    /api/storages/:id/access                      List access
POST   /api/storages/:id/access                      Grant access
DELETE /api/storages/:id/access                      Revoke access

GET    /api/storage_workers                          List workers
POST   /api/storage_workers                          Create worker
GET    /api/storage_workers/has_workers?storage_id=  Check workers

POST   /api/storages/:id/files/create_folder         Create folder
POST   /api/storages/:id/files/upload                Upload file
POST   /api/storages/:id/files/upload_to             Upload to path
GET    /api/storages/:id/files/tree/*                 Browse directory
GET    /api/storages/:id/files/download/*             Download file
GET    /api/storages/:id/files/search/*               Search files
DELETE /api/storages/:id/files/*                      Delete file/folder
```

All endpoints except login and register require a `Authorization: Bearer <token>` header.

---

## License

Based on the original [Pentaract](https://github.com/Dominux/Pentaract) by Dominux.
