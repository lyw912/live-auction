# Current PTS Run Review Template

> Status: required template for new PTS-1A/PTS-1B run reviews, 2026-05-31.

Use this template for every new PTS report review. Do not publish a free-form
"pass" report that omits runtime profile, `ENGINE_*`, settlement, verifier, or
fault-gate fields.

## Header

```text
# PTS Report <REPORT_ID> Review

Date:
Reviewer:
Git SHA:
Dirty tree:
Runtime profile/env source:
Workload:
JMX:
CSV:
Reset label:
Preflight label:
Server evidence path:
Verifier command:
Final classification:
```

Allowed classifications:

- `CURRENT_PASS`
- `CURRENT_FAILING`
- `CURRENT_ADJACENT`
- `HISTORICAL`
- `HARNESS_ONLY`
- `RAW_ARTIFACT`

## Verdict

```text
PERFORMANCE/CORRECTNESS VERDICT:
```

State exactly whether the report proves current PTS-1B success. If not, say
which gate failed.

## Load Model

| Field | Value |
|---|---|
| PTS report id |  |
| PTS report window |  |
| Agents |  |
| VUs |  |
| Intended unique bids |  |
| Actual unique bids |  |
| Workload type | PTS-1B contention / PTS-1A accepted ladder / smoke / other |
| Runtime profile |  |
| Admission | enabled / disabled |

## PTS And HTTP Metrics

| Metric | Value | Source |
|---|---:|---|
| POST sampler count |  |  |
| HTTP 2xx / 4xx / 5xx |  |  |
| HTTP accept p50 / p95 / p99 |  |  |
| Final `ENGINE_*` p50 / p95 / p99 |  |  |
| `202` / pending ratio |  |  |
| final-decision timeout ratio |  |  |
| Sample-log row count |  |  |
| Dropped/failed PTS iterations |  |  |

Explain any mismatch between PTS sampler count and server-side unique bids.
Sampling logs are diagnostic samples unless configured to 100%.

HTTP `202` RTT is acceptance/enqueue latency only. It must not be reported as
user-visible final bid-decision p99. If the sampler stops at `202`, classify the
latency evidence as ingress-only and do not mark the run `CURRENT_PASS`.

## Engine Decision Distribution

| Business result | Count |
|---|---:|
| `ENGINE_ACCEPTED` |  |
| `ENGINE_REJECTED` |  |
| `ENGINE_SOLD` |  |
| `ENGINE_PAUSED` |  |
| `RECONCILING` |  |
| `PROCESSING_RETRY_LATER` |  |
| other / unknown |  |

User-visible engine decision p99:

```text
<value and source>
```

## Correctness Gates

| Gate | Result | Evidence |
|---|---|---|
| 1000 intended unique bids classified |  |  |
| final highest valid amount is winner |  |  |
| every low reject justified at decision time |  |  |
| idempotency duplicate/concurrent behavior |  |  |
| Kafka/engine_seq continuity |  |  |
| PostgreSQL settlement complete |  |  |
| public auction seq contiguous |  |  |
| no DLQ / pending / pause left unresolved |  |  |
| verifier exit code |  |  |

## Durability And Settlement

| Layer | Status | Evidence |
|---|---|---|
| Redis hot state |  |  |
| Kafka topic/append/fence |  |  |
| PostgreSQL bids/settlements |  |  |
| Outbox / realtime projection |  |  |
| Reconciler |  |  |

## Fault Injection

| Fault | Run? | Result | Evidence |
|---|---|---|---|
| Redis restart / data loss | yes/no |  |  |
| Kafka timeout / broker restart | yes/no |  |  |
| PostgreSQL latency / restart | yes/no |  |  |
| settlement worker crash/restart | yes/no |  |  |
| WebSocket reconnect storm | yes/no |  |  |

If fault injection was not run, the report cannot claim fault readiness.

## Bottleneck Attribution

| Candidate | Evidence | Verdict |
|---|---|---|
| gateway auth/ACL/idempotency |  |  |
| Redis Lua / Redis latency |  |  |
| Kafka append/fence |  |  |
| PostgreSQL settlement |  |  |
| outbox/realtime |  |  |
| load generator / PTS harness |  |  |

## Required Next Action

- `[P0/P1/P2]` concrete fix, test, or runbook action.

## Citation Rule

End with one of these exact statements:

```text
This report is CURRENT_PASS evidence for current PTS-1B.
```

or

```text
This report is not CURRENT_PASS evidence for current PTS-1B because <specific failed gate>.
```
