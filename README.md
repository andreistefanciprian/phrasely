# Phrasely

## Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop/)
- Go 1.26+ (only needed to run outside Docker)

## Run locally

```bash
# 1. Start everything (Postgres + API)
docker compose up --build

# 2. Hit the health check to confirm it's up
curl http://localhost:8080/health
```

The API is available at `http://localhost:8080`.

## Rebuild after code changes

```bash
docker compose up --build
```

## Stop

```bash
docker compose down       # stop containers, keep DB data
docker compose down -v    # stop containers and delete DB data
```
