# S5 — 断连重连 / Reconnect & Recovery

> Maps to: brief 挑战二 "WebSocket 连接稳定，即使网络波动也能自动重连" + "心跳保活".
> Headline: **time-to-current-state** after reconnect + no lost/duplicate notifications.
> Tool: **local k6 + Toxiproxy** (0 VUM). Source asset:
> `tests/load/s5-reconnect-recovery.js`. Tier: **P2**.

## 1. The business moment

A viewer on mobile loses signal mid-auction. The WebSocket drops. The client must
reconnect automatically, and — critically — **catch up to the true current state**
(price, winner, ranking, terminal status) without showing stale or fabricated
data, and without missing or double-showing a "you were outbid" notification.
This proves the realtime layer is honest under the weak-network reality the brief
calls out.

## 2. What it proves that S3 does not

S3 measures fanout latency to *stable* connections. S5 measures **recovery**: when
a connection returns with a stale `last_seq`, how fast and how correctly does it
reach the current state? The recovery source matters:

```
reconnect with last_seq = K, current public seq = N
  if N − K small  → incremental outbox replay of seqs (K+1 .. N)
  if N − K large / gap unprovable → snapshot rebuild, then resume from snapshot seq
```

## 3. Script logic (local k6 + Toxiproxy)

```
1. cohort of viewers connect, receive price updates, track highest seq seen
2. inject disconnect:
     - client-driven: close the socket after seq K   (clean reconnect)
     - network-driven: Toxiproxy `reset_peer` on the WS port at toxicity≈30%  (weak-network)
3. reconnect: re-issue ws-ticket, reconnect, send last_seq=K
4. MEASURE time-to-current-state = reconnect_start → received current seq N with no gap
5. ASSERT: received every seq in (K, N] exactly once; no duplicate notification;
           UI-truth fields (price/winner/terminal) match server snapshot
```

Reconnect storm variant (the stress): disconnect a large fraction at once and
reconnect together — watch whether **snapshot rebuild saturates** (CPU, Redis
reads) or stays bounded.

## 4. Metrics

| Metric | Definition | Target |
|---|---|---|
| time-to-current-state p99 | reconnect → caught up to current seq | ≤ 1–2 s (state target) |
| recovery source distribution | % incremental replay vs snapshot rebuild | mostly incremental for small gaps |
| lost/duplicate notifications | gaps or repeats in the seq stream after recovery | **0** |
| snapshot rebuild rate under storm | rebuilds/s, Redis read load | bounded, no saturation |

## 5. Pitfalls

- **Client fabricating state during the gap.** While recovering, the UI must show
  a bounded "reconnecting/stale" state and disable dangerous bid actions — never
  show a locally-guessed winner/price. Assert this.
- **Snapshot rebuild storm.** A reconnect wave that all triggers full snapshot
  rebuilds can saturate the node; prefer incremental replay for small gaps and
  bound rebuild concurrency.
- **Heartbeat detection time counted as outage.** Separate "time to detect the
  drop" (heartbeat: 20 s ping + 5 s timeout) from "time to recover after
  reconnect"; report both.
- **Ordering.** Replayed seqs must arrive in order; a gap that cannot be proven
  filled must escalate to snapshot, not be silently skipped.
