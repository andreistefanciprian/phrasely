# Phrasely

Production: [getphrasely.com](https://getphrasely.com)

## Stack

- **API** (`backend/`): Go + gorilla/mux, slog, pgx/v5
- **Frontend** (`frontend/`): Go SSR server (html/template + plain CSS), port 3000
- **MCP Server** (`mcp/`): Go Streamable HTTP (`github.com/modelcontextprotocol/go-sdk`), OAuth 2.1 proxy + `/mcp` endpoint, port 8081
- **DB**: PostgreSQL 17
- **Infra**: Docker Compose (local), Railway (prod); DNS managed via Cloudflare

## Key files

```
backend/cmd/api/main.go                 — route registration, env wiring, migrations on startup
backend/internal/db/db.go               — Store interface + all PostgresStore SQL implementations
backend/internal/phrases/handler.go     — phrase CRUD handlers
backend/internal/oauth/handler.go       — OAuth 2.1 handlers (register, authorize, token)
backend/internal/auth/handler.go        — magic link + JWT verify handlers
backend/internal/middleware/auth.go     — JWT middleware (injects user_id into context)
backend/migrations/                     — goose SQL files (embedded into binary via embed.go)
mcp/main.go                             — MCP server wiring, requireBearer middleware
mcp/oauth.go                            — OAuth discovery + proxy routes
mcp/tools.go                            — MCP tool definitions (list_phrases, add_phrase)
mcp/api.go                              — typed API client used by tools
frontend/main.go                        — route registration, render helpers, security headers
frontend/handlers.go                    — all page handlers + apiProxy
frontend/templates/                     — html/template files (base.html, base-auth.html, navbar.html, ...)
```

## Engineering philosophy

**Don't over-engineer for scale or theoretical edge cases.** The app has a small user base — skip near-zero-probability race conditions, complex transaction schemes, and defensive code for attack vectors that require precise timing or adversarial clients. Fix things users actually hit.

## Conventions

- `db.Store` interface in `backend/internal/db/db.go` — one central interface; add all new DB methods here
- Concrete implementation: `db.PostgresStore` — holds `*pgxpool.Pool`; SQL lives in the same file
- Handler pattern: `NewHandler(store db.Store) *Handler` with `RegisterRoutes(r *mux.Router)` — register in `backend/cmd/api/main.go`
- Dependency injection via constructors; no globals
- `docker compose up --build` to run everything; no local Go needed
- **Structured logging**: all three services use `slog.NewJSONHandler` writing JSON to stdout; set `LOG_LEVEL=debug` for request-level detail, defaults to `INFO`

## Deployment architecture

- **Frontend** (Go SSR, port 3000) — public, exposed directly via Railway ingress; no nginx
- **API** (Go, port 8080) — private, only reachable from within the internal network
- **MCP** (Go, port 8081) — public, exposed via Railway ingress
- Frontend proxies browser API calls: `/fd/*` → strips `/fd`, prepends `/api/v1`, forwards to private API with JWT from cookie
- Internal API address configured via `API_HOST` env var — not hardcoded
- All frontend HTML uses relative paths — no hardcoded API URL; CORS not needed (same-origin)

## How to extend

**New API endpoint:**
1. Add method to `Store` interface in `backend/internal/db/db.go`
2. Implement on `PostgresStore` in the same file (SQL inline)
3. Add handler method in the relevant `internal/<domain>/handler.go`
4. Register route in `RegisterRoutes`
5. Add test in `internal/<domain>/handler_test.go` using `mockStore`

**New DB migration:**
Add `backend/migrations/000NN_description.sql` — goose runs automatically on startup, SQL files are embedded into the binary via `migrations/embed.go`. No separate migration step needed.

**New MCP tool:**
1. Define `Input` and `Output` structs in `mcp/tools.go`
2. Write a handler func returning `mcp.ToolHandlerFor[Input, Output]`
3. Call `mcp.AddTool(server, &mcp.Tool{Name: ..., Description: ...}, handler)` in `registerTools`
4. Add any required API client method in `mcp/api.go`

**New frontend page:**
1. Add handler in `frontend/handlers.go`
2. Register route in `frontend/main.go`
3. Use `app.render(w, "page.html", data)` for public pages or `app.renderAuth(...)` for authenticated ones
4. Add `frontend/templates/page.html` — templates are parsed with `base.html` + `navbar.html`
5. Protect with `app.requireAuth(handler)` if login is required

## Tests

- **Run**: `task test` or `cd backend && go test ./... -v`
- **No real DB needed**: handlers use a `mockStore` struct that satisfies `db.Store`; set only the fields the test exercises
- `testUserID` is injected into request context to simulate an authenticated user
- Tests live alongside handlers: `internal/<domain>/handler_test.go`

## Environment variables

| Variable | Service | Default | Notes |
|---|---|---|---|
| `DATABASE_URL` | backend | — | Required |
| `JWT_SECRET` | backend | — | Required |
| `PORT` | all | 8080/3000/8081 | Per service |
| `BASE_URL` | backend | `http://localhost:3000` | Frontend origin for magic links |
| `API_HOST` | frontend, mcp | `http://localhost:8080` | Private API address (overridden in prod) |
| `OPENAI_API_KEY` | backend | — | Optional; curate endpoint disabled if unset |
| `RESEND_API_KEY` | backend | — | Optional; magic links logged to stdout if unset |
| `EMAIL_FROM` | backend | — | Required when `RESEND_API_KEY` is set |
| `MCP_BASE_URL` | mcp | `http://localhost:8081` | Public MCP URL for OAuth discovery |
| `FRONTEND_BASE_URL` | mcp | `http://localhost:3000` | Public frontend URL for OAuth discovery |
| `LOG_LEVEL` | all | `INFO` | `DEBUG`, `INFO`, `WARN`, `ERROR` |
| `MAGIC_LINK_TTL` | backend | `15m` | Go duration format |
| `JWT_TTL` | backend | `720h` | Go duration format |

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

## OAuth 2.1 (complete)

MCP server is the public OAuth face; backend handles the real work over the private network.

- **Discovery**: `/.well-known/oauth-authorization-server` and `/.well-known/oauth-protected-resource` on MCP
- **Dynamic client registration**: `POST /register` (MCP) → proxies to `POST /internal/oauth/register` (backend)
- **Authorization**: `POST /internal/oauth/authorize` (backend, JWT-protected) — called by frontend after user approves consent; issues a 60s PKCE auth code
- **Token exchange**: `POST /token` (MCP) → `POST /internal/oauth/token` (backend); supports `authorization_code` and `refresh_token` grants
- **PKCE**: S256 only (`plain` deliberately omitted); `code_challenge` stored at authorize time, verified at token time before consuming the code
- **Refresh token rotation**: old token atomically revoked, new one issued on every `refresh_token` grant
- **Access token TTL**: 1 hour (JWT); refresh tokens are DB-persisted so they can be revoked
- **`/mcp`**: requires `Authorization: Bearer <access_token>`; per-request server factory scopes each caller's JWT to their own tool invocations

## Auth (magic link — complete)

- JWT expiry: 30 days (configurable via `JWT_TTL`); token expiry: 15 min (configurable via `MAGIC_LINK_TTL`); tokens are single-use
- In dev: magic link logged to stdout; in prod: email via Resend
- `user_id` extracted from JWT by middleware and injected into request context; all phrase endpoints scope queries to it

## Data model

- `headwords TEXT[]` — array of dictionary headwords or fixed expressions; aligned by index with `source_urls`
- `source_urls TEXT[]` — one Merriam-Webster URL per headword; AI generates these; empty array for phrases without links
- `view_count` — tracked in `localStorage` on the frontend only; no DB column needed

## Git workflow

- Every change goes in a PR — never push directly to `main`
- **Before pushing: show the full diff and ask the user to review — no exceptions**
- **Never push without explicit approval from the user**
- Branch naming: `pr/<number>-<short-description>`
- Commit messages must follow [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `chore:`, etc.) with scope when relevant, e.g. `feat(frontend): add shuffle button`

## Release process (release-please)

release-please runs on every push to `main` and tracks `frontend/`, `backend/`, and `mcp/` as separate packages. It reads commit messages to determine the version bump and generate changelogs.

- `fix:` → patch bump; `feat:` → minor bump; `feat!:` or `BREAKING CHANGE:` footer → major bump
- **Before creating a PR, ask the user whether the change is a patch, minor, or major release** so the correct commit type is used
- Merging the release-please PR tags the release as `frontend-vX.Y.Z` / `backend-vX.Y.Z` / `mcp-vX.Y.Z` and updates `CHANGELOG.md`

## Open questions

- **Should headwords be required?** Currently POST requires at least one headword and PATCH cannot set headwords to empty. But should a user be able to save a phrase without identifying the headword yet — e.g. draft phrases waiting to be curated by the AI? If yes, `headwords NOT NULL DEFAULT '{}'` and relaxed validation. If no, keep current behaviour.
- **Empty string headwords** — `{"headwords":[""]}` is currently accepted. Should we validate that each element in the array is non-blank?
