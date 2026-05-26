# 05 · API Contracts

## Common

Headers:

```text
X-Request-Id: optional client trace
Idempotency-Key: required for bid/payment/confirm where specified
```

Error shape:

```json
{
  "code": "BID_TOO_LOW",
  "message": "bid amount is below minimum",
  "trace_id": "tr_...",
  "details": {}
}
```

Server always derives user from session/mock token.

## PC APIs

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/items/upload-url` | MinIO presigned PUT URL |
| POST | `/api/items` | create item after upload |
| POST | `/api/auctions` | create DRAFT auction and rules |
| PATCH | `/api/auctions/{id}/rules` | update DRAFT rules |
| POST | `/api/auctions/{id}/schedule` | freeze rules and schedule |
| POST | `/api/auctions/{id}/unschedule` | return SCHEDULED to DRAFT |
| POST | `/api/auctions/{id}/start` | manual start |
| POST | `/api/auctions/{id}/cancel` | abnormal cancel |
| POST | `/api/auctions/{id}/narrate-start` | set focus |
| POST | `/api/auctions/{id}/narrate-stop` | clear focus |
| GET | `/api/host/auctions/{id}/prompts` | host-only advisory prompter suggestions from real auction data |
| GET | `/api/host/auctions/{id}/max-bid-summary` | host-only Max Bid/Pre-Bid readiness aggregate without private max amounts |
| GET | `/api/auctions` | list |
| GET | `/api/auctions/{id}` | detail/snapshot |
| GET | `/api/orders` | order list |
| GET | `/api/monitor/auctions` | diagnostic active auctions |
| GET | `/api/monitor/anomalies` | anomalies |
| GET | `/api/monitor/outbox` | outbox delivery |
| GET | `/api/monitor/outbox/watermarks` | outbox relay shard watermarks |
| GET | `/api/monitor/scheduler` | scheduler jobs |
| GET | `/api/monitor/snapshots` | snapshot rebuild lifecycle |
| GET | `/api/monitor/signals` | operator control signals |
| POST | `/api/monitor/signals` | create host-only operator control signal |

## H5 APIs

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/rooms/{room_id}/auctions` | room item/auction list |
| GET | `/api/auctions/{id}` | authoritative snapshot |
| POST | `/api/auctions/{id}/bids` | bid |
| POST | `/api/auctions/{id}/bids/confirm` | fat-finger confirm |
| GET | `/api/auctions/{id}/max-bid-intent` | read current user's private max bid intent |
| PUT | `/api/auctions/{id}/max-bid-intent` | create/update current user's private max bid intent |
| DELETE | `/api/auctions/{id}/max-bid-intent` | cancel current user's private max bid intent |
| GET | `/api/users/me/bids` | bid history |
| GET | `/api/users/me/orders` | order history |
| POST | `/api/orders/{id}/pay-mock` | mock payment |
| POST | `/api/rooms/{room_id}/chat` | send chat |
| GET | `/api/rooms/{room_id}/chat?limit=30` | seed chat |
| POST | `/api/auth/ws-ticket` | one-time WS ticket |

## Bid Request

```json
{
  "client_bid_id": "uuid",
  "amount_cents": 90000,
  "client_seen_seq": 41
}
```

`Idempotency-Key` must equal `client_bid_id`.

Response:

```json
{
  "result": "ACCEPTED_EXTENDED",
  "bid_id": "b_123",
  "auction_id": "a_123",
  "seq": 42,
  "current_price_cents": 90000,
  "current_winner_id": "u_1",
  "end_at": "2026-05-22T13:00:30Z",
  "server_time_ms": 1779435630000,
  "reject_reason": null
}
```

Fat-finger response:

```json
{
  "result": "FAT_FINGER_CONFIRM_REQUIRED",
  "confirm_token": "ft_...",
  "expires_in_ms": 30000,
  "current_price_cents": 10000,
  "amount_cents": 500000
}
```

## Confirm Bid

```json
{
  "confirm_token": "ft_...",
  "idempotency_key": "uuid"
}
```

The server resolves original amount from token and runs normal bid pipeline.

## Max Bid Intent

Create/update:

```text
PUT /api/auctions/{id}/max-bid-intent
Idempotency-Key: <uuid>
```

```json
{
  "max_amount_cents": 90000,
  "client_seen_seq": 41,
  "source": "MAX_BID"
}
```

Response:

```json
{
  "result": "ACTIVE",
  "intent": {
    "id": "mbi_123",
    "auction_id": "a_123",
    "user_id": "u_1",
    "max_amount_cents": 90000,
    "status": "ACTIVE",
    "source": "MAX_BID",
    "version": 0
  }
}
```

Cancel:

```text
DELETE /api/auctions/{id}/max-bid-intent
Idempotency-Key: <uuid>
```

Response result is `CANCELLED`.

`GET /api/auctions/{id}/max-bid-intent` returns only the authenticated user's private intent. Hosts and other bidders do not get another user's max amount through this endpoint.

`GET /api/auctions/{id}` is the recovery snapshot for H5. It may include the authenticated user's own private intent as:

```json
{
  "max_bid_intent": {
    "id": "mbi_123",
    "auction_id": "a_123",
    "user_id": "u_1",
    "max_amount_cents": 90000,
    "status": "ACTIVE",
    "source": "MAX_BID",
    "last_applied_seq": 42,
    "version": 2
  }
}
```

This field is user-scoped and must not appear in auction lists, public WebSocket events, Redis public snapshots, or another user's `GET /api/auctions/{id}` response.

Host aggregate:

```text
GET /api/host/auctions/{id}/max-bid-summary
```

Response:

```json
{
  "auction_id": "a_123",
  "room_id": "r_1",
  "status": "ACTIVE",
  "generated_at": "2026-05-27T04:00:00Z",
  "active_intent_count": 3,
  "pre_bid_count": 1,
  "max_bid_count": 2,
  "applied_intent_count": 1,
  "exhausted_count": 0,
  "cancelled_count": 0,
  "has_private_pressure": true,
  "source": "postgres:max_bid_intents"
}
```

This endpoint is for PC host readiness and audit navigation only. It must not return `max_amount_cents`, per-user private ceilings, bidder ranking by ceiling, or another user's private intent object. Automatic bid rows remain inspectable through the host-only flight recorder as ordinary bid rows with `payload.source = "AUTO_MAX_BID"`.

## Payment Mock

```text
POST /api/orders/{id}/pay-mock
Idempotency-Key: <uuid>
```

```json
{ "confirm": true }
```

Response:

```json
{
  "order_id": "o_1",
  "order_status": "PAID",
  "paid_at": "2026-05-22T13:10:00Z",
  "deposit_status": "REFUNDED"
}
```

## WebSocket

Ticket:

```text
POST /api/auth/ws-ticket
body: { "room_id": "r_1", "auction_id": "a_1" }
```

Connect:

```js
new WebSocket(
  "/ws?room_id=r_1&auction_id=a_1&last_seq=41",
  ["auction.v1", "ticket.<base64url>"]
)
```

Server accepts subprotocol `auction.v1` only.

Authoritative event:

```json
{
  "room_id": "r_1",
  "auction_id": "a_1",
  "seq": 42,
  "state_version": 17,
  "event_type": "bid_accepted",
  "server_time_ms": 1779435630000,
  "payload": {
    "current_price_cents": 90000,
    "leader_user_masked": "张**"
  }
}
```

Snapshot:

```json
{
  "event_type": "snapshot",
  "auction_id": "a_1",
  "seq": 42,
  "source": "db",
  "stale": false,
  "snapshot_age_ms": 0,
  "payload": {}
}
```

Gap:

```json
{
  "event_type": "outbox_gap_notice",
  "auction_id": "a_1",
  "missing_seq": [43],
  "server_time_ms": 1779435630000
}
```

## Error Codes

| Code | Meaning |
|---|---|
| INVALID_ARGUMENT | schema/range violation |
| UNAUTHORIZED | missing/invalid auth |
| FORBIDDEN_ROOM | user cannot access room |
| AUCTION_NOT_FOUND | not found |
| AUCTION_NOT_ACTIVE | not active |
| AUCTION_ENDED | server time past end |
| BID_TOO_LOW | below minimum |
| BID_INCREMENT_MISMATCH | not on increment grid |
| BID_ABOVE_CAP | above cap |
| MAX_BID_TOO_LOW | max bid below current executable minimum |
| MAX_BID_INCREMENT_MISMATCH | max bid not on increment grid |
| MAX_BID_ABOVE_CAP | max bid above cap |
| REJECTED_SELF_LEADING | current winner bids again |
| FAT_FINGER_CONFIRM_REQUIRED | need confirm token |
| RATE_LIMITED | per-user/IP limit |
| BID_AUCTION_TOO_HOT | auction global limit |
| BID_RETRY_LATER | local concurrency or lock timeout |
| IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST | key/hash mismatch |
| PROCESSING_RETRY_LATER | same key currently executing |
| IDEMPOTENCY_TIMEOUT | unknown result exceeded budget |
| IDEMPOTENCY_MAX_RETRIES_EXCEEDED | retryable state exceeded max |
| INVALID_AUCTION_RULE | generic invalid rule |
| INVALID_AUCTION_RULE_CAP_UNREACHABLE | cap not aligned with increment |
| RULE_FROZEN_AFTER_SCHEDULED | cannot edit after schedule |
| CONFLICT_ROOM_HAS_ACTIVE_AUCTION | one active per room |
| CONFLICT_ROOM_HAS_NARRATION | one narrating per room |
| INVALID_NARRATE_TARGET | active/focus conflict |
| ORDER_ALREADY_EXPIRED | payment after expiry |
| CONFIRM_USED | confirm token reused |
| SLOW_CONSUMER | WS close reason |

## Idempotency

Bid key:

```text
scope_type = bid
scope_id = auction_id
user_id = current user
idempotency_key = client_bid_id
request_hash = sha256("bid:v1|auction_id|user_id|client_bid_id|amount_cents")
```

Payment key:

```text
scope_type = payment
scope_id = order_id
user_id = winner_id
idempotency_key = Idempotency-Key
request_hash = sha256("payment:v1|order_id|user_id|idempotency_key")
```

Max bid intent key:

```text
scope_type = max_bid_intent
scope_id = auction_id
user_id = current user
idempotency_key = Idempotency-Key
request_hash = sha256("max-bid-intent:v1|auction_id|user_id|idempotency_key|max_amount_cents")
```

Cancel intent key:

```text
scope_type = max_bid_intent
scope_id = auction_id
user_id = current user
idempotency_key = Idempotency-Key
request_hash = sha256("max-bid-intent-cancel:v1|auction_id|user_id|idempotency_key")
```

Validation failures before executable section are not persisted. Any result after auction lock is persisted.
