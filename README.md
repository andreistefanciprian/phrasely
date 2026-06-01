# Phrasely

## Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop/)
- [Task](https://taskfile.dev) — `brew install go-task`

## Run locally

```bash
task up        # build images, start API + Postgres, stream logs
```

The API is available at `http://localhost:8080`.

```bash
curl http://localhost:8080/health
```

## Other commands

```bash
task down      # stop containers, keep DB data
task reset     # stop containers and wipe DB data (fresh start)
task test      # run unit tests
```
