#!/usr/bin/env bash
# Common helpers for fault-injection tests.
# Source this in each chaos script: source "$(dirname "$0")/common.sh"
set -euo pipefail

PTS_HOST="${PTS_HOST:-172.16.179.112:18080}"
AUCTION_ID="${AUCTION_ID:-auc_live}"
REDIS_ADDR="${REDIS_ADDR:-localhost:6380}"
REDIS_CLI="${REDIS_CLI:-redis-cli -p 6380}"
PG_DSN="${PG_DSN:-postgresql://auction:auction@localhost:5434/auction}"
TOKEN="${TOKEN:-}"

PASS=0; FAIL=0

assert_eq() {
  local label="$1" got="$2" want="$3"
  if [ "$got" = "$want" ]; then
    echo "  ✔ $label: $got"
    ((PASS++)) || true
  else
    echo "  ✘ $label: got='$got' want='$want'"
    ((FAIL++)) || true
  fi
}

assert_contains() {
  local label="$1" body="$2" want="$3"
  if echo "$body" | grep -q "$want"; then
    echo "  ✔ $label: contains '$want'"
    ((PASS++)) || true
  else
    echo "  ✘ $label: body does not contain '$want'"
    echo "    body=$body"
    ((FAIL++)) || true
  fi
}

assert_not_contains() {
  local label="$1" body="$2" want="$3"
  if ! echo "$body" | grep -q "$want"; then
    echo "  ✔ $label: does not contain '$want'"
    ((PASS++)) || true
  else
    echo "  ✘ $label: body unexpectedly contains '$want'"
    ((FAIL++)) || true
  fi
}

bid() {
  local user="$1" amount="$2" bid_id="${3:-bid-chaos-$(date +%s%N)}"
  local tok="${TOKEN:-$(head -2 "$(dirname "$0")/../pts/pts-1ab-1000vu-sessions.csv" 2>/dev/null | tail -1 | cut -d, -f2 || echo testtoken)}"
  curl -s -w '\n%{http_code}' -X POST "http://${PTS_HOST}/api/auctions/${AUCTION_ID}/bids" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer ${tok}" \
    -H "Idempotency-Key: ${bid_id}" \
    -d "{\"client_bid_id\":\"${bid_id}\",\"amount_cents\":${amount},\"client_seen_seq\":0}"
}

bid_code()  { bid "$@" | tail -1; }
bid_body()  { bid "$@" | head -n -1; }

pg_query() { psql "$PG_DSN" -t -c "$1" 2>/dev/null | tr -d ' '; }

redis_get() { $REDIS_CLI GET "$1" 2>/dev/null || echo ''; }

summary() {
  echo ""
  echo "Results: PASS=$PASS FAIL=$FAIL"
  if [ "$FAIL" -gt 0 ]; then echo "FAIL"; exit 1; else echo "PASS"; fi
}
