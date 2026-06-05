# 极致竞价氛围体验 + AI 亮点 · 调研 / 评审 / 设计方案

> Status: 评委视角独立评审 + 行业调研 + 落地设计，2026-06-05。
> Scope: 聚焦官方宣讲版「评分亮点（加分项）· 💫 极致的竞价氛围体验」及其相邻产品面（前端交互、实时反馈、商家/主播端氛围控制、AI 创新）。
> 关系: 本文是**产品/体验/创新**维度的攻击性审查，不取代 `docs/current/` 的热路径架构权威；后端正确性结论沿用 `docs/current/architecture.md` 与既有 `docs/reviews/judge-review-2026-06-05.md`。
> 证据: 代码结论均带 `file:line`；行业结论均带联网来源（见文末「参考来源」，使用 Tavily/Fetch 核实，未用 websearch）。

---

## 0. 执行摘要（先读这一页）

**一句话结论**：这是一套**后端正确性达到工业级、但「极致竞价氛围」被严重低交付**的项目。引擎/WAL/对账/容错是冠军级，但宣讲版里最能在**答辩现场被「感受到」**的那条加分线（紧张到窒息的竞价氛围）目前只完成了「正确但克制」的骨架，缺少「极致」与「耳目一新」。**如不补齐，最大的风险不是被挑出 bug，而是在 demo 台上「不够燃」，被一个后端更弱、但现场更炸的对手在观感上反超。**

**总评分（独立评审，满分 100，按宣讲版权重折算）**

| 维度 | 评委判断 | 分数 |
|---|---|---|
| 技术实现与工程完整度（权重 50%）| 链路闭环、可用性、可观测性、容错俱佳；前端正确性/恢复一流 | **45 / 50** |
| 技术深度与创新性（权重 25%）| 引擎/WAL/fence/对账有真差异化；但**产品与 AI 创新为 0** | **19 / 25** |
| 第三维度（宣讲版表格此处被截断，约 25%，疑为「产品体验/交互/答辩展示」）| **见 §1 风险提示**；若该维度重「氛围/体验」，本项目当前明显失分 | **15 / 25（估）** |
| **合计** | 工程定生死，体验定名次 | **~79 / 100** |

**单独给「💫 极致竞价氛围体验」打分：6.0 / 10**（骨架正确、细节克制、峰值与惊喜缺失、且有「假」与「死按钮」拉低可信度）。

**Top 3 优点（必须保住）**
1. **氛围与正确性强绑定**：cue 带 `cause_seq`/优先级/去重，恢复期全程抑制（`main.tsx:177-187`），不会在断连时播假「成交」。这是绝大多数竞品做不到的。
2. **倒计时是全项目最亮的一块**：时钟漂移无关 + 末 10 秒切到 0.1s 精度（`domain.ts:288-312`），直击挑战二「倒计时精确到毫秒」。
3. **断连/恢复 UX 一流**：RECOVERING 去饱和、危险操作禁用、2.5s 重拉（`styles.css:733-735`、`domain.ts:564-566`、`main.tsx:1096-1103`）。

**Top 6 致命/高优问题（攻击点）**
1. **末段无「紧张感」**：临近 0 秒**只有数字格式变化**，无变色/脉冲/心跳音/震动渐强，无「一次、两次、三次」。宣讲版「紧张到窒息」基本没兑现（§4 已逐行核实）。
2. **在线人数写死 `2333`**（`components.tsx:103`），类名叫 `avatar-stack` 却没有头像 —— 这是**唯一一处「假社交证明」暗黑模式**，恰恰是真实数据（活跃出价人数）唾手可得时不该犯的错。
3. **死按钮**：`点赞`/`礼物`（`components.tsx:157-158`）、`关注`（`:101`）、主播 Talk Points（`:524-526`）**全部无 `onClick`**。评委一点就穿帮。
4. **算好却不显示的「热度」**：`price_velocity_cents_per_min`、`active_bidders_30s`、`accepted_bids_30s` 已拉取/已定义但**前端从不渲染**（§3）。信号都在手里，氛围却看不见。
5. **胜利时刻平淡**：成交 = 一个 1.8s toast + 一个写着 "SOLD" 的圆圈 + 奖杯卡，随即跳支付。无礼花/聚光/分享（§5）。
6. **零 AI**：全仓库 grep `AI|gpt|llm|智能|生成式` 无命中。差异化创新缺口最大、也是「耳目一新」的最大机会。

**Top 3 新赌注（让评委眼前一亮、且商家真的会用）**
- **AI 拍品速建（Listing Copilot）**：一张图 → 标题/描述/起拍价/加价幅度/封顶/时长建议（人审后发布）。直击商家冷启动，证据：eBay「Magical Listing」30% 卖家试用、95% 采纳 AI 描述。
- **AI 实时拍场解说/气氛官**：基于**引擎已决策的事实**生成短促解说与「系统弹幕」，随出价速度/末段升温（恰好补上被禁用的主播 hype 杠杆）。延迟/成本完全可行（TTFT 150–300ms）。
- **AI 防托/风控哨兵（Shill Sentinel）**：对出价与用户行为流做异常/合谋评分 → 主播告警/可配置自动暂停。直击评分维度「数据采集→数据治理→异常告警」，并与本项目 fail-closed 气质天然契合。

**冠军判断**：以当前状态，**能进决赛、但难拿氛围/创新的「全场最佳观感」**。补齐 §6 的 P0 + 三个 AI 头牌后，本项目具备「工程 + 体验 + 创新」三杀的冠军相 —— 因为对手很难同时拥有这套后端正确性**和**炸裂现场。

---

## 1. 评审方法与重要前提

### 1.1 四位独立评委视角（均为 10 年+ 字节/TikTok 背景）
本评审刻意从四个互相「拷打」的视角交叉审查，避免被现有实现带偏：
- **资深后端/基础架构（SRE）**：只认正确性、可观测性、failure mode。
- **资深前端**：认渲染性能、动画工艺、可访问性、状态一致性。
- **资深产品经理（PM）**：认情绪曲线、可信度、记忆点、留存与转化。
- **资深直播运营（Live-Ops/营销）**：认开播暖场、人气承接、主播杠杆、榜单社交、合规与防诱导。

每条结论标注主张它的视角；分歧点单列（§9）。

### 1.2 ⚠️ 重要前提：宣讲版评分表疑似被截断
仓库内 `抖音电商AI全栈课题-直播竞拍全栈系统（宣讲版）.md` 的「评分标准」表**只到「技术深度与创新性 25%」即结束（50% + 25% = 75%）**，缺失约 25% 的第三维度（核对：文件 130 行、7040 字节，表格止于该行）。
- **影响**：若缺失维度是「产品体验/交互设计」或「答辩展示」，则本文聚焦的「氛围体验」**不是加分项，而可能是接近 1/4 的主分**。
- **行动建议**：向主办方确认完整评分表。无论结果如何，加大氛围投入都是正期望（要么主分、要么加分）。

### 1.3 范围豁免（按用户确认）
**未做支付与真实直播流可接受**：支付为模拟（`ResultSheet → 立即支付`）、直播为 demo 循环视频（`components.tsx:47,73`）。本文不因此扣分；仅在「模拟痕迹会在 demo 暴露」处提示打磨（如固定循环视频、`Lot A-102` 写死标签 `components.tsx:138`）。

---

## 2. 逐项对照评分（功能模块 / 核心挑战 / 加分项）

> 评级：✅ 完整且优秀 · 🟡 基本达标有短板 · 🟠 明显不足 · 🔴 缺失/造假 · ⛔ 豁免

### 2.1 功能模块

| 模块 | 要求 | 现状 | 评级 | 证据/备注 |
|---|---|---|---|---|
| 商家端·竞拍发布 | 上传商品 + 配置规则（起拍/加价/时长/封顶/延时）| `ItemCreatePanel`+`RuleEditor` 齐全 | ✅ | pc-console `components.tsx` |
| 商家端·商品管理 | 状态/进度/结果、改未开拍规则、取消异常 | `AuctionQueue`/`AuctionCommandPanel`（schedule/start/cancel）齐全 | ✅ | 取消已带 fence（见既有 judge-review P0 修复）|
| 商家端·订单管理 | 成交生成订单、查看详情 | `OrdersPanel`/`OrderDetailDrawer` | ✅ | |
| 用户端·直播间 | 固定视频/开源库模拟 | demo 循环视频 | ⛔→✅ | 豁免范围 |
| 用户端·竞拍浏览 | 列表/详情/规则/当前价/参与人数/提醒 | 大体齐全；**参与人数造假** | 🟠 | `2333` 写死 `components.tsx:103` |
| 用户端·出价参与 | 手动出价、实时排名、被超越/延时/结束提醒 | 出价闭环优秀；提醒**有真有假**（被超越 cue 有竞态 bug）| 🟡 | §4.5 outbid 竞态 |
| 用户端·结果查看 | 成交、模拟支付、历史 | `ResultSheet`+`History*` 齐全 | ✅ | 胜利仪式感弱（§5）|

### 2.2 核心挑战

| 挑战 | 要点 | 现状 | 评级 |
|---|---|---|---|
| 一·复杂规则零漏洞 | 0元起拍/加价/封顶/延时10-30s/异常取消 | 引擎 Lua 原子决策 + 服务端权威，规则齐全 | ✅ |
| 二·毫秒级实时同步 | 秒级一致、倒计时精确、WS 稳定自动重连 | 倒计时漂移无关+0.1s 精度；WS 票据鉴权+退避重连 | ✅ |
| 二·细节 | 排名一致 | 排行榜**REST 重拉非推送**，无增量动画；与 cue 竞态耦合 | 🟡 |

### 2.3 加分项

| 加分项 | 子项 | 现状 | 评级 |
|---|---|---|---|
| 💫 极致竞价氛围 | 动画（领先/被超越情绪反馈）| 6 个 one-shot keyframe，正确克制，无峰值/无持续 | 🟡 |
| | 实时排行榜（位置与差距）| 数据丰富（gap/next-valid/state），但 REST 重拉、无榜一榜二社交、`bid_count` 不显示 | 🟡 |
| | 紧张感营造（倒计时动画、提示音）| **末段无升温**；音为单振荡器 beep 且默认关闭 | 🟠 |
| ⚡ 硬核并发 | Redis 分层/读写分离 | 已实现（见 architecture.md）| ✅ |
| | 分布式锁/出价幂等「不扣两次钱」| request-hash 幂等 + 决策时依据 | ✅ |
| | WS 房间级隔离 + 1000+ 在线 | 已达标（PTS-1B）| ✅ |
| 🤖 AI（评分维度「开源模型调用·可选」明确提到）| 任意 AI | **完全缺失** | 🔴 |

**小结**：硬核并发与工程链路是满分气质；**「极致氛围」与「AI 创新」是失分/留分的两个洞**，且都极其「demo 可见」。

---

## 3. 「算好却看不见」——被浪费的氛围信号（独立代码审计已核实）

后端/类型层已经把「热度」算好并下发，**前端却从不渲染**，这是「信号都在手里、氛围却没拍出来」的最可惜之处：

| 字段 | 定义处 | 使用情况 |
|---|---|---|
| `price_velocity_cents_per_min` | `domain.ts:252` | **定义后零引用，从不渲染** |
| `active_bidders_30s` | `domain.ts:250` | H5 **从不渲染**（仅 PC 侧自有同名聚合）|
| `accepted_bids_30s` | `domain.ts:251` | 仅在某一分支的 freshness 文案出现一次（`domain.ts:388`）|
| `entries[].bid_count` | `domain.ts:233` | **榜单行从不渲染**（只渲染 rank/名/金额 `components.tsx:778-784`）|

> 结论：把这 4 个字段变成一个「竞价热度计 / 速度火花条 / 榜一连击数」，几乎**零后端成本**就能把「数百人狂点」的能量第一次画出来。这是 ROI 最高的氛围改造（§6 · P0-D）。

---

## 4. 「极致竞价氛围」现状深度审查（逐项·带证据）

> 以下为我方阅读 + 独立后台审计 agent 交叉核实的结论，均带 `file:line`。

### 4.1 情绪反馈 cue —— 正确但「是通知，不是氛围」
- 实现：单条 toast（`components.tsx:81-96`）+ `data-atmosphere-kind` 局部特效；kinds/优先级 `atmosphere.ts:1,26-33`；事件驱动（`main.tsx:642-913`）；去重/抑制/恢复期屏蔽（`main.tsx:177-187,228-235`）。
- 评价（前端+PM）：**工程正确性优秀**，但每个离散事件只换来 1 个 ≤1.8s 小 toast + 1 个 CSS flourish，**没有持续/升级的「场」**。快速连拍时，用户看到的是一串「弹出又消失」的通知，而非「越来越燃」的氛围。

### 4.2 倒计时 —— 全项目最佳，但只服务「正确」不服务「情绪」
- 100ms tick（`main.tsx:158-161`）；末 10s 切 0.1s（`domain.ts:288-297`）；漂移无关锚点（`main.tsx:131-136`、`domain.ts:299-312`）；本地归零触发重同步而非宣布赢家（`main.tsx:794-798`）。
- 攻击点（PM）：末段 UX **刻意降级**（显示「到点同步中/本地到零，正在同步」`domain.ts:310`、禁用 CTA `main.tsx:560-563`）。从正确性看完美，从情绪看**与「高潮」完全相反**。

### 4.3 声音/震动 —— 能用，不「极致」
- `playCueTone` 单正弦 beep ~0.18s（`domain.ts:421-442`）；默认关闭、需点铃铛开（`main.tsx:196-219`）；震动分级（`domain.ts:408-419`）。
- 攻击点：无背景张力音床、无 tempo 渐快、无落锤/金币/whoosh 真实音效。落锤声**根本不存在**。

### 4.4 动画 —— 6 个 keyframe，工艺干净但「峰值」缺席
`cue-slide`/`leading-ring`/`outbid-edge`/`hammer-mark`(写死文本 "SOLD")/`price-tick`/`countdown-stretch`（`styles.css:550-973`）。全部 transform/opacity（性能正确）、全部 one-shot、**无一个 looping 的「持续紧张」态**。

### 4.5 ⚠️ 末段紧张感 —— **基本为 0（逐行核实）**
- 临近 0 秒**唯一变化是数字精度**（0.1s）；**无变色、无脉冲、无屏闪、无 tempo、无震动渐强**；`styles.css` 没有任何按「剩余秒数」触发的规则。
- **没有「一次、两次、三次 / going once」**（grep 确认缺失）。
- `hammer-mark` 是**成交后**的 "SOLD" 印章，不是末段升温。
> 这是与宣讲版「价格每秒都在跳动，气氛紧张到窒息」差距最大的一条。

### 4.6 胜利时刻 —— 平淡（PM 视角最痛）
成交三件事：sold toast（1.8s，`main.tsx:663-671`）+ "SOLD" 圆圈（`styles.css:480-500`）+ `ResultSheet` 奖杯卡（`components.tsx:411-462`）→ **立即转支付漏斗**。grep `confetti|spotlight|celebrat|share` 无命中。**赢得稀世珍宝**的高光，被处理成了「结账」。

### 4.7 presence/社交能量 —— 基本是布景
- 在线数写死 `2333`（`components.tsx:103`）；无真实观众订阅/进退场。
- 无「XX 正在出价」feed；无飘心/无入场特效；`点赞`/`礼物` 死按钮（`components.tsx:157-158`）。
- 弹幕 5s 轮询（`main.tsx:984-1003`）、静态列表行、无飘屏动画。

### 4.8 可访问性/性能 —— 可访问性好，性能有 10Hz 隐患
- `prefers-reduced-motion` 全套关闭动画（`styles.css:975-997`）；ARIA live 分级到位（`components.tsx:84-85,298,320`）。动画只用合成层属性。
- **隐患**：100ms `setInterval` 使**整个 `App` 每秒重渲染 10 次**；`scenario` useMemo 依赖 `countdownCopy`（`main.tsx:565`）→ 每 tick 重建大对象；子组件**无 `React.memo`** → 10Hz 全树 reconcile。当前无感，但叠加氛围 DOM + 1000 并发时是 jank 风险源。

### 4.9 主播端（商家/Live-Ops）—— 运维强、**hype 杠杆为 0**
- `LiveAssistRail`：Prompter（**规则/指标派生，非生成式 AI**）、Talk Points（**死按钮**）、Demo 驱动（competing-bid，是**测试工具**）、Heat 30s（只读）、**系统弹幕被禁用**（`本场不启用主播一键系统弹幕`）。
- `AuctionCommandPanel`：schedule/start/cancel/讲解（is_narrating 只是状态位）。
- 结论（Live-Ops）：**主播没有任何「能让买家看到」的造氛围按钮**。这与抖音直播「主播一句话点燃全场」的现实完全相反。

### 4.10 氛围层的具体 Bug（后端级严谨，前端却漏了）
1. **被超越 cue 竞态**：仅当 `previousLeading`（`my_rank===1`，来自**上一次 REST 拉取**）才触发（`main.tsx:886-897,818,915`）。两次被超越之间若榜单未刷新 → **「被超越！」静默丢失**。讽刺地与后端的事件权威性背道而驰。
2. **自我领先 cue 依赖事件字段顺序**：`else if` 短路（`main.tsx:886`）使携带他人 `user_id` 的事件抢先，正确 cue 取决于服务端填 `current_winner_id` 还是 `user_id`（`main.tsx:902-913`）。
3. **cue id 碰撞**：`id = Date.now()`（`atmosphere.ts:42`），同毫秒两 cue id 相等，dismiss 清理可能清错 `activeCueRef`（`main.tsx:233`）。dedup key 稳健但 id 不稳健。
4. **死按钮族**：`点赞/礼物/关注/Talk Points` 无 handler —— demo 现场一点即穿帮。

---

## 5. 行业标杆对照（这是评委心里的「锚」）

字节/TikTok 评委对这条赛道极熟，他们会拿下面这些**真实在跑**的产品当尺子。

### 5.1 TikTok Shop「Countdown Bidding」——评委的直接参照
- TikTok Shop 已上线直播竞拍：限时倒计时制造紧迫感（来源：TikTok Seller University；zorillamarketing 指南）。
- **延时机制与本项目一致**：官方原文「any last-second bid will reset your countdown timer to the extended time you pre-select」，可开关 —— 证明本项目「自动延时」是行业标准做法（不是过度设计）。
- **「Surprise Set」惊喜盲盒**：奖池 + 数量追踪 + **旋转奖盘「dramatically stops」揭晓** + 剩余奖品可见「building anticipation」+ **强制披露最低价值**（max bid $5000 / 500 件）。这就是教科书级「揭晓仪式 + 期待感 + 合规披露」。
> 启示：本项目缺的正是 TikTok 已验证的两件事 —— **揭晓仪式（胜利时刻）** 与 **末段紧迫升温**。

### 5.2 Whatnot（直播拍卖独角兽，估值 $11.5B，2025 GMV ~$6B）
- 秒级一拍、breaks/盲盒、giveaway、tipping、**低延迟弹幕是其引以为傲的技术点**（来源：fortune；boardroom 访谈）。
- **明确禁止并监控 shill bidding（刷托抬价）**（来源：Whatnot Community Guidelines）→ 印证 §6 的 AI 风控价值。
- Guardian 报道其「gambling-style、易上瘾」争议 → 印证 §9 的合规/防沉迷必要性。

### 5.3 抖音原生玩法（评委的「母语」）
- **福袋/超级福袋**：挂车式倒计时抽奖，实时显示**参与人数 + 开奖倒计时**，任务（看播/口令/评论/加粉丝团）解锁资格，用于**承接流量、稳住在线、拉互动**（来源：巨量引擎博客；抖店「超级福袋」教程）。
- **红包/红包雨**：驱动点赞评论、点亮粉丝灯牌、打造氛围（抖音 2021 春晚 12 亿红包、703 亿次互动、峰值 498 万人同看）。
- **PK 连麦**：**实时进度条 + PK 倒计时**，官方产品方法论原文即「营造激烈的 PK 氛围…增加紧张感和刺激性」（来源：人人都是产品经理）。
- **榜单（榜一/榜二/榜三）**：社交地位是中文直播互动的核心。本项目榜单是「脱敏数字」，缺地位感。
> 启示：本项目可借鉴的不是「打赏」，而是**进度条式的双方对抗张力**、**榜一社交地位**、**开播暖场抽拍**。

### 5.4 紧张感的「真」与「假」——评委会区分（也是差异化）
- 心理学：损失厌恶 ≈ 2.5×（「losing feels twice as bad」）；稀缺+紧迫驱动冲动，但**长期带来后悔、信任下降**（来源：maccelerator；pioneerpublisher；PMC 直播冲动购买）。
- 暗黑模式：Princeton 研究——**温和暗黑模式 ~2×、激进 ~4× 非计划购买**；**假倒计时、假社交证明**是典型暗黑模式（来源：acowebs；marketplace.org）。
- **本项目的 `2333` 正是「假社交证明」**——而真实数据（活跃出价人数）就在手里。
> **冠军级洞察**：竞拍的紧张感**可以是「真」的**（真有一个赢家、真时钟、真对手、真差价）。把「真张力」拍到极致、并**显式设计反暗黑（无假稀缺、防诱导、未成年保护、防托）**，对字节评委（重治理/合规）就是**「有底线的极致氛围」**的差异化叙事。

---

## 6. 设计方案 A：竞价氛围 2.0（非 AI，优先级排序）

> 实现总原则（性能，来源：MDN/web.dev/Algolia/Flipboard）：**只动 `transform`/`opacity`**（合成层，绕过 layout/paint）；**粒子用 canvas + `requestAnimationFrame` + delta-time**（自动随隐藏标签暂停）；`will-change` 节制；**所有效果受 `prefers-reduced-motion` 与「恢复期/stale 不升温」双门控**。

### P0 —— 修可信度 + 立核心张力（成本低、demo 收益最高）

**P0-A 末段紧迫升温系统（直击「紧张到窒息」）**
- 末 10s/5s/3s 三档升级：背景渐变向警示红、价格区与倒计时**脉冲缩放**（transform）、**心跳音床 tempo 渐快**（WebAudio，可关）、**震动渐强**；末 3s 出「三 · 二 · 一」节拍 + 「going once / 第二次 / 第三次」文案。
- 门控：`RECOVERING/stale/disconnected` 时**不升温**（复用 `recoveringRef`）；尊重 reduced-motion（降级为纯文字精度，已有）。
- 价值：把「正确但克制」的末段，变成全场最炸的 5 秒。

**P0-B 用真实在线/出价人数替换 `2333`（修暗黑模式 + 可信度）**
- 订阅真实 watcher（PC 侧已可算 `watcher_count`）；若暂不可得，**用真实活跃出价人数派生，绝不写死**。配真头像栈或首字母占位（让 `avatar-stack` 名副其实）。

**P0-C 激活/移除死按钮（修穿帮）**
- `点赞` → canvas 飘心（transform-only、粒子上限、reduced-motion 关）；`礼物`/`关注` 要么接真行为要么删除；主播 Talk Points 接「插入解说/系统弹幕」。

**P0-D 竞价热度计（把 §3 浪费的信号画出来）**
- 用 `price_velocity_cents_per_min` + `active_bidders_30s` + `accepted_bids_30s` 渲染一个**实时热度条/速度火花**（如「🔥 近 30s 27 次出价 · ¥1,240/分」），每笔新出价一个 transform 粒子上跳。**几乎零后端成本**。

### P1 —— 峰值仪式 + 榜单社交（记忆点）

**P1-E 胜利仪式（对标 TikTok Surprise Set 揭晓）**
- 礼花（canvas/rAF、粒子封顶、reduced-motion 关）+ 赢家聚光 + **落锤音** + **可分享「战绩卡」**（拍品图/成交价/击败人数/价格曲线）**先于**支付出现；惜败者给「差一口」温柔收尾 + 下一拍钩子。

**P1-F 排行榜 2.0（榜一榜二榜三 + 反超戏剧）**
- 显示 `bid_count`、榜一/榜二/榜三地位标签、**FLIP 排名变化动画**（transform-only）、「你正被追赶 / 差一口反超」；**改用 WS 推送增量**替代 REST 重拉 —— 顺手修掉 §4.10-1 的 outbid 竞态。

**P1-G 声音设计 2.0**：分层（张力音床 + 事件音 + 落锤/金币/whoosh），可仍为合成或极小采样，加 ducking；与 reduced-motion 解耦但提供独立静音。

**P1-H 弹幕动效 + 系统弹幕通道**：真飘屏；**重新启用主播系统弹幕**（当前被禁用）作为 AI 解说的出口（接 §7-B）。

### P2 —— 抖音原生留存玩法（暖场/对抗/地位）

**P2-I 开播暖场「0 元抽拍 / 福袋」**：正式热拍前用一个 0 元起拍小件或福袋承接流量、聚人气（抖音验证的承接手法）。
**P2-J 「买家阵营 PK 进度条」**：把抖音 PK 的实时进度条张力迁移到「两件拍品/两个阵营」对抗，复用倒计时。
**P2-K 入场/榜一特效**（土豪进场）—— **必须配合防托/未成年门控**（见 §9）。

### P0' —— 顺手修的氛围层 Bug
- outbid cue 改**事件驱动**（不依赖 REST 新鲜度）；cue `id` 改**单调计数器**；自我领先 cue 明确 `current_winner_id` 优先级；10Hz 重渲染 → 把 tick 下沉到**叶子组件** + `React.memo` 或 rAF 节流。

---

## 7. 设计方案 B：AI 亮点（「耳目一新」且商家真的会用）

> 设计铁律（与本项目非协商项一致）：**AI 只做「叙述者/助手/哨兵」，绝不做「裁判」**。出价是否成功、谁赢、价格，永远由引擎决定；AI 的每句话都必须**锚定 `engine_seq`/`decision_basis` 等已发生的事实**，不得编造。多模态/文本均可（用户已提供 AI API）。

### 🥇 B1. AI 拍品速建 · Listing Copilot（商家端，最高 ROI）
- **做什么**：商家拍/传 1–N 张图 + 一句话 → 多模态模型产出**结构化草稿**：标题、卖点描述、类目、**建议起拍价/加价幅度/封顶价/时长/延时**、（珠宝/古玩/二奢）真伪与瑕疵提示、合规风险词。**人审后一键填入 `RuleEditor` 发布**，服务端再校验。
- **为何有人用**：直击宣讲版「高价值非标品难定价」的核心痛点，也直击商家冷启动。证据：eBay「Magical Listing」拍照即生成标题/描述/类目/建议价，**~30% 美区 App 卖家用过、>95% 采纳 AI 描述**；Poshmark/3Dsellers 同类已成默认入口。
- **链路对齐**：完美命中评分维度「数据采集 → 开源模型调用（可选）→ 后端服务（校验）→ 前端」。
- **可行性**：发布前**离线单次**多模态调用，无热路径风险；结果可缓存；人审兜底（卖家社区反馈：低质模型会编细节，故必须 human-in-loop）。
- **风险**：估价不可当承诺（标注「建议，非保证」）；珠宝真伪不可由 AI 背书（只提示「需第三方鉴定」）。

### 🥇 B2. AI 实时拍场解说 / 气氛官（用户端，最高「氛围 wow」）
- **做什么**：一个 AI「副播/解说」，在**引擎已决策的关键事件**（新榜首、大幅跳价、末窗延时、逼近封顶、三二一）上生成**短促有梗**的解说，作为**系统弹幕**展示，可选 TTS 语音；**激烈度随 §3 的出价速度/末段升温**（对标 ICML 2025「Excitement-Driven Commentary」：从现场反应自适应解说激烈度）。
- **为何有人用**：**补上 §4.9「主播零 hype 杠杆」的最大空洞**；中小商家/无口才主播也能有「全程高能解说」。证据：抖音/淘宝数字人主播规模化（百度「数字人罗永浩」6 小时 1300 万观看、**¥55M GMV**；京东「数字人刘强东」首小时 2000 万观看、¥50M）；Alibaba 现场实验：AI 直播助手**销量 +3%、退货 -12.55%**。
- **可行性/成本**：事件稀疏（每笔/每次延时一条），文本即可；**TTFT 150–300ms、e2e <800ms**（来源：Retell AI），成本可忽略（短输出 + 模板缓存 + 仅关键事件触发）；硬延迟选 fast tier（Haiku 4.5 等）。
- **安全边界**：每句锚定 `engine_seq` 与真实价格，**不得制造假稀缺/不得对疑似未成年施压**；与 P1-H 系统弹幕通道复用。

### 🥇 B3. AI 防托 / 风控哨兵 · Shill Sentinel（平台/主播端，最高「治理可信度」）
- **做什么**：对**出价流 + 用户行为**做实时异常/合谋评分（自我抬价图、节奏异常、设备/IP 聚集、撤退模式）→ 主播告警 + **可配置自动暂停/对账**（接 `异常取消`/`RECONCILING`）。
- **为何加分**：直击评分维度「数据采集（出价、用户行为）→ 数据治理 → 异常告警」，并与本项目 **fail-closed/对账** 气质天然契合；Whatnot 明确禁止/监控 shill，学术上集成模型 ~97% 可达（来源：IJETRM；Hawaii 实时检测）。
- **可行性**：基于聚合特征的轻量模型或「LLM-as-judge on aggregates」，**旁路/异步**、不进热路径；告警先建议、再人工/规则确认（hybrid，rules + ML）。

### 🥈 B4. AI 复盘 / 高光集锦（商家留存 + 病毒传播）
- 每拍结束自动生成：商家复盘（价格发现、需求强弱、可再营销人群、下一拍建议起拍价）+ **可分享「高光时刻」卡/短片**（成交瞬间 + 价格曲线）。批处理、无延迟压力。对标行业「自动赛后高光生成」（dataintelo/firework）。

### 🥉 B5. AI 导拍客服（用户端，降主播负载）
- 房内回答「成色/证书/能不能砍价/规则」，基于拍品结构化数据（Alibaba 助手证据：+3% 销量、-12.55% 退货）。命中宣讲版「查看规则/接收提醒」。

**AI 头牌推荐（答辩主打）**：**B1 + B2 + B3**——分别对应**商家价值、现场氛围、平台治理**三条评委最认的线，且都有真实业务证据与可行性背书。

---

## 8. 优先级路线图（按「答辩收益 / 工作量」）

| 优先 | 项目 | 评委收益 | 工作量 | 备注 |
|---|---|---|---|---|
| P0 | 末段升温 P0-A | 极高（直击核心挑战）| 中 | 纯前端 + WebAudio |
| P0 | 真在线数 P0-B / 死按钮 P0-C | 高（去暗黑/防穿帮）| 低 | 可信度生死线 |
| P0 | 热度计 P0-D | 高（零后端成本画能量）| 低 | 信号已就绪 |
| P0 | **AI Listing Copilot B1** | 极高（商家会用 + 链路全）| 中 | 单次多模态调用 |
| P1 | 胜利仪式 P1-E | 高（记忆点）| 中 | canvas 礼花 + 战绩卡 |
| P1 | 榜单 2.0 / WS 推送 P1-F | 高（顺修竞态）| 中 | 一并修 outbid bug |
| P1 | **AI 实时解说 B2** | 极高（氛围 + 补主播杠杆）| 中 | 文本先行，TTS 可后置 |
| P1 | 声音 2.0 / 弹幕动效 P1-G/H | 中高 | 中 | 系统弹幕通道复用 |
| P2 | **AI 风控哨兵 B3** | 高（治理叙事）| 中高 | 旁路异步 |
| P2 | 福袋暖场/PK 进度条 P2-I/J | 中（留存）| 中高 | 抖音原生 |
| P2 | AI 复盘/高光 B4、导拍 B5 | 中 | 中 | 留存/病毒/降负载 |

**最小冠军包（若时间紧）**：P0-A + P0-B + P0-C + P0-D + B1 + B2 = 「真张力 + 真社交 + 零穿帮 + 会用的 AI + 炸场解说」。

---

## 9. 风险与反方意见（评委会怎么反打）

**9.1 合规/伦理（字节评委必问，且是差异化）**
- 紧张感不能滑向暗黑模式：**禁止假倒计时、假稀缺、假社交证明（含 `2333`）**；末段升温要克制（避免诱导冲动，PK 乱象/未成年沉迷/诈骗有大量负面判例与《网络直播服务管理规定》压力）。
- 主动设计**「有底线的极致」**：消费提示/冷静期诚实、无虚假人气、**防托（接 B3）**、**未成年保护门控**（入场特效/打赏类玩法尤需）。把它写进答辩——这是把「氛围」从风险变成**成熟度加分**。

**9.2 过度工程风险（后端 SRE 视角自省）**
- AI 解说/风控**绝不能进热路径**、不得成为新的故障源；必须可一键降级为纯引擎流（沿用本项目 fail-closed 习惯）。
- 礼花/粒子要有上限与降级；10Hz 重渲染先治理再加 DOM（§4.8）。

**9.3 Demo 失败模式（PM/运营视角）**
- TTS/外网 AI 在答辩网络下可能抖动 → 备**本地缓存/模板兜底**与「纯文本解说」降级；演示脚本固定一条「必燃」末段对决。
- 死按钮/写死数字若不修，**一次点击即毁可信度**——P0-C/B 必须先做。

**9.4 反方最强论点**
- 「后端已经够强，氛围是锦上添花」——**反驳**：宣讲版把「极致氛围」单列为加分项、把「氛围动画/实时反馈」写进 50% 主维度的链路闭环、且评分表疑似还有约 25% 体验维度被截断（§1.2）。**在答辩台上，评委先被「感受」打动，再被架构说服。** 当前最强的后端配最弱的氛围，是性价比最低的失分结构。

---

## 10. 完整优化清单（可逐条勾选）

**A. 可信度/穿帮（P0，先做）**
- [ ] 替换写死在线数 `2333` → 真实/派生（`components.tsx:103`）
- [ ] `点赞` 接飘心 / `礼物`·`关注` 接行为或删除（`components.tsx:157-158,101`）
- [ ] 主播 Talk Points 接「插入解说/系统弹幕」（`components.tsx:524-526`）
- [ ] 清理写死标签 `Lot A-102`、`古玩榜第 8 名`、`保证金锁定` 无条件文案（`components.tsx:118-119,138,50-55`）

**B. 核心张力（P0/P1）**
- [ ] 末段 10/5/3s 升温（变色+脉冲+心跳音+震动+「三二一/going once」）
- [ ] 竞价热度计（velocity/active_bidders_30s/accepted_bids_30s 可视化）
- [ ] 胜利仪式（礼花 + 聚光 + 落锤音 + 可分享战绩卡，先于支付）
- [ ] 惜败温柔收尾 + 下一拍钩子

**C. 排行榜/实时（P1）**
- [ ] 榜单显示 `bid_count` + 榜一/榜二/榜三地位 + FLIP 名次动画
- [ ] 排行榜/弹幕改 **WS 推送增量**（替代 REST/5s 轮询）
- [ ] 修 outbid cue 竞态（事件驱动）、cue id 单调化、自我领先字段优先级

**D. 声音/动效/性能（P1）**
- [ ] 分层音效 + 张力音床 + ducking + 独立静音
- [ ] 弹幕真飘屏 + 重启用主播系统弹幕通道
- [ ] 10Hz tick 下沉叶子组件 + `React.memo`/rAF 节流

**E. AI 亮点（P0–P2）**
- [ ] B1 AI 拍品速建（图→标题/描述/起拍/加价/封顶/时长，人审发布）
- [ ] B2 AI 实时解说/气氛官（事件驱动、随热度升温、系统弹幕/TTS）
- [ ] B3 AI 防托/风控哨兵（旁路异常评分 → 告警/可配置暂停）
- [ ] B4 AI 复盘/高光（赛后）· B5 AI 导拍客服（房内问答）

**F. 抖音原生留存（P2）**
- [ ] 0 元抽拍/福袋暖场 · 买家阵营 PK 进度条 · 入场/榜一特效（带防托/未成年门控）

**G. 合规/伦理（贯穿）**
- [ ] 反暗黑：无假稀缺/假人气/假倒计时
- [ ] 防诱导冷静设计 + 未成年保护门控 + 消费提示诚实
- [ ] AI 全程「叙述者非裁判」、可一键降级、不进热路径

---

## 11. 参考来源（均经 Tavily/Fetch 核实）

**直播竞拍/氛围标杆**
- TikTok Shop Countdown Bidding（含 Surprise Set 揭晓、延时机制）: https://seller-us.tiktok.com/university/essay?knowledge_id=8427133325330222&lang=en ; https://www.zorillamarketing.com/articles/tiktok-shop-live-auctions-guide-for-sellers
- Whatnot 规模/特性: https://fortune.com/2025/06/16/whatnot-startup-5-billion-dollar-livestream-video-shopping-app-auctions-sports-trade-card-breaks-ebay ; 上瘾争议: https://www.theguardian.com/business/2025/dec/22/toy-touts-random-spins-and-frantic-bidding-the-murky-side-of-live-auction-site-whatnot ; 反 shill: https://help.whatnot.com/hc/en-us/articles/360061197472-Whatnot-Community-Guidelines
- 反狙击/自动延时: https://en.wikipedia.org/wiki/Auction_sniping
- 抖音福袋/红包: https://www.oceanengine.com/blog/fudai-jiwanyuan-liuliang.html ; 超级福袋: https://school.jinritemai.com/doudian/web/article/aHYhaQwYSpxX
- 抖音 PK 氛围方法论: https://www.woshipm.com/it/4240503.html ; PK 乱象/合规: http://zgjssw.jschina.com.cn/yaowen//202011/t20201110_6867437.shtml

**AI 在直播电商（业务证据）**
- 数字人主播（罗永浩 ¥55M、刘强东 ¥50M、¥270B by 2030）: https://www.wicinternet.org/2025-08/06/c_1114036.htm
- 直播电商市场/AI 协同主播（Alibaba ~$250B GMV）: https://dataintelo.com/report/live-commerce-platform-market ; https://anymindgroup.com/blog/ai-live-commerce-2026
- AI 直播助手现场实验（+3% 销量 / -12.55% 退货）: https://news.miami.edu/miamiherbert/stories/2025/08/ai-assistant-for-online-shopping.html
- 虚拟人销售（Brother +30%）: http://www.bevycommerce.com/insights/virtual-human-salespeople-in-live-commerce-the-future-of-selling-is-already-here

**AI 拍品速建 / 多模态电商**
- eBay Magical Listing（拍照即生成，30% 试用 / 95% 采纳）: https://innovation.ebayinc.com/stories/magical-listing-tool-harnesses-the-power-of-ai-to-make-selling-on-ebay-faster-easier-and-more-accurate ; https://www.3dsellers.com/ebay-ai
- Shopify 多模态商品目录: https://shopify.engineering/leveraging-multimodal-llms

**AI 解说 / 风控 / 实时性**
- 激烈度自适应解说（ICML 2025）: https://icml.cc/Expo/Conferences/2025/Expo
- shill 检测 ML（~97.72%）: https://ijetrm.com/issues/files/Jul-2025-24-1753378619-JULY46.pdf ; 实时检测: https://scholarspace.manoa.hawaii.edu/items/93ee2cd8-4bc6-49ab-a858-17da53b56699
- 实时语音 AI 延迟/成本（TTFT 150–300ms、<800ms、$0.28/hr）: https://www.retellai.com/blog/how-real-time-voice-ai-works-stt-llm-tts ; https://www.retellai.com/blog/best-llm-for-voice-ai-agents

**心理学 / 暗黑模式 / 性能**
- 损失厌恶/稀缺: https://maccelerator.la/en/blog/entrepreneurship/behavioral-psychology-behind-scarcity ; https://www.pioneerpublisher.com/jwe/article/view/1095 ; 直播冲动购买: https://pmc.ncbi.nlm.nih.gov/articles/PMC10979273
- 暗黑模式（Princeton 2×/4×、假倒计时/假社证）: https://acowebs.com/dark-patterns-ecommerce ; https://www.marketplace.org/story/2023/01/16/dark-patterns-web-design-consumer-awareness
- 动画性能（仅 transform/opacity、rAF）: https://developer.mozilla.org/en-US/docs/Web/Performance/Guides/CSS_JavaScript_animation_performance ; https://www.algolia.com/blog/engineering/60-fps-performant-web-animations-for-optimal-ux ; https://about.flipboard.com/engineering/60-fps-on-the-mobile-web

---

> 维护：本文为 2026-06-05 产品/体验/创新维度评审；代码 `file:line` 对应当日工作树。氛围层若改为 WS 推送/新增组件，请回链更新 §3/§4/§6 的证据点。
