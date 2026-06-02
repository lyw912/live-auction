# S3 — 万人围观 / Single-Room Fanout

> Maps to: brief 加分 "单直播间 1000+ 用户同时在线（超基础 10×）" + "出价数据秒级同步".
> Headline: **M2 fanout publish→receive p99 ≤ 1 s** at the target connection count
> + connections held + RAM/connection.
> Tool: **PTS** (clean per-connection p99 chart) + **local k6** (10 000 soak).
> Source assets: `tests/pts/L2-protocol/pts-2p1-bid-plus-ws-fanout.jmx` and
> `tests/load/s3-fanout-soak.js`; old `L2-*` names are file aliases only.

## 1. The business moment

One hot room, `auc_live`. A small number of people bid; **everyone else watches**
the price move in real time. The bonus criterion is 1000+ concurrent in one room
(10× the 100-user base). The question this scenario answers — and *only* this
scenario answers — is: **when an accepted bid happens, how long until all N
connected viewers see the new price, and how many connections can one node hold?**
Fanout latency is governed by *connection count*, not bid frequency, so this is a
separate workload from S1/S2.

## 2. The chosen headline: 10 000 WS — and the cost-smart variant

Per the scope decision, the headline target is **10 000 WS in one room**. Two
ways to produce the evidence; pick per run:

| Variant | What it produces | Scale | VUM | ≈¥ | When |
|---|---|---|---|---|---|
| **Headline (PTS)** | one PTS report: 10 000 connections held + `ws-fanout-receive` p99 | 10 000 WS ×5 min, 20 IP | 50 000 | 150 | the showcase artifact |
| **Cost variant** | PTS p99 chart at 2 000 WS + local k6 10 000-conn soak (Grafana) | 2 000 WS ×5 min (PTS) + 10 000 local | 10 000 | 30 | when budget matters |

> Honest framing either way: **10 000 real WS + active fanout on a box that also
> runs PG/Redis/Kafka is exactly where the single-node ceiling appears.** If it
> holds p99 ≤ 1 s — great, headline. If it bends, the bottleneck + the
> [scale-out story](scale-out-and-architecture-ceilings.md) (room-sharded gateways
> + sharded pub/sub) is itself the high-scoring answer. Do **not** explain a bend
> as "just buy a bigger box."

Why the cost variant is legitimate: fanout *latency* is a per-connection
measurement; 2 000 connections measure the same publish→receive path cleanly on
PTS, while the 10 000 *hold* (a connection-count + leak question) is what local
k6 proves for free. State that the p99 chart is from the 2 000 run and the
10 000-concurrency proof is the Grafana panel.

## 3. Measuring fanout latency (the part judges probe hardest)

```
server: on each accepted bid, broadcast {type:"price", seq, current_price_cents, published_at_ms}
client: for each received seq, latency = recv_local_ms − published_at_ms
M2 = p99 of latency across all connections × all seqs
```

**Clock skew is the trap, address it explicitly.** `published_at_ms` is the
server clock; `recv_local_ms` is the load-generator clock. Mitigation:
- **PTS:** put pressure IPs in the **same VPC** as the ECS; NTP keeps skew ≪ 1 s,
  so the raw delta is valid against a 1 s target. State this assumption.
- **local k6:** the generator shares the box's clock (or a same-host clock), so
  publish and receive use one monotonic source.

**PTS WebSocket sampler shape** (plugin; each sampler times only its op — see
[pts-playbook §4](pts-playbook.md)):
```
"建立连接 ws-connect"        Open Connection        → handshake p99 (join context)
(send nothing; viewers only watch)
"广播接收 ws-fanout-receive" Single Read, in a loop  → elapsed = wait for ONE broadcast = M2
```
The Single-Read elapsed time *is* the fanout latency; the multi-minute hold never
enters it. Each connection records its worst `MAX_LAT_MS` in `responseMessage` as
a cross-check, but the chart value is the sampler p99.

## 4. Decouple connection count from bid rate

Fanout latency depends on connections, not on how fast bids arrive. So:
```
bidders   : a SMALL fixed source of accepted updates (e.g. 1–10 accepted/s) — not the variable
viewers   : the variable under test — ramp 1k → 2k → 5k → 10k, hold each tier
observe   : at each tier, M2 p99 + connections held + RAM/conn + CPU
```
This isolates "fanout p99 vs connection count" — the curve a judge wants to see —
instead of confounding it with bid throughput. If M2 grows roughly linearly with
connections, that is the WS downlink (serialization/JSON/sendbuf) cost, and the
scale-out path is gateway sharding.

## 5. Connection cost & node prep (state these numbers)

| Quantity | Expectation | Note |
|---|---|---|
| RAM / connection (Go, goroutine-per-conn + buffers) | ~20–30 KB | **measure yours**; 10 000 ≈ 200–300 MB (fine on 32 GB) |
| CPU | scales with **accepted-update × connections** fanout volume | keep accepted rate low; avoid per-message-deflate at high conn counts (CPU/RAM blowup) |
| fd / `ulimit -n` | must exceed connections + headroom | first thing that breaks; raise `ulimit -n` and `fs.nr_open` |
| ephemeral ports on the **load generator** | ~28k/IP | **local k6 to the same box hits this before the server does** → use PTS's 20 source IPs, or multiple loopback IPs / a 2nd ECS for local 10k |

Node prep checklist before any 10k run:
```bash
ulimit -n 1048576
sysctl -w fs.nr_open=2000000 net.core.somaxconn=65535 net.ipv4.tcp_tw_reuse=1
# load generator only: widen ephemeral range / add source IPs
sysctl -w net.ipv4.ip_local_port_range="1024 65535"
```

## 6. Metric → chart / panel mapping

| Claim | Source |
|---|---|
| M2 fanout p99 ≤ 1 s | PTS `广播接收 ws-fanout-receive` sampler p99 (note JMeter-mode 15 s aggregation) + k6 client histogram |
| connections held | PTS concurrent VU = connections; Grafana `active_ws_connections` |
| RAM / connection | Grafana RSS ÷ connections at each tier |
| every viewer got every seq | scenario verifier: each sampled connection received seq 1..final, all with `published_at_ms` (`verify-l2p3/p4` style) |

## 7. Pitfalls

- **Hold counted as latency.** If `ws-fanout-receive` p99 is tens of seconds, the
  hold leaked into elapsed — split samplers per [playbook §4](pts-playbook.md)
  (`58A5X7KG`).
- **Local 10k to the same box.** Ephemeral-port exhaustion / CPU contention on the
  generator looks like a server limit but is not — prefer PTS multi-IP, document
  the local limit.
- **Cross-clock fanout math.** Same-VPC/same-host only, or use round-trip echo.
- **per-message-deflate at 10k.** Known to blow up CPU/RAM (gorilla #203) — disable
  compression for the high-conn run unless measured.
- **Reporting connection count as a latency claim.** "10 000 connected" is
  capacity; pair it with M2 (latency) and M4 (no leak) or it proves nothing about
  realtime sync.
- **Heartbeat closing idle viewers.** The viewer sampler must answer ping/pong so
  the server's 20 s+5 s heartbeat does not close the cohort mid-run.
