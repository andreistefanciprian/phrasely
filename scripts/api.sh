#!/usr/bin/env bash
# Usage:
#   ./scripts/api.sh list-phrases
#   ./scripts/api.sh list-phrases serendipitous
#   ./scripts/api.sh add-phrase
#   ./scripts/api.sh add-phrases

BASE_URL="${API_URL:-http://localhost:8080}/api/v1"

list_phrases() {
  local keyword="$1"
  local url="$BASE_URL/phrases"
  [[ -n "$keyword" ]] && url="$url?keyword=$keyword"

  echo "GET $url"
  curl -s "$url" | jq
}

add_phrase() {
  echo "POST $BASE_URL/phrases"
  curl -s -X POST "$BASE_URL/phrases" \
    -H "Content-Type: application/json" \
    -d '{
      "phrase": "It was serendipitous, we met at the right time.",
      "keyword": "serendipitous",
      "note": "A happy accident with a pleasant outcome."
    }' | jq
}

add_phrases() {
  local phrases=(
    '{"phrase":"It'\''s unfathomable to imagine yourself as a billionaire.","keyword":"unfathomable","note":"Used when something is so extreme it'\''s beyond normal comprehension."}'
    '{"phrase":"GCC is literally the linchpin of the american empire.","keyword":"linchpin","note":"The one thing that holds everything else together."}'
    '{"phrase":"Not everyone has the fortitude to take on these issues.","keyword":"fortitude","note":"Courage and resilience in the face of difficulty."}'
    '{"phrase":"As soon as things get dicey, governments take control of gold.","keyword":"dicey","note":"Informal. Used when a situation starts feeling unstable or risky."}'
    '{"phrase":"The gold market dwarfs the bitcoin market in terms of market cap.","keyword":"dwarfs","note":"Used when one thing is so much bigger it makes the other look insignificant."}'
    '{"phrase":"Powell doesn'\''t want people to think that a rate cut is a foregone conclusion.","keyword":"foregone conclusion","note":"When the outcome feels decided before any debate has happened."}'
    '{"phrase":"That'\''s a fallacy — the reasoning doesn'\''t hold up.","keyword":"fallacy","note":"A reasoning error that looks convincing but doesn'\''t hold up."}'
    '{"phrase":"Bitcoin is antithetical to the current system.","keyword":"antithetical","note":"Stronger than different or opposite — implies deep structural incompatibility."}'
    # Three ethos entries — use `./scripts/api.sh list-phrases ethos` to verify filtering works
    '{"phrase":"The mid seventies ethos was to read history and sociology critically.","keyword":"ethos","note":"The spirit and guiding values of a group or era."}'
    '{"phrase":"The ethos of the early internet was openness and decentralization.","keyword":"ethos","note":"Applied to a historical movement — the ethos of an era shapes its tools."}'
    '{"phrase":"The company'\''s ethos is built around innovation.","keyword":"ethos","note":"A company'\''s ethos is its actual character — what it stands for in practice."}'
    # conspicuous + inconspicuous — search `cons` to verify partial matching returns both
    '{"phrase":"The sign was placed in a very conspicuous spot.","keyword":"conspicuous","note":"Hard to miss or ignore. Often used when something stands out more than expected."}'
    '{"phrase":"He sat in an inconspicuous corner, hoping no one would notice him.","keyword":"inconspicuous","note":"Opposite of conspicuous — blending in, not drawing attention."}'
  )

  for phrase in "${phrases[@]}"; do
    echo "POST $BASE_URL/phrases"
    curl -s -X POST "$BASE_URL/phrases" \
      -H "Content-Type: application/json" \
      -d "$phrase" | jq .keyword
  done
}

cmd="$1"
shift
case "$cmd" in
  list-phrases) list_phrases "$@" ;;
  add-phrase)   add_phrase ;;
  add-phrases)  add_phrases ;;
  *)
    echo "Usage: $0 {list-phrases [keyword]|add-phrase|add-phrases}"
    exit 1
    ;;
esac
