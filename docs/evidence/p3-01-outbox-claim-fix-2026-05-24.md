# P3-01 Outbox Claim Fix And Retest

Date: 2026-05-24 Asia/Shanghai

Status: `NO_REGRESSION_WITH_CEILING`

Raw output root: `docs/perf/raw/p3-outbox-claim-fix-20260524-0448/`

This is Windows local downstream-pressure evidence. It is valid for bottleneck direction and before/after comparison only. It is not a final capacity claim.

## Root Cause

The first P3 stress-attacker run found the relay claim query was not viable under backlog:

- old raw bundle: `docs/perf/raw/p3-attack-20260524-035952/`
- old claim plan: `1584.153ms`
- old plan symptom: `Rows Removed by Join Filter: 31305491`
- old backlog after 30s: `PENDING=8142`, `PUBLISHED=5789`

The query enforced per-auction ordering by anti-joining each ready candidate against all older unfinished rows. With thousands of pending rows in one hot auction, that became effectively quadratic.

## Fix

- Added denormalized relay fields to `outbox_delivery`: `auction_id`, `auction_seq`, `event_created_at`.
- Added a trigger so new delivery rows copy those fields from `outbox_events`.
- Added partial indexes for unfinished and ready delivery rows.
- Rewrote relay claim to use indexed delivery rows and an indexed same-auction predecessor check.
- Changed `Relay.Run` to drain a small batch before sleeping, while keeping per-event claim/publish/mark semantics.
- Added integration gates:
  - blocked auction head cannot be skipped;
  - `ProcessBatch` drains multiple events;
  - claim/process remains bounded with 5000 pending rows.

## Retest Profile

Backend downstream-pressure settings:

```text
HTTP_ADDR=127.0.0.1:18080
ALLOW_MOCK_AUTH=true
BID_USER_LIMIT_PER_SECOND=100000
BID_IP_LIMIT_PER_SECOND=100000
BID_AUCTION_LIMIT_PER_SECOND=100000
BID_AUCTION_MAX_IN_FLIGHT=512
```

k6:

```text
RATE=300
DURATION=45s
PRE_ALLOCATED_VUS=320
MAX_VUS=800
k6 run --summary-export docs/perf/raw/p3-outbox-claim-fix-20260524-0448/bid-pressure-300rps.json tests/load/tmp-p3-bid-pressure.js
```

The durable script is now `tests/load/p3-bid-pressure.js`; the raw run used the temporary predecessor with the same workload shape.

## Before / After

| Metric | Before P3-00 R3 | After fix 0448 | Interpretation |
|---|---:|---:|---|
| Offered profile | 300 rps / 45s | 300 rps / 45s | same local pressure shape |
| Completed iterations | 8393 | 8264 | still PG/open-model limited |
| Dropped iterations | 5108 | 5236 | unchanged bottleneck at bid hot row/load generator ceiling |
| Accepted bids | 3244 | 3087 | same order of magnitude |
| p99 HTTP | 5.25s | 5.23s | bid path still dominated by hot auction row pressure |
| Claim query time | 1584.153ms | 14.165ms | claim complexity fixed |
| Pending after 30s | 8142 | 633 | relay drain improved materially |
| Published after 30s | 5789 | 7593 | relay caught up after the pressure window |

Later snapshot:

- `docs/perf/raw/p3-outbox-claim-fix-20260524-0448/outbox-status-later.txt` shows `auc_live` fully drained to `PUBLISHED`.

## Verdict

Primary P3-00 bottleneck fixed: the relay claim query no longer shows the O(pending squared) hash anti-join behavior.

Remaining bottleneck: the single hot auction bid path still hits PG row-lock/open-model saturation at this local 300 rps pressure profile. That is expected from the server-authoritative design and is not a reason to move auction truth to Redis without a separate ADR.

Next P3 work should continue in this order:

1. P3-02 relay shard ownership and owner failover, because relay is now fast enough locally to justify testing multi-instance ownership semantics.
2. P3-03 hot/cold multi-room isolation with per-room metrics.
3. P3-04 data-path decision only if repeated evidence shows PG hot-row or relay ownership remains the limiting factor after shard/failover work.

## Validation

```text
go test ./internal/outbox -run TestRelay -count=1 -v
go test -p 1 ./...
pnpm exec node tests/load/validate-k6-suite.mjs
```
