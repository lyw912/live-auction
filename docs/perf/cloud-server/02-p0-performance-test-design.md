# 02 · 从 P0 重新开始的性能测试设计

## P0 目标

P0 不是追求最高 TPS。P0 目标是建立可信压测纪律：

1. 环境可复现。
2. 数据集可解释。
3. 负载模型可证明。
4. HTTP 结果和业务结果分离。
5. 服务侧指标能归因。
6. 每个性能数字都有 raw evidence。
7. 能明确回答“为什么这里是瓶颈，不是别处”。

在 P0 完成前，禁止写：

- 支持 N 在线用户。
- 支持 N QPS。
- P99 低于某值。
- 工业级容量已经达标。

可以写：

- 当前瓶颈在哪里。
- 哪个假设被证据支持。
- 哪个能力还未证明。
- 下一轮如何证明或推翻。

## P0 阶段划分

### P0-A 环境冻结

目的：确保后续数据可复现。

必须记录：

```text
git sha
ECS 规格
OS/kernel
Go version
PostgreSQL version/settings
Redis version/settings
Docker compose sha
ulimit -n
sysctl: somaxconn, tcp backlog, ip_local_port_range
PTS 地域、压测模式、最大 VU/RPS、IP 数
后端启动命令和 env
admission 状态
```

验收：

- `readyz` 通过。
- `/metrics` 可访问。
- `auction_admission_enabled` 与本轮 profile 一致。
- DB/Redis/MinIO ready。
- 证据写入 `docs/perf/cloud-server/runs/<run-id>/environment.md`。

### P0-B 数据集和认证可信

目的：避免压测打到 401、ACL、假 token 或过期 auction。

必须验证：

- CSV session 数量 >= 最大并发用户数的 2 倍。
- session token 未过期。
- `ALLOW_MOCK_AUTH=false` 时仍能正常登录/下单/出价。
- `Idempotency-Key == client_bid_id`。
- auction `end_at` 覆盖完整压测时长和恢复观察窗口。
- auction 初始价格、cap、increment 使有效 bid 比例可控。

验收：

- 10 VU smoke: bid、snapshot、ws-ticket 全部 HTTP 2xx。
- bid 业务结果中 `ACCEPTED` 占比符合本轮目标。
- 没有 `AUCTION_ENDED` 除非本轮专门测结束态。

### P0-C 负载模型验证

目的：确认脚本能打到目标子系统。

至少三类 profile：

| Profile | Admission | 目的 | 成功判定 |
|---|---|---|---|
| `auth-smoke` | on | 登录、session、ACL | 无 401/403，业务可用 |
| `production-guarded` | on | 保护策略、429、用户体验 | admission 生效，下游不崩 |
| `downstream-pressure` | off | 找真实后端瓶颈 | 无 admission 拦截，DB/outbox/Redis/WS 指标移动 |

验收：

- 每轮报告写明 profile。
- admission-on 的结果不能用来宣称 DB/outbox 极限。
- admission-off 的结果不能宣传为生产默认容量。

### P0-D 单轮小压基线

目的：验证采集链路，不追求瓶颈。

配置：

```text
PTS VPC 内网
VU: 10
duration: 3m
ramp: 1m
auction end_at: now + 30m
```

必须采集：

- PTS report details。
- PTS sampling logs。
- before/during/after server evidence。
- DB bids/outbox/status。
- Redis info。
- `/metrics`。

验收：

- 业务分布可信。
- outbox pending 能回落到接近 0。
- 没有采集脚本失败。

### P0-E 第一个真实瓶颈轮

目的：在可控压力下找到第一个实证瓶颈。

推荐从低到高：

```text
100 VU / 10m / ramp 10m
300 VU / 10m / ramp 5m
600 VU / 10m / ramp 5m
```

每轮必须回答：

1. Offered load 是多少？
2. Achieved throughput 是多少？
3. HTTP error 分布？
4. 业务 result 分布？
5. admission 是否拦截？
6. DB lock wait 是否升高？
7. outbox backlog 是否持续增长？
8. Redis latency 是否升高？
9. CPU/IO/network 是否饱和？
10. 降压后 backlog 是否回落？

## P0 完成标准

P0 性能测试完成必须满足：

- 至少 1 个 small smoke 证据完整。
- 至少 1 个 downstream-pressure 证据完整。
- 至少 1 个 production-guarded 证据完整。
- 对每个瓶颈有明确证据和反证。
- 对每个未证明能力标注为 unverified。
- 所有脚本、数据集、报告、metrics、DB 查询可复跑。

## 失败也算成功的场景

以下失败是有效 P0 结果：

- outbox pending 单调增长。
- PG lock wait 成为主耗时。
- Redis latency 飙升。
- WS slow consumer 被关闭。
- PTS dropped iteration 或 VU 不足。
- admission 过早拦截。

前提是必须保留证据，并能说明失败点在 SUT、压测工具、网络还是数据模型。
