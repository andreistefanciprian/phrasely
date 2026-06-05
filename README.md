# Phrasely

![Phrasely](frontend/assets/logo.png)

A personal vocabulary tool for collecting English phrases from real life, with AI-powered curation and a word cloud that grows as you learn.

Production: [getphrasely.com](https://getphrasely.com)

![Word bubble](frontend/assets/bubble-preview.png)

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

## API script

`scripts/api.sh` is a convenience script for hitting the API from the terminal.

```bash
./scripts/api.sh                                 # run all commands in sequence (good for a quick sanity check)
./scripts/api.sh add-phrases                     # seed a batch of sample phrases
./scripts/api.sh add-phrase                      # add a single sample phrase
./scripts/api.sh list-phrases                    # list all phrases
./scripts/api.sh list-phrases serendipitous      # filter by headword
./scripts/api.sh get-phrase <id>                 # get a single phrase by ID
```

> Run `task reset` before `./scripts/api.sh` to start from a clean DB.

Point it at a different host with `API_URL`:

```bash
API_URL=https://your-app.railway.app ./scripts/api.sh list-phrases
```
