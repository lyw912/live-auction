# k6 ECS Command Cheatsheet

> Use from repo root on the independent k6 ECS.

## Bootstrap

```bash
chmod +x scripts/perf/*.sh
sudo bash scripts/perf/bootstrap-k6-ecs.sh
exec bash -l
```

## Target

```bash
export BASE_URL=http://SERVICE_PRIVATE_IP:18080
export WS_URL=ws://SERVICE_PRIVATE_IP:18080
curl -fsS "$BASE_URL/readyz"
```

## S2 30-Minute Soak

```bash
export LABEL=s2-ecs-30m-$(date +%Y%m%dT%H%M%S)
STAGE_DUR=10m \
STAGE1_RATE=20 \
STAGE2_RATE=60 \
STAGE3_RATE=100 \
PRE_ALLOC_VUS=80 \
MAX_VUS=300 \
bash scripts/perf/run-remote-k6.sh s2-long-soak
```

## S2 Read Interference

Smoke:

```bash
export LABEL=s2-read-smoke-ecs-$(date +%Y%m%dT%H%M%S)
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

Default 15-minute run:

```bash
export LABEL=s2-read-ecs-15m-$(date +%Y%m%dT%H%M%S)
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

## S5 Clean Reconnect

```bash
export LABEL=s5-clean-20-ecs-$(date +%Y%m%dT%H%M%S)
VUS=20 DURATION=2m bash scripts/perf/run-remote-k6.sh s5-clean

export LABEL=s5-clean-100-ecs-$(date +%Y%m%dT%H%M%S)
VUS=100 DURATION=2m bash scripts/perf/run-remote-k6.sh s5-clean

export LABEL=s5-clean-200-ecs-$(date +%Y%m%dT%H%M%S)
VUS=200 DURATION=2m bash scripts/perf/run-remote-k6.sh s5-clean
```

## S3 WebSocket Sanity

```bash
export LABEL=s3-ws-1000-ecs-$(date +%Y%m%dT%H%M%S)
VIEWER_VUS=1000 HOLD_SECONDS=120 BIDDER_VUS=3 \
bash scripts/perf/run-remote-k6.sh s3-ws-sanity

export LABEL=s3-ws-2000-ecs-$(date +%Y%m%dT%H%M%S)
VIEWER_VUS=2000 HOLD_SECONDS=120 BIDDER_VUS=3 \
bash scripts/perf/run-remote-k6.sh s3-ws-sanity
```

## Manual k6 With Host Collector

```bash
export LABEL=custom-$(date +%Y%m%dT%H%M%S)
mkdir -p "docs/perf/pts/evidence/incoming/${LABEL}"

INTERVAL_SECONDS=5 \
bash scripts/perf/collect-k6-host-metrics.sh \
  "docs/perf/pts/evidence/incoming/${LABEL}/k6-host" &
COLLECTOR_PID=$!

k6 run \
  --env "BASE_URL=$BASE_URL" \
  --summary-export "docs/perf/pts/evidence/incoming/${LABEL}/k6-summary.json" \
  --out "json=docs/perf/pts/evidence/incoming/${LABEL}/k6-samples.jsonl" \
  tests/load/s2-steady-soak.js

kill "$COLLECTOR_PID"
wait "$COLLECTOR_PID" 2>/dev/null || true
```

## Files To Preserve From Each Run

```text
docs/perf/pts/evidence/incoming/<LABEL>/k6-summary.json
docs/perf/pts/evidence/incoming/<LABEL>/k6-samples.jsonl
docs/perf/pts/evidence/incoming/<LABEL>/k6-exit.txt
docs/perf/pts/evidence/incoming/<LABEL>/k6-host/host-sample.log
docs/perf/pts/evidence/incoming/<LABEL>/k6-host/k6-pidstat.log
docs/perf/pts/evidence/incoming/<LABEL>/k6-host/network-sar-dev.log
docs/perf/pts/evidence/incoming/<LABEL>/k6-host/network-sar-tcp.log
docs/perf/pts/evidence/incoming/<LABEL>/k6-host/socket-sample.log
docs/perf/pts/evidence/incoming/<LABEL>/k6-host/k6-fd-sample.log
```

## Files To Upload To The ECS

If not using `git clone`, upload these repo paths while preserving directory
structure:

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
tests/pts/MANIFEST.md
```

Do not upload `.env`, SSH keys, cloud credentials, database passwords, or old
bulk evidence directories.
