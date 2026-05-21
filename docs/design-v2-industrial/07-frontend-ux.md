# 07 · Frontend UX

## Frontend Principles

- UI never decides winner, close, or payment truth.
- No optimistic success for bid.
- Dangerous CTA disabled during pending/recovering/disconnected.
- Every realtime visual effect is bound to server event.
- H5 must make stale/recovering state visible.
- PC diagnostic panels must be real, not mock.

## PC Console Screens

### Item Create

Fields:

- title
- description
- image

Upload flow:

1. request upload URL.
2. PUT to MinIO.
3. create item with `item_id_pending`.

### Auction Rule Form

Fields:

- start price
- increment
- cap
- duration
- start time
- extension window
- extension by
- max extension count
- fat-finger threshold
- deposit rule

Validation:

- frontend mirrors rule matrix.
- backend is final authority.
- cap input shows nearest legal caps.

### Room Auction List

Columns:

- item image/title
- status
- narrating badge
- current price
- winner masked
- start/end time
- accepted_bid_count
- actions

Rules:

- only one ACTIVE row.
- if ACTIVE exists, narrating buttons on other rows disabled.

### Auction Control

Display:

- current price
- leader
- end countdown from server snapshot
- bid count / participants approximate
- extension count
- connection state
- recent events

Actions:

- start
- cancel with reason
- narrate start/stop

### Orders

Display:

- auction/item
- winner masked
- amount
- order status
- deposit status
- expire_at/paid_at

### Diagnostics

Panels:

- active auctions
- recent rejects
- outbox delivery
- scheduler jobs
- anomalies
- reconnect/snapshot source mix

Every row must link to source data.

## H5 Room

### Layout

- simulated live video area
- chat/social overlay
- current focus item
- auction status strip
- price and leader
- countdown
- bid input/stepper
- primary CTA
- item detail half-sheet
- result/order section

### State Matrix

| State | Price | CTA | Feedback |
|---|---|---|---|
| SCHEDULED | start price | disabled/remind | start countdown |
| ACTIVE no bids | start price | bid start+increment | rule hint |
| ACTIVE with bids | current price | bid next increment | leader/rank |
| self leading | current price | disabled | already leading |
| pending | last authoritative price | loading disabled | waiting server |
| rejected | authoritative price | enabled | reason copy |
| extended | current price | enabled | extension toast |
| recovering | stale marked | disabled | syncing |
| disconnected | stale marked | disabled | reconnecting |
| SOLD winner | sold price | pay mock | expire countdown |
| SOLD loser | sold price | disabled | winner masked |
| ENDED no winner | start/current | disabled | ended |
| CANCELLED | current/start | disabled | cancel reason |

### Bid UX

- Bid amount defaults to minimum valid next amount.
- Stepper moves by increment.
- Large jump opens confirm flow after server returns token.
- Pending state blocks repeat click except idempotent retry logic.
- Rate limited state disables CTA for `Retry-After`.

### Recovery UX

- Last known price is shown with stale marker.
- During recovering, CTA disabled.
- Snapshot applied removes stale marker.
- If server returns stale snapshot, keep recovering state and retry after delay.

### Chat And Social

- Chat max 80 chars.
- Rate limit feedback is soft.
- Chat loss is acceptable; do not show scary errors.
- `user_joined` appears as lightweight toast, not critical event.

### Atmosphere Effects

Allowed:

- price tick on accepted bid;
- flash on outbid;
- extension pulse;
- hammer/result animation;
- chat overlay;
- user joined toast.

Forbidden:

- animation that blocks input;
- fake bids;
- UI effect that implies success before server response;
- full-screen effects hiding CTA during ACTIVE.

## Mobile Performance

Use lightweight CSS transforms. For test builds:

- PerformanceObserver longtask sampling.
- Playwright checks no overlap/CTA blocked.
- low-end devices can disable animations.

## Accessibility And Copy

Error copy must map to business action:

| Code | H5 Copy |
|---|---|
| BID_TOO_LOW | 出价低于最低可出价 |
| BID_INCREMENT_MISMATCH | 请按加价幅度出价 |
| REJECTED_SELF_LEADING | 你已领先，无需重复出价 |
| BID_AUCTION_TOO_HOT | 竞价激烈，请稍候 |
| PROCESSING_RETRY_LATER | 正在确认上一笔出价 |
| IDEMPOTENCY_TIMEOUT | 操作未确认，请重新出价 |
| AUCTION_ENDED | 竞拍已结束，正在同步结果 |
| FORBIDDEN_ROOM | 无法进入该直播间 |

## Frontend Tests

P0 Playwright:

- H5 state matrix screenshot coverage.
- bid pending/rejected/accepted.
- recovery disables CTA.
- cap sold result.
- payment double click.
- narrating/ACTIVE focus.
- PC rule validation and cap suggestions.
- diagnostic page with real anomaly.
