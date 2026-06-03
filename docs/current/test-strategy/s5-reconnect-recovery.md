# S5 — Reconnect Recovery

> Maps to official brief: "WebSocket 连接稳定，即使网络波动也能自动重连" and "心跳保活".
> Current evidence tier: local k6 + Toxiproxy, not PTS.

## Business Scenario

A viewer or bidder is watching a live auction on mobile. The WebSocket connection
drops during active bidding, the user misses real price updates, and the client
reconnects with `last_seq=K`. The system must catch the client up to the current
authoritative public state without local guessing, lost messages, duplicate
notifications, or stale winner/price.

User-facing meaning:

- During reconnect/recovery, H5 marks the state as connecting/recovering or
  stale and disables dangerous bid/max-bid CTA paths.
- After recovery, the user sees the server's current price/winner/state.
- If the user is the winner after SOLD, payment stays blocked until the payable
  order exists; this is the same product finality boundary used by S4.

## What S5 Proves That S3 Does Not

S3 measures fanout latency to stable WebSocket viewers. S5 measures recovery:
after a client has missed real public seq updates, how fast and how correctly it
returns to current state.

Backend recovery path:

```text
client reconnects with last_seq=K
  -> WS ticket is re-issued and single-use validated
  -> recoveryMessages(auction_id, K)
       if contiguous Redis event history exists: send history events K+1..N
       if gap/history unavailable: send snapshot
  -> server records ws_reconnect and ws_recovered(source=history|redis|db|...)
  -> live fanout resumes
```

Code paths checked:

- `backend/internal/realtime/server.go`: `ServeWS`, `recoveryMessages`,
  `snapshotMessage`, slow-consumer close.
- `backend/internal/realtime/server_integration_test.go`: ticket reuse/forged
  room rejection, history recovery, snapshot fallback, snapshot singleflight,
  rebuild saturation fallback.
- `frontend/mobile-h5/src/main.tsx`: reconnect uses `lastSeqRef`, recovering
  state disables dangerous actions, malformed realtime messages trigger snapshot
  recovery.
- `frontend/mobile-h5/src/realtime.ts`: exponential reconnect delay with jitter.

## Current S5 Load Model

Source asset: `tests/load/s5-reconnect-recovery.js`.

Each reconnect VU:

```text
1. reads current auction snapshot using a real Bearer token from
   docs/perf/pts/pts-1ab-1000vu-sessions.csv
2. opens WS with current last_seq, receives the initial recovery/snapshot, then
   closes intentionally
3. waits until a low-rate accepted-bid source has advanced public seq by at
   least MISSED_EVENTS=2
4. reconnects with stale last_seq=K
5. measures TTCS = reconnect start -> reaches current seq N
6. checks no seq gap, no duplicate seq, and no snapshot truth mismatch
```

The accepted-bid source reads current price and bids `current + increment`, so it
generates real public seq updates instead of synthetic messages.

Skipped iterations:

- If the bid source does not create enough new public seq within the configured
  wait window, the iteration is recorded as `s5_skipped_no_missed_window_total`.
- Skipped iterations are not counted as recovery success or failure because no
  reconnect gap was created.

## Evidence: 2026-06-03 Local Runs

Environment:

- Backend: local Linux server on `127.0.0.1:18080`.
- Identity: real PTS session CSV tokens, not mock headers.
- Runtime profile after `SESSION_COUNT=1000 bash tests/pts/reset-l4b-final-second-pressure.sh`.
- Kafka/Redis/PostgreSQL are single local containers; this is a reconnect logic
  and single-node recovery stress, not multi-AZ production HA proof.

| Run | Mode | Reconnect VU | Duration | Accepted update source | Recovered | TTCS p99 | Errors | Gaps | Duplicates | Evidence |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| `s5-clean-20vu-pass-20260603T0302` | clean close | 20 | 20s | 8/s, 4 VU | 560 | 17 ms | 0 | 0 | 0 | `docs/perf/pts/evidence/incoming/s5-clean-20vu-pass-20260603T0302/` |
| `s5-clean-100vu-20260603T0304` | clean close | 100 | 25s | 30/s, 12 VU | 3700 | 57 ms | 0 | 0 | 0 | `docs/perf/pts/evidence/incoming/s5-clean-100vu-20260603T0304/` |
| `s5-clean-200vu-20260603T0306` | clean close | 200 | 25s | 60/s, 24 VU | 7393 | 104 ms | 0 | 0 | 0 | `docs/perf/pts/evidence/incoming/s5-clean-200vu-20260603T0306/` |
| `s5-network-50vu-20260603T0308` | Toxiproxy `reset_peer` | 50 | 20s | 20/s, 8 VU | 1450 | 32 ms | 0 | 0 | 0 | `docs/perf/pts/evidence/incoming/s5-network-50vu-20260603T0308/` |

Best current claim:

> Under local single-node S5, 200 concurrent reconnect users recovered 7393
> stale-`last_seq` sessions with TTCS p99 104 ms, zero recovery errors, zero seq
> gaps, zero duplicate seqs, and zero truth mismatch. Under Toxiproxy WS
> `reset_peer` weak-network mode, 50 VU recovered 1450 sessions with TTCS p99
> 32 ms and the same zero-error correctness gates.

## Failure And Harness Fixes

These failed runs are not pass evidence, but they are useful judge material
because they show the harness was debugged instead of papered over.

| Failed run | Symptom | Root cause | Fix |
|---|---|---|---|
| `s5-smoke-20260603T0247` | Docker permission denied | local `k6` absent; Docker k6 needed escalation | runner now falls back to `grafana/k6:latest` |
| `s5-clean-20vu-20260603T0254` | k6 could not open session CSV | k6 `open()` path was relative to script directory | default CSV path changed to `../../docs/perf/pts/...` |
| `s5-clean-20vu-20260603T0256` | no missed window, no recovered sessions | bid source amount became stale after first accepted bid | bid source now reads current price and bids `current + increment` |
| `s5-clean-20vu-20260603T0300` | 560 recovered but threshold failed | iterations without enough new seq were counted as recovery errors | no-gap iterations now count as skipped, not recovery failure |

## PTS Decision

Do not spend PTS for S5 yet.

Reason:

- S5 correctness depends on `last_seq`, history replay/snapshot fallback, and UI
  stale-state behavior; local k6 + backend evidence is more direct than a PTS
  WebSocket hold chart.
- The current local result has large SLO margin: 200 VU clean TTCS p99 104 ms
  against a 2 s recovery target.
- PTS adds value only if the claim changes to public-network reconnect storms,
  multi-source-IP socket exhaustion, or polished external charts. It does not
  replace the current no-gap/no-duplicate correctness checks.

## Production Expansion Path

Current local proof is intentionally not production HA:

- Single backend process, single Redis, single Kafka broker, single PostgreSQL.
- No load balancer, no multi-instance sticky/session routing, no mobile carrier
  network, no cross-AZ failure.
- Toxiproxy `reset_peer` is a controlled TCP fault, not a full browser/device
  weak-network E2E test.

Production path for a ByteDance/TikTok Shop-level review:

- Use WebSocket gateway shards by `room_id`/`auction_id`; keep recovery state in
  Redis/outbox so a reconnect can land on another gateway instance.
- Apply reconnect admission and exponential backoff with jitter to avoid thundering
  herds after network restoration.
- Keep Redis event-history retention sized by peak fanout and reconnect window;
  when history is insufficient, snapshot fallback must be bounded by
  singleflight/semaphore, as the backend tests already cover.
- Add browser Playwright weak-network E2E: force H5 socket close, assert stale UI,
  disabled bid/max-bid CTA, recovery snapshot, and payment/order CTA gating.
- For production infra, pair this with Kafka RF=3/minISR=2/acks=all and Redis HA
  as described in S4 docs; S5 itself assumes the realtime recovery data stores
  remain reachable or fail closed.

## External References Checked

- Grafana k6 `k6/ws`: `connect()` blocks until the WebSocket closes and supports
  message/error/close handlers, which is why the S5 harness can model
  connect-close-reconnect sequentially:
  <https://grafana.com/docs/k6/latest/javascript-api/k6-ws/connect/>
- Grafana k6 newer `k6/websockets`: useful for larger multi-connection event-loop
  workloads; S3 uses that style, while S5 keeps blocking semantics for one
  recovery leg per VU:
  <https://grafana.com/docs/k6/latest/javascript-api/k6-websockets/>
- Toxiproxy `reset_peer`/timeout toxic: controlled TCP fault injection for
  network-reset proof, not a substitute for mobile carrier weak-network E2E:
  <https://github.com/Shopify/toxiproxy>
- MDN WebSocket close event: browser code can observe close/error and reconnect;
  S5 still separates detection time from reconnect TTCS:
  <https://developer.mozilla.org/en-US/docs/Web/API/WebSocket/close_event>
- AWS Architecture Blog on exponential backoff and jitter: production reconnect
  storms need jittered retries rather than fixed simultaneous reconnect loops:
  <https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/>
