# Independent ECS k6 Execution Runbook

> Status: operating runbook, 2026-06-04.
> Scope: configure a rented ECS as an independent k6 load generator, then run
> selected S1-S5 verification without contaminating service-node resources.

This document is meant for the Codex session running on the rented ECS. It does
not replace PTS for S1 judge evidence or large S3 WebSocket evidence.

## 1. What To Upload To The ECS

Upload the project files needed to run k6 scripts and helper scripts:

```text
tests/load/
tests/pts/
tests/chaos/
docs/perf/pts/pts-1ab-1000vu-sessions.csv
docs/perf/pts/s3-mixed-smoke-30-sessions.csv
docs/perf/pts/s3-mixed-4500-sessions.csv
scripts/perf/
docs/perf/k6-ecs-runbook.md
docs/perf/k6-ecs-command-cheatsheet.md
docs/current/test-strategy/independent-k6-runbook.md
docs/current/test-strategy/metrics-and-slo.md
docs/current/test-strategy/s1-s5-debug-and-system-change-log.md
tests/pts/MANIFEST.md
```

If using `git clone`, these files are already included. If copying a bundle,
keep the same relative paths from repo root.

Do not upload secrets:

```text
.env
.env.local
.env.production
SSH private keys
cloud access keys
database passwords
PTS API credentials
browser cookies
large historical docs/perf/raw/** artifacts
large historical docs/perf/pts/evidence/archive/** artifacts
```

Use environment variables on the ECS for target addresses:

```bash
export BASE_URL=http://SERVICE_PRIVATE_IP:18080
export WS_URL=ws://SERVICE_PRIVATE_IP:18080
```

Prefer the private VPC IP when the k6 ECS and service ECS are in the same VPC.
Only use public IP when private routing is unavailable, and record that fact in
the run note.

## 2. One-Time ECS Bootstrap

From repo root on the k6 ECS:

```bash
chmod +x scripts/perf/*.sh
sudo bash scripts/perf/bootstrap-k6-ecs.sh
```

Open the ECS security group only as needed:

| Port | Direction | Purpose |
|---:|---|---|
| 22 | inbound to k6 ECS | SSH/Codex access |
| 9100 | inbound from Prometheus/service VPC only | node_exporter scrape |
| 18080 | outbound to service ECS | API and WebSocket pressure |

After bootstrap, start a new shell so `ulimit -n` reflects the new limits:

```bash
ulimit -n
k6 version
curl -fsS "$BASE_URL/readyz"
curl -fsS "http://127.0.0.1:9100/metrics" | head
```

Expected: `ulimit -n` is high, k6 prints a version, service `/readyz` is
reachable, and node_exporter exposes metrics.

## 3. Before Every Run

Record the run label and the exact service target:

```bash
export LABEL=s2-ecs-$(date +%Y%m%dT%H%M%S)
export BASE_URL=http://SERVICE_PRIVATE_IP:18080
export WS_URL=ws://SERVICE_PRIVATE_IP:18080
mkdir -p "docs/perf/pts/evidence/incoming/${LABEL}"
```

Save an environment note:

```bash
{
  echo "label=$LABEL"
  echo "base_url=$BASE_URL"
  echo "ws_url=$WS_URL"
  echo "host=$(hostname)"
  echo "date=$(date -Is)"
  echo "kernel=$(uname -a)"
  echo "ulimit_nofile=$(ulimit -n)"
  k6 version
  nproc
  free -h
  ss -s
} | tee "docs/perf/pts/evidence/incoming/${LABEL}/k6-ecs-env.txt"
```

The service side should be reset/prepared by the service-node operator or a
separate Codex session. Do not assume the k6 ECS can access Docker containers on
the service ECS unless SSH or a remote collection flow was explicitly set up.

## 4. S2 Long Soak

Use independent k6 for the real 30-60 minute soak.

Default 30-minute shape:

```bash
export LABEL=s2-ecs-30m-$(date +%Y%m%dT%H%M%S)
export BASE_URL=http://SERVICE_PRIVATE_IP:18080
STAGE_DUR=10m \
STAGE1_RATE=20 \
STAGE2_RATE=60 \
STAGE3_RATE=100 \
PRE_ALLOC_VUS=80 \
MAX_VUS=300 \
bash scripts/perf/run-remote-k6.sh s2-long-soak
```

60-minute shape:

```bash
export LABEL=s2-ecs-60m-$(date +%Y%m%dT%H%M%S)
STAGE_DUR=20m bash scripts/perf/run-remote-k6.sh s2-long-soak
```

Required service-side follow-up:

```bash
BASE_URL=http://SERVICE_PRIVATE_IP:18080 \
bash tests/pts/collect-server-evidence.sh "$LABEL"
```

If the collector is not running on the service node, run it from the service
node and use the same label.

Interpretation note: this run proves long-running bid-decision stability
(`ENGINE_ACCEPTED` + `ENGINE_REJECTED`) and async convergence. It does not prove
accepted-heavy WebSocket fanout or reader interference. Use `S2-read-interference`
below for HTTP read pressure and S3 for WebSocket/fanout pressure.

## 4b. S2 Read Interference

Run this after `S2-long-soak` when you need to prove normal live-room polling
does not hurt bid decisions. It intentionally stays HTTP-only; WebSocket fanout
belongs to S3.

Service-side preparation:

```bash
ALLOW_MOCK_AUTH=true \
BID_MAX_VUS=400 \
READ_MAX_VUS=5000 \
bash tests/pts/prepare-s2-read-interference-pressure.sh
```

This resets `auc_live`, starts the backend with `ADMISSION_ENABLED=false` and
`BID_ENGINE_MODE=redis_ledger`, and pre-seeds mock-auth room membership / Redis
ACL cache for `k6_bidder_1..400` and `k6_user_1..5000`.

Attack / ceiling-discovery 15-minute shape:

> 2026-06-04 result: `s2-read-ecs-15m-20260604T113330` did **not** cleanly pass
> this 2k/5k/10k read profile. It found the current read-path ceiling: bid p99
> stayed 5.68ms at ~100/s, but read p99 became seconds-level and k6 recorded
> 2,057,742 dropped iterations. Use this shape only when intentionally attacking
> the read path or validating a read-path optimization.

```bash
export LABEL=s2-read-ecs-15m-$(date +%Y%m%dT%H%M%S)
export BASE_URL=http://SERVICE_PRIVATE_IP:18080
STAGE_DUR=5m \
BID_STAGE1_RATE=100 \
BID_STAGE2_RATE=100 \
BID_STAGE3_RATE=100 \
READ_STAGE1_RATE=2000 \
READ_STAGE2_RATE=5000 \
READ_STAGE3_RATE=10000 \
BID_PRE_ALLOC_VUS=120 \
BID_MAX_VUS=400 \
READ_PRE_ALLOC_VUS=1500 \
READ_MAX_VUS=4000 \
bash scripts/perf/run-remote-k6.sh s2-read-interference
```

Reduced display-ceiling rerun after the 10k/4k/read-display failures and the
2026-06-04 P0/P1 fixes:

- P0: Redis hot-engine state/log TTL raised from 30m to 24h; pause mirroring no
  longer recreates partial Redis hot state.
- P1: HTTP auction snapshot gets a 250ms Redis cache + singleflight, empty
  max-bid-intent lookups get a 5s negative cache, `my-bids` is capped at 50
  rows, ACL hit/miss metrics were added, and read-path indexes plus SQL
  attribution were added.
- 2026-06-04 result: `s2-read-display-postfix-ecs-15m-20260604T140509` is
  CURRENT_PASS for this display profile: k6 exit 0, dropped 0, HTTP failures 0,
  bid p99 3.76ms, snapshot p99 11.54ms, leaderboard p99 4.46ms, my-bids p99
  0.87ms, verifier P0/P1 PASS. The service verifier shows 91,714 cumulative
  settled decisions because the preceding smoke was not reset before the formal
  run; k6 formal-only bid decisions were 91,499.

```bash
export LABEL=s2-read-display-postfix-ecs-15m-$(date +%Y%m%dT%H%M%S)
export BASE_URL=http://SERVICE_PRIVATE_IP:18080
STAGE_DUR=5m \
BID_STAGE1_RATE=100 \
BID_STAGE2_RATE=100 \
BID_STAGE3_RATE=100 \
READ_STAGE1_RATE=1500 \
READ_STAGE2_RATE=1800 \
READ_STAGE3_RATE=2000 \
BID_PRE_ALLOC_VUS=120 \
BID_MAX_VUS=400 \
READ_PRE_ALLOC_VUS=1000 \
READ_MAX_VUS=2500 \
bash scripts/perf/run-remote-k6.sh s2-read-interference
```

The previous reduced attempt `s2-read-clean-ecs-15m-20260604T120823`
(`2000/3000/4000` reads/s) was still CURRENT_FAILING: 524,423 dropped
iterations, snapshot p99 1.02s, leaderboard p99 2.72s, my-bids p99 596ms.
The later `s2-read-display-ecs-15m-20260604T123644` attempt
(`1500/1800/2000` reads/s) was also CURRENT_FAILING: 63,531 dropped iterations,
snapshot p99 1.26s, leaderboard p99 3.41s, my-bids p99 730ms, and service-side
verification exposed the Redis hot-ledger TTL P0. Do not cite it as pass; cite
it as bottleneck evidence before the P0/P1 fixes.
The postfix run above supersedes it for the display profile, but not for the
3k/4k/5k/10k attack profiles.

Smoke shape:

```bash
export LABEL=s2-read-smoke-ecs-$(date +%Y%m%dT%H%M%S)
export BASE_URL=http://SERVICE_PRIVATE_IP:18080
STAGE_DUR=20s \
BID_STAGE1_RATE=1 \
BID_STAGE2_RATE=3 \
BID_STAGE3_RATE=5 \
READ_STAGE1_RATE=10 \
READ_STAGE2_RATE=30 \
READ_STAGE3_RATE=50 \
BID_PRE_ALLOC_VUS=10 \
BID_MAX_VUS=30 \
READ_PRE_ALLOC_VUS=20 \
READ_MAX_VUS=80 \
bash scripts/perf/run-remote-k6.sh s2-read-interference
```

Required service-side follow-up:

```bash
BASE_URL=http://SERVICE_PRIVATE_IP:18080 \
bash tests/pts/collect-server-evidence.sh "$LABEL"
EXPECTED_UNIQUE_BIDS="" FINAL_WAIT_SECONDS=0 \
bash tests/pts/verify-l4b-pts-correctness.sh "$LABEL"
```

## 5. S4 Fault Resilience

S4 fault injection still happens on the service side. The independent ECS is
only the client pressure source. There are two acceptable modes:

1. Service-side runner remains authoritative:

```bash
FAULT_TYPE=redis bash tests/pts/run-pts-1c-concurrent-fault.sh
FAULT_TYPE=pg bash tests/pts/run-pts-1c-concurrent-fault.sh
FAULT_TYPE=kafka bash tests/pts/run-pts-1c-concurrent-fault.sh
```

2. If a service-side operator injects faults manually, run the k6 script from
the independent ECS and record exact injection timestamps in the evidence note.

For final evidence, prefer mode 1 unless the runner has been updated to separate
the load generator from the fault injector. The report must include:

- exact fault type and injection timestamp;
- k6 summary and k6 host metrics;
- service-side `readyz`, Redis/Kafka/PostgreSQL/outbox convergence evidence;
- correctness verifier output.

## 6. S5 Reconnect Recovery

Clean reconnect from independent ECS:

```bash
export LABEL=s5-clean-100-ecs-$(date +%Y%m%dT%H%M%S)
export BASE_URL=http://SERVICE_PRIVATE_IP:18080
export WS_URL=ws://SERVICE_PRIVATE_IP:18080
VUS=100 \
DURATION=2m \
MISSED_EVENTS=3 \
BID_RATE_PER_S=10 \
BID_SOURCE_VUS=5 \
bash scripts/perf/run-remote-k6.sh s5-clean
```

Scale to 200 only after the 20/100 runs are clean:

```bash
export LABEL=s5-clean-200-ecs-$(date +%Y%m%dT%H%M%S)
VUS=200 DURATION=2m bash scripts/perf/run-remote-k6.sh s5-clean
```

Network/Toxiproxy mode currently belongs on the service-side runbook because
the proxy is part of the service environment. Do not claim weak-network evidence
from the independent ECS unless the Toxiproxy path and target URL are documented.

## 7. S3 WebSocket Sanity Only

A single 4c8G k6 ECS may run 1000-2000 WS sanity. Do not use it as the only
proof for 5000-10000 WS.

```bash
export LABEL=s3-ws-1000-ecs-$(date +%Y%m%dT%H%M%S)
export BASE_URL=http://SERVICE_PRIVATE_IP:18080
export WS_URL=ws://SERVICE_PRIVATE_IP:18080
VIEWER_VUS=1000 \
HOLD_SECONDS=120 \
BIDDER_VUS=3 \
bash scripts/perf/run-remote-k6.sh s3-ws-sanity
```

For 2000 WS:

```bash
export LABEL=s3-ws-2000-ecs-$(date +%Y%m%dT%H%M%S)
VIEWER_VUS=2000 HOLD_SECONDS=120 bash scripts/perf/run-remote-k6.sh s3-ws-sanity
```

Clean S3 sanity requires the k6 host to retain CPU/network/socket headroom and
the service side to report healthy active WS/fanout metrics.

## 8. Host Health Gates

Mark the run as load-generator-contaminated if any of these occurs:

- k6 process CPU is effectively saturated for a sustained period;
- `dropped_iterations` grows unexpectedly;
- `vus` reaches `vus_max` and target rate is still not delivered;
- memory pressure, OOM, or swap growth appears;
- TCP retransmits, NIC errors/drops, or socket count spikes abnormally;
- open file count approaches `ulimit -n`;
- WebSocket connection errors happen before service metrics show pressure.

Evidence files are under:

```text
docs/perf/pts/evidence/incoming/<LABEL>/
docs/perf/pts/evidence/incoming/<LABEL>/k6-host/
```

Keep at least:

```text
k6-summary.json
k6-samples.jsonl
k6-exit.txt
k6-host/host-sample.log
k6-host/k6-pidstat.log
k6-host/network-sar-dev.log
k6-host/network-sar-tcp.log
k6-host/socket-sample.log
k6-host/k6-fd-sample.log
k6-ecs-env.txt
```

## 9. Report Template

After each run, write a short note:

```text
LABEL:
Scenario:
Target URL type: private VPC / public IP
Load shape:
k6 exit:
dropped_iterations:
p95/p99/p99.9:
k6 host CPU/memory/network/socket health:
Service-side evidence path:
Correctness/verifier:
Verdict: CURRENT_PASS / CURRENT_FAILING / ENV_LIMIT / HARNESS_GAP
```

Do not publish a latency or capacity number unless both k6-host health and
service-side convergence/correctness evidence are present.
