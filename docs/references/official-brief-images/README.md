# Official Brief Image References

Source: `抖音电商AI全栈课题-直播竞拍全栈系统（宣讲版）.md`.

The official markdown embeds six Lark-hosted PNG screenshots. They were downloaded locally so UI/UX and atmosphere planning can reference the actual visual intent, not only the text brief.

## Image Inventory

| File | Brief section | Observed content | Design implication |
|---|---|---|---|
| `official-brief-image-01.png` | 商家/主播端（PC 管理后台） | PC live product list with search/filter, add product, product thumbnail, tags, status badges, start price, increment, cap price, current bid/sold amount, bid count, auction countdown, narrate/cancel/off-shelf actions. | PC must support dense auction operations, product management, narrating state, bid count, current price, and clear status/action layout. |
| `official-brief-image-02.png` | 用户端（移动端 H5/小程序） | Live room with host header, viewer avatars, product list half sheet, item cards, statuses: bidding, soon to start, ended unsold, ended sold, cutoff/in progress. Copy changes depending on state: start price, current highest price, hammer price. | H5 must support live overlay + product list browsing, not only one active auction. Status copy must distinguish bidding/upcoming/ended/sold/cutoff. |
| `official-brief-image-03.png` | 用户端出价面板 | Half-screen bid sheet with countdown, item thumbnail, title, current price, leader, my bid, stepper, increment, primary bid CTA. | Bid UI should concentrate price, countdown, leader, my bid, next amount, and CTA in one compact action panel. |
| `official-brief-image-04.png` | 出价边界/提示 | Two bid panel variants: one-more-bid hint showing amount above current price, and self-leading hint showing current user already highest. | H5 needs state-specific bid hints: valid next step and self-leading guardrail. |
| `official-brief-image-05.png` | 竞拍结束未成交 | Live overlay with ended sheet, item info, final/current price, disabled bid CTA, “auto return to live room” countdown. | Ended/unsold state must be visually distinct, disabled, and explain the next navigation behavior. |
| `official-brief-image-06.png` | 成交/落槌结果 | Winner payment modal with product, price, deposit return copy, pay CTA, payment countdown; sold result modal with winner masked, rounds count, final price; live chat and mini product card remain visible behind modal. | Result UX should preserve live context, show winner/loser outcome, payment countdown, deposit copy, and mini next-item card. |

## Usage Rule

These images are official visual references, not final design constraints. The project should meet or exceed the implied behaviors while preserving v2 engineering rules:

- no fake bids or fake heat;
- server-authoritative price, winner, countdown, and terminal state;
- no CTA hidden by decorative effects;
- weak-network and recovery states remain explicit;
- product image/detail trust matters more than decorative gradients.
