# Frontdoor Architecture

The frontdoor is a Go HTTP server that sits between the browser and the private API.
The browser never talks to the API directly — not even through a proxy that exposes raw API routes.

## Components

```
┌─────────────────────────────────────────────────────────────┐
│                        Internet                             │
└───────────────────────────┬─────────────────────────────────┘
                            │ HTTPS
                            ▼
                    ┌───────────────┐
                    │   Frontdoor   │  public (Railway URL / getphrasely.com)
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
    participant Frontdoor
    participant API

    User->>Browser: submits email on /login
    Browser->>Frontdoor: POST /login { email }
    Frontdoor->>API: POST /auth/request { email }
    API-->>Frontdoor: 200 OK
    Frontdoor-->>Browser: renders "check your inbox"

    User->>Browser: clicks magic link (https://getphrasely.com/auth-verify?token=...)
    Browser->>Frontdoor: GET /auth-verify?token=<uuid>
    Frontdoor->>API: GET /auth/verify?token=<uuid>
    API-->>Frontdoor: { token: "<signed JWT>" }

    Note over Frontdoor: stores JWT in memory<br/>sessions["abc123"] = "<JWT>"
    Frontdoor-->>Browser: Set-Cookie: session=abc123 (HttpOnly, Secure)
    Browser-->>Browser: redirects to /bubble

    Note over Browser: Browser holds only "abc123"<br/>JWT never leaves the server
```

## Page request flow

```mermaid
sequenceDiagram
    actor User
    participant Browser
    participant Frontdoor
    participant API

    User->>Browser: navigates to /bubble
    Browser->>Frontdoor: GET /bubble (Cookie: session=abc123)
    Frontdoor->>Frontdoor: look up sessions["abc123"] → JWT
    Frontdoor->>API: GET /api/v1/phrases (Authorization: Bearer <JWT>)
    API-->>Frontdoor: [ ...phrases... ]
    Frontdoor-->>Browser: rendered HTML with phrase data embedded
    Note over Browser: No API call from browser.<br/>No JWT visible anywhere.
```

## Interactive actions via /fd/

For actions that need JS interactivity (edit, delete, curate), the browser calls
`/fd/*` endpoints on the frontdoor. The frontdoor looks up the JWT from the session
and forwards the request to the private API.

```mermaid
sequenceDiagram
    actor User
    participant Browser
    participant Frontdoor
    participant API

    User->>Browser: clicks "Curate with AI"
    Browser->>Frontdoor: POST /fd/phrases/curate (Cookie: session=abc123)
    Note over Frontdoor: no Bearer token in the request —<br/>cookie is enough
    Frontdoor->>Frontdoor: look up sessions["abc123"] → JWT
    Frontdoor->>API: POST /api/v1/phrases/curate (Authorization: Bearer <JWT>)
    API-->>Frontdoor: curated phrase
    Frontdoor-->>Browser: JSON response

    User->>Browser: clicks Save
    Browser->>Frontdoor: POST /fd/phrases (Cookie: session=abc123)
    Frontdoor->>API: POST /api/v1/phrases (Authorization: Bearer <JWT>)
    API-->>Frontdoor: saved phrase
    Frontdoor-->>Browser: JSON response
```

## Session cookie vs JWT in localStorage

| | localStorage (insecure) | Frontdoor session cookie |
|---|---|---|
| Auth token in browser | Full JWT | Random session ID only |
| Readable by JavaScript | Yes — `localStorage.getItem(...)` | No — `document.cookie` returns empty |
| Visible in DevTools | Application → Local Storage | Application → Cookies (ID only, not JWT) |
| JWT leaves the server | Yes | Never |
| Stolen token risk | Copy JWT, use anywhere | Session ID useless without server |

## Key properties

| Property | Value |
|---|---|
| Session storage | In-memory map on frontdoor (resets on restart) |
| Session TTL | 30 days (matches JWT expiry) |
| Session ID | 32-char random hex string |
| Cookie flags | `HttpOnly`, `Secure` (in production), `SameSite=Lax` |
| `/fd/` routes | Session-authenticated, proxy to private API |
| API exposure | No public URL — only reachable via frontdoor on `railway.internal` |
