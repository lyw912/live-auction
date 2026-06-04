# S5 Judge Defense — Reconnect Recovery

## One-Sentence Claim

S5 proves that a user who disconnects during active bidding can reconnect with a
stale `last_seq` and return to the authoritative auction state within
sub-second TTCS, with zero lost/duplicate seqs and no local winner/price guess.
Production expansion for multi-gateway reconnect, LB/NAT idle timeout, mobile
weak networks, and cross-AZ/region failures is tracked in
[production-ha-expansion-and-judge-defense.md](production-ha-expansion-and-judge-defense.md).

## Explain It To A ByteDance Reviewer

Workload:

- Real PTS session tokens from `s1-s5-1000-user-sessions.csv`.
- One active hot auction `auc_live`.
- A low-rate accepted-bid source advances public seq by bidding current price +
  increment.
- Reconnect users first connect, close, wait until they missed at least 2 public
  seqs, then reconnect with the stale `last_seq`.

User view:

- The socket drops or is closed.
- H5 enters connecting/recovering/stale state.
- Bid and max-bid dangerous actions are disabled during uncertain state.
- Reconnect issues a fresh one-use WS ticket.
- Server replays history or sends snapshot.
- User sees current server price/winner again.

Current numbers:

| Scenario | Scale | User meaning | Result |
|---|---:|---|---|
| clean reconnect | 200 VU for 2m | local reconnect storm proof | `s5-20260604T221312`: 34,814 recovered, TTCS p99 87 ms, 0 gap/dup/error/truth-mismatch |
| Toxiproxy reset_peer | 50 VU for 2m | reconnect leg through real proxy-path reset turbulence | `s5-20260604T231925`: 8,849 recovered, TTCS p99 341 ms, 0 gap/dup/error/truth-mismatch; reconnect retries=3,826, so the proxy fault was active |

Server-side recovery monitor for the same network run recorded
`ws_reconnect=21,574`, `ws_recovered(history)=16,584`,
`ws_recovered(db)=4,913`, and `ws_recovered(snapshot_unavailable)=77`, with
readyz still healthy after the run.

Pressure reached the target because the evidence lines up at three layers:
k6 created stale-`last_seq` recovery iterations, the reconnect leg recorded
3,826 retry/attempt errors through Toxiproxy, and the server recorded
`ws_reconnect`/`ws_recovered` events across history and DB/snapshot paths. The
database is not expected to contain one row per reconnect; the business truth is
the current public seq/state, and S5 verifies recovered clients match it without
gaps, duplicates, or stale winner/price.

## Why This Is Not S3

S3 asks: can stable viewers receive broadcasts fast?

S5 asks: after the connection is gone and the user missed real seqs, can the
system restore the exact current state without gaps or duplicated notifications?

These are different failure modes. A system can pass S3 fanout and still fail S5
if it has no `last_seq` replay, no snapshot fallback, or a UI that keeps showing
stale price as truth.

## Defensible Answers

Q: What exactly does TTCS mean?

A: `reconnect_start -> received current seq N`. The test first records the old
`last_seq=K`, waits until the server public seq has advanced by at least 3, then
measures how long the reconnect takes to reach `N`. In the 200 VU local clean
run, TTCS p99 was 87 ms.

Q: What does "0 gap" mean?

A: For incremental recovery, received seqs after reconnect are sorted and checked
for missing or repeated seq numbers. If history cannot prove continuity, the
backend falls back to snapshot rather than silently skipping.

Q: What is the real business incident?

A: A viewer or bidder loses network during a live auction and misses price
changes. The bad outcome would be showing stale winner/price or enabling a bid
against unknown state. Current H5 marks recovery/stale phases and disables
dangerous actions until server state is restored.

Q: Why are there skipped iterations?

A: Skipped means the accepted-bid source did not create enough new public seq
inside the wait window for that iteration. No gap was created, so it is not a
reconnect failure. Pass/fail is computed on iterations where the client actually
missed seqs and reconnected.

Q: Does Toxiproxy prove mobile weak network?

A: It is a current backend reconnect pass for a controlled TCP reset path, not a
mobile-network certification. The accepted 2026-06-04 run used
`ws://127.0.0.1:18081` through Toxiproxy for the reconnect recovery leg and
recorded 3,826 reconnect attempt errors/retries, yet all 8,849 recovery
iterations caught up with zero gaps, duplicates, truth mismatch, or final
recovery errors. It still does not replace browser/device weak-network E2E, NAT
timeout, LB idle-timeout, or carrier packet-loss testing.

Q: Why not PTS for S5?

A: PTS is useful for distributed IP charts and socket-scale optics. S5's core
claim is correctness of `last_seq` recovery: no gap, no duplicate, no stale
truth. The local k6 harness directly asserts those invariants. Since 200 VU local
clean p99 is 87 ms against a 2 s target, PTS is optional unless the claim changes
to public-network reconnect storms.

Q: What would a production reviewer still attack?

A:

- No multi-backend gateway test with reconnect landing on a different instance.
- No real LB idle timeout / sticky routing evidence.
- No Playwright browser weak-network E2E proving visible CTA disabled state under
  actual H5 socket close.
- No mobile radio/network emulator evidence.
- Single local Redis/Kafka/PG means this is not an HA topology benchmark.

## Production Design Path

- WebSocket gateways sharded by `room_id`, with recovery data in shared
  Redis/outbox so reconnect is not pinned to the original process.
- Reconnect backoff with jitter to avoid thundering herd.
- History retention sized for peak reconnect window; snapshot fallback when
  history is missing.
- Snapshot rebuild singleflight and semaphore to cap storm load.
- H5 stale-state UX tests for bid CTA, max-bid CTA, final winner, and payment CTA.
- Production Kafka/Redis HA handled as S4 expansion: Kafka 3 brokers RF=3
  minISR=2 `acks=all`; Redis HA with managed failover/Sentinel and explicit
  fail-closed behavior.
- Full production HA and weak-network drilldowns are documented in
  [production-ha-expansion-and-judge-defense.md](production-ha-expansion-and-judge-defense.md),
  including multi-gateway, LB/NAT idle-timeout, mobile radio, and AZ/region
  failure test gates.

## Current Verdict

S5 clean reconnect is a strong local pass for reconnect recovery correctness and
single-node reconnect storm behavior. S5 network/reset-peer is also a current
local pass for backend reconnect recovery through a controlled Toxiproxy reset
path. It is not yet a production weak-network certification because browser,
mobile carrier, LB idle-timeout, and multi-gateway landing tests remain P1
follow-ups.

## References Used For The Defense

- Grafana k6 WebSocket APIs: <https://grafana.com/docs/k6/latest/javascript-api/k6-ws/connect/>
  and <https://grafana.com/docs/k6/latest/javascript-api/k6-websockets/>
- Toxiproxy TCP fault injection: <https://github.com/Shopify/toxiproxy>
- MDN WebSocket close event: <https://developer.mozilla.org/en-US/docs/Web/API/WebSocket/close_event>
- AWS exponential backoff and jitter: <https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/>
