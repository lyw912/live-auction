# P10 No-Mock Auction Demo

Date: 2026-05-27

## Purpose

P10 demo proves the auction product flow, not a scripted UI playback. The judge-facing trunk must use real backend APIs, PostgreSQL state, outbox events, Redis/WebSocket delivery, H5 state recovery, and PC diagnostics. Route-mocked Playwright tests remain useful for UI contract and visual regression, but they are not demo evidence for the live auction path.

## Allowed Demo Fixtures

- Product title, description, provenance copy, condition copy, and shipping copy may be sample content.
- Product image and looping product video may be local demo assets or downloaded assets stored in MinIO or served by the dev app.
- Demo host/user/room/session setup may be deterministic local setup. P10 does not add public registration, OAuth, SMS, or password-account management.
- Automated smoke may create a unique `room_p10_*` room through the host-only `/api/test/rooms` helper while `APP_ENV=test`; this is setup plumbing only, not a production API.
- Real live streaming is out of scope; a looping product video is the live-stage visual substitute.
- Real external payment is out of scope; the main no-mock trunk stops at SOLD/order creation. The existing local fake-provider payment path may be shown only as an explicitly labeled optional extension.

## Forbidden In The Main Demo Trunk

- No Playwright `page.route` or equivalent browser route interception.
- No frontend-local fake bid acceptance, fake SOLD, fake winner, or fake order.
- No route-mocked diagnostics or static dashboard rows.
- No pre-seeded ACTIVE auction presented as if it was created during the PC demo.
- No production capacity, payment, registration, or live-streaming claim without separate evidence.

## Main Trunk

1. Start local infrastructure: PostgreSQL, Redis, and MinIO.
2. Apply migrations.
3. Prepare only identity/room prerequisites if needed: demo host, demo bidder, room membership, and host ownership.
4. Start backend, H5, and PC console.
5. In PC console, select the demo room.
6. Create a product using sample product copy and a real image/video asset URL or upload.
7. Create a DRAFT auction for that product with real rules: start price, increment, cap, duration, extension window, extension by, max extensions, fat-finger threshold, and deposit rule.
8. Save or adjust DRAFT rules and show backend validation for invalid cap/increment if demonstrating guardrails.
9. Schedule and start the auction from PC.
10. Open H5 for the same room.
11. H5 first shows the live feed, chat, product media, and a floating product card with current price, countdown, status, connection, and next bid entry.
12. Tap the product card to open the full bidding panel. H5 loads `GET /api/rooms/{room_id}/auctions`, selects the backend-created auction, fetches the authoritative snapshot, obtains a WS ticket, and displays connected state.
13. Place a normal bid from H5. UI enters pending and updates only after the backend response or server event.
14. Place competing bids with another demo user or API client to show outbid, rank movement, authoritative bid hints, and event-driven atmosphere.
15. Place a final-window bid to trigger extension. H5 shows the server-provided old/new end time and extend count.
16. Trigger a reject branch, such as self-leading or increment mismatch, and show reason-specific copy with CTA behavior.
17. Drive the auction to cap/SOLD or let the scheduler end it, depending on the branch being demonstrated.
18. Show generated order rows after SOLD without claiming real external payment.
19. Open PC diagnostics and the auction flight recorder for the created auction. Show rules, bids, auction events, outbox delivery, order rows, snapshots, anomalies if any, and trace IDs.

## Optional Branches

- Cancel active auction: PC cancels with a reason; H5 receives terminal state; flight recorder shows the cancel event and outbox delivery.
- DRAFT rule edit: PC edits rules before schedule; invalid cap/increment is rejected by frontend guardrail and backend authority.
- Recovery: force reconnect or snapshot refresh; H5 marks stale/recovering and disables dangerous actions until a fresh snapshot applies.
- Extreme atmosphere: show sticky Bid Dock, rank strip, leaderboard sheet, official bid hints, extension pulse, sold mark, sound/haptic opt-in, and reduced-motion fallback.
- Local host demo driver: PC Live Assist exposes host-only buttons for reject, second-bidder outbid, extension-window bid, and cap SOLD. These buttons call `/api/demo/auctions/{id}/competing-bid`, which is restricted to `APP_ENV=local|test` and host ownership. It still writes through the real bid repository, auction events, outbox, and order path; it must be described as a local driver for another deterministic demo bidder, not as production product UI.
- Local fake-provider payment: use `/api/orders/{id}/pay-mock` only after saying it is a local provider boundary, not external payment.

## Manual Operation Map

PC host console:

1. Use the left nav `拍品` to open product creation. Enter title/description and either upload an image or use a URL such as `/demo/ceramic-tea-cup.jpg`.
2. Click `创建拍品和竞拍`. The new auction is DRAFT and appears in the queue.
3. Use the left nav `竞拍` or the `规则配置` tab to adjust DRAFT rules. Only DRAFT rules are editable.
4. In the main control panel click `排期`, then `开拍`. If another ACTIVE auction exists, cancel/end it first because the backend enforces one ACTIVE auction per room.
5. Use `取消` on DRAFT/SCHEDULED/ACTIVE auctions with a reason. Terminal auctions cannot be cancelled.
6. Use the queue to select lots. ACTIVE/SCHEDULED/DRAFT are prioritized; finished rows are capped to the latest few records so historical local runs do not dominate the demo.

H5 bidder room:

1. Open `/rooms/{room_id}`. The feed shows product media, chat, connection, price/countdown/status, and a floating product card.
2. Tap the product card to open the bid panel. Price, countdown, CTA, rank strip, Max Bid, leaderboard, rules, history, and orders are available there.
3. Bid from H5 to see pending then accepted/leading only after backend confirmation.
4. In PC Live Assist use `第二买家超越`, `触发 reject`, `窗口出价/延时`, and `封顶 SOLD` to drive the other-bidder branches while watching H5. H5 receives normal server responses/events and short non-blocking atmosphere animations.

## Evidence To Capture

- Exact commands and commit.
- Created `room_id`, `item_id`, and `auction_id`.
- H5 screenshots: connected active, pending, accepted/outbid, rejected, extended, SOLD/ENDED.
- PC screenshots: product/auction creation, rule validation, lifecycle controls, diagnostics, flight recorder.
- Backend logs for item creation, auction creation, schedule/start, bid/confirm, cancel/end/SOLD, outbox relay, WS ticket, and flight recorder.
- Optional DB query snippets for `auctions`, `bids`, `auction_events`, `outbox_events`, `orders`, and `anomalies`.
- Live smoke output proving no route mocks were used.

## Talking Points

- The product copy and media are demo content; the auction state is not.
- PostgreSQL decides price, winner, order, and terminal state.
- Redis/WebSocket deliver projections and realtime updates, but never decide auction truth.
- H5 never shows bid success before server confirmation and never hammers locally when countdown reaches zero.
- Diagnostics are backed by real persisted rows and outbox delivery state.
