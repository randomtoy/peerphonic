#!/bin/sh
set -eu

server_url=${PEERPHONIC_URL:-http://127.0.0.1:8080}
username=${PEERPHONIC_USERNAME:-admin}
password=${PEERPHONIC_PASSWORD:-admin}
client=${PEERPHONIC_CLIENT:-peerphonic-smoke}

request() {
  endpoint=$1
  shift
  curl --fail --silent --show-error --get "${server_url}/rest/${endpoint}" \
    --data-urlencode "u=${username}" \
    --data-urlencode "p=${password}" \
    --data-urlencode "v=1.16.1" \
    --data-urlencode "c=${client}" \
    --data-urlencode "f=json" \
    "$@"
}

assert_ok() {
  endpoint=$1
  shift
  response=$(request "$endpoint" "$@")
  printf '%s' "$response" | grep -q '"status":"ok"' || {
    printf 'OpenSubsonic %s failed: %s\n' "$endpoint" "$response" >&2
    exit 1
  }
}

assert_ok ping
assert_ok getOpenSubsonicExtensions
assert_ok getIndexes
assert_ok search3 --data-urlencode "query=peerphonic-smoke" \
  --data-urlencode "artistCount=1" --data-urlencode "albumCount=1" --data-urlencode "songCount=1"

if [ -n "${PEERPHONIC_TRACK_ID:-}" ]; then
  curl --fail --silent --show-error --range 0-31 --output /dev/null --get \
    "${server_url}/rest/stream" \
    --data-urlencode "u=${username}" --data-urlencode "p=${password}" \
    --data-urlencode "v=1.16.1" --data-urlencode "c=${client}" \
    --data-urlencode "id=${PEERPHONIC_TRACK_ID}"
fi

printf 'Peerphonic OpenSubsonic smoke test passed for %s\n' "$server_url"
