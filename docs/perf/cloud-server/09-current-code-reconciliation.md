# 09 · Cloud PTS Findings Reconciled With Current Code

Date: 2026-05-28

This note reconciles the cloud PTS commit findings with the later local
implementation work that was merged after the cloud run.

## Source Finding

Cloud report `3IVNW7TF` found a real pressure signal, but not a production
capacity number:

- HTTP request path stayed stable for the measured run.
- Most bid requests were business rejections, not accepted bids.
- DB full counts were about 298k rejected bids and 18k accepted bids.
- Outbox delivery backlog exceeded 304k pending rows.
- Redis publish pipeline latency was low, so Redis publish was not the primary
  bottleneck.
- The run found app-owned outbox relay / realtime delivery pressure. It did not
  prove accepted-bid capacity or WebSocket fanout capacity.

Do not use the reported 643 TPS as accepted-bid TPS.

## Still Required

### P0-3 Rejected Event Strategy

Current release work now separates ordinary rejected bids from full-room durable
realtime:

- `bid_accepted`, `auction_sold`, terminal, order, and payment events remain
  durable realtime through `auction_events` and `outbox`.
- Ordinary non-state rejects such as `BID_TOO_LOW`, `AUCTION_ENDED`, and
  `AUCTION_NOT_ACTIVE` are returned to the caller over HTTP and kept in `bids`
  for idempotency, audit, monitor rejects, and flight recorder timeline.
- Policy rejects that may be useful to full-room diagnostics still use durable
  realtime.

This must be validated with local tests and a fresh cloud PTS run before any
capacity claim.

### P0-6 PTS Sampling Analyzer

`tests/pts/fetch-pts-sampling-logs.sh` downloads sampling logs, but a committed
analyzer still needs to generate `analysis.md` with sampler percentiles, HTTP
codes, business result/reject reason, and DB full-count comparison.

### P0-5 During Evidence

Formal cloud runs still need before/during/after server evidence and timeline
alignment with PTS p99 spikes. Local P3 harnesses have during sampling, but
cloud PTS runbooks must preserve it for ECS evidence.

### P0-1 Accepted/Rejection Profile Proof

The seed and pressure scripts are improved, but current merged code still needs
fresh cloud evidence for separated accepted-pressure, rejected-pressure,
downstream-pressure, and admission-on profiles.

## Already Covered Or Parked

- P0-2 outbox relay batching: covered by P3 claim/index/shard lease/batch
  watermark work. Do not redesign again without fresh cloud evidence.
- P0-4 PostgreSQL hot-row attribution: covered by P3 hot-row drilldown and
  conservative seq-elision optimization.
- P1-1 Centrifugo: parked. P3 evidence keeps the self-hub until self-hub is
  proven to be the bottleneck after bid/outbox pressure is controlled.

## Next Cloud Run

The next ECS/PTS run should report:

- sampled and DB-full accepted/rejected distributions;
- outbox produced/s, published/s, final pending, oldest pending age;
- bid lock wait, bid latency, DB pool wait, Redis latency;
- before/during/after CPU, memory, IO, network, and process stats;
- admission counters and whether the profile was admission-on or admission-off.
