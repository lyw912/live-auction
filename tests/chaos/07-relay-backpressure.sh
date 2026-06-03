#!/usr/bin/env bash
# FAULT: Redis decision log stream grows beyond relay batch ceiling.
#
# BUSINESS SCENARIO:
#   A 1000-user burst fires in one second. The relay batch size is 512.
#   After the first batch of 512 decisions, 488 remain in the stream.
#   The relay must drain them in a second pass without losing any.
#   The hot path continues accepting bids during all relay passes.
#   CRITICAL: No silent accumulation — if relay falls behind permanently,
#   an alert must fire and the auction must pause after a configurable threshold.
#
# KEY v3 INVARIANT:
#   The relay is not a serialization lock. Multiple relay passes are fine.
#   Each pass reads up to relayBatchSize (512) entries and advances the cursor.
#   The stream is the WAL — it retains all entries until explicitly trimmed.
#
# EXPECTED:
#   - After N bids (>512), stream has N entries
#   - Relay pass 1: processes 512, cursor at entry 512
#   - Relay pass 2: processes remaining N-512, cursor at entry N
#   - No entries lost; no duplicates; final ledger count = N
set -euo pipefail
cd "$(dirname "$0")/../.."

echo "=== Fault 07: Relay backpressure (stream > batch ceiling) ==="
echo "Business scenario: 600 rapid decisions exceed the 512-entry relay batch ceiling; relay must drain in multiple passes without losing decisions."

echo ""
echo "[1] Run targeted backend integration gate"
echo "    Gate: TestRelayBackpressureDrainsBeyondBatchCeiling"
echo "    Scale: relayBatchSize(512) + 88 = 600 Redis engine decisions"
echo "    Assertions:"
echo "      - all 600 bids return DECIDED + ENGINE_DURABLE"
echo "      - Redis Stream contains all 600 decisions"
echo "      - first 512-entry stream page has valid payloads and seq 1..512"
echo "      - relay drains all 600 decisions over multiple passes"
echo "      - Redis pending hash returns to 0"
echo "      - relay cursor prevents duplicate Kafka append"
echo "      - engine is not paused"

(cd backend && go test ./internal/redisengine -run '^TestRelayBackpressureDrainsBeyondBatchCeiling$' -count=1 -v)

echo ""
echo "PASS"
