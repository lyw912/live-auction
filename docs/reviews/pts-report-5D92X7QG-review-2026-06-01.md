# PTS Report 5D92X7QG Review

Date: 2026-06-01
Reviewer: Codex performance/test review
Git SHA: `92fa6fd` plus dirty workspace
Runtime profile/env source: scripted PTS-1B reset, `BID_ENGINE_MODE=redis_ledger`, `ADMISSION_ENABLED=false`
Workload: L1-C1 / PTS-1B final-second contention burst, 1000 users, one bid per user
JMX: `tests/pts/L1-component/pts-1b-contention-burst-1000vu-1m.jmx`
CSV: `docs/perf/pts/pts-1ab-1000vu-sessions.csv`
Reset label: `L4B_PROFILE=pts-1b SESSION_COUNT=1000`
Preflight label: `before-l1c1-after-l1f`
Server evidence path: `docs/perf/pts/evidence/incoming/5D92X7QG/`
Verifier command: `FINAL_WAIT_SECONDS=0 bash tests/pts/verify-l4b-pts-correctness.sh 5D92X7QG`
Final classification: `CURRENT_FAILING` for strict end-to-end PTS-1B latency; `CURRENT_PASSING_CORRECTNESS` and `SERVER_CORE_PASS`

## Verdict

Correctness passed. All P0/P1 verifier gates passed: 1000 unique users, 1000 unique `client_bid_id`s, 1000 terminal bid decisions, complete `engine_seq=1..1000`, no DLQ, no settlement gap, outbox drained, and final winner equals the highest accepted bid.

Strict end-to-end performance did not fully pass if the SLA is interpreted as Alibaba PTS sampling-log p99 <= 50ms. The 1000-row sampling log shows client-observed p99 = 64ms. The server-side hot path did pass: Prometheus gateway `stage="total"` p99 is approximately 46ms and 999/1000 requests finished within the 50ms bucket.

This report is useful evidence that the L1-F recovery changes did not introduce a measurable L1-C1 hot-path regression. It is not enough by itself to claim strict user-visible p99 <= 50ms.

## Load Model

| Field | Value |
|---|---:|
| PTS report id | `5D92X7QG` |
| PTS report window | 2026-06-01 22:20:42 to 22:21:42 |
| Agents | 2 |
| Vum | 1532 |
| Intended unique bids | 1000 |
| Actual unique bids | 1000 |
| Workload type | PTS-1B contention burst |
| Runtime profile | `redis_ledger`, admission disabled |
| Admission | disabled, `auction_admission_enabled 0` |

## PTS And HTTP Metrics

| Metric | Value | Source |
|---|---:|---|
| POST sampler count | 1000 | sampling logs, 1000 rows |
| Report-details sampler count | 198 | Aliyun `get-jmeter-report-details` |
| HTTP 200 / 202 / failures | 1000 / 0 / 0 | sampling logs |
| Sampling-log p50 / p90 / p95 / p99 / max | 19ms / 39ms / 52ms / 64ms / 134ms | sampling logs |
| Connect p99 / max | 31ms / 62ms | sampling logs |
| Actual request window | 1716ms | sampling logs, min start to max end |
| Server gateway avg | 11.55ms | `auction_bid_gateway_stage_seconds_sum/count` |
| Server gateway p99 | ~46ms | Prometheus bucket interpolation |
| Server Redis engine avg | 5.82ms | `stage="redis_engine"` |
| Server Redis engine p99 | ~17.5ms | Prometheus bucket interpolation |

PTS `get-jmeter-report-details` is inconsistent for this report: it reports `AllCount=198`, while sampling logs contain 1000 successful POST rows and server evidence has 1000 bid decisions. Use sampling logs plus server evidence as the audit source for this run.

## Engine Decision Distribution

| Business result | Count |
|---|---:|
| `ENGINE_ACCEPTED` | 7 |
| `ENGINE_REJECTED` | 993 |
| `ENGINE_SOLD` | 0 |
| `RECONCILING` / `PROCESSING_RETRY_LATER` | 0 |

All 1000 responses were synchronous final decisions with HTTP 200.

## Correctness Gates

| Gate | Result | Evidence |
|---|---|---|
| 1000 intended unique bids classified | PASS | `pts_expected_total_bid_rows`, `pts_expected_unique_client_bid_ids`, `pts_expected_unique_users` |
| final highest valid amount is winner | PASS | `auction_winner_matches_highest_accepted` |
| every low reject justified at decision time | PASS | `bid_too_low_rejects_justified` |
| idempotency response consistency | PASS | `idempotency_response_matches_bid` |
| Kafka/engine_seq continuity | PASS | `engine_seq_complete`, `kafka_offset_matches_engine_order` |
| PostgreSQL settlement complete | PASS | `every_bid_has_settled_ledger` |
| public auction seq contiguous | PASS | `no_public_auction_event_seq_gap` |
| no DLQ / pending / pause left unresolved | PASS | `dlq_empty`, `engine_not_paused`, `redis_pending_decisions_empty` |
| verifier exit code | PASS | `l4b-correctness.txt`, `l4b-invariant-gates.tsv` |

## Comparison With UQ69X7RG And IV68X7KG

| Metric | IV68X7KG | UQ69X7RG | 5D92X7QG |
|---|---:|---:|---:|
| Sampling-log rows | 1000 | 1000 | 1000 |
| Sampling-log p99 | 68ms | 101ms | 64ms |
| Sampling-log max | 78ms | 116ms | 134ms |
| Connect p99 | 19ms | 74ms | 31ms |
| Actual request window | 500ms | 220ms | 1716ms |
| Server gateway avg | 16.12ms | 17.64ms | 11.55ms |
| Server gateway p99 estimate | ~90.6ms | ~48.2ms | ~46.1ms |
| Server Redis engine avg | 5.06ms | 8.22ms | 5.82ms |
| Server Redis engine p99 estimate | ~10.0ms | ~24.6ms | ~17.5ms |
| Accepted / rejected | 12 / 988 | 18 / 982 | 7 / 993 |

The newest run is not slower than the previous two on the measured hot path. Its client p99 is lower than both prior reports, and its server gateway average/p99 are also lower than or equal to the best prior evidence.

The important caveat is pressure shape. `5D92X7QG` spread the 1000 POSTs over about 1.716s, while `IV68X7KG` and `UQ69X7RG` compressed them into about 500ms and 220ms. This makes `5D92X7QG` less severe as a peak-burst test, so it should not be used to claim that peak contention capacity improved. It does support the narrower claim that the L1-F changes did not cause an L1-C1 regression.

## L1-F Change Impact

Production files changed during L1-F work:

| File | Change | L1-C1 hot-path impact |
|---|---|---|
| `backend/internal/redisengine/engine.go` | Pause on corrupted Redis idempotency replay; settlement retry/DLQ safety; reconcile status adjustment | Normal one-shot unique bids do not hit replay failure paths. No measured regression in `5D92X7QG`. |
| `backend/internal/redisengine/kafka_ledger.go` | Keep one fetched Kafka message uncommitted until explicit commit | Settlement consumer path, not synchronous bid decision path. No measured gateway regression. |
| `backend/internal/outbox/relay.go` | Filter relay control signals and respect lock expiry | Control-signal processing only, not bid POST hot path. |

The only changed code near the synchronous POST path is defensive idempotency replay handling. The PTS workload uses one unique `client_bid_id` per user and does not replay a corrupted idempotency record, so this branch was not exercised in the hot path. Metrics confirm the hot path did not slow: server gateway avg improved from 16.12/17.64ms to 11.55ms.

## Bottleneck Attribution

| Candidate | Evidence | Verdict |
|---|---|---|
| L1-F production code changes | Hot-path server metrics did not regress; changed branches mostly settlement/recovery/control paths | unlikely cause of latency |
| PTS/load-generator connection setup | connect p99 = 31ms, max = 62ms | contributes to client p99 gap |
| Pressure shape | this run spread requests over 1.716s | makes this run less severe than prior 220ms/500ms bursts |
| Redis single-writer queue | Redis engine p99 ~17.5ms | not the main reason client p99 exceeded 50ms |
| PTS summary aggregation | report-details `AllCount=198` contradicts 1000 sampling rows/server decisions | do not use report-details alone |

## Required Next Action

- P0 before a final judge-facing rerun: decide whether the official SLA is server core p99 or PTS client p99. If it is client p99, this report is not a strict pass.
- P1 adjust the JMX to model a realistic auction-room user with a warmed connection before the synchronized final bid, or explicitly document that one-shot PTS includes TCP connect time and is stricter than the real in-room click path.
- P1 rerun L1-C1 after the JMX decision, because `5D92X7QG` is correct and non-regressing but not a clean strict end-to-end p99 <= 50ms pass.

This report is not CURRENT_PASS evidence for current PTS-1B because Alibaba PTS sampling-log end-to-end p99 is 64ms, above the 50ms strict user-visible target.
