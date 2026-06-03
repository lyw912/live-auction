# S5 Judge Defense — Reconnect Recovery

## One-Sentence Claim

S5 proves that a user who disconnects during active bidding can reconnect with a
stale `last_seq` and return to the authoritative auction state within
sub-second TTCS, with zero lost/duplicate seqs and no local winner/price guess.

## Explain It To A ByteDance Reviewer

Workload:

- Real PTS session tokens from `pts-1ab-1000vu-sessions.csv`.
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
| clean reconnect | 20 VU | small room recovery smoke | 560 recovered, TTCS p99 17 ms, 0 gap/dup/error |
| clean reconnect | 100 VU | reconnect storm baseline | 3700 recovered, TTCS p99 57 ms, 0 gap/dup/error |
| clean reconnect | 200 VU | local bottleneck probe | 7393 recovered, TTCS p99 104 ms, 0 gap/dup/error |
| Toxiproxy reset_peer | 50 VU | weak-network TCP reset proof | 1450 recovered, TTCS p99 32 ms, 0 gap/dup/error |

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
`last_seq=K`, waits until the server public seq has advanced by at least 2, then
measures how long the reconnect takes to reach `N`. In the 200 VU local run,
TTCS p99 was 104 ms.

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

A: Partially. It proves a controlled TCP `reset_peer` on the WebSocket path does
not break recovery correctness. It does not replace browser/device weak-network
E2E, NAT timeout, LB idle-timeout, or carrier packet-loss testing.

Q: Why not PTS for S5?

A: PTS is useful for distributed IP charts and socket-scale optics. S5's core
claim is correctness of `last_seq` recovery: no gap, no duplicate, no stale
truth. The local k6 harness directly asserts those invariants. Since 200 VU local
p99 is 104 ms against a 2 s target, PTS is optional unless the claim changes to
public-network reconnect storms.

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

## Current Verdict

S5 is a strong local pass for reconnect recovery correctness and single-node
reconnect storm behavior. It is not yet a production weak-network certification.

## References Used For The Defense

- Grafana k6 WebSocket APIs: <https://grafana.com/docs/k6/latest/javascript-api/k6-ws/connect/>
  and <https://grafana.com/docs/k6/latest/javascript-api/k6-websockets/>
- Toxiproxy TCP fault injection: <https://github.com/Shopify/toxiproxy>
- MDN WebSocket close event: <https://developer.mozilla.org/en-US/docs/Web/API/WebSocket/close_event>
- AWS exponential backoff and jitter: <https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/>
