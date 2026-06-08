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

    Note over Frontend: stores JWT in memory<br/>sessions["abc123"] = "<JWT>"
    Frontend-->>Browser: Set-Cookie: session=abc123 (HttpOnly, Secure)
    Browser-->>Browser: redirects to /bubble

    Note over Browser: Browser holds only "abc123"<br/>JWT never leaves the server
```

## Page request flow

```mermaid
sequenceDiagram
    actor User
    participant Browser
    participant Frontend
    participant API

    User->>Browser: navigates to /bubble
    Browser->>Frontend: GET /bubble (Cookie: session=abc123)
    Frontend->>Frontend: look up sessions["abc123"] → JWT
    Frontend->>API: GET /api/v1/phrases (Authorization: Bearer <JWT>)
    API-->>Frontend: [ ...phrases... ]
    Frontend-->>Browser: rendered HTML with phrase data embedded
    Note over Browser: No API call from browser.<br/>No JWT visible anywhere.
```

## Interactive actions via /fd/

For actions that need JS interactivity (edit, delete, curate), the browser calls
`/fd/*` endpoints on the frontend. The frontend looks up the JWT from the session
and forwards the request to the private API.

```mermaid
sequenceDiagram
    actor User
    participant Browser
    participant Frontend
    participant API

    User->>Browser: clicks "Curate with AI"
    Browser->>Frontend: POST /fd/phrases/curate (Cookie: session=abc123)
    Note over Frontend: no Bearer token in the request —<br/>cookie is enough
    Frontend->>Frontend: look up sessions["abc123"] → JWT
    Frontend->>API: POST /api/v1/phrases/curate (Authorization: Bearer <JWT>)
    API-->>Frontend: curated phrase
    Frontend-->>Browser: JSON response

    User->>Browser: clicks Save
    Browser->>Frontend: POST /fd/phrases (Cookie: session=abc123)
    Frontend->>API: POST /api/v1/phrases (Authorization: Bearer <JWT>)
    API-->>Frontend: saved phrase
    Frontend-->>Browser: JSON response
```

## Key properties

| Property | Value |
|---|---|
| Session storage | In-memory map on frontend (resets on restart) |
| Session TTL | 30 days (matches JWT expiry) |
| Session ID | 32-char random hex string |
| Cookie flags | `HttpOnly`, `Secure` (in production), `SameSite=Lax` |
| `/fd/` routes | Session-authenticated, proxy to private API |
| API exposure | No public URL — only reachable via frontend on `railway.internal` |
