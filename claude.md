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

## Git workflow

- Every change goes in a PR — never push directly to `main`
- Always ask for review before pushing
- Branch naming: `pr/<number>-<short-description>`

## API endpoints

| Method | Path | Description |
|---|---|---|
| GET | `/health` | Health check |
| GET | `/api/v1/phrases` | List phrases (optional `?keyword=` filter) |
| GET | `/api/v1/phrases/{id}` | Get phrase by ID |
| POST | `/api/v1/phrases` | Create a phrase |
| PATCH | `/api/v1/phrases/{id}` | Update a phrase (partial) |
| DELETE | `/api/v1/phrases/{id}` | Delete a phrase |

## Auth plan (magic link — not started)

1. **Tables**: `users` (id, email, created_at), `magic_link_tokens` (id, user_id, token, expires_at, used_at)
2. `POST /auth/request` — upsert user by email, generate token, send link via email (log to stdout in dev)
3. `GET /auth/verify?token=` — validate token (not expired, not used), mark used, return signed JWT
4. **JWT middleware** — validates `Authorization: Bearer <token>` on all protected routes
5. JWT expiry: 30 days; token expiry: 15 min; tokens are single-use

## Frontend plan (not started)

- **Stack**: Next.js + TailwindCSS + shadcn/ui, deployed on Railway
- **Home (`/bubble`)**: keyword word cloud — bubble size driven by `localStorage` view counts (no DB writes on click)
- **Detail (`/phrases?keyword=`)**: phrase cards filtered by keyword; keyboard navigation (space/arrow to shuffle)
- **Compound keywords**: `keyword` field stores `"word1 vs word2"` as-is; UI splits on ` vs ` to render individual Merriam-Webster links
- **Reference**: PhraseFlow (`/Users/stefanandrei/Documents/Projects/PhraseFlow/`) has working bubble implementation — port the placement algorithm into a React component

## Data model decisions

- `keyword` — plain `TEXT`; compound keywords stored as `"word1 vs word2"`, no separate rows needed
- `source_urls` — `TEXT[]` array; one URL per keyword (AI generates from Merriam-Webster); empty array for phrases without links
- `view_count` — tracked in `localStorage` on the frontend only; no DB column needed
- Next migration adds: `source_urls TEXT[] NOT NULL DEFAULT '{}'`

## PR log

- **PR 1** — project skeleton: module, Docker Compose, Postgres connection, `db.Store` interface, `/health` endpoint
- **PR 2** — rename project to phrasely across repo, Go module, Postgres credentials
- **PR 3** — goose migrations wired into startup; `phrases` table; SQL files embedded into binary via `migrations/embed.go`
- **PR 4** — `POST /api/v1/phrases` with input validation, unit tests, Taskfile
- **PR 5** — `GET /api/v1/phrases` with keyword filter
- **PR 6** — `GET /api/v1/phrases/{id}` with UUID validation
- **PR 7** — `DELETE /api/v1/phrases/{id}`
- **PR 8** — `PATCH /api/v1/phrases/{id}` with partial update via COALESCE

## Data Model