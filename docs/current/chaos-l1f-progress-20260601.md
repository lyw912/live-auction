# Chaos And L1-F Progress Checkpoint - 2026-06-01

Status: temporary checkpoint for resuming the industrial chaos/L1-F hardening work.

## Current Stop Point

Work paused after completing and verifying the first three L1-F fault classes:

| Fault | Status | Evidence directory |
|---|---|---|
| `redis` | PASS | `docs/perf/pts/evidence/incoming/pts-1c-redis-20260601T035127/` |
| `redis-flush` | PASS | `docs/perf/pts/evidence/incoming/pts-1c-redis-flush-20260601T040603/` |
| `kafka` | PASS | `docs/perf/pts/evidence/incoming/pts-1c-kafka-20260601T042145/` |

Raw evidence directories remain local under `docs/perf/pts/evidence/incoming/`.
They are intentionally not part of the checkpoint commit because they include large
k6 JSON/evidence output.

## Important Interpretation Note

The L1-F runner uses closed-loop k6 virtual users. `K6_VUS=200` means 200
concurrent looping workers for the whole duration, not 200 total bid attempts.
Therefore a run can legitimately produce tens of thousands of bid decision log
entries. The decision count is throughput over time.

## Key Fixes Completed

- Redis engine reconcile now clears only narrow, recoverable engine pauses after
  reconciliation proves PostgreSQL, Redis pending state, and Kafka settlement are
  consistent.
- Recoverable Redis/Kafka append uncertainty no longer leaves the auction
  permanently paused after catch-up.
- Outbox relay no longer converts transient Redis/publish infrastructure errors
  into `DEAD` deliveries after `max_attempts`; those remain retriable with capped
  backoff. Payload/hash corruption still dead-letters immediately.
- L1-F runner now:
  - starts/stops the backend through a pid file instead of broad process kills;
  - supports Docker k6 fallback when local k6 is absent;
  - seeds mock auth users and Redis ACL cache for the 200 VU workload;
  - waits for settlement, Redis pending hash, outbox, and Kafka lag convergence;
  - requests reconcile/resume through `system_control_signals`;
  - handles Redis `FLUSHALL` as an intentional Redis data-loss profile.
- Toxiproxy has been moved out of the default compose file into
  `infra/docker-compose.toxiproxy.yml`, with an explicit chaos profile runner.

## Verification Already Run

Focused checks passed:

```bash
bash -n tests/pts/run-pts-1c-concurrent-fault.sh tests/pts/verify-l4b-pts-correctness.sh

cd backend
go test ./internal/redisengine ./internal/outbox \
  -run 'TestReconcile(DoesNotPauseWhenStreamHasPendingEntries|ClearsRecoverablePauseAfterSettlementCatchesUp)|TestRelay(RedisUnavailableRemainsRetriableAfterMaxAttempts|InvalidEnvelopeDeadLettersWithoutRetry)' \
  -count=1

node --check tests/chaos/run-toxiproxy-scenario.mjs
node tests/chaos/validate-toxiproxy-config.mjs
```

## Pending Work

Resume in this order:

```bash
FAULT_TYPE=pg bash tests/pts/run-pts-1c-concurrent-fault.sh
FAULT_TYPE=settlement bash tests/pts/run-pts-1c-concurrent-fault.sh
FAULT_TYPE=both bash tests/pts/run-pts-1c-concurrent-fault.sh
```

Then run toxiproxy chaos:

```bash
bash tests/chaos/run-chaos-profile.sh
node tests/chaos/run-toxiproxy-scenario.mjs redis_latency_reconnect --run
node tests/chaos/run-toxiproxy-scenario.mjs redis_timeout_reconnect --run
node tests/chaos/run-toxiproxy-scenario.mjs postgres_bid_latency --run
```

If a remaining fault fails, diagnose whether the failure is a real system bug or
a harness bug. Do not weaken P0 gates only to get a green script.

## Local State Notes

- Docker may still have the normal app dependencies running from the last pass.
- Toxiproxy is not part of the default compose startup; it must be started with
  the override compose file only when chaos tests need it.
- `docs/perf/pts/evidence/incoming/` was about 8 GB at checkpoint time and should
  stay local unless a reviewed evidence subset is promoted deliberately.
