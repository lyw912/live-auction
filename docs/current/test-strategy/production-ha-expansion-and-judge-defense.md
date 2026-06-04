# Production HA Expansion And Judge Defense

> Status: judge-defense expansion plan, 2026-06-04.
> Scope: how to defend S4/S5 results when reviewers ask about production HA:
> Kafka RF=3/minISR=2, Redis HA failover, multi WebSocket gateways, LB/NAT idle
> timeout, real mobile weak networks, and cross-AZ/cross-region failures.
>
> This is not a claim that the current local S4/S5 runs already prove production
> HA. It is the concrete scale-out and failure-contract plan that turns the
> current single-node evidence into falsifiable next tests.

## 1. The Honest Framing

Current S4/S5 evidence is strong in two ways:

- S4 proves the local auction safety contract under bounded dependency faults:
  Redis truth loss fails closed, PG/Kafka faults do not fabricate accepted bids,
  Kafka replay is settlement-idempotent, and finality/payment waits for
  convergence.
- S5 proves local reconnect correctness: a client reconnecting with stale
  `last_seq` can recover to authoritative state with zero gaps, duplicates, or
  truth mismatch, including a controlled Toxiproxy reset path.

What it does not yet prove:

- a three-broker Kafka cluster leader failover with RF=3/minISR=2;
- Redis Sentinel/managed-Redis primary failover, old-primary return, or
  split-brain fencing;
- reconnect landing on a different WebSocket gateway behind a real LB;
- ALB/NLB/NAT idle-timeout behavior for long-lived sockets;
- browser/device weak-network behavior on real H5;
- AZ isolation or cross-region disaster recovery.

The judge answer should be:

> "S4/S5 are current local correctness and recovery proofs. Production HA is a
> separate topology proof. The design path is not 'add more machines'; it has
> specific quorum, fencing, replay, heartbeat, and user-visible fail-closed
> contracts, and here is the exact test matrix I would run next."

## 2. Industrial Interpretation Of Current Numbers

The useful industrial reading is not "we are production HA already." It is:

| Evidence | Current number | Industrial meaning | Boundary |
|---|---:|---|---|
| S4 P0/P1 bidder RTO | 2s / 14s / 2s / 12s / 2s / 12s | A bidder either gets safe decisions or safe paused/error feedback within a short incident window. | local single-node dependency faults |
| S4 final convergence | worst current 29s | Back-office Kafka/Redis/PG/outbox drains before payment/finality opens. | not a multi-broker or multi-AZ SLA |
| S4 P2 Redis partial network | RTO 8s, final convergence 25s | Controlled Redis proxy-path connection reset is visible to clients as fail-closed `ENGINE_PAUSED`, not fake accepts. | not a full Redis Sentinel failover/split-brain test |
| S5 clean reconnect | TTCS p99 87ms, 34,814 recoveries | Local recovery path is comfortably sub-second under 200 reconnect VU. | no real LB/mobile/browser yet |
| S5 proxy reconnect | TTCS p99 341ms, 8,849 recoveries, 3,826 retries | Reset turbulence is active and the backend still recovers without gap/dup/truth mismatch. | controlled TCP reset, not carrier weak-network certification |

For a live auction business, the most important part is RPO/business safety:

- no phantom accepted bid during Redis truth loss;
- no duplicate settlement/order/outbox effect under Kafka redelivery;
- no payment CTA before settlement/outbox convergence;
- no stale price/winner treated as truth after WebSocket reconnect.

Sub-second reconnect TTCS and sub-30s bounded convergence are good local
engineering results, but they should be presented as local evidence plus a
production follow-up plan, not as a universal SLA.

## 3. Kafka RF=3 / minISR=2

### Production Topology

Target Kafka topology for the decision ledger:

- three brokers across independent failure domains;
- decision topics with `replication.factor=3`;
- topic or broker `min.insync.replicas=2`;
- producer `acks=all`;
- `unclean.leader.election.enable=false`;
- key by `auction_id` so one auction's decision order remains in one partition;
- consumer-group parallelism scales with partitions, not with one hot auction.

The point of RF=3/minISR=2 is not higher throughput. It changes the durability
contract:

- If one broker dies and at least two in-sync replicas remain, Kafka can still
  acknowledge durable writes.
- If ISR falls below two, the producer must fail rather than acknowledge a
  decision that is not durably replicated.
- If a stale replica is the only available replica, unclean election must stay
  disabled because electing it may lose acknowledged records and break auction
  order.

### Auction Contract

The business contract should be explicit:

- Redis may continue to generate foreground engine decisions while Kafka is
  temporarily unavailable only if those decisions remain in Redis pending relay
  and payment/finality is blocked.
- A bid must not be shown as financially final until Kafka lag, Redis pending,
  PostgreSQL settlement, and outbox gates converge.
- If the product wants Kafka durability before even returning `ENGINE_ACCEPTED`,
  then the foreground path must fail closed when Kafka ISR < minISR. That is a
  stricter latency/durability tradeoff and should be chosen deliberately.

Current implementation posture is closer to:

> "Foreground Redis decisions are fast; Kafka/settlement finality is enforced by
> convergence gates before payment or finance confirmation."

That is defensible only because payment convergence is now a system gate.

### Judge Drilldowns

Q: What happens if broker 2 is down?

A: With RF=3/minISR=2 and `acks=all`, writes can still succeed if leader and one
follower are in ISR. The accepted business effect is not final until the
settlement pipeline drains.

Q: What happens if two brokers are down or ISR shrinks below two?

A: Producer append fails with a durability exception. We must either keep Redis
decisions pending and block finality, or fail closed in the foreground if the
product requires Kafka-durable-before-response. We must not call a bid
financially final from a single non-quorum Kafka copy.

Q: Why not key one auction across many Kafka partitions?

A: A single auction needs total order for `engine_seq`, winner, current price,
and idempotency. Kafka partitions only preserve order per partition. Splitting
one auction across partitions would require a second sequencer to reassemble
order, which recreates the same single-writer problem with more failure modes.

### Required Next Tests

| Test | Fault | Expected result | Evidence gate |
|---|---|---|---|
| Kafka follower loss | kill one non-leader broker | producer continues; no visible bidder fault; lag drains | no lost `(auction_id, epoch, engine_seq)` |
| Kafka leader loss | kill leader broker | leader election, short append stall, then drain | RTO, lag 0, verifier PASS |
| ISR below minISR | partition or stop two brokers | producer cannot claim durable append | no payment/finality; clear alert |
| Unclean election guard | stale replica remains | stale replica is not elected | no seq rollback, no missing accepted |
| Consumer rebalance replay | restart settlement workers during load | duplicate Kafka delivery creates one business effect | one settlement/order/outbox effect |

## 4. Redis HA Failover And Split-Brain Fencing

### Production Topology

Redis is the hot decision truth. In production it needs either managed Redis HA
or a Sentinel/Cluster design:

- at least three Sentinel processes or provider-managed equivalent;
- Redis primary plus replicas across independent failure domains;
- client support for failover discovery and reconnect;
- engine fencing/epoch to prevent old primary writes from being accepted after
  a failover;
- persistence and checkpoint policy aligned with acceptable RPO.

Redis replication is asynchronous by default. Sentinel can promote a replica and
tell clients the new primary address, but it does not magically turn Redis into
a strongly consistent CP database. That matters for auctions.

### Auction Contract

The safe auction stance is:

- On Redis primary loss, role change, `READONLY`, reconnect churn, or unknown
  epoch, the engine enters `ENGINE_PAUSED` or `RECONCILING`.
- The UI disables bid/max-bid/payment dangerous actions during
  paused/reconciling state.
- A Redis primary can accept bid writes only when its engine epoch/fencing token
  matches the current durable checkpoint.
- An old primary returning after isolation must be treated as stale; its writes
  are rejected or discarded unless it has rejoined as a replica under the new
  epoch.
- If Redis hot state is missing but durable PG/Kafka history exists, cold seed is
  refused and controlled resume/reconcile is required. This is already aligned
  with the current S4 Redis FLUSHALL fix.

### Judge Drilldowns

Q: If Sentinel promotes a replica, can a write acknowledged by old primary be
lost?

A: Yes, Redis replication is asynchronous. The product response is not to pretend
Redis HA gives RPO=0 by itself. We either use `WAIT`/persistence to reduce the
window for selected writes, or, more importantly for this architecture, we keep
Kafka/PG settlement/finality gates and fail-closed/reconcile behavior. Payment
does not open until durable convergence is proven.

Q: What if the old primary comes back and still accepts writes?

A: Every engine write carries an epoch/fencing token. The stale primary's epoch
is lower or unknown, so the app rejects it and alerts. Rejoin requires wiping or
resyncing it as a replica of the new primary.

Q: What if Redis Cluster moves a key during an auction?

A: One auction's engine keys must stay in one hash slot via a hash tag, for
example `{auction:auc_live}:state`, `{auction:auc_live}:stream`, and related
keys. Cross-slot Lua would break the single-writer engine contract.

### Required Next Tests

| Test | Fault | Expected result | Evidence gate |
|---|---|---|---|
| Primary kill | stop Redis primary | `ENGINE_PAUSED` until new primary usable | no accepted during unknown truth window |
| Primary isolation | partition app from primary | fail closed, no dual writer | accepted-in-window=0 |
| Sentinel failover | promote replica | client reconnects and resumes with new epoch | no seq rollback |
| Old primary return | restore old primary | stale epoch rejected | no accepted from old primary |
| Hot-state loss | FLUSHALL/empty new primary | reconciling from checkpoint or fail closed | verifier PASS, no reseed from stale state |

## 5. Multiple WebSocket Gateways

### Production Topology

WebSocket gateways should be horizontally scalable and mostly stateless for
recovery:

- LB routes clients to multiple gateway instances;
- gateway owns live socket fanout and per-connection backpressure;
- Redis/Kafka/outbox/snapshot stores recovery truth;
- reconnect can land on any gateway with a fresh one-use ticket;
- `last_seq` recovery uses shared history first and snapshot fallback second;
- per-room fanout subscriptions are room-sharded to avoid every node receiving
  every room's traffic.

Sticky sessions can reduce churn, but correctness must not depend on stickiness.
A reconnect to gateway B after gateway A dies must still recover.

### Gateway Guardrails

- Per-room queue bounds and slow-consumer close policy.
- Snapshot rebuild singleflight so 10,000 reconnects do not trigger 10,000 DB
  rebuilds.
- Semaphore/admission limit for expensive recovery paths.
- Retry-after and jitter for reconnect storms.
- Presence cleanup on `CloseRead`/close/error, not only idle timeout.
- Same auth/ACL/ticket validation on every gateway.

### Judge Drilldowns

Q: What if 100k clients reconnect at once after a gateway crash?

A: The backend must treat reconnect as load, not free. Clients reconnect with
exponential backoff and jitter. Gateways cap concurrent snapshot rebuilds; shared
history handles most recent `last_seq` recoveries; old `last_seq` requests get a
snapshot; if the cap is hit, clients receive retry-after/recovering UI rather
than stale truth.

Q: What if reconnect history is already evicted?

A: The server must not claim continuity. It returns snapshot recovery with a
snapshot sequence at or above current state. Client marks recovered only when the
authoritative snapshot is applied.

### Required Next Tests

| Test | Fault | Expected result | Evidence gate |
|---|---|---|---|
| Gateway A kill | clients connected to A reconnect to B | TTCS within target | no gap/dup/truth mismatch |
| History retention miss | reconnect with old `last_seq` | snapshot fallback | current seq applied, no false gap-free claim |
| Reconnect storm | 10k clients reconnect | bounded recovery load | p99 TTCS, no DB pool collapse |
| Slow clients | clients stop reading | bounded queue and close | no room-wide fanout lag |
| Cross-gateway fanout | viewers split across A/B/C | same ordered updates | per-gateway receive p99 |

## 6. LB/NAT Idle Timeout

### Production Risk

Long-lived WebSocket connections can be closed by infrastructure even when the
application is healthy:

- AWS ALB supports WebSockets, but the connection idle timeout applies to those
  connections.
- ALB default idle timeout is 60s and can be configured.
- AWS NAT Gateway drops idle TCP connections after 350s and returns RST on later
  use.
- Mobile and carrier NATs can have their own shorter or silent timeouts.

The app should not rely on "the socket will stay open." It should prove liveness.

### Required Contract

- Server/client heartbeat interval must be below the shortest known idle timeout
  with margin, for example 20-30s for a 60s ALB default.
- Heartbeat data must actually reset the relevant idle timer; HTTP/2 PING does
  not reset ALB idle timeout, so WebSocket ping/pong or application frames are
  safer.
- If heartbeat fails or the close event fires, H5 enters stale/recovering state.
- Bid, max-bid, final winner, and payment CTA are blocked while state is stale.
- Reconnect uses a fresh WS ticket and jittered backoff.

### Required Next Tests

| Test | Fault | Expected result | Evidence gate |
|---|---|---|---|
| ALB idle timeout below heartbeat | set idle timeout low | socket closes predictably | reconnect recovers, UI stale before recovered |
| Heartbeat disabled | no app frames | LB/NAT closes idle socket | no stale bid CTA |
| NAT idle timeout | idle > 350s path | RST/close detected | reconnect + `last_seq` recovery |
| Server drain | deregister target | clients reconnect elsewhere | no gap/dup |

## 7. Real Mobile Weak Network

### What Toxiproxy Covers And Does Not Cover

Toxiproxy reset/timeout covers controlled TCP-level disruption. It is useful
because it is deterministic and repeatable. It does not cover all mobile
behavior:

- high latency and jitter;
- packet loss;
- bandwidth collapse;
- radio handoff between Wi-Fi/4G/5G;
- captive portal or DNS failures;
- background/foreground app lifecycle;
- browser-specific WebSocket close semantics.

### Product Contract

The H5 must be conservative:

- Display server-authoritative price/winner only.
- Mark state as stale/recovering when WebSocket liveness is unknown.
- Disable bid/max-bid/payment CTAs during stale/recovering/finality-blocked
  phases.
- Never infer winning/payment eligibility from a local optimistic update.
- On reconnect, recover by `last_seq`; if history is insufficient, apply
  snapshot and only then re-enable actions.

### Required Next Tests

| Test | Tool | Expected result | Evidence gate |
|---|---|---|---|
| Browser offline/online | Playwright browser context | visible stale state, then recover | CTA disabled while stale |
| Latency/jitter/loss | `tc netem` or provider network emulator | no gap/dup, bounded TTCS | p99 TTCS under profile |
| Mobile radio handoff | real device/emulator | reconnect with fresh ticket | no stale price/winner |
| Background resume | mobile browser/app lifecycle | resume triggers snapshot/replay | current state before CTA |
| Captive/DNS failure | DNS/proxy block | clear recovering/error state | no silent stale room |

## 8. Cross-AZ And Cross-Region Failures

### In-Region Multi-AZ

The realistic production first step is multi-AZ within one region:

- Kafka brokers spread across AZs with RF=3/minISR=2;
- Redis HA primary/replicas/Sentinel or managed Redis across AZs;
- PostgreSQL managed HA or primary/standby failover;
- multiple app/gateway nodes registered behind LB;
- outbox/settlement workers horizontally scaled by Kafka partition.

Failure stance:

- If one AZ is isolated but quorum remains, keep serving through healthy AZs.
- If quorum is lost for Kafka/Redis truth/finality, freeze or fail closed.
- If PG fails but Redis/Kafka hot path is safe, live bidding may continue while
  payment/finality remains blocked until PG recovery and convergence.

### Cross-Region

Cross-region live auction is harder because most cross-region replication is
asynchronous. The default safe position:

- Active-passive region failover for disaster recovery.
- Do not accept new bids in two regions for the same auction unless there is a
  single global sequencer/quorum contract.
- During region partition, freeze the auction, show reconciling, optionally
  extend auction end time, and block payment until one authoritative region and
  settlement truth are established.

Active-active cross-region bidding for the same auction is not a simple scale-out
of the current sequencer. It requires a global consensus/sequencer service or a
business rule that partitions auctions by region and never accepts bids for one
auction in two regions simultaneously.

### Required Next Tests

| Test | Fault | Expected result | Evidence gate |
|---|---|---|---|
| One AZ app loss | drain/kill gateway nodes in AZ | reconnect to healthy AZ | no gap/dup |
| Kafka broker AZ loss | kill broker/AZ route | quorum write or explicit fail closed | no acknowledged lost decision |
| Redis primary AZ loss | promote replica | paused/reconciling then resume | no dual writer |
| PG failover | managed DB failover | foreground contract honored, finality blocked | settlement convergence |
| Region isolation | block inter-region links | auction frozen or single region authoritative | no split winners |

## 9. Judge Boundary Questions

| Question | Answer |
|---|---|
| "Are your S4/S5 results production HA?" | No. They are local correctness/recovery evidence. Production HA needs RF=3 Kafka, Redis HA, multiple gateways, LB/NAT, mobile, and AZ tests listed here. |
| "Why should I trust the architecture then?" | Because the safety invariants are already local-testable and the production plan maps each untested topology to a specific failure contract and evidence gate. |
| "What is the most important invariant?" | No wrong winner, no phantom accepted bid, no duplicate order/payment, no stale client truth, and no payment before convergence. |
| "If Kafka ISR is below minISR, do you still return success?" | Not as durable finality. Either foreground fails closed, or Redis decision remains pending and payment/finality is blocked until durable append succeeds. |
| "If Redis HA loses an acknowledged write, what happens?" | Redis alone is not the final finance proof. The system reconciles from durable ledger/checkpoint and blocks finality; unknown or stale Redis epoch fails closed. |
| "Can multi-gateway reconnect depend on stickiness?" | No. Stickiness is an optimization only. `last_seq` recovery must work from shared history/snapshot on any gateway. |
| "What if a client bids while its socket is stale?" | The UI should disable dangerous actions. If a malicious client still posts, server-side engine remains authoritative and validates against current state. |
| "What if 100k reconnects overload snapshot rebuild?" | Backoff+jitter, shared history, snapshot singleflight, recovery semaphore, retry-after, and stale UI. The product degrades to recovering, not stale truth. |
| "What if cross-region links split?" | For one auction, freeze or single-region-authoritative mode. Do not accept bids in two regions without global sequencing/quorum. |

## 10. Source Notes

These are the external facts used in the defense:

- Kafka producer `acks=all` is the producer-side setting used with
  `min.insync.replicas` for stronger durability:
  <https://kafka.apache.org/40/configuration/producer-configs/>
- Kafka `min.insync.replicas` with RF=3/minISR=2/`acks=all` enforces stronger
  durability and raises producer exceptions when the minimum cannot be met:
  <https://kafka.apache.org/42/generated/kafka_config.html>
- Kafka unclean leader election can elect non-ISR replicas and may result in
  data loss, so it must stay disabled for the auction ledger:
  <https://kafka.apache.org/40/configuration/broker-configs/>
- Redis Sentinel monitors, promotes replicas, and provides clients the current
  master address, but robust deployment needs multiple Sentinels and testing:
  <https://redis.io/docs/latest/operate/oss_and_stack/management/sentinel/>
- Redis replication is asynchronous by default; `WAIT` can reduce but not remove
  all failover loss modes:
  <https://redis.io/docs/latest/operate/oss_and_stack/management/replication/>
- AWS ALB supports WebSockets and applies listener/load-balancer options to
  WebSocket connections:
  <https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-listeners.html>
- AWS ALB default idle timeout is 60s; the valid configurable range is 1-4000s,
  and HTTP/2 PING does not reset that timeout:
  <https://docs.aws.amazon.com/elasticloadbalancing/latest/application/edit-load-balancer-attributes.html>
- AWS NAT Gateway drops idle TCP connections after 350s and returns RST on later
  use:
  <https://docs.aws.amazon.com/vpc/latest/userguide/nat-gateway-troubleshooting.html>
- Backoff with jitter is the standard anti-herd reconnect pattern:
  <https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/>
- SLO/SLI thinking should separate user-visible recovery from back-office final
  convergence:
  <https://sre.google/sre-book/service-level-objectives/>
- Chaos experiments should have explicit hypotheses and steady-state invariants:
  <https://principlesofchaos.org/>
