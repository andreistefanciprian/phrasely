# Local Development

## Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop/)
- [Task](https://taskfile.dev) — `brew install go-task`

## Run Locally

```bash
task up        # build images, start API + Postgres, stream logs
```

The UI is available at `http://localhost:3000`.

## Common Commands

```bash
task down      # stop containers, keep DB data
task reset     # stop containers and wipe DB data (fresh start)
task test      # run unit tests
```

## API Script

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