# Database backup usage

Run these commands from the repository root. `script.sh` uses the file selected by
`ENV_FILE` for both connectivity checks and database backups.

## Setup

Install and start Docker (Docker Desktop on macOS). The script runs client tools
in a disposable `postgres:18` container and pulls the image on first use. No local
PostgreSQL tools are required. Use a local Docker engine.

Create your configuration:

```sh
cp .env.backup.example .env.backup
chmod 600 .env.backup
```

Edit `.env.backup`:

```sh
DATABASE_URL='postgresql://phrasely:phrasely@localhost:5432/phrasely?sslmode=disable'
BACKUP_DIR='./backups'
PGCONNECT_TIMEOUT=10
```

- `DATABASE_URL` is required. The example connects to the local Compose database;
  start it with `docker compose up -d db`. URLs using `localhost`, `127.0.0.1`,
  or `[::1]` as the host are mapped to `host.docker.internal` inside the container.
  Use a standard URL with an explicit port and database path.
- For production, use a database URL reachable from the machine running the
  script, including the required SSL parameters. An internal Railway hostname
  will not be reachable from your laptop. URL-encode special characters in credentials.
- `BACKUP_DIR` defaults to `./backups`. Relative paths, including `ENV_FILE`, are
  resolved from your current working directory, not the config file's directory.
- `PGCONNECT_TIMEOUT` defaults to 10 seconds and limits connection establishment.

The config file is sourced as Bash, so use only a trusted file and quote values
as shown. Values assigned in the file override inherited environment variables.
`.env.backup` and `.env.backup.*` are ignored by Git, except the example file.
The default `backups/` directory and `*.dump` files are also ignored.

## Check connectivity

```sh
ENV_FILE=.env.backup ./script.sh check
```

Runs `SELECT 1` to verify network access, authentication, and database access.
It creates no backup or output directory and does not change database data.
Success prints `Database connection successful (SELECT 1).` A failure exits
nonzero. This is the lightweight preflight command; there is no `--dry-run` flag.
It does not prove that the account can dump every table.

## Create a backup

```sh
ENV_FILE=.env.backup ./script.sh backup
```

Creates a compressed PostgreSQL custom-format archive containing the database
schema and data. Files are named like
`backups/phrasely-2026-09-08_143000Z-aB12Cd.dump` (UTC plus a unique suffix).
The directory is created if needed. New directories are private to your user;
backup files have owner-only permissions. Failed or interrupted dumps are removed
on normal error, Ctrl-C, or termination; a forced kill or power loss can leave an
unfinished file without the `.dump` suffix.

The script never prints the connection URL and suppresses PostgreSQL diagnostics
because they can contain connection details. Failure messages list the likely
causes. Backups are not encrypted and old backups are not automatically deleted.
Keep a protected copy elsewhere if you need recovery after losing this machine.
This is a single-database logical backup; cluster-wide roles are not included.

Use separate config files to switch databases:

```sh
ENV_FILE=.env.backup.production ./script.sh check
ENV_FILE=.env.backup.production ./script.sh backup
```

## Inspect and restore

Inspect an archive without connecting to a database:

```sh
docker run --rm -i postgres:18 pg_restore --list < backups/<backup-file>.dump
```

For a recovery check, create a separate, empty PostgreSQL 18 database with
pgvector available on its server. Restore into that database, using an account
that can create the required extensions and schema objects:

```sh
docker run --rm -it --log-driver=none --ulimit core=0 \
  --mount "type=bind,source=$PWD/backups/<backup-file>.dump,target=/backup.dump,readonly" \
  postgres:18 pg_restore --password --exit-on-error --no-owner --no-privileges \
  --host=host.docker.internal --port=5432 --username=phrasely \
  --dbname=phrasely_recovery /backup.dump
```

This writes to the selected database. Use a separate recovery database to verify
the archive before planning any production restore. `--no-owner --no-privileges`
makes restored objects belong to the restore account and skips original grants.
Replace the host, port, username, database, and archive path as needed. Enter the
password at the hidden prompt; do not put it in the command. Use
`host.docker.internal` to reach a database on your host machine. On Linux, also add
`--add-host=host.docker.internal:host-gateway` to `docker run`.
Check the restored schema and data; listing an
archive alone does not verify a full restore.


## Password handling

`check` and `backup` send the connection URL to the disposable container over
stdin. The password is removed from the URL before invoking PostgreSQL and is
passed to the client through its runtime `PGPASSWORD` environment. It is not
stored in Docker's configured environment (`docker inspect`) or process arguments.
The script writes no credential files, disables container logging and core dumps,
and uses a read-only container filesystem. Docker removes the container on exit.
PostgreSQL diagnostics are suppressed to avoid printing connection details.

Keep using `ENV_FILE=/path/to/config ./script.sh check` so shell history contains
only a file path. The original config file still contains your password: keep it
outside Git with owner-only permissions (`chmod 600 /path/to/config`). The file
is trusted Bash code; do not add tracing, logging, or other commands to it, or run
these commands under a tracing wrapper. The script disables Bash tracing before
loading it.

This avoids additional persistent password copies during normal operation; it
cannot promise zero traces. The password exists in memory and in the database
client's runtime environment while connected, where privileged users can inspect
it. OS swap, host monitoring, database server logging, and backups of your config
are outside the script's control. Existing shell history or logs from earlier
commands are not erased. Database archives contain application data and should
also be kept private.
