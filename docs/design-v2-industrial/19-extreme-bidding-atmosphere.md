# 19 · Extreme Bidding Atmosphere

## 定位

“极致竞价氛围体验”不是 UI 装饰，也不是把“领先/被超越”几个文字状态贴到页面上。它应该是一套服务于直播竞拍转化、停留、信任和主播控场的工业级体验系统：

```text
可信竞拍真源 -> 实时事件编排 -> 情绪反馈 -> 主播控场 -> 商家运营 -> 诊断复盘
```

评委追问时，核心回答不是“我们也有排行榜和动画”，而是：

1. 每个氛围反馈都绑定服务端权威事件，不伪造出价、不提前宣告成功。
2. 紧张感来自真实竞争、时间压力、稀缺性和观众在场感，而不是虚假倒计时或假热度。
3. 用户、主播、商家、平台风控和工程稳定性都有明确收益和约束。
4. 每个效果都有降级、可访问性、性能预算、测试证据和诊断路径。

## 调研依据

| 标杆/研究 | 可借鉴点 | 对本项目的启发 |
|---|---|---|
| Whatnot live auctions | 实时倒计时、短拍卖、Swipe to bid、Custom/Max Bid、Pre-bid、Outbid 后继续出价、成交后自动支付；卖家侧可设置起拍价、竞拍时间、最后 10 秒 counter bid reset、Sudden death，并实时看到 bid count/current winner。 | 直播竞拍要把“低摩擦出价 + 高速反馈 + 主播控场 + 严肃履约”组合起来；只做按钮和价格刷新不够。 |
| eBay auctions / eBay Live | 自动出价按 increment 保持领先、被超越通知、部分品类 extended bidding、eBay Live 即时自动支付、出价是 binding contract、错误出价撤回和卖家取消都有严格条件。 | 极致氛围必须配套承诺感、错误保护、自动加价/代理出价、反操纵规则；否则刺激感会损害信任。 |
| TikTok Shop LIVE Shopping Ads | 目标是让用户发现并观看 LIVE，同时在直播中浏览和购买商品；指标包含 LIVE views、10-second LIVE views、LIVE product clicks、checkout 等。 | 竞价氛围要面向短视频流转化漏斗设计：停下来、看满 10 秒、点商品、出价、支付，而不是只服务已在房间里的用户。 |
| Amazon Live / Amazon IVS | 直播中商品 carousel、实时 chat、主播回答问题、商品和视频通过 timed metadata 对齐；creator app 支持添加商品、开播、内置 analytics、creator level。 | 主播端需要“当前该讲什么/该提醒谁/该推哪个商品”的控场工具；商品卡和事件要与直播节奏同步。 |
| 竞拍心理学：competitive arousal / auction fever | 时间压力、竞争对手、观众在场感会提高唤醒水平；零售竞拍研究显示 time pressure + social competition 可提高 arousal 和 bid，且“赢的喜悦”强于“输的挫败”。 | 氛围设计要有节制地放大真实竞争和临门一脚，不用假数据；同时要防误触、防过度刺激、防沉迷和纠纷。 |
| Web 平台约束 | 音频自动播放通常需要用户交互；Vibration API 不是所有浏览器都支持；WCAG 要求闪烁不能超过安全阈值；低动效偏好要被尊重。 | 提示音/震动必须默认关闭、主动授权、可降级；动画不能阻塞 CTA，也不能用高频闪烁制造刺激。 |

参考链接：

- Whatnot bidding: https://help.whatnot.com/hc/en-us/articles/14932924544141-How-to-bid-in-an-auction
- Whatnot seller auction controls: https://help.whatnot.com/hc/en-us/articles/9779931101837-Start-an-auction-during-a-show
- Whatnot pre-bid/max bid: https://help.whatnot.com/hc/en-us/articles/14933026908301-Bidding-on-an-Item-Before-It-Goes-Up-For-Auction
- Whatnot verified buyers: https://help.whatnot.com/hc/en-us/articles/4968064385933-Require-Verified-Buyers-during-your-show
- eBay automatic bidding: https://www.ebay.com/help/selling/fees-credits-invoices/selling-fees?id=4014
- eBay bidding overview / extended bidding / eBay Live payment: https://www.ebay.com/help/bidding/buying/bidding/buying?id=4003
- eBay bid retraction: https://www.ebay.com/help/buying/bidding/retracting-bid?id=4013
- eBay seller bid cancellation: https://www.ebay.com/help/selling/listings/listing-items/troubleshooting-listing-issues?id=4140
- TikTok LIVE Shopping Ads: https://ads.tiktok.com/help/article/getting-started-live-shopping-ads?lang=en
- TikTok LIVE Shopping Ads metrics: https://ads.tiktok.com/help/article/key-reporting-metrics-for-live-shopping-ads?lang=en
- Amazon Live / IVS timed metadata: https://aws.amazon.com/blogs/media/prmbp-how-amazon-live-is-creating-interactive-shoppable-livestreams-amazon-ivs/
- Amazon Live dynamic carousel: https://advertising.amazon.com/solutions/products/amazon-live
- Competitive arousal, HBS: https://www.hbs.edu/faculty/Pages/item.aspx?research=7290
- Auction fever study: https://www.sciencedirect.com/science/article/pii/S0022435915000123
- Web autoplay constraints: https://developer.mozilla.org/en-US/docs/Web/Media/Guides/Autoplay
- Vibration API constraints: https://developer.mozilla.org/en-US/docs/Web/API/Vibration_API
- WCAG flashes threshold: https://www.w3.org/WAI/WCAG20/Understanding/three-flashes-or-below-threshold

## 北极星指标

竞价氛围体验不能只用“动画好不好看”评价。它至少要覆盖四类指标。

| 目标 | 指标 | 为什么重要 |
|---|---|---|
| 停留 | feed 进入后 3s/10s 留存、直播间停留时长、商品卡展开率 | TikTok 类短视频场景里，先要让用户停下来。 |
| 参与 | 有效出价人数、首次出价转化率、被超越后复出价率、最后 10 秒活跃出价率 | 竞价氛围的核心是把围观者推向真实参与。 |
| 成交 | 成交率、成交价/起拍价倍率、支付完成率、订单取消率 | 氛围不能只抬价不履约。 |
| 信任 | reject 解释命中率、误触确认拦截率、纠纷/取消率、恢复后继续出价率 | 高压竞价必须让用户相信系统公平且可恢复。 |

## 设计原则

### 1. 真竞争，不假热度

允许展示：

- 真实 accepted bid。
- 真实 Top N 和我的排名。
- 真实剩余时间、延时、成交。
- 真实围观/参与趋势，且要标注“近 30 秒出价人数”等口径。

禁止展示：

- 假出价、假在线人数、假库存紧张。
- 前端本地提前宣布成交。
- “即将结束”但服务端没有相应状态或规则支撑。
- 为了刺激而隐藏运费、保证金、封顶价、错误出价后果。

### 2. 情绪反馈要分层

不是每个事件都全屏炸裂。按影响程度分层：

| 层级 | 事件 | 反馈 | 时长 |
|---|---|---|---|
| L1 信息 | 用户加入、普通聊天、排行榜刷新 | 小 toast / 弹幕 | 0.8-1.2s |
| L2 竞价 | bid_accepted、被超越、排名变化 | price tick、状态 chip、短音效/轻震 | 1.2-1.8s |
| L3 时间压力 | 最后 10 秒、延时、连续加价 | 倒计时 pulse、节奏音、主播提示卡 | 2-3s |
| L4 终态 | SOLD/ENDED/CANCELLED | 落锤/结果动效、订单 CTA | 2-4s |
| L5 恢复 | 断线、gap、stale snapshot | 降噪 UI、禁用危险 CTA、恢复进度 | 持续到恢复 |

### 3. 刺激感要有护栏

极致不是无限刺激。高价值商品需要更强信任：

- 大额跳价必须 fat-finger confirm。
- Max Bid/代理出价要私密，不能泄露用户心理价。
- 被超越反馈要鼓励“理性提高预算”，不能羞辱用户。
- 赢家支付要顺滑，输家要有体面退场和相似商品承接。
- 主播/商家取消必须可解释、可审计、不可随意毁约。

### 4. 氛围体验是事件编排系统

前端不应该写散落的 `if event_type then animation`。应抽象为 Atmosphere Engine：

```text
Auction Event / Social Event / Timer Signal
-> Event Normalizer
-> User Context Enricher
-> Priority Scheduler
-> Effect Policy
-> UI Renderer + Sound/Haptic Renderer
-> Telemetry
```

核心价值：

- 同一事件对赢家、被超越者、围观者、主播看到的反馈不同。
- 多个事件并发时按优先级合并，避免 UI 噪音。
- 弱网恢复后重放事件不重复播强动效，只做状态同步。
- 所有效果可测试、可降级、可 A/B。

## 功能方案

### A. 3 秒停留钩子：让短视频用户愿意停下

目标用户：正在刷短视频、尚未打算购买的人。

标杆启发：TikTok LIVE Shopping Ads 强调让用户发现并观看 LIVE，再在直播中浏览/购买；Amazon Live 用动态商品 carousel 让用户不离开观看体验即可购物。

设计：

- 首屏必须同时出现：商品真实图/视频、当前价、剩余时间、正在竞争的人数、主播讲解状态。
- feed 入口文案不写“欢迎进入直播间”，而写可行动信息：
  - “最后 23 秒，当前 ¥350”
  - “3 人正在争，差 ¥50 可进前 3”
  - “主播正在讲瑕疵/证书/尺寸”
- 商品卡固定 pin 在直播画面下方，展示当前竞拍商品、下一件预告、运费/保证金摘要。
- 若用户从广告或短视频进入，带入 `entry_source`，首 10 秒优先展示“为什么值得停下”的商品证据，而不是直接催出价。

工程落地：

- `GET /api/rooms/{room_id}/live-summary`：返回 active auction、watcher count、active bidder count、recent bid count、item proof tags。
- WS 增加轻量 `room_heat_updated`，频率最多 1/s，聚合口径写入 payload。
- H5 首屏 skeleton 只显示服务端摘要，不等完整历史。

评委追问回答：

别人做直播购物入口，我们做的是“竞拍态入口”：用户一进来就知道差多少钱、还剩多久、竞争强度如何，并且所有热度都有真实聚合口径。

### B. 竞价状态反馈：从“有状态”升级为“有叙事”

当前已有：领先、被超越、延时、成交短动画。

需要升级：

| 场景 | 用户看到 | 为什么有效 |
|---|---|---|
| 首次有效出价 | “已入局 · 当前第 1” + price tick | 完成从围观到参与的身份转换。 |
| 自己领先 | “领先中 · 守住还剩 08.3s” | 强化拥有感，促使留到终局。 |
| 被超越 | “被张** 超 ¥50 · 一步追回” | 给出明确行动差距，而不是只制造焦虑。 |
| 排名下降但未出局 | “第 3 -> 第 4 · 距前 3 差 ¥100” | 对 Top N 外用户提供低门槛回归路径。 |
| 最后窗口出价 | “触发延时 +10s · 竞争继续” | 解释为什么倒计时回跳，减少不信任。 |
| 输掉 | “差 ¥50 结束 · 已为你保留相似拍品提醒” | 体面退场，承接下一轮转化。 |
| 赢得 | “成交 · 订单已锁定 15:00 内支付” | 情绪高潮后立刻导向履约。 |

工程落地：

- Atmosphere cue 必须带 `cause_seq`、`event_type`、`auction_id`、`user_scope`。
- 客户端保存 `last_effect_seq`，历史恢复不重复播放已消费强动效。
- 动效队列：终态 > outbid > leading > extension > social。
- 相同类型 2 秒内合并，例如连续 bid_accepted 只做 price tick，不连续弹 toast。

### C. 实时排行榜：从 Top N 到“行动型排名”

基础 Top N 不够。竞拍里排行榜的价值是告诉用户“我现在应该做什么”。

P1 排行榜字段：

```json
{
  "auction_id": "auc_1",
  "seq": 42,
  "server_time_ms": 1779435630000,
  "top": [
    { "rank": 1, "user_masked": "张**", "amount_cents": 90000, "last_bid_at": "..." }
  ],
  "me": {
    "rank": 4,
    "best_amount_cents": 75000,
    "gap_to_leader_cents": 15000,
    "gap_to_next_rank_cents": 5000,
    "next_valid_bid_cents": 95000,
    "state": "OUTBID"
  },
  "stats": {
    "active_bidders_30s": 12,
    "accepted_bids_30s": 38,
    "price_velocity_cents_per_min": 18000
  }
}
```

交互：

- Top 3 展示头像/遮罩名/金额；Top 1 有 crown，但不能遮挡金额。
- 我的排名卡固定在 CTA 上方：`第 4 · 距第 3 差 ¥50 · 下次有效出价 ¥950`。
- 排名变化做短位移动画，但金额不跳动错位。
- Top N 外用户不羞辱，只展示“你已入局/未入局/一步进榜”。

为什么比别人强：

- 普通榜单只能制造攀比；行动型榜单直接给出合法下一步，和服务端规则矩阵一致。
- 所有排名来自 accepted bids 聚合，服务端 seq 对齐，可解释、可恢复。

### D. 倒计时与延时：制造紧张，但不破坏信任

标杆启发：Whatnot 卖家可设置短拍卖、最后 10 秒 counter bid reset、Sudden death；eBay 对部分品类测试 final-minute extended bidding。

设计：

- 常规态：秒级倒计时，低干扰。
- 最后 10 秒：显示 0.1s 粒度，但仍由 `server_time_ms + end_at` 推导；本地只负责显示，不裁判。
- 最后窗口 accepted bid：触发延时解释条：
  - “张** 出价触发延时，服务端更新结束时间 18:03:12.400”
- 延时次数达到上限：明确展示“最后一次延时已用完”。
- 可选玩法：
  - `EXTEND_ON_BID`：公平延时，适合高价值商品。
  - `SUDDEN_DEATH`：最后一秒落锤，适合低客单高娱乐，但风险更高，需要商家显式选择。

工程落地：

- 规则增加 `clock_mode`：`EXTEND_ON_BID | SUDDEN_DEATH`。
- WS `auction_extended` 事件必须包含 old_end_at、new_end_at、extend_count、max_extend_count、trigger_bid_id。
- H5 本地倒计时到 0 后进入 syncing，禁用 CTA，直到服务端终态或 snapshot。

### E. 提示音与震动：可感知，但不打扰

Web 约束：浏览器通常要求用户交互后才能播放有声媒体；Vibration API 不保证所有浏览器支持。

设计：

- 默认关闭。
- 用户点击“开启提示音”后初始化 AudioContext，失败则提示“浏览器限制，已保留视觉提醒”。
- 音效分级：
  - leading：短上扬 80-120ms。
  - outbid：短促双音 120-180ms。
  - last 3s：不做连续滴答，最多每秒一次弱提示。
  - sold：落锤音 250-400ms。
- 震动分级：
  - leading：20ms。
  - outbid：30ms + 20ms pause + 30ms。
  - sold：50ms。
- 尊重系统偏好：
  - `prefers-reduced-motion`：关闭非必要动效。
  - 低电量/后台页/隐藏页：关闭声音和震动，只保留通知队列。

测试：

- Playwright 验证开启/关闭状态不会触发 longtask。
- 单事件动效不超过 2s。
- 闪烁不超过 WCAG 阈值。

### F. 主播控场：避免冷场，帮主播把竞价讲起来

主播需要的不是更多图表，而是下一句该说什么。

PC 主播端新增 `Auction Prompter`：

| 触发 | Prompter 文案 | 动作 |
|---|---|---|
| 15 秒无出价 | “可提醒证书/瑕疵/包邮规则” | 展开商品卖点卡 |
| 3 人以上连续出价 | “竞争升温，强调封顶/保证金/售后” | 高亮规则 |
| 最后 10 秒无人出价 | “提醒最后一次机会” | 一键发送系统弹幕 |
| 被超越高频 | “建议口播当前差价和下一口有效价” | 显示差价摘要 |
| 竞拍结束无人出价 | “建议降起拍/换品/重跑” | Re-run draft |

工程落地：

- `GET /api/host/auctions/{id}/prompts` 基于真实事件聚合返回建议。
- 不自动替主播发营销话术，必须人工确认。
- Prompter 事件进入 flight recorder，便于复盘。

为什么比别人强：

- 很多竞拍 Demo 只服务买家；工业级直播竞拍必须服务主播控场，因为冷场是直播转化最大敌人之一。

### G. 商家运营：灵活，但不能破坏公平

商家需要灵活配置玩法，但高压竞拍不能随意改规则。

可配置：

- 拍卖节奏：短拍/标准拍/长拍。
- 延时规则：延时窗口、延时时长、最大延时次数、是否 sudden death。
- 入场门槛：是否要求保证金/支付验证。
- 展示策略：排行榜 Top 3/Top 5、是否展示 active bidder count。
- 预热策略：是否允许 pre-bid/max bid。

不可随意配置：

- SCHEDULED/ACTIVE 后修改起拍价、加价幅度、封顶价。
- 隐藏当前最高价。
- 人工指定 winner。
- 删除已接受出价。

PC 端要给商家“调整空间”：

- DRAFT 可编辑完整规则。
- SCHEDULED 可 unschedule 回 DRAFT，但要记录。
- ACTIVE 只能取消且必须填原因；取消广播给所有用户。
- SOLD 后只走订单/售后，不允许改价。

### H. 代理出价 / Max Bid / Pre-bid：高级但高价值

这是能明显区别普通 Demo 的 P2 功能。

标杆：Whatnot 和 eBay 都支持用户设置最高愿付价，系统按 increment 代理出价；Whatnot pre-bid 可在直播前提交，卖家开拍后自动参与。

产品价值：

- 用户不必每次手动追价，降低高速竞拍操作压力。
- 短视频用户可先设置心理价，之后被直播间事件召回。
- 主播开拍前已有预热 bid，减少冷启动。

工程风险：

- Max Bid 是用户私密信息，不能进入公开 WS payload。
- 多个 Max Bid 竞争时必须服务端原子结算，不能在前端模拟。
- 代理出价可能瞬间连跳多口，必须产生可审计事件链。

设计：

```text
max_bid_intent(user, auction, max_amount)
-> server stores private intent
-> on auction start or competing bid
-> lock auction row
-> compute next public accepted bid
-> write bid rows with source=AUTO_MAX_BID
-> emit public bid_accepted only for actual price changes
```

评委追问回答：

别人做 Max Bid 是拍卖功能；我们把它接入直播氛围：pre-bid 开拍召回、被超越提醒、代理出价解释、主播预热人数都能形成完整转化闭环。

### I. 反作弊与信任氛围

极致氛围不能变成“看起来热闹但不可信”。

风控点：

- Verified bidder：高价值商品要求支付方式/保证金验证。
- Shill bidding 检测：商家关联账号、同 IP/设备、异常只抬价不支付。
- Troll bidding：频繁高额 bid 后不支付、反复误触取消。
- Price manipulation：主播取消重跑、恶意抬价后取消。

体验呈现：

- 用户侧只展示“本场要求保证金/验证，保护真实竞拍”。
- 主播侧展示风险提示，不公开羞辱用户。
- 诊断页记录 risk flags 和处置结果。

### J. 失败与恢复中的氛围

弱网时，最重要的氛围是“系统可信”。

状态：

- connected：正常强动效。
- degraded：降级动效，只展示必要状态。
- recovering：禁用出价，显示“正在同步服务端状态”。
- snapshot_applied：恢复后只补齐状态，不重放历史强动效。
- disconnected：保留最后价格但加 stale 标识。

原则：

- 断线时不播放“被超越”强刺激，避免用户因过期状态误操作。
- 恢复后如果发现用户已被超越，展示“同步后发现已被超越”，而不是伪装成实时被超越。
- 所有恢复过程可在 diagnostics 中看到 snapshot source、age、stale。

## 事件与数据模型

新增/强化事件：

| Event | 来源 | 用户侧 | 主播侧 | 诊断 |
|---|---|---|---|---|
| `auction_heat_updated` | 聚合器 | 热度/参与变化 | 控场参考 | 聚合口径 |
| `leaderboard_updated` | bid accepted 后聚合 | 排名变化 | 竞争态势 | seq 对齐 |
| `user_rank_changed` | 个性化派生 | 我的排名/差距 | 不展示个人隐私 | 不写公共 outbox |
| `auction_extended` | 竞拍事务 | 延时解释 | 口播提示 | old/new end_at |
| `auction_last_call` | scheduler/timer | 最后窗口 | 主播提醒 | server time |
| `max_bid_applied` | 服务端代理出价 | 私密解释 | 不泄露 max | audit only |
| `auction_sold` | 竞拍终态 | 落锤/订单 | 成交复盘 | order link |

注意：

- 公共 WS 只广播不敏感事件。
- 个性化事件可通过用户专属通道或 snapshot 派生，不能把用户 Max Bid 泄露到房间。
- 排行榜可从 PostgreSQL 聚合，P2 再加 Redis materialized projection。

## 当前实现差距

| 模块 | 当前状态 | 差距 |
|---|---|---|
| 排行榜 | 已有 Top N、我的排名、差领先者 | 还不是行动型榜单，缺 gap_to_next_rank、active_bidders_30s、price_velocity、seq freshness。 |
| 事件动效 | 已有 leading/outbid/extended/sold | 还没有统一 Atmosphere Engine、优先级合并、恢复后动效去重。 |
| 音效/震动 | 已有默认关闭开关 | 需要补 AudioContext 初始化失败处理、细分音型、震动分级、reduced-motion 策略。 |
| 倒计时 | 已基于服务端时间原则 | 需要最后 10 秒节奏、延时解释条、延时次数可视化、local zero syncing 状态更强。 |
| 主播控场 | PC 有控制台和近期事件 | 缺 Prompter、冷场检测、一键系统弹幕、竞价态势摘要。 |
| 商家玩法 | 有规则配置和冻结 | 缺 clock_mode、pre-bid/max bid、verified bidder、展示策略配置。 |
| 信任风控 | 有保证金展示、fat-finger、取消原因 | 缺 shill/troll 风险模型、公开口径、risk diagnostics。 |
| 短视频入口 | H5 可进直播间 | 缺 feed entry summary、3s hook、商品证据卡、来源指标。 |

## 分期路线

### P0.5：把已有加分项做扎实

- 排行榜 response 增加 `seq`、`server_time_ms`、`gap_to_next_rank_cents`。
- H5 增加 Atmosphere Engine 队列和去重，替代散落状态判断。
- 增加 reduced-motion、音效初始化失败兜底、震动能力检测。
- 延时事件 UI 展示 old/new end_at、extend_count/max_extend_count。
- 增加 Playwright：动效不遮挡 CTA、恢复后不重播强动效、音效开关不触发 longtask。

### P1：主播和商家可用

- PC Auction Prompter。
- 房间热度聚合 `auction_heat_updated`。
- 商家展示策略配置：Top 3/5、是否展示近 30 秒活跃出价人数。
- 主播系统弹幕：只允许模板化、人工确认、写审计。
- 竞拍复盘：成交价曲线、关键延时、最高参与者数、被超越复出价率。

### P2：行业级差异化

- Max Bid / Pre-bid 服务端代理出价。
- Verified bidder / 保证金验证。
- 风控诊断：shill/troll/关联账号异常。
- 个性化召回：pre-bid 商品开拍、被超越、即将落锤。
- Similar auction handoff：输家承接下一件相似商品。

## 测试门禁

| Gate | 证明 |
|---|---|
| event-truth | 所有强动效都有 `cause_seq`，不能由本地计时或 mock 热度触发。 |
| no-fake-bid | WS/REST 不存在伪造 accepted bid 的 UI 路径。 |
| effect-priority | SOLD 和 recovery 能压过 outbid/leading。 |
| reconnect-dedupe | 断线恢复不会重播历史 leading/outbid 强动效。 |
| longtask | 高连续 bid_accepted 下无不可接受 longtask。 |
| no-cta-block | 任意动效不遮挡/移动出价 CTA。 |
| sound-consent | 未主动开启不播放声音；开启失败有视觉兜底。 |
| reduced-motion | 系统低动效偏好下关闭非必要动画。 |
| leaderboard-freshness | 排行榜 seq 不低于已应用 auction seq 或明确显示 stale。 |
| host-prompter-audit | 主播提示和系统弹幕写入审计，不自动改竞拍状态。 |

## 评委拷打口径

### 你们和普通“排行榜 + 动画”有什么区别？

普通方案只做表现层。我们的方案把氛围做成事件编排系统：服务端权威事件、用户上下文、动效优先级、恢复去重、性能预算、诊断复盘都在设计里。即使弱网、重连、延时、终态竞争发生，也不会把用户引导到错误状态。

### 会不会为了刺激用户而诱导过度消费？

我们把刺激限定在真实竞争和透明规则内：大额跳价有 fat-finger confirm，Max Bid 私密且可控，出价是 binding contract，保证金/运费/封顶价明确展示，弱网时禁用危险 CTA。氛围的目标是提升参与和信任，不是制造虚假稀缺。

### 为什么要做主播 Prompter？

直播竞拍不是纯货架拍卖。冷场时用户会离开，主播需要知道当前该讲商品证据、提醒差价还是解释延时。Prompter 把实时事件转成控场建议，但不自动操控竞拍，因此既提升直播质量，也保留主播主导权和审计性。

### 为什么要考虑 Max Bid / Pre-bid？

成熟拍卖平台已经证明代理出价能降低操作压力。直播场景里它还能解决用户无法全程观看的问题：用户先设置心理价，开拍或被超越时被召回。技术上我们会把 Max Bid 当作私密意图，由服务端在 auction row lock 内结算，不泄露给公开 WS。

### 你们如何证明这套氛围不会拖垮性能？

所有强动效短时、非阻塞、CSS transform 优先；连续事件由 Atmosphere Engine 合并；低端设备和 reduced-motion 可降级；Playwright longtask 和 CTA overlap 是门禁；服务端热度聚合限频，排行榜有 freshness/stale 策略。

### 如果 WS 断了，用户看到的刺激反馈还可信吗？

断线/恢复中直接降级：禁用出价 CTA，停止强动效，展示 stale/recovering。恢复后只应用权威 snapshot，不重播历史强动效。用户看到的是“同步后状态”，不是伪实时幻觉。

## 结论

极致竞价氛围的竞争力不在于“更炫”，而在于把竞拍心理、直播带货、主播控场、商家运营、风控信任和实时工程合成一个闭环。最终展示时要让评委看到：

- 用户愿意停下、看懂、参与、复出价、支付。
- 主播知道怎么控场，不会冷场。
- 商家能配置玩法，但不能破坏公平。
- 平台能解释每个价格、每个 winner、每次延时、每次恢复。
- 工程能证明所有体验都来自真实事件，并能在弱网和高并发下保持可信。
