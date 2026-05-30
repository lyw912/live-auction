# 08 · 证据索引

> 2026-05-31 supersession notice: this is a historical evidence index for the
> old cloud core-pressure run. It is not current PTS-1B success evidence. Current
> evidence classification is governed by `docs/current/evidence-policy.md`,
> `tests/pts/MANIFEST.md`, and `tests/pts/HISTORICAL.md`.

## 当前 PTS 证据

| 内容 | 路径 |
|---|---|
| PTS PDF | `Jmeter压测报告-3IVNW7TF-20260528014811.pdf` |
| PDF page images | `docs/perf/pts/pdf-pages/` |
| PTS sampling CSV | `docs/perf/pts/evidence/archive/historical/after-3IVNW7TF-review/pts-sampling-logs/sampling-logs.csv` |
| PTS sampling JSONL | `docs/perf/pts/evidence/archive/historical/after-3IVNW7TF-review/pts-sampling-logs/sampling-logs.jsonl` |
| PTS sampling raw pages | `docs/perf/pts/evidence/archive/historical/after-3IVNW7TF-review/pts-sampling-logs/pages/` |
| Service metrics | `docs/perf/pts/evidence/archive/historical/after-3IVNW7TF-review/metrics.prom` |
| Redis info | `docs/perf/pts/evidence/archive/historical/after-3IVNW7TF-review/redis-info.txt` |
| CPU | `docs/perf/pts/evidence/archive/historical/after-3IVNW7TF-review/mpstat.txt` |
| Disk | `docs/perf/pts/evidence/archive/historical/after-3IVNW7TF-review/iostat-xz.txt` |
| Memory | `docs/perf/pts/evidence/archive/historical/after-3IVNW7TF-review/free-m.txt` |
| Processes | `docs/perf/pts/evidence/archive/historical/after-3IVNW7TF-review/top-processes.txt` |
| Sockets | `docs/perf/pts/evidence/archive/historical/after-3IVNW7TF-review/ss-s.txt` |

## 当前 PTS 工具

| 内容 | 路径 |
|---|---|
| JMX | `tests/pts/archive/historical/live-auction-core-pressure.jmx` |
| Prepare script | `tests/pts/prepare-cloud-pressure.sh` |
| Reset script | `tests/pts/archive/historical/reset-pressure-data.sh` |
| Evidence script | `tests/pts/collect-server-evidence.sh` |
| PTS sampling fetch | `tests/pts/fetch-pts-sampling-logs.sh` |
| Session CSV | `docs/perf/pts/archive/data/pts_sessions.csv` |

## 当前关键数字

```text
PTS AllCount: 383580
PTS AvgTps: 643.44
PTS AvgRt: 58.13ms
PTS P99: 152.99ms
PTS HTTP failures: 0

DB bids: 316400
accepted: 18199
rejected: 298201

outbox PENDING: >304000
outbox PUBLISHED: ~11695
pending shard: 13
oldest pending: >1h

Redis outbox publish pipeline avg: ~0.29ms
admission: disabled
```

## 缺失证据

这些缺口会被评委攻击，必须补：

1. during CPU/IO/network/DB 连续采样。
2. accepted-only pressure 报告。
3. rejected-only pressure 报告。
4. outbox relay 修复前后对比。
5. WS fanout 云端压测。
6. reconnect storm 云端压测。
7. slow consumer 云端压测。
8. 多房间热度偏斜压测。
9. 3-run final baseline。
10. pprof CPU/heap/goroutine 证据。

## 外部参考

| 主题 | 链接 |
|---|---|
| Google SRE monitoring | https://sre.google/sre-book/monitoring-distributed-systems |
| Google SRE cascading failures | https://sre.google/sre-book/addressing-cascading-failures |
| Google SRE capacity planning | https://sre.google/sre-book/introduction |
| Brendan Gregg USE method | https://www.brendangregg.com/usemethod.html |
| Linux USE checklist | https://www.brendangregg.com/USEmethod/use-linux.html |
| PostgreSQL SELECT locking | https://www.postgresql.org/docs/current/sql-select.html |
| PostgreSQL explicit locking | https://www.postgresql.org/docs/current/explicit-locking.html |
| Debezium outbox router | https://debezium.io/documentation/reference/stable/transformations/outbox-event-router.html |
| k6 constant arrival rate | https://grafana.com/docs/k6/latest/using-k6/scenarios/executors/constant-arrival-rate |
| k6 scenarios | https://grafana.com/docs/k6/latest/using-k6/scenarios/ |
| Kafka partition key ordering | https://www.confluent.io/learn/kafka-partition-key/ |
| Kafka message key | https://www.confluent.io/learn/kafka-message-key/ |
| Centrifugo design | https://centrifugal.dev/docs/getting-started/design |
| Centrifugo engines | https://centrifugal.dev/docs/5/server/engines |
| Alibaba Cloud PTS JMeter | https://www.alibabacloud.com/help/en/pts/performance-test-pts-3-0/user-guide/create-a-jmeter-scenario1 |
