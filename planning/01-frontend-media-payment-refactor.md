# 直播竞拍前端 / 媒体直播 / 支付入口 —— 调研与重构规划

> 范围：Phase 1（双前端重构）、Phase 2（统一媒体播放契约）、Phase 3（服务端模拟直播流）。
> 性质：调研、方案设计、分阶段计划、架构边界。**本文不实现代码**。
> 红线：不得破坏现有竞拍/出价/结算/恢复核心；**正确性优先于一切**。
> 一等交付物：在上述红线之内，**视觉 / 交互 / 氛围（"效果"）与功能架构同等优先**——目标是「最震撼评委、体现工程能力、极致氛围、与竞品差异化」，不只是 UI 还有 UX（互动 + 反馈）。这一维度见 **第二部分 §11–§15**（v1 只覆盖了功能架构，本次补齐）。
> 锁定决策（2026-06-14）：移动端 = Web 全量重写（保留已验证的实时/出价/恢复内核，非 RN/Flutter）；两端组件体系统一为 **shadcn/ui + Tailwind + Radix**（替换 Arco 与自研 `tokens.css` 拼盘），共享一套设计语言。
> 可信输入优先级：当前代码 > `submission/` 最终材料 > 本地证据归档 > 外部工业参考。
> 文档放置：放在仓库顶层 `planning/`，与评委向 `docs/`（最终定稿）隔离，符合 `CLAUDE.md` 对 `docs/` 的约束。

---

## 0. 阅读地图

| 关注点 | 直接跳转 |
|---|---|
| 一句话结论 | [§1 执行摘要](#1-执行摘要) |
| 角色 / 范围不变量 | [§2 产品边界与角色](#2-产品边界与角色) |
| 选型怎么比出来的 | [§3 候选技术路线对比](#3-候选技术路线对比) |
| 模块怎么切、谁负责什么 | [§4 推荐架构与模块职责](#4-推荐架构与模块职责) |
| 接口/类型长什么样 | [§5 数据与 API 契约草案](#5-数据与-api-契约草案) |
| 怎么拆 2500 行巨石 | [§6 前端重构原则](#6-前端重构原则) |
| 分几步、每步交付什么 | [§7 分阶段计划](#7-分阶段计划) |
| 会踩什么坑 | [§8 风险清单](#8-风险清单) |
| 怎么算做完了 | [§9 验收标准](#9-验收标准) |
| 还没查清楚的 | [§10 待进一步调研的开放问题](#10-待进一步调研的开放问题) |
| **— 第二部分：视觉 · 交互 · 氛围（v1 缺的维度）—** | |
| 统一视觉识别系统 | [§11 视觉识别系统 Auction Terminal](#11-视觉识别系统auction-terminal统一设计语言) |
| 标志性交互 / 动效 / 多感官 | [§12 标志性交互 · 动效 · 多感官氛围](#12-标志性交互--动效--多感官氛围h5-主战场) |
| 控制台视觉与数据可视化 | [§13 PC 控制台：控制室视觉与数据可视化](#13-pc-控制台控制室视觉与数据可视化) |
| 视觉如何接进分阶段 | [§14 视觉/交互的分阶段落地](#14-视觉交互的分阶段落地接回-7) |
| 视觉验收 / 开放问题 | [§15 视觉/交互验收与开放问题](#15-视觉交互验收与开放问题) |

---

## 1. 执行摘要

### 1.1 当前真实状态（基于代码，不是基于旧文档）

三件事同时为真：

1. **两个前端功能很全，但架构是巨石。** 移动端 `frontend/mobile-h5/src/main.tsx`（约 2526 行，单 `App` 组件 150+ hooks）、`components.tsx`（约 2359 行，40+ 组件）、`domain.ts`（约 1085 行）；PC 端 `frontend/pc-console/src/{main.tsx ≈1394, components.tsx ≈2442, domain.ts ≈893}`。全部 `useState`/`useRef` 手搓，无服务端状态缓存库、无客户端状态库、无状态机库。服务端派生数据（价格、领先者、倒计时、连接态、订单态）和纯 UI 开关混在同一批 hooks 里，这是巨石化的根因。

2. **“直播”是写死的本地 MP4。** 媒体接缝非常清晰且单一：`LiveStage`（`frontend/mobile-h5/src/components.tsx:57`）里一行 `const videoURL = demoLiveVideoURL;` 写死成 `/demo/jade-live-loop.mp4`（`domain.ts:392`），再直接喂给 `<video className="live-video-bg" src={videoURL} … autoPlay muted loop playsInline />`。`displayMediaURL`（`domain.ts:4`）只负责把 MinIO 直链改写成 `/api/media/...`，那是**商品图/封面**，与**直播流**是两条独立的 URL。没有任何媒体能力抽象。

3. **支付在前端是 mock 按钮，但后端早已有真实的提供方边界。** 后端 `backend/internal/auction/bid.go` 已实现订单状态机（`ORDER_PENDING → PAYMENT_INITIATED → PAYMENT_SUCCEEDED → PAID → ORDER_EXPIRED`，`:56`）、`HandleProviderWebhook`（`:485`，签名校验 + `payment_events` 去重）、`ReconcileProviderPayments`（`:595`，轮询兜底，差异落 `PAYMENT_RECONCILE_MISMATCH` 异常）、`Sign/VerifyProviderWebhook`（`:680`），迁移 `202605240001_payment_provider_boundary.sql` 已落 `provider_payment_id` 唯一索引和 `payment_events UNIQUE(provider, provider_event_id)`。H5 `payOrder`（`main.tsx:2011`）已带 `Idempotency-Key`、已用 `order_status==='PAID'`（服务端真相）而非 HTTP 200 判成功、已用 `GET /api/users/me/orders` 重查兜底（`main.tsx:1148`）；PC 控制台对支付只读（`pc-console/src/components.tsx:2129`）。唯一不“面向未来”的点是买家入口打的是 `POST /api/orders/{id}/pay-mock`，且“跳转/确认”是内部伪造的。

### 1.2 总体判断

这不是“推倒重来”，而是**三个精准的结构化动作**，全部绕开 server-authoritative 的出价/决策/结算/恢复核心：

- **A. 前端去巨石化**：按“务实版 FSD（分层/切片/段）+ 依赖规则”重组，把服务端派生数据迁出 `useState` 进入 **TanStack Query v5（由 WebSocket 喂）**，纯客户端状态进 **Zustand**，把**出价交互**和**连接/恢复**两条最危险的状态流用 **XState v5** 显式建模，让“非法状态不可表达 / 失败即收口”。
- **B. 媒体能力抽象**：把那一行 `videoURL = demoLiveVideoURL` 升级成一个 **`MediaPlayback` 契约**（来源数组 + 协议 + 海报 + 是否直播 + 延迟目标 + 能力位），两个前端通过统一契约消费；demo 阶段返回 MP4 描述符，未来无缝替换为真实拉流地址，**不改调用方**。
- **C. 支付真实化**：不是从零设计，而是**把已存在的后端 provider 边界接到真实产品流**，把 `pay-mock` 泛化成 `POST /api/orders/{id}/payments` 返回 `client_action`，并**预留 Alipay 适配器接缝**（`out_trade_no`/`trade_no`/`notify_url` 异步通知/RSA2 验签/分→元换算只在适配器边界发生）。

- **D. 视觉 / 交互 / 氛围一等化（与 A 同权重）**：把"功能可用但太丑"的现状（自研 `tokens.css` 拼盘 + PC 端 Arco，用户多次改仍不满意）升级为**一套统一的 shadcn/ui + Tailwind + OKLCH token 设计系统**（代号 "Auction Terminal"），并在其上做**标志性交互 + 动效 + 多感官反馈**，把现有 `atmosphere.ts` 氛围引擎从"能力"提升为"评委记得住的差异化体验"。关键洞察：用户"改过好多次还是不满意"的根因不是配色，而是 `tokens.css`（91 行）是**没有体系的临时拼盘**（Arco 蓝 `#165DFF` + TikTok 粉 `#FE2C55` + 金 `#D4AF37` 混在一起）；缺的是 **token 体系**（OKLCH + shadcn 主题给的就是体系），不是某个颜色。详见 [§11](#11-视觉识别系统auction-terminal统一设计语言)–[§13](#13-pc-控制台控制室视觉与数据可视化)。

- **Phase 3 关键结论（媒体服务端）**：服务端模拟直播推荐 **MediaMTX + LL-HLS（hls.js，Safari/iOS 走原生 HLS）**，用 `ffmpeg -stream_loop -1 -re -c copy` 把一个 MP4 循环“假装现拍”，**不转码**，CPU 接近零，无需 UDP/TURN，原生 iOS 支持，最贴合 4 vCPU / 8GB 的 demo 机器；**WebRTC/WHEP 作为同一 `MediaPlayback` 契约下的后续升级**，是客户端换实现，不动产品流。理由与备选见 [§3.1](#31-媒体传输协议与服务端) 与 [§7 Phase 3](#phase-3服务端模拟直播流)。

### 1.3 一句话

> 用四个动作——**前端分层（A） + 媒体契约（B） + 支付接真（C） + 统一的 Auction Terminal 视觉/交互/氛围系统（D）**——把”功能齐全但又丑又巨石 + 写死的假直播 + 假支付按钮”升级成”既经得起工程审视、又能震撼评委的工业级前端”，**且一行都不碰已经过证据门禁验证的出价/结算/恢复内核**。功能正确性是地基（A/B/C 与红线），视觉与交互是评委看得见的差异化（D），二者同等重要、互不牺牲。

---

## 2. 产品边界与角色

### 2.1 两个前端，两种人，不合并

| | 商家 / 主播控制台（PC） | 用户竞拍端（Mobile H5） |
|---|---|---|
| 使用者 | 商家、主播、运营、SRE | 买家（出价人） |
| 心智 | **作业台 / 控制室**：上架、配规则、排期、启动、监控、AI 辅助、复盘、飞行记录器 | **看直播 + 抢拍**：看流、看价、出价、确认、看结果、付款 |
| 对交易的权力 | 发起运营操作 + **只读**展示后端权威结果；**永远没有“支付”控件** | 出价 + 付款（winner-only）；**永远不决定**成败/赢家/终态/价格 |
| 当前栈 | Arco Design 2.65 + lucide | 自研：motion 12.40 + @icon-park + lucide + canvas-confetti + Web Audio + 原生 WebSocket |

**红线（来自 `CLAUDE.md` 非协商项与现有代码语义）：**

- 不合并两个前端；不发明当前不需要的“主播采集端/买家角色”。媒体当前是**播放消费**，不是采集推流——Phase 1–3 没有任何摄像头/采集客户端（推流由服务端用文件模拟，见 Phase 3）。
- 不写死媒体能力（这正是要修的）。
- 客户端永不信任：客户端时间、当前价、赢家、终态、出价成功。终态只来自服务端事件或快照（`deriveCountdown` 只用“上次同步后的本地经过时间”，手机时钟跳变不致自行宣布结束）。
- 不拿 HTTP 状态当竞拍结果；要看 `ENGINE_*`、durability、settlement 字段。
- 出价路径必须 fail-closed 或 reconcile；幂等三层（HTTP `Idempotency-Key`==`client_bid_id` → Redis request hash → PG 唯一约束/engine_seq CAS）不得削弱。

### 2.2 本次范围 vs 明确不做

**做（Phase 1–3）：** 双前端架构/交互/视觉/实时状态/出价体验重构；统一媒体播放契约（demo 用视频文件占位，留干净的真实直播接缝）；服务端用文件模拟直播流、浏览器像真直播一样**拉流**消费；支付**入口/预订单/状态机 UI** 接到已存在的后端边界。

**只提不深入（Future）：** 真实商家摄像头/采集推流；真实支付宝结算落地；完整直播云生产化（多码率/CDN/鉴权回源）；原生 App。这些在架构上**预留接缝**（媒体源适配器、支付适配器），但本轮不实现。

### 2.3 必须保住的“契约级”不变量（重构的护栏）

这些是重构期间任何 PR 都不能回归的，建议作为 characterization test 的断言基线：

1. 出价响应解读：以 `BidResponse.{result, reject_reason, code, decision_status, durability_status, confirm_token, seq, engine_seq}` 为准（`mobile-h5/src/domain.ts`）。
2. WS 接入：`POST /api/auth/ws-ticket` 取一次性票据 → `new WebSocket(url, ['auction.v1', 'ticket.xxx'])`；票据 scope 绑定 room/auction/user 且消费即失效。
3. 断线恢复：`recoverFromSnapshot` 由 gap/outbox gap/断连/stale/手动刷新/出价后不确定触发；恢复来源 history/db/redis_stale/snapshot_unavailable。
4. 倒计时：`deriveCountdown(endAt, serverTimeMS, nowMS, serverTimeSyncedAt, …)`，服务端时间锚定。
5. 危险操作禁用：`isDangerousActionDisabled` / `isBidCloseGuardActive`（断连/恢复中/stale/临近落槌禁用出价）。
6. 出价幂等：同一 `client_bid_id`（`createClientBidID`）在超时重试时复用；8s 超时（`BID_REQUEST_TIMEOUT_MS`）+ `AbortController` → 不确定态 → 幂等重试。
7. 支付：`order_status==='PAID'` 为成功真相；`Idempotency-Key`；重查兜底；PC 只读。

---

## 3. 候选技术路线对比

> 原则：按“最终效果 / 体验 / 演示可信度 / 可扩展性”选，不按“哪个库最省事”选。每条给候选、优劣、适用性、风险、推荐与理由。

### 3.1 媒体传输协议与服务端

这是 Phase 2/3 的核心选型，分两层：**浏览器侧拉流协议** 和 **服务端模拟直播的产生方式**。

#### 3.1.1 浏览器拉流协议

| 协议 | 延迟 | iOS/Safari | 浏览器实现 | 是否需 UDP/TURN | 适用性 / 风险 |
|---|---|---|---|---|---|
| **HLS（标准）** | 6–30s | 原生支持 | 原生 `<video>` 或 hls.js（MSE） | 否（纯 HTTP） | 最稳、最易部署；延迟高，“像录播”不够“像直播”。 |
| **LL-HLS（低延迟 HLS）** | **~2–5s** | **原生支持（含 iOS）** | hls.js（非 Safari）/ 原生（Safari） | 否（纯 HTTP/HTTPS） | **“看起来像直播”的最安全解**；纯 HTTP，过防火墙/HTTPS 无痛；分片机制成熟。风险：服务端要正确产分片（`hls_time`/`hls_list_size`/`delete_segments`）。 |
| **HTTP-FLV** | 1–3s | 不支持原生，需 mpegts.js（flv.js 已停更） | mpegts.js（MSE） | 否 | 低延迟、单连接简单；**iOS Safari MSE 历史坑多**，且生态在向 LL-HLS/WebRTC 迁移；作为主路线风险偏高。 |
| **WebRTC / WHEP** | **<1s（亚秒）** | 支持（标准化拉流用 WHEP） | 浏览器原生 RTCPeerConnection | **公网部署通常需 STUN/TURN，UDP 端口** | 延迟最低、最“真直播”；但**部署复杂度最高**：ICE/UDP/TURN、防火墙、HTTPS 下的证书与端口，4vCPU/8G 上 TURN 中转还吃带宽。作为**后续升级**而非首发。 |

**推荐：首发 LL-HLS（hls.js + Safari 原生），把 WebRTC/WHEP 预接在同一 `MediaPlayback` 契约后面。**

理由：

- demo/比赛规模、4vCPU/8G、“能跑就行”，**纯 HTTP 的 LL-HLS 在部署面（HTTPS/端口/防火墙/浏览器兼容）几乎零摩擦**，而 WebRTC 的 UDP/TURN 在公网 + HTTPS 环境是最容易“演示翻车”的一环。
- LL-HLS **原生 iOS 支持**，移动端 H5 是主战场，不能赌 MSE 在 iOS 的边角。
- “看起来像直播”2–5s 完全够用（评委不会拿秒表比 200ms）；真要亚秒，WHEP 是**客户端换实现**，契约不变（见 §4 媒体源适配器）。
- 自动播放：移动端必须 `muted + playsInline + autoPlay`，否则被浏览器拦截（iOS 17.1+ 的 Managed Media Source 可作为 hls.js 的增强路径）。

#### 3.1.2 服务端“把文件假装成直播”的产生方式

| 方案 | 是否转码 | CPU | 部署复杂度 | 适用性 / 风险 |
|---|---|---|---|---|
| **MediaMTX + `alwaysAvailableFile` 循环 MP4** | **否（`-c copy`）** | **接近 0** | 低（单二进制，配置文件） | **推荐**。一个静态二进制把 MP4 循环重发为 RTSP/HLS/WebRTC 多协议出口；天然“现拍”语义；同一服务可同时出 LL-HLS（首发）和 WebRTC（后续），与 §3.1.1 的契约升级路线完美吻合。 |
| **裸 ffmpeg `-stream_loop -1 -re -c copy` 滚动写 HLS 分片** | 否 | 低 | 中（要自己管分片目录、Nginx 静态托管、清理） | 可行且极轻，但要手搓分片生命周期（`hls_flags delete_segments`）、静态服务、并发；不如 MediaMTX 省心。 |
| **SRS** | 可选 | 低–中 | 中–高 | 功能强（RTMP/HLS/WebRTC 全），但相对 MediaMTX 配置面更大；demo 体量过重。 |
| **nginx-rtmp** | 可选 | 低 | 中 | 老牌，但 LL-HLS/WebRTC 支持弱、维护活跃度低；不推荐新建。 |

**推荐：MediaMTX，`-c copy` 循环本地 MP4，首发出口 LL-HLS。** 理由：单二进制部署、不转码、CPU 几乎为零、多协议出口与“LL-HLS 现在 / WebRTC 以后”的升级路线天然对齐。已知可用域名：`mediamtx.org`、`github.com/bluenviron/mediamtx`（`mediamtx.io` 不可达，勿用）。

#### 3.1.3 带宽/端口/资源粗算（4vCPU/8G）

- `-c copy` 不转码：CPU 主要消耗在分片切割与 HTTP 出口，单路源可忽略。
- 带宽 = 单路码率 × 在线人数（LL-HLS 是 HTTP，可被反代/CDN 缓存分片；demo 直连即可）。一个 2–4 Mbps 的源、几十路观众，几十~百 Mbps 量级，单机可扛。
- 端口：LL-HLS 仅需 HTTP(S) 端口（复用 443/反代），**无需额外 UDP**；WebRTC 后续才需要 UDP + TURN。
- 降级：LL-HLS 不可用 → 回退标准 HLS（拉长分片）→ 再回退静态海报/录播；这条降级链要写进 `MediaPlayback` 的能力协商（见 §5.1）。

### 3.2 前端服务端状态：TanStack Query（由 WS 喂） vs SWR vs 裸手搓

| 方案 | 与 WS 推送真相的契合 | 乐观更新/回滚 | 适用性 |
|---|---|---|---|
| **TanStack Query v5** | **官方惯用法**：把 Query 当缓存，WS 消息当“mutation 结果”——`queryClient.setQueryData` 写实时字段、`invalidateQueries` 触发列表重取、`staleTime: Infinity` 让 Query 不自作主张轮询。 | `onMutate`/`onError`/`onSettled` 一等公民 | **推荐**。本项目价格/赢家/状态全靠 WS push，先 REST 取快照再 WS 流入同一缓存，正中 TkDodo 的“snapshot-then-stream”模式。 |
| SWR | 也能做，但 WS 写缓存与失效的工具链不如 RQ 完整 | 弱 | 次选 |
| 裸手搓（现状） | 就是 150+ hooks 的来源 | 全手写、易错 | 淘汰 |

**推荐：TanStack Query v5，WS 喂缓存，`staleTime: Infinity`。** 理由：它把现状里那 150 个 hooks 中“手写缓存/loading/重取”的部分整类消掉，且官方就有 WS 驱动缓存的成熟范式。**注意**：实时出价决策仍以服务端 WS 事件为唯一真相，乐观写入只是“提示”，不是结果（见 §3.4、§6）。

### 3.3 前端客户端状态：Zustand vs Jotai vs Redux Toolkit

| 库 | 体积 | 模型 | DevTools/时间旅行 | 推荐度 |
|---|---|---|---|---|
| **Zustand** | ~3KB | 单 store + selector 订阅 | 中（redux devtools 中间件） | **推荐**：极小、hook 原生、selector 精准重渲染，最适合突发高频更新下的 UI 态/“暂停直播”缓冲/乐观标志/连接态。 |
| Jotai | ~4KB | 原子化 | 中 | 当状态是大量独立小原子时很好；本项目可用但非首选。 |
| Redux Toolkit | ~15KB | reducer/action/slice | **最强（时间旅行/动作日志）** | 仅当需要极致客户端审计/可回放动作日志；但后端已有权威 WAL/飞行记录器，客户端审计需求低，重量不划算。 |

**推荐：Zustand 管纯客户端状态。** 服务端状态归 TanStack Query，二者分工，不混。

### 3.4 出价/连接状态机：XState v5 vs 手搓

**推荐：仅对“出价交互”和“连接/恢复”两条最危险的流用 XState v5，其余（列表/表单）保持普通 hooks。**

理由：非协商项要求“永不信任客户端结果、失败即收口”。出价交互天生是状态图：`idle → pending →（accepted | rejected | confirmRequired → confirming → …）`，且服务端可随时推来 `outbid`；外加并行的连接区域 `connected | reconnecting | resuming | degraded`。用 XState 把它编码，能从结构上**保证**“确认未完成时不可能显示已接受”、“socket 掉线强制进入 fail-closed/不确定态而非停在‘你正领先’”。不要滥用——别把列表/表单也做成状态机。

### 3.5 组件体系：shadcn/ui（已锁定） vs 保留 Arco vs Ant Pro vs HeroUI

**已锁定决策（2026-06-14，用户拍板）：两端统一迁移到 shadcn/ui（+ Tailwind + Radix），替换 PC 端 Arco 与自研 `tokens.css` 拼盘。** 这是一次有意的”换脸”，不是增量修补——理由与各候选的取舍如下，完整视觉论证见 [§11](#11-视觉识别系统auction-terminal统一设计语言)。

| 方案 | 性质 | 对”评委 wow + 差异化”的适用性 | 结论 |
|---|---|---|---|
| **shadcn/ui + Tailwind + Radix** | 不是组件库，是”把源码 copy 进你仓库的 primitives 集合” | **最高**：组件源码归你，可深改 token/radius/字体/CVA 变体，做出”看起来完全定制”的系统；两端共享一套 token；Radix 保证可访问性 | **采用（两端统一）** |
| 保留 Arco（v1 旧推荐） | ByteDance 维护的成品企业库 | 低：用户明确”很丑，改过好多次还是不满意”；成品库的视觉天花板被框死，深度定制要逆战其样式系统 | **否决**（与用户决策冲突，本次推翻 v1） |
| Ant Design Pro | Arco 同类 + ProTable/ProForm CRUD 抽象 | 低：同样是成品库观感；其价值在 CRUD 装配蓝图，可在 shadcn 上自建复刻 | 仅借鉴 ProTable/ProForm 的**装配模式**，不引库 |
| HeroUI（曾被用户提及） | 基于 Tailwind 的成品组件库（圆润、偏消费风） | 中：好看但风格偏”现成消费 App”，定制深度与去同质化不如 shadcn 的”源码归你”；与控制室/交易终端调性不符 | 否决：差异化与可控性不如 shadcn |

**为什么是 shadcn 而非”换个好看的成品库”**：用户”改过好多次还是不满意”，本质是成品库（Arco/HeroUI）把视觉决策权关在库里——你只能在它给的旋钮内调，永远调不出”这是我们自己的”。shadcn 把组件源码放进你的仓库，配合 OKLCH token 体系，**视觉天花板由你定**。社区也已点名”每个 shadcn 站长得一样”的同质化问题，解法是”真的建自己的设计系统（自定义 token + 圆角 + 字体 + CVA 变体），而不是堆 block”——这正是 [§11](#11-视觉识别系统auction-terminal统一设计语言) 的内容。

**迁移成本诚实评估**：换库 = PC 控制台所有 Arco 组件（Table/Form/Descriptions/Drawer/Layout）要在 shadcn 上重建；这是 Phase 1 的主要工作量。缓解：① 自建 `<DataTable>`（TanStack Table headless + shadcn 样式 + TanStack Virtual）和 `<PageShell>`（shadcn Sidebar + topbar）复刻 ProTable/ProLayout 的工效；② §2.3 不变量测试 + 行为等价搬迁守住正确性，让”换库”只换皮不改语义。长日志/飞行记录器/海量行用 **TanStack Virtual**（headless 虚拟化，60FPS），shadcn `Table` 默认不擅长几万行。

### 3.6 前端分层：FSD vs feature-folders vs container/presentational

**推荐：目标是 FSD（layers/slices/segments + 依赖规则），落地从务实的 feature-folders 起步，逐步固化为 slice。** 逻辑复用单元是**自定义 hook**（现代“容器”就是 hook，Dan Abramov 2019 已不再推荐人为拆 container/presentational），展示组件保持纯渲染。PC 与 H5 **共享** `entities/features/shared` 层，只在 `widgets/pages`（展示）层分叉。

---

## 4. 推荐架构与模块职责

### 4.1 分层总览

```
shared        设计 token、UI 原语、网络封装、MediaPlayback 契约类型、WS 客户端
  └ entities  auction / bid / order / live-session / payment（领域类型 + 取数 + 模型）
      └ features  place-bid / auction-controls / diagnostics / pay-order / media-playback
          └ widgets  live-ops（PC）/ live-stage（H5）/ orders-panel / monitor
              └ pages  PC 各页 / H5 各屏
                  └ app  路由、Provider、QueryClient、store 装配
依赖规则：只能向下引用，禁止平级/向上引用。
```

PC 与 H5 复用 `shared/entities/features`；差异只在 `widgets/pages`。

### 4.2 模块职责（逐个对照用户清单）

| 模块 | 职责 | 边界与红线 | 落点 |
|---|---|---|---|
| **Merchant Console（商家控制台）** | 上架/配规则/排期/启动/取消/监控/AI 辅助/复盘/飞行记录器；控制室式三区布局（活跃拍品+控制 / 5–9 指标卡 / 实时流+飞行记录器时间线） | 发起运营操作 + **只读**展示后端权威；**无支付控件**；规则启动后冻结，热路径用服务端规则 | `pc-console`，shadcn + 自建 `<PageShell>`/`<DataTable>` |
| **User Mobile Client（用户移动端）** | 看流、看价、出价、确认、看结果、付款；全屏视频 + 叠加层 + 底部动作坞 | 不决定成败/赢家/终态/价格；危险操作按 `isDangerousActionDisabled` 禁用 | `mobile-h5` |
| **Live Session API（直播会话 API，新增）** | 按 auctionID 返回**播放描述符**（`MediaPlayback`）：来源列表、协议、海报、是否直播、延迟目标、能力位 | 只描述“去哪拉、怎么拉、降级链”，**不承载**任何竞拍真相 | 新增 `GET /api/live/sessions/{auctionID}` |
| **Media Playback Contract（媒体播放契约，新增）** | 把 `videoURL = demoLiveVideoURL` 升级为统一契约；前端只认契约，不认具体协议/URL | 不写死 MP4；demo 返回 MP4 描述符，未来返回 LL-HLS/WHEP 描述符，**调用方不变** | `shared/media`（类型）+ `features/media-playback`（hook） |
| **Media Provider Adapter（媒体源适配器，新增）** | 把契约里的 `protocol` 落地为具体播放器：`mp4`→原生 video；`hls/ll-hls`→hls.js 或 Safari 原生；`whep`→RTCPeerConnection | 选择器在适配器内；升级 WebRTC 是**换适配器**，不动 `LiveStage` 调用 | `features/media-playback/adapters/*` |
| **Auction Realtime Adapter（竞拍实时适配器，重构现有）** | 统一封装 ws-ticket 取票、`auction.v1` 子协议、`last_seq` 恢复、gap 检测、慢消费者断开、rAF 合帧、快照-then-流 | **不改协议语义**；只把散落在 `main.tsx` 的 WS 逻辑收敛成 `ReconnectionManager` + `useConnection()` | `shared/ws` + `features/*/model` |
| **Payment Provider Boundary（支付提供方边界，接真）** | 复用后端已存在的 provider 边界；前端把 `pay-mock` 换成读 `client_action` 的真实入口；预留 Alipay 适配器 | 客户端跳转/回跳**永不作为支付凭据**；以服务端 `order_status` 为准；分→元换算只在适配器边界 | 后端 `bid.go` 已有；前端 `features/pay-order` |
| **Order/Payment State UI（订单/支付状态 UI，重构现有）** | 买家：发起→跳转/二维码→处理中→成功/失败/超时→重查恢复；商家：只读状态徽标/`provider_payment_id`/事件数/对账异常 | 买家付款入口**只在 H5**；商家**只读**；支付态用 XState 收口 | H5 `features/pay-order`；PC 复用只读视图 |

### 4.3 实时数据流（重构后）

```
(re)connect
   │  REST 取快照(useQuery)  ───────────────►  TanStack Query 缓存（live 字段）
   │                                              ▲  setQueryData（每帧 rAF 合并）
   └─ WS(auction.v1, ticket) ── ReconnectionManager ──┤
        backoff+jitter / heartbeat / lastSeq 恢复 / gap 检测   │
        事件 ── rAF 合帧缓冲 ──► 写 live 字段 setQueryData   │
                              └► 实体列表 invalidateQueries ─┘
   连接区域(XState): connected | reconnecting | resuming | degraded
   出价区域(XState): idle | pending | accepted | rejected | confirmRequired→confirming | outbid
```

要点：1000 笔/末秒突发**不 per-message setState**，缓冲到 ref，每个 `requestAnimationFrame` 刷一次（只取最新价 + 批量追加日志）；把渲染工作量压到刷新率内，主线程不被打满。

---

## 5. 数据与 API 契约草案

> 以下为**草案**：媒体侧是新增；支付侧大部分后端已存在，前端按既有字段对齐。

### 5.1 MediaPlayback 契约（新增，前端唯一认的媒体真相）

```ts
// shared/media/contract.ts —— 草案
export type MediaProtocol = 'mp4' | 'hls' | 'll-hls' | 'whep';

export interface MediaSource {
  protocol: MediaProtocol;
  url: string;               // demo: /demo/jade-live-loop.mp4 ；未来: https://.../index.m3u8 或 WHEP endpoint
  mimeType?: string;         // 'application/vnd.apple.mpegurl' 等
  priority: number;          // 适配器按优先级 + 浏览器能力选择
}

export interface MediaPlayback {
  auctionId: string;
  isLive: boolean;           // demo MP4=false（循环占位）；真直播=true
  posterURL?: string;        // 经 displayMediaURL 处理的封面（与流解耦）
  sources: MediaSource[];    // 按优先级排列的来源 + 降级链
  latencyTargetMs?: number;  // ll-hls≈3000；whep≈800；用于 UI 文案与监控
  capabilities: {            // 服务端/客户端能力协商结果
    nativeHlsOnSafari: boolean;
    mseHls: boolean;
    webrtc: boolean;
  };
}
```

- 调用方（`LiveStage`）只拿 `MediaPlayback`，把 `sources` 交给**媒体源适配器**选择实现；现有那行 `<video src={videoURL}>` 退化为 `mp4` 适配器的一个分支。
- 降级链 = `sources` 优先级顺序：`ll-hls → hls → mp4(海报/录播)`。

### 5.2 Live Session API（新增）

```
GET /api/live/sessions/{auctionID}
200 → MediaPlayback（见上）
```

- 该接口**只描述媒体**，不含价格/赢家/终态。Demo 阶段直接返回写死的 MP4 描述符；Phase 3 接 MediaMTX 后返回 LL-HLS 描述符；接口形状不变。
- 与竞拍实时事件**完全解耦**：媒体掉线不影响竞拍真相，竞拍恢复不依赖媒体。

### 5.3 支付：泛化既有边界（后端已存在，前端对齐）

| 端点 | 调用方 | 行为 | 现状 |
|---|---|---|---|
| `POST /api/orders/{id}/payments`（由 `pay-mock` 泛化） | 买家(H5) | 需 `Idempotency-Key`；winner-only；创建支付意图；返回 `{order_status, payment_intent_id, client_action}`。mock→`client_action:null` 自动推进；Alipay→`client_action:{redirect_url|qr_code}` | `pay-mock` 已在，待泛化 |
| `GET /api/orders/{id}` / `GET /api/users/me/orders` | 买家+商家(读) | 服务端权威状态（`order_status`/`provider_payment_id`/`paid_at`）；恢复与轮询来源 | **已存在**（H5 `main.tsx:1148`） |
| `POST /api/payments/notify/{provider}`（由 webhook 泛化） | 提供方服务器 | `ParseAndVerifyNotify`→按 `(provider, provider_event_id)` 去重→应用→ack（Alipay 回 `"success"`） | **已存在**为 `HandleProviderWebhook`（`bid.go:485`） |
| 内部对账 sweep | 调度器 | `QueryStatus` 兜底；差异落异常 | **已存在** `ReconcileProviderPayments`（`bid.go:595`，**注意：尚未接入定时 runner**） |
| `POST /api/orders/{id}/refunds`（后续） | 商家/运营（非买家） | 全额/部分退款 | 预留 |

**现在就预留（便宜，避免日后重塑）：** `orders.currency`（默认 `'CNY'`）；区分**我方** `out_trade_no`（=order/intent id）与**提供方** `provider_ref`（=Alipay `trade_no`）；`payment_events.event_type` 预留 `payment_canceled`/`refund_*`；`payment_events.provider` 泛化（别写死 `'local_fake'`）。

**Alipay 落地时才加（本轮不做）：** 真实适配器配置（`app_id`/RSA2 密钥/网关）、RSA2 验签、`notify_id` 二次校验、分↔元换算（适配器边界）、时间戳/重放容忍、退款模型。

### 5.4 支付/订单状态机（前端 UI 对齐后端真相）

```
                         支付窗口超时
   ORDER_PENDING ───────────────────────────────► ORDER_EXPIRED (押金 FORFEITED)
        │  ▲                                              ▲
 payment.initiate │  payment.failed / 放弃(可重试)        │ 过期后才到的成功 ⇒ 异常
        ▼  │                                              │ （绝不静默翻转）
   PAYMENT_INITIATED ──(跳转/二维码: requires_action, 不改状态)──┐
        │  payment.succeeded(签名异步通知)  或  对账轮询(兜底)     │
        ▼                                                        │
   PAYMENT_SUCCEEDED ──► PAID (押金 REFUNDED) ──► [REFUNDING ──► REFUNDED]
```

UI 阶段：`idle | pending(处理中) | paid | failed | expired`（H5 现有 `paymentPhase` 即此）；**处理中**屏只能由服务端确认的 `order_status` 推进，绝不由跳转回跳断言成功；关页重开走**重查订单**而非重放跳转。

---

## 6. 前端重构原则

1. **依赖规则铁律**：只能向下引用。这是消灭 2400 行巨石耦合的根本手段。
2. **先抽 hook，再抽 JSX**：那 150+ inline hooks 才是真耦合。先把内聚的状态搬进命名 hook（`useAuctionState`/`useBidSubmission`/`useConnection`/`useLiveMetrics`），让组件退化为“组合 + 展示”。
3. **服务端派生数据彻底搬离 `useState`**：进 TanStack Query 缓存，`staleTime: Infinity`，由 WS `setQueryData` 喂。手写的缓存/loading/重取整类删除。
4. **乐观更新只是提示，不是结果**：`onMutate` 即时本地反馈（<100ms 手感），但状态停在 `pending`，**唯有服务端 WS 决策事件（`ENGINE_*`+durability+settlement）**才翻成 accepted/rejected/outbid。HTTP 200 不是成功。
5. **危险路径上状态机**：出价、连接/恢复用 XState，让非法状态不可表达、失败即收口。
6. **行为等价的特征测试保护重构**：先加 characterization test 锚住 §2.3 的不变量，再做等价搬迁（同 props 进同 UI 出），最后才重构内部。触碰正确性/竞态/恢复路径必须有测试包裹。
7. **PC/H5 共享下层**：`shared/entities/features` 共用，只在 `widgets/pages` 分叉，避免两端逻辑漂移。
8. **性能可证明**：`web-vitals` 的 `onINP`（p75 good ≤200ms）打点出价交互；`performance.mark/measure` 量 tap→paint；`PerformanceObserver('longtask')` 警>50ms 主线程阻塞；`yieldToMain` + rAF 合帧扛住末秒突发。与后端“工作负载+profile+verifier+证据”同等纪律。
9. **媒体只认契约**：任何组件不得再出现写死的流地址；一律经 `MediaPlayback` + 媒体源适配器。
10. **不过度抽象**：列表/表单保持普通 hooks；XState 只用在两条危险流；不为假想需求造轮子。

---

## 7. 分阶段计划

> 每阶段给：做什么 / 验收 / **不要做**。阶段间可独立交付、独立回滚。

### Phase 0：ADR + 脚手架（不改行为）

- **做什么**：写 3 篇 ADR（媒体路线=LL-HLS/MediaMTX；前端状态=RQ+Zustand+XState；支付=泛化既有边界）；引入 TanStack Query/Zustand/XState/TanStack Virtual 依赖与 `QueryClient`/Provider 装配；建 `shared/entities/features/widgets/pages` 目录骨架与依赖规则 lint；为 §2.3 不变量补 characterization test。
- **验收**：依赖与目录就位；现有 e2e/单测全绿；新增不变量测试全绿；构建无回归。
- **不要做**：不迁移业务逻辑、不动 UI、不碰后端。

### Phase 1：双前端重构（行为保持）

- **做什么**：
  - H5：把 `main.tsx` 的服务端派生态迁入 RQ（WS 喂）；WS 逻辑收敛为 `ReconnectionManager`+`useConnection()`；出价交互改 XState（pending 只由 WS 决策事件翻转）；纯 UI 态进 Zustand；`components.tsx` 按 widgets/features 拆分；保留全部氛围/音效/合帧能力。
  - PC：迁移到 shadcn 体系，自建 `<PageShell>`（shadcn Sidebar+topbar 复刻 ProLayout）/`<DataTable>`（TanStack Table headless + shadcn 样式 + TanStack Virtual 复刻 ProTable）；Live Ops 三区控制室布局；长日志/飞行记录器上 TanStack Virtual；实时态用 delta 指示+sparkline+200–400ms 高亮脉冲（不闪烁）+”暂停直播/N 条新”。视觉走 [§13](#13-pc-控制台控制室视觉与数据可视化) 的控制室方向。
- **验收**：§2.3 全部不变量测试仍绿；出价/确认/恢复/倒计时/支付既有 e2e 通过；出价交互 INP p75 ≤200ms 有打点证据；PC 在万行日志下 60FPS；视觉/交互在真机浏览器走查（golden path + 弱网 + 断连恢复）。
- **不要做**：不改 WS 协议语义、不改出价幂等/确认/恢复契约、不改后端、不动媒体（仍用现有写死 MP4）、不引入支付真实跳转。

### Phase 2：统一媒体播放契约

- **做什么**：落地 `MediaPlayback` 类型 + `GET /api/live/sessions/{auctionID}`（demo 返回 MP4 描述符）；`LiveStage` 改为消费契约 + 媒体源适配器（先实现 `mp4` 适配器，等价替换现状那行 `<video src>`）；实现降级链 `ll-hls→hls→mp4/海报`；播放与竞拍实时**解耦**验证（媒体掉线不影响竞拍真相）。
- **验收**：`LiveStage` 不再出现任何写死流地址；切换描述符即可换源（用本地 LL-HLS 样例验证 hls.js/Safari 原生两条路径）；媒体掉线时竞拍出价/恢复不受影响；移动端 `muted+playsInline+autoPlay` 自动播放成功。
- **不要做**：不在前端写死协议；不把竞拍真相塞进媒体接口；不实现 WebRTC（仅留适配器接缝）。

### Phase 3：服务端模拟直播流（关键）

- **做什么**：部署 MediaMTX，`-c copy` 循环本地 MP4，出口 LL-HLS；`/api/live/sessions/{id}` 返回真实 LL-HLS 描述符；走 HTTPS/反代复用 443，无额外 UDP；浏览器像真直播一样**拉流**（无任何采集/推流客户端）；压一遍几十路并发观众的带宽/CPU；把降级（LL-HLS 不可用→标准 HLS→海报）跑通。
- **验收**：浏览器（含 iOS Safari 原生 HLS + 桌面 hls.js）拉到“看起来像直播”的循环流，端到端延迟 2–5s；4vCPU/8G 上 CPU 主要不在转码、单机扛住目标并发；HTTPS 下无混合内容、无防火墙/端口问题；演示链路一键启动。
- **不要做**：不上 WebRTC/TURN（留作 Future，换适配器即可）；不做多码率/CDN 回源鉴权（生产化属 Future）；不引入摄像头/采集端。

### Future（只提，不做）

真实商家采集/摄像头推流；真实支付宝结算落地（填 Alipay 适配器：`page.pay`/`wap.pay`/`precreate` + RSA2 验签 + `trade.query` + `refund`，分↔元换算）；直播云生产化（多码率/CDN/鉴权/WebRTC 亚秒）；原生 App。架构已为这些预留接缝（媒体源适配器、支付适配器、`MediaPlayback.protocol='whep'`）。

---

## 8. 风险清单

| # | 风险 | 等级 | 缓解 |
|---|---|---|---|
| R1 | 重构误伤出价/决策/恢复内核（改错赢家/价格/终态/丢事件） | **高** | §2.3 不变量 characterization test 先行；行为等价搬迁；触碰即测试包裹；内核代码本轮不改。 |
| R2 | WS→RQ 迁移引入“applied delta to nothing”/乱序渲染 | 高 | snapshot-then-stream；gap 检测→请求重放/重快照；旧序号丢弃；rAF 合帧只取最新。 |
| R3 | iOS 自动播放/MSE 兼容翻车 | 中 | 首发 LL-HLS（Safari 原生），`muted+playsInline+autoPlay`；hls.js 仅非 Safari；避开 HTTP-FLV 主路线。 |
| R4 | Phase 3 部署面坑（HTTPS/端口/防火墙/混合内容） | 中 | 选纯 HTTP 的 LL-HLS、复用 443、反代；不上 UDP/TURN；预置降级链。 |
| R5 | 把客户端跳转回跳当支付成功（资损/错单） | **高** | 以 `order_status==='PAID'` 服务端真相为准；处理中屏不断言成功；关页重开重查；后端验签+去重+对账已在。 |
| R6 | 乐观出价 UI 被误当结果 | 高 | 乐观只是“已发送”提示；XState 停在 pending，仅 WS 决策事件翻转；HTTP 200 不算数。 |
| R7 | `ReconcileProviderPayments` 未接定时 runner，漏单只能靠通知 | 中 | Phase 0/1 顺手把对账接入调度器（后端已有函数，仅缺触发）。 |
| R8 | 引库（RQ/Zustand/XState/Virtual）膨胀包体/学习成本 | 低 | 体积小（合计可控）；XState 只用两处；feature-folders 起步降低心智负担。 |
| R9 | 末秒 1000 笔突发打满主线程，掉帧/INP 退化 | 中 | rAF 合帧 + `yieldToMain`；`longtask` 观察器告警；INP 打点门禁。 |
| R10 | PC 控制台实时流“信息过载”淹没操作员 | 低 | 5–9 指标卡；progressive disclosure；暂停直播+“N 条新”；虚拟化长日志；时间戳/连接态常驻。 |
| R11 | 泄露的 API key（见文末安全提示）被误用 | **高** | 不写入任何代码/文档/日志；建议立即轮换；与本规划无关，单独处理。 |
| R12 | Arco→shadcn 换库引入 PC 控制台正确性回归（表格/表单/抽屉行为漂移） | 中 | §2.3 不变量测试 + 行为等价搬迁（同 props 进同 UI 出）；自建 `<DataTable>`/`<PageShell>` 先过特征测试再替换；只换皮不改语义。 |
| R13 | 动效/氛围误导（动效暗示成交/价格/赢家由前端判定） | **高** | 动效只在服务端权威事件兑现（§12.2 乐观/权威列）；乐观态停在中性"已发送"；`shouldGateAtmosphere` 不确定态收敛。 |
| R14 | 视觉过度炫技伤可访问性/性能（>3 闪/秒、减动未尊重、末秒掉帧、iOS 无震动当承重反馈） | 中 | WCAG 2.3.1/2.3.3 门禁；一次性 fade 不连续闪；强度 0–3 当渲染预算；触觉仅安卓增强、关键反馈有视听等价。 |
| R15 | 做成"默认 shadcn 同质脸"，丢掉差异化 | 中 | §11.8 去同质化清单（自定义 token/小圆角/tabular 字体/CVA 业务变体/侧栏材质）；建自己的系统而非堆 block。 |

---

## 9. 验收标准

**全局（贯穿所有阶段）**

- §2.3 七条不变量的 characterization test 始终绿；任何阶段不得回归。
- 客户端永不宣布成败/赢家/终态/价格；终态只来自服务端事件/快照。
- 支付以服务端 `order_status` 为唯一真相；幂等/验签/对账不削弱。

**Phase 1**：双前端去巨石（无单文件 >800 行的“上帝组件”作为目标，逻辑在 hooks，展示在纯组件）；服务端态全部经 RQ；出价/连接走 XState；出价交互 INP p75 ≤200ms 有 `web-vitals` 证据；PC 万行日志 60FPS；真机浏览器走查（golden path + 弱网 + 断连恢复 + 末秒突发）通过。

**Phase 2**：`LiveStage` 无写死流地址；切描述符即可换源（LL-HLS 样例双路径验证）；媒体掉线不影响竞拍；移动端自动播放成功；降级链可触发。

**Phase 3**：iOS Safari（原生 HLS）+ 桌面（hls.js）拉到循环“类直播”，延迟 2–5s；4vCPU/8G CPU 不主要耗在转码、扛住目标并发；HTTPS 无混合内容、无端口/防火墙问题；一键演示。

**支付接真（可与 Phase 1/2 并行的最小切片）**：`pay-mock`→`POST /api/orders/{id}/payments` 返回 `client_action`；mock 适配器端到端走通（含关页重开重查恢复）；`ReconcileProviderPayments` 接入调度器；预留字段（`currency`/`out_trade_no` vs `provider_ref`/event_type/provider）落库；Alipay 适配器留空接缝但接口形状就位。

---

## 10. 待进一步调研的开放问题

1. **LL-HLS 在 MediaMTX 的确切配置与端到端延迟实测**：`hls_time`/`hls_list_size`/`delete_segments` 与 part 时长的组合，在 4vCPU/8G 上的真实延迟与 CPU；需要在目标机器上实测，而非引用文档数字。
2. **Live Session API 是否需要鉴权**：demo 阶段流地址是否要 token 化（与 WS ticket 类似），还是直连即可？取决于是否担心盗链与演示便利的权衡。
3. **媒体与竞拍时间轴是否需要对齐**：现在 `deriveCountdown` 用服务端时间锚定；若未来要“流里的落槌瞬间”与倒计时严格同步，需要单独的时间对齐方案（本轮 demo 不需要，先记下）。
4. **`payment_intents` 是否要提前建表**：当前“一订单一次性结算”可让 order 自身充当 intent；若未来允许同一订单多次支付尝试（Stripe 模型），需提升为独立表——本轮先不建，仅预留概念。
5. **Alipay 沙箱接入的最小可验证切片**：`wap.pay`（H5）与 `page.pay`/`precreate`（PC 扫码）哪个先做沙箱联调；RSA2 密钥管理与 `notify_url` 的公网可达性（演示环境如何暴露回调）。
6. **PC 控制台是否迁移 React 19 / 是否参考 `thirdparty/TikTokShop-B-` 的目录分解**：该参考工程（React 19.2 + antd 6 + socket.io）是**分解模式**的范例，不是栈强制；是否借鉴其 `api/components/domain/hooks/layouts/pages/store/types` 划分需团队拍板。
7. **XState 引入范围的边界确认**：仅出价+连接两条流，是否还要覆盖“支付态”？支付态在 H5 已有 `paymentPhase`，是否值得升格为状态机需要权衡收益。

---

---

# 第二部分：视觉 · 交互 · 氛围（v1 缺失维度的补齐）

> **为什么有这一部分。** v1（§1–§10）只回答了"架构怎么不烂"，没回答"评委为什么会记住我们"。用户的真实成功判据是：**最震撼评委、体现工程能力、极致氛围、与竞品差异化——不只是 UI 还有 UX（互动 + 反馈）**。所以视觉/交互在红线之内是**一等交付物**，与功能架构同权重。
>
> **贯穿全篇的红线（与第一部分一致，不重复论证）：** 所有视觉/动效/高亮都只是**呈现层**，永远不能让动效**暗示**成交/价格/赢家/终态是前端算出来的。乐观态是"提示"，唯有服务端权威 WS 事件（`ENGINE_*` + durability + settlement）才能翻转 accepted/rejected/outbid/sold。动效必须服从 `prefers-reduced-motion`、不得 >3 闪/秒（WCAG 2.3.1 Level A）、在不确定态（恢复中/stale/断连）自动收敛——现有 `atmosphere.ts` 的 `shouldGateAtmosphere` 已经是这个"正确性感知"的雏形，下面把它从"能力"升级成"差异化招牌"。

## 11. 视觉识别系统：Auction Terminal（统一设计语言）

### 11.1 先定位"改多次仍不满意"的真根因（基于代码，不是猜）

读 `frontend/shared-design/tokens.css`（91 行）可证：现状不是"配色选错"，而是**根本没有体系**——同一文件里并存 Arco 蓝 `--color-primary:#165DFF`、TikTok 粉 `--color-cta:#FE2C55`、金 `--color-gold:#D4AF37`，外加一堆一次性颜色，彼此没有统一的明度/色相阶梯，暗色派生靠手填。**这种拼盘无论怎么改单个颜色都不会"成系统"**，所以反复改、反复不满意。

**结论：要换的不是颜色，是 token 体系本身。** shadcn/ui 的主题机制（CSS 变量 + `@theme inline` + `components.json` 的 `cssVariables:true`）配合 **OKLCH** 色彩空间，提供的正是这套缺失的体系——这是把"美观"从玄学变成工程的关键一步。

### 11.2 为什么用 OKLCH（不是 hex/HSL）

- **感知均匀**：OKLCH 的 L（明度）符合人眼感知，调 L 不会像 HSL 那样产生明度突变——这让"暗色派生""hover/active 阶梯""语义色同明度对齐"变成可预测的算术，而不是逐个肉眼调。
- **广色域（P3）**：可表达 sRGB 之外更饱和的金/红/绿，在 OLED 手机屏上"极致氛围"有上限提升。
- **WCAG 可预测**：因为 L 是真明度，对比度可被 L 差近似预判，过 4.5:1（文字）/3:1（组件）有章可循。
- shadcn 新版默认主题本身就用 OKLCH（如 `--background: oklch(0.145 0 0)`），与此天然兼容。

### 11.3 推荐视觉方向："Auction Terminal"（墨金 × 交易终端 × 克制直播能量）

在三个候选方向中（A 简约黑白精密派 / B 交易floor高密度派 / C 产品化运维暗色派）做**融合**，理由是单一方向都不够：A 稳但 wow 上限低、易撞脸；B 差异化最强但易"花哨堆砌";C 现代但强调色选错会显廉价。融合策略：

> **暗色优先的"墨金"威望底色（ink-black + 克制金，金只占 5–15%）作气质 + 交易终端的价格/出价引擎（等宽数字、涨绿跌红方向闪动、暗底辉光）作"心跳" + 在张力时刻（末秒、落槌）释放有纪律的直播能量。**

这套组合：B 的高密度等宽数字给"工程味/差异化"，C 的产品化暗色给外壳现代感，A 的克制（**永不纯白文字**、单一品牌强调色）作撞脸风险最低的底盘。墨金的"威望感"把直播拍卖与"通用 TikTok 暗色克隆 / 通用电商"区分开——这正是用户要的"避免和竞品一样平庸"。

### 11.4 Token 体系（shadcn 之上扩展拍卖语义层）

在 shadcn 标准 token（`background/foreground/primary/card/border/ring/destructive…`）之上，**新增一组拍卖状态语义 token**（OKLCH，暗色优先），这是把"氛围"沉淀进设计系统的关键：

| 语义 token | 用途 | 红线绑定 |
|---|---|---|
| `--state-leading` | 我方领先 | 仅服务端确认领先时点亮；乐观 pending 用中性"已发送"色 |
| `--state-outbid` | 被超越 | 仅 WS `outbid` 事件触发 |
| `--state-won` / `--state-lost` | 成交结果 | 仅 settlement 终态；金色=won（威望） |
| `--bid-cta` | 出价主按钮（稀有高饱和强调） | 全局唯一最高饱和点，聚焦"出价"这一核心动作 |
| `--flash-up` / `--flash-down` | 价格涨/跌方向闪动 | 仅服务端确认价变；一次性 fade，**不持续闪烁** |
| `--live` / `--stale` / `--paused` | 连接/数据新鲜度 | 必须反映系统真实状态，不可假"Live" |

原则：① **永不纯白**正文（控制室经验，降眼疲劳）；② 金（gold）保留给威望/成交/won，平时克制在 5–15% 面积，避免"土豪金"；③ **状态永不只靠颜色**（叠加图标/形状/文字，应对 1/12 男性色盲，WCAG 1.4.1）；④ 单一 `--radius` 基准派生圆角阶梯，拍卖/控制室偏**小圆角**（更"硬"、更终端感）。

### 11.5 字体三角色系统（"终端感"最廉价高效的来源）

| 角色 | 选型 | 用在哪 |
|---|---|---|
| **Display 衬线**（高对比） | 一套高对比衬线 | 拍品标题、落槌大字、威望时刻——给"高端拍卖行"气质 |
| **UI 无衬线** | 一套有性格的 grotesk/sans（避开 Inter/Geist 的默认味），Latin-first 声明 + **思源黑体 / Source Han Sans** 作 CJK fallback | 正文、按钮、表单、标签 |
| **等宽 / Tabular** | 等宽或 `font-variant-numeric: lining-nums tabular-nums` | **所有实时数字**（价格、出价额、倒计时、日志、指标）——数字等宽对齐是"交易终端专业感"最强信号 |

注：现有 `tokens.css` 已有 `.tabular-nums` 类，方向对，但要扩成完整三角色系统并接进 shadcn 字体 token。

### 11.6 浮层与可读性（金钱关键 UI 不能输给视频背景）

全屏视频之上做叠加层，金钱关键信息（当前价/出价按钮/倒计时）必须始终清晰：

- **底部 scrim（约 40% 黑渐变）** 托住底部动作坞，保证按钮/价格在任意视频画面上可读。
- **低模糊玻璃面板**（low-blur glass）承载金钱关键 UI；模糊要克制——模糊在很多平台开销大，且 `opacity` 动画也昂贵（NN/g），不要满屏玻璃。
- 文字/组件过 WCAG（4.5:1 / 3:1）；`--bid-cta` 是全屏唯一最高饱和点，视线天然落到"出价"。

### 11.7 一套语言、两个应用的共享方式

两端共享设计语言但密度档不同（H5 舒适、PC 紧凑）。落地：**monorepo 内一个共享 `ui` 包**承载 OKLCH token、字体、语义色、shadcn base 组件；两端通过 shadcn 的 `registry:base`/`registry:font`/`--preset` 复用同一套主题，只在密度（间距/行高档位）与"widgets/pages"层分叉。这呼应第一部分 §4.1 的"`shared` 层共享"。

### 11.8 如何**不**像"默认 shadcn"（去同质化清单）

社区公认风险："每个 shadcn 站长得一样"。解法是真建自己的系统，不是堆 block：

1. 重定义 token，**不用默认 neutral**（选偏冷的中性基底 + 非默认品牌强调色，避开示例紫）；单独定义 `--success/--warning/--danger/--info`（shadcn 默认只有 `--destructive`）。
2. 改 `--radius` 与密度档成"自己的味道"（控制室用更小圆角 + 更紧行高）。
3. 换字体系统（§11.5），数字强制 tabular。
4. 改 **CVA variants** 而非到处加 class——把 Button/Badge 变体改成项目语义（`badge: live/stale/paused/sealed/won`），组件层就带品牌。
5. **包装/复合组件**承载业务，**绝不直接改 `ui/` 基础组件**（保持可升级）。
6. 侧栏用 shadcn 的 `--sidebar-*` token 配成"另一种材质"（更深/半透明 + 专用 border/ring），与工作区拉层次。

> 注意一个常见坑：shadcn 的图表（基于 Recharts，但**源码归你、不是 wrapper**）必须在 `chartConfig` 用 `var(--chart-1)` 而非 `--primary`，否则图表是黑白的（GitHub discussion #7241）。

---

## 12. 标志性交互 · 动效 · 多感官氛围（H5 主战场）

### 12.1 已有的"半成品弹药"（基于代码，不是从零造）

H5 端已有一套**正确性感知**的氛围基础设施，差异化故事应建在它之上而非另起炉灶：

- `frontend/mobile-h5/src/atmosphere.ts`：氛围"大脑"（纯领域、无渲染）。`AtmosphereKind`（leading/outbid/extended/sold/recovering/social）、`AtmosphereCue`（带 `cause_seq`/`event_type`/`user_scope`/`priority`）、`calculateAtmosphereIntensity`（由 30s 接受出价数/价格速度/剩余时间推强度 0–3）、`shouldGateAtmosphere`（恢复中/stale/断连**硬门**，reducedMotion/低电量/AI 关闭**软门**）。
- `domain.ts`：音频引擎（`createAudioContext`/`loadAuctionSoundPack`/`playLayeredCue`）与 `vibratePattern`。
- `components.tsx`：`ClimaxLayer`/`BidWaterfall`（canvas + rAF + DPR 钳制）、`FinalSecondsLayer`、`BarrageLayer`、`HeatMeter`、`RaceBoard`。
- `main.tsx`：`showAtmosphere` 按 `cause_seq` 去重、`commitLeaderboardDelta` 拒绝过期 seq、leaderboard rAF 合帧。

> **这是最强的差异化叙事**：我们的动效不是装饰，是**服务端事件驱动**（每个动效 keyed by `cause_seq`）、**强度自适应**、且**状态不确定时自动收敛**——"正确性感知的炫技"。评委同时看到"好看"和"工程上站得住"。

### 12.2 标志性时刻地图（每个都遵循"预期蓄力 → 单一焦点变化 → 干净退场"）

高级感的动效公式（Yu-kai Chou 的 anticipation parade）：**蓄力建立预期 → 一个焦点变化兑现 → 利落退场**；且"juice 要呼应机制"（反馈要对应真实发生的事）。每个标志性时刻都标注"乐观 vs 权威"列以守红线：

| 时刻 | 蓄力 | 焦点兑现 | 退场 | 乐观/权威 |
|---|---|---|---|---|
| **末秒张力** | 倒计时进入末 10s，房间能量层渐强、等宽倒计时放大 | 数字逼近 0 的脉动 | 落槌或延时 | 倒计时服务端时间锚定（`deriveCountdown`） |
| **落槌 Hammer Hold → SOLD** | "going once/twice"递进、短暂"屏息"停顿（the hold） | 权威 `sold` 事件到达瞬间，SOLD 大字 + 金色定格 | 收敛到结果卡 | **只在 settlement 终态触发**，绝不前端预判 |
| **名次跃迁** | 出价被接受 | 共享元素（`layoutId`）把你的头像/名次 FLIP 滑到榜首 | 落位微弹 | 仅服务端确认领先后（`commitLeaderboardDelta` 拒过期 seq） |
| **价格跳动** | — | 等宽价格 odometer 翻滚 + `--flash-up` 一次性 fade + 强度分档的涌动 | fade 结束 | 仅服务端确认价变 |
| **被超越 outbid** | — | 一次"冲击波"震感 + outbid 色脉冲 | 引导再出价 | 仅 WS `outbid` 事件 |
| **领先 leading** | — | 克制的金色描边 | — | 仅服务端确认 |

### 12.3 推荐的"招牌交互集"（集中火力的差异化点）

1. **Hammer Hold → 权威 SOLD**：落槌前的"屏息"停顿，把成交瞬间仪式化；只在 settlement 终态兑现。
2. **共享元素名次跃迁**：用 Motion 的 `layoutId` 做 FLIP，出价人滑入榜首——比"列表重排"高级得多。
3. **实时价格 odometer + 涌动分层**：等宽数字翻滚 + 强度（0–3）分档的房间能量。
4. **强度自适应房间能量**：把 `calculateAtmosphereIntensity` 的 0–3 同时当**视觉强度**和**渲染预算**（见 §12.6）。
5. **Outbid 冲击波**：被超越的瞬时多感官反馈（视觉脉冲 +（安卓）震动）。
6. **品牌字形 confetti + 视图转场**：成交用自定义品牌字形彩带 + View Transitions 做屏间交接。

### 12.4 技术栈

- **Motion（前 Framer Motion）v12**：`layoutId` 共享元素、`AnimatePresence` 进出场、手势、spring（stiffness≈400 / damping≈17）。注意：`AnimateNumber` 是 **Motion+ 付费**功能——价格 odometer 改用**自写 DOM 文本节点 + rAF**实现，不依赖付费件。
- **View Transitions API**：同文档转场（Chrome/Edge/Safari/FF144），跨文档（Chromium + Safari 18.2）；不支持则优雅降级为无操作（不阻塞）。
- **canvas-confetti**：`useWorker:true`（不阻塞主线程）、`disableForReducedMotion:true`、`shapeFromPath/Text` 做品牌字形、粒子 **≤150**。
- **Web Audio 分层提示**：master `GainNode` + 床/事件/强调三层 + `DynamicsCompressor` 防爆音 + `AudioParam` 调度（已有 `playLayeredCue` 雏形）。
- **动画只动 `transform`/`opacity`**（合成器线程，不触发 layout/paint）。动画时长遵循 NN/g：100–400ms、ease-out、入场略长于出场。

### 12.5 多感官的硬约束（红线 + 可访问性）

- **iOS Safari 没有 `navigator.vibrate`**：触觉只能作为**安卓 Chrome 的增强**，**永不作为承重反馈**（关键反馈必须有视觉/听觉等价物）。现有 `vibratePattern` 要按此降级。
- **WCAG 2.3.1（Level A）**：任何东西**不得 >3 闪/秒**——价格闪动/outbid 脉冲是主要风险点，必须用一次性 fade 而非连续 blink。
- **WCAG 2.3.3 + `prefers-reduced-motion`**：减动偏好下关闭非必要动效（`shouldGateAtmosphere` 已软门处理）。
- **ARIA live region** 静默播报关键变更（价格/领先/成交），让动效之外有可访问通道。

### 12.6 末秒 1000 笔突发的"动效不打满主线程"预算

把 `AtmosphereIntensity`（0–3）当**渲染预算调速器**：

- **rAF 合帧**：所有 delta（价格/榜单/出价流）缓冲到 ref，每帧只刷一次最新值 + 批量追加（已有 leaderboard rAF 合帧范式）。
- **canvas 不用 DOM** 画高频粒子（`BidWaterfall`/`ClimaxLayer` 已是 canvas + DPR 钳制）。
- **音频节流 ≤1 cue/帧**，避免 1000 事件触发 1000 声。
- 强度越高，越优先"聚合表达"（一个涌动）而非"逐条表达"（千条闪烁）——既是美学也是性能。
- 用 `web-vitals` `onINP`（p75 ≤200ms）给氛围层打点，`PerformanceObserver('longtask')` 告警 >50ms 阻塞。

---

## 13. PC 控制台：控制室视觉与数据可视化

### 13.1 视觉方向：暗色控制室（外壳 C + 现场 B + 底线 A）

- **外壳/稳态页**（侧栏、顶栏、设置、订单只读、AI 复盘）：Linear/Sentry 式产品化暗色，可选低模糊玻璃浮层做"看着底层数据同时操作筛选"。
- **拍卖现场/出价流/飞行记录器**：Bloomberg 式高密度——等宽数字、密集行、左上角 telemetry、sparkline 海。这是控制台最该"惊艳"的地方，密度本身传递工程能力。
- **底线**：永不纯白文字、单色基底 + 单一品牌强调色 + 严格语义色（红/琥珀/绿/蓝），暗色图表饱和度降 15–20%，分隔线提对比维持结构。
- 信息层级按 RED / Four Golden Signals 组织（最重要 KPI 放左上视线首落点），对 PG/Redis/Kafka/outbox/scheduler 监控是现成骨架。

### 13.2 数据可视化：三库分工（本控制台最大 wow 杠杆）

视觉统一靠 token（三库都吃 `--chart-*` / OKLCH），不靠用同一个库：

| 用例 | 选型 | 理由 |
|---|---|---|
| **稳态图**（KPI 卡、area/donut、行内 sparkline） | **shadcn Charts（=你自己的 Recharts 源码）**，KPI/spark 可借 **Tremor**（已被 Vercel 收购、MIT 免费）加速 | 与控制室 token 无缝、SSR 友好、源码归你、开发最快 |
| **实时密集**（实时价格流、live 吞吐、末秒出价流、热力） | **Apache ECharts**（Canvas + `appendData` 流式 + 增量渲染 + TypedArray） | 题面"1000 bids/末秒"的正面战场，SVG 系会掉帧；PTS-1B 单热拍卖数据量在 ECharts 舒适区内 |
| **定制 wow**（flight-recorder 时间线/span waterfall、服务关系图、p99 决策延迟仪表） | **visx**（Airbnb，按需 primitive、最小包体；必要时辅以 D3 math） | Recharts/Tremor 画不出独特感；Airbnb 官方已用 visx 做可观测性/radial timeline，这是评委最记得住、最难复刻的差异化件 |

**诚实标注的上限与缓解**：ECharts Canvas 在 100k 点 ~350ms、流式长会话（>30min）会随数据累积掉帧（每帧重绘随数据增长）——所以**必做窗口化/降采样/节流**（如每 100–250ms 批量 flush），"库无法弥补一次渲染太多"。当前 PTS-1B（单热拍卖）远未触及上限，无需上商业 WebGL 库（LightningChart）。三库并存用动态 import 分路由加载，ECharts 必须 `echarts/core` 按需注册、Next 下包成 Client Component。

### 13.3 优雅的密集表格（飞行记录器 / 订单大表 / live 日志）

- **TanStack Table（headless）+ shadcn `Table` 样式 + TanStack Virtual**：headless 给"服务端分页/排序 + shadcn 皮 + 60FPS 虚拟化"三全。
- 工程要点：virtualizer 放尽量低层避免无谓重渲染；只订阅必要 state（v8 大表内存偏高）；**live 日志禁用 grouping**（`groupBy` 大数据有 1000× 慢的著名坑）；header 背景设非透明防滚动穿透；动态行高需 `estimateSize` + Firefox 特判。
- 实时呈现范式：**"滚动即暂停自动跟随 + 顶部'N 条新 ↓'浮条 + 一次性 fade 高亮（<300ms，不闪烁）+ 行内 delta/sparkline"**（Logz.io Live Tail 范式；Grafana 社区明确投诉"每秒 flash 角标很扰人"）。
- 金额/状态/赢家列只渲染服务端字段（带 `ENGINE_*`/durability/settlement），表格不做任何"看起来像判定"的客户端推断。

### 13.4 实时但不过载的 UX（写成一页可验收规范）

可直接进设计系统的硬规则：

- **动画** 100–400ms / ease-out / 尊重减动；硬切色闪烁可诱发癫痫，禁止。
- **每屏 ≤5–9 个 KPI**，最关键放左上；相关数据分组成卡。
- **delta 指标 + trend sparkline + mini-history**（可回看几分钟）对抗"数字瞬变看不懂"；不只靠颜色。
- **可信度可视化（对 SRE/审计红线极重要）**：Data Freshness 指示 + "Data as of 10:42" + 手动刷新；**Live/Stale/Paused** 三态；数据不可用显**带时间戳的缓存快照**而非空白；连接失败指数退避自动重试，失败才显"Reconnecting…"横幅；**skeleton 取代 spinner**；ARIA live region 静默播报；告警附"为何触发 + 决策时刻依据"（呼应 `CLAUDE.md`"所有 reject 要有 decision-time basis"）。
- **按重要性分更新频率**——不是所有数据都每秒刷。

> Smashing 原话精神：实时仪表是 **decision assistant，不是被动展示**——把"实时"做成"沉稳掌控感"，这正是评委眼中"成熟工业级"的信号。

### 13.5 PC 端最强 wow 三件（集中精力处）

① **visx 自绘的 flight-recorder 时间线 / 服务关系图**（独一无二、最像"重工程"）；② **ECharts 末秒出价流 + 出价速率 sparkline**（直击题面）；③ **Bloomberg 式等宽数字高密度表 + 行内 sparkline/delta**（密度即专业）。

---

## 14. 视觉/交互的分阶段落地（接回 §7）

视觉不是独立阶段，而是**编织进 Phase 0/1**，并与媒体（Phase 2/3）协同：

- **Phase 0（追加）**：除功能 ADR 外，加一篇**视觉 ADR**（锁定 Auction Terminal 方向 + OKLCH token 体系 + shadcn 迁移）；搭建 monorepo 共享 `ui` 包骨架与 OKLCH token、字体、语义色 preset；引入 Motion v12（H5 已有 motion 12.40，确认版本）、ECharts/visx/Tremor（PC）、canvas-confetti（H5 已有）依赖。**不改行为、不动业务逻辑。**
- **Phase 1（追加视觉维度）**：
  - 两端落地 shadcn token 体系，替换 `tokens.css` 拼盘与 Arco；H5 重建全屏视频 + 叠加层 + 底部动作坞的视觉，PC 重建控制室三区 + 自建 `<PageShell>`/`<DataTable>`。
  - H5 把 §12.3 招牌交互集建在现有 `atmosphere.ts`/canvas 层之上（Hammer Hold、名次跃迁、odometer、强度自适应、outbid 冲击波、品牌 confetti）；严格遵守 §12.5 约束与 §12.6 预算。
  - PC 落地 §13.2 三库分工与 §13.3 密集表、§13.4 实时 UX 规范。
  - **红线不变**：所有视觉/动效仍只呈现服务端权威结果，乐观态只是提示；§2.3 七条不变量测试始终绿。
- **Phase 2/3（视觉协同）**：媒体契约落地后，H5 的全屏视频背景换成真实 LL-HLS 流，叠加层视觉与 §11.6 浮层契约不变（媒体掉线时叠加层与竞拍真相不受影响）。

---

## 15. 视觉/交互验收与开放问题

### 15.1 验收标准（视觉/交互维度）

- **设计系统**：`tokens.css` 拼盘与 Arco 已被统一 shadcn + OKLCH token 体系替代；两端共享一套 token/字体/语义色；新增拍卖状态语义 token（leading/outbid/won/lost/bid-cta/flash-up/flash-down/live/stale/paused）就位；UI**不像默认 shadcn**（自定义中性基底 + 非默认强调色 + 小圆角 + tabular 数字 + CVA 业务变体）。
- **H5 招牌交互**：§12.3 至少落地 Hammer Hold→权威 SOLD、共享元素名次跃迁、价格 odometer、强度自适应房间能量四件；每件遵循"蓄力→焦点→退场"且只在权威事件兑现；真机（含 iOS Safari）走查通过。
- **可访问性/红线**：无 >3 闪/秒；`prefers-reduced-motion` 下非必要动效关闭；iOS 无震动时关键反馈有视觉/听觉等价；动效在恢复中/stale/断连自动收敛（`shouldGateAtmosphere`）；ARIA live region 播报关键变更。
- **性能**：氛围层在末秒 1000 笔突发下出价交互 INP p75 ≤200ms 有 `web-vitals` 证据；动画只动 transform/opacity；canvas 高频粒子不掉帧。
- **PC 控制台**：暗色控制室视觉落地；三库分工各就位（稳态 shadcn-Charts、实时 ECharts 含窗口化/节流、flight-recorder visx）；密集表 60FPS（万行）+"滚动暂停/N 条新/一次性 fade/行内 delta"；实时 UX 规范（≤5–9 KPI、Live/Stale/Paused、Data as of、skeleton、缓存快照）落地；图表统一吃 `--chart-*` token。

### 15.2 开放问题（视觉/交互维度）

1. **服务端是否发"going once/twice"标记事件？** Hammer Hold 的蓄力递进需要它；若无，需确认是用倒计时阈值近似，还是请后端补一个非权威的"提示性"标记事件（不影响判定）。
2. **Motion+ 付费是否可接受？** 若不可，价格 odometer 用自写 rAF DOM 方案（已规划），确认无遗漏其它付费依赖。
3. **末秒突发的音频节流窗口**取多大（≤1 cue/帧 vs 固定时间窗）需真机调优。
4. **`atmosphereSeenRef`（按 `cause_seq` 去重）长会话增长**是否需要定期清理/上限，避免内存累积。
5. **氛围层的 INP verifier**：需要一个可复现的"末秒 1000 笔"前端压测脚本来量氛围层的 INP，而非引用文档数字。
6. **PC 是否需要 SSR/首屏**（Next App Router）？若需，稳态走 SVG（Recharts/visx static）友好，ECharts/Canvas 一律 Client Component/动态 import。
7. **两端密度档与图表库边界**：token/字体/语义色共享已定，但密度档（PC 紧凑 vs H5 舒适）与"H5 是否也用 ECharts"需另行拍板。
8. **品牌字形/Logo 资产**：confetti 的 `shapeFromPath` 与 Display 衬线选型依赖是否有可用的品牌字形/视觉资产——目前未知，需向用户确认。

---

## 附：本规划引用的代码锚点（当前仓库真实路径）

- 媒体接缝：`frontend/mobile-h5/src/components.tsx:57`（`LiveStage`，内含 `const videoURL = demoLiveVideoURL` 与 `<video className="live-video-bg" src={videoURL}>`）；`frontend/mobile-h5/src/domain.ts:4`（`displayMediaURL`）、`:392`（`demoLiveVideoURL`/`demoProductImageURL`）。
- **视觉/氛围锚点（第二部分）**：`frontend/mobile-h5/src/atmosphere.ts`（氛围大脑：`AtmosphereKind`/`AtmosphereCue.cause_seq`/`calculateAtmosphereIntensity`/`shouldGateAtmosphere`）；`frontend/mobile-h5/src/domain.ts`（音频引擎 `createAudioContext`/`loadAuctionSoundPack`/`playLayeredCue`、`vibratePattern`）；`frontend/mobile-h5/src/components.tsx`（`ClimaxLayer`/`BidWaterfall`/`FinalSecondsLayer`/`BarrageLayer`/`HeatMeter`/`RaceBoard`，canvas+rAF+DPR 钳制）；`frontend/mobile-h5/src/main.tsx`（`showAtmosphere` 按 `cause_seq` 去重、`commitLeaderboardDelta` 拒过期 seq、leaderboard rAF 合帧）；`frontend/shared-design/tokens.css`（91 行拼盘，待替换为 OKLCH 体系；含 `.tabular-nums`）。
- 出价/实时/恢复契约：`frontend/mobile-h5/src/domain.ts`（`BidResponse`/`deriveCountdown`/`isDangerousActionDisabled`/`isBidCloseGuardActive`/`createClientBidID`）；`frontend/mobile-h5/src/main.tsx`（ws-ticket、`submitBid`、`confirmBid`、`recoverFromSnapshot`、`BID_REQUEST_TIMEOUT_MS`）。
- 支付边界（后端已存在）：`backend/internal/auction/bid.go`（状态 `:56`、`PayMock :376`、`HandleProviderWebhook :485`、`ReconcileProviderPayments :595`、`Sign/VerifyProviderWebhook :680`）；`backend/migrations/202605240001_payment_provider_boundary.sql`；H5 `frontend/mobile-h5/src/main.tsx:2011`（`payOrder`）、`:1148`（重查恢复）；PC 只读 `frontend/pc-console/src/components.tsx:2129`。
- 栈现状：React 18.3.1 + Vite 5.3.3 + TS 5.5.3；H5 = motion 12.40 + @icon-park + lucide + canvas-confetti + Web Audio + 原生 WebSocket；PC = Arco Design 2.65 + lucide；`shared-design/tokens.css`。

## 附：外部工业参考（仅供对标，不替代本项目实现）

- 媒体：MediaMTX（`mediamtx.org`、`github.com/bluenviron/mediamtx`，`alwaysAvailableFile` 循环、`-c copy`、多协议出口）；HLS/LL-HLS、hls.js、mpegts.js、WebRTC/WHEP、iOS Managed Media Source（17.1+）；`ffmpeg -stream_loop -1 -re -c copy`。
- 前端：TanStack Query v5（WS 喂缓存、`setQueryData`/`invalidateQueries`/`staleTime:Infinity`，TkDodo）、Zustand/Jotai/Redux Toolkit、XState v5（Stately）、TanStack Virtual、Feature-Sliced Design、Ant Design Pro ProComponents（蓝图）、web.dev INP（2024-03-12 取代 FID，p75 good≤200ms）、`web-vitals`、MDN Performance/Event Timing/Long Tasks。
- 支付：Stripe PaymentIntent 生命周期与 API 设计博客（`next_action`、无 `failed` 态、idempotency、webhook 去重/验签/重放容忍）、Alipay 开放文档（`wap.pay`/`page.pay`/`precreate`/`trade.query`/`refund`、`return_url` vs `notify_url`）、Adyen Alipay、Baymard 支付方式 UX。
- 视觉/设计系统（第二部分）：shadcn/ui Theming 与 Chart 文档（`cssVariables`、`--chart-*`、源码归你非 wrapper、discussion #7241 黑白图坑）、OKLCH（感知均匀/P3/WCAG 可预测）、Tailwind v4 `@theme inline`、shadcn `--sidebar-*` token、"建自己的设计系统而非堆 block"（Jan Marshal）、Tremor（Vercel 收购、MIT）、思源黑体/Source Han Sans。
- 交互/动效/多感官（第二部分）：Motion（Framer Motion）v12（`layoutId` 共享元素 FLIP、`AnimatePresence`、spring、`AnimateNumber` 属 Motion+ 付费）、View Transitions API（同/跨文档支持矩阵 + 降级）、canvas-confetti（`useWorker`/`disableForReducedMotion`/`shapeFromPath`/≤150 粒子）、Web Audio（`GainNode`/`DynamicsCompressor`/`AudioParam` 分层）、Yu-kai Chou anticipation parade、NN/g 动画时长（100–400ms/ease-out）、WCAG 2.3.1（≤3 闪/秒 Level A）/2.3.3/`prefers-reduced-motion`、iOS Safari 无 `navigator.vibrate`。
- 数据可视化/控制台（第二部分）：Apache ECharts（Canvas/`appendData`/增量/TypedArray + 流式累积掉帧上限）、visx（Airbnb，最小包体、可观测性/radial timeline）、TanStack Table v8/v9 + TanStack Virtual（虚拟化/只订阅必要 state/grouping 1000× 慢坑）、Recharts、ApexCharts gauge、Bloomberg 终端 UX、Tufte（data-ink/small multiples/sparkline）、RED/Four Golden Signals、Smashing《UX Strategies for Real-Time Dashboards》、Logz.io Live Tail、Honeycomb trace waterfall、Linear/Sentry/Vercel 暗色风格。
