# 20 · UI/UX Redesign

## 定位

这是一套全新的 UI/UX 产品设计方案，不以当前页面结构、颜色、组件和实现方式为边界。当前实现只作为后文差距审计的输入，不能反向限制新方案。

当前 UI/UX 的问题不是“还不够花”，而是缺少一套能同时服务直播、竞拍、交易信任和高压操作的视觉系统。极致竞价氛围要成立，界面必须做到：

```text
第一眼愿意停下 -> 三秒内看懂局势 -> 一秒内找到行动 -> 出错时知道原因 -> 弱网时相信系统
```

本方案专注 UI/UX 重构，不改变竞拍真源原则：

- H5 是用户的直播竞拍驾驶舱。
- PC 是主播/商家的竞拍运营控制台。
- Host Live Assist 是直播主的控场辅助层。
- Seller Studio 是商家的上架、规则、复盘和风险运营层。
- 所有炫酷效果必须服务清晰、可控、可信；不能遮挡价格、倒计时、CTA、错误和恢复状态。

## 设计北极星

### 体验公式

```text
美感 = 真实商品质感 + 清晰视觉层级 + 品牌化状态语言
炫酷 = 真实事件驱动的可感知反馈 - 装饰噪音
方便 = 关键任务路径最短 + 错误可恢复 + 状态可解释
可信 = 服务端权威状态 + 规则透明 + 失败不掩盖
```

### 核心评价标准

| 问题 | 合格答案 | 极致答案 |
|---|---|---|
| 用户为什么停下？ | 页面好看 | 3 秒内看到真实商品、当前价、剩余时间、竞争强度和下一步动作 |
| 用户为什么敢出价？ | 有按钮 | 金额、规则、保证金、延时、错误保护和服务端状态都清楚 |
| 主播为什么不冷场？ | 有最近事件 | 有 Prompter、热度曲线、竞价态势和口播建议 |
| 商家为什么能运营？ | 能发布商品 | 能配置玩法、预热、复盘、风控，但不能破坏公平 |
| 评委为什么认可？ | UI 炫 | 每个视觉/交互选择都有行业依据、可用性理由和工程门禁 |

## 调研依据

| 标杆/规范 | 观察 | 对本项目的启发 |
|---|---|---|
| 官方宣讲版图片 | PC 图展示商品列表、状态、价格、出价次数、讲解/取消/下架操作；H5 图展示直播间商品半屏列表、出价半屏面板、未成交禁用态、赢家支付弹窗、落槌结果弹窗和直播底部 mini 商品卡。 | 官方最低期望不是单页竞拍，而是“直播间 + 商品列表/半屏面板 + 状态化出价 + 结果/支付弹窗”。我们的重构应保留这些行为，并用 Bid Dock/Bottom Sheet/Command Center 提升清晰度和工业完整度。 |
| TikTok Shop / TikTok LIVE | 购物入口天然在短视频和 LIVE 内，商品链接、购物袋、产品卡要降低离开直播的成本。 | H5 首屏必须像直播产品，不像后台页面；商品卡和出价 CTA 要贴近视频观看动线。 |
| Whatnot | 直播竞拍核心操作是 Swipe/Click to bid、Custom Bid、Max Bid、Pre-bid；用户在实时视频、聊天、商品和出价之间高速切换。 | 出价 CTA 必须是底部稳定主操作；Custom/Max Bid 放二级入口；聊天不能挡商品和 CTA。 |
| eBay Live | 实时直播、聊天、卖家问答、短拍卖；最后几秒出价会延长时间。 | 延时、灰置按钮、错误原因必须清晰解释；直播竞拍 UI 要把规则可视化，减少“为什么又延时”的疑惑。 |
| Amazon Live / IVS | 商品 carousel、chat、timed metadata 让视频内容与商品展示同步。 | 主播讲解、当前商品、证书/瑕疵/卖点应该随事件同步，不应只靠静态商品标题。 |
| Baymard mobile commerce research | 移动电商依赖清晰商品信息、可见操作、精确错误文案和低摩擦表单。 | 出价/支付是交易动作，不是游戏按钮；错误和确认不能模糊。 |
| Apple HIG / Material / Ant Design | 色彩用于表达交互和状态；企业控制台要强调效率、一致性、密度和可扫读性。 | H5 可更沉浸，PC 必须更克制；两端共享 tokens，但信息密度不同。 |
| NN/g usability heuristics | 系统状态可见、匹配真实世界、错误可恢复、一致性、减少记忆负担。 | 实时竞拍 UI 的第一原则是“状态可见”，其次才是“好看”。 |

参考链接：

- Official brief images: `docs/references/official-brief-images/`
- TikTok Shop and Showcase: https://ads.tiktok.com/help/article/tiktok-shopping-and-showcase
- TikTok Shop Tab: https://newsroom.tiktok.com/all-you-need-to-know-about-the-tiktok-shop-tab
- Whatnot livestream bidding: https://help.whatnot.com/hc/en-us/articles/11429967449869-Bidding-on-an-Auction-During-a-Livestream
- Whatnot bidding overview: https://help.whatnot.com/hc/en-us/articles/14932924544141-How-to-bid-in-an-auction
- eBay Live buyer help: https://www.ebay.com/help/buying/ebay-live/livestream-shopping-ebay-live?id=5421
- eBay Live FAQ: https://pages.ebay.com/ebaylive-faq/
- eBay Live Seller Center: https://www.ebay.com/sellercenter/selling/how-to-sell/ebay-live
- Amazon Live / IVS shoppable livestream: https://aws.amazon.com/blogs/media/prmbp-how-amazon-live-is-creating-interactive-shoppable-livestreams-amazon-ivs/
- Amazon Live advertising: https://advertising.amazon.com/solutions/products/amazon-live
- Baymard mobile product pages: https://baymard.com/mcommerce-usability/benchmark/mobile-page-types/product-page
- Baymard validation errors: https://baymard.com/blog/adaptive-validation-error-messages
- Apple Human Interface Guidelines: https://developer.apple.com/design/human-interface-guidelines/
- Material color system: https://m2.material.io/guidelines/style/color.html
- Ant Design principles: https://ant.design/docs/spec/introduce
- NN/g usability heuristics: https://www.nngroup.com/articles/ten-usability-heuristics/

## 当前 UI 问题诊断

## 官方图片参考解读

官方宣讲版的 6 张截图已经下载到 `docs/references/official-brief-images/`。它们应作为 UI/UX 的官方语义参考，但不应成为照抄限制。

| 图片 | 官方隐含要求 | 在本方案中的升级 |
|---|---|---|
| PC 商品列表 | 商家/主播端要能搜索筛选、添加商品、查看起拍价/加价/封顶/当前价或成交价/出价次数，且有讲解、取消讲解、下架、竞拍中/已成交状态。 | P8 的 PC Command Center 保留密集运营能力，但把 ACTIVE 控制面从列表中提升为主战情室。 |
| H5 商品半屏列表 | 用户在直播间内浏览多个竞拍商品，状态包括竞拍中、即将开拍、未成交、已结束、截拍中。 | P6 的 Bottom Sheet 系统保留商品列表/详情浏览，并增加 Bid Dock 保证主竞拍动作不被列表挤走。 |
| H5 出价面板 | 半屏面板集中展示倒计时、商品、当前价、领先者、我的出价、stepper、加价幅度和出价 CTA。 | P6 的 Bid Dock 将这些关键信息常驻底部，避免每次都依赖弹层；Product/Leaderboard sheet 作为扩展。 |
| 出价边界提示 | 高于当前价、自己已最高价等状态要在按钮附近明确提示。 | P7 的 RankStrip/Atmosphere Engine 将提示升级为 action copy：差多少、下一口多少、为何不能重复出价。 |
| 未成交结束态 | 结束未成交时 CTA 禁用，并提示 5 秒后返回直播间。 | P6-S5 拆 Winner/Loser/Unsold result sheet，终态解释和下一步承接更清晰。 |
| 成交/支付弹窗 | 中奖支付弹窗、保证金退回说明、支付倒计时、落槌结果弹窗、直播背景和 mini 商品卡仍可见。 | P6-S5 保留直播上下文中的结果 sheet，增加支付状态、订单锁定、输家承接和诊断可追溯。 |

结论：官方图像强调“半屏面板 + 状态化商品列表 + 结果弹窗 + 直播上下文”。本设计仍坚持全新 UI/UX，但必须覆盖这些官方语义，不能只做一个孤立的单拍品页面。

### H5

| 问题 | 影响 |
|---|---|
| 视觉主题像“静态茶盏展示页”，不够直播、不够竞拍、不够高价值交易。 | 用户第一眼感知不到“正在发生的竞争”。 |
| 背景由渐变和装饰模拟商品，不是真实商品/直播画面。 | 高价值商品缺乏信任证据；不适合珠宝、奢侈品、收藏品。 |
| 价格、倒计时、领先者、CTA 虽然都在，但视觉层级还不够极致。 | 高压最后几秒，用户需要扫一眼就知道能不能出价、下一口多少。 |
| 颜色偏单一绿/灰，状态语义不够强。 | ACTIVE、OUTBID、SOLD、RECOVERING 的情绪差异弱。 |
| 排行榜、历史、弹幕按普通 section 堆叠。 | 像功能清单，不像直播间。 |
| 动效 cue 是普通 toast。 | 有反馈，但缺少品牌级“竞拍落锤/价格跳动/被超越”的记忆点。 |

### PC

| 问题 | 影响 |
|---|---|
| 更像后台 demo，缺少主播运营台的信息架构。 | 主播不知道当前最该看什么、做什么。 |
| 表单、订单、诊断混在同一滚动流里。 | 高压直播时查找成本高。 |
| 当前竞拍控制面不够“战情室”。 | 当前价、倒计时、风险、最近事件没有形成操作焦点。 |
| 诊断表格可用但偏工程裸数据。 | 评委能看出数据是真的，但主播/商家不一定知道问题是什么。 |
| Ant Design 默认感明显，缺少项目品牌识别。 | 工业完整度有，但视觉竞争力弱。 |

## 设计目标

| 目标 | H5 | PC |
|---|---|---|
| 好看 | 沉浸直播、真实商品、强层级、克制炫酷 | 专业控制台、密度清晰、状态色强、少装饰 |
| 炫酷 | 价格 tick、落锤、最后窗口、排名位移 | 实时事件流、热度曲线、风险灯、Prompter |
| 清晰 | 一眼看价格/倒计时/下一口/状态 | 一屏控当前竞拍/规则/订单/诊断 |
| 方便 | 底部固定 CTA、二级操作收纳 | 快捷操作、异常原因、可追溯 drilldown |
| 可信 | 真实商品、服务端状态、弱网透明 | 规则冻结、审计、诊断解释 |

## 全新产品信息架构

不要把系统想成“一个 H5 页面 + 一个 PC 后台”。直播竞拍是四个协作界面：

```text
Viewer H5          用户观看、出价、支付、追拍
Host Live Assist   主播控场、讲解、提醒、互动
Seller Studio      商家上架、规则、排期、复盘、风控
Ops Diagnostics    工程诊断、异常、恢复、证据链
```

### Viewer H5

核心任务：

1. 判断这个商品值不值得看。
2. 判断现在竞拍局势。
3. 用最短路径出价。
4. 出错/弱网/被超越时知道下一步。
5. 成交后支付，未成交后承接下一件。

导航不使用传统 tab-first 结构，而使用直播间结构：

- Stage：视频/商品/主播。
- Bid Dock：价格、时间、排名、CTA。
- Drawer：商品详情、规则、榜单、历史、订单。
- Overlay：弹幕、事件 cue、主播提示。

### Host Live Assist

核心任务：

1. 当前商品讲解。
2. 当前竞拍控场。
3. 冷场时找话术。
4. 高潮时提醒差价和倒计时。
5. 异常时解释并安抚。

界面应是右侧辅助层，而不是复杂后台：

- 当前竞拍卡：价格、剩余时间、领先者、延时次数。
- Prompter：下一句建议。
- Audience Pulse：近 30 秒互动、出价、停留。
- Quick Actions：系统弹幕模板、强调规则、切换下件。

### Seller Studio

核心任务：

1. 上架高价值商品并提供信任材料。
2. 配置竞拍规则和展示策略。
3. 排期/预热/开拍。
4. 查看成交、支付、复盘。
5. 处理取消、异常和风控。

信息架构：

- Catalog：商品与素材。
- Auction Setup：规则与玩法。
- Live Schedule：排期与预热。
- Results：订单、支付、成交复盘。
- Risk & Trust：保证金、验证、异常、取消审计。

### Ops Diagnostics

核心任务：

1. 解释为什么某次价格/排名/成交发生。
2. 定位 WS/outbox/scheduler/payment/recovery 问题。
3. 为评委和工程复盘提供证据。

UI 应该专业但不面向普通用户：

- Timeline-first flight recorder。
- 每个 row 可 drilldown 到 trace。
- 异常有 severity、impact、next action。

## 三条关键旅程

### 用户旅程：从刷到到出价

```text
Feed / 分享进入
-> 看到真实商品 + 当前价 + 剩余时间
-> 展开证据 chips 或听主播讲解
-> 看到“差 ¥50 入前 3”
-> 点击/滑动出价
-> 服务端确认 leading/outbid
-> 被超越后一步追回
-> 成交后支付或输家承接下一件
```

设计要求：

- 首屏不放完整规则长文，只放“起拍/加价/封顶/延时”短 chips。
- 出价前必须看得到下一口金额。
- 被超越不只提示情绪，要给“下一口合法价”。
- 输家要看到“差多少输”和“下一件相似拍品”，避免挫败直接离开。

### 主播旅程：从讲解到控场

```text
开播
-> 当前商品卡 + 讲解要点
-> 竞价升温看到 Prompter
-> 最后 10 秒口播差价
-> 延时发生解释规则
-> 成交后引导支付/下一件
-> 冷场时切换讲点或重跑策略
```

设计要求：

- Prompter 不抢主屏，只做右侧/浮层提示。
- 主播所有一键弹幕必须人工确认。
- 主播看到的是聚合态势，不暴露用户隐私上限。

### 商家旅程：从发布到复盘

```text
上传商品图/证书/描述
-> 配置规则和玩法
-> 系统检查 cap/increment/保证金
-> 排期预热
-> 直播中看结果和异常
-> 成交/支付/取消审计
-> 复盘成交曲线和流失点
```

设计要求：

- 配置页不是表单堆叠，而是“价格规则/时间规则/信任规则/展示策略”分步。
- 每一步有预览：用户端会怎么显示。
- ACTIVE 后冻结规则必须清楚解释，不只是灰按钮。

## 视觉方向

### 品牌关键词

```text
高价值 / 直播感 / 竞拍张力 / 可信交易 / 专业控场
```

避免：

- 游戏化到廉价。
- 纯后台灰白。
- 单一绿/蓝/紫主题。
- 大面积渐变、装饰光球、假 3D 背景。
- 卡片套卡片。

## 美学系统

### 视觉隐喻

推荐隐喻不是“电商促销”，而是“现代拍卖现场 + 直播工作室”：

- 现代拍卖现场：高价值、克制、可信、落锤仪式感。
- 直播工作室：实时、互动、热度、主播控场。
- 交易终端：数字清晰、状态明确、错误可解释。

这三者的比例：

```text
H5: 50% 直播工作室 + 30% 现代拍卖现场 + 20% 交易终端
PC: 20% 直播工作室 + 20% 现代拍卖现场 + 60% 交易终端
```

### 色彩策略

高价值竞拍不能用廉价大促红，也不能用全黑游戏风。建议用“深色视频舞台 + 金色交易高光 + 状态色点缀”。

色彩层级：

| 层级 | 用途 | 色彩 |
|---|---|---|
| Brand | 高价值、成交、领先 | auction-gold |
| Action | 主出价 CTA、最后窗口 | bid-red 或 gold，按状态二选一 |
| Trust | 连接成功、支付成功、验证通过 | trust-green |
| Info | 直播、同步、普通提示 | live-cyan |
| Warning | 延时、待确认、恢复中 | risk-orange |
| Neutral | 商品信息、规则、历史、表格 | ink/surface/muted |

状态色使用必须满足：

- 同屏最多一个高饱和主色。
- 红色只用于“紧急/被超越/最后窗口/危险操作”。
- 金色只用于“领先/高价值/成交”，不能用于普通装饰。
- 绿色只用于“可信成功”，不能用于诱导点击。

### 材质与空间

H5：

- 视频/商品为最大视觉面。
- Bid Dock 是半透明但高对比的实用面板，不能玻璃化到看不清。
- Bottom sheet 使用清晰白/深灰实体面，不用模糊背景承载正文。
- CTA 有厚度和按压反馈，但不做夸张拟物。

PC：

- 明亮表面、低阴影、清晰分割线。
- 当前竞拍面板可以有一条状态色边框。
- 诊断/表格使用 density tokens，减少大块空白。

### 图像规范

UI 好看的第一优先级是真实商品资产，而不是 CSS 背景。

商品图要求：

- 主图展示实际商品，不使用纯氛围图。
- 高价值商品需要细节图：证书、瑕疵、尺寸参照、材质纹理。
- H5 Stage 支持 video poster；没有真实视频时用商品主图 + 轻动效，不用渐变假图。
- PC 商品列表必须显示缩略图，主播控制面显示主图。

### 图标与符号

使用 lucide 或设计系统图标：

- Hammer：成交/落锤。
- Trophy/Crown：领先/榜首。
- Wifi/WifiOff：连接。
- Shield/BadgeCheck：验证/保证金。
- Clock：倒计时/延时。
- AlertTriangle：风险/异常。

原则：

- 图标辅助扫描，不替代文字。
- 不用自绘复杂 SVG 做主要卖点。
- 所有图标按钮有 tooltip 或 aria-label。

### 动效美学

动效风格：短、准、像拍卖现场，不像游戏抽卡。

节奏：

- Micro：100-200ms，按钮按压、chip 更新。
- Event：300-700ms，价格 tick、排名移动。
- Moment：800-1800ms，领先/被超越/延时 cue。
- Finale：1200-2400ms，成交结果，但 CTA/支付路径必须可见。

禁止：

- 无限循环高亮。
- 纯装饰粒子。
- 震屏导致阅读困难。
- 页面整体 scale 或 blur。
- 背景大面积闪烁。

## 交互范式

### H5：One-Hand Auction

用户大概率单手持机，底部是行动区，上半屏是观看区。

规则：

- 主 CTA 在拇指热区，固定底部。
- Stepper 与主 CTA 相邻。
- 商品详情/榜单/历史都用 bottom sheet，不跳出直播。
- 二级操作隐藏在 sheet，但关键状态不隐藏。
- 所有弹层都不能盖住恢复/错误状态。

### 出价方式矩阵

| 方式 | 适用 | 优点 | 风险 | 设计 |
|---|---|---|---|---|
| Tap to Bid | 普通加价 | 快 | 误触 | 小额可 tap，大额 confirm |
| Press and Hold | 高价值/最后窗口 | 降误触 | 慢 | 仅在风险高时启用 |
| Slide to Bid | 高价值/拍卖高潮 | 仪式感强 | 单手成本 | 可配置，不做默认 |
| Custom Bid | 专业用户 | 灵活 | 输入成本 | bottom sheet |
| Max Bid | 无法盯直播用户 | 留存强 | 隐私/解释复杂 | P2 服务端代理 |

默认建议：

- 普通竞拍：Tap to Bid + fat-finger confirm。
- 高价值珠宝：Tap to Bid + 大额 Press confirm。
- 娱乐低价：可选 Slide to Bid 增强仪式感。

### PC：Command Center

PC 不追求沉浸，而追求一屏控场。

规则：

- 当前 ACTIVE 永远置顶。
- 危险动作放右侧或底部，必须二次确认。
- 最近事件流常驻。
- Prompter 常驻但可折叠。
- 表格是辅助，不是主屏。

### Seller Studio：Wizard + Preview

商家配置不能只堆字段。

推荐：

```text
Step 1 商品素材
Step 2 价格规则
Step 3 时间/延时玩法
Step 4 信任与保证金
Step 5 用户端预览
Step 6 排期发布
```

每一步右侧展示 H5 preview，让商家知道用户会看到什么。

### 推荐主题：Auction Studio

H5 用“暗色视频舞台 + 高亮交易控件”，PC 用“明亮控制台 + 强状态色”。

颜色 token：

```text
ink-950        #111315  主文字/深色舞台
surface-0      #FBFCFE  主背景
surface-1      #F3F5F8  次级背景
line           #D9DEE7  分割线
muted          #667085  次级文字
bid-red        #FF3B30  被超越/紧急/最后窗口
auction-gold   #D6A84F  领先/成交/高价值
trust-green    #10B981  成功/已连接/已支付
live-cyan      #22D3EE  直播/同步/信息
risk-orange    #F97316  延时/恢复/待确认
```

使用规则：

- H5 背景 70% 来自真实商品/视频画面，不靠纯色主题撑场。
- CTA 使用 `auction-gold` 或 `bid-red`，但同屏只能一个主 CTA。
- `trust-green` 只用于连接成功、支付成功、规则合法，不能滥用。
- `bid-red` 不用于普通按钮，避免降低警示意义。
- PC 主体保持浅色，状态色只用于标签、左边框、趋势线和危险按钮。

### 字体与排版

H5：

- 价格：36-44px，tabular numbers，固定行高。
- 倒计时：20-28px，最后 10 秒可切 0.1s，但宽度固定。
- CTA：18px，按钮高度 56-64px。
- 商品标题：18-22px，最多两行。
- 辅助信息：12-14px。

PC：

- 控制台标题：20-24px。
- 核心数字：28-36px。
- 表格正文：13-14px。
- 标签/状态：12px，固定高度。

排版原则：

- H5 用垂直信息阶梯：视频/商品 -> 竞拍状态 -> CTA -> 互动。
- PC 用三栏控制台：左商品/规则，中当前竞拍，右事件/风险。
- 数字对齐使用 tabular nums，避免价格跳动造成布局抖动。
- 所有固定格式组件定义稳定高度和 min/max 宽度。

## H5 重构方案

### 首屏布局

目标：打开直播间后 3 秒内，用户不用思考就知道“这是什么、多少钱、还剩多久、下一步干嘛”。

```text
┌────────────────────────────┐
│ 真实直播/商品画面             │
│ LIVE · room · viewer         │
│ 主播/商品证据浮层              │
│ pinned item mini card        │
├────────────────────────────┤
│ 当前价 ¥350.00     剩 08.4s  │
│ 张** 领先 · 我第 2 · 差 ¥50  │
│ [ - ] ¥400.00 [ + ]          │
│ [ 长按/滑动 出价 ¥400.00 ]    │
└────────────────────────────┘
```

关键要求：

- 视频/商品画面必须是真图或真实占位素材；不再用渐变模拟商品。
- 底部竞价面板 sticky，价格、倒计时、下一口、CTA 永远同屏。
- 弹幕只在视频区域左下角，不能进入 CTA 安全区。
- 排行榜默认折叠成“我的排名条”，点开再看 Top N。
- 历史/订单放二级 tab，不在首屏堆叠。

### H5 全新界面蓝图

```text
┌──────────────────────────────┐
│ LIVE · 主播 · 连接状态 · 静音   │
│                              │
│         真实商品/直播画面       │
│    证书 chip  成色 chip        │
│                              │
│ 弹幕 overlay                  │
│ 商品 mini card · 查看详情       │
├──────────────────────────────┤
│ ¥350.00           剩 08.4s    │
│ 张**领先 · 我第2 · 差¥50       │
│ ─ 延时 1/3 · 服务端同步 ─       │
│ [ - ]   ¥400.00   [ + ]       │
│ [ 出价 ¥400.00 ]              │
│ 规则 · 榜单 · 历史 · 订单       │
└──────────────────────────────┘
```

首屏视觉层级：

1. 商品/直播画面。
2. 当前价 + 倒计时。
3. 下一口 CTA。
4. 我的排名/差距。
5. 商品证据和弹幕。

理由：

- 用户先进直播间是看商品，不是看表格。
- 但最后几秒决策依赖价格/时间/CTA，所以 Bid Dock 要稳定压在底部。
- 榜单、历史、规则都是重要信息，但不能挤掉主行动。

### 底部 Bid Dock 规格

高度：

- normal：188-220px。
- compact：150-170px，适配小屏。
- safe area：底部加 `env(safe-area-inset-bottom)`。

内容：

- Row 1：当前价 + 倒计时。
- Row 2：领先者/我的排名/连接状态。
- Row 3：stepper。
- Row 4：主 CTA。
- Row 5：规则/榜单/历史/订单快捷入口。

状态变体：

| State | Dock 视觉 |
|---|---|
| ACTIVE | 高对比、CTA 可用 |
| SELF_LEADING | CTA 降级为“你已领先”，主色 gold |
| OUTBID | 左边红色 edge，CTA 文案“一步追回 ¥x” |
| PENDING | CTA loading，价格保持权威旧价 |
| RECOVERING | dock 降饱和，CTA disabled，显示同步条 |
| SOLD_WINNER | CTA 变“去支付”，成交价固定 |
| SOLD_LOSER | CTA 变“查看下一件”，显示差价 |

### Bottom Sheet 系统

所有非主行动进入 sheet：

- Product Sheet：商品证据、规则、主播讲解节点。
- Leaderboard Sheet：Top N、我的排名、价格曲线。
- History Sheet：我的出价、订单、支付。
- Custom Bid Sheet：手动输入。
- Confirm Sheet：高额确认、支付确认。

Sheet 规则：

- 默认半高，不遮住顶部直播完全。
- 拖到全高后才进入详细阅读。
- 关闭手势明确，不误触提交。
- 主 CTA 不在多个 sheet 里重复出现，避免用户不知道点哪个。

### Empty / Loading / Error 美学

不要用“暂无数据”结束用户旅程。

| 状态 | 文案/视觉 |
|---|---|
| 无出价 | “还没人出价 · 第一口 ¥x” + 主 CTA |
| 榜单空 | “出价后显示排名” |
| 历史空 | “本场还没有你的出价” |
| 恢复中 | “正在同步服务端价格” + shimmer |
| WS 断开 | “网络断开，出价已暂停” + 重连状态 |
| 出价失败 | 业务原因 + 下一步行动 |

### 支付与成交 UX

成交不是终点，是履约开始。

赢家：

- 成交结果 sheet 自动弹出。
- 展示成交价、订单锁定倒计时、保证金状态。
- 主 CTA “去支付”。
- 支付成功后反馈“保证金已处理/订单已完成”。

输家：

- 不只显示失败。
- 展示“差 ¥50 结束”。
- 推荐下一件相似拍品/提醒。
- 保留历史，减少不公平感。

### 出价交互

基础：

- 主按钮：`出价 ¥400.00`。
- Stepper：左右减/加，按钮尺寸 44x44。
- Custom bid：二级入口，底部 sheet 输入。
- Max bid：P2 二级入口，明确“系统会按加价幅度代理出价，不公开你的上限”。

防误触：

- 高价值商品主 CTA 可用“按住确认”或“滑动出价”，但不能让普通出价太慢。
- fat-finger confirm 用 bottom sheet，不用全屏弹窗。
- 如果价格在用户准备点击期间变化，按钮文案必须更新并短暂显示“价格已变，下一口已同步”。

为什么不是简单大按钮：

- 直播竞拍中用户常单手操作，底部 sticky CTA 是最高效路径。
- 但 Reddit/用户反馈类问题显示 swipe/误触可能造成高额错误，因此高价跳价必须有确认和清晰金额。

### 商品信息架构

高价值商品要让用户信任，不只看氛围。

首屏可见：

- 商品标题。
- 当前价。
- 证书/成色/尺寸/运费/保证金最多 3 个 proof chips。

底部 sheet：

- 图集/视频证据。
- 规则：起拍、加价、封顶、延时。
- 保证金/支付/退货口径。
- 主播讲解时间点：由 timed metadata 或最近事件驱动。

### 排行榜 UI

从“榜单列表”改成“行动型排名条”。

默认态：

```text
第 2 名 · 差领先者 ¥50 · 下一口 ¥400
```

展开态：

- Top 3 横向 podium。
- Top 4-10 紧凑列表。
- 我的排名固定 sticky。
- 金额、排名变化有位移动效，但不改变容器高度。

颜色：

- 我领先：gold + trust glow。
- 我被超越：red edge flash 1 次。
- 我未入局：neutral + “出一口进入竞争”。

### 弱网与恢复 UI

弱网时不要“炫”，要“稳”。

状态：

- Connected：绿色小点 + “服务端同步中”。
- Degraded：青色/橙色小条 + “网络波动，正在校准”。
- Recovering：面板变低饱和，CTA disabled，显示 skeleton。
- Stale：价格旁加 `stale` 标签，不能只放角落。

禁止：

- 恢复中播放被超越强动画。
- 本地倒计时归零后直接显示 SOLD。
- 把断线提示藏在很小的连接状态里。

### 动效语言

动效要像“拍卖锤”和“价格电报码”，不是普通 toast。

| 场景 | 动效 | 技术 |
|---|---|---|
| price tick | 数字 rolling + 背景短 pulse | CSS transform, tabular nums |
| leading | 金色 ring 从 CTA 扩散一次 | transform/opacity |
| outbid | 左侧红色 edge flash + 排名条上移 | transform/opacity |
| extended | 倒计时条拉长，显示 +10s | width transform |
| sold | 落锤 icon + 结果 sheet | CSS keyframe, <= 400ms 主动效 |
| recovery | shimmer skeleton + 状态条 | low-cost gradient, reduced-motion fallback |

门禁：

- 强动效不超过 2 秒。
- 不遮挡 CTA。
- 不使用超过 WCAG 安全阈值的闪烁。
- `prefers-reduced-motion` 下关闭位移/闪烁，只保留颜色和文案。

## PC 控制台重构方案

### 信息架构

PC 不是“移动端放大版”。主播/商家在直播中需要控场、改 DRAFT、查订单、看异常。

推荐结构：

```text
Top bar: room / health / clock / user
Left rail: Auctions / Create / Orders / Diagnostics / Replay
Main center: Active Auction Control
Right rail: Event Stream + Prompter + Risk
Bottom/secondary: Rules and tables
```

### PC 全新界面蓝图

```text
┌────────────────────────────────────────────────────────────┐
│ Room · Server clock · WS/Outbox/Scheduler health · Host    │
├───────────────┬──────────────────────────────┬─────────────┤
│ Auction Queue │ ACTIVE COMMAND PANEL          │ Live Assist │
│ ACTIVE pinned │ ¥350.00       08.4s           │ Prompter    │
│ Scheduled     │ Leader 张**   Seq 42          │ Heat pulse  │
│ Draft         │ [Start] [Narrate] [Cancel]    │ Event feed  │
├───────────────┴──────────────────────────────┴─────────────┤
│ Rule Editor / Orders / Diagnostics / Replay drawers         │
└────────────────────────────────────────────────────────────┘
```

设计理由：

- 主播直播中最需要当前竞拍和下一步控场，不应该在表格里找 ACTIVE。
- 商家配置和诊断重要，但不是每秒都要看，应作为 secondary workspace。
- 工程诊断保持可见入口，但不挤占主播操作核心。

### Active Auction Control

当前竞拍控制面升级为战情室：

- 当前价：最大数字。
- 剩余时间：第二大数字。
- 领先者/出价轮数/延时次数：同一 stats row。
- 状态灯：ACTIVE/SCHEDULED/SOLD/CANCELLED。
- 操作按钮：Start、Schedule、Cancel、Narrate，危险操作带 confirm。
- 当前 Prompter：主播下一句建议。

视觉：

- 左边用真实商品缩略图，不用空表格文字。
- 中间核心数字大而少。
- 右侧事件流按 severity 着色。
- ACTIVE 竞拍有清晰边框或顶部状态条，但不使用全屏红色。

### Host Live Assist

这是新增的主播控场 UX，不受当前 PC 实现限制。

模块：

| 模块 | 内容 | UI |
|---|---|---|
| Prompter | “最后 10 秒，可提醒差 ¥50 一步追回” | 右侧高优先建议卡 |
| Heat Pulse | 近 30 秒观看/聊天/出价趋势 | 小折线 + 数字 |
| Talk Points | 证书、瑕疵、尺寸、售后 | 可点击口播卡 |
| System Chat | 模板弹幕：规则/延时/成交提醒 | 人工确认按钮 |
| Risk Hint | 网络波动/支付未完成/异常取消风险 | 橙/红色提示 |

交互：

- Prompter 卡只显示 1 条主建议 + 2 条次建议。
- 主播可 dismiss，dismiss 写入本地偏好。
- 一键弹幕必须二次确认，不能自动发送。
- 高风险提示优先级高于营销提示。

### Seller Studio 全新方案

商家端不是主播直播台，而是运营准备和复盘系统。

信息架构：

```text
Catalog
  - 商品资料
  - 图片/视频/证书
  - 瑕疵说明
Auction Setup
  - 价格规则
  - 时间玩法
  - 保证金/验证
  - 展示策略
Preview
  - H5 首屏预览
  - 主播 Prompter 预览
Schedule
  - 排期
  - 预热
Results
  - 成交曲线
  - 出价漏斗
  - 支付/订单
Risk & Audit
  - 取消原因
  - 异常用户
  - 规则变更记录
```

关键 UX：

- Wizard + Preview：每配置一步都看到 H5 用户端效果。
- Rule Explain：不是只填 `extend_window_seconds`，而是解释“最后 10 秒内有人出价，自动延长 10 秒，最多 3 次”。
- Strategy Templates：
  - High-value jewelry：保证金、延时、verified bidder、证书优先。
  - Fast collectibles：短拍、强倒计时、Top 3 榜单。
  - Charity/celebrity：透明规则、故事卡、捐赠结果。
- Publish Checklist：主图、证书、规则、保证金、取消口径全部通过后才能排期。

### Ops Diagnostics 全新方案

诊断不是普通表格，而是“解释竞拍事实”的界面。

Timeline-first：

```text
seq 39 bid_accepted
seq 40 auction_extended
seq 41 bid_rejected BID_TOO_LOW
seq 42 auction_sold
order created
payment initiated
```

每个节点显示：

- 事件类型。
- server_time_ms。
- trace_id。
- 影响用户。
- 对应 outbox delivery。
- 对应 Redis snapshot/history 状态。
- 如果异常，显示 impact 和 next action。

评委看点：

- UI/UX 不只好看，也能解释为什么 winner 是这个人。
- 诊断界面是可信体验的一部分。

### 规则表单

当前规则字段完整，但布局可以更专业：

- 分组：价格规则、时间规则、保证金/确认、风控。
- 每组 2-3 列栅格。
- 封顶价错误用 inline message + 建议 chips。
- SCHEDULED/ACTIVE 冻结态不是简单 disabled 灰掉，而是显示“已冻结于 xx:xx，可取消排期后修改”。
- 保存按钮 sticky 在表单底部，避免滚动后找不到。

### 诊断 UX

诊断页要让工程评委看出真实，也要让主播/商家知道该做什么。

改造：

- 每个 tab 上显示 count 和状态点。
- 表格上方给一句解释：
  - Outbox：`3 READY · oldest 1200ms · shard 2`
  - Recovery：`12 reconnects · 2 snapshot from DB`
- Row 点击打开 flight recorder drawer，不跳新页面。
- 异常 row 显示“影响/建议动作/trace_id”。
- 空状态要说明“暂无异常”，不是“暂无数据”。

### PC 视觉风格

PC 应采用“运营中台”风格：

- 浅色背景。
- 信息密度高。
- 表格清晰。
- 状态色克制。
- 图标辅助扫描，不做装饰。

不要：

- 大 hero。
- 玻璃拟态。
- 大面积深色背景。
- 卡片套卡片。
- 表格列过多但没有优先级。

## 组件规范

### H5 组件

| 组件 | 用法 | 约束 |
|---|---|---|
| LiveStage | 真实视频/商品画面、顶部直播状态、商品证据浮层 | 不承载主 CTA |
| BidDock | price/countdown/rank/stepper/CTA | sticky bottom, fixed height |
| RankStrip | 我的排名、差距、下一口 | 默认显示，Top N 折叠 |
| ProductSheet | 商品详情、规则、证据 | bottom sheet，不跳页面 |
| ConfirmSheet | 高额确认/支付确认 | 明确金额和后果 |
| EventCue | 领先/被超越/延时/成交 | 有 cause_seq，不阻塞 |
| ChatOverlay | 弹幕 | 限高，可隐藏，不进 CTA safe zone |

### PC 组件

| 组件 | 用法 | 约束 |
|---|---|---|
| AuctionCommandPanel | 当前价/倒计时/操作 | 一屏核心 |
| AuctionQueue | DRAFT/SCHEDULED/ACTIVE 商品列表 | ACTIVE 固定置顶 |
| RuleEditor | 分组规则表单 | 冻结态解释 |
| PrompterPanel | 主播控场建议 | 人工确认 |
| EventTimeline | 最近事件 | 按 seq/time 可追溯 |
| DiagnosticsDrawer | flight recorder | 不离开控制台 |
| HealthRibbon | WS/outbox/scheduler/DB 摘要 | 顶部常驻 |

## 独立任务级深挖清单

下面每一项都可以作为独立设计/实现任务，不依赖当前 UI 结构。

### 1. 视觉品牌系统

交付：

- Design tokens。
- H5/PC 色板。
- 状态色语义。
- 字体和数字排版。
- 商品图使用规范。

验收：

- 截图不再像普通后台 demo。
- ACTIVE/OUTBID/SOLD/RECOVERING 一眼可区分。
- 颜色不靠单一色系堆叠。

### 2. H5 Live Stage

交付：

- 真实商品/视频 stage。
- 顶部 LIVE/连接/静音栏。
- 商品证据 chips。
- 弹幕安全区。

验收：

- 首屏像直播竞拍，不像静态页面。
- 弹幕不遮挡商品核心和 Bid Dock。

### 3. H5 Bid Dock

交付：

- Sticky bottom 出价面板。
- price/countdown/rank/next bid/CTA 同屏。
- 所有状态变体。

验收：

- 390x844 和 360px 宽度都不溢出。
- 最后 10 秒仍能一眼看懂下一步。

### 4. 出价交互系统

交付：

- Tap bid。
- Stepper。
- Custom bid sheet。
- Fat-finger confirm sheet。
- P2 Max Bid 入口设计。

验收：

- 普通出价一步完成。
- 高额出价不会误触。
- 价格变化时按钮不会提交过期金额。

### 5. 行动型排行榜

交付：

- RankStrip。
- Leaderboard sheet。
- 我的差距/下一口/排名变化。

验收：

- 用户知道“差多少、下一步出多少”。
- Top N 不挤掉主 CTA。

### 6. 商品信任详情

交付：

- Product sheet。
- 证书/瑕疵/尺寸/保证金/售后。
- 主播讲解节点。

验收：

- 高价值商品有足够信任材料。
- 规则解释不是工程字段，而是用户语言。

### 7. 动效与声音系统

交付：

- Motion language。
- EventCue。
- sound/haptic policy。
- reduced-motion fallback。

验收：

- 炫酷但不遮挡 CTA。
- 所有强动效绑定服务端事件。
- longtask 通过。

### 8. 弱网与恢复体验

交付：

- degraded/recovering/stale/disconnected UI。
- 恢复后状态解释。
- CTA 禁用和重连进度。

验收：

- 弱网时用户不会误以为还能安全出价。
- 恢复后不重播过期强动效。

### 9. 成交/支付/输家承接

交付：

- Winner result sheet。
- Loser result sheet。
- Payment CTA。
- Similar auction handoff。

验收：

- 赢家从情绪高潮顺滑进入支付。
- 输家不直接流失。

### 10. PC Command Center

交付：

- ACTIVE 置顶。
- 当前价/倒计时/状态/操作。
- 事件流和风险提示。

验收：

- 主播不用滚动就能控当前竞拍。
- 危险操作可解释、可确认。

### 11. Host Live Assist

交付：

- Prompter。
- Heat Pulse。
- Talk Points。
- System Chat templates。

验收：

- 冷场时有建议。
- 高潮时有差价/倒计时口播提示。
- 不自动操纵竞拍。

### 12. Seller Studio

交付：

- Catalog。
- Auction setup wizard。
- H5 preview。
- Strategy templates。
- Results replay。

验收：

- 商家能配置玩法但不能破坏公平。
- ACTIVE 前能看到用户端展示效果。

### 13. Diagnostics Experience

交付：

- Timeline-first flight recorder。
- Impact/next action。
- Drawer drilldown。

验收：

- 能解释价格、排名、延时、成交。
- 工程数据变成可理解证据链。

### 14. Visual Regression System

交付：

- H5 多状态截图。
- PC 控制台截图。
- reduced-motion 截图。
- 小屏 text-fit 截图。

验收：

- UI 质量不靠人工感觉。
- 每次改动能发现重叠、遮挡、风格回退。

## 响应式与安全区

H5：

- 390x844 作为主设计基准。
- 360px 宽度下 CTA 文字必须不换成三行。
- 底部 safe area 适配 `env(safe-area-inset-bottom)`。
- 视频区域聊天最多占 35% 高度。
- 横屏不作为主体验，但不能遮挡 CTA。

PC：

- 1440px：三栏。
- 1280px：右侧事件栏可折叠。
- 1024px：规则/订单下移，控制面保持顶部。
- 表格列优先级：source/status/seq/time/error，其他进 drawer。

## 可访问性

- 所有状态变化用 `aria-live="polite"`，成交/错误用 assertive 但限频。
- 颜色不能作为唯一状态区分；必须有文字/图标。
- CTA 最小触控 44px。
- 错误文案给行动建议。
- reduced-motion 下禁用价格 rolling、edge flash、rank movement。
- 音效默认关闭；开启失败有视觉兜底。

## 技术落地

### Design Tokens

新增 `frontend/shared-design/tokens.css` 或两端各自导入同源 token：

```css
:root {
  --color-ink-950: #111315;
  --color-surface-0: #fbfcfe;
  --color-surface-1: #f3f5f8;
  --color-line: #d9dee7;
  --color-muted: #667085;
  --color-bid-red: #ff3b30;
  --color-auction-gold: #d6a84f;
  --color-trust-green: #10b981;
  --color-live-cyan: #22d3ee;
  --color-risk-orange: #f97316;
  --radius-sm: 6px;
  --radius-md: 8px;
  --space-1: 4px;
  --space-2: 8px;
  --space-3: 12px;
  --space-4: 16px;
  --space-6: 24px;
  --font-tabular: "Inter", "Segoe UI", Arial, sans-serif;
}
```

### H5 重构顺序

1. 抽 `BidDock`，先稳定价格/倒计时/CTA。
2. 抽 `LiveStage`，替换当前渐变商品背景为真实 image/video。
3. 抽 `RankStrip`，将排行榜从 full section 改为首屏行动条 + bottom sheet。
4. 抽 `ProductSheet` 和 `HistorySheet`，减少首屏堆叠。
5. 接入 Atmosphere Engine 的视觉层。
6. 做 reduced-motion、safe-area、text-overflow、longtask 门禁。

### PC 重构顺序

1. 抽 `AuctionCommandPanel`，把当前控制面升级为顶部核心。
2. 重排 Layout：left auction queue / center control / right event+prompter。
3. RuleEditor 分组化。
4. Diagnostics 改 drawer + count/status tab。
5. 统一 tokens，降低 AntD 默认感。

## 测试门禁

| Gate | 检查 |
|---|---|
| h5-first-screen | 390x844 下 price/countdown/rank/CTA 同屏可见 |
| h5-safe-zone | chat/effect/sheet 不遮挡 CTA |
| h5-visual-states | leading/outbid/recovering/sold 截图差异明确 |
| h5-text-fit | 360px 下最长金额/错误文案不溢出 |
| h5-reduced-motion | reduced-motion 下无位移动效 |
| h5-longtask | 连续 bid_accepted 下无不可接受 longtask |
| pc-command-panel | ACTIVE 竞拍一屏可控，按钮状态正确 |
| pc-rule-freeze | SCHEDULED/ACTIVE 冻结态有解释，不只是 disabled |
| pc-diagnostics | row 可打开 flight recorder drawer |
| visual-regression | H5/PC 关键状态截图进入基线 |

## 评委拷打口径

### 为什么你们的 UI 比普通直播购物好？

普通直播购物 UI 以商品购买为核心，我们的是竞拍驾驶舱：当前价、倒计时、下一口、我的排名和出价 CTA 永远同屏；商品信息和直播互动不挤压交易动作。它既有直播沉浸感，又保留拍卖场的高压清晰度。

### 为什么不做更炫的全屏动画？

竞拍是高价值交易，不是纯游戏。全屏动画会遮挡价格和 CTA，弱网或最后几秒会直接伤害成交和信任。我们的动效只强化真实事件，不阻塞输入，并有 longtask、safe-zone、reduced-motion 门禁。

### 为什么 PC 不做成炫酷大屏？

主播和商家需要的是控场效率，不是观赏大屏。PC 重构为运营控制台：当前竞拍、规则、订单、事件和诊断各有固定位置；异常能追溯，规则冻结能解释，危险操作有 guardrail。

### 为什么要投入真实商品视觉？

高价值商品的信任来自真实细节：证书、瑕疵、尺寸、材质、主播讲解。渐变背景再好看也不能替代商品证据。H5 的视觉吸引力应该由真实商品画面和清晰交易控件共同建立。

### 如何避免“好看但不好用”？

每个视觉决定都有任务约束：首屏 3 秒看懂、CTA 不遮挡、状态颜色有语义、错误可恢复、弱网可解释。我们用截图回归、可访问性、longtask、text-fit 和 safe-zone 测试把审美变成工程门禁。

## 当前实现差距

| 项目 | 当前 | 重构目标 |
|---|---|---|
| H5 首屏 | 视频舞台 + 下方多个 section | 直播画面 + sticky BidDock + RankStrip |
| 商品视觉 | 渐变/装饰模拟 | 真实商品图/视频 + 证据 chips |
| 颜色 | 单一绿灰 | Auction Studio 多状态 token |
| CTA | 普通按钮 | 底部固定、金额明确、状态稳定 |
| 排行榜 | 独立 section | 行动型排名条 + sheet |
| 弹幕 | section 内列表 | 视频 overlay，可隐藏，不遮挡 |
| PC | 后台流式布局 | 三栏运营控制台 |
| 诊断 | tab + table | count/status tab + drawer + 建议动作 |
| 动效 | toast cue | 事件驱动 motion language |

## 结论

UI/UX 重构的目标不是把页面“装饰得更炫”，而是把直播竞拍的复杂信息压缩成清晰、漂亮、可信、可操作的界面。最终效果应该让用户愿意停下、敢于出价、知道自己为什么赢/输；让主播知道如何控场；让商家能安全运营；让评委看到这不是 demo UI，而是一套可扩展的产品级设计系统。
