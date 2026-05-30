# PTS Report C33WX7MG Review

Date: 2026-05-30

Verdict: `FAIL_CLOSED_ENGINE_PAUSE`

`C33WX7MG` cannot be used as performance evidence. It exposed a real L4B
operational failure mode: the Redis ledger engine paused the auction during the
run.

PTS overview:

- report id: `C33WX7MG`;
- time: `2026-05-30 02:13:37` to `2026-05-30 02:14:37`;
- `AgentCount=2`;
- `Vum=880`;
- `POST PTS-1 hotspot bid` samples reported by PTS: `641`;
- PTS failed samples: `238`;
- PTS success rate: about `62.87%`;
- PTS p99: about `35.44ms`.

Server-side business truth:

- DB unique bids: `403`;
- accepted: `275`;
- rejected: `128`;
- all business rejects were `BID_TOO_LOW`;
- `auc_live.current_price_cents=2040000`;
- `auc_live.accepted_bid_count=275`;
- `auc_live.seq=275`;
- `auc_live.engine_seq=403`;
- `auc_live.engine_paused=true`;
- `auc_live.engine_pause_reason=REDIS_ENGINE_PENDING_KAFKA_APPEND_UNKNOWN`;
- outbox/auction_events: `275`;
- later PTS failures were HTTP `409 Conflict` from the paused engine.

Root cause:

The engine paused at `2026-05-29 18:14:29.803957+00` with anomaly payload:

```json
{"pending_decisions":40,"recovered_pending":0,"trace_id":""}
```

The settlement loop was processing Kafka appends in batches, but the periodic
reconcile loop observed Redis pending decisions before the settlement loop had
finished draining them. The current reconcile policy treats any non-zero pending
hash as `REDIS_ENGINE_PENDING_KAFKA_APPEND_UNKNOWN` and fail-closes the auction.
That is safe for correctness but too aggressive under burst pressure: it can
pause a healthy auction while settlement is simply behind by a short interval.

Code and gate fixes:

- Reconcile now treats a live `bid:{auction}:engine:pending:append-lock` as
  in-flight work, not data loss. It returns
  `REDIS_PENDING_APPEND_IN_PROGRESS` without pausing the auction.
- Reconcile still drains Redis pending decisions synchronously and still
  fail-closes with `REDIS_ENGINE_PENDING_KAFKA_APPEND_UNKNOWN` if pending work
  remains after the drain and no append worker owns the lock.
- Kafka writer retry budget was raised while preserving synchronous
  `acks=all`, so short broker hiccups are retried before Redis pending is
  classified as unrecoverable.
- `verify-l4b-pts-correctness.sh` now fails when `auctions.engine_paused=true`;
  before this report it only checked settlement/outbox/Redis pending
  convergence, which produced a false PASS for the first C33 verification.

- `P0 engine_not_paused`

Re-running the gate against the C33 state correctly fails on that P0 gate. The
next PTS run must still prove the code fix under pressure; this report remains a
failed run.

Evidence:

- PTS export: `docs/perf/pts/evidence/archive/current-adjacent/C33WX7MG/`
- after snapshot: `docs/perf/pts/evidence/archive/current-adjacent/after-C33WX7MG-l4b-accepted-ladder-10ms-1000vu/`
- fixed-gate proof: `docs/perf/pts/evidence/archive/current-adjacent/after-C33WX7MG-l4b-accepted-ladder-10ms-1000vu-gate-fixed/`
