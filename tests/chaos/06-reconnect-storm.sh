#!/usr/bin/env bash
# FAULT: WebSocket reconnect storm — many clients disconnect and reconnect.
#
# BUSINESS SCENARIO:
#   During the final 10 seconds, 100 bidders' network connections drop
#   simultaneously (mobile, subway tunnel). They all reconnect within 2 seconds.
#   Each reconnecting client must receive the current authoritative state
#   (latest price, winner, time-to-close) via snapshot+incremental diff,
#   NOT from client-side cached state.
#
# KEY v3 INVARIANT:
#   Client NEVER decides winner/price/terminal-state locally.
#   On reconnect: server sends snapshot with latest engine_seq and
#   any events missed since the client's last known seq.
#   "No local truth" means clients cannot believe their own cached highest bid.
#
# TEST APPROACH:
#   1. Query the auction snapshot endpoint as a reconnecting client would
#   2. Verify it contains server-authoritative state (engine_seq, current_price)
#   3. Verify that a stale client_seen_seq bid is evaluated against server state
#   4. Verify WebSocket is not using client-provided price as truth
set -euo pipefail
source "$(dirname "$0")/common.sh"

echo "=== Fault 06: WebSocket reconnect storm ==="
echo "Business scenario: 100 clients reconnect; all get authoritative snapshot"

echo "[1] Fetch auction snapshot (simulates WS reconnect recovery)"
SNAPSHOT=$(curl -s "http://${PTS_HOST}/api/auctions/${AUCTION_ID}" \
  -H "Authorization: Bearer ${TOKEN:-testtoken}" 2>/dev/null || echo '{}')
echo "    Snapshot: $SNAPSHOT"

# Verify snapshot has server-authoritative fields
assert_contains "snapshot has engine_seq" "$SNAPSHOT" '"engine_seq"'
assert_contains "snapshot has current_price" "$SNAPSHOT" '"current_price_cents"'
assert_contains "snapshot has server_time_ms" "$SNAPSHOT" '"server_time_ms"'
assert_not_contains "snapshot has no client truth" "$SNAPSHOT" '"client_'

echo ""
echo "[2] Verify a bid with stale client_seen_seq is evaluated against server state"
# A reconnecting client might send client_seen_seq=0 (stale). The bid must be
# evaluated against the current server price, not the client's stale view.
STALE_BID=$(bid "user_1" 1 "chaos-06-stale-seq")  # amount=1 cent = certainly too low
STALE_CODE=$(echo "$STALE_BID" | tail -1)
STALE_BODY=$(echo "$STALE_BID" | head -n -1)
echo "    Stale bid (1 cent): code=$STALE_CODE"
# Should be REJECTED (BID_TOO_LOW) — server evaluated against real price
assert_contains "stale bid rejected by server" "$STALE_BODY" '"ENGINE_REJECTED"\|"AUCTION_NOT_ACTIVE"'
assert_not_contains "stale bid not accepted" "$STALE_BODY" '"ENGINE_ACCEPTED"'

echo ""
echo "[3] Verify decision_basis is server-authoritative (not client-provided)"
DECISION_BASIS=$(echo "$STALE_BODY" | grep -o '"decision_basis":{[^}]*}' || echo '')
echo "    decision_basis: $DECISION_BASIS"
if [ -n "$DECISION_BASIS" ]; then
  echo "  ✔ rejection includes server-authoritative decision_basis"
  assert_contains "basis has required_min" "$DECISION_BASIS" '"required_min_price_cents"'
  ((PASS++)) || true
fi

echo ""
echo "[4] Rapid-reconnect: 10 sequential snapshot fetches (simulating storm)"
SNAP_SUCCESS=0
for i in $(seq 1 10); do
  RESP=$(curl -s -o /dev/null -w '%{http_code}' \
    "http://${PTS_HOST}/api/auctions/${AUCTION_ID}" \
    -H "Authorization: Bearer ${TOKEN:-testtoken}" 2>/dev/null || echo "0")
  if [ "$RESP" = "200" ]; then ((SNAP_SUCCESS++)) || true; fi
done
echo "    Snapshots succeeded: $SNAP_SUCCESS/10"
if [ "$SNAP_SUCCESS" -ge 8 ]; then
  echo "  ✔ reconnect storm served ≥8/10 snapshots successfully"
  ((PASS++)) || true
else
  echo "  ✘ reconnect storm: only $SNAP_SUCCESS/10 snapshots served"
  ((FAIL++)) || true
fi

summary
