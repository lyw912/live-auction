# P3-08 Redis Lua Borrowed Hardening Evidence

Date: 2026-05-25 Asia/Shanghai

Commit: pending at evidence creation

## Claim

The project selectively borrowed Redis Lua scripting discipline without moving auction money truth into Redis.

Implemented borrowings:

- shared Redis Lua script runner with stable script names;
- `EVALSHA`-first execution through go-redis `Script.Run`, with `EVAL` fallback on `NOSCRIPT`;
- script-level metrics: `redis_lua_script_total{script,outcome}` and `redis_lua_script_latency_seconds{script,outcome}`;
- script error classification for timeout, BUSY, NOSCRIPT, unavailable, missing, and generic errors;
- hash-tagged bid admission keys: `bid:{auction}:limit:user:{user}`, `bid:{auction}:limit:ip:{ip}`, and `bid:{auction}:limit:auction`;
- one-time WS ticket key helper preserved as `ws_ticket:{ticket}`;
- focused tests for key conventions, error classification, admission metrics, and ticket metrics.

## Files

- `backend/internal/redisx/scripts.go`
- `backend/internal/redisx/keys.go`
- `backend/internal/redisx/scripts_test.go`
- `backend/internal/gateway/bid_admission.go`
- `backend/internal/gateway/bid_admission_integration_test.go`
- `backend/internal/realtime/ticket.go`
- `backend/internal/realtime/ticket_test.go`
- `docs/design-v2-industrial/04-data-and-storage.md`
- `docs/reviews/p3-08-redis-lua-borrowing-review-2026-05-25.md`

## Verification

Commands:

```text
go test ./internal/redisx ./internal/gateway ./internal/realtime ./internal/auction
git diff --check
```

Result:

```text
go test ./internal/redisx ./internal/gateway ./internal/realtime ./internal/auction PASS
git diff --check PASS
go test ./... PASS
```

Focused tests covering the borrowing:

- `backend/internal/redisx/scripts_test.go`
  - error classes map Redis/script failures to stable observability outcomes;
  - bid admission keys carry the `{auction}` hash tag needed for future multi-key cluster-safe scripts.
- `backend/internal/gateway/bid_admission_integration_test.go`
  - completed idempotency replay still bypasses Redis limiter;
  - Redis-down admission still fails open and records anomaly;
  - user limit still returns `RATE_LIMITED`;
  - Redis Lua admission metrics are emitted for allowed and rejected outcomes.
- `backend/internal/realtime/ticket_test.go`
  - ticket consume remains one-time;
  - `ws_ticket_consume` emits consumed/missing metrics.

## Known Limits

- Redis still does not decide winner, price, cap, cancel, end, order, auction seq, or idempotency response.
- The three admission checks remain separate single-key Lua calls. This is intentional because admission is protective, not money truth. A strict combined limiter would require a same-slot multi-key script ADR.
- No Redis Lua reservation path is implemented.
- No Redis Lua performance improvement is claimed. Reservation remains gated by `docs/p3-decision-log.md` P3-D14.
