#!/usr/bin/env bash
# Usage (always use source or TOKEN= prefix, not bare bash, to pass TOKEN):
#   ./scripts/api.sh auth-request <email>
#   ./scripts/api.sh auth-verify <token>
#   export TOKEN=<jwt>   # then run any command below
#   TOKEN=<jwt> ./scripts/api.sh list-phrases
#   TOKEN=<jwt> ./scripts/api.sh list-phrases ethos
#   TOKEN=<jwt> ./scripts/api.sh get-phrase <id>
#   TOKEN=<jwt> ./scripts/api.sh add-phrase
#   TOKEN=<jwt> ./scripts/api.sh add-phrases
#   TOKEN=<jwt> ./scripts/api.sh   (no args: seed + test all phrase endpoints)

BASE_URL="${API_URL:-http://localhost:8080}"
API="$BASE_URL/api/v1"

# AUTH_ARGS is a bash array holding the Authorization header when TOKEN is set.
# Declare it once here; all curl calls use "${AUTH_ARGS[@]}".
AUTH_ARGS=()
[[ -n "$TOKEN" ]] && AUTH_ARGS=(-H "Authorization: Bearer $TOKEN")

# --- Auth ---

auth_request() {
  local email="$1"
  if [[ -z "$email" ]]; then
    echo "Usage: $0 auth-request <email>"
    exit 1
  fi
  echo "POST $BASE_URL/auth/request"
  curl -s -X POST "$BASE_URL/auth/request" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"$email\"}" | jq
  echo ""
  echo "Check logs for the magic link:"
  echo "  docker compose logs api | grep 'magic link'"
}

auth_verify() {
  local token="$1"
  if [[ -z "$token" ]]; then
    echo "Usage: $0 auth-verify <token>"
    exit 1
  fi
  echo "GET $BASE_URL/auth/verify?token=***"
  local jwt
  jwt=$(curl -s "$BASE_URL/auth/verify?token=$token" | jq -r '.token')
  if [[ "$jwt" == "null" || -z "$jwt" ]]; then
    echo "Error: could not obtain JWT"
    curl -s "$BASE_URL/auth/verify?token=$token" | jq
    exit 1
  fi
  echo ""
  echo "JWT obtained. Export it to use with other commands:"
  echo "  export TOKEN=$jwt"
}

# --- Phrases ---

list_phrases() {
  local headword="$1"
  local url="$API/phrases"
  [[ -n "$headword" ]] && url="$url?headword=$headword"
  echo "GET $url"
  curl -s "${AUTH_ARGS[@]}" "$url" | jq
}

get_phrase() {
  local id="$1"
  if [[ -z "$id" ]]; then echo "Usage: $0 get-phrase <id>"; exit 1; fi
  echo "GET $API/phrases/$id"
  curl -s "${AUTH_ARGS[@]}" "$API/phrases/$id" | jq
}

update_phrase() {
  local id="$1" body="$2"
  if [[ -z "$id" || -z "$body" ]]; then echo "Usage: $0 update-phrase <id> '<json>'"; exit 1; fi
  echo "PATCH $API/phrases/$id"
  curl -s -X PATCH "${AUTH_ARGS[@]}" "$API/phrases/$id" \
    -H "Content-Type: application/json" \
    -d "$body" | jq
}

delete_phrase() {
  local id="$1"
  if [[ -z "$id" ]]; then echo "Usage: $0 delete-phrase <id>"; exit 1; fi
  echo "DELETE $API/phrases/$id"
  curl -s -o /dev/null -w "%{http_code}\n" -X DELETE "${AUTH_ARGS[@]}" "$API/phrases/$id"
}

add_phrase() {
  echo "POST $API/phrases"
  curl -s -X POST "${AUTH_ARGS[@]}" "$API/phrases" \
    -H "Content-Type: application/json" \
    -d '{"phrase":"It was serendipitous, we met at the right time.","headwords":[{"text":"serendipitous","canonical":"serendipitous","meaning":"happening by fortunate chance","source_url":"https://www.merriam-webster.com/dictionary/serendipitous"}],"note":"A happy accident with a pleasant outcome."}' | jq
}

add_phrases() {
  local phrases=(
    # Single headwords
    '{"phrase":"It'\''s unfathomable to imagine yourself as a billionaire.","headwords":[{"text":"unfathomable","canonical":"unfathomable","meaning":"impossible to fully comprehend","source_url":"https://www.merriam-webster.com/dictionary/unfathomable"}],"note":"Used when something is so extreme it'\''s beyond normal comprehension."}'
    '{"phrase":"GCC is literally the linchpin of the american empire.","headwords":[{"text":"linchpin","canonical":"linchpin","meaning":"the essential part holding everything together","source_url":"https://www.merriam-webster.com/dictionary/linchpin"}],"note":"The one thing that holds everything else together."}'
    '{"phrase":"Not everyone has the fortitude to take on these issues.","headwords":[{"text":"fortitude","canonical":"fortitude","meaning":"courage through difficulty","source_url":"https://www.merriam-webster.com/dictionary/fortitude"}],"note":"Courage and resilience in the face of difficulty."}'
    '{"phrase":"As soon as things get dicey, governments take control of gold.","headwords":[{"text":"dicey","canonical":"dicey","meaning":"risky or uncertain","source_url":"https://www.merriam-webster.com/dictionary/dicey"}],"note":"Informal. Used when a situation starts feeling unstable or risky."}'
    '{"phrase":"The gold market dwarfs the bitcoin market in terms of market cap.","headwords":[{"text":"dwarfs","canonical":"dwarf","meaning":"makes something seem much smaller","source_url":"https://www.merriam-webster.com/dictionary/dwarf"}],"note":"Used when one thing is so much bigger it makes the other look insignificant."}'
    '{"phrase":"That'\''s a fallacy — the reasoning doesn'\''t hold up.","headwords":[{"text":"fallacy","canonical":"fallacy","meaning":"a mistake in reasoning","source_url":"https://www.merriam-webster.com/dictionary/fallacy"}],"note":"A reasoning error that looks convincing but doesn'\''t hold up."}'
    '{"phrase":"Bitcoin is antithetical to the current system.","headwords":[{"text":"antithetical","canonical":"antithetical","meaning":"fundamentally opposed","source_url":"https://www.merriam-webster.com/dictionary/antithetical"}],"note":"Stronger than different or opposite — implies deep structural incompatibility."}'
    # Expression headwords
    '{"phrase":"Powell doesn'\''t want people to think that a rate cut is a foregone conclusion.","headwords":[{"text":"foregone conclusion","canonical":"foregone conclusion","meaning":"an outcome regarded as certain","source_url":"https://www.merriam-webster.com/dictionary/foregone%20conclusion"}],"note":"When the outcome feels decided before any debate has happened."}'
    '{"phrase":"He saved the team from conceding a goal just in the nick of time.","headwords":[{"text":"in the nick of time","canonical":"in the nick of time","meaning":"just before it was too late","source_url":"https://www.merriam-webster.com/dictionary/nick"}],"note":"With no time to spare."}'
    '{"phrase":"Let'\''s get cracking — we have a deadline to hit.","headwords":[{"text":"get cracking","canonical":"get cracking","meaning":"start working promptly","source_url":"https://www.merriam-webster.com/dictionary/crack"}],"note":"A call to stop delaying and start acting immediately."}'
    # Three ethos entries
    '{"phrase":"The mid seventies ethos was to read history and sociology critically.","headwords":[{"text":"ethos","canonical":"ethos","meaning":"the guiding values of a group","source_url":"https://www.merriam-webster.com/dictionary/ethos"}],"note":"The spirit and guiding values of a group or era."}'
    '{"phrase":"The ethos of the early internet was openness and decentralization.","headwords":[{"text":"ethos","canonical":"ethos","meaning":"the guiding values of a group","source_url":"https://www.merriam-webster.com/dictionary/ethos"}],"note":"Applied to a historical movement."}'
    '{"phrase":"The company'\''s ethos is built around innovation.","headwords":[{"text":"ethos","canonical":"ethos","meaning":"the guiding values of a group","source_url":"https://www.merriam-webster.com/dictionary/ethos"}],"note":"A company'\''s actual character — what it stands for in practice."}'
    # conspicuous + inconspicuous
    '{"phrase":"The sign was placed in a very conspicuous spot.","headwords":[{"text":"conspicuous","canonical":"conspicuous","meaning":"impossible to miss","source_url":"https://www.merriam-webster.com/dictionary/conspicuous"}],"note":"Hard to miss or ignore."}'
    '{"phrase":"He sat in an inconspicuous corner, hoping no one would notice him.","headwords":[{"text":"inconspicuous","canonical":"inconspicuous","meaning":"not attracting attention","source_url":"https://www.merriam-webster.com/dictionary/inconspicuous"}],"note":"Opposite of conspicuous — blending in, not drawing attention."}'
    # Multiple headwords
    '{"phrase":"Unfettered, inalienable property rights are essential to a free society.","headwords":[{"text":"unfettered","canonical":"unfettered","meaning":"unrestrained","source_url":"https://www.merriam-webster.com/dictionary/unfettered"},{"text":"inalienable","canonical":"inalienable","meaning":"unable to be taken away","source_url":"https://www.merriam-webster.com/dictionary/inalienable"}],"note":"Two powerful words used together in political contexts."}'
    '{"phrase":"The most egregious markup was so conspicuous that even non-technical customers questioned it.","headwords":[{"text":"egregious","canonical":"egregious","meaning":"outrageously bad","source_url":"https://www.merriam-webster.com/dictionary/egregious"},{"text":"conspicuous","canonical":"conspicuous","meaning":"impossible to miss","source_url":"https://www.merriam-webster.com/dictionary/conspicuous"}],"note":"Egregious is about magnitude; conspicuous is about visibility."}'
  )

  for phrase in "${phrases[@]}"; do
    echo "POST $API/phrases"
    curl -s -X POST "${AUTH_ARGS[@]}" "$API/phrases" \
      -H "Content-Type: application/json" \
      -d "$phrase" | jq .headwords
  done
}

# --- Helpers ---

header() {
  echo ""
  echo "=================================================="
  echo "  $1"
  echo "=================================================="
}

run_all() {
  local email="test@example.com"

  # --- Auth ---
  header "Request magic link for $email"
  curl -s -X POST "$BASE_URL/auth/request" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"$email\"}" | jq
  sleep 2

  header "Extract token from logs and verify"
  local raw_token
  raw_token=$(docker compose logs api 2>/dev/null \
    | grep "magic link" | tail -1 \
    | grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' \
    | tail -1)

  if [[ -z "$raw_token" ]]; then
    echo "Could not extract token from logs — is docker compose running?"
    exit 1
  fi

  TOKEN=$(curl -s "$BASE_URL/auth/verify?token=$raw_token" | jq -r '.token')
  if [[ -z "$TOKEN" || "$TOKEN" == "null" ]]; then
    echo "Could not obtain JWT"
    exit 1
  fi
  # Update AUTH_ARGS now that we have a token
  AUTH_ARGS=(-H "Authorization: Bearer $TOKEN")
  echo "JWT obtained: ${TOKEN:0:40}..."
  sleep 2

  # --- Phrases ---
  header "No token (expect 401)"
  curl -s "$API/phrases" | jq
  sleep 2

  header "Seed phrases"
  add_phrases
  sleep 2

  header "List all phrases"
  list_phrases
  sleep 2

  header "Filter by headword: ethos (expect 3)"
  list_phrases "ethos"
  sleep 2

  header "Filter by headword: cons (expect conspicuous + inconspicuous)"
  list_phrases "cons"
  sleep 2

  header "Get phrase by ID"
  local id
  id=$(curl -s "${AUTH_ARGS[@]}" "$API/phrases" | jq -r '.[0].id')
  get_phrase "$id"
  sleep 2

  header "Get phrase by invalid ID (expect 404)"
  get_phrase "00000000-0000-0000-0000-000000000000"
  sleep 2

  header "Update phrase note (expect 200)"
  update_phrase "$id" '{"note":"updated via run_all"}'
  sleep 2

  header "Delete phrase (expect 204)"
  local del_id
  del_id=$(curl -s "${AUTH_ARGS[@]}" "$API/phrases" | jq -r '.[1].id')
  delete_phrase "$del_id"
  sleep 2

  header "Delete same phrase again (expect 404)"
  delete_phrase "$del_id"
  sleep 2

  header "Delete with invalid ID (expect 400)"
  delete_phrase "not-a-uuid"
  sleep 2

  header "Reuse magic link token (expect 401)"
  curl -s "$BASE_URL/auth/verify?token=$raw_token" | jq
}

# --- Dispatch ---

cmd="$1"
shift
case "$cmd" in
  auth-request)  auth_request "$@" ;;
  auth-verify)   auth_verify "$@" ;;
  list-phrases)  list_phrases "$@" ;;
  get-phrase)    get_phrase "$@" ;;
  update-phrase) update_phrase "$@" ;;
  delete-phrase) delete_phrase "$@" ;;
  add-phrase)    add_phrase ;;
  add-phrases)   add_phrases ;;
  "")            run_all ;;
  *)
    echo "Usage:"
    echo "  $0 auth-request <email>"
    echo "  $0 auth-verify <token>"
    echo "  TOKEN=<jwt> $0 {list-phrases [headword]|get-phrase <id>|update-phrase <id> '<json>'|delete-phrase <id>|add-phrase|add-phrases}"
    echo "  $0   (no args: full flow)"
    exit 1
    ;;
esac
