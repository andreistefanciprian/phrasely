# Phrasely

A platform for building and sharing English phrase collections, with AI-powered learning via MCP.

## Stack

- **API**: Go + gorilla/mux, slog, pgx/v5
- **Frontend**: React + TailwindCSS (not started)
- **MCP Server**: Go Streamable HTTP (not started)
- **DB**: PostgreSQL 17
- **Infra**: Docker Compose (local), cloud-agnostic container deployment

## Conventions

- `db.Store` interface in `internal/db/db.go` — one central interface, all domain methods added here as we build them
- Concrete implementation: `db.PostgresStore` — holds `*pgxpool.Pool`
- Dependency injection via constructors (urlshortener pattern)
- `docker compose up --build` to run everything; no local Go needed

## API endpoints

| Method | Path | Description |
|---|---|---|
| GET | `/health` | Health check |
| POST | `/api/v1/phrases` | Create a phrase |

## PR log

- **PR 1** — project skeleton: module, Docker Compose, Postgres connection, `db.Store` interface, `/health` endpoint
- **PR 2** — rename project to phrasely across repo, Go module, Postgres credentials
- **PR 3** — goose migrations wired into startup; `phrases` table; SQL files embedded into binary via `migrations/embed.go`
- **PR 4** — `POST /api/v1/phrases` with input validation, unit tests, Taskfile

## Data Model