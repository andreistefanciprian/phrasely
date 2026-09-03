# Phrase Digest

A one-shot Go binary (`backend/cmd/send-phrase-digest`) that emails each opted-in user
one random phrase from their collection. Run timing is controlled by the Railway cron schedule.

## Architecture

```
Railway cron (configured schedule)
        │
        ▼
┌─────────────────────┐
│  phrase-digest      │  ← one-shot container, exits after run
│  (cron worker)      │
└────────┬────────────┘
         │ pgx (private network)
         ▼
   PostgreSQL ──── digest_preferences table
                   users table

         │ HTTPS
         ▼
   Resend API  ──── delivers email to user
```

The worker has no HTTP server and no port. It connects directly to Postgres (same
`DATABASE_URL` as the API), does its work, and exits. The API is the only binary that
runs goose migrations — the worker assumes the schema is already up to date.

## How it works

1. Reads `DATABASE_URL`, `RESEND_API_KEY`, `EMAIL_FROM` from env.
2. Calls `ListDigestRecipients` — fetches all users whose `frequency != 'disabled'`.
3. For each recipient, calls `isDue`:
   - `last_sent_at IS NULL` → always due (first send).
   - `now - last_sent_at >= threshold` → due (threshold: daily=23h45m, weekly≈7d, monthly≈30d).
   - The 15-minute grace window on each threshold absorbs Railway cron scheduling jitter.
4. For due recipients, picks one random phrase via `GetRandomPhrases`.
5. Sends the email via Resend (or logs to stdout in dev).
6. Writes `last_sent_at = now` to `digest_preferences`.

The worker no longer enforces a fixed UTC hour in code. Railway is the source of truth
for when runs happen; the worker only decides who is due based on `last_sent_at` and
frequency thresholds.

## User settings

Users opt in via **Settings → Phrase Digest** in the frontend. The preference is stored
in `digest_preferences`:

| Column | Type | Notes |
|---|---|---|
| `user_id` | UUID PK | FK → `users(id)` ON DELETE CASCADE |
| `frequency` | TEXT | `daily`, `weekly`, `monthly`, `disabled` (default) |
| `last_sent_at` | TIMESTAMPTZ | NULL until first send |
| `created_at` / `updated_at` | TIMESTAMPTZ | auto-managed by trigger |

The `frequency` column has a `CHECK` constraint — only the four valid values are accepted
at the DB level. The API handler (`GET/POST /api/v1/settings/email`) enforces the same
set so clients get a clean 400 rather than a constraint error.

## Email

- **No phrases** → user is skipped silently (Debug log). `last_sent_at` is NOT updated,
  so the worker will try again the next day.
- **One phrase** → subject: `"Your phrase for today: {headword}"`; phrases with
  multiple headwords join them with ` • `.
- **RESEND_API_KEY unset** → falls back to `LogSender`, which logs the recipient and headwords to stdout (not the full phrase text).
  Useful for local dev and CI.

## Local development

```bash
# Spin up all services
docker compose up --build

# Trigger the worker manually
docker compose run --rm phrase-digest
```

To test immediately, run the worker manually with `docker compose run --rm phrase-digest`.

Without `RESEND_API_KEY` set in `.env`, emails are logged to stdout — no real sends occur.

## Railway deployment

1. Add a new service in the Railway project → Deploy from GitHub repo.
2. Root directory: `backend/`, Dockerfile: `Dockerfile.send-phrase-digest`.
3. Cron schedule: set this in Railway (for example, once daily at your preferred time).
4. Environment variables:
   - `DATABASE_URL` — copy from the Railway Postgres service.
   - `RESEND_API_KEY` — Resend API key.
   - `EMAIL_FROM` — e.g. `noreply@getphrasely.com`.
   - `LOG_LEVEL` — optional, defaults to `INFO`.

The worker shares `DATABASE_URL`, `RESEND_API_KEY`, and `EMAIL_FROM` with the API service —
reference them from the shared Railway environment rather than duplicating values.
