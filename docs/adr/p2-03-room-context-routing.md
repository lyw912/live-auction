# ADR P2-03 · Room Context Routing

Date: 2026-05-24 Asia/Shanghai

Status: Accepted

## Context

P0/P1 UI and smoke scripts assumed `room_main`. After P2-01/P2-02, identity and room ACL are real enough that fixed room paths became the next demo shortcut: PC could not choose host rooms, and H5 could not enter a room route.

## Decision

- Add authenticated `GET /api/rooms` returning rooms accessible to the current session.
- PC console loads rooms and uses a room selector as the source for auction list and auction creation.
- H5 derives room context from `/rooms/{room_id}`, falling back to `room_main` only for root local demo entry.
- Demo seed now creates `room_main` and `room_side`, each with independent room context and chat; `room_side` has a separate draft auction.
- Mock E2E stubs auth and room APIs explicitly; live E2E uses real session and backend room data.

## Consequences

- `room_main` remains sample seed data, not the only UI path.
- P2-04/P2-07 load scripts can keep defaulting to `room_main`, but load seed now creates memberships for k6 users.
- Multi-room isolation can now be tested from product surfaces, not only raw API calls.

## Follow-Up Gates

- P2-04 should accept room/auction env vars for abuse workloads.
- P2-07 should include a multi-room workload using the same room context model.
