# PTS Playbook — make the report *be* the evidence, cheaply

> Status: governing PTS execution playbook, 2026-06-02.
> Goal: a judge opens the exported PTS PDF and sees one clean p99 chart per
> metric — no 100% sampling-log pull, no spreadsheet, no "trust me".

This encodes how Alibaba Cloud PTS 3.0 actually computes and renders numbers, so
the report chart maps 1:1 to a metric in [metrics-and-slo.md](metrics-and-slo.md).
Every claim here is sourced from Alibaba PTS docs (links inline).

## 1. The one fact that changes everything

> **PTS report percentiles (TP50/90/99) are rank-computed over *all* measured
> requests, and are independent of the "采样日志 / sampling-log" sample rate.**
> ([percentile FAQ](https://help.aliyun.com/zh/pts/performance-test-pts-2-0/support/what-do-quantiles-mean-in-stress-test-reports),
> [report doc](https://help.aliyun.com/zh/pts/performance-test-pts-2-0/user-guide/view-a-stress-test-report))

Consequences:
- **Keep sampling log at the default 1%.** It does not affect the p99 chart; it
  only feeds the per-request waterfall viewer and, above 1%, multiplies VUM cost
  (`×(1+rate)`, up to ×2 at 100%). The old habit of "open 100% sampling and pull
  everything" was paying double for zero latency-chart benefit.
- **You no longer post-process logs to get p99.** Screenshot/export the report.
- Pull sampling logs *only* for forensic spot-checks of `ENGINE_*` bodies (see §9).

## 2. Cost model & IP sizing (memorize this)

```
VUM = ⌈max_VU / 500⌉ × 500 × duration_minutes × (1 + sample_rate)     # sample_rate=0.01 ⇒ ×1.01 ≈ ×1
cost(¥, public cloud) = VUM × 0.003
1 specified pressure IP  ≈ 500 VU  ≈ 1 pressure machine  (RPS-mode cap 4000 RPS/IP)
指定IP数 = ⌈max_VU / 500⌉
```
([billing](https://help.aliyun.com/zh/pts/performance-test-pts-3-0/product-overview/pts-3-0-billing-overview),
[IP doc](https://help.aliyun.com/zh/pts/performance-test-pts-3-0/user-guide/specifies-the-number-of-testing-ip-addresses))

You pay for `⌈maxVU/500⌉×500` whether real concurrency is 50 or 500, so **short
runs are the main saving**, and never over-specify IPs. New accounts get a free
**5000 VUM** tier (covers S1 outright).

| Scenario | max VU | IPs | min | VUM | ≈¥ |
|---|---|---|---|---|---|
| S1 绝杀 | 1000 | 2 | 1 | 1 000 | 3 |
| S2 稳态 (RPS) | ~1000 | 2 | 10 | 10 000 | 30 |
| S3 围观 headline | 10 000 | 20 | 5 | 50 000 | 150 |
| S3 围观 variant | 2 000 | 4 | 5 | 10 000 | 30 |

## 3. One sampler = one metric (the naming rule)

PTS reports business metrics **per named Sampler**: the overview table lists each
Sampler with 总请求数 / 平均TPS / 成功率 / 平均响应时间, and a drill-down shows
**max/min and 各分位 (all percentiles)** plus a per-Sampler time-series.
([JMeter report](https://help.aliyun.com/zh/pts/performance-test-pts-3-0/user-guide/view-jmeter-pressure-test-report))

So **name each sampler exactly as its judge-facing metric**, and never reuse a
name for two different operations (they merge). Canonical sampler labels for this
project (use verbatim across all JMX so charts are comparable across runs):

| Sampler label | Metric | Notes |
|---|---|---|
| `出价决策 bid-decision` | M1 | the only sampler whose p99 you cite as decision latency |
| `广播接收 ws-fanout-receive` | M2 | WebSocket Single-Read; elapsed = wait-for-one-broadcast |
| `建立连接 ws-connect` | join context | WebSocket Open Connection (handshake only) |
| `加入房间 join-snapshot` | join context | GET snapshot |
| `领取票据 ws-ticket` | join context | POST ws-ticket |
| `读榜 read-leaderboard` | read context | leaderboard/my-bids reads |

Avoid the legacy numeric-prefixed names (`10 PTS-1B ...`, `30 ws ticket ...`) in
judge-facing runs — they read as internal noise. Keep them only in smoke JMX.

## 4. WebSocket timing — never let the hold contaminate latency

PTS runs the **JMeter WebSocket Samplers plugin** (Peter Doornbosch); each sampler
times only its own op, so the multi-second connection *hold* is never folded into
a latency number — **if** you split the lifecycle into separate samplers.
([PTS WebSocket](https://help.aliyun.com/zh/pts/performance-test-pts-3-0/use-cases/how-to-perform-pressure-measurement-of-websocket-protocol))

```
Open Connection   → sampler "建立连接 ws-connect"        (elapsed = handshake)
Single Write      → send bid / no metric                (elapsed = send only)
Single Read (loop)→ sampler "广播接收 ws-fanout-receive"  (elapsed = wait for ONE frame = fanout latency)
Close             → no metric
```

- A `Single Read` blocks until one queued frame arrives or it times out; its
  elapsed time *is* the publish→receive wait. That is M2, directly, per
  connection — no post-processing.
- Do **not** rely on `SampleResult.samplePause()/sampleResume()` to subtract hold
  time — PTS support for it is [UNVERIFIED]. The per-sampler split above makes it
  unnecessary.
- Embed `published_at_ms` in each broadcast payload; the Read's response
  assertion verifies the seq and (optionally) records `MAX_LAT_MS` as a
  cross-check in `responseMessage`, while the sampler elapsed is the chart value.

## 5. JMX owns logic; the console owns only pressure shape

The console pressure panel controls **only**: 压力模式 / 流量模型 / 最大虚拟用户 /
起始百分比 / 压测时长 / 是否指定循环 + 循环次数 / 指定IP数. Everything else —
endpoints, host/port defaults, thread-group ratio, the bid timing barrier, WS
hold, CSV names, assertions, sampler names — lives in the JMX.

Two hard rules from the docs:
- **The console cannot inject custom `__P(name)` properties per run.** Properties
  come from uploaded `.properties` files in "JMeter 环境管理", *not* the pressure
  panel. So **bake defaults into the JMX**: `${__P(bid_threads,1000)}`. This is
  exactly why `GUA1X7HG` failed — it expected the panel to pass formal defaults.
  ([env management](https://help.aliyun.com/zh/pts/performance-test-pts-3-0/user-guide/jmeter-environment-management))
- **The console concurrency + loop count OVERRIDE the main Thread Group** (not
  Setup/Teardown), and PTS rewrites the script to hit the target.
  ([multi-thread-group](https://help.aliyun.com/zh/pts/performance-test-pts-3-0/user-guide/jmeter-instructions-for-using-multiple-thread-groups))
  - One-shot burst (S1): set 是否指定循环=是, 循环次数=1 so the cohort fires once.
  - Duration-driven (S2/S3): set 是否指定循环=否 so the script's own loop/observe
    logic governs; a console loop count would override and break it.

## 6. Mode choice & time resolution

| Workload | Mode | Engine | Why |
|---|---|---|---|
| S1 绝杀 (HTTP burst) | VU mode, loop=1 | **PTS JMeter current asset** | 1000 distinct one-shot clients; reuse the validated JMX and CSV/verifier path |
| S2 稳态 soak | local k6 `ramping-arrival-rate` | k6 | current open-model asset; reports dropped iterations and supports long soak/Grafana |
| S2 optional PTS chart | VU mode, duration-driven | PTS JMeter current asset | use `pts-2p4-steady-interactive-auction.jmx` only when a polished PTS PDF is worth the VUM |
| S3 围观 (WS hold) | **VU mode** | PTS JMeter + local k6 | VUs == held connections == online users; the metric is connection count and fanout receive p99 |

- There is no separate native-HTTP PTS script in the current plan. Current
  formal evidence uses the checked-in JMX/k6 assets listed above. If a native
  script is added later for finer chart granularity, it is a new asset that must
  pass the same `ENGINE_DURABLE` assertions and M3 verifier gates before it can
  replace a JMX.
- RPS mode is Alibaba's stated best practice for short-connection systems; VU
  mode is correct for long-lived connections ("并发用户数即并发接入能力").
  ([metrics doc](https://help.aliyun.com/zh/pts/performance-test-pts-3-0/product-overview/test-metrics))

## 7. Pressure source = same VPC

Set 压力来源 = **阿里云VPC内网**, same region as the ECS. Public-net pressure adds
internet latency you'll mis-attribute to the server, and it corrupts M2's clock
assumption. Same-VPC keeps `published_at_ms → recv` skew negligible.

## 8. Exporting the judge artifact

- 报告导出 → **无水印版本** (PDF) — that's the file for the judge packet.
  ([report doc](https://help.aliyun.com/zh/pts/performance-test-pts-2-0/user-guide/view-a-stress-test-report))
- 报告对比 compares up to 3 reports — use it for **before/after** (e.g. PG-lane
  baseline vs Redis single-writer) to *show* the optimization, not assert it.
- Sampling/agent data is retained **30 days** — export promptly.

## 9. When to actually pull sampling logs (and how)

Only to prove the *business distribution* behind the latency chart — that the
fast responses were real `ENGINE_*` decisions, not `202`/`409`. Pull a **small**
sample (the default 1% is plenty for distribution), using the existing helpers:

```bash
bash tests/pts/fetch-pts-sampling-logs.sh   <report-id>
bash tests/pts/summarize-pts-sampling-logs.sh <report-id>   # ENGINE_* / HTTP / settlement histogram
```

Then the verifier ties it together:
```bash
BASE_URL=http://127.0.0.1:18080 bash tests/pts/collect-server-evidence.sh <label>
FINAL_WAIT_SECONDS=0 bash tests/pts/verify-l4b-pts-correctness.sh <label>
```

## 10. Harness-invalid pitfalls checklist (learned the expensive way)

Run this before trusting any PTS report. Each line is a real past failure.

- [ ] **Bid sampler rows present with p99?** If only WS/join rows appear and no
      `出价决策` rows, the run is invalid regardless of success rate.
- [ ] **Sampler counts exact?** Burst sample count == intended (e.g. 1000), not
      ~987 or ~2×. Wrong counts ⇒ console loop/ratio overrode the script.
- [ ] **No `setStampAndTime()` after JMeter set times?** That throws
      `IllegalStateException` and voids bid/fanout aggregation (`TZ9GX7ZG`).
- [ ] **Formal defaults actually applied?** If the panel didn't inject `__P`,
      the run used JMX defaults — confirm they are the formal values, not smoke
      values (`GUA1X7HG`: ran `bid_delay_sec=90` in a 1-min window → no bids).
- [ ] **WS hold not counted as latency?** `ws-fanout-receive` p99 must be sub-second,
      not tens of seconds (that means the hold leaked into elapsed — `58A5X7KG`).
- [ ] **Burst lands while WS cohort is still connected?** Server heartbeat (20 s
      ping + 5 s timeout) must not close load-gen sockets before the burst
      (`3W9CX76G`: `burst_wait_ms=35000` was too late).
- [ ] **Sampling kept at 1%?** Higher = paying ×(1+rate) for nothing.
- [ ] **是否指定循环 correct?** =是,1 for S1 one-shot; =否 for S2/S3 duration-driven.
- [ ] **M3 verifier PASS** before citing M1/M2 from the run.
