# 03 · 工作负载矩阵与 PTS 配置

## PTS 基础配置

云端正式 PTS 配置：

```text
压力来源: 阿里云 VPC 内网
地域: 华南2（河源）
目标 host: 172.16.179.112
目标 port: 18080
protocol: http
IPv6: off
指定 IP 数: 1
```

为什么不用公网：

- PTS 和 ECS 同地域同 VPC 时，内网压测能减少公网带宽、NAT、安全组和公网抖动干扰。
- 本项目当前目标是找后端瓶颈，不是测试公网入口容量。
- 如果未来需要公网入口容量，必须单独建 profile，记录公网带宽、SLB/NLB、WAF、NAT、TLS 等变量。

## JMX 和 CSV

当前 JMX：

```text
tests/pts/archive/historical/live-auction-core-pressure.jmx
```

session CSV：

```text
docs/perf/pts/archive/data/pts_sessions.csv
```

要求：

- CSV 上传到 PTS 数据源。
- JMX 引用 CSV Data Set Config。
- 线上 `ALLOW_MOCK_AUTH=false` 时，必须走 `/api/auth/login` 或使用由真实 login 生成的 session CSV。
- bid body 的 `client_bid_id` 必须等于 HTTP header `Idempotency-Key`。

## Profile 1: P0 Auth Smoke

目的：证明认证、ACL、CSV、后端、数据库都准备好。

配置：

```text
压力模式: 虚拟用户模式
流量模型: 均匀递增
最大虚拟用户: 10
递增时长: 1 分钟
压测时长: 3 分钟
指定循环: 否
```

验收：

- `/readyz` 200。
- `/api/auth/ws-ticket` 200。
- snapshot 200。
- bid 业务结果可解释。
- 无 401/403。

## Profile 2: P0 Downstream Pressure 100 VU

目的：关闭 admission 后找第一瓶颈。

配置：

```text
压力模式: 虚拟用户模式
流量模型: 均匀递增
最大虚拟用户: 100
递增时长: 10 分钟
压测时长: 10 分钟
指定循环: 否
```

JMeter properties：

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

服务要求：

```text
ALLOW_MOCK_AUTH=false
ADMISSION_ENABLED=false
SESSION_TTL>=12h
HTTP_ADDR=0.0.0.0:18080
```

验收：

- admission metric 为 0。
- PTS HTTP fail 为 0 或可解释。
- bid accepted/rejected 分布符合本轮目标。
- outbox backlog 不应持续增长。若增长，必须归因。
- during 采样有 CPU/IO/DB/Redis 证据。

## Profile 3: Valid Accepted Bid Pressure

目的：测有效成交写路径，而不是大量 `BID_TOO_LOW`。

脚本要求：

- amount 基于全局时间和 rate hint 单调上升。
- cap 足够高，确保压测期间不会提前卖出。
- `end_at` 覆盖压测 + 恢复观察。
- 每轮 seed 后清空旧 bids/outbox。

验收：

```text
ACCEPTED 占比 >= 80%
AUCTION_ENDED = 0
BID_TOO_LOW 可控且可解释
seq 连续
最终 winner/price 正确
outbox lag 可观测
```

## Profile 4: Rejected Bid Pressure

目的：单独测试业务拒绝路径和拒绝事件策略。

形态：

- 大量 self-leading、低价、过期状态、重复 idempotency。
- 明确测 reject 是否应该进入 outbox。

验收：

- reject reason 分布确定。
- 不污染 accepted bid 容量结果。
- 如果 reject 进入 outbox，必须证明 outbox 能承受或有降级策略。

## Profile 5: Outbox Relay Focused

目的：验证 relay 消费能力和 backlog 回落。

负载：

- 固定 event/s 生产。
- relay 独立运行。
- 记录 publish/s、claim latency、mark latency、oldest age。

验收：

```text
steady-state: published/s >= produced/s
oldest_ready_age 不持续增长
PENDING 不单调增长
单 auction 顺序保持
poison event 触发 DEAD + gap notice
```

## Profile 6: WebSocket Fanout

目的：证明 realtime gateway/hub 能承受连接和广播。

形态：

- watcher-heavy。
- bid rate 低到中等。
- 连接数逐步上升。
- 部分 slow consumer。

验收：

- healthy client 收到连续 seq。
- slow client 被关闭。
- RSS、goroutine、FD bounded。
- reconnect 后 history/snapshot 正常。

## Profile 7: Reconnect Storm

目的：证明断线恢复不会压垮 DB snapshot。

形态：

- N 个客户端断开。
- bid 继续推进 seq。
- 客户端带 stale `last_seq` 重连。

验收：

- Redis history 命中率。
- DB snapshot rebuild 数受 semaphore 限制。
- retry-after 行为正确。
- 客户端 CTA 在 recovering 状态禁用。

## Profile 8: Production Guarded

目的：开启 admission，验证保护策略和产品体验。

要求：

- 不用于声明 downstream 极限。
- 用于证明 overload 时返回稳定 429/业务错误。
- 下游 DB/outbox/Redis 不被打爆。

验收：

- 429/retry-after 可解释。
- 用户体验文案明确。
- 服务侧 backlog 不失控。
