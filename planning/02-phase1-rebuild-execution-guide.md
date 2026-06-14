# Phase 1 双前端重构 + 视觉/交互/氛围 —— 工程落地执行手册

> **本文性质**：给「实现模型 / 实现者」直接照着做的**执行手册（playbook）**，不是论证文档。论证与选型理由见 `planning/01-frontend-media-payment-refactor.md`（下称「调研稿」），本文只讲**做什么、按什么顺序做、做到什么算完、绝不能碰什么**。
> **本期范围**：仅 ① 调研稿 **Phase 1：双前端重构（行为保持）** + ② 调研稿 **第二部分 §11–§15（视觉·交互·氛围）全部**。
> **本期不做**：媒体只保留「直接放视频演示」，**只留一个口子**给后续（不实现 MediaPlayback 契约/Live Session API/MediaMTX/WebRTC）；支付保持现有 mock 行为，只留口子（不接真实跳转/支付宝）。详见 [§10](#10-媒体口子本期唯一要动媒体的地方) / [§11](#11-支付口子行为保持) / [§13](#13-留给后续阶段的口子汇总)。
> **硬约束（用户明确）**：**禁止使用 Motion+ 付费功能**（如 `AnimateNumber`）。数字滚动一律自写 rAF。见 [§9.2](#92-motion-允许付费禁止清单)。
> **最高优先级红线**：不得破坏服务端权威的出价/决策/结算/恢复内核。**正确性 > 视觉**。见 [§2](#2-红线契约任何-pr-不得回归)。

---

## 0. 给实现模型的元说明（先读这节）

### 0.1 你的工作方式（强制）

1. **行为保持式重构（strangler-fig）**：本期是「换骨架 + 换皮 + 加效果」，**不是改业务语义**。同样的输入必须产生同样的服务端交互与同样的最终 UI 结果。
2. **先锚后改**：动任何一个内核相关模块前，先为 [§2](#2-红线契约任何-pr-不得回归) 的不变量写 characterization test（行为快照测试）锚住现状，再做等价搬迁，最后才重构内部。**没有锚定测试不许改内核相关代码。**
3. **小步可回滚**：每个工作包（WP）独立成 PR、独立可回滚、独立过验收门。不要一个大 PR 推翻两个前端。
4. **不确定就停**：遇到「这个改动会不会影响出价正确性 / 恢复 / 结算」拿不准时，**停下来留 TODO 并标记 `// RED-LINE-REVIEW`，不要赌**。
5. **不过度抽象**：列表/表单用普通 hooks；状态机只用在出价 + 连接两条流；媒体只留薄口子，别提前把 Phase 2 契约写出来。

### 0.2 阅读顺序

`§2 红线` → `§1 范围` → `§3 目标结构` → `§4 依赖脚手架` → `§5 执行总顺序`（按 WP 顺序做）→ 对应的 `§6/§7/§8/§9/§10/§11` 细则 → `§12 验收`。

### 0.3 完成的定义（Definition of Done，全局）

一个 WP 算完成，当且仅当：① 该 WP 的验收项（见各 WP）全绿；② [§2](#2-红线契约任何-pr-不得回归) 的不变量 characterization test **全部仍绿**；③ 现有 e2e/单测无回归；④ 真机/浏览器走查（golden path + 弱网 + 断连恢复）通过；⑤ 无新增 Motion+ 付费依赖、无写死媒体地址（除 §10 指定的口子内部）。

---

## 1. 本期范围与边界

### 1.1 做（In Scope）

| 块 | 内容 | 章节 |
|---|---|---|
| 前端去巨石（行为保持） | 双前端按务实 FSD 分层；服务端派生态迁 TanStack Query（WS 喂）；纯 UI 态进 Zustand；出价+连接两条流上 XState；长列表上 TanStack Virtual | [§6](#6-h5-端落地细则) [§7](#7-pc-端落地细则) |
| 组件体系迁移 | 两端统一 **shadcn/ui + Tailwind + Radix**，替换 PC 端 Arco 与自研 `tokens.css` 拼盘 | [§7.1](#71-arcoshadcn-迁移行为保持) [§8](#8-视觉系统落地auction-terminal) |
| 视觉识别系统 | "Auction Terminal" + OKLCH token 体系 + 拍卖状态语义 token + 三角色字体 + 去同质化 | [§8](#8-视觉系统落地auction-terminal) |
| 标志性交互/动效/多感官 | 建在现有 `atmosphere.ts`/canvas/音频引擎之上的招牌交互集 | [§9](#9-交互动效多感官落地) |
| PC 控制台视觉 + 数据可视化 | 暗色控制室 + 三库分工（shadcn-Charts/ECharts/visx）+ 密集表 + 实时不过载 UX | [§7](#7-pc-端落地细则) |

### 1.2 不做（Out of Scope，只留口子）

- **媒体**：本期**直接放视频演示**（保留写死的 `demoLiveVideoURL` 行为），只把取流点收敛成一个**薄口子**（[§10](#10-媒体口子本期唯一要动媒体的地方)）。**不**实现 `MediaPlayback` 完整契约、`GET /api/live/sessions/{id}`、MediaMTX、LL-HLS/hls.js、WebRTC/WHEP、降级链。
- **支付**：保持现有 `pay-mock` 行为与状态逻辑，仅做视觉重建 + 留口子（[§11](#11-支付口子行为保持)）。**不**做 `POST /api/orders/{id}/payments` 泛化、`client_action`、真实跳转、支付宝适配器、对账定时 runner。
- **后端**：本期**不改后端任何代码**（含 `bid.go`、迁移、WS 协议语义）。
- **Motion+ 付费功能**：禁止。

### 1.3 边界判定速查

> 「这个要不要在本期做？」——若它属于「让现状更分层/更好看/更有反馈」→ 做；若它属于「让媒体变成真直播」或「让支付变成真支付」→ 不做，留口子。

---

## 2. 红线契约（任何 PR 不得回归）

这 7 条来自调研稿 §2.3 与现有代码语义，是**重构护栏**。**第一个 WP（WP-0）就要把它们写成 characterization test 并长期保持绿。**

| # | 不变量 | 锚点（现仓库真实位置） | 断言要点 |
|---|---|---|---|
| I1 | 出价响应解读以服务端字段为准 | `mobile-h5/src/domain.ts` `BidResponse.{result, reject_reason, code, decision_status, durability_status, confirm_token, seq, engine_seq}` | UI 的 accepted/rejected/confirm-required 只由这些字段推导；**HTTP 200 ≠ 成功** |
| I2 | WS 接入用一次性票据 | `POST /api/auth/ws-ticket` → `new WebSocket(url, ['auction.v1','ticket.xxx'])` | 票据 scope 绑 room/auction/user、消费即失效；子协议不变 |
| I3 | 断线恢复来源与触发完整 | `recoverFromSnapshot`（`main.tsx`） | gap/outbox gap/断连/stale/手动刷新/出价后不确定 → 恢复；来源 history/db/redis_stale/snapshot_unavailable 都要处理 |
| I4 | 倒计时服务端时间锚定 | `deriveCountdown(endAt, serverTimeMS, nowMS, serverTimeSyncedAt, …)`（`domain.ts`） | 用「上次同步后的本地经过时间」，手机时钟跳变不得自行宣布结束 |
| I5 | 危险操作禁用 | `isDangerousActionDisabled` / `isBidCloseGuardActive`（`domain.ts`） | 断连/恢复中/stale/临近落槌时出价按钮禁用 |
| I6 | 出价幂等三层 | `createClientBidID`、`BID_REQUEST_TIMEOUT_MS`、`AbortController`（`main.tsx`） | 同 `client_bid_id` 超时重试复用；8s 超时 → 不确定态 → 幂等重试；不得削弱 |
| I7 | 支付以服务端真相为准 | `order_status==='PAID'`、`Idempotency-Key`、重查兜底 `GET /api/users/me/orders`（`main.tsx`）；PC 只读 | 处理中屏只由服务端 `order_status` 推进；PC 永无支付控件 |

**强制规则**：
- 触碰 I1–I7 任一相关路径的 PR，必须附带或更新对应的 characterization test，且 PR 描述里点名「本 PR 与哪条不变量相关、如何验证未回归」。
- 乐观更新（optimistic）**只是「已发送」提示**，状态停在 `pending`；**唯有服务端 WS 决策事件（`ENGINE_*` + durability + settlement）**才能把状态翻成 accepted/rejected/outbid/sold。这条在 [§9](#9-交互动效多感官落地) 的每个动效里都要体现。

---

## 3. 目标工程结构

### 3.1 分层与依赖规则（两端一致）

```
app        路由、Provider、QueryClient、store 装配、主题注入
 └ pages     各屏/各页（仅组合 widgets，薄）
   └ widgets   live-stage(H5) / live-ops(PC) / orders-panel / monitor ...（成块的 UI）
     └ features  place-bid / connection / pay-order / diagnostics / live-media(口子) ...
       └ entities auction / bid / order / live-session(口子) / payment（领域类型 + 取数 + 模型）
         └ shared  设计 token、shadcn ui 原语、网络封装、WS 客户端、工具
依赖规则（铁律）：只能向下引用；禁止平级互相引用、禁止向上引用。
```

- **PC 与 H5 复用 `shared / entities / features`**；差异只在 `widgets / pages`。
- 逻辑复用单元 = **自定义 hook**（不要人为拆 container/presentational）；展示组件保持纯渲染。
- 上 ESLint 依赖规则（如 `eslint-plugin-boundaries` 或等价），把分层写成可机检规则，违例即 CI 失败。

### 3.2 共享设计语言的物理落点

monorepo 内一个共享包（例如 `frontend/shared-design` 升级为可被两端 import 的 `ui` 包），承载：OKLCH token、字体、shadcn base 组件、拍卖语义 token、CVA 业务变体。两端通过同一 preset 复用，仅密度档（PC 紧凑 / H5 舒适）在各自 `app` 层覆盖。**这是「一套语言、两个应用」的根**。

### 3.3 单文件体量目标

- 目标：无单文件 >800 行的「上帝组件/上帝模块」；逻辑在 hooks，展示在纯组件。
- 现状基线（待拆）：H5 `main.tsx≈2526` / `components.tsx≈2359` / `domain.ts≈1085`；PC `main.tsx≈1394` / `components.tsx≈2442` / `domain.ts≈893`。

---

## 4. 依赖与脚手架（WP-0，不改行为）

### 4.1 需要引入/确认的依赖

| 用途 | 选型 | 备注 |
|---|---|---|
| 服务端状态缓存 | **TanStack Query v5** | WS 喂缓存，`staleTime: Infinity` |
| 客户端状态 | **Zustand** | selector 精准重渲染 |
| 危险流状态机 | **XState v5** | 仅出价 + 连接两条流 |
| 长列表虚拟化 | **TanStack Virtual** | 飞行记录器/订单/日志万行 |
| 组件体系 | **shadcn/ui + Tailwind v4 + Radix** | `components.json` 设 `cssVariables:true` |
| 动效 | **Motion（OSS）v12** | H5 已有 `motion 12.40`；PC 新增。**仅用免费 API**（[§9.2](#92-motion-允许付费禁止清单)） |
| 彩带 | **canvas-confetti** | H5 已有；`useWorker:true` + `disableForReducedMotion:true` |
| 图表（PC 稳态） | **shadcn Charts（=自有 Recharts 源码）**，KPI/spark 可借 **Tremor** | 吃 `--chart-*` token |
| 图表（PC 实时） | **Apache ECharts** | `echarts/core` 按需注册；窗口化/节流 |
| 图表（PC 定制） | **visx** | flight-recorder 时间线/服务关系/p99 仪表 |

### 4.2 脚手架步骤（全部不改运行时行为）

1. 装依赖；在 `app` 层装配 `QueryClient` + `<QueryClientProvider>`、Zustand store 根、`MotionConfig`（透传 `prefers-reduced-motion`）。
2. 初始化 shadcn（`components.json`，`cssVariables:true`），建共享 `ui` 包骨架与 OKLCH token 占位（[§8](#8-视觉系统落地auction-terminal)）。
3. 建 `shared/entities/features/widgets/pages/app` 目录骨架 + 依赖规则 lint。
4. 写 WP-0 的 characterization test（[§2](#2-红线契约任何-pr-不得回归) I1–I7）。
5. **验收 WP-0**：依赖与目录就位；现有 e2e/单测全绿；新增不变量测试全绿；构建无回归；**业务逻辑、UI、后端一行未动**。

---

## 5. 执行总顺序（工作包 WP 与依赖图）

> 顺序原则：先锚定 → 先搭体系（token/外壳）→ 再搬状态（行为保持）→ 再换皮 → 最后叠效果。**效果叠在「状态已迁移、皮已换」之上，不要在巨石上直接糊动效。**

| WP | 名称 | 依赖 | 产出 | 章节 |
|---|---|---|---|---|
| **WP-0** | 脚手架 + 不变量锚定 | — | 依赖/目录/Provider/lint/characterization test | [§4](#4-依赖与脚手架wp-0不改行为) |
| **WP-1** | OKLCH token 体系 + 共享 ui 包 | WP-0 | Auction Terminal token、语义 token、字体、shadcn base | [§8](#8-视觉系统落地auction-terminal) |
| **WP-2** | H5 状态迁移（行为保持） | WP-0 | RQ 喂缓存、`useConnection`/`ReconnectionManager`、出价/连接 XState、Zustand UI 态 | [§6.1](#61-状态迁移行为保持) |
| **WP-3** | H5 组件拆分 + 视觉重建 | WP-1,WP-2 | `widgets/features` 拆分、全屏视频+叠加层+动作坞重建 | [§6.2](#62-组件拆分与视觉重建) |
| **WP-4** | H5 媒体口子 | WP-3 | `useLiveMediaSource` + `<LiveBackdrop>` 薄边界（仍放 demo 视频） | [§10](#10-媒体口子本期唯一要动媒体的地方) |
| **WP-5** | H5 招牌交互/动效/多感官 | WP-3 | 建在 `atmosphere.ts` 上的招牌集；rAF odometer；WCAG/红线门 | [§9](#9-交互动效多感官落地) |
| **WP-6** | PC Arco→shadcn 迁移（行为保持） | WP-1 | `<PageShell>`/`<DataTable>`、各页等价搬迁 | [§7.1](#71-arcoshadcn-迁移行为保持) |
| **WP-7** | PC 控制室视觉 + 数据可视化 | WP-6 | 暗色控制室、三库分工、密集表、实时 UX 规范 | [§7.2](#72-控制室视觉与数据可视化) |
| **WP-8** | 支付 UI 视觉重建（行为保持） | WP-1,WP-3 | shadcn 重建支付/订单态 UI，逻辑不变，留口子 | [§11](#11-支付口子行为保持) |

H5 线（WP-2→3→4→5）与 PC 线（WP-6→7）在 WP-1 之后可并行。WP-8 待 WP-3 的支付屏骨架就绪后做。

---

## 6. H5 端落地细则

### 6.1 状态迁移（行为保持）

把 `main.tsx` 那 150+ inline hooks 按「数据归属」三分，**先抽 hook，再抽 JSX**：

1. **服务端派生数据 → TanStack Query 缓存（WS 喂）**
   - 价格、领先者、倒计时所需的服务端时间锚、连接态可见数据、订单态等：用 `useQuery` 先 REST 取快照（snapshot），WS 事件经 `queryClient.setQueryData` 写实时字段、`invalidateQueries` 触发列表重取，`staleTime: Infinity`（禁止 RQ 自作主张轮询）。
   - **突发合帧**：1000 笔/末秒**不要 per-message setState**。所有 delta 缓冲到 ref，每 `requestAnimationFrame` 刷一次（只取最新价 + 批量追加日志）。沿用现有 leaderboard rAF 合帧范式。
2. **WS 接入收敛为 `ReconnectionManager` + `useConnection()`**（`shared/ws` + `features/connection`）
   - 封装：ws-ticket 取票（I2）、`auction.v1` 子协议、指数退避 + jitter、心跳、`sessionId`/`lastSeqId` 恢复、pending ack Map、gap 检测、snapshot-then-stream、慢消费者断开。
   - **只搬不改**：把散落在 `main.tsx` 的 WS 逻辑搬进来，协议语义、恢复来源（I3）一字不改。
3. **纯客户端 UI 态 → Zustand**：抽屉/弹层开关、「暂停直播」缓冲、乐观标志、本地偏好等。
4. **出价交互 → XState v5（`features/place-bid`）**：
   ```
   states: idle → pending →（accepted | rejected | confirmRequired → confirming →（accepted|rejected）)
   并行/外部事件：服务端可随时推 outbid → 进 outbid 态
   保证：confirm 未完成不可能显示已接受；HTTP 200 不翻状态；只有 WS 决策事件（I1）翻状态
   超时：BID_REQUEST_TIMEOUT_MS=8s + AbortController → uncertain → 幂等重试（复用 client_bid_id，I6）
   ```
5. **连接/恢复 → XState v5（`features/connection`）**：
   ```
   states: connected | reconnecting | resuming | degraded
   规则：socket 掉线强制进 fail-closed/不确定态，绝不停留在「你正领先」；恢复完成才回 connected
   ```

**WP-2 验收**：I1–I7 测试全绿；出价/确认/恢复/倒计时/支付既有 e2e 通过；服务端派生态已无手写 `useState` 缓存；WS 逻辑集中在 `ReconnectionManager`；出价与连接走 XState。**UI 外观此步可暂不变**（先保证行为等价）。

### 6.2 组件拆分与视觉重建

- 把 `components.tsx`（40+ 组件）按 `widgets/features` 切；`LiveStage` 拆成：背景媒体（→ [§10](#10-媒体口子本期唯一要动媒体的地方) 口子）、叠加信息层、底部动作坞、氛围层（保留 `ClimaxLayer`/`BidWaterfall`/`FinalSecondsLayer`/`BarrageLayer`/`HeatMeter`/`RaceBoard`）。
- 视觉按 [§8](#8-视觉系统落地auction-terminal) 的 Auction Terminal：全屏视频 + 底部 scrim（约 40%）+ 低模糊玻璃面板托金钱关键 UI；`--bid-cta` 为全屏唯一最高饱和点。
- **保留全部氛围/音效/合帧能力**，只换它们的视觉皮与触发接线（接线仍由服务端事件驱动）。

**WP-3 验收**：无 >800 行上帝组件；金钱关键 UI（当前价/出价按钮/倒计时）在任意视频画面上 WCAG 达标；真机走查（golden path + 弱网 + 断连恢复 + 末秒突发）通过；I1–I7 仍绿。

---

## 7. PC 端落地细则

### 7.1 Arco→shadcn 迁移（行为保持）

- **自建两个装配件**（复刻 Ant Pro 工效，地基是 shadcn）：
  - `<PageShell>`：shadcn Sidebar（用 `--sidebar-*` token 配成更深材质）+ topbar（放全局 freshness/连接态/命令面板）+ 内容区 + 底部工具条。
  - `<DataTable>`：**TanStack Table（headless）+ shadcn `Table` 样式 + TanStack Virtual**；含搜索区/工具栏/分页/列宽可调/关键列 pin。
- **等价搬迁**：每个 Arco 页（上架/配规则/排期/启动/监控/复盘/订单只读）逐页换成 shadcn 实现，**同输入同结果**；先过该页的行为快照测试再替换。
- **工程要点**：virtualizer 放尽量低层；只订阅必要 state；**live 日志禁用 grouping**（`groupBy` 大数据有 1000× 慢坑）；header 背景设非透明防滚动穿透；动态行高用 `estimateSize` + Firefox 特判。
- **支付只读保持**：PC 永无支付控件（I7）；只读展示状态徽标/`provider_payment_id`/事件数/对账异常。

**WP-6 验收**：所有 Arco 组件已被 shadcn 等价替换；各页行为快照无回归；万行日志 60FPS；I7（PC 只读）保持。

### 7.2 控制室视觉与数据可视化

- **视觉方向**（调研稿 §13.1）：外壳/稳态页走 Linear/Sentry 式产品化暗色；拍卖现场/出价流/飞行记录器走 Bloomberg 式高密度（等宽数字、密集行、左上 telemetry、sparkline）；底线永不纯白文字、暗色图表饱和度降 15–20%；信息层级按 RED/Four Golden Signals（最重要 KPI 左上）。
- **三库分工**（视觉统一靠 `--chart-*`/OKLCH token，不靠同一个库）：
  - 稳态图（KPI 卡/area/donut/行内 sparkline）→ **shadcn Charts（自有 Recharts 源码）**，Tremor 加速 KPI/spark。**坑**：`chartConfig` 用 `var(--chart-1)` 而非 `--primary`，否则图表黑白。
  - 实时密集（实时价格流/吞吐/末秒出价流/热力）→ **ECharts**（Canvas + `appendData` + 增量），**必做窗口化/降采样/节流（每 100–250ms 批量 flush）**，`echarts/core` 按需注册，长会话防累积掉帧。
  - 定制 wow（flight-recorder 时间线/span waterfall、服务关系图、p99 决策延迟仪表）→ **visx**。
- **密集表实时范式**：滚动即暂停自动跟随 + 顶部「N 条新 ↓」浮条 + 一次性 fade 高亮（<300ms，**不连续闪烁**）+ 行内 delta/sparkline。
- **实时不过载 UX 规范**（写成设计系统一页，可验收）：动画 100–400ms/ease-out/尊重减动；每屏 ≤5–9 KPI、最关键左上；Data Freshness 指示 + 「Data as of …」+ 手动刷新；**Live/Stale/Paused** 三态；数据不可用显**带时间戳缓存快照**而非空白；连接失败指数退避，失败才显「Reconnecting…」；**skeleton 取代 spinner**；ARIA live region 静默播报；告警附「为何触发 + 决策时刻依据」（呼应 `CLAUDE.md`「reject 要有 decision-time basis」）；按重要性分更新频率。
- **金额/状态/赢家列只渲染服务端字段**（带 `ENGINE_*`/durability/settlement），表格不做任何「看起来像判定」的客户端推断。

**WP-7 验收**：暗色控制室落地；三库各就位且统一吃 `--chart-*`；ECharts 实时图有窗口化/节流；密集表实时范式落地；实时 UX 规范项可逐条勾验；freshness/连接态反映系统真实状态（不得假 Live）。

---

## 8. 视觉系统落地（Auction Terminal）

> 目标：把「改多次仍不满意」的根因——`tokens.css`(91 行) 无体系拼盘（Arco 蓝 `#165DFF` + TikTok 粉 `#FE2C55` + 金 `#D4AF37` 混搭）——替换为**一套 OKLCH token 体系**。要换的是体系，不是某个颜色。

### 8.1 OKLCH token 体系（WP-1 核心产物）

- 用 OKLCH（感知均匀/P3 广色域/WCAG 可预测）定义 shadcn 标准 token（`background/foreground/primary/card/border/ring/destructive…`），暗色优先。
- **不用默认 neutral**：选偏冷中性基底（控制室气质）+ **非默认品牌强调色**（避开 shadcn 示例紫）；单独定义 `--success/--warning/--danger/--info`（默认只有 `--destructive`）。
- 单一 `--radius` 基准派生圆角阶梯；拍卖/控制室偏**小圆角**（更硬、更终端）。

### 8.2 拍卖状态语义 token（把氛围沉淀进设计系统）

| token | 用途 | 红线绑定 |
|---|---|---|
| `--state-leading` | 我方领先 | 仅服务端确认领先时点亮；乐观 pending 用中性「已发送」色 |
| `--state-outbid` | 被超越 | 仅 WS `outbid` 事件触发 |
| `--state-won` / `--state-lost` | 成交结果 | 仅 settlement 终态；金色=won |
| `--bid-cta` | 出价主按钮（稀有高饱和） | 全局唯一最高饱和点 |
| `--flash-up` / `--flash-down` | 价格涨/跌方向闪动 | 仅服务端确认价变；一次性 fade，不持续闪 |
| `--live` / `--stale` / `--paused` | 连接/数据新鲜度 | 必须反映系统真实状态，不假 Live |

原则：永不纯白正文；金克制在 5–15% 面积（威望/成交/won 才用）；**状态永不只靠颜色**（叠图标/形状/文字，WCAG 1.4.1）。

### 8.3 字体三角色

| 角色 | 选型 | 用途 |
|---|---|---|
| Display 衬线（高对比） | 一套高对比衬线 | 拍品标题、落槌大字、威望时刻 |
| UI 无衬线 | 有性格的 grotesk/sans（避开 Inter/Geist 默认味），Latin-first + **思源黑体/Source Han Sans** CJK fallback | 正文/按钮/表单 |
| 等宽/Tabular | 等宽或 `font-variant-numeric: lining-nums tabular-nums` | **所有实时数字**（价格/出价额/倒计时/日志/指标） |

（现有 `.tabular-nums` 类方向对，扩成完整三角色并接进 shadcn 字体 token。）

### 8.4 去同质化清单（别像默认 shadcn）

1. 重定义 token（§8.1）；2. 改 `--radius` 与密度档；3. 换字体（§8.3）；4. 改 **CVA variants** 而非到处加 class（`badge: live/stale/paused/sealed/won`）；5. 业务用**包装/复合组件**承载，**绝不直接改 `ui/` 基础组件**（保持可升级）；6. 侧栏用 `--sidebar-*` 配成另一种材质。

**WP-1 验收**：`tokens.css` 拼盘被 OKLCH 体系替代；两端共享同一 token/字体/语义色 preset；语义 token 就位；示例页观感**不像默认 shadcn**（自定义中性基底+非默认强调色+小圆角+tabular 数字+CVA 业务变体）。

---

## 9. 交互/动效/多感官落地

> 原则：建在**现有 `atmosphere.ts`/canvas/音频引擎**之上，不另起炉灶。差异化叙事 = 「正确性感知的炫技」——动效服务端事件驱动（keyed by `cause_seq`）、强度自适应、不确定态自动收敛。

### 9.1 复用的现有弹药（不要重写）

- `atmosphere.ts`：`AtmosphereKind`/`AtmosphereCue(cause_seq,event_type,user_scope,priority)`/`AtmosphereIntensity(0–3)`/`calculateAtmosphereIntensity`/`shouldGateAtmosphere`（恢复中/stale/断连硬门，reducedMotion/低电量/AI 关软门）/`atmospherePriority`/`normalizeAtmosphere`/`clampIntensity`/`nextAtmosphereCueID`。
- `domain.ts`：`createAudioContext()`(:661)、`vibratePattern()`(:706)、`playLayeredCue()`(:769)。
- `components.tsx`：`ClimaxLayer`/`BidWaterfall`（canvas + rAF + DPR 钳制）等。
- `main.tsx`：`showAtmosphere` 按 `cause_seq` 去重、`commitLeaderboardDelta` 拒过期 seq。

### 9.2 Motion 允许/付费禁止清单

- **允许（Motion OSS / 免费）**：`motion.*` 组件、`AnimatePresence`、`LayoutGroup`、`layout`/`layoutId`（FLIP 共享元素）、`useAnimate`、`useMotionValue`/`useTransform`/`useSpring`、variants、手势（`whileHover`/`whileTap`/`drag`）、`MotionConfig`（reducedMotion）。
- **禁止（Motion+ 付费）**：`AnimateNumber`、`Cursor`、`Ticker` 等 Motion+ 专属件，以及任何需要 Motion+ 许可的资源。
- **数字滚动一律自写 rAF**：价格/出价额 odometer = 自写 DOM 文本节点 + `requestAnimationFrame` 插值，**不依赖任何付费件**。

### 9.3 招牌交互集（每个遵循「蓄力 → 单一焦点兑现 → 干净退场」）

| 招牌 | 兑现条件（红线） | 实现要点 |
|---|---|---|
| **落槌 Hammer Hold → 权威 SOLD** | **仅 settlement 终态触发**，绝不前端预判 | going once/twice 递进 + 短暂「屏息」停顿 → 权威 `sold` 到达瞬间 SOLD 大字 + 金色定格 → 收敛结果卡 |
| **共享元素名次跃迁** | 仅服务端确认领先后（`commitLeaderboardDelta` 拒过期 seq） | Motion `layoutId` FLIP 把头像/名次滑入榜首 |
| **价格 odometer + 涌动分层** | 仅服务端确认价变 | **自写 rAF odometer** + `--flash-up/down` 一次性 fade + 强度分档涌动 |
| **强度自适应房间能量** | 由 `calculateAtmosphereIntensity` 0–3 驱动 | 同时当视觉强度与渲染预算（§9.5） |
| **Outbid 冲击波** | 仅 WS `outbid` 事件 | 视觉脉冲 +（安卓）震动；引导再出价 |
| **品牌字形 confetti + 视图转场** | 仅成交 | `canvas-confetti` `shapeFromPath` 品牌字形 ≤150 粒子 + View Transitions 屏间交接 |

### 9.4 多感官硬约束（红线 + 可访问性）

- **iOS Safari 无 `navigator.vibrate`**：触觉只能作**安卓 Chrome 增强**，**永不承重**（关键反馈必须有视觉/听觉等价）；现有 `vibratePattern` 按此降级。
- **WCAG 2.3.1（Level A）**：任何东西**不得 >3 闪/秒**——价格闪动/outbid 脉冲用一次性 fade，禁连续 blink。
- **WCAG 2.3.3 + `prefers-reduced-motion`**：减动下关非必要动效（`shouldGateAtmosphere` 软门已处理，接线时尊重它）。
- **ARIA live region** 播报关键变更（价格/领先/成交）。
- 动画**只动 `transform`/`opacity`**（合成器线程）；时长 100–400ms/ease-out/入场略长于出场。

### 9.5 末秒 1000 笔渲染预算

把 `AtmosphereIntensity`(0–3) 当**渲染预算调速器**：rAF 合帧（所有 delta 缓冲，每帧刷一次最新+批量）；高频粒子用 **canvas 不用 DOM**；**音频节流 ≤1 cue/帧**；强度越高越优先「聚合表达（一个涌动）」而非「逐条表达（千条闪烁）」。用 `web-vitals` `onINP`（p75 ≤200ms）给氛围层打点，`PerformanceObserver('longtask')` 告警 >50ms。

**WP-5 验收**：§9.3 至少落地 Hammer Hold→权威 SOLD、共享元素名次跃迁、价格 odometer、强度自适应四件；每件只在权威事件兑现；无 >3 闪/秒；减动下非必要动效关闭；iOS 无震动时关键反馈有视听等价；末秒 1000 笔出价交互 INP p75 ≤200ms 有 `web-vitals` 证据；**无任何 Motion+ 付费依赖**。

---

## 10. 媒体口子（本期唯一要动媒体的地方）

> 本期**直接放视频演示**，保留 `demoLiveVideoURL` 行为。唯一要做的是把「取流点」收敛成一个**薄边界**，让 Phase 2/3 能在不碰调用方的前提下替换。**不要把 Phase 2 的完整契约提前写出来。**

### 10.1 现状（已核验）

- `mobile-h5/src/domain.ts:392` `export const demoLiveVideoURL = '/demo/jade-live-loop.mp4'`。
- `mobile-h5/src/components.tsx:129` `const videoURL = demoLiveVideoURL;`
- `mobile-h5/src/components.tsx:185` `<video className="live-video-bg" src={videoURL} poster={mediaURL || demoProductImageURL} autoPlay muted loop playsInline aria-hidden="true" />`

### 10.2 本期要做的薄口子（仅此而已）

把上面三处收敛成 `features/live-media` 下一个 hook + 一个组件：

```ts
// features/live-media/useLiveMediaSource.ts —— 仅 Phase 1，故意保持最小
export interface LiveMediaSourceV0 {
  kind: 'video-file';        // 本期唯一取值；Phase 2 再扩 'll-hls' | 'whep'
  url: string;               // 本期 = demoLiveVideoURL
  posterURL?: string;
  isLive: boolean;           // 本期 = false（循环文件占位）
}

// 本期实现：直接返回 demo 描述符；不发任何请求、不查 Live Session API
export function useLiveMediaSource(auctionId: string): LiveMediaSourceV0 {
  return { kind: 'video-file', url: demoLiveVideoURL, posterURL: /* 经 displayMediaURL 的封面 */ , isLive: false };
}
```

```tsx
// widgets/live-stage/LiveBackdrop.tsx —— 本期只处理 'video-file'
function LiveBackdrop({ source, poster }: { source: LiveMediaSourceV0; poster?: string }) {
  // 本期：等价于现状那行 <video className="live-video-bg" ... />
  // Phase 2：在这里按 kind 分发到 hls.js / Safari 原生 / WHEP 适配器，调用方不变
  return <video className="live-video-bg" src={source.url} poster={poster} autoPlay muted loop playsInline aria-hidden="true" />;
}
```

`LiveStage` 改为 `const media = useLiveMediaSource(auctionId)` + `<LiveBackdrop source={media} poster={...} />`。**行为与现状完全等价**（仍是那段循环 MP4）。

### 10.3 本期**不要**做（留给后续阶段）

- 不定义完整 `MediaPlayback`（`MediaProtocol` 联合/`MediaSource[]`/`capabilities`/`latencyTargetMs`/降级链）。
- 不实现 `GET /api/live/sessions/{auctionID}`、不接 MediaMTX、不引 hls.js/mpegts.js、不做 LL-HLS/WebRTC、不做能力协商。
- 不改后端、不动 `displayMediaURL`（商品图/封面改写，与流无关，照旧）。

**WP-4 验收**：`LiveStage` 不再直接出现 `demoLiveVideoURL`/`<video src=…>`（已收敛进 `useLiveMediaSource`/`<LiveBackdrop>`）；运行时画面与现状一致（同一段循环视频，自动播放成功）；移动端 `muted+playsInline+autoPlay` 仍生效；I1–I7 不受影响。

---

## 11. 支付口子（行为保持）

> 本期支付**保持现有 mock 行为与状态逻辑**，只做视觉重建 + 留口子。**不接真。**

- **保留不变**：`pay-mock` 调用、`Idempotency-Key`、`order_status==='PAID'` 为成功真相（I7）、重查兜底 `GET /api/users/me/orders`、H5 `paymentPhase`（`idle|pending|paid|failed|expired`）状态流、PC 只读。
- **本期要做**：用 shadcn 重建支付/订单态 UI（发起→处理中→成功/失败/超时→重查恢复的各屏视觉），状态推进仍只由服务端 `order_status` 驱动；处理中屏**绝不**因任何「跳转回跳」断言成功；关页重开走重查而非重放。
- **留口子**：把买家支付入口封装成 `features/pay-order` 的一个动作边界（当前内部打 `pay-mock`）。后续阶段把内部换成 `POST /api/orders/{id}/payments` 读 `client_action` + 真实跳转/二维码 + 支付宝适配器，**调用方不变**。本期不写 `client_action`、不写适配器、不接对账 runner。

**WP-8 验收**：支付/订单态 UI 已 shadcn 化且视觉符合 Auction Terminal；I7 保持（PC 只读、服务端真相、幂等、重查）；既有支付 e2e 无回归；入口已封装成可替换边界但内部仍是 mock。

---

## 12. 验收（汇总）

### 12.1 全局门（每个 WP 都要过）

- [§2](#2-红线契约任何-pr-不得回归) I1–I7 characterization test 全绿，零回归。
- 客户端永不宣布成败/赢家/终态/价格；终态只来自服务端事件/快照。
- 无 Motion+ 付费依赖；无写死媒体地址（除 §10 口子内部）；后端未改。
- 真机/浏览器走查：golden path + 弱网 + 断连恢复 + 末秒突发。

### 12.2 分 WP 门

见各 WP 末尾「WP-N 验收」。关键量化门：出价交互 **INP p75 ≤200ms**（`web-vitals` 证据）；PC 万行日志 **60FPS**；动画 100–400ms；KPI/屏 ≤5–9；无 >3 闪/秒。

---

## 13. 留给后续阶段的口子汇总

| 口子 | 本期状态 | 后续阶段要做 |
|---|---|---|
| `useLiveMediaSource` / `<LiveBackdrop>`（[§10](#10-媒体口子本期唯一要动媒体的地方)） | 仅 `kind:'video-file'`，返回 demo 视频 | 扩 `MediaPlayback` 完整契约、`GET /api/live/sessions/{id}`、hls.js/Safari 原生/WHEP 适配器、降级链 |
| 媒体服务端 | 无 | MediaMTX `-c copy` 循环 MP4、LL-HLS 出口、HTTPS 反代、并发压测 |
| `features/pay-order` 动作边界（[§11](#11-支付口子行为保持)） | 内部 `pay-mock` | 泛化 `POST /api/orders/{id}/payments` + `client_action` + 真实跳转/二维码 |
| 支付适配器 | 无 | 支付宝 `wap.pay`/`page.pay`/`precreate` + RSA2 验签 + `trade.query` + 退款 + 分↔元换算 |
| 对账 | 无 | `ReconcileProviderPayments` 接入定时 runner |
| WebRTC/WHEP | 无 | 在 `<LiveBackdrop>` 内换适配器实现亚秒延迟，调用方不变 |
| 预留字段 | 无 | `orders.currency`、`out_trade_no` vs `provider_ref`、`payment_events.provider`/`event_type` 泛化 |

> 口子设计原则：**接口形状现在就对，内部实现以后填**；后续阶段是「换实现」不是「改调用方」。

---

## 14. 不要做清单（Consolidated Don'ts）

- 不改后端任何代码、不改 WS 协议语义、不削弱出价幂等/确认/恢复契约（I1–I7）。
- 不把 HTTP 200 当出价/支付成功；不让任何动效暗示前端在判定拍卖。
- 不在巨石上直接糊动效——先迁状态、再换皮、最后叠效果（[§5](#5-执行总顺序工作包-wp-与依赖图) 顺序）。
- 不实现 Phase 2/3 媒体（MediaPlayback 契约/Live Session API/MediaMTX/hls.js/WebRTC/降级链）；媒体只动 §10 薄口子。
- 不接真实支付（`client_action`/真实跳转/支付宝/对账 runner）。
- 不用 Motion+ 付费功能；数字滚动自写 rAF。
- 不把列表/表单做成状态机（XState 只用出价+连接两条流）。
- 不直接改 shadcn `ui/` 基础组件（业务用包装/复合组件）。
- live 日志不开 grouping；高频更新不 per-message setState；不连续闪烁（>3 闪/秒）。
- 不合并两个前端；不发明当前不需要的主播采集端/买家角色。

---

## 15. PR 切分建议（让评审与回滚都安全）

按 WP 切 PR，每个 PR 满足「行为保持 + 单一关注点 + 独立可回滚」：

1. `WP-0` 脚手架 + 不变量测试（不改行为）。
2. `WP-1` token 体系 + 共享 ui 包（先出一个 demo 页证明观感，不接业务）。
3. `WP-2` H5 状态迁移（**纯逻辑搬迁，UI 尽量不动**，最易出正确性问题，单独评审）。
4. `WP-3` H5 组件拆分 + 视觉重建。
5. `WP-4` H5 媒体薄口子（极小 PR）。
6. `WP-5` H5 招牌交互/动效（可再按招牌拆多个小 PR）。
7. `WP-6` PC Arco→shadcn（**可按页拆多个 PR**，每页一个，逐页过行为快照）。
8. `WP-7` PC 控制室视觉 + 可视化（可按「外壳 / 三库各一 / 密集表 / 实时 UX 规范」再拆）。
9. `WP-8` 支付 UI 视觉重建。

每个 PR 描述必须含：涉及哪条红线（I1–I7）、如何验证未回归、是否纯视觉/纯逻辑、回滚影响面。

---

## 附：本手册引用的真实代码锚点（已核验）

- 媒体口子：`mobile-h5/src/components.tsx:129`（`const videoURL = demoLiveVideoURL`）、`:185`（`<video className="live-video-bg" …>`）；`mobile-h5/src/domain.ts:392`（`demoLiveVideoURL`）、`:393`（`demoProductImageURL`）、`:4`（`displayMediaURL`，封面改写，**本期不动**）。
- 氛围/音频引擎：`mobile-h5/src/atmosphere.ts`（`calculateAtmosphereIntensity`/`shouldGateAtmosphere`/`AtmosphereCue.cause_seq`/`atmospherePriority`/`AtmosphereIntensity` 等）；`mobile-h5/src/domain.ts`（`createAudioContext:661`/`vibratePattern:706`/`playLayeredCue:769`）；`mobile-h5/src/components.tsx`（`ClimaxLayer`/`BidWaterfall` 等，canvas+rAF+DPR）；`mobile-h5/src/main.tsx`（`showAtmosphere` 按 `cause_seq` 去重、`commitLeaderboardDelta` 拒过期 seq）。
- 出价/实时/恢复契约（I1–I7）：`mobile-h5/src/domain.ts`（`BidResponse`/`deriveCountdown`/`isDangerousActionDisabled`/`isBidCloseGuardActive`/`createClientBidID`）；`mobile-h5/src/main.tsx`（ws-ticket、`submitBid`/`confirmBid`、`recoverFromSnapshot`、`BID_REQUEST_TIMEOUT_MS`、支付 `payOrder`、重查 `GET /api/users/me/orders`）。
- 支付只读：`pc-console/src/components.tsx`（只读支付/订单视图）。
- 待替换视觉拼盘：`frontend/shared-design/tokens.css`（91 行，含 `.tabular-nums`）。
- 栈现状：React 18.3.1 + Vite 5.3.3 + TS 5.5.3；H5 已有 `motion 12.40` + canvas-confetti + Web Audio + 原生 WebSocket；PC 现为 Arco（本期迁出）。
