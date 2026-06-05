# S1 Evidence Methodology

> Status: current, 2026-06-05. This document explains how S1 final-second PTS
> evidence is recomputed and defended when reviewers do not accept a PTS page at
> face value.

## Claim Boundary

S1 proves one narrow claim:

> In one hot auction, 1000 distinct authenticated bidders submit one bid each in
> the final-second burst window. The system returns synchronous `ENGINE_*`
> decisions with p99 <= 60 ms in the default `kafka_ack` response profile,
> returns `KAFKA_ACKED` for >= 99% of responses with bounded `ENGINE_DURABLE`
> fallback, classifies all 1000 bids exactly once, picks the highest valid
> winner, justifies every reject, and drains Redis/Kafka/PG/outbox state.

The current JMX default is `contention_release_window_ms=500`. That means the
script synchronizes bidders to a common final-second start and then spreads each
pressure agent's local bidders across a short 500 ms target window. The actual
delivered span is not asserted from configuration; it is recomputed after the
run from:

- PTS sampling `startTimeTS`: load-generator send-start span.
- Response `server_time_ms`: server decision timestamp span.
- Server Prometheus counters: independent request and Redis engine counts.

Because PTS scheduling, JVM threads, TCP connection reuse, same-VPC network
delivery, and multi-pressure-agent start alignment are not literally
simultaneous, the observed span is expected to be a real number. If a run
observes a wider global span, the result may still prove correctness and
latency, but it does not prove all 1000 requests arrived inside one global
500 ms wall-clock interval. The review must say the measured per-agent and
global spans explicitly.

## Raw Sources

For each judge-facing S1 run, preserve these files under
`docs/perf/pts/evidence/incoming/<REPORT_ID>/`:

| Source | Why it is needed |
|---|---|
| `pts-sampling-logs/sampling-logs.jsonl` | PTS/JMeter per-request timing, request body, response body, assertion result. Use 100% sampling for judge-facing S1 forensics. |
| `pts-report-details.json` | PTS API/page aggregate fields, used as chart context and consistency check. |
| `metrics.prom` | Server-side counters and histograms independent of the load generator. |
| `l4b-invariant-gates.tsv` | Correctness verifier gates over Redis/Kafka/PostgreSQL/outbox state. |
| `l4b-correctness.txt` | Verifier command output and SQL/diagnostic context. |

Generate the review:

```bash
PAGE_SIZE=100 bash tests/pts/fetch-pts-sampling-logs.sh <REPORT_ID>
bash tests/pts/review-s1-pts-run.sh <REPORT_ID>
```

## Exact Calculations

Sampling-log latency uses the JMeter sample field `elapsedTime`. Apache JMeter's
`SampleResult.getTime()` is the sample duration, and the same object exposes
start time, end time, latency, connect time, response code, response message,
and `setIgnore()` for hiding non-measured warmups from listeners.

For 100% sampling:

```text
N = count(rows where samplerLabel == "出价决策 bid-decision")
sorted = elapsedTime values sorted ascending
p(phi) = sorted[ceil(phi * N)] using 1-based indexing
```

This is the nearest-rank percentile. It is intentionally simple and auditable:
any reviewer can sort the 1000 values and check the 990th value for p99.

Arrival and decision-span calculations:

```text
pts_send_start_span_ms = max(startTimeTS) - min(startTimeTS)
pts_completion_span_ms = max(endTimeTS) - min(endTimeTS)
server_decision_span_ms = max(response.server_time_ms) - min(response.server_time_ms)
```

These are span checks, not latency percentiles. They answer "did the cohort
actually arrive/complete as a tight burst?" PTS average TPS does not answer that,
because report TPS can be aggregated over a wider chart bucket than the one-shot
request timestamps.

Business and identity checks:

```text
unique_client_bid_ids = count(distinct requestData.client_bid_id)
unique_response_bid_ids = count(distinct response.bid_id)
engine_seq_complete = engine_seq min..max has no gap and no duplicate
result distribution = group by response.result
durability distribution = group by response.durability_status
settlement distribution = group by response.settlement_status
```

Server-side Prometheus checks:

```text
http_request_total{method="POST",path="/api/auctions/{id}/bids",status="200"} == 1000
redis_lua_script_total{script="bid_redis_ledger",outcome="ok"} == 1000
```

Prometheus latency histograms are bucketed counters. The review reports bucket
upper bounds such as "1000/1000 <= 25 ms"; it does not invent exact server p99
from coarse buckets. If using PromQL later, use `histogram_quantile()` and state
that the value is estimated from buckets.

Correctness verifier gates are authoritative for business truth because they
query persisted and durable state after convergence:

- exactly 1000 persisted bid decisions;
- exactly 1000 distinct users and client bid IDs;
- no duplicate `client_bid_id`;
- `engine_seq` complete and monotonic;
- every bid has a terminal Redis/Kafka settlement row;
- winner/current price equals the highest accepted bid;
- every `BID_TOO_LOW` reject is justified by decision-time price floor;
- Kafka consumer lag zero and DLQ empty;
- Redis pending decisions empty;
- outbox drained and public event sequence contiguous.

## Why PTS Report p99 Can Be Rejected

PTS is still useful: its report gives the external client-view chart, presser
monitoring, report export, and API-level summaries. Alibaba Cloud documents PTS
reports as covering business metrics, load-generator resource usage, request
sampling logs, insights, and comparisons. The same documentation says presser
CPU or memory bottlenecks can distort results, which is why S1 reviews preserve
server-side metrics too.

However, a PTS aggregate field must pass basic consistency. For a finite sample,
a percentile cannot exceed the maximum observation. Prometheus's official
histogram guide defines a phi-quantile as the observation ranked at `phi * N`
among `N` observations; therefore `p99 <= max` must hold for one sampler's
finite observations. If `Seg99Rt > MaxRt`, that aggregate field is internally
invalid for the run. In that case:

- do not cite the PTS API/page p99 field;
- cite the raw 100% sampling-log nearest-rank p99;
- keep the PTS PDF/API output as evidence of the contradiction, not as the final
  latency truth;
- cross-check with server counters and correctness gates.

This is not "ignoring bad data"; it is rejecting a field that violates a
mathematical invariant and replacing it with a reproducible calculation from
the raw per-request data.

## Current Default kafka_ack Example: UIPAX7JG

`UIPAX7JG` is the current formal S1 PTS evidence after making
`BID_ENGINE_RESPONSE_DURABILITY=kafka_ack` the default response profile.

Evidence:

```text
docs/perf/pts/evidence/incoming/UIPAX7JG/
docs/perf/pts/evidence/incoming/UIPAX7JG/s1-review.md
```

Key facts from the recomputation:

- 1000 PTS sampling rows, 1000 unique request `client_bid_id`, 1000 unique
  response `bid_id`.
- Server Prometheus cross-check: 1000 POST `/api/auctions/{id}/bids`, 1000 Redis
  Lua `bid_redis_ledger` executions.
- Outcome: 264 `ENGINE_ACCEPTED`, 736 `ENGINE_REJECTED`, 1000 `DECIDED`.
- Sampled durability: 998 `KAFKA_ACKED`, 2 `ENGINE_DURABLE`. Post-run DB
  evidence showed all 1000 decisions reached Kafka and settlement.
- Engine seq complete: count=1000, min=1, max=1000, unique=1000, no gaps or
  duplicates.
- Verifier: 41/41 gates PASS; Kafka lag 0, Redis pending 0, outbox drained.
- PTS send-start span: `startTimeTS` 1780659075002..1780659075507 = 505 ms.
- Response server timestamp span: `server_time_ms`
  1780659075004..1780659075511 = 507 ms.
- Sampling-log `elapsedTime` p99 = 58 ms, max = 67 ms.
- Server/gateway histogram: 985/1000 gateway total samples are within the
  <=50 ms bucket and 1000/1000 are within the <=100 ms bucket; Redis Lua itself
  has 1000/1000 <=25 ms.

Interpretation:

`UIPAX7JG` is a clean S1 correctness pass and the current default `kafka_ack`
M1 evidence under a 60 ms envelope. It does not prove strict <=50 ms at the
Kafka-ack response boundary. The two sampled `ENGINE_DURABLE` responses are
bounded fallback, not lost or undecided requests; convergence evidence proves
they reached Kafka/PostgreSQL after the response.

## Legacy redis_aof Example: 2MLCX7WG

`2MLCX7WG` is the current formal S1 PTS evidence after returning to the
controlled `contention_release_window_ms=500` profile.

Evidence:

```text
docs/perf/pts/evidence/current/s1-s5/s1-final-second-contention-2MLCX7WG/
docs/perf/pts/evidence/current/s1-s5/s1-final-second-contention-2MLCX7WG/s1-review.md
```

Key facts from the recomputation:

- 1000 PTS sampling rows, 1000 unique request `client_bid_id`, 1000 unique
  response `bid_id`.
- Server Prometheus cross-check: 1000 POST `/api/auctions/{id}/bids`, 1000 Redis
  Lua `bid_redis_ledger` executions.
- Outcome: 285 `ENGINE_ACCEPTED`, 715 `ENGINE_REJECTED`, 1000 `DECIDED`, 1000
  `ENGINE_DURABLE`.
- Engine seq complete: count=1000, min=1, max=1000, unique=1000, no gaps or
  duplicates.
- Verifier: 41/41 gates PASS; Kafka lag 0, Redis pending 0, outbox drained.
- PTS send-start span: `startTimeTS` 1780599030514..1780599031865 = 1351 ms.
- Response server timestamp span: `server_time_ms`
  1780599030517..1780599031865 = 1348 ms.
- Sampling-log `elapsedTime` p99 = 23 ms, max = 28 ms.
- Split by PTS `instanceId`: instance 0 released 500 VU in 501 ms and instance
  1 released 500 VU in 525 ms; server response timestamp spans were 503 ms and
  524 ms. The two pressure agents were offset, so the global span was about
  1.35 s.
- Server/gateway histogram: 1000/1000 HTTP and gateway total samples are within
  the <=25 ms and <=50 ms Prometheus buckets; Redis Lua has 1000/1000 <=25 ms.

Interpretation:

`2MLCX7WG` is a clean S1 correctness and M1 latency pass for the current
windowed-burst profile. It proves 1000 final bid decisions under the configured
500 ms release window per PTS pressure agent. It does not prove all 1000 requests
arrived at the service inside one global 500 ms wall-clock interval; the measured
global multi-agent span is about 1.35 s, and that number must be reported.

## Diagnostic Example: TGLBX7GG

`TGLBX7GG` is a diagnostic strict-barrier S1 rerun with
`contention_release_window_ms=0`.

Evidence:

```text
docs/perf/pts/evidence/current/s1-s5/s1-diagnostic-strict-barrier-TGLBX7GG/
docs/perf/pts/evidence/current/s1-s5/s1-diagnostic-strict-barrier-TGLBX7GG/s1-review.md
```

Key facts from the recomputation:

- 1000 PTS sampling rows, 1000 unique request `client_bid_id`, 1000 unique
  response `bid_id`.
- Server Prometheus cross-check: 1000 POST `/api/auctions/{id}/bids`, 1000 Redis
  Lua `bid_redis_ledger` executions.
- Outcome: 10 `ENGINE_ACCEPTED`, 990 `ENGINE_REJECTED`, 1000 `DECIDED`, 1000
  `ENGINE_DURABLE`.
- Engine seq complete: count=1000, min=1, max=1000, unique=1000, no gaps or
  duplicates.
- Verifier: 41/41 gates PASS; Kafka lag 0, Redis pending 0, outbox drained.
- PTS send-start span: `startTimeTS` 1780597371515..1780597372659 = 1144 ms.
- Response server timestamp span: `server_time_ms`
  1780597371517..1780597372664 = 1147 ms.
- Sampling-log `elapsedTime` p99 = 134 ms, max = 140 ms.
- Split by PTS `instanceId`: each pressure agent released its 500 VU in about
  113-114 ms and server response timestamps spanned 117-120 ms per agent; the
  two agents were offset by about 1 second.
- Server/gateway histogram: 1000/1000 HTTP and gateway total samples are within
  the <=50 ms Prometheus bucket.

Interpretation:

`TGLBX7GG` proves the JMX did not artificially spread the burst: the configured
release window is zero. It also proves PTS did not deliver a literal 0 ms or
500 ms burst; the actual measured send-start/response timestamp spans are about
1.15 s. The root cause is PTS/JMeter distributed execution: the two pressure
agents each released locally within about 120 ms, but their barrier targets were
about 1 second apart. Therefore cite it as a strong shared-barrier pressure and
correctness artifact, not as strict M1 client-side p99 <=50 ms and not as
"1000 requests arrived within 500 ms."

External basis: Apache JMeter's synchronizing/rendezvous behavior is scoped to
threads in a JVM, and Alibaba Cloud PTS documents that JMeter assembly points are
single-pressure-machine/JVM scoped rather than a global synchronization primitive
across multiple pressure machines.

## Previous Example: 8LGBX71G

`8LGBX71G` is a valid latency/correctness pass for the 1000-bid S1 workload, but
it is not proof of a 500 ms arrival window because it used the previous 1000 ms
release-window default and observed about 1.2 s spans.

The generated review is:

```text
docs/perf/pts/evidence/incoming/8LGBX71G/s1-review.md
```

Key facts from the recomputation:

- 1000 PTS sampling rows, 1000 unique `client_bid_id`, 1000 server POSTs, 1000
  Redis Lua executions.
- Sampling-log `elapsedTime` p99 = 17 ms, max = 22 ms.
- PTS API reports `Seg99Rt=34.99` while `MaxRt=22`, so that API p99 field is
  invalid for this run.
- 349 `ENGINE_ACCEPTED`, 651 `ENGINE_REJECTED`; all `DECIDED` and
  `ENGINE_DURABLE`.
- Correctness gates PASS, including winner, reject justification,
  `engine_seq`, settlement, Kafka lag, Redis pending, DLQ, and outbox drain.

## References

- Alibaba Cloud PTS test metrics: RT is elapsed client response time; percentile
  response times should be used alongside averages:
  <https://www.alibabacloud.com/help/en/pts/performance-test-pts-3-0/product-overview/test-metrics>
- Alibaba Cloud PTS report docs: reports include business metrics, presser
  monitoring, request sampling logs, and exported report context:
  <https://www.alibabacloud.com/help/en/pts/performance-test-pts-3-0/user-guide/check-pts-3-report>
- Alibaba Cloud PTS sampling logs: sampling logs contain request/response
  details and timing breakdown; default sampling is 1% and retained for 30 days:
  <https://www.alibabacloud.com/help/en/pts/performance-test-pts-3-0/user-guide/view-sampling-log>
- Apache JMeter `SampleResult`: official API for sample time, start/end,
  latency, connect time, response fields, and ignored samples:
  <https://jmeter.apache.org/api/org/apache/jmeter/samplers/SampleResult.html>
- Prometheus histograms and quantiles: phi-quantile definition,
  `histogram_quantile()`, bucketed estimation, and quantile error:
  <https://prometheus.io/docs/practices/histograms>
