# Experience System Design

## Design Goal

The experience should feel like a live auction room where the user can understand:

- what is happening now;
- how intense the room is;
- whether they are winning, losing, or close;
- why the countdown changed;
- what happened at the hammer moment.

Intensity comes from real auction state, not fabricated pressure.

## P0 Experience Slice

### 1. Trust cleanup

Replace or remove:

- hardcoded viewer count;
- static rank/lot/hype labels not backed by data;
- dead follow/like/gift/more buttons;
- unconditional deposit or guarantee chips.

Acceptance:

- no rendered number implies real viewers unless it is sourced from backend or explicitly labeled as active bidders/bids;
- every clickable control changes local state, calls an API, opens a real panel, or is absent.

### 2. Final-countdown tension

Use the existing server-time-derived countdown as the source. Add a derived phase:

| Phase | Remaining | UI behavior |
|---|---:|---|
| `normal` | >10s | Current compact countdown. |
| `hot` | <=10s | Price and timer gain subtle pulse; heat meter becomes prominent. |
| `critical` | <=5s | Stronger timer color, short heartbeat tone if sound enabled, haptic single pulses. |
| `hammer-window` | <=3s | "第一次 / 第二次 / 第三次" or "3 / 2 / 1" beat markers, no local winner declaration. |
| `syncing` | <=0 before terminal event | Current honest "到点同步中"; no celebration until server event arrives. |

Suppression:

- disconnected, recovering, stale snapshot, engine paused, reconciling, cancelled, ended, or reduced-motion mode.

Implementation:

- derive numeric remaining milliseconds separately from display copy;
- set `data-countdown-phase` on the stage/bid dock;
- animate only `transform`, `opacity`, and color variables;
- sound and haptics trigger at phase boundaries, not every 100ms tick.

### 3. Heat meter

Render a compact, honest room heat component:

- active bidders in last 30s;
- accepted bids in last 30s;
- price velocity per minute;
- total accepted bid count if available;
- "watchers unavailable" should never be replaced by a fake number.

Suggested copy:

- `近30秒 12 人出价 · 27 口 · ¥1,240/分`
- fallback: `近30秒暂无有效出价`

Data source:

- prefer leaderboard payload fields already typed in `frontend/mobile-h5/src/domain.ts`;
- otherwise add bidder-safe aggregate fields to a room/auction endpoint.

### 4. Action rail

P0 behavior:

- product list opens existing product sheet;
- like button emits a local capped heart burst and increments a local session count, clearly not a business fact;
- gift button is removed unless a real non-payment demo action exists;
- follow button toggles local follow state or is removed;
- more button opens a small sheet with sound, motion, and report controls.

Do not introduce real payment/tipping unless the backend order/payment scope is intentionally expanded.

## P1 Experience Slice

### Leaderboard 2.0

Add:

- `bid_count` per row;
- "榜一/榜二/榜三" labels;
- current user gap and next valid bid;
- FLIP row transition animation for rank changes;
- event-authoritative outbid cue that does not wait for REST refresh.

Longer-term:

- move leaderboard updates to WebSocket delta or include enough leaderboard state in bid events to avoid stale cue logic.

### Hammer ceremony

Server terminal event starts the ceremony:

- winner: spotlight on item, confetti canvas, hammer tone, result card with item image, price, winner state, defeated bidder count if real, and payment CTA after the ceremony is visible;
- loser: respectful close, winning price, gap if available, next auction CTA;
- unsold: calm close with "无人有效出价" or backend reason;
- cancelled: no celebration; show host/system reason.

The ceremony must never start from local countdown reaching zero.

### Sound design

Sound is opt-in and independently mutable:

- small accepted/leading/outbid tones;
- heartbeat or metronome only in critical countdown;
- hammer tone on terminal sold;
- no autoplay and no sound during recovery/stale state.

### Barrage/chat motion

Turn stage chat into actual movement only when:

- there is bounded message volume;
- reduced-motion is off;
- slow device/performance budget is respected.

System commentary and AI commentary should use a distinct style and provenance label.

## P2 Experience Slice

These are optional after P0/P1 is stable:

- warm-up zero-price micro auction or lucky draw, only if rules and compliance are explicit;
- buyer-side PK progress bar for two lots or two teams;
- special entrance/leader effects with anti-shill and minor-protection gates;
- shareable highlight card or short recap.

## Performance Budget

Mobile H5 budget for atmosphere features:

- no layout-triggering animation in hot paths;
- no unbounded particles;
- canvas/rAF effects capped and paused when hidden;
- avoid root app 10Hz re-render for all components;
- P95 interaction-to-next-paint should stay smooth under local mobile emulation before claiming the feature is demo-ready.
