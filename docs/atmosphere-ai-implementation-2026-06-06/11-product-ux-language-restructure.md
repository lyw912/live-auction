# Product UX Language And Information Architecture Refactor

Date: 2026-06-06

## Problem

The previous H5 and PC surfaces had many implemented capabilities, but the product semantics were weak:

- H5 placed warm-up tasks, 福袋, buyer PK, entry effects, bid controls, rules, leaderboard, orders, Q&A, and settings too close together.
- Buyer-facing surfaces exposed implementation words such as `WebSocket`, `seq`, `Max Bid`, `ORDER`, and raw status labels.
- PC merchant surfaces mixed workbench language with engineering/ops language, making it harder for a merchant or主播 to know what to do first.
- Visual treatment leaned toward repeated white cards, which made unrelated actions feel equal and blurred the boundary between conversion, trust, activity, and risk.

## Research Notes

Current live-commerce/mobile-checkout guidance converges on the same product pattern:

- Keep the live content persistent and use the bottom product/checkout area for the primary conversion action.
- Keep price/order summary visible, then expand details through bottom sheets or drawers when the user asks for them.
- Remove distractions around the primary transaction moment, especially on mobile.
- Use explicit next-step buttons and clear milestones rather than ambiguous links or technical state labels.
- Preserve trust context near payment or bidding decisions, but avoid crowding the primary action area.

Sources added to `08-research-source-index.md` include Nielsen Norman Group livestream ecommerce guidance, mobile checkout guidance, and checkout/product-card pattern references.

## Implemented IA

### H5 Buyer

First screen now prioritizes:

- live stage and current product identity;
- current price;
- countdown;
- real heat;
- one primary entry into bidding;
- small action rail for product list, interaction, like, and settings.

Moved out of the first screen or bid-panel primary path:

- warm-up tasks;
- 福袋;
- buyer PK;
- entry/leader effect card.
- secondary history/order/Q&A actions.

Those now live in bottom sheets, with `更多` acting as the compact navigation hub for Q&A, interaction benefits, bid history, and orders. The bid panel keeps four buyer-relevant shortcuts only: `拍品与规则`, `出价榜`, `自动加价`, and `更多`. All entries remain wired to real APIs or existing interactive sheets.

### H5 Language

Buyer-facing replacements:

- `WebSocket 已连接 · 状态来自服务端事件` -> `已同步`
- `seq ...` -> `刚刚更新`
- `Max Bid` -> `自动加价`
- `ORDER` -> `查看订单`
- raw `ACTIVE/SOLD/...` -> `竞拍中/已成交/...`
- `AI` in buyer chat/barrage -> `助手`
- browser titles now use `直播竞拍` and `商家直播竞拍台`
- live-room label no longer exposes `room_main`; it reads `竞拍专场` or a readable room name
- floating product card CTA now says `进入竞拍`, `查看结果`, or `看拍品信息` according to auction state

### PC Merchant

Merchant-facing replacements:

- `Live Auction` -> `直播竞拍台`
- `DRAFT/SCHEDULED/ACTIVE/FINISHED` -> `待完善/已排期/开拍中/已结束`
- `Live Assist` -> `直播助手`
- `Max Bid readiness` -> `自动加价概况`
- `Heat 30s` -> `近30秒热度`
- `Risk queue` -> `风险待处理`
- `Flight recorder` on workbench actions -> `事件回放`
- `room_main/room_side` on the merchant workbench -> `主直播间/副直播间`
- `auc_*` in primary merchant copy -> the拍品名称
- provider/model/status codes in listing draft review -> `草稿已生成/生成失败/生成中`
- `图片 URL` -> `图片地址`
- `直播健康` -> `风险处理`
- `API`/raw queue wording in diagnostics header -> `运维排查信息`, `待同步`, `待执行`

Diagnostics and audit drawers may still show low-level fields and event names because those views are explicitly for ops/debugging.

## Visual Direction

The refactor avoids making every module a same-looking white card:

- Primary bidding action uses a stronger red/orange conversion treatment.
- Interaction benefits use warmer campaign colors.
- Buyer PK uses blue participation styling.
- Trust/proof content uses calmer green/neutral treatment.
- Automatic bidding uses blue/green trust styling to distinguish it from direct bidding.
- Bottom sheets use a light grey surface with content-specific panels rather than a stack of identical white cards.

## Evidence

- `pnpm --filter mobile-h5 build`
- `pnpm --filter pc-console build`
- `H5_PORT=5276 PC_PORT=5277 PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 pnpm exec playwright test tests/e2e/mobile-h5.spec.ts tests/e2e/pc-console.spec.ts --project=mobile-h5 --project=pc-console --reporter=line -g 'feed product card|AI|recap|leaderboard'`
- 2026-06-06 product-language retest: `H5_PORT=5318 PC_PORT=5319 PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 pnpm exec playwright test tests/e2e/mobile-h5.spec.ts tests/e2e/pc-console.spec.ts --project=mobile-h5 --project=pc-console --reporter=line -g 'leaderboard sheet|auction queue pins'`
- Playwright MCP visual checks confirmed H5 title `直播竞拍`, PC title `商家直播竞拍台`, H5 bid panel shortcuts reduced to four, and PC workbench top summary shows `主直播间` plus拍品名称 instead of `room_main/auc_live`.

All passed on 2026-06-06 after the refactor.

## Remaining Guardrail

This refactor does not remove diagnostics language from diagnostics/audit tools. It draws a product boundary:

- buyer and merchant workbench surfaces must use business language;
- ops diagnostics may expose internal fields when they are needed for incident investigation or judge defense.
