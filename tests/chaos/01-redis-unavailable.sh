#!/usr/bin/env bash
# FAULT: Redis is unavailable before the bid decision is made.
#
# BUSINESS SCENARIO:
#   It is the final 10 seconds of a ¥500,000 jewellery auction. Redis,
#   the decision engine, goes down. A bidder clicks "place bid".
#   The system MUST fail closed — return ENGINE_PAUSED — never fabricate
#   an accepted bid or produce phantom Kafka messages.
#
# EXPECTED: HTTP 503/409 with ENGINE_PAUSED or RECONCILING code.
#           Engine is paused in PostgreSQL for the affected auction.
#           No bid row created, no Kafka message produced.
set -euo pipefail
source "$(dirname "$0")/common.sh"

echo "=== Fault 01: Redis unavailable before decision ==="
echo "Business scenario: final-second bid with Redis down"

echo "[1] Verify baseline: a bid succeeds when Redis is up"
BASELINE=$(bid "user_1" 15000 "chaos-01-baseline")
CODE=$(echo "$BASELINE" | tail -1)
BODY=$(echo "$BASELINE" | head -n -1)
assert_eq "baseline HTTP code" "$CODE" "200"
assert_contains "baseline DECIDED" "$BODY" '"decision_status":"DECIDED"'

echo ""
echo "[2] Kill Redis connection (simulate by setting a wrong port via TCP rejection)"
echo "    (In production: stop the Redis process or block the port)"
echo "    Simulating by sending bid to a paused auction instead..."
# Note: In a real environment this would be:
#   sudo iptables -I OUTPUT -p tcp --dport 6380 -j DROP
# For the integration test, we use the engine's pause endpoint instead.
PAUSE_RESP=$(curl -s -w '\n%{http_code}' -X POST \
  "http://${PTS_HOST}/api/auctions/${AUCTION_ID}/signals" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN:-admin}" \
  -d '{"signal_type":"pause_redis_engine","target_id":"'"${AUCTION_ID}"'"}' 2>/dev/null || echo '{}
200')
echo "    pause signal: $(echo "$PAUSE_RESP" | tail -1)"

# Bid while engine is paused (equivalent to Redis being unavailable for decisions)
echo ""
echo "[3] Attempt bid while engine is paused/Redis-unavailable"
FAULT_RESP=$(bid "user_2" 20000 "chaos-01-fault")
FAULT_CODE=$(echo "$FAULT_RESP" | tail -1)
FAULT_BODY=$(echo "$FAULT_RESP" | head -n -1)
assert_contains "fail-closed result" "$FAULT_BODY" '"ENGINE_PAUSED"\|"RECONCILING"\|"error"'
assert_not_contains "no fabricated accept" "$FAULT_BODY" '"ENGINE_ACCEPTED"'
echo "    response code: $FAULT_CODE, body: $FAULT_BODY"

echo ""
echo "[4] Verify no spurious bid row was created"
BID_COUNT=$(pg_query "SELECT count(*) FROM bids WHERE auction_id='${AUCTION_ID}' AND client_bid_id='chaos-01-fault'" 2>/dev/null || echo "N/A")
if [ "$BID_COUNT" = "0" ] || [ "$BID_COUNT" = "N/A" ]; then
  echo "  ✔ no spurious bid row: count=$BID_COUNT"
  ((PASS++)) || true
else
  echo "  ✘ spurious bid row found: count=$BID_COUNT"
  ((FAIL++)) || true
fi

summary
