# PTS Report 6A3YX7NG Review

Date: 2026-05-30

Verdict: `CORRECTNESS_PASS_FOR_PTS_1B_CONTENTION`, with workload-scope caveat.

This report is valid evidence for PTS-1B final-window contention: simultaneous
high-stride bid pressure, deterministic low-price rejection, complete engine
ordering, and final highest-price winner. It must not be used as accepted hot
path throughput evidence; PTS-1A/PTS-2 cover that separately.

## PTS Overview

- report id: `6A3YX7NG`;
- report name: `PTS-1B-20260530030931`;
- time: `2026-05-30 03:09:31` to `2026-05-30 03:10:31`;
- `AgentCount=2`;
- `Vum=891`;
- `POST PTS-1 hotspot bid` count: `1000`;
- PTS success rate: `100%`;
- PTS report-details average RT: about `170.86ms`;
- PTS report-details p90/p99 fields: `421.90ms` / `497.95ms`, but these
  percentile fields are internally inconsistent with the same API's `MaxRt=314ms`
  and must not be cited as valid percentile evidence.
- PTS console summary observed by operator: average TPS `5`, peak TPS `144`,
  average response time `171ms`, total requests `1000`, errors `0`, assertion
  failures `0`.

PTS sampling logs are sampled detail only: `sampling-logs.jsonl` contains 13
records and `sampling-logs.csv` contains 14 lines including header. Correctness
must be judged from server truth and invariant gates, not from sampled rows.

Official PTS documentation explains the small sampling-log count: sampling logs
are collected at a default `1%` rate and are intended for request/response detail
diagnosis, not as the full request ledger. Alibaba Cloud also exposes JMeter
sampler aggregate metrics separately through `GetJMeterSampleMetrics`. Therefore:

- PTS console and sampler aggregate APIs are the PTS-side summary sources for
  counts, RT, and TPS. For this report, the console summary and
  `GetJMeterReportDetails` disagree on TPS, and the report-details percentile
  fields are impossible because p90/p99 exceed max. Treat the percentile fields
  as invalid until Alibaba support or a later run provides a self-consistent
  aggregate export.
- `GetJMeterSamplingLogs` is a sampled diagnostic view. With a 1000-request
  one-shot run, about 10 detailed samples is normal at 1% sampling; 13 samples in
  this report is consistent with that order of magnitude.
- Sampling logs cannot prove or disprove full-run p95/p99. They can only provide
  example lifecycle timings and response bodies.
- `GetJMeterSampleMetrics` for the real POST sampler (`sampler-id=2`) gives
  one-second aggregate buckets only, not full-run percentile distribution. It
  shows all 1000 POST requests in two one-second buckets: `513` then `487`,
  with per-bucket max RT `314ms` and `257ms`.
- Server-side PostgreSQL/Redis/Kafka evidence remains the full business truth
  for correctness, uniqueness, engine order, winner, reject legality, and
  settlement.

Sources:

- Alibaba Cloud PTS "view sampling logs" documentation: sampling logs are
  collected at `1%` and retained for 30 days.
- Alibaba Cloud PTS pay-as-you-go documentation: pressure log sampling rate
  defaults to `1%`, and higher sampling rates increase VUM cost.
- Alibaba Cloud PTS JMeter API overview: `GetJMeterSampleMetrics` returns
  sampler aggregate data, while `GetJMeterSamplingLogs` returns sampler
  sampling logs.

## Server Truth

- PostgreSQL unique bids: `1000`;
- accepted: `12`;
- rejected: `988`, all settled;
- Redis/Kafka ledger metrics: `ENGINE_ACCEPTED=12`, `ENGINE_REJECTED=988`;
- Redis Lua bid script count: `1000`;
- `auc_live.current_price_cents=5000010000`;
- `auc_live.current_winner_id=k6_bidder_143_5`;
- `auc_live.accepted_bid_count=12`;
- `auc_live.seq=12`;
- `auc_live.engine_seq=1000`;
- public `auction_events=12`, contiguous `seq=1..12`;
- `redis_engine_settlements=1000`, contiguous `engine_seq=1..1000`;
- outbox delivery: `12` public events, all `PUBLISHED`;
- Kafka DLQ empty;
- Kafka consumer lag zero;
- Redis pending decisions empty;
- `engine_paused=false`.

## Correctness Audit

The original verification script already covered core system gates: Redis/Kafka
to PostgreSQL accepted-count match, no pending/DLQ/consumer lag, no duplicate
client bid id, no duplicate accepted engine sequence, public event seq
continuity, outbox drained, Redis no-eviction policy, and settlement terminality.

For PTS-1B, that was not enough. The missing business-critical assertions were:

- auction winner/current price equals the highest accepted bid;
- every engine decision sequence from `1..1000` is present;
- every `BID_TOO_LOW` reject is justified by the price floor at its engine order;
- every bid has a matching terminal settlement row;
- accepted bids have exact public event payload mappings;
- public events have published outbox delivery rows;
- bid idempotency `response_json` exactly matches persisted bid status, amount,
  engine epoch/seq, and reject reason.

Those assertions have now been added to
`tests/pts/verify-l4b-pts-correctness.sh` as P0 gates and were rerun against
`6A3YX7NG`. All passed.

Additional adversarial SQL also passed:

- winner matches highest accepted: true;
- engine sequence completeness: min `1`, max `1000`, actual `1000`, missing `0`;
- accepted non-increasing violations: `0`;
- unjustified rejects: `0`;
- accepted bid to exact auction event mapping missing: `0`;
- outbox exact mapping missing: `0`;
- idempotency exact coverage missing: `0`;
- settlement exact coverage missing: `0`;
- response body violations: `0`.

## Interpretation

PTS-1B intentionally creates a contention/reject workload. Only `12` of `1000`
bids were accepted because higher prices reached the engine early; the later
lower prices were correctly rejected as `BID_TOO_LOW`. This is the expected
signal for this workload, not a product failure.

The correctness claim is strong for this scope: all 1000 requests became exactly
one durable engine decision, engine order is gapless, rejects are rule-justified,
the final database winner is the maximum accepted price, and public/outbox/idempotency
state agrees with the persisted bid decisions.

## Performance Questions And Optimization Scope

This run should not be oversold. It proves PTS-1B correctness under contention,
but it also exposes performance questions that need dedicated measurement before
any "extreme performance" claim.

### HTTP RT Tail

The local `report-details.json` and `get-jmeter-report-details` response record:

- average RT: about `170.86ms`;
- p90: about `421.90ms`;
- p99: about `497.95ms`;
- max RT: `314ms`.

The `p99 > max` inconsistency means those percentile fields are invalid. After
pulling `GetJMeterSampleMetrics`, the real POST sampler (`sampler-id=2`) shows
two non-zero one-second buckets:

| Timestamp(ms) | Count | TPS | Avg RT | Min RT | Max RT | HTTP |
|---:|---:|---:|---:|---:|---:|---|
| `1780081822000` | `513` | `511.62` | `165ms` | `25ms` | `314ms` | `200` |
| `1780081823000` | `487` | `525.35` | `177ms` | `0ms` | `257ms` | `200` |

This resolves one part of the confusion: PTS did record the 1000 POST requests
as a roughly two-second client-side burst in the sampler time series, and the
per-second max RT never exceeds `314ms`. It does not recover true full-run
p95/p99 because this API does not return raw request latencies or percentile
histograms for the sampler. The report-details p90/p99 values remain
self-contradictory and should not be cited.

The `271ms` server-side number is not single-request Redis Lua latency. It is
the span between the first and last persisted engine `server_time_ms` across all
1000 bid decisions:

- `1000/1000` decisions had engine `server_time_ms` within `271ms`;
- `697/1000` were within `250ms`;
- `1000/1000` were within `500ms`.

Current code shows HTTP return is not waiting for Kafka append. The hot path is:

1. HTTP auth/schema;
2. PostgreSQL ACL membership check;
3. bid admission idempotency probe and limiter;
4. Redis guard;
5. Redis ledger Lua decision and Redis pending/idempotency write;
6. HTTP response.

Kafka append happens later in the Redis engine worker through
`ProcessPendingAppends`; PostgreSQL settlement happens after `ProcessKafka`.
Therefore the HTTP p99 hypothesis list should focus on PTS agent/network,
connection reuse, gateway DB ACL/idempotency probes, Redis/Go scheduling, and
handler-level queueing, not synchronous Kafka producer latency.

Required next measurement before optimization:

- add `auction_bid_http_latency_seconds` and stage histograms for ACL,
  admission DB probe, Redis guard, Redis engine Lua, response write;
- keep PTS console screenshots and `GetJMeterSampleMetrics` aggregate output;
- collect Go pprof, Redis latency, PostgreSQL wait events, and host CPU/IO
  during the pressure window.

### Settlement Lag

PostgreSQL `created_at` for the 1000 bid rows spans about `6090ms`. Comparing
engine `server_time_ms` to PG `created_at` gives:

- p50: about `3228ms`;
- p90: about `5372ms`;
- p99: about `5865ms`;
- max: about `5916ms`.

This is an asynchronous settlement lag. It does not block the immediate HTTP
result, but it is not free: if the product claims sub-second durable PostgreSQL
convergence, this run does not prove it. Treat it as a backlog/lag optimization
target and alert condition, not as a failure of PTS-1B correctness.

Likely contributors in the current implementation:

- `ProcessPendingAppends` drains at most `100` decisions per cycle for one
  auction;
- Kafka append is per decision;
- `ProcessKafka` settles and commits messages one by one;
- all bids for one auction use the same Kafka key, preserving order but limiting
  same-auction parallelism.

Optimization candidates must preserve same-auction engine order:

- measure pending-to-Kafka, Kafka-fetch-to-PG, PG transaction, and commit time
  separately;
- batch append/drain where order and recovery proof remain intact;
- reduce PostgreSQL transaction work per bid;
- add per-auction settlement lag metrics and alerts;
- parallelize across auctions/partitions, not inside one auction's ordered
  decision stream unless a reorder-proof design is added.

### Durable Command Log Contract

If the ADR claim is "return only after synchronous durable ordered command log",
this Redis-ledger implementation needs a precise definition of that log. The
current HTTP path returns after Redis ledger Lua writes pending state; Kafka and
PostgreSQL are asynchronous. That can be a valid low-latency design only if the
Redis side is explicitly treated as the durable command log and is configured and
tested for that role. Otherwise, the implementation should either:

- synchronously append to Kafka before returning; or
- synchronously commit a PostgreSQL command/outbox row before returning; or
- revise the ADR to state the actual Redis-pending durability contract, including
  AOF/replica/WAIT/reconciliation guarantees and crash tests.

Do not present the current path as "Kafka durable before return"; the code does
not do that.

## Evidence

- PTS report details: `docs/perf/pts/evidence/6A3YX7NG/report-details.json`
- PTS sampled logs: `docs/perf/pts/evidence/6A3YX7NG/pts-sampling-logs/`
- server after snapshot:
  `docs/perf/pts/evidence/after-6A3YX7NG-pts-1b-contention-burst-1000vu/`
- main gates:
  `docs/perf/pts/evidence/after-6A3YX7NG-pts-1b-contention-burst-1000vu/l4b-invariant-gates.tsv`
- detailed correctness output:
  `docs/perf/pts/evidence/after-6A3YX7NG-pts-1b-contention-burst-1000vu/l4b-correctness.txt`
- extra adversarial audit:
  `docs/perf/pts/evidence/after-6A3YX7NG-pts-1b-contention-burst-1000vu/pts-1b-adversarial-correctness.txt`
