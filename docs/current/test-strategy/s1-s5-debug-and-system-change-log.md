# S1-S5 Debug And System Change Log

> Status: judge-defense engineering log, 2026-06-04.
> Scope: S1-S5 runs, failures, fixes, and **system-code** changes discovered
> while validating the current Redis/Kafka/PostgreSQL live-auction architecture.
> Test harness/script changes are mentioned only when they explain evidence; they
> are not counted as system-code changes.

## 1. Executive Summary

This is the story to tell a senior Douyin/TikTok Shop reviewer:

> "We did not just run a happy-path benchmark. S1-S5 exposed measurement
> contamination, settlement drain bottlenecks, payment-finality risk, Redis state
> loss edge cases, relay cursor safety issues, Kafka redelivery idempotency, and
> WebSocket close/reconnect behavior. For each one we either fixed the production
> code, rejected a tempting optimization after data, or documented the remaining
> boundary instead of overclaiming."

Current evidence snapshot:

| Scenario | Current evidence | Verdict boundary |
|---|---|---|
| S1 final-second contention | `5D92X7QG`: 1000 unique bids, 7 accepted, 993 rejected, 0 HTTP failures, correctness PASS; PTS sampling p99 64ms, server gateway p99 about 46ms | Correctness strong; strict client-side p99 <=50ms needs rerun or explicit server-vs-client boundary |
| S2 steady soak / convergence | `s2-ecs-30m-20260604T095720`: 85,499 decisions over independent-ECS 20/s -> 60/s -> 100/s 30-minute open-model soak, HTTP p99 3.30ms, custom decision p99 4ms, dropped 0, all verifier gates PASS. `s2-convergence-drain-decision-ecs-20260604T1937`: 49,049 decision/reject-heavy decisions over 100/s -> 200/s -> 400/s -> 600/s, decision p99 4ms, final Kafka/Redis/PG/outbox zero, verifier PASS | Bid-decision endurance and decision/reject-heavy convergence PASS; read-interference and accepted-heavy fanout are separate, accepted-heavy 600/s remains attack evidence |
| S3 fanout | `s3-local-scale-1000-liveonly-20260602T2303`: 1000 WS, 301 accepted updates, 276,000 receive samples, fanout p99 22ms, viewer errors 0 | 1000+ single-room local PASS; 2000/10k headline unproven |
| S4 fault resilience | 2026-06-04 P0/P1 local pass: Redis/backend/PG/Kafka/flush/both; bidder RTO 2/14/2/12/2/12s; full convergence worst 29s; RPO=0. Independent VPC k6 Kafka run `s4-p1-kafka-independent-20260604T202510` also pass. P2 Redis partial now passes with `pts-1c-partial-20260604T224626`: 4000 decisions, 200 paused, RTO 8s, convergence 25s, verifier PASS. | Strong local single-node chaos proof including one Redis proxy-path partial-network fault; not Kafka RF=3/Redis HA production benchmark |
| S4 07/08 | `07`: 600 decisions over 512 relay batch ceiling -> 600/600 relayed. `08`: same SOLD ledger message 3x -> one settlement/order/outbox effect | P0 depth gates now PASS; backlog-age observability remains P1 |
| S5 reconnect | Clean 200 VU for 2m: `s5-20260604T221312`, 34,814 recovered, TTCS p99 87ms, 0 gap/dup/error/truth mismatch. Toxiproxy reset_peer 50 VU for 2m: `s5-20260604T231925`, 8,849 recovered, TTCS p99 341ms, reconnect retries 3,826, 0 gap/dup/error/truth mismatch. | Clean reconnect and backend proxy-path reconnect PASS; browser/mobile/LB weak-network E2E still P1 |

Production HA and weak-network expansion is documented separately in
[`production-ha-expansion-and-judge-defense.md`](production-ha-expansion-and-judge-defense.md).
That document preserves the current boundary honestly: Kafka RF=3/minISR=2,
Redis HA failover/split-brain fencing, multi-WS-gateway reconnect, LB/NAT idle
timeout, real mobile weak networks, and cross-AZ/cross-region failures are
concrete next topology tests, not claims already proven by local S4/S5.

## 2. System-Code Changes Made Because Of S1-S5

Only runtime/system behavior changes are listed here. Test scripts, docs, and
evidence collectors are excluded from this table.

| Area | System file | What changed | Why it changed | Proof / test |
|---|---|---|---|---|
| Payment finality gate | `backend/internal/auction/bid.go` | `PayMockWithSecret` now calls `ensurePaymentConvergenceReady` before starting a new payment when the order is not already paid. The gate blocks payment if Redis-engine settlements are open/failed/DLQ or auction outbox deliveries are unpublished. | S2/S4 showed that foreground bidding can be correct while settlement/outbox still drains. In a high-value auction, payment must not open from incomplete settlement state. | `TestPaymentWaitsForSettlementConvergence`, `TestPaymentWaitsForOpenRedisEngineSettlement`; S4 payment-finality convergence gate |
| Redis state loss fail-closed | `backend/internal/redisengine/engine.go` | Cold Redis seeding is now refused if settlement rows, engine checkpoints, or non-zero PostgreSQL engine seq exist; the system requires controlled resume/reconcile instead of silently rebuilding a new hot state. | Redis `FLUSHALL`/state-loss testing exposed the danger of restarting a hot auction from stale PG state or seq 1 after durable history exists. | `TestRedisLedgerMissingHotStateAfterSettledLedgerFailsClosed`, `TestRedisLedgerMissingHotStateWithDurableSettlementAttemptFailsClosed`, `TestRedisLedgerMissingHotStateAfterPrewarmCheckpointFailsClosed`, `TestRedisLedgerMissingHotStateAfterExistingAuctionSeqFailsClosed`; S4 Redis FLUSHALL PASS |
| ACL error ordering under Redis loss | `backend/internal/redisengine/engine.go` Lua script | ACL membership check moved after state/paused/reconciling checks. | After Redis hot state loss, ACL cache may be gone too. Returning `ACL_FORBIDDEN` would mislead the user and hide that the engine is reconciling. Correct behavior is fail-closed/recovering semantics first. | Redis state-loss tests and S4 Redis FLUSHALL/reconciling evidence |
| Relay cursor safety | `backend/internal/redisengine/engine.go` | `relayAuctionLogBatch` now fails the pass on missing payload, malformed payload, auction-id mismatch, or `AppendBatch` returning fewer/more ledger messages than input results. | S4 07 forced us to reason about Redis Stream cursor movement. Skipping bad entries while advancing the cursor can silently lose a bid decision. The safer behavior is fail/alert/retry. | `TestRelayBackpressureDrainsBeyondBatchCeiling`; adjacent relay tests; `tests/chaos/07-relay-backpressure.sh` PASS |
| Checkpoint availability on resume | `backend/internal/redisengine/engine.go` | Resume path can upsert a checkpoint from current PostgreSQL state before writing Redis state snapshot. | Redis data-loss recovery needs a checkpoint/current-state anchor. Local one-auction FLUSHALL recovery passed, but docs now honestly limit the 2s result to local scope. | S4 Redis FLUSHALL evidence; Redis state-loss integration tests |
| WebSocket close/read detection | `backend/internal/realtime/server.go` | `ServeWS` now derives connection context from `conn.CloseRead(connCtx)` so client close/error cancels keepalive/recovery work promptly. | S5 reconnect testing needs real disconnects, not server goroutines waiting until timeout. Prompt read-side close detection reduces stale sessions and makes reconnect TTCS meaningful. | S5 clean 200 VU 2m: 34,814 recovered, TTCS p99 87ms, 0 gap/dup/error/truth mismatch |
| Rejected settlement materialization | `backend/internal/redisengine/engine.go` current path | Rejected decisions remain materialized with bid row, idempotency record, settlement audit, and optional public event only when the reject should broadcast. | S2 proved rejected decision volume is the settlement bottleneck. We considered a more aggressive direct-SETTLED/skip path, measured it, and rejected it because it worsened lag and risked verifier/payment safety. | S2 direct-SETTLED failed run; current set-based/log-suppressed path retained; verifier later PASS |
| Accepted settlement batching | `backend/internal/redisengine/engine.go` | Settlement worker batches safe same-auction, same-epoch, consecutive `ENGINE_ACCEPTED` Kafka prefixes in one transaction while still writing bids, auction events, outbox, idempotency, settlement rows, auction seq/price/winner, and checkpoint. It now continues through multiple safe prefixes in the same fetched Kafka batch instead of returning after the first prefix; mixed, gap, stale, terminal, or replay cases still create boundaries or fall back. | `s2-capacity-accepted-ecs-20260604T150519` and `s2-capacity-accepted-postfix-ecs-20260604T161315` both proved Redis decisions stayed fast at the 50/100/200/400/600 accepted-heavy stair, but async PG settlement/Kafka lag remained the end-to-end bottleneck. The current fix is incremental drain-efficiency work, not a 600/s pass claim. | `TestKafkaSettlementBatchesAcceptedPrefix`, `TestKafkaSettlementBatchesAcceptedPrefixBeforeReject`, `TestKafkaSettlementBatchesAcceptedSuffixAfterReject`; next independent-ECS rerun should first target a clean 50/100/200/300/400 ceiling |

## 3. Scenario Debug Timeline

### S1 Final-Second Contention

Business scene: 1000 high-intent bidders submit near the last second of one hot
auction. The important metric is not accepted TPS; it is final `ENGINE_*`
decision p99 plus winner/reject correctness.

What happened:

| Step | Observation | Engineering response | Result |
|---|---|---|---|
| Initial framing | Low accepted count looked weak if explained as accepted TPS. | Reframed S1 as decision goodput: accepted and correctly rejected bids are both successful adjudications. | `5D92X7QG`: 1000 unique bids, 7 accepted, 993 rejected, 0 HTTP failures, correctness PASS. |
| Measurement boundary | PTS sampling p99 was 64ms while server gateway p99 was about 46ms. | Kept the evidence but did not overclaim strict client p99 <=50ms. Marked S1 as correctness strong but needing a clean rerun/review for final external PASS. | Defensible boundary: server-core path appears inside target; published PTS client-side p99 still needs rerun or explanation. |

System-code impact: no current runtime-code change is attributed to S1 in this
log. S1 mainly forced better metric framing and correctness verification.

Judge answer:

> "S1 is not saying only 7 bids were handled. It says 1000 final decisions were
> made under final-second contention; 993 rejections were correct because the
> price had already moved. The remaining work is a clean client-side p99 <=50ms
> PTS artifact, not a correctness gap."

### S2 Steady Auction / Soak

Business scene: normal auction minutes, with sustained offered bid traffic,
mostly rejected because the price ladder moves. Users care that bid decisions
stay fast during the auction; finance/payment care that settlement converges
after close.

Current independent-ECS long-soak result:

| Signal | Value |
|---|---:|
| Run label | `s2-ecs-30m-20260604T095720` |
| Load model | open-model k6, independent same-VPC ECS |
| Stages | 20/s -> 60/s -> 100/s bid attempts, 10 min each |
| Total bid attempts | 85,499 |
| Final `ENGINE_*` decisions | 85,499 |
| Accepted / rejected | 61 / 85,438 |
| HTTP p99 / custom decision p99 | 3.30ms / 4ms |
| dropped_iterations | 0 |
| HTTP failures / auth-ACL / admission / non-decision failures | 0 / 0 / 0 / 0 |
| k6 host health | CPU mostly 3-7%, RSS about 83MB, FD about 86, TCP retransmits essentially 0 |
| Settlement state | 61 accepted + 85,438 rejected all `SETTLED` |
| Kafka / Redis / outbox drain | lag 0, pending 0, outbox watermarks all 0 |
| Verifier | all P0/P1 gates PASS |

Evidence:

```text
docs/perf/pts/evidence/incoming/s2-ecs-30m-20260604T095720/
```

Boundary: this proves 30-minute bid-decision endurance and async convergence.
It does not prove accepted-heavy WebSocket fanout. The follow-up
`S2-read-interference` run used 100 bid attempts/s plus 2000/5000/10000 HTTP
reads/s across auction snapshot, leaderboard, and my-bid history. It is recorded
as bottleneck evidence, not a pass: bid p99 stayed 5.68ms, but reads delivered
only about 2142/s with 2,057,742 dropped iterations and DB-pool wait saturation.

Debug path:

| Attempt | Evidence | Decision |
|---|---|---|
| Batch drain baseline | `s2-stair-1000-batchdrain-20260602T193000`: 80,999 decisions, M1 p99 5.79ms, convergence PASS in 158s. | Correct but too slow for payment/finality comfort. |
| Set-based rejected settlement | `s2-stair-1000-setbased-logdebug-2min-20260602T202733`: 70,999 decisions, p99 5.59ms, convergence PASS in 105s. | Direction looked good; logging state still noisy. |
| Set-based + success-log suppression | `s2-stair-1000-setbased-logsuppressed-2min-20260602T203614`: p99 5.52ms, convergence PASS in 122s. | Kept as current direction; convergence still variable. |
| More workers | `s2-stair-1000-setbased-workers4-90s-20260602T204827`: p99 5.38ms, fail at 90s with lag 84. | More workers do not fully parallelize one hot auction because the Kafka partition/order boundary remains. |
| Current 100s gate | `s2-stair-1000-setbased-logsuppressed-100s-20260602T211311`: 70,999 decisions, p99 5.44ms, p99.9 32.21ms, dropped 0, fail at 102s with lag 1371; verifier later PASS. | Foreground PASS; convergence near miss. Need 110s rerun before calling convergence PASS. |
| Direct-SETTLED trial | `s2-stair-1000-directsettled-100s-20260602T212330`: p99 5.36ms but lag 32033 and verifier incomplete. | Rejected and reverted. Faster-looking SQL idea was not kept because it harmed convergence/correctness. |
| Accepted-heavy capacity stair | `s2-capacity-accepted-ecs-20260604T150519`: k6 exit 0, dropped 0, HTTP failed 0, 131,574 final decisions, 125,376 accepted, 6,198 rejected, p99 3.89ms, p99.9 7.47ms; PG settlement only about 61k and Kafka lag about 77,888 at collection. `s2-capacity-accepted-postfix-ecs-20260604T161315`: k6 still clean, 131,574 decisions, 107,624 accepted, p99 13ms, but immediate service sample still had only about 69.7k accepted settlements and Kafka lag 64,476. | Split verdict remains. Redis synchronous decision layer is strong; async settlement/outbox is the capacity knee. Accepted-prefix batching improved code shape but 600/s is still attack evidence, not a clean pass. Next clean-ceiling target is 50/100/200/300/400. |
| Decision/reject convergence drain | `s2-convergence-drain-decision-ecs-20260604T1937`: independent same-VPC k6 used service private IP `172.16.179.112`, not public `47.113.223.90`; 100/s -> 200/s -> 400/s -> 600/s, 30s hold per stage, 49,049 final decisions, 6 accepted, 49,043 rejected, decision p99 4ms, p99.9 18ms, dropped 0, HTTP failed 0. Service verifier: 49,049/49,049 settlements, Kafka lag 0, Redis pending 0, Redis stream 49,049, outbox unpublished 0, all P0/P1 PASS. Final outbox publish `19:53:41.981 CST`, final settlement update `19:53:45.828 CST`, approximately the k6 ramp-down end. | Clean `S2-convergence-drain` PASS for normal decision-heavy traffic. It does not contradict older 100-122s local drain evidence because this run was smaller, smoother, later-code, and only 6 accepted decisions. Do not use it to claim accepted-heavy 600/s immediate drain. |

System-code impact:

- Current settlement path keeps rejected decision audit/idempotency instead of
  dropping rejected bids. This preserves retry semantics and arbitration data.
- Payment finality gate was added in `backend/internal/auction/bid.go` so S2
  settlement lag cannot open payment early.

Judge answer:

> "The S2 bottleneck is not Redis Lua. Redis/Gateway p99 is single-digit
> milliseconds. The bottleneck is asynchronous PostgreSQL settlement drain of a
> high volume of rejected decisions. We tried direct-SETTLED and rejected it
> after data. The product guard is payment convergence: users can keep bidding,
> but payment/final finance waits for settlement/outbox convergence."

### S3 Single-Room Fanout

Business scene: one hot live room has 1000+ online viewers; when a real accepted
bid changes price, viewers should see it within seconds.

Debug path:

| Run | Symptom | Root cause | Fix / decision |
|---|---|---|---|
| `s3-local-scale-1000-20260602T2300` | fanout p99 59.6s at 1000 WS, which looked catastrophic. | Harness counted history/recovery messages that were published before the viewer connected. That measures recovery age, not live fanout. | Changed S3 measurement to count only messages whose `published_at_ms >= connectedAtMs`. Kept the bad run as pollution evidence. |
| `s3-local-scale-1000-liveonly-20260602T2303` | 1000 WS held 60s, 301 accepted updates, 276,000 samples, p99 22ms, max 51ms, viewer errors 0. | Correct live-only M2 population. | Current S3 local 1000+ PASS. |
| 2000 WS attempt | incomplete summary / capacity not proven. | Local generator/resource boundary not yet controlled. | Do not claim 2000/10k; next is PTS cost variant or controlled local hold. |

System-code impact: no current runtime-code change is attributed to S3 in this
log. The change was measurement/harness-side: filter live fanout by connection
time. That distinction matters because a reviewer may ask whether the system was
fixed or the metric was fixed.

Judge answer:

> "The 59.6s and 22ms 1000-WS numbers do not conflict. The first counted old
> recovery/history messages; the second measures live publish-to-online-viewer
> receive. S3's business metric is live fanout. Recovery age is covered by S5."

### S4 Fault Resilience

Business scene: core infrastructure fails during live bidding. The system must
either keep deciding safely or fail closed, then converge with RPO=0.

Current live-fault evidence:

| Fault | Result |
|---|---|
| Redis SIGKILL | RTO 2s, convergence 19s, fault window accepted=0 |
| backend/settlement crash | RTO 14s, convergence 29s, 1200 failed attempts, 0 duplicate settlement |
| PostgreSQL SIGKILL | RTO 2s, convergence 18s, 1000 decisions during PG fault, 0 unsettled accepted after recovery |
| Kafka SIGKILL | bidder RTO 12s, full convergence 28s, Redis pending drained, Kafka lag 0 |
| Redis FLUSHALL | RTO 2s, convergence 20s, `RECONCILING`, settlements 1000/1000 |
| Redis+Kafka | RTO 12s, convergence 28s, accepted-in-window=0 |
| Redis partial network via Toxiproxy | PASS in current run: `pts-1c-partial-20260604T224626` had 4000 decisions, 200 paused, 0 reconciling, 0 HTTP errors, fault-window paused=200, recovery RTO 8s, restore-start-to-final convergence 25s, verifier PASS |

Debug path and system changes:

| Problem found | Why it mattered | System-code response | Evidence |
|---|---|---|---|
| PG/Kafka faults can leave back-office settlement/outbox temporarily behind while foreground bidding continues. | If payment opens during that window, the user may pay from incomplete finality state. | Added `ensurePaymentConvergenceReady` to payment initiation. | `TestPaymentWaitsForSettlementConvergence`, `TestPaymentWaitsForOpenRedisEngineSettlement`; S4 full-convergence gate. |
| Redis `FLUSHALL` / state loss can make Redis hot state and ACL cache disappear together. | Returning ACL denied or reseeding from stale PG hides the real safety issue and may produce wrong seq/winner. | ACL check moved after state/reconciling; cold seed now fails closed when durable history/checkpoint exists; controlled resume required. | Redis state-loss tests; S4 FLUSHALL PASS. |
| Kafka/Redis relay backlog over one batch ceiling was not explicitly proven. | A reviewer can ask whether async relay silently loses decisions when the stream exceeds one batch. | Added relay hardening: missing/malformed/mismatched stream entries and `AppendBatch` count mismatch fail the pass instead of skipping. | S4 07: 600 decisions over 512 batch, 600/600 relayed, next relay 0. |
| Worker crash proves replay indirectly, but not "same message delivered 3 times". | At-least-once delivery is a core Kafka reality; no-double-charge must be direct proof. | Settlement uniqueness/order/outbox path already supported the invariant; added focused system test to prove one business effect under 3x delivery. | S4 08: same SOLD message 3x -> 1 settlement/order/outbox delivery. |

Boundary kept honest:

- Kafka 12s / 28s is local single-broker readiness plus drain, not production
  leader-election proof. Settlement crash is the current worst full convergence
  at 29s.
- Redis FLUSHALL 2s is local one-auction rebuild evidence, not a Redis cluster
  restore SLA.
- S4 P2 partial weak Redis network is counted as pass only for the current
  reset-peer + timeout toxic. Earlier latency-only/timeout-only attempts remain
  diagnostic failures and are not used as pass evidence.
- Production path is Kafka RF=3/minISR=2/acks=all/unclean election disabled,
  Redis HA/Sentinel or managed Redis, and backlog-age observability.

Judge answer:

> "S4's strongest claim is correctness, not magic HA. Redis truth loss fails
> closed, PG/Kafka faults do not lose accepted decisions, Kafka redelivery does
> not duplicate settlement, and payment/finality waits for convergence. P0/P1 are
> pass locally and Kafka also passed with independent VPC k6 pressure. P2 partial
> Redis proxy-path reset/timeout now also reaches clients as fail-closed paused
> responses and converges. Production HA is a separate RF=3/Redis-HA expansion
> path."

### S5 Reconnect Recovery

Business scene: a mobile viewer/bidder loses WebSocket during active bidding,
misses real seq updates, and reconnects with stale `last_seq`.

Debug path:

| Run / issue | Root cause | Fix / decision |
|---|---|---|
| Docker/k6 permission issue | local k6 absent / Docker needed escalation | runner uses Docker k6 path with explicit evidence directory |
| session CSV not found | k6 `open()` path relative to script directory | script default path fixed |
| no missed window | bid source amount became stale after first accepted bid | bid source now reads snapshot and bids `current + increment` |
| 560 recovered but threshold failed | iterations without enough missed seq were counted as recovery errors | no-gap iterations count as skipped, not failed recovery |
| server did not always detect client close promptly enough for clean reconnect semantics | websocket read side was not tied into connection context | `conn.CloseRead(connCtx)` now cancels connection work on client close/error |
| `s5-20260604T220852` invalid | session CSV contained only header, so k6 users had no token | reran `SESSION_COUNT=1000 L4B_PROFILE=pts-1b reset-l4b-final-second-pressure.sh`; valid CSV has 1001 lines |
| S5 network default could bypass proxy | `DISCONNECT_MODE=network` left `WS_URL` at the normal 18080 default unless explicitly set | runner now defaults network mode to `ws://127.0.0.1:18081`; `s5-20260604T221634` explicitly used that proxy path |
| S5 network reset-peer failed | persistent reset turbulence produced 2697 `s5_recovery_errors` while successful recoveries were fast and ordered | added bounded reconnect retry metrics and separated initial online connection from the toxic reconnect leg |
| S5 network invalid/diagnostic reruns | `s5-20260604T224839` had empty session CSV/token errors; `s5-20260604T225824` had 2539 recovery errors; `s5-20260604T230907` had only 2 recovery errors but still failed | added session CSV validation, retry counters, and clean-initial/toxic-reconnect harness semantics |

Current evidence:

| Run | Result |
|---|---|
| `s5-20260604T221312` clean 200 VU, 2m | 34,814 recovered, TTCS p99 87ms, 0 gap/dup/error/truth mismatch, 267,750 HTTP reqs failed 0, 69,828 WS sessions |
| `s5-20260604T231925` Toxiproxy reset_peer 50 VU, 2m | 8,849 recovered, TTCS p99 341ms, reconnect attempt errors/retries 3,826, 0 recovery errors, 0 gap/dup/truth mismatch, HTTP failed 0, k6 exit 0 |
| `s5-20260604T221634` Toxiproxy reset_peer 50 VU, 2m | 2,552 recovered, TTCS p99 17ms, 0 gap/dup/truth mismatch, but 2,697 recovery errors; k6 exit 99, diagnostic NOT PASS |

System-code impact:

- `backend/internal/realtime/server.go` now uses `conn.CloseRead(connCtx)` in
  `ServeWS`, improving close detection and cleanup for reconnect tests and real
  clients.

Judge answer:

> "S5 is not fanout p99. It measures time-to-current-state after a real missed
> seq window. At 200 local reconnect VU for 2 minutes, 34,814 stale sessions
> recovered to current seq with TTCS p99 87ms and no gaps/duplicates/errors.
> The reset-peer network run now also passes for backend reconnect recovery:
> 8,849 stale sessions recovered through Toxiproxy with TTCS p99 341ms and no
> gaps/duplicates/errors. Browser weak-network UI proof is still a P1 follow-up."

## 4. Why These Changes Show Engineering Maturity

| Theme | Example |
|---|---|
| Correct metric boundary | S3 59.6s was not hidden; it was classified as history/recovery contamination and separated from live fanout. |
| Data-driven rejection of bad optimization | S2 direct-SETTLED sounded faster but produced worse lag and incomplete verifier, so it was reverted. |
| Product safety over raw speed | S4 back-office convergence can lag bidder recovery, so payment/finality is gated instead of pretending bidding RTO means finance finality. |
| Fail-closed under uncertainty | Redis state loss returns reconciling/paused semantics and requires controlled resume. |
| At-least-once is handled at business layer | S4 08 does not claim Kafka exactly-once; it proves idempotent settlement/order/outbox effect. |
| Test artifact honesty | S1 strict PTS p99 and S2 100s convergence are explicitly marked as rerun/near-miss boundaries. |

## 5. Questions A Senior Reviewer Will Ask

**Q: Did you change business code or only pressure scripts?**

A: Both happened, but they are separated. System-code changes include payment
convergence gate, Redis state-loss fail-closed behavior, relay cursor safety, and
WebSocket close detection. Harness changes include S3 live-only measurement and
S5 reconnect load generation fixes.

**Q: Why does payment wait if the bidder already saw `ENGINE_SOLD`?**

A: `ENGINE_SOLD` is foreground decision truth, but payment is financial finality.
If Kafka/PG/outbox have not converged, opening payment risks acting on incomplete
settlement. `PayMockWithSecret` now blocks with `PROCESSING_RETRY_LATER` until
open settlement/outbox work is zero.

**Q: Why not just parallelize settlement with more workers?**

A: One hot auction is one ordered stream. More Kafka consumers help across many
partitions/auctions, not one ordered auction partition. S2 worker4 was a near
miss but did not eliminate convergence; the safer next step is reducing rejected
settlement write cost without losing audit/idempotency.

**Q: What prevents Redis state loss from restarting sequence at 1?**

A: Cold seeding now refuses to proceed if durable settlement rows, checkpoints,
or non-zero PG engine seq exist. The engine enters reconciling/paused and must
resume from controlled checkpoint/current state.

**Q: Does S4 prove production Kafka HA?**

A: No. It proves functional replay/drain and business idempotency on a local
single broker. Production HA requires RF=3, minISR=2, `acks=all`, unclean leader
election disabled, and a separate broker-loss chaos profile.

## 6. Remaining Follow-Ups

| Priority | Follow-up | Why |
|---|---|---|
| P0/P1 | Rerun S1 for clean strict client p99 <=50ms or write a formal server-vs-client boundary review | Current S1 correctness is strong, but PTS client p99 64ms is not a strict PASS |
| P0/P1 | Rerun S2 with `S2_CONVERGENCE_TIMEOUT_SECONDS=110` | Current 100s gate is a near miss; 110s target needs proof |
| P1 | Browser weak-network payment/bid CTA E2E | Backend S5 network now passes; visible H5 disabled-state proof is still needed |
| P1 | Relay backlog age / cursor-lag observability | S4 07 proves 600/600 drain; production reviewers will ask how backlog age alerts |
| P2 | Optional latency-only S4 enhancement | Current pass uses reset-peer + timeout; latency-only remains diagnostic coverage, not the fail-closed baseline |
| P1 | S3 PTS cost variant at 2000 WS and/or controlled local 10k hold | Current 1000+ local pass is credible; 10k headline is unproven |
| P1 | Read-interference and multi-room gates | S1-S5 focus one hot auction; official scope includes room-level routing and full-stack traffic |
