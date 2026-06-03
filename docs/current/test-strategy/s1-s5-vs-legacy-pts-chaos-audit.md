# S1-S5 vs Legacy L1-L4 / Chaos Audit

> Status: internal audit, 2026-06-03.
> Viewpoint: a senior Douyin/TikTok Shop engineer reviewing the official
> live-auction assignment, not a graduation-project benchmark.
> Scope: compare the current S1-S5 scenario spine against legacy
> `tests/pts` L1-L4 assets and `tests/chaos` scripts; decide what is missing,
> what should be aligned, and when PTS is worth the cost.

## 1. Executive Verdict

S1-S5 is the right external story: it maps directly to the assignment's live
auction moments: final-second bidding, steady auction, single-room fanout, fault
recovery, and weak-network reconnect.

The old L1-L4 assets are still useful, but they should stay as **assets**, not
return as the public test naming. They add several missing dimensions:

| Gap | Why it matters | Priority |
|---|---|---|
| Bid + HTTP read traffic interference | real H5 users poll snapshot, leaderboard, and bid history while others bid; this can exhaust DB pool or CPU even if S1 bid-only passes | P0/P1 |
| Relay backpressure gate | async Redis Stream -> Kafka/settlement may lag silently under bursts; senior reviewers will ask how backlog is bounded and alerted | P0 |
| Explicit Kafka redelivery idempotency gate | S4 worker crash proves replay indirectly; chaos 08 explicitly attacks duplicate Kafka delivery 3x | P0 |
| Multi-room isolation under load | official brief asks room-level WebSocket routing and multi-room isolation; S1-S5 mostly focus one hot room | P1 |
| Full auction lifecycle / 30-min soak | S2 local stair proves foreground p99 but not a full business lifecycle with final close, extension, order, and memory slope over 30 min | P1 |
| L4 full mixed workload | closest to production readiness: bid + WS + reads + side auctions + monitor traffic | P2 unless claiming production readiness |
| Browser weak-network CTA gate | S5 backend/k6 proves recovery correctness; H5 visible disabled bid/payment CTA under forced socket close is not yet fully proven | P1 |

PTS is necessary for polished external charts and distributed source IP evidence
for **S1** and **S3**. PTS is not necessary for **S4/S5 correctness**: SIGKILL,
Toxiproxy, replay, and convergence gates answer those questions better and cost
nothing.

## 2. External Research Boundaries

Use these as calibration, not as fake "industry averages":

| Topic | Source-backed fact | Impact on our interpretation |
|---|---|---|
| RTO/RPO | AWS defines RTO/RPO as business objectives based on acceptable recovery delay and data loss, not universal constants. | S4's RPO=0 is mandatory for accepted auction decisions; RTO must be explained by user journey and payment/finality gates. |
| Open vs closed load | k6 arrival-rate executors are open-model: iterations start independently of response time. Closed VU loops self-throttle under overload. | S2 sustained-load proof should stay local k6 arrival-rate; old closed-loop JMeter/VU p99 is supporting only. |
| PTS cost and IPs | Alibaba Cloud PTS bills by VUM; concurrency mode rounds `max concurrent users / 500`, sampling adds a multiplier, and extra IPs add cost. | PTS should be used only where distributed IPs and exportable charts are worth the money. |
| Kafka durability | Kafka docs: `acks=all` waits for ISR acknowledgements; `min.insync.replicas` with RF=3/minISR=2 enforces majority persistence; unclean leader election may cause data loss. | Current S4 single broker proves functional replay/drain, not production Kafka HA. Production path must be RF=3, minISR=2, `acks=all`, unclean election disabled. |
| Redis HA | Redis Sentinel requires quorum and down-after/failover-timeout settings; Redis `WAIT` can wait for replica acknowledgements but does not make Redis a CP system. | Current single Redis proves fail-closed/rebuild logic, not Sentinel/managed-Redis failover. |
| Retry storms | AWS backoff+jitter guidance exists because synchronized retry waves amplify incidents. | S5 must keep jittered reconnect/backoff; fixed simultaneous reconnect loops are only a stress scenario. |

References are listed at the end.

## 3. Current S1-S5 Internal Coverage

| Scenario | Test logic | Real business scene | Current concrete evidence | Metric -> business meaning | User view | External benchmark / gap | Verdict |
|---|---|---|---|---|---|---|---|
| S1 final-second contention | 1000 PTS users, one hot auction, one synchronized bid each; verifier checks winner, reject basis, seq, settlement. | The last second of a jewellery/live-commerce auction where many users try to snipe. | `5D92X7QG`: 1000 unique bids, 7 accepted, 993 rejected, 0 HTTP failures, correctness PASS; PTS sampling-log p99 64ms, server gateway p99 about 46ms. | M1 is request -> final `ENGINE_*`; accept and reject both count. 64ms means the bidder sees a final accept/reject in ~0.06s, but strict <=50ms client-side target is not met in that review. | User gets fast "accepted/outbid/too low" rather than pending guess; winner equals highest valid accepted bid. | Current strict user-visible PTS p99 still needs a clean <=50ms run or a defensible "server-core p99 vs PTS client p99" boundary. PTS is the right tool here. | Core correctness strong; strict current PASS evidence needs rerun/review. |
| S2 steady auction / soak | Local k6 open arrival-rate stair, sustained decisions, dropped iterations, convergence wait. | Normal live auction minutes: minority actively bid, price climbs, many users watch. | `s2-stair-1000-setbased-logsuppressed-100s-20260602T211311`: 70,999 decisions, HTTP p99 5.44ms, p99.9 32.21ms, dropped 0; 100s convergence gate failed at 102s with Kafka lag 1371, later verifier passed after drain. Best previous convergence PASS: 158s. | Foreground p99 says bidding remains smooth; convergence seconds say when settlement/payment/finance can be trusted. | During bidding, users get fast decisions. After close, payment/final confirmation must wait for lag/pending/outbox drain. | k6 open model is correct for sustained rate. Gap is back-office convergence, not foreground latency. Need a <=110s rerun before marking convergence PASS. | Foreground strong; payment-safety convergence still a limit. |
| S3 single-room fanout | 1000 local WS viewers online; accepted updates broadcast with `published_at_ms`; client measures live-only receive latency. | One hot room: few bidders change price, 1000+ viewers must see it within seconds. | `s3-local-scale-1000-liveonly-20260602T2303`: 1000 WS held 60s, 301 accepted updates, 276,000 receive samples, fanout p99 22ms, p95 14ms, max 51ms, viewer errors 0. Polluted `s3-local-scale-1000-20260602T2300` p99 59.6s excluded because it counted history/recovery messages. 2000 WS attempt incomplete. | M2 is server publish -> online viewer receive. 22ms means online viewers saw price changes near-immediately in the local 1000-WS run. | Viewer sees price/ranking update without page refresh; no disconnects in the completed 1000-WS run. | Official bonus is 1000+ single room, so 1000 local pass is meaningful. 10k headline still unproven; PTS multi-IP or controlled local generator needed. | 1000+ bonus pass locally; 2000/10k capacity unproven. |
| S4 fault resilience | 200 active local bidders loop snapshot -> bid -> sleep ~1s for 25s; inject 5s Redis/Kafka/PG/backend/Toxiproxy fault; verify RTO/RPO/convergence. | Core infra fails during live bidding. | Redis 2s RTO / 19s convergence; settlement crash 2s / 17s, 1200 failed attempts, 0 duplicate settlement; PG 3s / 19s with 1000 decisions during PG fault; Kafka 16s bidder RTO / 33s full convergence; Redis FLUSHALL 2s / 20s; Redis+Kafka 11s / 28s; Redis timeout 3s / 19s. RPO=0, no admission contamination. | RTO gate = when new bid attempts are safe again. Full convergence = when payment, final winner, and finance export may open. RPO=0 = no accepted durable bid lost or duplicated. | Redis fault: user sees recovering/retry, no fake accept. PG/Kafka fault: user can still bid, but payment/finality stays "settlement confirming" until convergence. Backend crash: repeated failed attempts until restart. | Single-node local evidence, not Kafka RF=3/Redis Sentinel/Postgres HA. Kafka 16s is local single-broker readiness + drain, not production leader-election proof. | Strong functional chaos pass; add backpressure/redelivery/browser CTA gates for senior review. |
| S5 reconnect recovery | k6 users connect, intentionally miss >=2 public seqs, reconnect with stale `last_seq`, measure TTCS and gaps/dups; Toxiproxy reset_peer mode. | Mobile viewer/bidder loses network during active bidding and reconnects. | Clean 20 VU: 560 recovered, TTCS p99 17ms. Clean 100 VU: 3700 recovered, 57ms. Clean 200 VU: 7393 recovered, 104ms. Toxiproxy reset_peer 50 VU: 1450 recovered, 32ms. All have 0 gap, 0 duplicate, 0 error. | TTCS = reconnect start -> reaches current server seq. 104ms means a stale client returns to authoritative current state in ~0.1s locally. | H5 should show connecting/recovering/stale, disable dangerous actions, then show current price/winner after replay/snapshot. | Local single backend, no LB, carrier network, multi-gateway routing, or browser-device E2E. PTS optional unless proving public-network reconnect storm. | Strong local recovery pass; browser weak-network UI gate still needed. |

## 4. Legacy L1-L4 Asset Comparison

| Legacy asset | What it tests | Is it larger/more complex than S1-S5? | Current S1-S5 equivalent | Missing / risk if not aligned | Recommendation |
|---|---|---|---|---|---|
| L1-C1 `pts-1b-contention-burst-1000vu-1m.jmx` | 1000 one-shot final-second bid contention. | Not larger; it is S1's core asset. | S1. | Current review `5D92X7QG` is correctness pass but strict PTS client p99 64ms > 50ms. | Keep as S1 source of truth; rerun or write a current PASS review before final external claim. |
| L1-C0 accepted ladder | 1000 ordered increasing bids, high accept ratio. | Different, not more realistic; it isolates accept-path capacity. | S1 ladder control. | Without it, reviewers may misread low accepted/s in S1-burst as weak capacity. | Keep as optional control; not a headline. |
| L2-P1 bid + WS fanout | Bid burst while thousands of WS viewers are connected. Planned 1000 bid + 8000-9000 WS. | Yes for protocol stacking and viewer scale. | S3 covers fanout; S1 covers bid. | Current S3 local 1000 does not prove bid path and fanout interact at 8000+ WS. Prior PTS L2-P1 reports were harness gaps. | P1/P2 PTS after S1/S3 local clean: run cost variant first, with active WS verified during bid window. |
| L2-P2 bid + reads | 1000 bid VU plus 2000-5000 HTTP readers polling snapshot/leaderboard/history. | Yes; S1/S3 do not include this read interference. | Not fully covered. S2 has bid soak, not high reader mix. | DB pool/cache/CPU read pressure may regress bid p99 or monitor endpoints. | Add P0/P1 local or PTS-light S2-read-interference gate before any "full-stack performance" claim. |
| L2-P3 bid + WS + reads | Combined protocol stack: bid, WS fanout, read traffic. | Yes; closest current partial is report `AB9EX7TG`, but classified harness gap. | Partially overlaps S1/S3/S2. | `AB9EX7TG` had good backend signals but only 987 bids and WS harness drift. | Keep as P1 integration gate; do not claim formal pass yet. |
| L2-P4 steady interactive auction | 2400 WS + 360 bidders + 240 readers, 10 min. | More realistic than pure S2 local for presentation; less clean than k6 open model for offered-rate proof. | Optional S2 PTS chart. | Without it, S2 has local evidence but no polished PTS PDF. | Optional. Run only if needing a polished external chart; not required for correctness. |
| L3-S1 full lifecycle 30min | 30-min auction curve: 5/s -> 20/s -> 80/s -> 500/s final minute, WS viewers, close/settlement checks. | Yes; it adds lifecycle and memory-leak realism. | S2 only approximates steady pressure; S4/S5 cover faults/reconnect separately. | Current docs do not prove full close -> order -> payment timing over a full auction lifecycle. | P1 if time permits; especially valuable for "whole product" review. |
| L3-S2 multi-room isolation | 3 auctions, about 300 bid VU each plus reads. | Yes for room isolation. | S1-S5 mostly one room/auction. | Official brief names room-level WS routing and multi-room isolation; a senior judge will attack `room_main` single-room bias. | P1 local/PTS-light gate: 3 rooms, no cross-room winner/event contamination. |
| L4-M1 full mixed workload | 1000 hot bidders + 1000 WS + 3000 readers + 200 side bidders + 50 monitor, 10 min. | Yes, much more complex. It is production-readiness, not minimum assignment evidence. | No direct S1-S5 equivalent. | Without it, do not claim full production readiness; only claim scenario-specific evidence. | P2. Run after S1/S2/S3/S4/S5 and L2/L3 gates pass; likely PTS/ECS only. |

Interpretation: L1-L4 are not "better" than S1-S5; they are lower-level and
integration assets. S1-S5 should remain the explanation spine, while missing
L2/L3/L4 dimensions become explicit backlog gates.

## 5. Chaos Script Comparison

| Chaos script | Scenario | Covered by S4/S5 now? | Difference / missing proof | Priority |
|---|---|---|---|---|
| `01-redis-unavailable.sh` | Redis down before decision -> fail closed. | Yes, S4 Redis SIGKILL. | Current S4 is stronger because it runs 200 active bidders and verifier gates. | Done |
| `02-redis-state-flush.sh` | Redis state loss -> rebuild. | Yes, S4 Redis FLUSHALL. | Current S4 documents checkpoint/current-PG rebuild and caveats. | Done |
| `03-kafka-relay-down.sh` | Kafka down during relay, hot path unaffected. | Yes, S4 Kafka SIGKILL. | Need keep payment/finality convergence gate visible; current single broker is not HA. | Done/P1 HA extension |
| `04-settlement-crash.sh` | Worker crash -> Kafka replay idempotently. | Yes, S4 backend/settlement crash. | Current local backend and worker share process; a separate worker-only crash would be cleaner. | P1 refinement |
| `05-pg-unavailable.sh` | PG down, live engine continues, orders wait. | Yes, S4 PG SIGKILL. | Add browser/payment CTA E2E during PG/Kafka lag. | P1 |
| `06-reconnect-storm.sh` | WS disconnect/reconnect storm -> snapshot/diff recovery. | Yes, S5 stronger via stale `last_seq` and TTCS. | Browser UI visibility still not fully proven. | P1 |
| `07-relay-backpressure.sh` | Redis decision stream exceeds relay batch ceiling; drain/alert/no silent queue. | Now explicitly covered by focused S4 gate. | Repaired script runs `TestRelayBackpressureDrainsBeyondBatchCeiling`: 600 Redis decisions exceed 512 batch ceiling; stream length 600; first 512 payloads valid; ledger drains 600/600; next relay 0. Remaining gap is production-style backlog-age alerting under larger load. | Done/P1 observability |
| `08-settlement-idempotency.sh` | Kafka redelivers same decision 3x -> exactly one settlement/order/outbox. | Now explicitly covered by focused S4 gate. | Repaired script runs `TestKafkaSettlementTripleDuplicateMessageHasSingleBusinessEffect`: same SOLD ledger message settled 3x -> one settlement, one bid, one order, one outbox event/delivery, zero duplicate deliveries. | Done |
| `toxiproxy-scenarios.json` Redis latency/timeout, Postgres latency | Partial network failures and slow dependencies. | Redis timeout is covered by S4 partial; WS reset_peer by S5. | Redis latency and PG latency under current Redis-engine wording are still useful, but the JSON has stale "PG-authoritative" language. | P1 update wording + run selected cases |

## 6. PTS Decision

| Candidate | Should use PTS? | Why | Recommended next step |
|---|---|---|---|
| S1 final-second contention | Yes | Official performance claim needs distributed source IPs and a clean exportable p99 chart; current strict client p99 review is not a clean <=50ms pass. | Rerun S1 after warmed/session/JMX decision; classify with `pts-run-review-template.md`. |
| S2 steady soak | No for required proof; optional for chart | k6 open arrival-rate is technically stronger for sustained offered rate and dropped iterations. PTS chart is presentation value. | First rerun local S2 with <=110s convergence gate. PTS only after local pass. |
| S3 1000+ fanout | Yes for external headline; local is enough for internal 1000 pass | PTS multi-IP avoids local generator/source-port ambiguity and gives clean WS p99 chart. | Run S3 cost variant: 2000 WS PTS + local 10k hold if resources permit. |
| S4 fault | No | Fault correctness is structural: SIGKILL/Toxiproxy, RPO, duplicate settlement, pending/lag/outbox convergence. PTS distributed IPs do not add much. | P0 backpressure and redelivery gates are now local PASS; next is browser payment/finality UI proof. |
| S5 reconnect | No unless public-network story | k6 directly checks stale `last_seq`, TTCS, gaps, duplicates. | Add browser weak-network E2E; PTS only for public-network/multi-IP reconnect chart. |
| L2/L3/L4 combined | Only after local gates | These are expensive and harder to debug; PTS is useful once lower-layer gates are clean. | L2-P2 read interference local first, then L2-P3/L4 PTS if claiming production readiness. |

Cost note: Alibaba PTS VUM economics make S1 cheap (about 1000-2000 VUM) and
large WS/headline runs expensive (10k-50k VUM+). This is why S3 cost variant is
preferable before a 10k PTS headline.

## 7. Prioritized Alignment Plan

### P0: must add before a harsh senior review

| Gate | Exact proof required | Suggested asset |
|---|---|---|
| S1 current PASS review | A current report with 1000 unique final `ENGINE_*` decisions, correctness PASS, and strict decision p99 boundary explicitly classified. | Rerun `pts-1b-contention-burst-1000vu-1m.jmx` and write review. |
| Payment/finality UI gate | During Kafka/PG lag or open settlement/outbox, H5 payment CTA/final confirmation/finance export stay disabled; after convergence they open. | Playwright + S4 backend convergence fixture. |

### Completed P0 alignment from legacy chaos

| Gate | Proof now saved |
|---|---|
| Relay backpressure | `tests/chaos/07-relay-backpressure.sh` passes: 600 decisions, 512 batch ceiling, 600/600 relay drain, cursor prevents duplicate append. |
| Settlement redelivery idempotency | `tests/chaos/08-settlement-idempotency.sh` passes: same SOLD message delivered 3x, one settlement/order/outbox business effect. |

### P1: strong differentiation

| Gate | Why | Suggested asset |
|---|---|---|
| Bid + read interference | Shows full-stack API read paths do not hurt bid latency. | L2-P2 local/PTS-light. |
| Multi-room isolation | Directly answers room-level routing and no cross-contamination. | L3-S2 local first; PTS if stable. |
| Full lifecycle 30min | Proves extension, close, order, payment gate, settlement, and no leak over a realistic auction. | L3-S1 local/ECS. |
| Browser weak-network reconnect | S5 backend is strong; browser UX needs visible proof. | Playwright network close/offline/online test. |

### P2: production-readiness, not needed for minimum claim

| Gate | Why | Suggested asset |
|---|---|---|
| L4-M1 full mixed PTS | Validates emergent interactions among bid, WS, reads, side auctions, monitor. | Run only after L2/L3 pass. |
| Kafka 3-broker HA chaos | Converts S4 from functional single-broker proof to real broker-loss proof. | `infra/docker-compose.kafka-production-example.yml` + RF=3/minISR=2/acks=all profile. |
| Redis Sentinel/managed-HA chaos | Converts Redis fail-closed proof into HA failover proof. | Sentinel/managed Redis test: primary kill, client reconnect, no unsafe accept. |

## 8. Judge-Safe Claim Boundaries

Use these exact boundaries:

| Claim | Safe wording |
|---|---|
| Current S3 | "1000 single-room local WS pass: 60s hold, 301 accepted updates, 276k receive samples, fanout p99 22ms, 0 viewer errors. 2000/10k not yet proven." |
| Current S4 | "Local single-node S4 proves fail-closed/replay/convergence: RPO=0, no duplicate settlement, bidder RTO 2-16s, full convergence worst 33s. It is not Kafka/Redis production HA evidence." |
| Kafka 16s | "This is local single-broker readiness plus relay/settlement drain. Production HA path is RF=3, minISR=2, acks=all, unclean election disabled; not yet measured here." |
| Redis FLUSHALL 2s | "This proves one-auction local checkpoint/current-state rebuild, not that production cache rebuild is universally 2s." |
| S2 | "Foreground decision latency is healthy; payment/finality waits for settlement convergence, and the current <=110s convergence target still needs a pass run." |
| PTS | "Use PTS where source-IP distribution and exportable p99 charts answer the question. Do not use PTS as a substitute for correctness/verifier gates." |

## References

- AWS Well-Architected Reliability Pillar, RTO/RPO definitions:
  <https://docs.aws.amazon.com/wellarchitected/latest/reliability-pillar/disaster-recovery-dr-objectives.html>
- Grafana k6 `ramping-arrival-rate`, open-model executor:
  <https://grafana.com/docs/k6/latest/using-k6/scenarios/executors/ramping-arrival-rate/>
- Grafana k6 arrival-rate VU allocation:
  <https://grafana.com/docs/k6/latest/using-k6/scenarios/concepts/arrival-rate-vu-allocation/>
- Alibaba Cloud PTS VUM pricing and IP/sampling cost model:
  <https://www.alibabacloud.com/help/en/pts/performance-test-pts-3-0/product-overview/pay-as-you-go>
- Apache Kafka producer configs, `acks=all` and idempotence:
  <https://kafka.apache.org/41/configuration/producer-configs>
- Apache Kafka broker/topic configs, `min.insync.replicas` and unclean leader election:
  <https://kafka.apache.org/41/configuration/broker-configs>
- Redis Sentinel high availability:
  <https://redis.io/docs/latest/operate/oss_and_stack/management/sentinel/>
- Redis high availability guide and `WAIT` discussion:
  <https://redis.io/tutorials/operate/redis-at-scale/high-availability/>
- AWS Architecture Blog, exponential backoff and jitter:
  <https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/>
- Toxiproxy TCP fault injection:
  <https://github.com/Shopify/toxiproxy>
