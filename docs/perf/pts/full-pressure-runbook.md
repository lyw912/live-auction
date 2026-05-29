# Full PTS Pressure Runbook

Use this flow for each pressure round.

For the current PTS-1 hotspot optimization work, use the dedicated hotspot
exploration bundle:

```bash
cd /root/workspace/live-auction
bash tests/pts/reset-hotspot-pressure-data.sh
bash tests/pts/collect-server-evidence.sh before-pts1-hotspot-YYYYMMDD-HHMM
```

Current phase: post-`1L29X7UG` hotspot optimization validation, not final judge
evidence and not production capacity evidence. The explicit goal is to verify
that Redis guard filters stale-low pressure before PostgreSQL, that accepted
bids refresh the guard projection immediately after commit, and that the
per-auction lane keeps single-auction PG concurrency bounded.

The reset script starts the backend with:

```text
ADMISSION_ENABLED=false
BID_ENGINE_MODE=redis_guard
BID_LANE_WORKERS=1
BID_LANE_QUEUE_SIZE=2048
BID_LANE_QUEUE_TIMEOUT=3s
DB_MAX_CONNS=90
DB_MIN_CONNS=16
```

This is intentionally different from the `1L29X7UG` high-lane exploration
profile. `BID_LANE_WORKERS=256` was useful for exposing the PG row-lock convoy,
but it is not the optimized single-auction profile. The optimized profile
converts DB lock wait into bounded application queue wait plus explicit
`BID_AUCTION_TOO_HOT` / `BID_RETRY_LATER` when offered pressure exceeds the
single-auction serialization capacity.

If a future diagnostic round needs to push through the lane to expose a lower
component, override the variables explicitly and label the evidence as
`HARNESS_EXPLORATION`, not as a user-facing latency optimization result:

```bash
BID_LANE_WORKERS=256 BID_LANE_QUEUE_SIZE=100000 BID_LANE_QUEUE_TIMEOUT=10m \
  bash tests/pts/prepare-cloud-pressure.sh
```

Upload:

- `tests/pts/live-auction-hotspot-pressure.jmx`
- `docs/perf/pts/pts_hotspot_sessions.csv`

PTS settings for optimization validation rounds:

```text
Pressure source: Alibaba Cloud VPC internal network
Mode: virtual users
Traffic model: ramping evenly
Max VU: 1000 first; raise only after guard reject rate, queue wait, DB lock
wait, and correctness are understood
Duration: 8 minutes
Ramp duration: 3 minutes
Specified loop: no
IP count: 1
IPv6: off
```

After the run:

```bash
bash tests/pts/collect-server-evidence.sh after-REPORTID-pts1-hotspot-review
bash tests/pts/fetch-pts-sampling-logs.sh REPORTID docs/perf/pts/evidence/after-REPORTID-pts1-hotspot-review/pts-sampling-logs
```

`1L29X7UG` is the high-lane failure baseline for this repair: Redis guard was
mostly stale, `auction_bid_lock_wait_seconds_sum` was about `40973s`, and DB
pool wait was about `236216s`. The next run must report guard outcomes,
projection update outcomes, queue wait/rejects, DB lock wait, tx duration,
outbox lag, and correctness invariants together. Do not publish a capacity
number from a single validation run.

## 1. Reset Data

```bash
cd /root/workspace/live-auction
bash tests/pts/reset-pressure-data.sh
```

Verify:

```bash
curl -fsS http://127.0.0.1:18080/readyz
curl -fsS http://127.0.0.1:18080/metrics | grep 'auction_admission_enabled 0'
wc -l docs/perf/pts/pts_sessions.csv
```

Expected: ready, admission 0, CSV around 4097 lines.

## 2. Collect Before Evidence

```bash
bash tests/pts/collect-server-evidence.sh before-r2
```

This saves metrics, DB summary, Redis info, CPU, memory, disk, and process data
under `docs/perf/pts/evidence/before-r2/`.

## 3. Run PTS

Upload:

- `tests/pts/live-auction-core-pressure.jmx`
- `docs/perf/pts/pts_sessions.csv`

PTS settings:

```text
Pressure source: Alibaba Cloud VPC internal network
Mode: virtual users
Traffic model: ramping evenly
Max VU: 100 for first real round, then 300, then 600
Duration: 10 minutes
Ramp duration: 10 minutes for first round, 5 minutes for later rounds
Specified loop: no
IP count: 1
IPv6: off
```

JMeter properties:

```text
protocol=http
host=172.16.179.112
port=18080
room_id=room_main
auction_id=auc_live
base_price_cents=10000
increment_cents=5000
bid_rate_hint_rps=300
bid_threads=100
snapshot_threads=30
ticket_threads=50
```

## 4. Collect After Evidence

Immediately after PTS ends:

```bash
bash tests/pts/collect-server-evidence.sh after-r2
```

Then preserve the PDF report in `docs/perf/pts/evidence/after-r2/`.

## 5. Pull PTS Sampling Logs

Alibaba Cloud `GetJMeterSamplingLogs` gets JMeter sample logs by page.
Required parameters are `ReportId`, `PageNumber`, and `PageSize`; useful filters
include `SamplerId`, `Success`, `ResponseCode`, `BeginTime`, and `EndTime`.
Alibaba Cloud PTS sampling logs are diagnostic samples, not the full request
ledger. Official documentation states that sampling logs are collected at a
default `1%` sampling rate and retained for 30 days; pay-as-you-go billing
documentation also describes the pressure-log sampling rate as defaulting to
`1%`. Use `GetJMeterSampleMetrics` or report details for PTS-side aggregate
counts/RT/percentiles, use sampling logs for examples and failure diagnosis, and
use server-side DB/Redis/Kafka evidence for full business correctness.

If using OpenAPI Explorer:

```text
Product: PTS / 2020-10-20
API: GetJMeterSamplingLogs
ReportId: 3IVNW7TF
PageNumber: 1
PageSize: 100
```

Repeat pages until the returned list is empty. Export or copy the JSON per page.

If the `aliyun` CLI is installed and configured, the shape is:

```bash
aliyun pts GetJMeterSamplingLogs \
  --ReportId 3IVNW7TF \
  --PageNumber 1 \
  --PageSize 100
```

Save raw JSON pages under:

```text
docs/perf/pts/evidence/after-r2/pts-sampling-logs/
```

## 6. Decide Verdict

Do not use the PDF alone for architecture decisions.

Valid bottleneck evidence requires:

- PTS success/error distribution;
- before/after `/metrics`;
- outbox pending/backlog age;
- PostgreSQL lock/activity;
- Redis info;
- CPU, memory, disk IO.

Current first-round signal:

```text
HTTP layer was stable, but outbox_delivery accumulated a large PENDING backlog.
Next round should focus on outbox relay backlog and delivery lag.
```
