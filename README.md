# Phrasely

![Phrasely](frontend/static/logo.png)

A personal vocabulary tool for collecting English phrases from real life, with AI-powered curation and a word cloud that grows as you learn.

Production: [getphrasely.com](https://getphrasely.com)

![Word bubble](frontend/static/bubble-preview.png)

## How It Works

1. **Capture** a phrase, sentence fragment, or even a single word you encounter in a podcast, book, movie, article, or conversation.
2. **Curate with AI** to enrich missing context, correct grammar, add clear definitions, attach Merriam-Webster links, and generate useful notes.
3. **Build your vocabulary** by saving curated phrases to your personal collection.
4. **Review and reinforce** in Shuffle mode, which presents one phrase at a time for focused learning.
5. **Visualize your progress** with the Vocabulary Bubble, where the expressions you revisit most often grow larger over time.

The goal is simple: turn interesting words and expressions you hear in everyday life into part of your active vocabulary.

## Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop/)
- [Task](https://taskfile.dev) — `brew install go-task`

## Run locally

```bash
task up        # build images, start API + Postgres, stream logs
```

The UI is available at `http://localhost:3000`.

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