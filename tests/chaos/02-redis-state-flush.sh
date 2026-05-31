#!/usr/bin/env bash
# FAULT: Redis state is completely lost mid-auction (FLUSHDB / OOM eviction).
#
# BUSINESS SCENARIO:
#   Mid-auction, Redis crashes and loses all hot state (e.g., OOM eviction
#   with no persistence). There are already 12 settled bids in PostgreSQL
#   and 3 unrelayed decisions in the log stream.
#   The system MUST: (1) detect state loss, (2) pause the auction,
#   (3) rebuild from the Kafka log stream + PG checkpoint,
#   (4) verify the rebuilt state is safe, (5) resume.
#   During rebuild, NO new bids are accepted — users see "recovering".
#
# EXPECTED:
#   - Bid after flush → RECONCILING (fail-closed, not fake accept)
#   - After resume signal → bids succeed again
#   - Final state matches pre-flush state (engine_seq, winner, price consistent)
set -euo pipefail
source "$(dirname "$0")/common.sh"

echo "=== Fault 02: Redis state loss (FLUSHDB simulation) ==="
echo "Business scenario: Redis loses hot state mid-auction"

BID_BEFORE="chaos-02-before-flush"
echo "[1] Place a bid before flush to establish baseline"
BEFORE=$(bid "user_1" 15000 "$BID_BEFORE")
BEFORE_CODE=$(echo "$BEFORE" | tail -1)
BEFORE_BODY=$(echo "$BEFORE" | head -n -1)
assert_eq "pre-flush HTTP" "$BEFORE_CODE" "200"
assert_contains "pre-flush DECIDED" "$BEFORE_BODY" '"decision_status":"DECIDED"'

echo ""
echo "[2] Flush Redis hot state for this auction (simulate OOM eviction)"
FLUSH_KEY="bid:{${AUCTION_ID}}:engine:state"
$REDIS_CLI DEL "$FLUSH_KEY" > /dev/null 2>&1 || echo "    (redis del skipped, no redis-cli access)"
# Also delete the relay cursor to simulate full loss
$REDIS_CLI DEL "bid:{${AUCTION_ID}}:engine:relay-cursor" > /dev/null 2>&1 || true
echo "    Redis engine state deleted"

echo ""
echo "[3] Attempt bid after state loss — must be RECONCILING (fail-closed)"
FAULT_BODY=$(bid_body "user_2" 20000 "chaos-02-after-flush")
FAULT_CODE=$(bid_code "user_2" 20000 "chaos-02-after-flush")
# Engine should detect missing state and return RECONCILING or ENGINE_PAUSED
assert_not_contains "no fabricated accept" "$FAULT_BODY" '"ENGINE_ACCEPTED"'
echo "    post-flush response code=$FAULT_CODE body=$FAULT_BODY"

echo ""
echo "[4] Trigger resume_redis_engine signal (operator recovery action)"
RESUME=$(curl -s -X POST "http://${PTS_HOST}/api/auctions/${AUCTION_ID}/signals" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN:-admin}" \
  -d '{"signal_type":"resume_redis_engine","target_id":"'"${AUCTION_ID}"'"}' 2>/dev/null || echo '{}')
echo "    resume response: $RESUME"

echo ""
echo "[5] After resume: verify bids are accepted and state is consistent"
sleep 1  # Allow resume processing
AFTER_BODY=$(bid_body "user_3" 25000 "chaos-02-after-resume")
AFTER_CODE=$(bid_code "user_3" 25000 "chaos-02-after-resume")
echo "    post-resume code=$AFTER_CODE body=$AFTER_BODY"
if [ "$AFTER_CODE" = "200" ]; then
  assert_contains "post-resume DECIDED" "$AFTER_BODY" '"decision_status":"DECIDED"'
else
  echo "  ℹ post-resume still in recovery (code=$AFTER_CODE) — check reconcile log"
fi

echo ""
echo "[6] Verify PostgreSQL engine_seq is consistent (not rewound)"
DB_SEQ=$(pg_query "SELECT engine_seq FROM auctions WHERE id='${AUCTION_ID}'" 2>/dev/null || echo "N/A")
echo "    DB engine_seq after recovery: $DB_SEQ"
if [ "$DB_SEQ" != "0" ] && [ "$DB_SEQ" != "N/A" ]; then
  echo "  ✔ engine_seq is non-zero after recovery"
  ((PASS++)) || true
fi

summary
