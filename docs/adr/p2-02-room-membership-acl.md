# ADR P2-02 · Room Membership And Host Ownership ACL

Date: 2026-05-23 Asia/Shanghai

Status: Accepted

## Context

P2-01 made identity session-backed, but room access was still inferred from request paths and auction-room validation. A user with a valid session could call bid/chat/ws-ticket on any known room or auction unless the handler explicitly checked ownership or membership.

## Decision

- Add `room_memberships(room_id, user_id, role, status, joined_at, left_at)`.
- Host mutations require `rooms.host_id = current_user` and an open room.
- Viewer room list, chat, bid, confirm bid, auction detail, and WS ticket require an active room membership.
- Banned/blocked users are represented as membership rows with `role='blocked'` or `status='BANNED'`, and are rejected by active membership checks.
- ACL checks live in gateway-level `roomACL`; money correctness transactions remain unchanged.
- ACL rejects write real `ACL_FORBIDDEN` anomalies with room, auction, user, role, reason, and trace id.

## Consequences

- P2-03 can build real multi-room UX on top of an existing access model.
- Existing tests and seeds must explicitly create membership for synthetic rooms.
- WS ticket issuance now checks both auction-room relation and user membership before issuing one-time ticket.

## Follow-Up Gates

- P2-03 should add room selector and two-room E2E using this ACL model.
- P2-06 should add PC filtering/drilldown for ACL anomalies.
- Banned-user ticket revocation after issuance is still a P2/P3 realtime revocation-window problem; current gate blocks new ticket issuance.
