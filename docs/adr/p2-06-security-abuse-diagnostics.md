# ADR P2-06 · Security And Abuse Diagnostics

Date: 2026-05-24 Asia/Shanghai

Status: Accepted

## Context

P2-01 through P2-05 added real session, ACL, bid admission, and payment provider failure modes. Some producers already existed, but operators still needed one filtered surface to answer which room, user, auction, trace, and anomaly type caused a failure.

## Decision

- Add `AUTH_SESSION_EXPIRED` anomaly producer in auth middleware for invalid, revoked, or expired sessions.
- Keep existing producers for `ACL_FORBIDDEN`, `RATE_LIMIT_REDIS_DOWN`, `RATE_LIMITED`, `BID_AUCTION_TOO_HOT`, `PAYMENT_WEBHOOK_INVALID_SIGNATURE`, and `PAYMENT_RECONCILE_MISMATCH`.
- Extend `GET /api/monitor/anomalies` with filters: `type`, `room_id`, `user_id`, `auction_id`, and `trace_id`.
- Keep filters backed by real anomaly rows and `payload_json`; do not add static security dashboard cards.
- Add PC monitor controls for anomaly type, auction id, user id, trace id, and current room context.

## Consequences

- Diagnostics can now narrow common auth/ACL/rate/payment failures without SQL access.
- AUTH failures without known user/room are still searchable by trace, remote IP in payload, and anomaly type.
- P2-06 does not add alerting rules; P1 alerting remains separate and P2-07/P5 can decide final alert thresholds.

## Follow-Up Gates

- P2-07 baseline should include raw evidence that anomaly filters work during abuse/payment scenarios.
- P5 release docs should include runbook snippets for the filtered anomaly workflow.
