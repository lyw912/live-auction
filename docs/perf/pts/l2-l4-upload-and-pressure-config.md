# L2-L4 PTS Upload And Pressure Configuration

> Status: current runbook draft for Alibaba Cloud PTS/JMeter execution.

## Engine Choice

Use **JMeter pressure test** for formal L2-L4 evidence. These workloads need
CSV splitting, Groovy timing barriers, dynamic idempotency keys, and WebSocket
long connections. The JMX files use JSR223 with Java `HttpClient WebSocket`, so
no third-party JMeter plugin JAR is required.

Use **PTS YML / PTS native** only for the auxiliary `L2-P2 read-only` probe.
It is not formal L2 evidence because it does not run in the same clock window as
the bid burst.

## Shared Target

| Field | Value |
|---|---|
| Region | same VPC / same region as ECS |
| Target host | `172.16.179.112` |
| Port | `18080` |
| Protocol | `http` |
| Backend profile | `BID_ENGINE_MODE=redis_ledger`, `ADMISSION_ENABLED=false` |
| CSV mode | enable Split File for every uploaded CSV |

## Data Preparation

```bash
bash tests/pts/prepare-l2-protocol-pressure.sh
bash tests/pts/prepare-l3-l4-pressure.sh
```

Generated/uploaded CSVs:

| File | Purpose |
|---|---|
| `docs/perf/pts/pts-l2-bidder-1000-sessions.csv` | bidder auth pool |
| `docs/perf/pts/pts-l2-bidder-1008-sessions.csv` | exact L2-P3 bidder auth pool for 14 PTS IPs |
| `docs/perf/pts/pts-l2-viewer-4998-sessions.csv` | exact L2-P3 WebSocket viewer auth pool for 14 PTS IPs |
| `docs/perf/pts/pts-l2-reader-994-sessions.csv` | exact L2-P3 HTTP reader auth pool for 14 PTS IPs |
| `docs/perf/pts/pts-l2p4-bidder-360-sessions.csv` | exact L2-P4 active bidder auth pool for 6 PTS IPs |
| `docs/perf/pts/pts-l2p4-viewer-2400-sessions.csv` | exact L2-P4 WebSocket viewer auth pool for 6 PTS IPs |
| `docs/perf/pts/pts-l2p4-reader-240-sessions.csv` | exact L2-P4 HTTP reader auth pool for 6 PTS IPs |
| `docs/perf/pts/pts-l2-viewer-10000-sessions.csv` | WebSocket viewer auth pool |
| `docs/perf/pts/pts-l2-reader-5000-sessions.csv` | HTTP reader auth pool |

## Formal JMeter Upload Matrix

| Workload | Upload files | JMeter properties | Alibaba panel |
|---|---|---|---|
| L2-P1 bid + WS formal first run | `tests/pts/L2-protocol/pts-2p1-bid-plus-ws-fanout.jmx`, bidder CSV, viewer CSV | `host=172.16.179.112`, `port=18080`, `bid_threads=1000`, `ws_threads=8000`, `ws_ramp_sec=30`, `bid_delay_sec=0`, `burst_wait_ms=15000`, `run_duration_sec=120`, `ws_hold_sec=75` | JMeter pressure test; see PTS pressure-configuration template below |
| L2-P1 bid + WS quota-max formal run | same as above | `bid_threads=1000`, `ws_threads=9000`, `ws_ramp_sec=30`, `bid_delay_sec=0`, `burst_wait_ms=15000`, `run_duration_sec=120`, `ws_hold_sec=75` | Use only if the 8000 viewer run passes; see PTS pressure-configuration template below |
| L2-P1 WS capacity probe | same as above | `bid_threads=1000`, `ws_threads=10000` or `20000`, shorten `run_duration_sec` as needed | Capacity exploration only unless VUM budget and evidence collection are recorded |
| L2-P2 bid + reads | `tests/pts/L2-protocol/pts-2p2-bid-plus-reads.jmx`, bidder CSV, reader CSV | `bid_threads=1000`, `read_threads=2000`, `read_delay_ms=250`, `run_duration_sec=180` | JMeter pressure test; duration 3 min |
| L2-P3 combined first run | `tests/pts/L2-protocol/pts-2p3-bid-ws-reads.jmx`, `pts-l2-bidder-1008-sessions.csv`, `pts-l2-viewer-4998-sessions.csv`, `pts-l2-reader-994-sessions.csv` | `bid_threads=1008`, `ws_threads=4998`, `read_threads=994`, `ws_ramp_sec=20`, `read_ramp_sec=20`, `burst_wait_ms=15000`, `run_duration_sec=60`, `fanout_expected_final_seq=11` | JMeter pressure test; see L2-P3 template below |
| L2-P4 steady interactive auction | `tests/pts/L2-protocol/pts-2p4-steady-interactive-auction.jmx`, `pts-l2p4-bidder-360-sessions.csv`, `pts-l2p4-viewer-2400-sessions.csv`, `pts-l2p4-reader-240-sessions.csv` | `pressure_ips=6`, `bid_threads=360`, `ws_threads=2400`, `read_threads=240`, `run_duration_sec=600`, `bid_duration_sec=420`, `fanout_observe_ms=510000` | JMeter pressure test; first production-realistic realtime auction gate |
| L3-S1 lifecycle | `tests/pts/L3-scenario/pts-3s1-full-lifecycle-30min.jmx`, all three CSVs | `bid_threads=500`, `bid_loops=12`, `ws_threads=500`, `read_threads=1000`, `run_duration_sec=1800` | JMeter pressure test; duration 30 min |
| L3-S2 multi-room | `tests/pts/L3-scenario/pts-3s2-multi-room-isolation.jmx`, bidder CSV, reader CSV | `bid_threads=900`, `read_threads=300`, `run_duration_sec=300` | JMeter pressure test; duration 5 min |
| L4-M1 full mixed | `tests/pts/L4-combined/pts-4m1-full-mixed.jmx`, all three CSVs | `bid_threads=1000`, `ws_threads=1000`, `read_threads=3000`, `side_bid_threads=200`, `run_duration_sec=600` | JMeter pressure test; duration 10 min; requires >5000 VU quota |

## If Using The Limited Native PTS Panel

Only use it for `tests/pts/L2-protocol/pts-2p2-read-only.pts.yml`.

Suggested panel values:

| Field | Value |
|---|---|
| Scenario type | PTS YML |
| Pressure mode | RPS mode for read-only probe |
| Traffic model | manual or step increase |
| Max VU | start with 1000 if account quota is limited |
| Duration | 3 minutes |
| Loop count | no fixed loop for sustained reads |

Do not use native PTS for L2-P1/L2-P3/L3/L4 formal evidence: it cannot express
the WebSocket long-connection and bid timing-barrier semantics captured in the
JMeter scripts.

## L2-P1 Scale Rationale

L2-P1 models a high-intent live auction room, not a generic marketing
livestream. Viewers should outnumber bidders: most connected users watch price
changes, while a smaller subset actively bids near the close. A `1000 bid VU +
500 WS viewer` mix is therefore not representative because it implies more
active bidders than watchers. Use `1000 bid VU + 8000-9000 WS viewers` for the
first judge-facing run under the current 30000 VUM account budget. Use 10000,
20000, or higher WS counts only as capacity exploration or with shorter duration
until the system has passing evidence at the lower tiers.

The hard user-experience ceiling for L2-P1 is bid p99 <= 100ms from PTS sampling
logs. Also report server-core gateway p99 separately; this distinguishes backend
decision latency from PTS/TCP/WebSocket connection overhead.

For WebSocket samplers, PTS "response time" must mean the open/auth/connect
operation, not the long-connection dwell time. Apache JMeter records sampler
elapsed time, and its `SampleResult` API supports `samplePause()` /
`sampleResume()` for idle time. The L2-P1 JMX pauses the sample while holding the
socket, so the report does not misclassify a 75s viewer hold as 75s latency.
If PTS shows only `open and hold WebSocket viewer` rows and no `POST L2-P1 hot
bid` rows, the run is harness-invalid regardless of success rate.

Java's `java.net.http.WebSocket` automatically sends a reciprocal pong for
received ping frames, but the L2-P1 listener still requests ping/pong frames
explicitly to avoid starving control-frame delivery under many viewer threads.

## Realtime Auction Load Caveat

The final-window burst model remains valuable because it isolates contention and
fanout correctness under a synchronized hot moment. It is not, by itself, a
production-realistic realtime auction load model. Real auctions are interactive:
one user's accepted bid is broadcast, other users react, and this repeats while
the room stays connected.

Therefore `L2-P3` is a mixed-protocol instrumentation gate, not the final
judge-facing realtime load claim. The broader workload plan is in
`docs/perf/pts/realtime-auction-load-model-2026-06-02.md`:

- `L2-P4`: steady interactive auction, 3000-5000 WS, minority active bidders,
  open-model bid arrivals for 10 minutes.
- `L2-P5`: long-connection fanout soak, up to 10000 WS for 10-30 minutes.
- `L2-P6`: reconnect storm while accepted updates continue.
- `L1-F2`: sustained bid + WS load while faults are injected.

Do not explain Redis/Kafka saturation as "just infrastructure". Redis
connection/PubSub topology and Kafka partitioning/consumer parallelism are
architecture constraints that must be documented and tested separately.

## PTS Pressure-Configuration Template

In this project, **PTS pressure configuration** means only these Alibaba Cloud
console fields:

```text
压力模式
流量模型
最大虚拟用户
起始百分比
压测时长
是否指定循环
循环次数
指定IP数
```

All endpoint paths, host/port defaults, thread-group mix, bid timing barrier,
WebSocket hold time, CSV filenames, assertions, and samplers are controlled by
the uploaded JMX and JMeter properties, not by the PTS pressure panel.

### L2-P1 Formal First Run

JMX properties:

```text
bid_threads=1000
ws_threads=8000
ws_ramp_sec=30
bid_delay_sec=0
burst_wait_ms=15000
run_duration_sec=120
ws_hold_sec=75
```

PTS pressure configuration:

| Field | Value |
|---|---|
| 压力模式 | 虚拟用户模式 |
| 流量模型 | 手动调速 |
| 最大虚拟用户 | 9000 |
| 起始百分比 | 100% |
| 压测时长 | 2 分钟 |
| 是否指定循环 | 是 |
| 循环次数 | 1 |
| 指定IP数 | 18 |

Do not use `最大虚拟用户=1000` for L2-P1. The maximum VU is the total concurrent
JMeter users required by the scene, not just the bidder CSV. With
`bid_threads=1000` and `ws_threads=8000`, the scene needs roughly 9000
concurrent JMeter threads during the bid window.

Do not set `起始百分比=1%` for this JMeter scene. The JMX already contains
`ws_ramp_sec=30` for WebSocket ramp and `burst_wait_ms=15000` for the
synchronized bid moment. Setting the PTS scene to start at 1% adds a second
pressure ramp outside the script and can prevent the WebSocket cohort from being
connected before the bid burst.

Do not add both `bid_delay_sec` and `burst_wait_ms`. `bid_delay_sec` delays the
whole bidder ThreadGroup, while `burst_wait_ms` is the per-thread barrier before
the bid request. L2-P1 keeps `bid_delay_sec=0` and uses only
`burst_wait_ms=15000`, so the one-shot bid burst lands while the WebSocket
cohort is connected but before the server-side 20s heartbeat ping plus 5s
timeout window can close Java load-generator sockets.

`3W9CX76G` proved that `burst_wait_ms=35000` is too late for the current Java
HttpClient WS load generator: the 8000 viewer connections opened, then all
closed on server heartbeat timeout before the bid burst. That is a WS-client
hold problem, not a bid-path latency problem. Long-lived heartbeat stability is
tracked separately from L2-P1's bid p99 gate.

Set `是否指定循环=是, 循环次数=1` for L2-P1. Alibaba Cloud's one-request scenario
guidance uses this setting, and the older PTS/JMeter operation guide says the
PTS pressure loop count can override all thread groups. Without the explicit
one-loop cap, the scene may continue looping until the duration expires, creating
far more bid attempts or WebSocket connection attempts than intended. After all
virtual users complete one loop and the console concurrency falls to zero, stop
the run manually instead of waiting for the full duration.

### L2-P1 Quota-Max Formal Run

Use only after the 8000-viewer run passes.

JMX properties:

```text
bid_threads=1000
ws_threads=9000
ws_ramp_sec=30
bid_delay_sec=0
burst_wait_ms=15000
run_duration_sec=120
ws_hold_sec=75
```

PTS pressure configuration:

| Field | Value |
|---|---|
| 压力模式 | 虚拟用户模式 |
| 流量模型 | 手动调速 |
| 最大虚拟用户 | 10000 |
| 起始百分比 | 100% |
| 压测时长 | 2 分钟 |
| 是否指定循环 | 是 |
| 循环次数 | 1 |
| 指定IP数 | 20 |

### Why Not RPS Mode

L2-P1 contains long-lived WebSocket connections. RPS mode is for controlling
request throughput; it is the wrong primary pressure model for a workload whose
main pressure is concurrent connection count plus a synchronized one-shot bid
burst. Use RPS mode for HTTP read-only probes, not for L2-P1 formal evidence.

### Specified IP Count

Alibaba Cloud PTS documents that a specified pressure IP corresponds to one
pressure machine, and one pressure IP is billed/sized as 500 VU. Therefore:

```text
specified_ip_count = ceil(max_vu / 500)
```

For `最大虚拟用户=9000`, use `指定IP数=18`; for `最大虚拟用户=10000`, use
`指定IP数=20`. This avoids paying for unnecessary unused VU/IP capacity while
still matching the scene's required concurrency.

### L2-P3 Combined First Run

L2-P3 models a single hot live-auction room with all major user protocols active
at the same time:

- 1008 high-intent bidders submit one synchronized final-window bid.
- 4998 viewers connect through WebSocket and receive price-change fanout.
- 994 readers issue one snapshot/leaderboard/my-bids read bundle.

This intentionally does not max out the new 50000 VU quota. The first combined
run should prove that the three protocols coexist without destroying bid p99.
If it fails, run L2-P1 or L2-P2 to isolate the source instead of increasing
scale.

The counts are not arbitrary. Alibaba PTS distributes JMeter work across
specified pressure IPs and its JMeter multi-thread-group documentation warns
that console concurrency/loop settings affect the main Thread Groups. The
previous 1000/5000/1000 model on 14 pressure IPs produced 987 bid samples and
about twice the intended WS samples. The formal P3 model therefore uses exact
multiples for 14 pressure IPs while preserving the user story:

```text
1008 / 14 = 72 bid VU per pressure IP
4998 / 14 = 357 WS viewer VU per pressure IP
994  / 14 = 71 reader VU per pressure IP
total = 7000 VU, exactly 500 VU/IP * 14 IP
```

Do not cite a P3 report unless `tests/pts/verify-l2p3-pts-evidence.sh` passes
against its PTS sampling log.

JMX properties:

```text
bid_threads=1008
ws_threads=4998
read_threads=994
ws_ramp_sec=20
read_ramp_sec=20
bid_delay_sec=0
burst_wait_ms=15000
run_duration_sec=60
fanout_expected_final_seq=11
fanout_receive_timeout_ms=15000
fanout_p99_sla_ms=1000
ws_first_message_timeout_ms=5000
```

Expected offered shape:

- bid: one-shot 1008 request burst, landing around the active WS window.
- WS: 4998 connection attempts. The script records `UX join snapshot load`,
  `POST WS ticket`, `WS upgrade to first message`, `WS fanout receive all seq`,
  and `WS close` separately.
- fanout: `WS fanout receive all seq` succeeds only if the connection receives
  every public seq from 1 through `fanout_expected_final_seq` and every message
  has `published_at_ms`. Its elapsed time is the connection's worst
  server-published-to-client-received latency, so PTS can display client fanout
  p99 directly.
- reads: 994 reader VUs each execute snapshot, leaderboard, and my-bids once.
  This produces a controlled concurrent read burst during the same bid/WS
  window. Sustained multi-minute read soak remains L2-P2/L3, not P3.

PTS pressure configuration:

| Field | Value |
|---|---|
| 压力模式 | 虚拟用户模式 |
| 流量模型 | 手动调速 |
| 最大虚拟用户 | 7000 |
| 起始百分比 | 100% |
| 压测时长 | 1 分钟 |
| 是否指定循环 | 是 |
| 循环次数 | 1 |
| 指定IP数 | 14 |

Use `是否指定循环=是, 循环次数=1` for L2-P3. Alibaba PTS documents that console
loop count can override JMeter main Thread Groups. P3 therefore makes all three
groups one-loop, uses exact 14-IP counts, and treats repeated-loop or short-count
reports as harness failures.

The first-pass gate is:

- `POST L2-P3 hot bid` sampled count exactly 1008 and p99 <= 100ms.
- server DB bid count close to 1000, no outbox backlog, no pending Redis
  settlement.
- HTTP read samplers mostly 2xx and p99 <= 200ms.
- `UX join snapshot load`, `POST WS ticket`, and `WS upgrade to first message`
  p99 are reported as join-latency components. They are not the same thing as
  "user clicked the room and waited N seconds"; the actual UX join claim is the
  sum/distribution of these components and must be reviewed separately.
- `WS fanout receive all seq` count exactly 4998, success 100%, and p99 <= 1000ms
  for this same-region pressure run. This is the client-side fanout SLA. Server
  metrics such as `auction_fanout_latency_seconds` remain useful attribution,
  but they do not prove every connected viewer received every message.

### L2-P4 Steady Interactive Auction

L2-P4 is the first judge-facing realtime auction load model. It represents a
busy auction room during normal bidding, not a synchronized final-second burst:

- 2400 viewers join and hold WebSocket connections.
- 360 active bidder sessions generate sustained bid arrivals after viewers are
  connected.
- 240 reader sessions poll snapshot, leaderboard, and my-bids with jitter.
- Bid arrival is paced per pressure IP to approximate an open-model arrival
  curve. Total offered bid rate is 20/s for 2 minutes, 60/s for 2 minutes, then
  100/s for the remaining bid window. This keeps active bidders a minority while
  still pressuring the single hot auction's state machine and fanout path.

The 3000 VU shape is intentional for the first paid P4 run. It fits 6 PTS IPs
at 500 VU/IP, runs for 10 minutes, and gives enough time to see whether bid
latency, fanout latency, goroutines, fd, RSS/heap, and outbox lag are stable.
If it passes, run a higher P4 tier before making larger capacity claims.

JMX properties:

```text
pressure_ips=6
bid_threads=360
ws_threads=2400
read_threads=240
ws_ramp_sec=60
read_ramp_sec=60
bid_ramp_sec=60
bid_delay_sec=90
read_delay_sec=60
run_duration_sec=600
bid_duration_sec=420
fanout_observe_ms=510000
fanout_catchup_timeout_ms=30000
bid_rate_stage1_per_sec=20
bid_rate_stage2_per_sec=60
bid_rate_stage3_per_sec=100
read_delay_ms=2000
read_jitter_ms=3000
ws_first_message_timeout_ms=5000
```

Expected offered shape:

- 0-60s: WS/read cohorts ramp and join.
- 60-90s: readers are active, WS cohort settles before bid pressure.
- 90-510s: steady bids run in three arrival-rate stages.
- 0-600s: WS observes messages; at the end each WS sampler fetches final auction
  seq and verifies it received every public seq from 1 through final seq.

PTS pressure configuration:

| Field | Value |
|---|---|
| 压力模式 | 虚拟用户模式 |
| 流量模型 | 手动调速 |
| 最大虚拟用户 | 3000 |
| 起始百分比 | 100% |
| 压测时长 | 10 分钟 |
| 是否指定循环 | 否 |
| 循环次数 | 不填 |
| 指定IP数 | 6 |

Use `是否指定循环=否` for L2-P4. Unlike P3, bidder and reader groups are
duration-driven loops. Setting a console loop count risks overriding the
script's sustained interaction model. The formal gate is
`tests/pts/verify-l2p4-pts-evidence.sh`.

The first-pass gate is:

- `POST L2-P4 steady bid` count >= 18000 and p99 <= 100ms.
- `WS L2-P4 fanout observe final seq` count exactly 2400, success 100%, and
  p99 <= 1000ms.
- Join segments p99 <= 1000ms each.
- HTTP read samplers success 100% and p99 <= 200ms.
- Server evidence shows no pending Redis decisions, no outbox backlog, no DLQ,
  and no monotonic runtime resource climb.

## Post-Run Evidence

```bash
BASE_URL=http://127.0.0.1:18080 bash tests/pts/collect-server-evidence.sh <report-id-or-label>
FINAL_WAIT_SECONDS=0 bash tests/pts/verify-l4b-pts-correctness.sh <report-id-or-label>
bash tests/pts/verify-l2p4-pts-evidence.sh <report-id-or-label>
```

For L3-S2/L4, also inspect per-auction rows for `auc_live`, `auc_side`, and
`auc_inv_001`; the existing verifier is centered on the hot auction gate.
