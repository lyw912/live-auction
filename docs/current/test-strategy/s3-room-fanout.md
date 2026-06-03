# S3 — 万人围观 / Single-Room Fanout

> Maps to: brief 加分 "单直播间 1000+ 用户同时在线（超基础 10×）" + "出价数据秒级同步".
> Headline: **M2 fanout publish→receive p99 ≤ 1 s** at the target connection count
> + connections held + RAM/connection.
> Tool: **PTS** (clean per-connection p99 chart) + **local k6** (10 000 soak).
> Current PTS assets:
> `tests/pts/S3-room-fanout/s3-live-fanout-smoke-30vu-single-branch-20ws-5bid-5read.jmx`
> and
> `tests/pts/S3-room-fanout/s3-live-fanout-4500vu-single-branch-3000ws-1000bid-500read.jmx`.
> Do not use old L2/P3 filenames for new S3 reports.
> Expanded split: `S3-live-only-fanout` and `S3-mixed-final-burst` are governed
> by [s2-s3-expanded-test-design.md](s2-s3-expanded-test-design.md).

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
"S3 WS handshake complete"              → WebSocket open/accept latency
"S3 WS first snapshot/business message" → realtime-ready latency for room entry
"S3 live fanout receive"                → live published_at_ms -> client receive latency
```
The live fanout sampler must not poll `/api/auctions/{id}` per viewer. S3 v6
records live fanout only from WebSocket messages whose `published_at_ms` is not
older than the viewer connection time. Read traffic is isolated in the explicit
reader thread group.

Handshake interpretation: `S3 WS handshake complete` is a client-observed
JMeter sampler around Java `HttpClient.newWebSocketBuilder().buildAsync(...).join()`.
It includes TCP/HTTP Upgrade and pressure-agent/client scheduling effects, not
just backend `websocket.Accept`. The sampler reuses a shared Java `HttpClient`
and pauses timing while constructing listener/client state; its response message
records both `SETUP_MS` and `HANDSHAKE_MS` so a slow chart can be separated from
client-side setup noise. Server-side `auction_ws_join_stage_seconds` remains the
backend attribution source for ticket consume, room/access validation, accept,
recovery, first write, and total join.

## 4. Decouple connection count from bid rate

Fanout latency depends on connections, not on how fast bids arrive. So:
```
bidders   : a SMALL fixed source of accepted updates (e.g. 1–10 accepted/s) — not the variable
viewers   : the variable under test — ramp 1k → 2k → 5k → 10k, hold each tier
observe   : at each tier, M2 p99 + connections held + RAM/conn + CPU
```
This is `S3-live-only-fanout`. It isolates "fanout p99 vs connection count" —
the curve a judge wants to see — instead of confounding it with bid throughput.
If M2 grows roughly linearly with connections, that is the WS downlink
(serialization/JSON/sendbuf) cost, and the scale-out path is gateway sharding.

`S3-mixed-final-burst` is a later integration rehearsal, not the M2 baseline. It
adds final-window bid pressure and controlled read traffic only after
`S3-live-only-fanout` and `S2-read-interference` are clean.

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

For S3 PTS reports, use Alibaba Cloud `GetJMeterReportDetails` /
`SamplerMetricsList` as the source for sampler `AllCount`, `SuccessRateReq`, and
`Seg99Rt`. Alibaba Cloud documents `GetJMeterSamplingLogs` as the API for
sampler log rows with filters such as `SamplerId`; those rows are useful for
request/response-body forensics, but they are not the S3 exact-count source.
`VAH7X7CG` showed this concretely: 100% sampling-log retrieval returned only
sampler ids 0,4,5,6,7, while report details showed all S3 HTTP, WS, and reader
samplers ran with the expected counts and 100% success.

PTS full-run configuration must set `是否指定循环=是` and `循环次数=1`.
Alibaba Cloud PTS documents that console concurrency/loop settings override main
Thread Group settings in uploaded JMeter scripts. If loop count is not specified,
PTS can keep the main Thread Groups running until the configured duration, which
turns the reader group into continuous polling. `VLH9X7NG` demonstrated this
failure mode: the 994-reader group produced about 1.31 million GET requests
instead of the intended `994 * 3`, saturated the DB pool, and invalidated the
report as clean S3 fanout evidence.

The current mixed PTS asset uses one main Thread Group with a mixed-role CSV:
3000 WebSocket viewers, 1000 bidders, and 500 readers. This is intentional.
Earlier multi-main-ThreadGroup variants depended on PTS distributing several
groups proportionally; small and full runs repeatedly ended with wrong role
counts or reader loops. The single-branch asset makes the expected count equal
to the mixed CSV rows and verifies it through sampler counts.

## 7. Pitfalls

- **Hold counted as latency.** If `ws-fanout-receive` p99 is tens of seconds, the
  hold leaked into elapsed — split samplers per [playbook §4](pts-playbook.md)
  (`58A5X7KG`).
- **History/recovery counted as live fanout.** A reconnect or newly joined viewer
  can receive older public events that still carry `published_at_ms`. Those
  events answer snapshot/recovery correctness, not M2 live fanout. The k6 S3
  script must only measure messages with `published_at_ms >= connectedAtMs`.
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
- **Sampling-log row coverage.** A 100% sampling setting does not make
  `GetJMeterSamplingLogs` the S3 exact-count ledger. Verify counts and p99 from
  `GetJMeterReportDetails`; use sampling logs only to inspect diagnostic
  response messages such as `S3_V6_LIVE_FANOUT_OK...WS_ONLY`.
- **Too few accepted updates.** Thousands of viewers with only a dozen accepted
  public updates is useful connection evidence, not a rich fanout sample. Report
  accepted update count, receive sample count, and publish subscriber count
  together.
- **Read traffic mislabeled as fanout.** High reader RPS belongs to
  `S2-read-interference`; S3's M2 baseline should keep reads absent or tightly
  controlled.
- **PTS loop override.** Do not rely on JMX `LoopController.loops=1` alone in PTS
  JMeter mode. For the full S3 asset, set PTS `是否指定循环=是` and `循环次数=1`;
  the formal JMX also sets reader CSV `recycle=false` and `stopThread=true` as a
  guardrail.
- **Handshake p99 misread.** If PTS reports a high `S3 WS handshake complete`
  p99, compare it with server `auction_ws_join_stage_seconds{stage="total"}` and
  response markers `SETUP_MS/HANDSHAKE_MS`. In `VAH7X7CG`, server total join was
  below 10ms for all 20 connections while the client-observed PTS handshake p99
  was 596ms, pointing at pressure-agent/network/client timing rather than the
  backend accept path.

## 8. Current Local Evidence Snapshot

> Status: raw incoming evidence, not yet a final PTS PDF. Use for judge rehearsal
> only with the scope below.

| Run | Scale | Fanout p99 | Viewer errors | Interpretation |
|---|---:|---:|---:|---|
| `s3-local-scale-1000-liveonly-20260602T2303` | 1000 WS, 60s, 301 accepted updates | 22 ms | 0 | Current local S3/M2 evidence: live publish-to-online-viewer receive latency passes the 1s target |
| `s3-local-scale-1000-20260602T2300` | 1000 WS, 60s, 74 accepted updates | 59.6 s | 9 | Harness-contaminated run: history/recovery messages were counted as fanout latency |
| `s3-local-scale-2000-20260602T2305` | attempted 2000 WS | no complete summary | unknown | Failed/incomplete run; usable only as single-node/local-generator ceiling evidence |

Judge-safe wording:

> "The first 1000-WS local run exposed a measurement bug: historical/recovery
> messages were being included in M2, producing a false 59.6s fanout p99. After
> fixing the harness to count only live messages published after the viewer
> connection opened, the 1000-WS rerun produced p99 22ms with zero viewer errors.
> We cite the live-only run for S3/M2 and keep the earlier run as a harness
> correction record."

Do not claim 2000 or 10000 WS success from the current local artifacts. The next
judge-grade S3 artifact should be either the PTS 2000-WS cost variant or a
completed local 10k hold with Grafana resource panels and a clear generator-limit
statement.
