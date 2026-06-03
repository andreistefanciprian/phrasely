#!/usr/bin/env bash
# Usage:
#   ./scripts/api.sh list-phrases
#   ./scripts/api.sh list-phrases serendipitous
#   ./scripts/api.sh get-phrase <id>
#   ./scripts/api.sh add-phrase
#   ./scripts/api.sh add-phrases

BASE_URL="${API_URL:-http://localhost:8080}/api/v1"

list_phrases() {
  local headword="$1"
  local url="$BASE_URL/phrases"
  [[ -n "$headword" ]] && url="$url?headword=$headword"

  echo "GET $url"
  curl -s "$url" | jq
}

get_phrase() {
  local id="$1"
  if [[ -z "$id" ]]; then
    echo "Usage: $0 get-phrase <id>"
    exit 1
  fi
  echo "GET $BASE_URL/phrases/$id"
  curl -s "$BASE_URL/phrases/$id" | jq
}

update_phrase() {
  local id="$1"
  local body="$2"
  if [[ -z "$id" || -z "$body" ]]; then
    echo "Usage: $0 update-phrase <id> '<json>'"
    exit 1
  fi
  echo "PATCH $BASE_URL/phrases/$id"
  curl -s -X PATCH "$BASE_URL/phrases/$id" \
    -H "Content-Type: application/json" \
    -d "$body" | jq
}

delete_phrase() {
  local id="$1"
  if [[ -z "$id" ]]; then
    echo "Usage: $0 delete-phrase <id>"
    exit 1
  fi
  echo "DELETE $BASE_URL/phrases/$id"
  curl -s -o /dev/null -w "%{http_code}\n" -X DELETE "$BASE_URL/phrases/$id"
}

add_phrase() {
  echo "POST $BASE_URL/phrases"
  curl -s -X POST "$BASE_URL/phrases" \
    -H "Content-Type: application/json" \
    -d '{
      "phrase": "It was serendipitous, we met at the right time.",
      "headwords": ["serendipitous"],
      "note": "A happy accident with a pleasant outcome.",
      "source_urls": ["https://www.merriam-webster.com/dictionary/serendipitous"]
    }' | jq
}

add_phrases() {
  local phrases=(
    # Single headwords
    '{"phrase":"It'\''s unfathomable to imagine yourself as a billionaire.","headwords":["unfathomable"],"note":"Used when something is so extreme it'\''s beyond normal comprehension.","source_urls":["https://www.merriam-webster.com/dictionary/unfathomable"]}'
    '{"phrase":"GCC is literally the linchpin of the american empire.","headwords":["linchpin"],"note":"The one thing that holds everything else together.","source_urls":["https://www.merriam-webster.com/dictionary/linchpin"]}'
    '{"phrase":"Not everyone has the fortitude to take on these issues.","headwords":["fortitude"],"note":"Courage and resilience in the face of difficulty.","source_urls":["https://www.merriam-webster.com/dictionary/fortitude"]}'
    '{"phrase":"As soon as things get dicey, governments take control of gold.","headwords":["dicey"],"note":"Informal. Used when a situation starts feeling unstable or risky.","source_urls":["https://www.merriam-webster.com/dictionary/dicey"]}'
    '{"phrase":"The gold market dwarfs the bitcoin market in terms of market cap.","headwords":["dwarfs"],"note":"Used when one thing is so much bigger it makes the other look insignificant.","source_urls":["https://www.merriam-webster.com/dictionary/dwarf"]}'
    '{"phrase":"That'\''s a fallacy — the reasoning doesn'\''t hold up.","headwords":["fallacy"],"note":"A reasoning error that looks convincing but doesn'\''t hold up.","source_urls":["https://www.merriam-webster.com/dictionary/fallacy"]}'
    '{"phrase":"Bitcoin is antithetical to the current system.","headwords":["antithetical"],"note":"Stronger than different or opposite — implies deep structural incompatibility.","source_urls":["https://www.merriam-webster.com/dictionary/antithetical"]}'
    # Expression headwords (multi-word, single concept)
    '{"phrase":"Powell doesn'\''t want people to think that a rate cut is a foregone conclusion.","headwords":["foregone conclusion"],"note":"When the outcome feels decided before any debate has happened.","source_urls":["https://www.merriam-webster.com/dictionary/foregone%20conclusion"]}'
    '{"phrase":"He saved the team from conceding a goal just in the nick of time.","headwords":["in the nick of time"],"note":"With no time to spare. Originally from a notch on a tally stick marking the exact moment.","source_urls":["https://www.merriam-webster.com/dictionary/nick"]}'
    '{"phrase":"Let'\''s get cracking — we have a deadline to hit.","headwords":["get cracking"],"note":"A call to stop delaying and start acting immediately.","source_urls":["https://www.merriam-webster.com/dictionary/crack"]}'
    # Three ethos entries — search `ethos` to verify array filtering works
    '{"phrase":"The mid seventies ethos was to read history and sociology critically.","headwords":["ethos"],"note":"The spirit and guiding values of a group or era.","source_urls":["https://www.merriam-webster.com/dictionary/ethos"]}'
    '{"phrase":"The ethos of the early internet was openness and decentralization.","headwords":["ethos"],"note":"Applied to a historical movement — the ethos of an era shapes its tools.","source_urls":["https://www.merriam-webster.com/dictionary/ethos"]}'
    '{"phrase":"The company'\''s ethos is built around innovation.","headwords":["ethos"],"note":"A company'\''s ethos is its actual character — what it stands for in practice.","source_urls":["https://www.merriam-webster.com/dictionary/ethos"]}'
    # conspicuous + inconspicuous — search `cons` to verify partial matching returns both
    '{"phrase":"The sign was placed in a very conspicuous spot.","headwords":["conspicuous"],"note":"Hard to miss or ignore. Often used when something stands out more than expected.","source_urls":["https://www.merriam-webster.com/dictionary/conspicuous"]}'
    '{"phrase":"He sat in an inconspicuous corner, hoping no one would notice him.","headwords":["inconspicuous"],"note":"Opposite of conspicuous — blending in, not drawing attention.","source_urls":["https://www.merriam-webster.com/dictionary/inconspicuous"]}'
    # Multiple headwords — two source_urls aligned by index
    '{"phrase":"Unfettered (unrestrained), inalienable (cannot be taken away) property rights are essential to a free society.","headwords":["unfettered","inalienable"],"note":"Two powerful words often used together in political and philosophical contexts about freedom and rights.","source_urls":["https://www.merriam-webster.com/dictionary/unfettered","https://www.merriam-webster.com/dictionary/inalienable"]}'
    '{"phrase":"The most egregious (outrageously bad) markup was so conspicuous (impossible to miss) that even non-technical customers questioned it.","headwords":["egregious","conspicuous"],"note":"Egregious is about magnitude of wrongness; conspicuous is about visibility. Things that are egregious often make themselves conspicuous.","source_urls":["https://www.merriam-webster.com/dictionary/egregious","https://www.merriam-webster.com/dictionary/conspicuous"]}'
  )

  for phrase in "${phrases[@]}"; do
    echo "POST $BASE_URL/phrases"
    curl -s -X POST "$BASE_URL/phrases" \
      -H "Content-Type: application/json" \
      -d "$phrase" | jq .headwords
  done
}

header() {
  echo ""
  echo "=================================================="
  echo "  $1"
  echo "=================================================="
}

run_all() {
  header "Seeding phrases"
  add_phrases

  header "List all phrases"
  list_phrases

  header "Filter by headword: ethos (expect 3)"
  list_phrases "ethos"

  header "Filter by headword: cons (expect conspicuous + inconspicuous)"
  list_phrases "cons"

  header "Get phrase by ID"
  local id
  id=$(curl -s "$BASE_URL/phrases" | jq -r '.[0].id')
  get_phrase "$id"

  header "Get phrase by invalid ID (expect 404)"
  get_phrase "00000000-0000-0000-0000-000000000000"

  header "Update phrase note by ID (expect 200)"
  local upd_id
  upd_id=$(curl -s "$BASE_URL/phrases" | jq -r '.[0].id')
  update_phrase "$upd_id" '{"note":"updated note via run_all"}'

  header "Update with empty body (expect 400)"
  update_phrase "$upd_id" '{}'

  header "Delete phrase by ID (expect 204)"
  local del_id
  del_id=$(curl -s "$BASE_URL/phrases" | jq -r '.[1].id')
  delete_phrase "$del_id"

  header "Delete same phrase again (expect 404)"
  delete_phrase "$del_id"

  header "Delete with invalid ID (expect 400)"
  delete_phrase "not-a-uuid"
}

cmd="$1"
shift
case "$cmd" in
  list-phrases)   list_phrases "$@" ;;
  get-phrase)     get_phrase "$@" ;;
  update-phrase)  update_phrase "$@" ;;
  delete-phrase)  delete_phrase "$@" ;;
  add-phrase)     add_phrase ;;
  add-phrases)    add_phrases ;;
  "")             run_all ;;
  *)
    echo "Usage: $0 {list-phrases [headword]|get-phrase <id>|update-phrase <id> '<json>'|delete-phrase <id>|add-phrase|add-phrases}"
    echo "       $0            (no args: run all tests in sequence)"
    exit 1
    ;;
esac
