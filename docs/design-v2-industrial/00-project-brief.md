# 00 · Project Brief

## 官方场景背景

想象这样一个场景：直播间里，一件稀世珠宝正在竞拍，数百人同时出价，价格每秒都在跳动，气氛紧张到窒息——这就是我们要构建的系统。

直播电商的兴起为高价值商品（珠宝、艺术品、二手奢侈品）开辟了全新赛道。这些商品价值难以统一定价，竞拍这种充满互动和竞争感的形式，能让市场动态定价最大化商品价值。

这个背景定义了系统的产品压力：直播间里大量用户同时围观、感知价格变化、在关键时刻出价，并且成交结果必须可信、可解释、可恢复。它不是未经测试的容量承诺；任何并发或性能数字仍必须来自基线证据。

## 目标

实现一个可演示、可测试、可追问的直播竞拍全栈系统：

```text
商品上架 -> 规则配置 -> 定时/手动开拍 -> 实时出价 -> 动态排名
-> 自动延时/封顶成交 -> 订单生成 -> mock 支付/历史记录
-> 监控诊断与故障恢复
```

核心不是“功能堆满”，而是证明：

1. 复杂竞拍规则由服务端强约束执行。
2. 最后一秒并发出价不会错价、错排名、重复成交。
3. WebSocket 断线、乱序、重连后能恢复到服务端权威状态。
4. 性能与稳定性声明有脚本、环境、原始输出和瓶颈解释。
5. 监控诊断能定位真实问题，不是静态大屏。

## 官方功能范围

### PC 商家/主播端

| 功能 | P0 要求 |
|---|---|
| 商品发布 | 标题、图片、描述、规则配置 |
| 规则配置 | 起拍价、加价幅度、时长、封顶价、自动延时、保证金展示参数 |
| 规则修改 | 仅 DRAFT 可改；SCHEDULED 后冻结 |
| 开拍 | 手动 start 或 scheduler start |
| 竞拍控制台 | 当前价、领先者、出价轮数、剩余时间、状态、讲解中 |
| 异常取消 | 终态不可逆，写事件和广播 |
| 订单 | 成交价、winner、mock 支付状态、保证金状态 |
| 诊断 | 活跃竞拍、异常、outbox、scheduler、reject 分布 |

### Mobile H5 用户端

| 功能 | P0 要求 |
|---|---|
| 直播间 | 模拟直播、商品列表、弹幕、用户加入氛围 |
| 商品详情 | 图片、描述、规则解释、当前状态 |
| 出价 | 手动出价、pending、reject、fat-finger confirm |
| 实时状态 | 领先/被超越、延时、截拍、成交、取消 |
| 实时排行榜 | Top N、我的排名、与领先者差距，数据来自服务端 accepted bid 聚合 |
| 竞价氛围 | 领先、被超越、延时、成交事件驱动短动画；提示音/震动默认静音可开关 |
| 断线恢复 | 恢复中状态、禁用危险 CTA、snapshot 后恢复 |
| 订单 | 我是赢家时 mock 支付，非赢家看结果 |
| 历史 | 我的出价、成交/失败结果、订单状态 |

## 两个核心挑战

### 复杂竞拍规则

必须覆盖：

- 0 元起拍：第一笔至少 `start + increment`。
- 固定加价：出价差额必须是 `increment` 的正整数倍。
- 封顶价：cap 必须可达；`amount == cap` 成交，`amount > cap` 拒绝。
- 自动延时：最后窗口内出价延时，end_at 单调不减。
- 异常取消：与 bid/cap/end job 竞争时只有一个终态。
- 状态机：所有非法转换被服务端拒绝。

### 毫秒级实时同步

必须覆盖：

- server-authoritative time。
- auction seq 连续且客户端可检测 gap。
- outbox commit 后崩溃可恢复投递。
- WS 显式 ping/pong 心跳、断线/重连/慢客户端/背景页。
- snapshot fallback 有防击穿。
- UI 不做乐观成交，不从本地倒计时裁判。

## 加分项路径

### 极致竞价氛围体验

必须基于服务端事件，不允许前端伪造成功或插入假出价：

- 事件动画：`bid_accepted` 后展示“领先”；当前用户被其他有效出价超过时展示“被超越”；延时和成交分别展示短反馈。
- 实时排行榜：H5 展示服务端 Top N、我的排名、我的最高出价、与领先者差距。
- 紧张感细节：倒计时展示服务端时间推导结果；提示音和震动默认关闭，可由用户主动开启；效果必须短、非阻塞，并通过 longtask 测试约束。
- 可恢复一致性：排行榜和动画只作为体验层，成交、领先者、订单仍以 PostgreSQL 竞拍真源和 outbox/WS 事件为准。

### 高并发架构硬核优化

继续保持现有证据纪律：

- PostgreSQL 行锁与幂等记录保证竞拍真源；Redis/WebSocket 只做投影和投递。
- Redis history/snapshot、transactional outbox、slow-consumer 断开、admission control、诊断/flight recorder 必须有测试或证据。
- 未经原生环境基线验证，不宣传 QPS、P99 或 1000+ 在线容量。

## 评分短词展开

| 官方短词 | 工程要求 |
|---|---|
| 完整工程链路 | 上架、规则、竞拍、成交、订单、支付、历史、诊断 |
| 竞拍数据采集 | bid/reject/detail/chat/join/ws/order 事件 |
| 数据治理 | schema、枚举、trace_id、幂等、ordering、replay 边界 |
| 后端服务 | rule validation、row lock、state machine、outbox、scheduler |
| 接口网关 | auth、ACL、schema、rate limit、idempotency、错误码 |
| 前端交互 | H5 状态矩阵、PC 控制台、弱网/pending/disabled、排行榜、事件氛围反馈 |
| 系统可用性 | 重启恢复、Redis down、DB lock timeout、outbox poison |
| 性能 | load model、benchmark、raw output、瓶颈定位 |
| 稳定性 | backpressure、snapshot 防击穿、hot table 观测 |
| 可观测性 | 诊断页、anomaly、metrics、logs、drilldown |
| 核心挑战优化 | final-second bid、recoverable realtime、failure gates |
| 独特思考 | server-authoritative correctness + recoverable realtime |

## 最终亮点

1. **服务端权威竞拍正确性**：PG row lock、幂等、规则矩阵、终态竞争测试。
2. **可恢复实时链路**：transactional outbox、分片保序 relay、Redis history、snapshot fallback、WS 背压。
3. **失败可见可诊断**：异常 producer、诊断页 drilldown、failure gates。
4. **证据纪律**：所有性能数字来自可复现 baseline。
5. **事件驱动 H5 体验**：领先、被超越、延时、落锤、恢复中、排行榜都绑定真实服务端事件/查询。

## 非目标

- 真实支付。
- 真实直播推流。
- 微服务拆分。
- AI 参与出价/取消/成交判定。
- Redis 作为唯一真源。
- 未实测的大规模性能宣传。
