# 直播竞拍全栈系统 · 评委深度评审报告

> 评审日期：2026-06-05　评审基准 commit：`1788333`
> 评审依据：`抖音电商AI全栈课题-直播竞拍全栈系统（宣讲版）.md`（不可变官方源）
> 评审方法：四类资深评委视角独立交叉审查 + 代码级取证 + 联网事实核验（Tavily / Fetch）
> 评审定位：以 TikTok/字节成熟可落地项目标准打分，而非 hackathon 完成度标准

---

## 0. 评审方法与边界声明

本报告**不以文档为准，以代码与产物为准**。所有结论均给出 `file:line` 或产物路径作为证据。
四位虚拟评委（均 10 年以上经验）：

- **D1 资深后端/规则工程师**：竞价规则零漏洞、并发正确性、幂等。
- **D2 架构师**：Redis/Kafka/PG 热路径、一致性、SPOF、容量天花板。
- **D3 测试/SRE**：证据诚实度、压测语义、故障注入、可观测性与告警。
- **D4 前端/产品经理**：功能闭环、实时体验、跨端状态、可维护性。

**前置声明（评分时已计入）**：用户明确说明（1）"极致竞价氛围体验"未做、不参与考核；（2）受成本限制，单 Redis / 单 Kafka / 单 PG、压测规模与重复次数不足属正常。本报告对这两点**不做扣分式攻击**，但会在"竞争力判定"里说明它们如何影响夺冠概率。

**官方评分表缺口**：宣讲版评分表只列出"技术实现与工程完整度 50% + 技术深度与创新性 25% = 75%"，**剩余 25% 在原文档被截断**（极可能是"产品体验/前端交互"或"工程规范/答辩"）。本报告按 50/25 逐条打分，并对缺失的 25% 按通行口径（产品完整度 + 文档/可维护性）给出参考分。

---

## 1. 执行摘要与总评

**一句话结论**：这是一个**工程深度与证据诚实度都明显高于训练营平均水准**的项目，热路径架构（Redis 单写定序 + Kafka WAL + 幂等结算 + 对账器 + Redis 重建）已具备**准生产级**思维；但存在 **1 个 P0 正确性漏洞（主播取消未栅栏 Redis 热引擎）**、**1 个名不副实的 epoch 围栏**、**缺少真正的异常告警**，以及若干"宣称强于实测"的表述。修掉 P0 + 补 1~2 个 HA 证据后，足以进入决赛梯队前列。

**总分（原始评审）：86 / 100**　|　**Phase 1 修复后预估：91~93（需在完整 Redis/Kafka/PG 环境复跑集成验证）**

| 维度 | 权重 | 得分 | 评语 |
|---|---|---|---|
| 技术实现与工程完整度 | 50% | 42 / 50 → 46+ | 闭环完整、可用性/一致性/可观测性强；原主要失分 **取消栅栏漏洞** 与 **无告警** 已进入 Phase 1 修复 |
| 技术深度与创新性 | 25% | 23 / 25 | 单写定序 + WAL + 对账 + 重建是真·差异化；选型与场景高度契合 |
| 产品/前端/文档（缺口 25% 参考口径） | 25% | 21 / 25 → 22+ | 功能近乎全闭环、服务端权威；氛围未做（已豁免）、前端巨石化仍在；订单详情偏薄已补详情抽屉 |

---

## 2. 核心挑战逐条拆解

### 2.1 挑战一：复杂规则的逻辑攻坚（"零漏洞"）

规则实现集中在两处：原子决策 Lua（`backend/internal/redisengine/engine.go:61-355`）与创建期校验 `backend/internal/auction/rules.go`。

| 规则 | 要求 | 实现 | 证据 | 判定 |
|---|---|---|---|---|
| 0 元起拍 | 从 0 起，人人可参与 | `start_price` 可为 0；首单 `base=start_price`，门槛 `base+increment` | Lua:276-280, 293 | ✅ 正确 |
| 加价幅度 | 必须按固定幅度递增 | `amount>=base+increment` 且 `(amount-base)%increment==0` | Lua:293-298；`rules.go:106-112` | ✅ 正确，且与创建期校验同源 |
| 封顶价自动成交 | 达上限自动成交 | `amount==cap → ENGINE_SOLD`；`amount>cap → 拒` | Lua:272-274, 305-307 | ✅ 正确 |
| ↳ **封顶价网格可达性** | （隐含）cap 必须落在网格上，否则永远拍不到 | 创建期强制 `cap>=start+increment` 且 `(cap-start)%increment==0`，并返回 `suggested_caps` | `rules.go:57-82`（`CodeInvalidAuctionRuleCapUnreachable`） | ✅ **加分**：主动堵住了我第一时间想攻击的边界 |
| 自动延时 10-30s | 结束前有人出价则延长 | 软关闭：`(end-now)<=window` 且 `extend_count<max` 时 `end+=extend_by`，并 clamp 到 `absolute_end_ms` 硬顶 | Lua:309-321 | ✅ 正确，且有**防无限延时硬顶**（超出 brief） |
| ↳ 延时窗口取值 | brief 写"10-30 秒" | 创建期强制 `extend_window∈[10,30]`、`extend_by∈[10,30]`、`max_extend∈[1,10]` | `rules.go:44-52` | ✅ **逐字命中 brief** |
| 异常取消 | 主播可随时取消异常竞拍 | Phase 1 已补 `Cancel → FenceAuction` + `checkTerminalFenced` 对账修复 | `AuctionHandler.Cancel`；`Engine.FenceAuction`；`checkTerminalFenced` | ✅ 设计闭环已补；需完整 Redis 集成环境复跑新增测试 |
| （额外）自我领先拒绝 | brief 未要求 | 当前最高者不能继续抬自己价（防自抬价） | Lua:266-268 | ⚠️ 设计选择，建议在文档显式说明，避免被判为"偏离常识" |

**D1 评语（更新）**：规则数学是我审过的同类项目里最干净的之一——网格锚点在 Lua 与 Go 校验两侧一致、cap 网格可达性被提前堵死、延时区间逐字命中 brief、还附带防无限延时硬顶。原始评审指出的"异常取消"热引擎漏洞已在 Phase 1 补齐取消栅栏与对账修复；当前剩余要求是在完整 Redis/Kafka/PG 环境复跑新增集成测试和故障场景。

### 2.2 挑战二：毫秒级实时同步

| 能力 | 要求 | 实现 | 证据 | 判定 |
|---|---|---|---|---|
| WebSocket 长连接 | 房间级隔离 | 子协议票据鉴权 `['auction.v1','ticket.…']`；房间 hub 隔离 | `mobile-h5/src/main.tsx:1609-1624`；`internal/realtime/hub.go` | ✅ |
| 心跳保活 | 心跳保活 | **服务端**每 20s ping、5s 超时则关连接，并打点 `auction_ws_heartbeat_*` | `internal/realtime/server.go:379-397` | ✅ **达成（服务端）**。浏览器 JS 无法主动发 WS ping 帧，会自动回 pong；客户端缺应用层主动探活（半开服务端检测稍弱，但有重连+last_seq 兜底） |
| 断连重连 | 网络波动自动重连 | 指数退避+jitter，遵守服务端 `Retry-After` | `mobile-h5/src/main.tsx:1597-1604`；`realtime.ts` | ✅ |
| 断点续传/补洞 | 排名一致、不错乱 | 重连 URL 带 `last_seq` 让服务端补发；序号单调去重；解析失败回退快照；2.5s 快照轮询兜底 | `main.tsx:1623, 1387, 1637, 1671-1678` | ✅ **强项** |
| 倒计时精确到毫秒 | 不能有偏差 | 锚定 `serverTimeMS` + 本地步进；<10s 显示十分位 | `main.tsx:305-313, 294-297` | ⚠️ **半达成/略有夸大**：渲染 tick 为 **1000ms**（`:734`），毫秒位每秒才刷新一次；且锚定无 RTT 补偿、事件间靠本地时钟，长会话/标签页节流会漂移 |
| 排名/被超越/延时/结束提醒 | 关键提醒 | 全部由服务端事件/`my_rank` 驱动，无客户端臆测 | `main.tsx:1419-1471, 1899-1902` | ✅ 服务端权威 |
| 防抖节流 | 技术关键词 | in-flight 锁 + 服务端 `Retry-After` 冷却 | `main.tsx:1684, 1724` | ✅ |

**D4 评语**：实时这块"骨架"是对的——服务端权威、重连补洞、序号去重都做了，断连恢复是亮点。两个会被现场抓的点：①**"精确到毫秒"是夸大**，1s 渲染 tick 下十分位 90% 时间是停滞的，评委录屏最后 10 秒就会发现；②倒计时只锚定不做 RTT/时钟偏移校正。这两点是**打磨问题不是架构问题**，半天可修。

---

## 3. 功能模块与加分项对照

### 3.1 商家/主播端（PC 管理后台）
| 功能 | 状态 | 证据 |
|---|---|---|
| 竞拍发布（图片预签名上传 + 规则配置） | ✅ 完整 | `pc-console/src/main.tsx:785-819` |
| 商品管理（状态/进度/结果分组展示） | ✅ 完整 | `:280-281, 244-258` |
| 修改未开始竞拍规则 | ✅ **保守冻结 + 可撤回排期**：`DRAFT` 可改；`SCHEDULED` 不直接改价以保护已排期买家预期，商家可先“撤回排期”回到 `DRAFT` 再修改并重新排期 | PC `AuctionCommandPanel`；后端 `Unschedule` |
| 取消异常竞拍 | ✅ 前端已接 API；后端已补 Redis 热引擎栅栏和对账修复（见 Phase 1 FIXED / P0） | `AuctionHandler.Cancel`；`Engine.FenceAuction`；`checkTerminalFenced` |
| 订单管理（成交自动生成 + 查看详情） | ✅ 自动生成 + 订单详情抽屉：展示中标人、成交价、保证金、支付/过期时间、provider payment id，并可跳转 flight recorder 审计 | PC `OrdersPanel` / `OrderDetailDrawer` |

### 3.2 用户端（移动 H5）
直播间（固定视频，brief 允许）、竞拍浏览、参与人数（`accepted_bidder_count` 真实服务端值）、手动出价（带 `Idempotency-Key`）、实时排名、四类提醒、**模拟支付**（`/api/orders/{id}/pay-mock`，幂等、winner-only、保证金 10%、15 分钟过期、HMAC 自 webhook 收敛）、**历史记录**（真实接口）——**全部为真实后端调用，无硬编码**。证据：`mobile-h5/src/main.tsx:1698-1957`；`backend/internal/auction/bid.go:356-459`。

**D4 唯一硬伤**：`StateMatrixTabs` 演示组件**被留在了线上 UI**（`main.tsx:2050, 2967-2988`），可点击切换"成交/流拍/已取消"等状态直接 `setSelected(...)`，会渲染出中标横幅/去支付 CTA。虽然支付按钮仍受真实 `payableOrderID` 闸门保护（钱动不了），但这是一个"看起来像伪造客户端真相"的演示捷径，与项目自己"绝不伪造客户端真相"的契约自相矛盾，**上线前必须摘掉**。

### 3.3 加分项（硬核高并发架构）
| 加分方向 | 实现度 | 证据 |
|---|---|---|
| Redis 分层缓存 / 读写分离 | ✅ 热状态决策引擎 + 快照/投影读路径 | `redisengine/engine.go`；`gateway/redis_guard.go` |
| 分布式锁/幂等"绝不一笔扣两次" | ✅ **强**：Lua 内原子幂等（同 key 同 hash 重放、同 key 异 hash 冲突）；结算侧 epoch+seq CAS；支付幂等+重复 webhook 去重 | Lua:81-90;engine.go:1659-1695;bid.go:356-459 |
| WebSocket 房间级路由隔离 1000+ | ✅ 隔离实现；并发证据见 §5（实测为 2×500 形态，非单窗 1000） | `realtime/hub.go`；`s1-review` |
| 极致竞价氛围 | — 未做（已豁免） | — |

---

## 4. 关键缺陷清单（按严重度排序）

> **Phase 1 FIXED（2026-06-05）**：本节前五项高 ROI 缺陷已进入第一期修复：
> - P0 取消栅栏：`AuctionHandler.Cancel` 成功取消后 best-effort 调用 `Engine.FenceAuction`；Redis 热态写入 `paused=1`、`pause_reason`、终态 `status=CANCELLED`；`Worker.Reconcile` 新增 `checkTerminalFenced`，发现 PG `CANCELLED/ENDED` 但 Redis 仍 `ACTIVE` 时修复并递增 `auction_reconcile_terminal_unfenced_total`；新增 `TestCancelFencesHotEngine` / `TestReconcileFencesTerminalHotEngine`。
> - `engine_epoch`：`rebuildRedisFromCheckpoint` 在 checkpoint rebuild 时 `UPDATE auctions SET engine_epoch = engine_epoch + 1 RETURNING engine_epoch`，并用新 epoch 写回 Redis snapshot；settlement 批处理已区分旧 epoch 已覆盖消息（skip）和旧 epoch 未覆盖消息（fail closed）。
> - 告警链路：Prometheus 已接入 Alertmanager，新增 settlement lag / bid p99 / terminal-unfenced 等规则；`/api/internal/alert-webhook` 会把 firing alert 写入 `system_anomaly_events`。
> - 倒计时：H5 tick 已改为 100ms，倒计时用 `serverTimeMS` 锚点 + 本地同步时刻计算 elapsed，避免客户端 epoch 与服务端 epoch 直接相减。
> - `StateMatrixTabs`：仅 `import.meta.env.DEV && ?stateMatrix=1` 可用；生产构建产物不含 `stateMatrix`/`StateMatrixTabs`。

### 🔴 P0-1　主播"异常取消"未栅栏 Redis 热引擎（真·正确性漏洞）
- **FIXED 2026-06-05**：见 Phase 1 记录。剩余验证风险：当前本机未启动 Redis `127.0.0.1:6380`，新增 Redis 集成测试在本机按 helper 跳过；需在 infra Redis/Kafka/PG 环境中重跑。
- **事实**：`Repository.Cancel`（`repository.go:393-430`）只做 PG `UPDATE status='CANCELLED'` + 事件 + 提交；网关 `Cancel`（`auction_handlers.go:248-263`）只调它，**全链路无任何 Redis 引擎 pause/栅栏/终态写入**（已 grep 全仓确认）。
- **机理**：Lua 引擎只在 Redis key **缺失**时（`EXISTS==0`，`engine.go:92`）回读 PG 快照；热态存在时只信缓存里的 `status`。每次出价都 `PEXPIRE` 续期，热态**永不过期**。于是取消后：PG=CANCELLED，Redis 热态仍 `ACTIVE` → Lua `status~='ACTIVE'` 判定为假 → **继续接受出价 → 结算 → 为已取消竞拍生成订单**。
- **窗口**：从 PG 提交 CANCELLED 到该竞拍 `end_at` 到达（或热态 TTL 到期），**可长达整个剩余竞拍时长**。
- **为何严重**：①它是 brief 五条规则之一；②它发生在系统真正运行的 `redis_ledger` 热引擎模式，而**唯一的取消测试 `TestCancelActiveThenLaterBidRejects` 跑的是 PG 回退 lane**（`bid_integration_test.go:446`），给了虚假信心；③对账器 8 项检查里**没有一项**比对 PG 终态 vs Redis 栅栏，所以也不会被发现/修复。
- **修复**（半天）：取消时在 PG 事务内/旁同步将 Redis 热态置为 `paused`+终态（或 `requires_postgres`），并写一条终态决策入流；对账器新增检查 `PG terminal ⇒ Redis fenced`；把取消测试改为跑 `Engine.PlaceBid`。

### 🟠 P1-2　`engine_epoch` 围栏名不副实
- **FIXED 2026-06-05**：checkpoint rebuild 会自增 epoch，并让旧 epoch settlement 消息按覆盖状态 skip/fail-closed，不再把 epoch 当静态字段。
- **事实**：`engine_epoch` 迁移里 `DEFAULT 1`，全仓**只被 SELECT 读、从不 +1**（resume/rebuild/Lua 均不自增，`writeRedisStateSnapshot` 还写回同值）。结算里的 epoch CAS 结构在、但一个"复活的旧 Redis"携带同样的 epoch=1 会照样通过。真正防脑裂靠的是 seq 单调 + checkpoint 哈希，**epoch 是看起来像围栏的死脚手架**。
- **修复**（1~2 小时）：每次 resume/rebuild 自增 epoch 并先持久化再开引擎；否则从文档移除"epoch 防脑裂"表述以免被现场 grep 打脸。

### 🟠 P1-3　有监控、无告警
- **FIXED 2026-06-05**：`infra/prometheus/prometheus.yml` 配置 Alertmanager，`infra/alertmanager/alertmanager.yml` 配置 webhook receiver，规则文件覆盖 settlement lag、bid p99、terminal-unfenced 等，后端 webhook 写入 `system_anomaly_events`。
- **事实**：`observability/metrics.go` 手写 Prometheus 暴露、`monitor_handlers.go` 有看板、有 readiness/liveness/OTel——但全仓**无任何 alert rule / 通知 / 阈值告警通道**（grep 无 alert/notify/pagerduty/webhook 告警）。brief 评分明确含"异常告警"。内部 `pause`+对账是**故障反应**，不是**对运维的告警**。
- **修复**（半天）：补一份 Prometheus alert rules（kafka lag、settlement gap、reconcile pause、ws heartbeat timeout、p99 越界）+ 一个 webhook/日志告警出口即可拿满该子项。

### 🟡 P2 级（容量/证据/工程）
- **P2-4 拒单写放大 + 单分区结算容量上限**：拒单仍必须进入 Redis Stream→Kafka→PG，因为它是用户可见的最终裁决，承担幂等重放、拒绝理由、decision basis 与争议审计。当前已不再是原始逐条低效形态：已保留 Redis relay batch、Kafka `AppendBatch`、set-based rejected settlement、accepted contiguous-prefix batch、settlement success log suppression，并有 S2 decision/reject-heavy convergence PASS（49,049 decisions，49,043 rejected，最终 Kafka/Redis/PG/outbox 全清）。剩余问题是下一阶段容量上限：单 hot auction 的 Kafka partition/`engine_seq` 仍必须有序推进，不能靠多 worker 并行打散；未来优化方向是窄 `bid_decision_audit`、按 auction/time 分区或 `COPY`，但不能删除拒单审计/幂等。
- **P2-5 单 Redis 即单定序器、生产 HA 仍未实测**：短期已补 `REDIS_MODE=sentinel`/`redis.NewFailoverClient` 入口、Kafka RF=3/minISR=2 示例拓扑、Redis Cluster 启动拒绝与 key-slot 边界测试、HA 答辩文档。**仍不应宣称生产 HA 已证明**：当前没有 Sentinel failover、PG 主备切换、Kafka broker loss 的正式证据。**注**：单写定序本身是正当甚至高端的模式（见 §6 联网核验/LMAX），当前扣分点从"仅一句注释"降为"HA 拓扑未实测"。
- **P2-6 Redis-local durability vs `ENGINE_DURABLE` 语义**：本地 infra 已固定 Redis AOF `appendfsync always` + `noeviction`，所以不再是默认 `everysec` 丢 1s 的配置问题；剩余风险是语义边界：`ENGINE_DURABLE` 表示 Redis 热态 + Redis Stream + 幂等记录已写入，不等于 Kafka RF=3/minISR=2/`acks=all` 的 quorum durable，也不等于 Redis Sentinel/托管 HA failover 零丢失。生产强语义口径应区分 `ENGINE_DURABLE`（Redis-local recorded）、`KAFKA_ACKED`（ledger durable）和 PG `SETTLED`（财务/订单真相）。
- **P2-7 "1000 最后一秒"实为 2×500/500ms**：S1 PASS 全局跨度 1351ms、500 单/agent×2，团队**已诚实披露**"未证明 1000 落在单一全局 500ms 窗口"。p99=23ms 真实，但峰值瞬时并发约 500 而非 1000。
- **P2-8 峰值资源无证据 / n=1**：PASS 目录的 mpstat/top 采于跑后 ~90s（97% idle），**无突发期内 CPU/Redis/PG 利用率**；每场景仅 1 次成功跑，无方差/置信区间。
- **P2-9 S3 实测 3000 围观（非 brief 万人）**、S4 故障在 200VU 而非 1000 突发、S5 k6 导出里部分 threshold 标记 `false`（原始计数其实达标，需解释）——均已部分披露。
- **P2-10 前端巨石化**：H5 3107 行、PC 2118 行单文件 `main.tsx`，~50 个 useState、无组件拆分/路由/状态库/前端单测。代码卫生其实好（strict TS、无 any/console/TODO），但可维护性/可测性会被资深前端直接点名。
- **P2-11 改规则仅 DRAFT、订单详情偏薄**：已修正为“排期冻结但可撤回排期后修改”的工业化路径，并补订单详情抽屉（见 §3.1）。

---

## 5. S1–S5 证据诚实度（D3 视角）

**罕见地诚实，是本项目的隐形加分项。** 已核验：
- **真实同步决策 p99**：S1 `2MLCX7WG`，100% 采样 N=1000，`elapsedTime` p99=**23ms**、max 28ms；采样体是同步 `200` 携带 `ENGINE_*`+`ENGINE_DURABLE`+`engine_seq`+`decision_basis`，即 RTT=最终决策延迟（非 202 假象）。
- **真对账器**：`verify-l4b-pts-correctness.sh`（56KB SQL 闸门），41 条 PASS，含 `auction_winner_matches_highest_accepted=t`、拒单理由逐条成立、`engine_seq` 1..1000 无缺/无重、redis/kafka/pg 一致、consumer lag=0、outbox drained。
- **真故障注入**：`live-auction-toxiproxy` 容器，7 类故障（Redis kill/FLUSHALL、Kafka、PG、backend SIGKILL、Redis+Kafka 同杀、Redis 网络延迟），fail-closed 证据齐（`ENGINE_PAUSED=600`、故障窗口内零接受、收敛后 open_settlements=0/redis_pending=0/kafka_lag=0），RTO 实测。
- **真重连**：S5 干净重连 34,814 次 + toxiproxy 切断 8,849 次，`seq_gaps=0`、`duplicate_seq=0`、`truth_mismatch=0`。
- **未藏失败**：把 p99=134ms 的失败诊断跑 `TGLBX7GG` 保留在索引并标注 INVALID、给出根因（跨 agent JMX 栅栏偏移，非后端）。

**D3 会按的点**：①1000 并发的真实形态（2×500）；②突发期内无资源利用率证据，50ms 下有无余量未证；③全链路单实例、横向扩展零实测；④每场景 n=1。

---

## 6. 联网事实核验（依据，非臆断）

- **软关闭/防狙击是行业标准**：eBay 硬关闭助长 sniping；专业拍卖行普遍用 soft-close 自动延时（Lightning Auctions 等用 1 分钟级窗口）。→ 项目实现方向正确；brief 的 10-30s 比真实拍卖行（≥1min）短，但**逐字符合 brief**，不扣分；可在答辩点出"窗口可配"。
- **单写定序是高端模式**：LMAX Disruptor / Martin Fowler / Chronicle 均确立"single-writer principle"——单核独写消除竞争、避免锁与缓存行抖动，正是交易所撮合引擎用法。→ **支撑团队"单 Redis 定序是特性非缺陷"的论点**，是技术深度维度的有力背书。
- **Redis 持久性**（redis.io 官方）：`appendfsync always`≈0 丢失但很慢；`everysec`（默认）**崩溃最多丢 1s**；`no`≈最多 30s。→ 支撑 P2-6：`ENGINE_DURABLE` 在单节点 everysec 下高估了真实持久性。
- **WebSocket 心跳**：RFC6455 ping/pong 帧专为探测半开连接而设；TCP keepalive 默认 2h 过久、NAT/代理常 ~60s 回收空闲连接。→ 服务端 20s ping 是对的做法；浏览器 JS 不能主动发 ping 帧，故客户端探活只能靠应用层 JSON 心跳或依赖重连兜底——本项目选了后者，可接受。

---

## 7. 真正的亮点（公允陈述）

1. **服务端权威贯穿前后端**：客户端从不本地判定 winner/price/accepted，pending/uncertain/reconciling 如实呈现——这是 brief 最难的部分，做对了。
2. **结算幂等 + DLQ + 毒丸 pause**：epoch+seq 闸门→CAS→重复投递安全 no-op；毒丸有界重试后 DLQ + pause，不会永久堵分区。
3. **对账器是真的**：8 条 SQL 不变量、30s 跑一次、漂移即 pause、干净才解除。
4. **Redis 重建严谨**：checkpoint 哈希 + 与 PG 全字段相等 + 对账前后置闸门。
5. **订单/支付生命周期堪称范本**：`ON CONFLICT (auction_id)` 保证一拍一单（结算 worker 与调度器双写也安全）、支付幂等+重复 webhook 去重+成交收敛闸门。
6. **证据诚实度**：见 §5，远超同类。

---

## 8. 可落地优化路线图（按 ROI 排序）

| 优先级 | 动作 | 工作量 | 收益 |
|---|---|---|---|
| **P0** | 取消同步栅栏 Redis 热引擎 + 对账新增"PG 终态⇒Redis 栅栏"检查 + 取消测试改跑 Engine 路径 | 0.5 天 | 堵掉唯一 P0 正确性反例，"零漏洞"成立 |
| **P0** | 摘掉线上 `StateMatrixTabs` 演示组件 | 10 分钟 | 消除"伪造客户端真相"观感 |
| **P1** | 补 Prometheus alert rules + webhook/日志告警出口（kafka lag/settlement gap/reconcile pause/p99/ws timeout） | 0.5 天 | 拿满"异常告警"子项 |
| **P1** | epoch 真自增（resume/rebuild 时 +1 并先持久化），或删除"epoch 防脑裂"表述 | 1-2 小时 | 消除"安全剧场"被打脸风险 |
| **P1** | 倒计时最后 10s 用 rAF/100ms tick + 一次 RTT 偏移校正 | 半天 | 兑现"毫秒级"，现场录屏经得起看 |
| **P2** | 跑一次"突发期内"资源采集（mpstat/redis info/pg stat 与负载同窗）+ 同场景重复 3 次给方差 | 半天 | 证明 50ms 下有余量、消除 n=1 质疑 |
| **P2** | 客户端补应用层 JSON 心跳（探活半开服务端） | 2 小时 | 闭合心跳故事 |
| **P2（可选/工作量大）** | 结算 `COPY` 批量 + PG 分区 + 拒单与接受拆表 | 2-3 天 | 抬高单分区结算天花板，支撑更高 TPS |
| **P2（可选）** | 跑一次最小 HA 形态（Redis 副本切换 / Kafka RF=2 / PG 流复制）哪怕单次 | 1-2 天 | 把"横向扩展仅文档"变成"有 1 个证据点" |
| **P3（已豁免）** | 极致竞价氛围（领先/被超越动画、音效、排行榜张力） | 1-2 天 | 直接影响"产品体验"那 25%，对夺冠有实质帮助 |

---

## 9. 竞争力判定：能否夺冠？

**D2/D3 综合判断**：在"工程深度 + 证据诚实度"两个维度，本项目大概率处于**参赛前 10-15%**，对评委里的资深架构师/SRE 极具说服力——Kafka WAL/对账/幂等结算/Redis 重建这套组合拳，绝大多数参赛者拿不出来，且**敢于保留并标注失败跑**这一点会赢得专业评委的信任。

**但要"无争议夺冠"，当前有三块短板会被对手利用**：
1. **一个能被现场复现的 P0 正确性反例（取消未栅栏）**——这是最危险的，正确性类评委会一票压低；**必须修**。
2. **"异常告警"缺失 + epoch 围栏名不副实**——给挑剔评委"宣称强于实现"的口实。
3. **被豁免的"极致氛围体验"恰好是产品/前端评委最直观打分项**——一个功能稍弱但氛围炫酷、Demo 流畅的对手，可能在"产品体验那 25%"反超。

**最终建议**：先用 ~1.5 天清掉 P0 + 告警 + epoch + 倒计时 + 摘演示组件（全是低工作量高收益），项目即从"技术很硬但有反例"升级为"技术硬且无明显反例"；若行有余力，再补半天突发期资源证据 + 1-2 天氛围体验，则在**技术深度（25%）拿满、工程完整度（50%）逼近满分、产品体验（隐含 25%）不失分**，具备**冲击冠军**的完整面貌。

> **评委签名**：D1 后端规则 / D2 架构 / D3 测试·SRE / D4 前端·产品（交叉复核：服务端心跳认定、取消栅栏漏洞、epoch 围栏、证据形态均经二次代码取证）
