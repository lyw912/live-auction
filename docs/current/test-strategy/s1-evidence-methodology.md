# S1 Evidence Methodology

> Status: current, 2026-06-03. This document explains how S1 final-second PTS
> evidence is recomputed and defended when reviewers do not accept a PTS page at
> face value.

## Claim Boundary

S1 proves one narrow claim:

> In one hot auction, 1000 distinct authenticated bidders submit one bid each in
> the final-second burst window. The system returns synchronous `ENGINE_*`
> decisions with p99 <= 50 ms, classifies all 1000 bids exactly once, picks the
> highest valid winner, justifies every reject, and drains Redis/Kafka/PG/outbox
> state.

The current JMX default is `contention_release_window_ms=500`. That is a target
release window. The actual delivered span is not asserted from configuration; it
is recomputed after the run from:

- PTS sampling `startTimeTS`: load-generator send-start span.
- Response `server_time_ms`: server decision timestamp span.
- Server Prometheus counters: independent request and Redis engine counts.

If a run with a 500 ms target observes a wider span, the result may still prove
correctness and latency, but it does not prove "arrived within 500 ms". The
review must say that explicitly.

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

## Current Example: 8LGBX71G

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
