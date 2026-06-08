# Magic Link Auth Flow

## How it works

```mermaid
sequenceDiagram
    actor User
    participant Frontdoor
    participant API
    participant DB
    participant Email

    User->>Frontdoor: enters email on /login
    Frontdoor->>API: POST /auth/request { email }
    API->>DB: upsert user by email
    DB-->>API: user { id, email }
    API->>DB: insert magic_link_tokens (user_id, expires_at = now + 15min)
    DB-->>API: token { token: "uuid" }

    alt Production
        API->>Email: send link to user@email.com
    else Local dev
        API->>API: log link to stdout
    end

    API-->>Frontdoor: { message: "magic link sent" }
    Frontdoor-->>User: renders "check your inbox"

    User->>Frontdoor: clicks link (GET /auth-verify?token=<uuid>)
    Frontdoor->>API: GET /auth/verify?token=<uuid>

    API->>DB: look up token by value
    DB-->>API: token record

    alt token not found
        API-->>Frontdoor: 401 invalid token
    else token already used
        API-->>Frontdoor: 401 token already used
    else token expired
        API-->>Frontdoor: 401 token expired
    else token valid
        API->>DB: mark token used_at = NOW()
        API->>API: sign JWT (sub=user_id, exp=now+30days)
        API-->>Frontdoor: { token: "<signed JWT>" }
    end

    Frontdoor->>Frontdoor: store JWT server-side (sessions["abc123"] = JWT)
    Frontdoor-->>User: Set-Cookie: session=abc123 (HttpOnly, Secure)

    Note over Frontdoor,API: All subsequent requests

    User->>Frontdoor: GET /bubble (Cookie: session=abc123)
    Frontdoor->>Frontdoor: look up sessions["abc123"] → JWT
    Frontdoor->>API: GET /api/v1/phrases (Authorization: Bearer <JWT>)
    API->>API: validate JWT signature + expiry
    API-->>Frontdoor: phrases for this user
    Frontdoor-->>User: rendered HTML page
```

## Key properties

| Property | Value |
|---|---|
| Magic link TTL | 15 minutes |
| JWT TTL | 30 days |
| Token reuse | Not allowed — marked used on first click |
| Token storage | DB (`magic_link_tokens` table) |
| JWT storage | Frontdoor server-side only (never in browser) |
| Session cookie | httpOnly, Secure — session ID only, not the JWT |
| JWT signing | HMAC-SHA256 with `JWT_SECRET` env var |

## Local dev

The magic link is logged to the API stdout instead of being emailed.
Run `docker compose logs api` after submitting your email and click the link directly from the terminal.
