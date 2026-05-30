# 07 · 云服务器下一轮执行手册

> 2026-05-31 supersession notice: this is a historical cloud pressure runbook
> for the earlier core-pressure/PG-lane era. It is not the current PTS-1B
> execution authority. For current PTS-1B use `tests/pts/MANIFEST.md`,
> `docs/current/runtime-profiles.md`, and
> `docs/current/performance-correctness-contract.md`.
>
> Commands in this file that call `tests/pts/archive/historical/reset-pressure-data.sh` now require
> `ALLOW_HISTORICAL_PTS=1` and must be labeled historical/harness evidence.

## 0. 基本原则

每一轮压测必须有唯一 run id：

```text
cloud-p0-r<N>-<profile>-YYYYMMDD-HHMMSS
```

每一轮必须保存：

```text
environment.md
pts-report.json
pts-sampling-logs.csv/jsonl
before evidence
during evidence
after evidence
db invariant check
analysis.md
decision.md
```

## 1. 启动后端

Air 热重载用于开发调试，不用于正式压测。

正式压测建议使用稳定二进制：

```bash
cd /root/workspace/live-auction
APP_ENV=local \
HTTP_ADDR=0.0.0.0:18080 \
DATABASE_URL='postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable' \
REDIS_ADDR=localhost:6379 \
ALLOW_MOCK_AUTH=false \
ADMISSION_ENABLED=false \
SESSION_TTL=12h \
OUTBOX_WORKER_ID=pts-cloud-1 \
SCHEDULER_WORKER_ID=pts-cloud-1 \
/tmp/live-auction-pts/live-auction-server
```

开发调试才用：

```bash
cd /root/workspace/live-auction/backend
air
```

## 2. 准备数据

谨慎：这会重置压测数据。

```bash
cd /root/workspace/live-auction
bash tests/pts/archive/historical/reset-pressure-data.sh
```

验证：

```bash
curl -fsS http://127.0.0.1:18080/readyz
curl -fsS http://127.0.0.1:18080/metrics | grep 'auction_admission_enabled 0'
wc -l docs/perf/pts/archive/data/pts_sessions.csv
```

DB 验证：

```bash
docker exec live-auction-postgres psql -U live_auction -d live_auction -c \
"select id,status,current_price_cents,accepted_bid_count,seq,end_at from auctions where id='auc_live';"
```

## 3. Before 采集

```bash
bash tests/pts/collect-server-evidence.sh before-<run-id>
```

必须确认 `postgres-summary.txt` 非空。

## 4. During 采集

压测期间每 60 秒执行：

```bash
bash tests/pts/collect-server-evidence.sh during-<run-id>-01
bash tests/pts/collect-server-evidence.sh during-<run-id>-02
bash tests/pts/collect-server-evidence.sh during-<run-id>-03
```

未来应加 loop 脚本自动采集。

## 5. PTS 执行

上传：

```text
tests/pts/archive/historical/live-auction-core-pressure.jmx
docs/perf/pts/archive/data/pts_sessions.csv
```

配置：

```text
压力来源: 阿里云 VPC 内网
地域: 华南2（河源）
压力模式: 虚拟用户模式
流量模型: 均匀递增
最大虚拟用户: 100
递增时长: 10 分钟
压测时长: 10 分钟
指定循环: 否
指定 IP 数: 1
IPv6: 关闭
```

JMeter properties：

```text
protocol=http
host=172.16.179.112
port=18080
auction_id=auc_live
room_id=room_main
```

## 6. After 采集

PTS 结束后立即执行：

```bash
bash tests/pts/collect-server-evidence.sh after-<run-id>
```

## 7. 拉取 PTS API 数据

报告详情：

```bash
aliyun pts get-jmeter-report-details \
  --endpoint pts.aliyuncs.com \
  --region cn-heyuan \
  --report-id <REPORT_ID>
```

采样日志：

```bash
bash tests/pts/fetch-pts-sampling-logs.sh <REPORT_ID> docs/perf/cloud-server/runs/<run-id>/pts-sampling-logs
```

注意：

- 当前接口 `PageSize=100` 可用，`1000` 会报 `InvalidPageSize`。
- `GetJMeterSamplingLogs` 可能只返回采样明细，不是全量请求。
- 全量业务事实以 DB 查询为准。

## 8. DB invariant check

每轮结束后至少执行：

```sql
select count(*) as bids,
       count(*) filter (where status='ACCEPTED') as accepted,
       count(*) filter (where status='REJECTED') as rejected
from bids;

select event_type, count(*)
from outbox_events
where auction_id='auc_live'
group by event_type
order by count(*) desc;

select status, count(*)
from outbox_delivery
group by status
order by status;

select shard_id, status, count(*), max(now()-event_created_at) as max_age
from outbox_delivery
where status in ('PENDING','FAILED','PUBLISHING')
group by shard_id,status
order by count(*) desc;
```

## 9. 分析报告模板

每轮 `analysis.md` 必须包含：

```text
Run id:
Report id:
Profile:
Admission:
Load model:
Environment:
PTS summary:
Business result distribution:
DB full result:
Outbox result:
Redis result:
Machine resource:
Bottleneck:
Alternative explanations ruled out:
Unverified:
Next action:
```

## 10. 停止条件

出现以下任一情况必须停止升压，先归因：

- outbox PENDING 单调增长。
- oldest_ready_age 持续增长。
- P99 > 目标且无法解释。
- admission 意外打开。
- `AUCTION_ENDED` 意外出现。
- PTS dropped/timeout 明显。
- DB lock wait 或 pool wait 突增。
- Redis evictions/rejected connections。
- CPU steal 或 iowait 异常。
