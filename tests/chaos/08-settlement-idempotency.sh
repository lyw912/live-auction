#!/usr/bin/env bash
# FAULT: Kafka delivers the same decision message multiple times (at-least-once).
#
# BUSINESS SCENARIO:
#   A network partition between the settlement worker and Kafka causes
#   the consumer to re-receive 3 copies of the same decision message
#   (typical at-least-once Kafka delivery scenario).
#   The settlement worker must handle this gracefully:
#   - Insert the bid row exactly once (ON CONFLICT DO NOTHING)
#   - Create at most one order (if the decision is ENGINE_SOLD)
#   - Mark the settlement row as SETTLED only once
#   - Never generate a second payment charge
#   - Never write a second outbox event for the same seq
#
# KEY v3 INVARIANT:
#   Settlement is idempotent on (auction_id, engine_seq) via UNIQUE INDEX.
#   redis_engine_settlements tracks dedup by (auction_id, stream_id).
#   The fencing token on auctions.settlement_fence guards the final SOLD commit.
#
# EXPECTED:
#   - After 3 identical Kafka deliveries: exactly 1 bid row, 1 settlement row
#   - No duplicate orders, no duplicate outbox events
#   - Settlement status: SETTLED (not multiple SETTLED rows)
set -euo pipefail
cd "$(dirname "$0")/../.."

echo "=== Fault 08: Settlement idempotency (3x Kafka redelivery) ==="
echo "Business scenario: Kafka at-least-once delivery redelivers the same decision 3x; settlement must have exactly one business effect."

echo ""
echo "[1] Run targeted backend integration gate"
echo "    Gate: TestKafkaSettlementTripleDuplicateMessageHasSingleBusinessEffect"
echo "    Fault model: call settlement on the same Kafka ledger message three times"
echo "    Assertions:"
echo "      - exactly one redis_engine_settlements row"
echo "      - exactly one SETTLED row"
echo "      - exactly one bid row"
echo "      - exactly one order"
echo "      - exactly one outbox delivery per legitimate business event"
echo "      - engine is not paused"

(cd backend && go test ./internal/redisengine -run '^TestKafkaSettlementTripleDuplicateMessageHasSingleBusinessEffect$' -count=1 -v)

echo ""
echo "PASS"
