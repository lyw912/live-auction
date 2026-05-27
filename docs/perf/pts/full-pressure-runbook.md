# Full PTS Pressure Runbook

Use this flow for each pressure round.

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
