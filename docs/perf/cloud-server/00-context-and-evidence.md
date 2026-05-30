# 00 · 当前背景与证据

## 环境事实

| 项 | 当前值 |
|---|---|
| 云厂商 | 阿里云 |
| ECS | 8c / 32G |
| 地域 | 华南2（河源） |
| PTS 压力来源 | 阿里云 VPC 内网，华南2（河源） |
| SUT 内网地址 | `172.16.179.112:18080` |
| SUT 公网地址 | `47.113.223.90` |
| 后端端口 | `18080` |
| code-server 端口 | `8080`，不能用于后端 |
| PostgreSQL | Docker container `live-auction-postgres` |
| Redis | Docker container `live-auction-redis` |
| MinIO | Docker dependency |
| Admission | 本轮压测关闭，`auction_admission_enabled 0` |

## 已完成压测

PTS 报告：

```text
ReportId: 3IVNW7TF
ReportName: pts-20260528013714
StartTime: 2026-05-28 01:37:14
EndTime:   2026-05-28 01:47:14
AgentCount: 1
Vum: 4967
```

PTS 汇总：

| 指标 | 值 |
|---|---:|
| AllCount | 383580 |
| AvgTps | 643.44 |
| AvgRt | 58.13 ms |
| Seg90Rt | 110 ms |
| Seg99Rt | 152.99 ms |
| FailCountReq | 0 |
| SuccessRateReq | 100% |

接口明细：

| API | Count | Avg TPS | Avg RT | P90 | P99 | Max | Fail |
|---|---:|---:|---:|---:|---:|---:|---:|
| `POST bid downstream pressure` | 313905 | 526.57 | 62.91 ms | 111 ms | 152.98 ms | 456 ms | 0 |
| `POST ws-ticket issue` | 45658 | 76.59 | 35.73 ms | 55 ms | 62 ms | 284 ms | 0 |
| `GET snapshot under bid pressure` | 13134 | 22.03 | 47.26 ms | 72 ms | 80 ms | 237 ms | 0 |
| `GET auction snapshot auth ACL` | 3627 | 6.08 | 52.32 ms | 71 ms | 79.72 ms | 157 ms | 0 |
| `GET /readyz` | 3628 | 6.09 | 14.54 ms | 21 ms | 26 ms | 153 ms | 0 |
| `GET /metrics admission flag` | 3628 | 6.09 | 14.54 ms | 20 ms | 26 ms | 56 ms | 0 |

## PTS 采样明细

PTS `GetJMeterSamplingLogs` 接口本轮返回 3895 条采样日志，不是 38 万全量请求明细。第 40 页开始为空。

采样路径：

```text
docs/perf/pts/evidence/archive/historical/after-3IVNW7TF-review/pts-sampling-logs/sampling-logs.csv
docs/perf/pts/evidence/archive/historical/after-3IVNW7TF-review/pts-sampling-logs/sampling-logs.jsonl
```

采样中的 bid 业务结果：

| 结果 | 数量 |
|---|---:|
| `REJECTED / BID_TOO_LOW` | 1948 |
| `REJECTED / AUCTION_ENDED` | 1064 |
| `ACCEPTED` | 176 |
| 解析为空或非 bid body | 10 |

这证明 PTS 的 HTTP 100% 成功不能直接解释为有效成交路径容量。它代表 HTTP handler 没有错误，但业务上大多数请求是拒绝。

## 数据库真实结果

压测后查询：

```text
bids total: 316400
accepted:    18199
rejected:   298201
```

outbox event 分布：

```text
bid_rejected: 298201
bid_accepted:  18199
```

outbox delivery 状态：

```text
PENDING:   304705+
PUBLISHED: 11695+
```

所有未完成 outbox 都集中在一个 shard：

```text
shard_id=13
status=PENDING
count=304705+
max_age > 1 hour
```

## 服务指标证据

关键 Prometheus 指标：

```text
auction_admission_enabled 0
auction_bid_request_total{reason="AUCTION_ENDED",result="REJECTED"} 104646
auction_bid_request_total{reason="BID_TOO_LOW",result="REJECTED"} 193555
auction_bid_request_total{reason="",result="ACCEPTED"} 18196
auction_bid_request_total{reason="",result="ACCEPTED_EXTENDED"} 3
```

bid lock 和 latency：

```text
auction_bid_lock_wait_seconds_count 316400
auction_bid_lock_wait_seconds_sum   2593.008
avg lock wait ≈ 8.2 ms

auction_bid_latency_seconds_count 316400
auction_bid_latency_seconds_sum   8700.134
avg backend bid latency ≈ 27.5 ms
```

Redis publish pipeline：

```text
redis_command_latency_seconds_count{command="outbox_publish_pipeline"} 8591
redis_command_latency_seconds_sum{command="outbox_publish_pipeline"} 2.485s
avg ≈ 0.29 ms
```

结论：Redis publish 不是瓶颈。

## 机器资源证据

压测后采集结果：

```text
CPU idle ≈ 84%
iowait ≈ 0.1%
内存 available ≈ 28GB
磁盘 util 低
```

注意：这是压测后采集，不是压测过程中连续采样。因此它能排除“压测后仍被打满”，但不能完全证明压测峰值期间没有瞬时资源瓶颈。下一轮必须增加 during 采样。

## 当前瓶颈判断

当前主瓶颈不是 PTS、VPC、公网、Redis、磁盘、内存或机器规格，而是：

```text
app-owned outbox relay 在单 auction/shard 高事件量下消费能力不足。
```

同时压测脚本存在业务负载污染：

```text
94% 以上 bid 是业务拒绝，主要为 BID_TOO_LOW 和 AUCTION_ENDED。
```

因此本轮只能证明：

1. HTTP handler 在约 643 TPS 下稳定返回。
2. admission 关闭后压力确实到达后端。
3. bid 写路径能承受 31.6 万次请求级写入。
4. outbox relay 和实时投递链路不能承受当前事件生产速度。

不能证明：

1. 31 万有效成交容量。
2. WebSocket 大规模 fanout 容量。
3. 工业级多房间容量。
4. 生产 admission 阈值。
5. 完整竞拍体验在高压下实时可靠。
