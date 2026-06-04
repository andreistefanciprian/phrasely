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

## Deployment architecture

- **Frontend (nginx)** — public, exposed via load balancer / ingress
- **API (Go)** — private, only reachable from within the internal network; never directly exposed to the internet
- nginx reverse-proxies `/api/` and `/auth/` to the API using `API_HOST` env var (`http://api:8080` locally, `http://api.railway.internal:8080` on Railway)
- All frontend HTML uses relative paths — no hardcoded API URL; CORS not needed (same-origin)

## Security backlog (not yet implemented)

- **Rate limit curate endpoint** — highest priority; each call costs OpenAI tokens. Target: 20 curations/hour per user using `golang.org/x/time/rate` (in-memory token bucket, no Redis needed).
- **Rate limit `POST /auth/request`** — public endpoint, no JWT. Target: 5 requests/hour per IP + 2-minute cooldown per email to prevent inbox flooding.
- **Rate limit phrase CRUD** — lower priority; target: 100 operations/hour per user to prevent DB flooding.
- Already protected: JWT on all `/api/v1/` routes, CORS, `MaxBytesReader`, server timeouts, non-root container.

## Open questions

- **Should headwords be required?** Currently POST requires at least one headword and PATCH cannot set headwords to empty. But should a user be able to save a phrase without identifying the headword yet — e.g. draft phrases waiting to be curated by the AI? If yes, `headwords NOT NULL DEFAULT '{}'` and relaxed validation. If no, keep current behaviour.
- **Empty string headwords** — `{"headwords":[""]}` is currently accepted. Should we validate that each element in the array is non-blank?

## Git workflow

- Every change goes in a PR — never push directly to `main`
- Always ask for review before pushing
- Branch naming: `pr/<number>-<short-description>`

## API endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/health` | public | Health check |
| POST | `/auth/request` | public | Send magic link to email |
| GET | `/auth/verify?token=` | public | Verify token → JWT |
| GET | `/api/v1/phrases` | JWT | List user's phrases (optional `?headword=` filter) |
| GET | `/api/v1/phrases/{id}` | JWT | Get phrase by ID |
| POST | `/api/v1/phrases` | JWT | Create a phrase |
| PATCH | `/api/v1/phrases/{id}` | JWT | Update a phrase (partial) |
| DELETE | `/api/v1/phrases/{id}` | JWT | Delete a phrase |
| POST | `/api/v1/phrases/curate` | JWT | Curate a raw phrase via OpenAI |

## Auth (magic link — complete)

- JWT expiry: 30 days; token expiry: 15 min; tokens are single-use
- In dev: magic link logged to stdout; in prod: email via Resend (not yet wired)
- Phrases scoped to `user_id` from JWT context on all endpoints

## Frontend plan (not started)

- **Stack**: Next.js + TailwindCSS + shadcn/ui, deployed on Railway
- **Home (`/bubble`)**: headword cloud — bubble size driven by `localStorage` view counts (no DB writes on click)
- **Detail (`/phrases?headword=`)**: phrase cards filtered by headword; keyboard navigation (space/arrow to shuffle)
- **Compound headwords**: `headwords` array stores multiple entries e.g. `["unfettered","inalienable"]`; UI joins with " vs " and renders individual Merriam-Webster links
- **Reference**: PhraseFlow (`/Users/stefanandrei/Documents/Projects/PhraseFlow/`) has working bubble implementation — port the placement algorithm into a React component

## Data model decisions

- `headwords TEXT[]` — array of dictionary headwords or fixed expressions; aligned by index with `source_urls`
- `source_urls TEXT[]` — one Merriam-Webster URL per headword; AI generates these; empty array for phrases without links
- `view_count` — tracked in `localStorage` on the frontend only; no DB column needed

## PR log

- **PR 1** — project skeleton: module, Docker Compose, Postgres connection, `db.Store` interface, `/health` endpoint
- **PR 2** — rename project to phrasely across repo, Go module, Postgres credentials
- **PR 3** — goose migrations wired into startup; `phrases` table; SQL files embedded into binary via `migrations/embed.go`
- **PR 4** — `POST /api/v1/phrases` with input validation, unit tests, Taskfile
- **PR 5** — `GET /api/v1/phrases` with headword filter
- **PR 6** — `GET /api/v1/phrases/{id}` with UUID validation
- **PR 7** — `DELETE /api/v1/phrases/{id}`
- **PR 8** — `PATCH /api/v1/phrases/{id}` with partial update via COALESCE
- **PR 9** — `headwords TEXT[]` replacing `keyword TEXT`; trigram index for substring search
- **PR 10** — fix trigram index (IMMUTABLE wrapper function)
- **PR 11** — `POST /api/v1/phrases/curate` powered by OpenAI gpt-4o-mini
- **PR 12** — `users` table + `user_id` nullable FK on `phrases`
- **PR 13** — `magic_link_tokens` table + `POST /auth/request` (logs link to stdout in dev)
- **PR 14** — `GET /auth/verify?token=` → signed JWT; `signJWT`/`parseJWT` helpers; auth tests
- **PR 15** — JWT middleware scoping all routes; `WWW-Authenticate` header; middleware tests; `scripts/api.sh` full auth flow
- **PR 16** — all phrase endpoints scoped to authenticated user (`user_id` injected from JWT context)

## Data Model