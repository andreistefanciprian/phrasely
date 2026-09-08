#!/usr/bin/env bash
set +xv
set -euo pipefail
umask 077
ulimit -c 0

usage() {
  echo 'Usage: ENV_FILE=.env.backup ./script.sh {check|backup}'
}

fail() {
  echo "Error: $*" >&2
  exit 1
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
  check|backup) [[ $# -eq 1 ]] || { usage >&2; exit 1; } ;;
  *) usage >&2; exit 1 ;;
esac
command_name=$1

[[ -n "${ENV_FILE:-}" ]] || fail 'Set ENV_FILE to your backup configuration file.'
[[ -f "$ENV_FILE" && -r "$ENV_FILE" ]] || fail 'ENV_FILE must point to a readable file.'
# Prefix relative paths so source never searches PATH. Only source trusted files.
case "$ENV_FILE" in
  /*) config_file=$ENV_FILE ;;
  *) config_file=./$ENV_FILE ;;
esac
source "$config_file"
[[ -n "${DATABASE_URL:-}" ]] || fail 'DATABASE_URL is required.'

# Send the URL over stdin, keeping it out of Docker arguments and configuration.
# A container's localhost is not the host running this script.
container_url="$DATABASE_URL"
loopback_url='^(postgres(ql)?://([^/@]*@)?)(localhost|127\.0\.0\.1|\[::1\])([:/?].*|$)'
if [[ "$container_url" =~ $loopback_url ]]; then
  container_url="${BASH_REMATCH[1]}host.docker.internal${BASH_REMATCH[5]}"
fi
# Do not pass credentials to child processes through the host environment.
export -n DATABASE_URL container_url 2>/dev/null || true
unset PGDATABASE PGPASSWORD
[[ "$container_url" != *$'\n'* && "$container_url" != *$'\r'* ]] || fail 'DATABASE_URL must be a single line.'
# Password query parameters would remain visible in the connection arguments.
[[ "$container_url" != *'password='* ]] || fail 'Put the password in the URL user information, not query parameters.'
export PGCONNECT_TIMEOUT="${PGCONNECT_TIMEOUT:-10}"
command -v docker >/dev/null 2>&1 || fail 'Docker is required; install and start Docker.'
docker info >/dev/null 2>&1 || fail 'Docker is not running or is not accessible.'
if ! docker image inspect postgres:18 >/dev/null 2>&1; then
  echo 'Pulling PostgreSQL 18 client image...'
  docker pull postgres:18 || fail 'Could not pull postgres:18.'
fi

postgres() {
  # Docker Desktop supplies this hostname; Linux Docker needs an explicit mapping.
  local -a network_args=()
  if [[ "$(uname -s)" == Linux ]]; then
    network_args=(--add-host=host.docker.internal:host-gateway)
  fi
  printf '%s\n' "$container_url" | docker run --rm -i --log-driver=none \
    --ulimit core=0 --read-only \
    ${network_args[@]+"${network_args[@]}"} \
    --env PGCONNECT_TIMEOUT postgres:18 bash -c '
      set +xv
      IFS= read -r connection_url || exit 1
      # Keep only the password-free URL in PostgreSQL process arguments.
      credentials="^(postgres(ql)?://)([^/@]*):([^@]*)@(.*)$"
      if [[ "$connection_url" =~ $credentials ]]; then
        encoded_password=${BASH_REMATCH[4]}
        safe_url="${BASH_REMATCH[1]}${BASH_REMATCH[3]}@${BASH_REMATCH[5]}"
        # Decode URL percent escapes without interpreting literal backslashes.
        password=""
        while [[ -n "$encoded_password" ]]; do
          if [[ "$encoded_password" == %* ]]; then
            hex=${encoded_password:1:2}
            [[ "$hex" =~ ^[[:xdigit:]]{2}$ && "$hex" != 00 ]] || exit 1
            printf -v character "%b" "\\x$hex"
            password+=$character
            encoded_password=${encoded_password:3}
          else
            password+=${encoded_password:0:1}
            encoded_password=${encoded_password:1}
          fi
        done
        export PGPASSWORD="$password"
      else
        safe_url=$connection_url
      fi
      unset connection_url password encoded_password BASH_REMATCH
      exec "$@" --dbname="$safe_url"
    ' bash "$@"
}

if [[ "$command_name" == check ]]; then
  if ! postgres psql -X --no-password --set=ON_ERROR_STOP=1 --tuples-only --no-align \
    --command='SELECT 1' >/dev/null 2>&1; then
    fail 'Database connection failed. Check the URL, credentials, network access, and SSL settings.'
  fi
  echo 'Database connection successful (SELECT 1).'
  exit 0
fi

backup_dir=${BACKUP_DIR:-./backups}
mkdir -p -- "$backup_dir"
backup_dir=$(cd -- "$backup_dir" && pwd)
partial_file=''
cleanup() {
  if [[ -n "$partial_file" ]]; then
    rm -f -- "$partial_file"
  fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# A random suffix avoids overwriting another backup made in the same second.
partial_file=$(mktemp "$backup_dir/phrasely-$(date -u +%Y-%m-%d_%H%M%SZ)-XXXXXX")
if ! postgres pg_dump --no-password --format=custom >"$partial_file" 2>/dev/null; then
  fail 'Backup failed. Check connectivity, dump permissions, free disk space, and pg_dump/server version compatibility. The incomplete file will be removed.'
fi
backup_file="$partial_file.dump"
mv -- "$partial_file" "$backup_file"
partial_file=''
printf 'Backup saved: %s\n' "$backup_file"
