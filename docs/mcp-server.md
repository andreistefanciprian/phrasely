# MCP Server

Exposes `list_phrases`, `sample_phrases`, `explore_phrase`, and `add_phrase` over MCP (Streamable HTTP), backed by the
private `backend` API. Deployed as a separate public Railway service alongside `frontend`.

## Architecture

`mcp` is a thin public adapter in front of the private `backend` API. It has no direct
DB connection and shares no Go `internal/` packages with `backend` — it is purely an HTTP
client of the private API, the same role `frontend/api.go` plays.

```
Internet
   │ HTTPS
   ▼
┌─────────────────────────────────────────────────┐
│  MCP server (public)        Frontend (public)   │
│  mcp:8081                   frontend:3000        │
└───────────────┬─────────────────────┬───────────┘
                │ internal network    │
                ▼                     ▼
        ┌───────────────────────────────────┐
        │         API (private)             │
        │         backend:8080              │
        └───────────────────────────────────┘
                        │
                        ▼
                  PostgreSQL
```

Tool calls forward the caller's Bearer JWT straight through to `backend`, so
`middleware.Auth` and per-`user_id` scoping work unchanged — `mcp` never inspects JWT
internals beyond passing the header along.

## Auth: OAuth 2.1 + PKCE

In production, access to a user's collection goes through OAuth 2.1. MCP initialization, `tools/list`, and the stateless `explore_phrase` tool are public; collection-backed tool calls require `Authorization: Bearer <access_token>` and return an `mcp/www_authenticate` challenge when the token is missing, invalid, or expired. For local ad-hoc testing, a magic-link JWT works directly as the Bearer token (see [mcp/README.md](../mcp/README.md)).

OAuth *logic and storage* (tables, code generation, token minting) live in `backend`.
The *endpoints that clients call directly* are hosted/proxied by `mcp` — calling `backend`
internally for the actual work.

### Discovery endpoints (hosted by `mcp`)

- `/.well-known/oauth-authorization-server` (RFC 8414) — advertises `/authorize` (on
  `frontend`), `/token` and `/register` (on `mcp`), PKCE method (`S256`).
- `/.well-known/oauth-protected-resource` (RFC 9728) — points the MCP resource at the
  auth server above.

### Dynamic Client Registration — `POST /register` (proxied to `backend`)

Client POSTs a `redirect_uris` array (RFC 7591), gets back a `client_id` (public client, no secret —
PKCE is mandatory). Redirect URIs are pinned at registration and validated on every
`/authorize` call (open-redirect defense).

### `/authorize` (frontend, public)

If the request has a valid `auth_token` cookie (existing magic-link session), shows a
consent screen ("Allow access to your Phrasely phrases?") and issues an authorization
code via an internal call to `backend`. If not logged in, redirects into the magic-link
flow, then bounces back to `/authorize` to complete.

### `/token` (proxied to `backend`)

Client POSTs `grant_type=authorization_code`, `code`, `code_verifier`, `client_id`, and `redirect_uri`. `backend` validates the PKCE challenge, marks the code used, and mints:
- **access_token** — JWT (1h expiry), same format as `auth.signJWT`, so `middleware.Auth`
  works unchanged.
- **refresh_token** — DB-persisted for revocation and rotation.

Refresh token rotation is enforced: old token is atomically revoked, new one issued on
every `refresh_token` grant.

### DB tables (in `backend`)

| Table | Purpose |
|---|---|
| `oauth_clients` | `client_id`, pinned `redirect_uris[]`, `created_at` |
| `oauth_authorization_codes` | `code`, `client_id`, `user_id`, `redirect_uri`, `code_challenge`, `expires_at` (~60s), `used_at` |
| `oauth_refresh_tokens` | long-lived tokens for rotation and revocation |

## Full auth flow

```mermaid
sequenceDiagram
    participant U as User
    participant C as ChatGPT/Claude
    participant FE as Frontend
    participant MCP as MCP Server
    participant API as Backend API
    participant DB as PostgreSQL

    Note over C,DB: 1. Discovery + Registration (one-time per connector)
    C->>MCP: GET /.well-known/oauth-authorization-server
    MCP-->>C: authorization_endpoint (FE/authorize), token_endpoint (MCP/token), registration_endpoint (MCP/register), code_challenge_methods_supported=[S256]
    C->>MCP: POST /register {redirect_uris: [...]}
    MCP->>API: POST /internal/oauth/register {redirect_uris: [...]}
    API->>DB: INSERT oauth_clients (generate client_id, redirect_uris[])
    DB-->>API: ok
    API-->>MCP: {client_id}
    MCP-->>C: {client_id}

    Note over C,DB: 2. Authorization Code + PKCE
    C->>C: generate code_verifier (random 32B), code_challenge = BASE64URL(SHA256(code_verifier))
    C->>U: open browser to /authorize?client_id=X, redirect_uri=Y, code_challenge=Z, code_challenge_method=S256
    U->>FE: GET /authorize (Cookie: auth_token=JWT)
    alt no valid session
        FE->>U: redirect to /login
        U->>FE: click magic link from email
        FE->>U: set auth_token cookie, redirect back to /authorize
    end
    FE->>U: consent screen - Allow access to your Phrasely phrases?
    U->>FE: POST /authorize/approve
    FE->>API: POST /internal/oauth/authorize {client_id, redirect_uri, code_challenge, user_id}
    API->>DB: INSERT oauth_authorization_codes (code_challenge, user_id, client_id, redirect_uri, expires_at=+60s)
    DB-->>API: code = gen_random_uuid() via RETURNING
    API-->>FE: {code}
    FE-->>C: 302 redirect_uri?code=...

    Note over C,DB: 3. Token Exchange
    C->>MCP: POST /token {grant_type=authorization_code, code, code_verifier, client_id, redirect_uri}
    MCP->>API: POST /internal/oauth/token (forwarded verbatim)
    API->>DB: SELECT oauth_authorization_codes WHERE code=X, not used, not expired
    API->>API: PKCE check - assert BASE64URL(SHA256(code_verifier)) == stored code_challenge
    API->>DB: UPDATE oauth_authorization_codes SET used_at=now
    API->>DB: INSERT oauth_refresh_tokens (user_id, client_id)
    DB-->>API: token = gen_random_uuid() via RETURNING
    API-->>MCP: {access_token (JWT, OAUTH_ACCESS_TOKEN_TTL default 1h), refresh_token, token_type=Bearer, expires_in=3600}
    MCP-->>C: {access_token, refresh_token, token_type=Bearer, expires_in=3600}

    Note over C,DB: 4. MCP Tool Calls (access token valid)
    C->>MCP: POST /mcp {tool: list_phrases, arguments: {}} - Authorization: Bearer access_token
    MCP->>API: GET /api/v1/phrases - Authorization: Bearer access_token
    API->>API: middleware.Auth: verify JWT signature, extract user_id
    API->>DB: SELECT phrases WHERE user_id=...
    DB-->>API: rows
    API-->>MCP: [{id, phrase, headwords, context, ...}]
    MCP-->>C: tool result {phrases: [...]}

    Note over C,DB: 5. Token Refresh (after 1h when access token expires)
    C->>MCP: POST /token {grant_type=refresh_token, refresh_token, client_id}
    MCP->>API: POST /internal/oauth/token (forwarded verbatim)
    API->>DB: DELETE oauth_refresh_tokens WHERE token=old (atomic revoke)
    API->>DB: INSERT oauth_refresh_tokens (user_id, client_id)
    DB-->>API: token = gen_random_uuid() via RETURNING
    API-->>MCP: {access_token (JWT, 1h), refresh_token (rotated), expires_in=3600}
    MCP-->>C: {access_token, refresh_token}
```

**Why PKCE?** Clients like ChatGPT and Claude Desktop are public clients that cannot
keep a `client_secret`. PKCE fixes the interception risk: the `code_challenge` (SHA-256
hash) is sent upfront; the `code_verifier` (the pre-image) is sent only at token
exchange. An intercepted `code` is useless without the `code_verifier`.

## MCP tools

| Tool | Purpose | Backend call |
|---|---|---|
| `list_phrases(headword?)` | List the user's saved phrases, optionally filtered by headword | `GET /api/v1/phrases` |
| `sample_phrases(count?)` | Randomly pick N phrases (1–10) for practice or quizzing | `GET /api/v1/phrases/random` |
| `explore_phrase(phrase)` | Return learning instructions for understanding an expression and generating memorable contexts — no backend call, no data persisted | — |
| `add_phrase(phrase, headwords, note?, source_urls?)` | Save a finished phrase constructed locally by the assistant | `POST /api/v1/phrases` |

### Exploration and save flow

Exploration and saving are separate workflows:

1. When the user wants to understand or explore an expression, the assistant calls `explore_phrase(phrase)` and applies the returned learning instructions conversationally. Nothing is persisted.
2. When the user asks to save a supplied or selected phrase, the assistant constructs the finished phrase, headwords, note, and source URLs locally, then calls `add_phrase` directly. Calling `explore_phrase` first is not required.

`add_phrase` is persistence-only — it does not call OpenAI or enrich the entry itself.

## Local testing

See [mcp/README.md](../mcp/README.md) for local setup, quick JWT testing, MCP Inspector
usage, and Claude Desktop configuration.
