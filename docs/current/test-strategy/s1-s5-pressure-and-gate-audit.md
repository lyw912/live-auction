# S1-S5 Pressure And Gate Audit

> Status: judge-defense audit, 2026-06-05.
> Scope: explain, for each S1-S5 scenario, how we prove the intended pressure
> actually reached the system and how the PASS/FAIL gates are checked. This is
> the document to use when a reviewer asks "what exactly did PASS mean?"

## 1. Audit Rule

No S1-S5 pass is accepted from one chart alone.

Every scenario needs two independent chains:

| Chain | Question | Evidence examples |
|---|---|---|
| Pressure reached | Did the workload we intended really hit the target path? | PTS/k6 counts, unique IDs, server request counters, Redis Lua counters, WebSocket subscriber-delivery counters, fault-window overlap counters |
| Correctness gates | Did the system produce the right durable and user-visible outcome? | `l4b-*gates.tsv`, PostgreSQL settlement summaries, Kafka lag/DLQ, Redis pending/cursor, outbox status, k6 checks, PTS assertions |

For bid-decision scenarios, accepted and rejected decisions both count as
successful adjudications if they return terminal `ENGINE_*` results and the
reject reason is justified. For fanout and reconnect scenarios, the proof is not
"number of bids"; it is online viewer delivery and state recovery.

## 2. Gate Taxonomy

### 2.1 Load And Identity Gates

These gates prove the pressure population is the intended population, not a
smaller or duplicated workload:

| Gate | Checked by | Failure meaning |
|---|---|---|
| expected sampler/iteration count | PTS sampler count or k6 iteration metrics | The test did not generate the intended pressure. |
| unique `client_bid_id` | PTS sampling logs + PostgreSQL `bids` | CSV split, loop override, idempotency replay, or duplicate workload. |
| unique users | verifier `pts_expected_unique_users` when an exact bid count is expected | PTS did not distribute the CSV/session population as expected. |
| server POST count | Prometheus `http_request_total` or collected evidence | Load generator saw requests that did not reach the API, or API routing was wrong. |
| Redis Lua decision count | Prometheus `redis_lua_script_total{script="bid_redis_ledger"}` | Requests did not enter the hot decision engine. |
| fault-window overlap | S4 k6/harness counters such as `bid_fault_window_decided_total` | The load ran before or after the injected fault, so the chaos run is invalid. |
| WebSocket subscriber-delivery count | `auction_ws_publish_subscribers_sum` | Fanout pressure did not scale as accepted publishes x online subscribers. |

### 2.2 Bid And Settlement Gates

These are produced by `tests/pts/verify-l4b-pts-correctness.sh` and saved as
`l4b-invariant-gates.tsv`, `l4b-kafka-gates.tsv`,
`l4b-redis-pending-gates.tsv`, `l4b-reject-reason-gates.tsv`, and
`l4b-soft-close-gates.tsv`.

| Gate family | Representative gates | Meaning |
|---|---|---|
| Terminal settlement | `no_non_terminal_settlements`, `every_bid_has_settled_ledger` | No bid decision is left in `PROCESSING`, `FAILED`, `DLQ`, or missing settlement state after the chosen wait window. |
| Redis/Kafka/PG consistency | `redis_kafka_pg_accepted_match`, `redis_engine_seq_matches_settlement`, `kafka_position_present`, `kafka_offset_matches_engine_order` | The hot Redis engine, Kafka ledger, and PostgreSQL settlement agree on accepted decisions and ordering. |
| Ordering and uniqueness | `engine_seq_complete`, `engine_epoch_seq_monotonic`, `no_duplicate_engine_seq`, `no_duplicate_client_bid_id`, `idempotency_response_matches_bid` | The auction has one complete ordered decision sequence with no duplicate bid identity or duplicate accepted engine sequence. |
| Winner and price correctness | `auction_winner_matches_highest_accepted`, `increment_grid_valid`, `auction_accepted_count_matches_pg` | The visible winner/current price is the highest valid accepted bid and accepted increments obey the auction grid. |
| Reject correctness | `bid_too_low_rejects_justified`, `rejected_bids_have_expected_reason` | Rejected bids were rejected for engine/business reasons, not hidden auth/admission/rate-limit contamination. |
| Public event/outbox | `accepted_settlement_has_public_event`, `accepted_public_event_exact_mapping`, `public_events_have_published_outbox`, `outbox_drained`, `no_public_auction_event_seq_gap` | Accepted price changes are published exactly and contiguously; no stale unpublished outbox remains. |
| Auction lifecycle safety | `no_accepted_after_final_end`, `soft_close_no_stacked_subwindow_extension`, `soft_close_extension_delta_correct`, `cap_terminal_single_sold_order`, `at_most_one_order` | Soft-close/end/cap/order behavior cannot create post-close winners, stacked extensions, or duplicate orders. |
| Room isolation | `no_cross_auction_event_payload_leak` | An event for this auction cannot carry another auction's bid or room payload. |
| Infrastructure safety | `redis_no_eviction`, `redis_noeviction_policy`, `redis_no_rejected_connections`, `dlq_empty`, `kafka_consumer_group_lag_zero`, `redis_pending_decisions_empty`, `v3_relay_stream_complete`, `v3_relay_cursor_advanced` | Redis did not evict hot state, Kafka did not leave lag/DLQ, and the Redis decision stream relay drained without losing cursor position. |

The important point: these gates are not just "count rows." They check the
business invariants that reviewers care about: no fake winner, no missing
settlement, no duplicate order, no outbox gap, no hidden admission contamination,
and no payment/finality before convergence.

## 3. Scenario Audit

### S1 Final-Second Contention

Current formal evidence: `2MLCX7WG`.

Pressure reached:

| Signal | Value | Why it matters |
|---|---:|---|
| PTS `bid-decision` rows | 1000 | The intended 1000 one-shot bidder samples exist. |
| Unique request `client_bid_id` / response `bid_id` | 1000 / 1000 | No duplicated CSV row or idempotency collapse. |
| Server POST `/bids` | 1000 | Requests reached the service API. |
| Redis Lua `bid_redis_ledger` | 1000 | Requests entered the hot decision engine. |
| Per-agent release span | 501 ms / 525 ms | Each PTS pressure machine delivered its local 500 bidders inside the intended 500 ms window. |
| Global span | `startTimeTS=1351ms`, `server_time_ms=1348ms` | The honest multi-agent boundary; do not claim one global 500 ms interval. |

Pass gates:

| Gate group | Result | Meaning |
|---|---|---|
| decision finality | 285 accepted + 715 rejected = 1000 `ENGINE_DURABLE` decisions | All bidder attempts received terminal engine decisions. |
| latency | sampling-log p99 23 ms, max 28 ms | Formal M1 p99 passes for the windowed profile. |
| bid identity | `pts_expected_total_bid_rows`, `pts_expected_unique_client_bid_ids`, `pts_expected_unique_users` PASS | The 1000-row workload is not duplicated or truncated. |
| ordering | `engine_seq_complete`, `engine_epoch_seq_monotonic`, `no_duplicate_engine_seq` PASS | The Redis engine produced one complete ordered decision sequence. |
| winner/reject correctness | `auction_winner_matches_highest_accepted`, `bid_too_low_rejects_justified` PASS | The final winner is the highest accepted bid; rejects are justified by the engine price floor at their decision seq. |
| durability/drain | `every_bid_has_settled_ledger`, `kafka_consumer_group_lag_zero`, `redis_pending_decisions_empty`, `outbox_drained` PASS | Kafka/Redis/PG/outbox all converged after the burst. |

Judge-safe wording:

> "S1 passed because the exact 1000-bid population reached both the API and
> Redis engine, all 1000 decisions became durable settlements, the ordered
> engine sequence was complete, the winner and rejects were verified from
> persisted state, and all convergence gates drained. The accepted count is not
> the pressure count; accepted plus correctly rejected decisions is the pressure
> population."

### S2 Steady Soak And Convergence Drain

Current evidence:

- Long soak: `s2-ecs-30m-20260604T095720`.
- Convergence drain: `s2-convergence-drain-decision-ecs-20260604T1937`.

Pressure reached:

| Signal | Long soak | Convergence drain | Why it matters |
|---|---:|---:|---|
| Load model | independent same-VPC k6 open arrival | independent same-VPC k6 open arrival | Pressure source is separate from the service host and does not self-throttle like closed VU loops. |
| Final decisions | 85,499 | 49,049 | Offered pressure became terminal `ENGINE_*` decisions. |
| dropped_iterations | 0 | 0 | k6 did not fail to schedule the intended open-arrival load. |
| HTTP/auth/ACL/admission/non-decision failures | 0 | 0 | The run was not contaminated by auth, ACL, admission, or non-terminal responses. |
| Decision p99 | 4 ms custom / 3.30 ms HTTP p99 | 4 ms custom | Redis/gateway foreground path stayed fast. |

Pass gates:

| Gate group | Result | Meaning |
|---|---|---|
| settlement terminality | all decisions settled; `no_non_terminal_settlements` PASS | Payment/finality is not opened while decisions remain processing/failed/DLQ. |
| Kafka drain | `kafka_consumer_group_lag_zero` and `dlq_empty` PASS | Settlement worker consumed the Kafka ledger and no DLQ work remains. |
| Redis relay drain | `v3_relay_stream_complete`, `v3_relay_cursor_advanced`, `redis_pending_decisions_empty` PASS | Redis decision stream was relayed; no pending hash or stuck cursor. |
| PG/outbox convergence | outbox unpublished 0; `outbox_drained`, `public_events_have_published_outbox` PASS | Public events are published and viewer/payment state is not stale. |
| business invariants | winner, reject, idempotency, engine seq, accepted event mapping gates PASS | Sustained pressure did not corrupt winner, order, idempotency, or reject basis. |

Important distinction:

- S2 decision/reject-heavy convergence drain proves normal high decision volume
  drains cleanly.
- Accepted-heavy 600/s remains attack evidence for the async settlement/outbox
  knee. Do not use the 600/s accepted-heavy trial as a clean payment/finality
  pass.

Judge-safe wording:

> "S2 passed because the independent open-arrival generator delivered the stated
> final decision count with dropped=0, and service-side Redis/Kafka/PG/outbox
> gates drained to zero. Foreground p99 and back-office convergence are reported
> separately because payment safety depends on the latter."

### S3 Single-Room Fanout

Current evidence:

- PTS live-only: `XWLAX70G`.
- PTS mixed final-burst: `20L8X79G`.
- Local 1000-WS live-only: `s3-local-scale-1000-liveonly-20260602T2303`.

Pressure reached:

| Signal | `XWLAX70G` | Why it matters |
|---|---:|---|
| accepted update source | 100 accepted bids | There were real price-changing publishes, not idle sockets. |
| viewer sampler chains | 2994 | PTS established about 3000 same-room viewer flows. |
| `S3 live fanout receive` sampler | 2994, 100% success/assertion | Each viewer executed the live receive sampler. |
| service subscriber-delivery surface | `auction_ws_publish_subscribers_sum=299400` | Backend saw `100 accepted publishes * 2994 subscribers`; this is the true fanout surface. |
| PostgreSQL/outbox | 100 accepted bids, 100 outbox rows, pending 0 | Business effects and publication drain match the update source. |
| WS queues/connections | queue depth 0, connections closed after run | No hidden backlog or leaked sockets after the run. |

Pass gates:

| Gate | Checked by | Meaning |
|---|---|---|
| join snapshot success | PTS `S3 viewer join snapshot` success/assertion | Viewers can enter the room and see initial state. |
| ticket success | PTS `S3 POST WS ticket` success/assertion | WebSocket auth/ticket path did not fail under load. |
| handshake complete | PTS `S3 WS handshake complete` success/assertion | Long-lived sockets were actually established. |
| first snapshot/business message | PTS first-message sampler success/assertion | The realtime channel becomes useful, not just TCP-open. |
| live fanout receive | PTS receive sampler success/assertion + sampled markers | Online viewers received live price updates. |
| subscriber-delivery volume | `auction_ws_publish_subscribers_sum` | PTS sampler count is not raw frame count; service metric proves publish x subscribers. |
| outbox drain | PostgreSQL outbox summary + service evidence | Accepted price updates were published; no stale unpublished deliveries remain. |

Important distinction:

- PTS `S3 live fanout receive=2994` is one receive/observe sampler per viewer,
  not `2994 * 100` request rows.
- `299400` is not database rows. It is a service metric counting subscriber
  deliveries.
- With 1% PTS sampling, sampling logs are response-marker forensics. Exact
  volume proof comes from PTS API-list counts plus service metrics.

Judge-safe wording:

> "S3 passed because about 3000 viewer flows joined, opened WebSockets, received
> first realtime state, and executed the live receive sampler with 100% success,
> while the service independently recorded 299400 subscriber deliveries and zero
> queue/outbox backlog. The database correctly has 100 accepted bids/outbox
> effects; the larger number is fanout delivery surface, not stored rows."

### S4 Fault Resilience

Current evidence:

- Local P0/P1 faults: Redis, backend/settlement, PG, Kafka, Redis FLUSHALL,
  Redis+Kafka.
- Independent Kafka fault: `s4-p1-kafka-independent-20260604T202510`.
- P2 Redis partial network: `pts-1c-partial-20260604T224626`.
- Depth gates: `07-relay-backpressure`, `08-settlement-idempotency`.

Pressure reached:

| Signal | Example | Why it matters |
|---|---|---|
| active bidder loops | 200 VU, 25s, snapshot -> bid -> sleep about 1s | S4 is not one request per user; it is repeated live bidding during a fault. |
| fault-window counters | Kafka independent `bid_fault_window_decided_total=1000` | Proves load overlapped the injected dependency fault. |
| independent k6 source | VPC private path, k6 host CPU/RSS/TCP healthy | Avoids the claim that the service host self-loaded the result. |
| service settlement count | Kafka independent 5000/5000 settlements | Fault traffic became durable settlement state. |
| fault-specific client classes | `decided`, `paused`, `reconciling`, `http_errors`, `accepted-in-window=0` | Shows whether users saw safe decisions, safe pause, or expected backend failure. |

Pass gates:

| Gate group | Meaning |
|---|---|
| RPO=0 | No accepted durable bid is lost, duplicated, or left unsettled after recovery. |
| fail-closed Redis behavior | During Redis truth loss or partial network, no fake accepted bid is created; clients see paused/reconciling/retry semantics. |
| PG/Kafka convergence | Foreground Redis decisions may continue, but payment/finality waits until Kafka lag, Redis pending, PG settlement, and outbox are zero. |
| idempotent replay | Backend/settlement crash and S4 08 prove repeated Kafka delivery creates one settlement/order/outbox business effect. |
| relay backlog safety | S4 07 proves more decisions than one relay batch ceiling drain over multiple passes without skipped cursor or duplicate append. |
| normal L4B invariants | Winner, reject basis, engine order, event/outbox, no duplicate client/seq/order all remain PASS after the fault. |

Example: `s4-p1-kafka-independent-20260604T202510`:

| Signal | Value |
|---|---:|
| k6 iterations / decisions | 5000 / 5000 |
| fault-window decisions | 1000 |
| HTTP failed / admission contamination | 0 / 0 |
| accepted / rejected | 15 / 4985 |
| p99 | 43.04 ms |
| service settlements | 5000/5000 |
| Kafka lag / Redis pending | 0 / 0 |
| verifier gates | PASS |

Judge-safe wording:

> "S4 passed only when the injected fault overlapped live bidding and the
> post-fault durable state still satisfied RPO=0, no phantom accepts, no duplicate
> settlement/order/outbox, zero Kafka lag, zero Redis pending, and normal auction
> invariants. A fault-window decision count proves overlap; the gates prove
> safety."

### S5 Reconnect Recovery

Current evidence:

- Clean reconnect: `s5-20260604T221312`.
- Toxiproxy reset path: `s5-20260604T231925`.

Pressure reached:

| Signal | Clean | Toxiproxy reset | Why it matters |
|---|---:|---:|---|
| recovered iterations | 34,814 | 8,849 | Users actually missed seqs and recovered; not idle sockets. |
| bid source accepted updates | n/a in summary table / active source | 1201 | Public sequence advanced during the test. |
| reconnect retries/errors | normal reconnect churn | 3,826 | The proxy fault was active; this was not a clean path accidentally bypassing Toxiproxy. |
| server monitor | `ws_reconnect` / `ws_recovered` distributions | `ws_reconnect=21,574`, `ws_recovered(history)=16,584`, `db)=4,913` | Backend recovery paths were exercised. |

Pass gates:

| k6 check / metric | Current Toxiproxy result | Meaning |
|---|---:|---|
| `s5 initial socket connected before disconnect` | 8849 pass / 0 fail | The client was genuinely online before the forced reconnect path. |
| `s5 missed real events before reconnect` | 8849 pass / 0 fail | The reconnect client actually missed public seqs. |
| `s5 caught up in time` | 8849 pass / 0 fail | Reconnect reached current state within the TTCS SLO. |
| `s5 no seq gap in recovered stream` | 8849 pass / 0 fail | Incremental recovery did not skip seqs. |
| `s5 no duplicate seq in recovered stream` | 8849 pass / 0 fail | Recovery did not replay duplicate seqs to the client. |
| `s5_truth_mismatch` | 0 | Client final price/winner matched server truth. |
| `s5_recovery_errors` | 0 | No terminal recovery failure after retries. |
| `s5_ttcs_ms` | p99 341 ms | Time-to-current-state is sub-second and below the 2s gate. |

Important distinction:

- S5 is not fanout p99. It measures stale-`last_seq` recovery after missed
  events.
- Recovered counts are recovery attempts, not database rows.
- Retry counts in the Toxiproxy run are not contamination; they prove network
  turbulence was active. The pass condition is zero final recovery gaps,
  duplicates, truth mismatches, or recovery errors.

Judge-safe wording:

> "S5 passed because clients first connected, then missed real public seqs, then
> reconnected with stale `last_seq` and caught up to server truth within the
> TTCS gate. The checks assert no gaps, no duplicate seqs, no truth mismatch, and
> no final recovery errors. The Toxiproxy run also recorded thousands of retry
> errors, so the network fault was not bypassed."

## 4. What A PASS Does Not Claim

| Scenario | Explicit non-claim |
|---|---|
| S1 | Does not prove one global 1000-user 500 ms interval; it proves 500 ms per pressure agent with measured 1.35 s global span. |
| S2 | Does not prove accepted-heavy 600/s immediate settlement/outbox drain; accepted-heavy remains capacity-knee evidence. |
| S3 | Does not prove 10k viewers or per-frame p99 over every delivered WebSocket message; it proves the tested 3000-viewer PTS point and service subscriber-delivery surface. |
| S4 | Does not prove Kafka RF=3/minISR=2 or Redis HA production failover; it proves local functional fail-closed/replay/convergence safety. |
| S5 | Does not prove browser/mobile/LB weak-network certification; it proves backend reconnect recovery correctness and controlled Toxiproxy reset behavior. |

This is the defensible posture: strong measured claims, explicit boundaries, and
clear next topology tests where production HA or larger scale is not yet proven.
