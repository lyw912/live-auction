# Live Auction — 直播竞拍全栈系统

> 抖音电商 AI 全栈挑战赛参赛作品

一套落地了 **Redis Lua 原子决策 → Kafka 有序决策 WAL → PostgreSQL 结算真相 → Reconciler 校验闭环** 的失败关闭(fail-closed)实时竞拍内核，配合 AI 智能运营助手、支付宝沙箱支付、WebRTC 直播推流与极致竞价氛围体验。

```
商品上架 → 规则配置 → 排期开拍 → 实时出价(原子Lua决策) → 动态排名
→ 反狙击延时/封顶成交 → Kafka WAL → PG 结算建单 → 支付宝沙箱支付
→ 监控诊断 / AI 解说 / 复盘高光
```

## 技术栈

| 层 | 技术 | 说明 |
|----|------|------|
| 后端 | Go 1.26 · Chi · pgx · go-redis · kafka-go | 模块化单体，嵌入式 5 Worker |
| 热决策 | Redis 7 (Lua) | 单写者原子决策引擎，AOF appendfsync=always |
| 持久化 | Kafka 3.9 (KRaft) | 有序决策 WAL/Fence，group-commit |
| 真相 | PostgreSQL 16 | 结算/审计/订单，exactly-once 边界 |
| 前端 H5 | React 18 · TypeScript · Vite | 服务器权威 · 断线重连 · 氛围动效 |
| 前端 PC | React 18 · Arco Design · Vite | 商家后台 · AI 工具 · 监控 |
| AI | OpenAI-compatible relay · GLM-4.6V/Qwen-VL 等可配 | 选品/解说/哨兵/Q&A/复盘 |
| 支付 | Alipay OpenAPI Sandbox · 本地 fake provider | 电脑网站/手机网站支付表单、支付查询、回调/事件入账 |
| 直播 | MediaMTX · WHIP/WHEP WebRTC | PC 摄像头推流，H5 低延迟观看，MP4 显式 fallback |
| 可观测 | Prometheus · Grafana · Tempo · OTel | 指标/追踪/告警 |
| 存储 | MinIO (S3) | 商品图片 |
| 测试 | Go tests · Playwright · K6 · Toxiproxy · PTS | 90+ 集成 · 35 正确性门禁 |

## 快速启动

```bash
# 1. 基础设施
cd infra && docker compose up -d

# 2. 数据库迁移
cd backend && make migrate-up

# 3. 后端（配置 .env 后启动）
cp .env.example .env   # 填入 AI API Key
make run               # :18080

# 4. 前端
npm run dev:h5         # H5 竞拍端 :5276
npm run dev:pc         # PC 商家端 :5277
```

详细架构与答辩阅读入口见 [docs/README.md](docs/README.md)，本地基础设施说明见 [infra/README.md](infra/README.md)。

## 目录结构

```
backend/                    Go 后端 (25,928 行)
  cmd/server/               入口：嵌入 Outbox Relay / Settlement Worker / AI Worker / Scheduler
  internal/
    redisengine/            热竞价引擎 — Lua 决策 + Relay + 结算 + 重建 (5,495 行)
    auction/                领域模型 / 规则校验 / 出价逻辑 / 状态机
    ai/                     AI Generator / 安全护栏 / Provider 路由
    gateway/                Chi 路由 / 鉴权 / Admission / 支付宝 / Handler
    realtime/               WebSocket Hub / 排行榜投影 / 背压
    reconcile/              Redis↔Kafka↔PG 漂移检测
    scheduler/              终态转换(SOLD/ENDED) + Fencer
    outbox/                 变更日志投递
    config/                 91 项可配参数
  migrations/               31 个 goose SQL 迁移
frontend/
  mobile-h5/                H5 竞拍端 — 服务器权威 / 氛围引擎 / 断线重连
  pc-console/               PC 商家端 — Arco Design / AI 工具 / 飞行记录器
  shared-design/            设计令牌 (CSS Variables)
tests/
  pts/                      PTS 正确性校验器 (35 门禁) + JMeter 场景
  load/                     K6 压测脚本 (18 个)
  chaos/                    Toxiproxy 混沌测试 (8 场景)
  e2e/                      Playwright E2E (7 spec)
infra/                      Docker Compose / MediaMTX / systemd / nginx
docs/
  README.md                 评委阅读入口
  00-project/               项目总览 / 产品范围 / 覆盖矩阵
  01-architecture/          系统架构 / 数据一致性 / 技术选型
  03-backend/               出价 / 结算 / 恢复 / AI / 工程难点
  05-frontend/              H5 与 PC 闭环
  09-judge-defense/         答辩材料
```

## 架构概览

### 热出价路径

```
HTTP POST /bids
  → Gateway (鉴权 / ACL / Admission 限流 / 幂等门)
  → Redis Lua 原子决策 (一次 RTT)
      幂等 → 状态校验 → 5条规则+误触 → engine_seq++ → 决策流
  → Group-Commit Relay → Kafka AppendBatch(acks=all)
  → HTTP 返回 ENGINE_ACCEPTED/REJECTED/SOLD + durability_status
  → Settlement Worker 消费 Kafka → 幂等写 PG → 建单 → Outbox → WS 广播
```

### 四层持久性

| 层 | 响应字段 | 含义 | 恢复路径 |
|----|---------|------|---------|
| Redis AOF | `ENGINE_DURABLE` | 本地持久化 | AOF 重放 / 从 Kafka+PG 重建 |
| Kafka ACK | `KAFKA_ACKED` | 分布式 WAL 持久化 | RF=3 (生产) |
| PG 结算 | `SETTLED` | exactly-once 边界 | 幂等重放 |
| 订单 | order exists | 业务闭合 | 幂等建单 |

### 失败关闭

- Redis 丢失 → 暂停(RECONCILING) → 从 Kafka 高水位+PG 重建 → 校验 → 恢复
- Kafka 故障 → 熔断器 → 零额外延迟降级到 ENGINE_DURABLE
- 状态不完整 → RECONCILING，绝不凭空 ACCEPT

## 核心特性

**竞拍规则（零漏洞）** — 0 元起拍 / 加价网格校验 / 封顶自动成交(可达性+网格对齐验证) / 反狙击延时(+绝对硬顶防 bot) / 误触二次确认(重跑全部守卫) / 主播随时取消

**分布式正确性** — 幂等出价(请求哈希冲突检测) / engine_seq 全序无空洞 / at-least-once WAL + 幂等消费者 = effectively exactly-once / Reconciler 漂移检测 / 35 条正确性门禁

**实时体验** — 服务器权威(无乐观成功) / 时钟漂移免疫倒计时 / 断线重连+序号空洞快照恢复 / 房间级 WS 隔离+背压

**竞价氛围** — 领先/被超越/自动防守/延时/落槌分阶段动效+音效+触觉 / 心跳音床 / 竞速榜瀑布 / 热度计

**支付宝沙箱闭环** — 中拍订单发起 `alipay.trade.page.pay` / `wap.pay` 表单跳转，回调与主动查询都落到统一 payment event，订单详情展示 provider、trade status、支付宝交易号与处理时间；本地 fake provider 只作为配置缺失时的开发兜底。

**低延迟直播链路** — PC 端摄像头通过 WHIP 推到 MediaMTX，H5 通过 WHEP 播放；公网 IP HTTPS、UDP/TCP ICE、端口代理和浏览器安全上下文都有独立配置。

**AI 智能运营** — 选品 Copilot / 自动解说 / 哨兵告警 / 商品 Q&A / 复盘高光；AI 永不碰钱/胜者/终态

## 测试与验证

| 层 | 覆盖 |
|----|------|
| Go 集成测试 | 90+ 测试，testcontainers (PG/Redis)，CI 每 push 运行 |
| PTS 正确性校验 | 28 条 P0 不变式 + 7 条基础设施门禁，P0 零容忍 |
| K6 压测 | 18 脚本：最后一秒竞争 / 稳态浸泡 / WS 扇出 / 重连风暴 |
| 混沌测试 | 8 场景：Redis 不可用 / FLUSHALL / Kafka 宕机 / 结算崩溃 / PG 不可用 |
| Playwright E2E | 7 spec：H5 / PC / 氛围引擎 / 视觉回归 |

## 配置

通过 `.env` 管理，关键参数：

```bash
# 出价引擎
BID_AUCTION_LIMIT_PER_SECOND=80    # 单拍品决策频率上限
BID_USER_LIMIT_PER_SECOND=3        # 单用户限频
BID_ENGINE_KAFKA_APPEND_TIMEOUT=750ms

# WebSocket
WS_QUEUE_MESSAGES=256              # 单连接背压队列
WS_RECOVERY_MAX_EVENTS=300

# AI
AI_PROVIDER_MODE=provider
AI_TEXT_MODEL=deepseek-v4-flash
AI_COMMENTARY_BATCH_SIZE=4

# 支付宝沙箱
ALIPAY_SANDBOX_ENABLED=true
ALIPAY_GATEWAY_URL=https://openapi-sandbox.dl.alipaydev.com/gateway.do
ALIPAY_PAY_METHOD=alipay.trade.page.pay

# WebRTC 直播
LIVE_DEMO_MEDIA_PROTOCOL=whep
LIVE_DEMO_MEDIA_URL=/mtx/auction-live/whep
```

完整参数见 `.env.example` (91 项)。

## 工程规则

- Redis 是热态原子决策，仅在 Kafka WAL/Fence 和 Reconciliation 合同下运行
- PostgreSQL 是结算/审计/订单真相，不做热路径行锁
- 客户端时间永不决定关闭时间或胜者
- HTTP 状态不是拍品结果，检查 `ENGINE_*` / `durability_status` / `settlement_status`
- 每个出价尝试幂等
- 无性能数字未经原始基线证据

## 代码规模

| 分类 | 行数 |
|------|------|
| Go 后端源码 | 25,928 |
| Go 测试代码 | 15,578 |
| 前端源码 | 17,710 |
| 测试脚本 | 17,152 |
| **合计** | **~76,000** |

## License

MIT
