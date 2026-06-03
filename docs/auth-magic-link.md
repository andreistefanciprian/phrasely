# Magic Link Auth Flow

## How it works

```mermaid
sequenceDiagram
    actor User
    participant Frontend
    participant API
    participant DB
    participant Email

    User->>Frontend: enters email address

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

    User->>Frontend: clicks link in email (or terminal)
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

    Frontend->>Frontend: store JWT (localStorage or cookie)

    Note over Frontend,API: All subsequent requests

    Frontend->>API: GET /api/v1/phrases (Authorization: Bearer <jwt>)
    API->>API: validate JWT signature + expiry
    API-->>Frontend: phrases for this user
```

## Key properties

| Property | Value |
|---|---|
| Magic link TTL | 15 minutes |
| JWT TTL | 30 days |
| Token reuse | Not allowed — marked used on first click |
| Token storage | DB (`magic_link_tokens` table) |
| JWT storage | Frontend only (never in DB) |
| JWT signing | HMAC-SHA256 with `JWT_SECRET` env var |

## Local dev

The magic link is logged to the API stdout instead of being emailed.
Run `docker compose logs api` after calling `POST /auth/request` and click the link directly from the terminal.
