# P3-08 Redis Lua Borrowing Review

Date: 2026-05-25 Asia/Shanghai

Status: `REVIEWED_BORROW_EXISTING_LUA_DO_NOT_REBUILD_MONEY_PATH`

Scope:

- Compare the current live-auction bid architecture against Redis Lua scripting and the local Redis source under `references/oss/sources/redis`.
- Decide whether Redis Lua should be reused, reimplemented in part, or used as the basis of a full bid-path rewrite.
- Map the decision back to `docs/design-v2-industrial/00-project-brief.md`: official feature scope, two core challenges, and scoring terms.

## Judge Verdict

`PASS WITH DAMAGE`

Do not rebuild P3 around Redis Lua reservation now.

The project already uses Redis Lua in the right places:

```text
gateway bid admission -> Redis Lua GCRA per user/IP/auction
WS ticket connect     -> Redis Lua one-time GET+DEL consume
```

Those are good borrowing points because they are short, bounded, non-authoritative, and easy to fail open or fail closed by business role. They borrow Redis's strongest property: a small read-decide-write sequence is atomic inside Redis.

The current money path must stay PostgreSQL-authoritative unless future evidence proves otherwise:

```text
bid command -> completed idempotency replay -> PostgreSQL transaction
            -> idempotency PROCESSING row -> FOR UPDATE auction row
            -> rule/state/cap/extension/fat-finger evaluation
            -> bid/order/auction_event/outbox/idempotency commit
            -> relay -> Redis projection/history -> WS recovery
```

The defensible P3 position is:

```text
I studied Redis Lua as a mature atomic scripting mechanism. I did not blindly
move auction truth into Redis. I borrowed Lua where Redis is actually the right
authority: gateway admission counters and one-time WebSocket tickets. Price,
winner, cap, cancel/end race, order, idempotency result, auction seq, and audit
remain in PostgreSQL. A Redis Lua reservation path is documented as an
evidence-gated prototype only, because it needs reconciliation and invariant
proof before it can improve the official project goals.
```

## Redis Lua Proof Checked

External official docs:

| Redis Area | Source | What It Proves |
|---|---|---|
| Rate limiting | Redis rate limiter docs: https://redis.io/docs/latest/develop/use-cases/rate-limiter/ | Redis is a good fit for centralized gateway quotas; Lua `EVAL` keeps the read-decide-update cycle atomic for token/counter state. |
| Lua scripting atomicity | Redis EVAL docs: https://redis.io/docs/latest/commands/eval/ and eval intro docs | Scripts run inside Redis without interleaving from other clients, but that atomicity is scoped to Redis, not to PostgreSQL, browser recovery, payment/order, or audit. |

Local Redis source anchors:

| Redis Source Area | Source Anchor | What It Proves |
|---|---|---|
| Cluster slot discipline | `references/oss/sources/redis/src/script.c:514`, `:543`, `:567` | Redis scripts that touch multiple keys must respect cluster slot boundaries unless explicitly allowed. A future reservation script needs hash-tagged per-auction keys. |
| BUSY behavior | `references/oss/sources/redis/src/server.c:2203` | Long scripts can block Redis and produce BUSY errors. Reservation scripts must stay small and observable. |
| Script kill/timeout path | `references/oss/sources/redis/src/script.c:124`, `:157`, `:326`, `:360`; `references/oss/sources/redis/src/script_lua.c:1605` | Lua timeout/kill behavior is an operational failure mode, not just an algorithmic concern. |
| `redis.call` execution bridge | `references/oss/sources/redis/src/script_lua.c:1018` | Lua scripts execute Redis commands through `redis.call`; correctness is only as broad as the keys and Redis state they mutate. |
| Cluster scripting tests | `references/oss/sources/redis/tests/unit/cluster/scripting.tcl:1` | Redis tests explicitly cover same-slot and cross-slot script behavior. |

## Current Project Proof Checked

| Current Area | Source Anchor | Verdict |
|---|---|---|
| Gateway Redis Lua GCRA admission | `backend/internal/gateway/bid_admission.go:158`, `:191`, `:210` | Good. The project already borrowed Redis Lua for distributed request-path protection. It checks completed idempotency replay before rate limiting, then uses Lua to atomically update per-key theoretical-arrival-time state. The script is now named `bid_admission_gcra`, run through `redis.Script.Run` (`EVALSHA` with `EVAL` fallback), and emits `redis_lua_script_*` metrics. |
| Redis down behavior for admission | `backend/internal/gateway/bid_admission.go:193`, `:262` | Correct boundary. Redis limiter failure records `RATE_LIMIT_REDIS_DOWN` and fails open so legitimate bids can still reach PostgreSQL. Abuse protection degrades; auction correctness does not. |
| Admission tests | `backend/internal/gateway/bid_admission_integration_test.go:24`, `:70`, `:94`, `:124`, `:162` | The limiter has tests for completed replay bypass, Redis-down fail-open anomaly, user limit, local hot auction, and admission-disabled bypass. |
| WS one-time ticket consume | `backend/internal/realtime/ticket.go:44` | Good. Lua atomically consumes one ephemeral ticket with GET+DEL. This is a safe Redis authority because a ticket is not money truth. The script is named `ws_ticket_consume` and reports consumed/missing/error metrics. |
| Ticket tests | `backend/internal/realtime/ticket_test.go:13`, `:56` | One-time consume and Redis-down behavior are tested. |
| Bid truth path | `backend/internal/auction/bid.go:125`, `:196`, `:648`, `:678` | Strong. Place/confirm bid lock the auction row and evaluate auction rules in the transaction. |
| Atomic event/outbox append | `backend/internal/auction/repository.go:481` | Strong. Auction seq/version, `auction_events`, `outbox_events`, and `outbox_delivery` are appended in the same PostgreSQL transaction. |
| Redis/DB projection reconciliation | `backend/internal/reconcile/checker.go:26`, `:428` | Good for current projection use. It is not yet a Redis reservation reconciler; that would be a new requirement. |
| P3 governing decision | `docs/archive/progress/p3-decision-log.md` P3-D14 | Redis remains projection/cache/admission support; Lua reservation is evidence-gated. |

## Redis Lua Versus Current Architecture

| Question | Direct Redis Lua Reservation | Current PostgreSQL Truth Path |
|---|---|---|
| Atomicity scope | Atomic inside Redis for selected keys only. Does not automatically cover DB rows, order creation, outbox, scheduler, or browser recovery. | One PostgreSQL transaction covers idempotency, auction row, bid/order, auction event, and outbox delivery row. |
| Auction rule authority | Must duplicate rule/state/cap/extension logic in Lua or call back to DB later. Duplication risks drift. | Go domain code owns rule validation and state machine; tests exercise the same path as production. |
| Cancel/end/cap race | Needs a cross-system settlement model. Redis may reserve after a cancel/end unless guarded by synchronized state and reconciliation. | Cancel/start/end/bid all serialize through PostgreSQL row locks and terminal state rules. |
| Idempotency | Redis can dedupe a reservation key, but DB still needs final idempotency result and replay payload. | Idempotency row is part of the same transaction and stores the replayable response. |
| Audit | Redis state is ephemeral unless persisted and reconciled. TTL/eviction/loss must be explained. | `bids`, `auction_events`, `outbox_events`, `orders`, and idempotency records form an audit trail. |
| Realtime recovery | Lua does not create browser recovery semantics. | Outbox, Redis history/snapshot, seq gap detection, and snapshot fallback are already aligned. |
| Performance story | Potentially reduces PostgreSQL hot-row pressure if DB lock is proven release-blocking. No current small test can prove this. | Slower under a single hot auction, but simple and defensible. Current evidence requires P3-R4 attribution before changing it. |
| Failure mode | Redis timeout/BUSY/cluster slot/failover/eviction can affect admission/reservation. | DB lock timeout is visible; correctness remains centralized. Redis down affects projection/admission, not winner truth. |

## Borrowing Decision Matrix

| Redis Lua Mechanism | Direct Reuse? | Borrow/Reimplement? | Project Landing | Priority |
|---|---:|---:|---|---|
| One-key atomic read-decide-update | Already | Yes | GCRA admission in `bid_admission.go`. | Implemented |
| Atomic one-time consume | Already | Yes | WS ticket `GET` + `DEL` in `ticket.go`. | Implemented |
| Distributed token bucket/GCRA | Already | Yes | Per user/IP/auction admission gates; idempotency replay is checked before limiter. | Implemented |
| Fail-open limiter boundary | Already | Yes | `RATE_LIMIT_REDIS_DOWN` anomaly and PostgreSQL correctness fallback. | Implemented |
| Fail-closed auth/ticket boundary | Already | Yes | Redis ticket consume failure blocks unsafe WS connection. | Implemented |
| Script observability | No | Yes | Add script outcome/latency/error-class metrics only if P3 diagnostics need more Redis attribution. | Low-risk P3/P4 |
| Script preload / `EVALSHA` | No | Maybe | Current scripts are small. Preload only if profiling shows script transfer/cache overhead or operational clarity matters. | Defer |
| Combined multi-key limiter | No | Maybe | A single Lua check for user+IP+auction would need same-slot key design or a non-clustered assumption. Existing three single-key scripts are acceptable because admission is protective, not money truth. | Defer |
| Redis Cluster hash-tag key taxonomy | No | Yes for future | Future multi-key scripts must use keys like `bid:{auction_id}:limit:user:{user_id}` and document slot ownership. | P3/P4 doc |
| Redis Lua reservation | No | Prototype only after evidence | Can only be a pre-admission/reservation cache before DB settlement; never final winner/price/order truth. | Evidence-gated |
| Full Redis money-path rewrite | No | No | Rejected for P3. It weakens auditability and shifts the hardest correctness questions into cross-system reconciliation. | No-go |
| Redis Streams for auction truth | No | No | Streams can be projection/internal delivery only; PostgreSQL outbox remains commit truth. | Evidence-gated separately |

## Full Rebuild Assessment

A full rebuild around Redis Lua is not justified for P3.

Reasons:

1. Redis Lua solves atomic mutation inside Redis. The official project is judged on `rule validation`, `row lock`, `state machine`, `outbox`, `scheduler`, `recoverable realtime`, and failure gates.
2. The current bid path has one auditable serialization point. A Lua rewrite creates at least two authorities unless PostgreSQL settlement remains final.
3. Auction correctness is not just "highest price wins". It includes fixed increment, reachable cap, auto extension, fat-finger confirm, cancel/end/cap races, one order, idempotency replay, and seq continuity.
4. Redis scripts have operational constraints: BUSY/timeouts, same-slot cluster rules, script cache/preload behavior, failover, restart, TTL, eviction, and observability.
5. Current P3 evidence does not yet prove the PostgreSQL hot-row is a release-blocking bottleneck after clean admission-off attribution.
6. Rebuild would invalidate P0/P1/P2 proof bundles and require a new crash/race/load evidence set.

Rebuild becomes discussable only if all are true:

- P3-R4 clean admission-off drilldown proves PostgreSQL row locking is the release-blocking bottleneck after ruling out outbox, pool sizing, transaction work, k6, and Windows-local artifacts.
- ADR defines Redis key topology, hash tags, reservation status model, script versioning, TTL/eviction/failover behavior, reconciliation, rollback, and metrics.
- PostgreSQL settlement remains final; Redis can only issue a reservation candidate.
- Tests prove bid/cancel/end/cap races, Redis down/restart, script BUSY/timeout, duplicate client bid, expired reservation, order uniqueness, seq continuity, and no accepted Redis reservation left unreconciled after crash/load.

## Concrete Borrowing Plan

### P3: Defensible Current Claim

Keep the current architecture and claim only what is already true:

```text
Redis Lua is already used in two bounded places:
1. GCRA admission protection for user/IP/auction request pressure.
2. One-time WebSocket ticket consumption.

I intentionally did not use Redis Lua as auction truth because Redis atomicity
does not cover PostgreSQL audit rows, order creation, outbox recovery, browser
gap recovery, or payment/idempotency replay. PostgreSQL remains money truth.
```

This is stronger than saying "we did not use Redis Lua"; the accurate story is selective use with an explicit boundary.

### P3/P4: Low-Risk Hardening Worth Borrowing

These improve explainability without changing auction truth:

1. Script taxonomy:
   - Name the scripts in docs and metrics as `bid_admission_gcra` and `ws_ticket_consume`.
   - Record their role: protective admission or ephemeral auth, never money truth.
   - Implementation landed in `backend/internal/redisx`.

2. Script outcome metrics:
   - Track allowed/rejected/error/timeout by script and dimension.
   - Keep Redis-down anomaly as the product-visible signal.
   - Implementation emits `redis_lua_script_total{script,outcome}` and `redis_lua_script_latency_seconds{script,outcome}`.

3. Error classification:
   - Distinguish Redis unavailable, context timeout, BUSY/script timeout, `NOSCRIPT`, and script result parsing errors.
   - Admission can fail open; tickets should fail closed.
   - Implementation classifies errors in `redisx.ClassifyScriptError`.

4. Key naming discipline:
   - Current single-key scripts work in standalone and are cluster-compatible enough because each script touches one key.
   - Future multi-key scripts must use hash tags and avoid programmatically generated undisclosed keys.
   - Bid admission keys now use `bid:{auction}:limit:*` helpers so every per-auction key has the same hash tag if a future combined script is introduced.

5. Optional script preload:
   - Direct `Eval` has been replaced by go-redis `Script.Run`, which optimistically uses `EVALSHA` and falls back to `EVAL` on `NOSCRIPT`.
   - This improves operational discipline and script identity; it does not change auction correctness.

### P3-R4/P3-R6: Evidence-Gated Reservation Prototype

Only after the go criteria are met, build a prototype behind a feature flag:

```text
Redis reservation script:
  input: auction_id, user_id, client_bid_id, amount, observed_state_version
  keys:  auction:{auction_id}:reservation:{client_bid_id}
         auction:{auction_id}:candidate
         auction:{auction_id}:dedupe:{user_id}:{client_bid_id}
  output: RESERVED / REJECTED_PRECHECK / DUPLICATE / STALE_STATE / TOO_HOT

PostgreSQL settlement:
  lock auction row
  re-evaluate all rules from DB truth
  write bid/order/event/outbox/idempotency
  mark reservation SETTLED or REJECTED in audit/reconciliation state
```

Reservation invariants:

- Redis never decides final winner, price, cap sold, cancel, end, order, or idempotency response.
- Every reserved candidate must become `SETTLED`, `REJECTED_BY_DB`, or `EXPIRED`.
- Redis loss cannot create a winner; it can only force fallback to direct PostgreSQL path or reject with a safe retry.
- A stale Redis view cannot override DB terminal state.
- The DB response is the only replayable response for idempotency.
- The outbox event `auction_id + seq` remains the browser recovery order.

New reconciliation anomalies needed before adoption:

| Anomaly | Meaning |
|---|---|
| `REDIS_RESERVATION_UNSETTLED` | Reservation exists past TTL/grace without DB settlement or explicit expiry. |
| `REDIS_RESERVATION_DUPLICATE` | Same client bid/user has multiple active reservations. |
| `REDIS_RESERVATION_PRICE_DRIFT` | Redis candidate price differs from DB-settled price. |
| `REDIS_RESERVATION_STATE_DRIFT` | Redis candidate accepted against stale DB status/version. |
| `REDIS_RESERVATION_SEQ_GAP` | Settled reservation did not produce continuous auction event seq. |

## Mapping To Official Scope

| Official Scoring Term | Redis Lua Borrowing Landing |
|---|---|
| 完整工程链路 | Keep the complete item/rule/bid/order/payment/history/diagnostics chain in PostgreSQL-backed application code. Redis Lua protects the gateway and tickets; it does not replace the chain. |
| 竞拍数据采集 | Admission rejects, Redis-down anomalies, bid/reject events, order events, and WS activity remain collected through app-owned producers. A reservation prototype would add reservation audit events before adoption. |
| 数据治理 | Current truth path preserves schema, enums, trace_id, idempotency, ordering, and replay boundary in DB. Redis keys stay derived/protective. |
| 后端服务 | Keep `rule validation`, `row lock`, `state machine`, `outbox`, and `scheduler` as the backend core. Lua admission is an edge guard before the backend truth path. |
| 接口网关 | This is the strongest current Redis Lua landing: auth/ACL/schema/idempotency probe/rate limit/error code remain in gateway, with Lua GCRA for distributed admission. |
| 前端交互 | H5 still receives server-authoritative events and snapshot recovery. Redis Lua does not authorize optimistic成交 or local countdown裁判. |
| 系统可用性 | Redis down for rate limit fails open with anomaly; ticket Redis down fails closed; DB lock timeout and outbox poison remain visible. |
| 性能 | Do not claim a speed win from small tests. Claim bottleneck discipline: Lua protects overload, while reservation is gated on P3-R4 lock evidence and P3-R6 ADR. |
| 稳定性 | Lua is used for bounded admission/backpressure, not unbounded auction truth. Future scripts need BUSY/timeout/cluster-slot handling. |
| 可观测性 | Current anomalies and monitor pages should surface Redis limiter down/rejects; future hardening can add script latency/error classification. |
| 核心挑战优化 | Final-second bid correctness remains PostgreSQL-serialized; recoverable realtime remains outbox/seq/snapshot based. Lua helps failure gates by protecting request pressure. |
| 独特思考 | The project does not chase "Redis is faster" as a slogan. It uses Redis Lua where its atomicity matches the domain boundary and refuses it where audit/recovery would become weaker. |

## Mapping To Two Core Challenges

### 复杂竞拍规则

Redis Lua should not be the main selling point.

The answer must stay:

- PostgreSQL row lock serializes bid/cancel/end/cap races.
- Go domain code enforces 0 start, fixed increment, reachable cap, automatic extension, fat-finger confirm, self-leading reject, terminal states, and one order.
- Idempotency replay is DB-backed and stores the response payload.
- Lua admission only decides whether a request is allowed to enter the money path.

If a Redis reservation prototype is later added, it must be described as a pre-settlement optimization. The DB still re-evaluates every rule.

### 毫秒级实时同步

Redis Lua helps indirectly:

- Admission Lua reduces overload before downstream queues collapse.
- Ticket Lua prevents token reuse during reconnect/connect storms.
- Redis projection/history remains a recovery substrate, reconciled against PostgreSQL.

Redis Lua does not replace:

- server-authoritative time;
- auction seq continuity;
- outbox crash recovery;
- WS gap detection;
- snapshot fallback;
- disabled dangerous CTA during recovery.

## Competitive Position Against Direct Redis Lua Users

If another project directly uses Redis Lua for bidding, do not argue that Redis Lua is bad. Argue scope and proof:

1. Redis Lua proves atomicity inside Redis. It does not prove DB audit, order creation, payment idempotency, outbox recovery, browser gap recovery, or cancel/end/cap race correctness.
2. If Redis decides winner or price, they must explain Redis restart, AOF/RDB persistence, failover, eviction, TTL expiry, script BUSY/timeout, cluster slots, duplicate client bid, and reconciliation to SQL/audit.
3. A fast benchmark without crash/race/reconciliation proof is not enough. The official score includes `完整工程链路`, `数据治理`, `系统可用性`, `稳定性`, `可观测性`, and `核心挑战优化`, not only QPS.
4. This project can show the exact transaction that writes bid/order/event/outbox/idempotency, then show Redis Lua as a protective edge optimization.
5. If their design keeps DB final settlement and uses Redis only for reservation candidates, it is closer to this project's future gated prototype. Then the comparison is evidence quality: reconciliation, invariant verifier, and race tests.
6. If their design makes Redis the only auction truth, the evaluator should ask how history, order, payment, audit, replay, and terminal races survive Redis loss or split-brain.

## Interview Drill

| Question | Defensible Answer | Code/Evidence To Show |
|---|---|---|
| Did you use Redis Lua? | Yes. I use it for GCRA admission and one-time WS tickets, where Redis is the correct bounded authority. | `backend/internal/gateway/bid_admission.go:210`, `backend/internal/realtime/ticket.go:44` |
| Why not use Lua for the final bid result? | Redis atomicity is scoped to Redis keys. Final bid result also needs DB audit, order creation, idempotency replay, outbox seq, scheduler, and browser recovery. | `backend/internal/auction/bid.go:648`, `backend/internal/auction/repository.go:481` |
| What exactly did you borrow from Redis Lua? | Small atomic scripts for read-decide-update, per-key TTL state, bounded gateway admission, and atomic one-time token consume. | This review; Redis rate limiter docs |
| What happens if Redis is down? | Admission fails open with anomaly so correctness still reaches PostgreSQL; WS ticket consume fails closed because unsafe connection should not proceed. | `backend/internal/gateway/bid_admission.go:193`, `backend/internal/realtime/ticket_test.go:56` |
| Are your three limiter checks atomic together? | No, and they do not need to be for money correctness. They are layered protective gates. A strict combined limiter would need a same-slot multi-key Lua design. | `backend/internal/gateway/bid_admission.go:181` |
| What would make Redis Lua reservation worth building? | Clean P3-R4 evidence that PG hot-row is release-blocking, plus ADR/reconciliation/invariant tests proving Redis cannot become split-brain auction truth. | `docs/archive/progress/p3-decision-log.md` P3-D14 |
| Why are you stronger than a team that says "we used Redis Lua for speed"? | I can show where Lua is safe, where it is unsafe, and the DB transaction that proves auction correctness. Speed claims without reconciliation do not answer the official scoring table. | `docs/design-v2-industrial/00-project-brief.md` |

## No-Go Claims

Do not say:

- "Redis Lua guarantees auction correctness."
- "Redis is our bid source of truth."
- "Lua reservation improves performance" without a clean benchmark and reconciliation proof.
- "Redis Cluster scaling is automatic" for multi-key scripts.
- "EVALSHA/preload makes the design correct."
- "Small local tests prove Redis Lua is better than PostgreSQL row locks."

Say instead:

- "Redis Lua is used for bounded atomic admission and ticket consume."
- "PostgreSQL remains auction money truth."
- "Redis Lua reservation is a future optimization candidate with explicit reconciliation and invariant gates."
- "The current design wins on auditability, recovery, and defensible correctness; performance changes are evidence-gated."
