# Judge 3-Minute Demo Mode Plan

Date: 2026-06-07

Purpose: organize the existing product into a judge-facing 3-minute demo story. This document is a recording plan and future "judge demo mode" design note; it does not change the runtime product by itself.

## Core Principle

Do not show every feature. Show one complete business story:

> A merchant creates and starts a live auction, one buyer bids from H5, the merchant uses a host-only demo driver to simulate another real bidder, the auction heats up, extends, sells, then both buyer result and merchant recap are visible.

The judge should understand:

- who is using the system;
- what each side is trying to do;
- which actions are real backend actions;
- what result the system produces;
- where the technical depth is if they ask.

## 3-6 Core Capabilities To Submit

1. Merchant auction setup: create a lot, set start price, increment, cap, deposit, and final-window extension rules.
2. Buyer H5 bidding: enter the live room, inspect product/rules, submit a bid, and see server-confirmed state.
3. Real-time competition: leaderboard, countdown, outbid cues, extension, and atmosphere update from backend events.
4. Host demo driver: merchant console can simulate another bidder through real backend bid APIs, not frontend fake state.
5. Settlement and recap: SOLD creates buyer-facing result/order state and merchant-facing recap/highlight/diagnostic evidence.

Do not include 福袋, PK, full diagnostics, every AI panel, or payment extension in the main 3-minute trunk. Keep them as optional appendix material.

## End-To-End Flow

1. Merchant opens the PC workbench, selects the live room, and creates a new lot with product image, title, and description.
2. Merchant configures the auction rules and starts the lot.
3. Buyer opens H5, sees the live stage, product proof, current price, countdown, and enters the bidding panel.
4. Buyer submits a bid; H5 waits for backend confirmation before showing leading state.
5. Merchant uses the demo driver to create a competing bidder action; H5 updates to outbid/leaderboard/atmosphere states through server events.
6. Merchant triggers a final-window bid to show automatic extension and urgency copy.
7. Merchant triggers a cap/SOLD branch; H5 shows result/order state.
8. Merchant opens recap/highlight and event evidence to show that the demo produced real persisted records.

## Recommended Recording Layout

Use two browser windows only:

- Left: H5 buyer, mobile viewport.
- Right: PC merchant console, desktop viewport.

Do not use three H5 windows in the main video. It creates visual noise and makes the judge ask "which user am I watching?".

Use one real H5 buyer plus the PC host demo driver for the second buyer. Say this clearly:

> "The right-side demo driver represents another buyer, but it calls the same backend bid path and creates the same events as a normal H5 bidder."

This is less suspicious than a fully automated script because the merchant is visibly controlling the business scenario, while the resulting state appears independently on H5.

## 3-Minute Video Timeline

### 0:00-0:15 - What This Is

Screen: both windows visible, PC on `竞拍`, H5 on live room.

Voiceover:

> "This is a live auction system. Merchants control lots from the PC workbench, buyers bid from H5, and price, ranking, extension, order, and recap are all driven by backend state."

Show:

- PC title/room.
- H5 live stage.
- One visible product card.

Avoid:

- diagnostics tables;
- raw IDs;
- performance claims.

### 0:15-0:45 - Merchant Starts A Lot

Screen: PC focus.

Actions:

1. Show selected lot and rule summary.
2. Click `排期` if needed.
3. Click `开拍`.

Voiceover:

> "The merchant sets the auction rules before opening the lot: start price, increment, cap, deposit, and final-window extension. Once the lot starts, rules are frozen for buyer trust."

Must show:

- product name;
- current price;
- countdown;
- rule/freeze explanation.

Skip:

- creating a product from scratch if time is tight. A pre-created DRAFT lot is acceptable if disclosed as prepared demo content.

### 0:45-1:20 - Buyer Places First Bid

Screen: H5 focus.

Actions:

1. Tap floating product card.
2. Show `拍品与规则` very briefly if rules are important.
3. Submit one bid.

Voiceover:

> "The buyer sees product proof and bidding rules before placing a bid. The H5 client does not declare success locally; it waits for server confirmation."

Must show:

- bid button;
- pending/confirmed feedback;
- price or rank change.

Skip:

- Q&A;
- 福袋;
- sound toggle.

### 1:20-1:55 - Competition And Outbid

Screen: split view.

Actions:

1. PC clicks the host-only demo driver for "another buyer outbids".
2. H5 visibly changes to outbid/leaderboard update.
3. Open `出价榜` if it helps explain the ranking.

Voiceover:

> "To keep the video short, the merchant console has a demo driver for another bidder. It is not a fake frontend button: it calls the backend bid path, creates auction events, and the buyer side updates through realtime delivery."

Must show:

- PC action;
- H5 outbid/rank/price reaction;
- no manual H5 refresh.

### 1:55-2:25 - Final Window Extension And SOLD

Screen: split view.

Actions:

1. PC triggers final-window bid/extension.
2. H5 shows extension/urgency copy.
3. PC triggers cap/SOLD.
4. H5 shows result sheet.

Voiceover:

> "Final-window bids extend the auction by rule, preventing last-second sniping. When the cap or terminal condition is reached, the server emits the result and the buyer sees the outcome."

Must show:

- extension copy or countdown change;
- result state;
- order/result entry.

### 2:25-2:50 - Merchant Recap And Evidence

Screen: PC focus.

Actions:

1. Click recap/highlight.
2. Show generated recap card or WebM action.
3. Optionally open event replay for a few seconds.

Voiceover:

> "After the auction, the merchant gets a recap and highlight asset. If a judge wants proof, event replay shows the same bid, order, and delivery records behind the demo."

Must show:

- recap/highlight;
- one evidence view only.

Skip:

- all diagnostic tabs.

### 2:50-3:00 - Closing

Voiceover:

> "In three minutes, this demonstrates the complete loop: merchant setup, buyer bidding, real-time competition, rule-based extension, server-side result, and merchant recap."

## What Not To Show In The Main Video

- Full PTS/S1-S5 performance evidence. Mention that it is attached separately.
- Every AI capability.
- Full diagnostics table sweep.
- 福袋/PK/entry effects unless the main flow finishes early.
- Payment mock unless the rubric explicitly wants post-order payment behavior.
- Three or more browser windows.
- Pure script automation as the only evidence.

## Handling The "Only One User" Problem

Recommended approach:

- One human-operated H5 buyer window.
- One PC merchant window.
- PC host-only demo driver simulates the competing buyer.

Why this is acceptable:

- The demo driver is a scenario accelerator, not a frontend fake.
- It calls backend APIs and produces real bid/order/event rows.
- The H5 buyer window receives state through the normal realtime path.

How to explain it:

> "For recording, I use a host-side demo driver to represent the second buyer. In a full room, that second action would come from another H5 user; here it uses the same backend bid path so the resulting price, rank, extension, and order are still real."

If challenged:

- show event replay for the auction;
- show bid rows in diagnostics;
- run a second H5 window only as an appendix, not the main video.

## Future Judge Demo Mode Design

If implemented later, add a `评委演示` drawer in PC with a guided checklist:

1. `准备演示拍品`: create or select a clean DRAFT lot with Chinese product copy and image.
2. `开拍`: schedule/start the lot and show rule freeze.
3. `另一买家超越`: call existing demo competing-bid backend route.
4. `最后窗口延时`: create a final-window competing bid.
5. `封顶成交`: drive SOLD through backend path.
6. `生成复盘`: call recap/highlight and open result.

Guardrails:

- label the drawer "演示助手：用于录屏加速，所有动作写入真实竞拍链路";
- never mutate H5 local state directly;
- after each step, show "已完成/失败原因";
- include a reset/prepare command only if it creates a new demo lot instead of rewriting history;
- hide advanced options by default.

## Suggested Submission Text

### Core Function List

- 商家端创建拍品、设置竞拍规则并开拍。
- 买家端在 H5 查看拍品、规则和实时价格后出价。
- 服务端驱动实时榜单、倒计时、延时和出价气氛。
- 商家演示助手模拟另一买家竞争，走真实后端出价链路。
- 成交后生成买家结果、订单状态、商家复盘和事件证据。

### End-To-End Flow

商家先在 PC 端准备拍品和规则，开拍后买家从 H5 进入直播间查看商品和竞拍信息。买家提交出价后，页面等待服务端确认再显示领先状态。商家端用演示助手模拟另一位买家出价，H5 通过实时事件看到被超越和榜单变化。最后窗口出价触发规则延时，封顶或终态条件触发成交。成交后买家看到结果和订单入口，商家端生成复盘和高光，并可打开事件回放证明出价、成交和订单链路真实发生。

### Video Caption

> 左侧为买家 H5，右侧为商家工作台。为压缩录屏时长，右侧演示助手模拟另一位买家的竞争行为；该行为调用真实后端接口并产生真实竞拍事件。

## Recording Checklist

- Use Chinese demo product copy, not `P0 Live Smoke Item`.
- Start from a clean room or a clearly selected demo lot.
- Keep browser zoom at 90%-100%; avoid tiny text.
- Use side-by-side layout; H5 left, PC right.
- Turn off browser bookmarks/sidebar.
- Do not open DevTools.
- Narrate user intent before clicking.
- Show one diagnostic/evidence screen only near the end.
- Keep final video under 3 minutes 15 seconds.

## Open Questions Before Implementation

- Should the demo drawer create a fresh lot automatically, or only guide existing buttons?
- Should we add a visible "演示助手" label to current competing-bid controls to avoid judge confusion?
- Should H5 display a small "演示中" badge when backend is in local/test demo mode?
- Should the recording use a fixed Chinese product seed to avoid English sample names?
