# 16 · Industrial P2/P3+ Roadmap

> Date: 2026-05-23 Asia/Shanghai  
> Status: design update after P1 completion. This document extends `01-scope-and-roadmap.md`; it does not weaken the existing v2 non-negotiables.

## Why This Exists

P1 has completed the evidence and observability layer. The project is now past the "can demonstrate the core" stage, but it is not yet defensible as an industry-grade live-commerce auction platform.

The current weak points are real:

- `backend/internal/gateway/auth.go` still uses mock headers for identity.
- `backend/internal/realtime/server.go` validates that an auction belongs to a room, but not a real room membership relation.
- H5 and PC flows still assume a narrow seeded room path in several demos/load scripts.
- `backend/internal/auction/bid.go` has idempotent mock payment, but not a provider callback/webhook state machine.
- Bid rate-limit error codes exist in `05-api-contracts.md`, but the gateway does not enforce a Redis-backed limit.
- Runtime remains a single backend process with an in-process WebSocket hub.
- Performance evidence is honest local/Windows smoke plus P1 scripts, not final Linux 3-run capacity proof.

These are not P1 failures. They are the correct next work if the goal is a senior-reviewable, competition-grade system.

## Design Principle

The next stages should not chase "more infrastructure" for its own sake. A judge will reward systems that preserve the hard invariants under more realistic product, abuse, scale, and failure conditions.

The target posture:

```text
P2: remove demo shortcuts without changing the correctness core.
P3: prove multi-room and multi-instance realtime behavior with measured local gates first.
P4: add differentiating verification, replay, and risk-control tooling.
P5: freeze reproducible release evidence, including final Linux capacity calibration, and interview defense material.
```

PostgreSQL remains the auction truth. Redis, Centrifugo/NATS, WebSocket, metrics, and replay tooling are secondary systems around that truth.

## Public Research Baseline

Private TikTok Shop/Taobao/Whatnot/eBay backend details are not public. The usable evidence is a mix of public product behavior, mature open-source systems, official docs, and production engineering literature.

| Area | Public Evidence | Design Takeaway |
|---|---|---|
| Live commerce product surface | TikTok Shop LIVE Shopping documents product showcase and live product management before/during livestreams: https://seller-us.tiktok.com/university/essay?knowledge_id=6927759780628226 | PC/H5 must feel like live-commerce operations, not only an auction CRUD demo. Product set, focus item, room context, and order history matter. |
| Fast live auctions | Whatnot documents live auction bidding, seller auction creation, and many auctions lasting 30 seconds or less: https://help.whatnot.com/hc/en-us/articles/14932924544141-Bid-on-an-item-during-a-show and https://help.whatnot.com/hc/en-us/articles/9779931101837-Start-an-auction-during-a-show | Last-second contention, short timers, custom amounts, immediate feedback, and payment readiness are not edge cases; they are the main workload. |
| Marketplace bidding | eBay automatic bidding uses a maximum-bid/proxy model: https://www.ebay.com/help/buying/bidding/automatic-bidding?id=4014 | Proxy bidding is a credible P4 extension, but it changes domain semantics and should not be slipped into the current fixed-increment live auction without an ADR. |
| SQL serialization point | PostgreSQL documents MVCC and explicit row-level locks: https://www.postgresql.org/docs/current/mvcc-intro.html and https://www.postgresql.org/docs/current/explicit-locking.html | The current PG row lock design is defensible for single-auction money correctness. Do not replace it with Redis authority without reconciliation proof. |
| Realtime fanout | Centrifugo provides WebSocket pub/sub, channel history, missed-message recovery, and cluster fanout: https://centrifugal.dev/docs/getting-started/highlights and https://centrifugal.dev/docs/tutorial/recovery | If self hub fails connection/fanout gates, use Centrifugo as transport while keeping DB/outbox/snapshot truth in app code. |
| Outbox/CDC | Debezium documents the outbox event router pattern for capturing a dedicated outbox table: https://debezium.io/documentation/reference/stable/transformations/outbox-event-router.html | Polling outbox is acceptable until measured hot-table pain. CDC is P3/P4 only after the outbox schema and delivery semantics are stable. |
| Distributed rate limit | Redis documents centralized token-bucket rate limiting with Lua for distributed services: https://redis.io/docs/latest/develop/use-cases/rate-limiter | P2 should implement Redis token/GCRA limits for user/IP/auction, with idempotent replay before limit checks. |
| Payment reliability | Stripe recommends idempotency keys for retryable POSTs and webhooks for async event delivery: https://docs.stripe.com/plan-integration/get-started/server-side-integration and https://docs.stripe.com/webhooks | P2 payment should model provider order creation, signed callbacks, idempotent event processing, and reconciliation even if the provider is local mock. |
| Monitoring discipline | Google SRE monitoring emphasizes simple, actionable monitoring and diagnosis: https://sre.google/sre-book/monitoring-distributed-systems | Metrics must map to user-visible symptoms and drilldown. P1 dashboards are the base, not the final operational story. |
| Tail latency | "The Tail at Scale" emphasizes tail-tolerant techniques for responsive large systems: https://cacm.acm.org/research/the-tail-at-scale | The project should discuss p99/p999, backpressure, bounded queues, admission control, and degraded modes rather than average latency. |
| Correctness testing | Jepsen analyses focus on histories, linearizability/serializability, and failure behavior: https://jepsen.io/analyses/etcd-3.4.3 and https://jepsen.io/analyses/voltdb-6-3 | Add an auction history checker and invariant verifier. Make correctness claims checkable after chaos/load runs. |
| Overload control | AWS Builders Library discusses fairness, throttling, bounded work, backpressure, and operational instrumentation: https://aws.amazon.com/builders-library/fairness-in-multi-tenant-systems and https://aws.amazon.com/builders-library/instrumenting-distributed-systems-for-operational-visibility | P2/P3 should implement layered admission control, per-room isolation, and logs/metrics that explain why requests were shed. |

Open-source candidates to compare before adoption:

| Candidate | Fit | Why Not Immediately Adopt |
|---|---|---|
| Centrifugo | Browser realtime pub/sub, channel history, recovery, clustering. | It solves transport, not auction truth, idempotency, order creation, or snapshot correctness. |
| NATS / JetStream | Durable pub/sub, service messaging, edge/cloud messaging. | Adds message-broker operations and does not directly solve browser WebSocket auth/recovery UX. |
| Debezium | CDC and outbox routing from PostgreSQL to Kafka-compatible consumers. | Strong P3/P4 option only if polling outbox is measured as bottleneck. |
| Envoy/global ratelimit style services | Mature gateway-side throttling pattern. | Too heavy for current monolith; Redis-backed in-app limiter is enough for P2, with clear migration boundary. |
| Prometheus/Grafana/k6/Toxiproxy | Already aligned with P1 evidence discipline. | Need final baselines and better workload coverage before marketing numbers. |

Do not cite exact GitHub star counts in final materials unless they are rechecked on the release date.

## Current-State Score Attack

| `00-project-brief.md` Scoring Term | Current Proof | Attack From Senior Judge | Next Fix |
|---|---|---|---|
| 完整工程链路 | P0/P1 flow exists from item to bid/order/mock pay/history/diagnostics. | "Why is identity and room access still mock/header-driven?" | P2-01 real session/auth boundary and P2-02 room ACL. |
| 接口网关 | Schema/idempotency exists; mock auth exists. | "Where is actual auth, ACL, and rate limit?" | P2-01, P2-02, P2-04. |
| 数据治理 | DB constraints, events, idempotency, trace_id, outbox exist. | "Can you replay or audit a full contested auction after load?" | P4-01 auction flight recorder and P4-02 invariant verifier. |
| 毫秒级实时同步 | Self hub, history/snapshot, backpressure tests exist. | "Can two backend instances serve the same room without event loss or duplicate fanout?" | P3-01 Centrifugo adapter or shared fanout, P3-02 relay shard ownership. |
| 系统可用性 | Restart/outbox/recovery gates exist. | "What happens when rate-limit Redis fails, payment callback repeats, or room ACL changes mid-session?" | P2-04, P2-05, P2-06 degradation tests. |
| 性能 | P1 scripts and Windows local smoke exist. | "Where is local bottleneck evidence, and where is final Linux capacity proof?" | P2-07 Windows/local bottleneck harness, P5 final Linux baseline. |
| 独特思考 | Server-authoritative correctness + recoverable realtime is real. | "This is still a single-room mock-auth demo unless product/security scope is hardened." | P2 product/security hardening, P4 verifier/replay/risk tooling. |

## P2 · Product And Security Hardening

P2 goal: remove shortcuts that make the system look like a demo while preserving the current money/realtime correctness model.

### P2-01 Real Session Boundary

Implement a real local auth/session model suitable for demo and tests:

- `users` table remains identity source.
- Add `auth_sessions` table with hashed session token, role, expiry, revoked_at, created_ip/user_agent.
- Add login endpoints for host/user demo accounts, not OAuth.
- Replace `X-Mock-Role` and `X-Mock-User-Id` in normal runtime with secure cookie or bearer session.
- Keep mock headers only behind `APP_ENV=test` or explicit `ALLOW_MOCK_AUTH=true`.
- Logs must never include tokens.

Acceptance gates:

- host-only APIs reject user sessions.
- expired/revoked session rejects.
- tests prove mock headers are unavailable when disabled.
- H5/PC e2e use real sessions.

### P2-02 Room Membership And Host Ownership ACL

Implement room membership as a first-class relation:

```sql
room_memberships (
  room_id text not null references rooms(id),
  user_id text not null references users(id),
  role text not null check (role in ('host','viewer','blocked')),
  status text not null check (status in ('ACTIVE','LEFT','BANNED')),
  joined_at timestamptz not null,
  left_at timestamptz,
  primary key (room_id, user_id)
)
```

Rules:

- host APIs require `rooms.host_id = current_user`.
- viewer bid/chat/ws-ticket require active membership.
- blocked/banned users cannot bid, chat, or receive WS ticket.
- `POST /api/auth/ws-ticket` checks membership and auction-room relation.
- membership changes emit non-money audit event, not auction seq event unless UI must update current viewer state.

Acceptance gates:

- forged room and foreign auction tests cover REST and WS.
- banned user cannot bid with a previously issued ticket after server-side revocation window.
- PC cannot manage another host's room.
- diagnostics show ACL rejects with trace_id.

### P2-03 Remove Fixed Room Path

Make room context real:

- PC can list owned rooms and choose active room.
- H5 starts from `/rooms/{room_id}` and loads auction list from backend.
- k6 seed creates named rooms and passes room IDs through env vars.
- demo docs no longer rely on `room_main` except as sample seed data.

Acceptance gates:

- at least two rooms in e2e; each has distinct auctions and chat.
- cross-room event leak test passes.
- H5 reload into each room restores correct state.

### P2-04 Bid Admission Control And Abuse Behavior

Implement layered rate/admission control:

1. completed idempotency replay bypasses limiter.
2. per-user-per-auction limiter.
3. per-IP-per-auction limiter.
4. per-auction global limiter.
5. local in-process hot-auction semaphore before PG lock.

Use Redis token bucket or GCRA-style atomic script. Redis official docs support centralized token-bucket rate limiting with Lua; for this project, the limiter must be small, testable, and observable rather than generic middleware.

Failure posture:

- Redis down for bid rate limit: fail open for safety of legitimate buyers, keep local semaphore, emit `RATE_LIMIT_REDIS_DOWN`.
- Auction global overload: return `BID_AUCTION_TOO_HOT` with `Retry-After`.
- User/IP abuse: return `RATE_LIMITED`.
- Confirmed idempotent replay must still return the original result.

Acceptance gates:

- limiter correctness unit tests with fake clock.
- idempotent retry after first success is not rate limited.
- Redis down bid-limit gate passes.
- high reject/limit diagnostics appear in PC monitor.
- k6 abuse workload records accepted/rejected/limited distribution.

### P2-05 Payment Provider Boundary

Keep payment fake-provider local, but model a real provider lifecycle:

```text
ORDER_PENDING
  -> PAYMENT_INITIATED
  -> PAYMENT_SUCCEEDED
  -> PAID

ORDER_PENDING/PAYMENT_INITIATED
  -> ORDER_EXPIRED
```

Add:

- provider_payment_id unique index.
- `payment_events` table with provider event id, signature status, processed_at.
- signed webhook endpoint from local fake provider.
- idempotent webhook processing.
- reconciliation job: order pending but provider says success/fail/unknown.
- wrong-user payment and stale/historical order guards.

Acceptance gates:

- double callback creates one PAID transition.
- callback before client return works.
- expired order rejects late success or records manual-review anomaly according to ADR.
- provider signature failure is audited and ignored.

### P2-06 Security And Abuse Diagnostics

Add real producers and runbooks for:

- AUTH_SESSION_EXPIRED spike.
- ACL_FORBIDDEN spike.
- RATE_LIMIT_REDIS_DOWN.
- BID_AUCTION_TOO_HOT.
- PAYMENT_WEBHOOK_INVALID_SIGNATURE.
- PAYMENT_RECONCILE_MISMATCH.

Acceptance gates:

- each diagnostic has a producer test.
- PC monitor can filter by room/user/auction/trace_id.
- no static security cards.

### P2-07 Local Baseline And Bottleneck Harness

Build and run local Windows-friendly evidence for all existing workloads before deeper scale work:

- final-second bid burst.
- watcher fanout.
- reconnect storm.
- slow consumer.
- outbox burst.
- multi-room isolation after P2-03.

This is not the final marketing benchmark. It is a bottleneck discovery and correctness/attack baseline. Windows local numbers may be recorded honestly, but only as local smoke or relative comparisons.

Acceptance gates:

- scripts run locally or fail with actionable environment errors;
- raw outputs or evidence summaries are recorded;
- bid/outbox/realtime/recovery invariants are checked where applicable;
- PG hot-row, outbox, WS fanout, reconnect, slow-consumer, and multi-room workloads have an execution path;
- documentation points to `docs/perf/windows-local-strategy.md`;
- no final QPS/p99/fanout/online-user capacity claim is made from Windows local evidence.

Final Linux calibration is deferred to P5. Minimum machine for credible final capacity baseline:

- Linux native, not WSL2.
- 8 vCPU, 16 GB RAM, NVMe SSD.
- PostgreSQL/Redis/backend/k6 boundaries documented.
- `ulimit -n`, kernel, CPU model, Go/Postgres/Redis/k6 versions recorded.

For final competition baseline, prefer a quieter 16 vCPU / 32 GB Linux instance so DB, Redis, backend, and k6 do not fight for the same small laptop resources.

## P3 · Scale And Realtime Architecture

P3 goal: prove the architecture can move beyond one process and one hot room without weakening correctness.

### P3-01 Realtime Transport Decision

Decision options:

| Option | Pros | Cons | Recommended Use |
|---|---|---|---|
| Keep self hub | Full control, low dependency count, already implemented. | Multi-instance fanout and history become app-owned; high risk of subtle backpressure bugs. | Keep only if it passes strict multi-instance tests. |
| Centrifugo adapter | Browser WebSocket, channels, history/recovery, cluster fanout are product features. | Adds component and auth integration; still needs app-owned truth/snapshot. | Preferred P3 path if goal is credible industrial realtime. |
| Redis Pub/Sub between hubs | Simple to wire. | Pub/Sub is not durable; history/snapshot still app-owned; cluster behavior easy to under-test. | Only as a temporary bridge, not final claim. |
| NATS/JetStream | Strong messaging substrate, useful for services. | Does not directly provide browser WS semantics; more ops. | Consider later for internal eventing, not P3 browser fanout. |

Recommended P3 design:

```text
outbox relay -> publish auction event to Centrifugo channel
              -> update Redis snapshot/history owned by app
H5/PC client -> Centrifugo WebSocket
snapshot API -> app backend
```

The app still owns:

- auth/session and membership.
- channel token generation.
- auction seq and payload shape.
- snapshot fallback.
- gap semantics.
- diagnostics.

### P3-02 Relay Shard Ownership

When multiple backend instances run:

- outbox relay workers use DB leases per shard.
- only one active owner per shard.
- shard ownership changes are visible in diagnostics.
- head-of-line rule remains per auction.
- DEAD event still emits gap notice through the chosen realtime adapter.

Acceptance gates:

- kill relay owner during burst; another instance resumes without reorder.
- two instances do not publish same outbox event twice beyond client-dedupe tolerance.
- relay lag metrics are per shard and per room.

### P3-03 Multi-Room Isolation

Implement and test:

- independent hot/cold room workloads.
- per-room admission counters.
- per-room WS/fanout metrics.
- per-room snapshot rebuild semaphore or fair scheduling.
- no global lock around all room fanout.

Acceptance gates:

- one hot room does not leak events to another room.
- cold room bid p99 and fanout lag remain explainable under hot-room load.
- overload in one room returns room-scoped throttles, not whole-service collapse.

### P3-04 Data Path Evolution

Do not jump directly to CDC. Use measurements:

| Symptom | Required Evidence | Next Design |
|---|---|---|
| PG row lock dominates hot auction latency | local lock/tx/pool metrics first; final Linux confirmation before capacity claim | consider Redis Lua reservation ADR, but only with reconciliation. |
| outbox claim/update hot table dominates | outbox burst explain/analyze, table bloat, delivery lag | partition outbox table or Debezium CDC outbox. |
| snapshot rebuild DB pressure | reconnect storm metrics show semaphore saturation | precomputed snapshot versioning and room-level fair queue. |
| fanout CPU/memory dominates | WS pprof and fanout lag | Centrifugo or serialization-once-per-event optimization. |

Redis Lua reservation remains design-only unless an ADR proves:

- PG remains final settlement truth.
- every Redis reservation can be reconciled to bids/events.
- Redis loss cannot create a winner.
- cap/cancel/end races still resolve in one terminal DB state.

## P4 · Differentiators Beyond Standard Engineering

P4 goal: make the project harder to dismiss as "a well-tested CRUD + WebSocket app".

### P4-01 Auction Flight Recorder

Build a replayable timeline for one auction:

- rule creation and freeze.
- start/end/cancel/order/payment events.
- accepted/rejected bids with seq and reason.
- outbox delivery attempts and gaps.
- WS recovery events and snapshot source.
- frontend-visible state transitions.

UI: PC diagnostic detail page, not a marketing dashboard.

Value under review:

- answers "show me exactly what happened during the race".
- proves event sourcing is not decorative.
- supports post-load forensic analysis.

### P4-02 Correctness Invariant Verifier

After load/chaos tests, run a checker that verifies:

- auction seq continuous except documented DEAD gaps.
- one terminal state.
- at most one order per auction.
- order winner and amount match winning event.
- accepted bid count equals accepted bids.
- idempotency records match bid/payment responses.
- no cross-room auction/event leak.
- every terminal mutation has event and outbox.

Inspired by Jepsen-style history checking, but scoped to this domain. This is a strong differentiator because it converts "trust me" into machine-checkable evidence.

### P4-03 Risk-Control Simulator

Add scenario scripts for:

- bot user spamming low bids.
- self-leading retries.
- fat-finger large jump.
- reconnect storm while outbox has a DEAD gap.
- Redis outage during bid limit.
- payment callback duplication and late callback.

Each scenario should output:

- expected business outcome.
- DB invariant result.
- user-facing UI state.
- anomaly and metrics emitted.

### P4-04 Optional Proxy-Bid ADR

Do not implement this unless the core release gates are already strong.

Proxy bidding can be an advanced feature inspired by eBay, but it changes the auction model:

- users submit max willingness.
- server computes displayed price.
- bid privacy and tie-break rules become core.
- fairness questions get harder.

If implemented, it needs a separate state machine and cannot reuse the current fixed-increment bid path casually.

## P5 · Final Release And Defense

P5 goal: turn the system into a defensible submission artifact.

Required release evidence:

- all P0/P1/P2/P3 gates with command, env, raw output, commit.
- Linux native 3-run report for every workload.
- failed or weak benchmarks included honestly with bottleneck.
- pprof bundle for at least one bottleneck investigation.
- threat model summary for auth/ACL/payment/rate-limit.
- known-limits doc updated after every scope decision.
- demo flow that does not rely on hidden seed assumptions.

Submission claims must be phrased as:

- implemented and tested;
- implemented but only locally measured;
- designed and documented;
- intentionally not implemented.

Do not use:

- "supports N users" without final baseline.
- "exactly-once WebSocket".
- "production payment".
- "TikTok-level scale".
- "Redis solves correctness".

## Self-Critical Debate

### Should P2 Start With Multi-Instance Realtime?

No. A senior reviewer can reject the project before scale if auth, membership, room selection, rate limit, and payment callback are still mock/demo. Multi-instance realtime is impressive only after product/security boundaries are credible.

### Should Redis Lua Replace PG Row Lock For Speed?

No, not yet. The strongest current technical story is a simple, auditable serialization point for money. Redis Lua can reduce latency under one workload, but it risks split-brain authority. It belongs behind measured PG bottleneck evidence and a reconciliation ADR.

### Should The Project Adopt Centrifugo Immediately?

Not before P2 hardening. Centrifugo is a strong P3 candidate because it addresses known transport concerns, but adopting it before ACL/session/channel token design would create another mock integration. The adapter boundary should be designed in P2 and implemented in P3.

### Should Real OAuth/Real Payment/Real Live Streaming Be Added?

Not as the next step. They consume time and add external dependencies while not proving the two core challenges. The right move is a real local session boundary, a fake provider with real webhook semantics, and a simulated live surface that makes auction state rigorous.

### Is This Enough To Beat Elite Projects?

Not yet. After P1, the project has a credible core, but it can still be attacked as single-process, mock-auth, mock-payment, and locally benchmarked. After P2 + P3 + P4, the defensible pitch becomes:

```text
This is a server-authoritative live auction platform with tested money correctness,
recoverable realtime, real room ACL, abuse controls, payment callback semantics,
multi-room/multi-instance fanout evidence, and domain-specific invariant verification.
```

That is the level at which senior experts have to argue with concrete tradeoffs instead of dismissing it as a demo.

## Execution Order

| Order | Milestone | Why First |
|---:|---|---|
| 1 | P2-01 real session boundary | All ACL/rate/payment tests need real user identity. |
| 2 | P2-02 room membership ACL | Removes the biggest demo shortcut and protects WS/bid/chat. |
| 3 | P2-03 remove fixed room path | Makes multi-room load and UI credible. |
| 4 | P2-04 bid admission control | Protects the hot path and answers abuse questions. |
| 5 | P2-05 payment provider boundary | Makes order/payment lifecycle reviewable. |
| 6 | P2-06 diagnostics | Keeps new failure modes visible. |
| 7 | P2-07 local baseline and bottleneck harness | Finds local correctness, script, and bottleneck-direction issues before scale work. |
| 8 | P3-01/P3-02 realtime adapter + relay leases | Enables multi-instance claim. |
| 9 | P3-03 multi-room isolation | Proves scale is not one hot-room benchmark. |
| 10 | P4 verifier/flight recorder/risk simulator | Converts correctness into judge-facing evidence. |
| 11 | P5 release freeze | Locks claims to raw evidence. |

## New Progress Trackers To Create

- `docs/p2-progress.md`
- `docs/p3-progress.md`
- `docs/evidence/p2-*`
- `docs/evidence/p3-*`
- ADRs under `docs/adr/` for:
  - session/auth boundary.
  - room membership ACL.
  - rate limiter degradation.
  - payment callback state machine.
  - Centrifugo vs self hub.
  - CDC outbox decision.
  - Redis Lua reservation, only if benchmark evidence justifies it.

## Engineering Gate

ENGINEERING GATE: PASS WITH BLOCKERS

Blockers before "industry-grade" claim:

- [P2] Auth remains mock-header based in normal runtime.
- [P2] Room membership ACL is not a first-class model.
- [P2] Bid rate-limit codes exist but no limiter enforces them.
- [P2] Payment is a direct mock endpoint, not callback/idempotent provider flow.
- [P3] Realtime fanout is single-process.
- [P5] Final Linux 3-run capacity evidence is not complete.

Decision:

```text
ACCEPT this roadmap as the next design baseline.
Do not claim final industrial competitiveness until P2/P3 evidence exists.
```
