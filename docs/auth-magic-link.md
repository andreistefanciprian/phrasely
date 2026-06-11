# Magic Link Auth Flow

## How it works

```mermaid
sequenceDiagram
    actor User
    participant Frontend
    participant API
    participant DB
    participant Email

    User->>Frontend: enters email on /login
    Frontend->>API: POST /auth/request { email }
    API->>DB: upsert user by email
    DB-->>API: user { id, email }
    API->>DB: insert magic_link_tokens (user_id, expires_at = now + 15min)
    DB-->>API: token { token: "uuid" }

    alt Production
        API->>Email: send link to user@email.com
    else Local dev
        API->>API: log link to stdout
    end

    API-->>Frontend: { message: "magic link sent" }
    Frontend-->>User: renders "check your inbox"

    User->>Frontend: clicks link (GET /auth-verify?token=<uuid>)
    Frontend->>API: GET /auth/verify?token=<uuid>

    API->>DB: look up token by value
    DB-->>API: token record

    alt token not found
        API-->>Frontend: 401 invalid token
    else token already used
        API-->>Frontend: 401 token already used
    else token expired
        API-->>Frontend: 401 token expired
    else token valid
        API->>DB: mark token used_at = NOW()
        API->>API: sign JWT (sub=user_id, exp=now+30days)
        API-->>Frontend: { token: "<signed JWT>" }
    end

    Frontend-->>User: Set-Cookie: auth_token=<JWT> (HttpOnly, Secure)

    Note over Frontend,API: All subsequent requests

    User->>Frontend: GET /bubble (Cookie: auth_token=<JWT>)
    Frontend->>API: GET /api/v1/phrases (Authorization: Bearer <JWT>)
    API->>API: validate JWT signature + expiry
    API-->>Frontend: phrases for this user
    Frontend-->>User: rendered HTML page
```

## Key properties

| Property | Value |
|---|---|
| Magic link TTL | 15 minutes |
| JWT TTL | 30 days |
| Token reuse | Not allowed — marked used on first click |
| Token storage | DB (`magic_link_tokens` table) |
| JWT storage | Browser cookie (`auth_token`, `HttpOnly`, `Secure` in production) |
| Cookie content | JWT (not a random session ID) |
| JWT signing | HMAC-SHA256 with `JWT_SECRET` env var |

## Security trade-off

The previous session-ID approach was more secure in theory because the JWT never touched the browser.
In practice, both approaches rely on the same browser protections that we still enforce now: `HttpOnly`, `Secure` (in production), and `SameSite=Lax`.

The main capability we gave up is immediate server-side revocation. With a session store, deleting the server entry invalidates access instantly. With a signed JWT cookie, revocation is not immediate unless extra infrastructure is added (for example, a denylist or short-lived access tokens plus refresh rotation).

At the current scale (personal app, around 50 users), we are prioritizing a smoother UX that avoids forced re-logins on frontend restarts/redeploys.
As usage grows or revocation requirements become stricter, we should revisit the session-ID model (or an equivalent revocation-capable design).

## Local dev

The magic link is logged to the API stdout instead of being emailed.
Run `docker compose logs api` after submitting your email and click the link directly from the terminal.
