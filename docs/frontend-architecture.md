# Frontend Architecture

The frontend is a Go HTTP server that sits between the browser and the private API.
The browser never talks to the API directly — not even through a proxy that exposes raw API routes.

## Components

```
┌─────────────────────────────────────────────────────────────┐
│                        Internet                             │
└───────────────────────────┬─────────────────────────────────┘
                            │ HTTPS
                            ▼
                    ┌───────────────┐
                    │   Frontend   │  public (Railway URL / getphrasely.com)
                    │   Go server   │  handles pages + /fd/ proxy
                    └───────┬───────┘
                            │ internal network only
                            │ http://phrasely.railway.internal:8080
                            ▼
                    ┌───────────────┐
                    │      API      │  private — no public URL
                    │   Go server   │  JWT-authenticated
                    └───────┬───────┘
                            │
                            ▼
                    ┌───────────────┐
                    │  PostgreSQL   │  private
                    └───────────────┘
```

## Sign-in flow

```mermaid
sequenceDiagram
    actor User
    participant Browser
    participant Frontend
    participant API

    User->>Browser: submits email on /login
    Browser->>Frontend: POST /login { email }
    Frontend->>API: POST /auth/request { email }
    API-->>Frontend: 200 OK
    Frontend-->>Browser: renders "check your inbox"

    User->>Browser: clicks magic link (https://getphrasely.com/auth-verify?token=...)
    Browser->>Frontend: GET /auth-verify?token=<uuid>
    Frontend->>API: GET /auth/verify?token=<uuid>
    API-->>Frontend: { token: "<signed JWT>" }

    Frontend-->>Browser: Set-Cookie: auth_token=<JWT> (HttpOnly, Secure)
    Browser-->>Browser: redirects to /bubble

    Note over Browser: Browser sends auth_token cookie automatically<br/>JS cannot read it (HttpOnly)
```

## Page request flow

```mermaid
sequenceDiagram
    actor User
    participant Browser
    participant Frontend
    participant API

    User->>Browser: navigates to /bubble
    Browser->>Frontend: GET /bubble (Cookie: auth_token=<JWT>)
    Frontend->>API: GET /api/v1/phrases (Authorization: Bearer <JWT>)
    API-->>Frontend: [ ...phrases... ]
    Frontend-->>Browser: rendered HTML with phrase data embedded
    Note over Browser: No API call from browser.<br/>No JWT visible anywhere.
```

## Interactive actions via /fd/

For actions that need JS interactivity (edit, delete, curate), the browser calls
`/fd/*` endpoints on the frontend. The frontend reads the JWT from the
`auth_token` cookie and forwards the request to the private API.

```mermaid
sequenceDiagram
    actor User
    participant Browser
    participant Frontend
    participant API

    User->>Browser: clicks "Curate with AI"
    Browser->>Frontend: POST /fd/phrases/curate (Cookie: auth_token=<JWT>)
    Note over Frontend: no Bearer token in the request —<br/>cookie is enough
    Frontend->>API: POST /api/v1/phrases/curate (Authorization: Bearer <JWT>)
    API-->>Frontend: curated phrase
    Frontend-->>Browser: JSON response

    User->>Browser: clicks Save
    Browser->>Frontend: POST /fd/phrases (Cookie: auth_token=<JWT>)
    Frontend->>API: POST /api/v1/phrases (Authorization: Bearer <JWT>)
    API-->>Frontend: saved phrase
    Frontend-->>Browser: JSON response
```

## Key properties

| Property | Value |
|---|---|
| Auth storage | Browser cookie (`auth_token`) |
| Cookie TTL | 30 days (matches JWT expiry) |
| Cookie content | Signed JWT |
| Cookie flags | `HttpOnly`, `Secure` (in production), `SameSite=Lax` |
| `/fd/` routes | Cookie-authenticated, proxy to private API |
| API exposure | No public URL — only reachable via frontend on `railway.internal` |

## Security trade-off

The old session-ID design was more secure in theory because the JWT stayed server-side.
The practical XSS protection is still strong in this design because the auth cookie is protected by `HttpOnly`, `Secure` (in production), and `SameSite=Lax`.

The real trade-off is revocation behavior:

- Session store model: immediate server-side invalidation by deleting the session record.
- JWT cookie model: token remains valid until expiry (30 days) unless explicit revocation infrastructure is added.

At the current product size (personal app, around 50 users), we are prioritizing a smoother experience that avoids forced re-logins on frontend restarts/redeploys.
As the platform grows, we should revisit a session-ID model (or another revocation-capable approach) to improve immediate invalidation controls.
