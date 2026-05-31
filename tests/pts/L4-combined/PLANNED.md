# L4 Combined Production — Planned

> Status: PLANNED. Not yet implemented. See `tests/pts/MANIFEST.md` for the full framework.

## Purpose

L4 is the production-readiness gate. All traffic types run simultaneously: bid pressure,
WebSocket fanout, read traffic, background settlement writes, and concurrent auctions.
This is the test that tells you the system is safe to take live.

Run L4 only after L1 + L2 + L3 all pass individually. A regression in L4 that does not
appear in L1–L3 points to an emergent interaction at the full-system integration boundary.

## Workloads

### L4-M1 — Full mixed workload

| Traffic type | VU count | Protocol |
|---|---|---|
| Hot auction bidders | 1000 | HTTP POST /api/rooms/{room}/auctions/{auc}/bids |
| WS viewers (hot auction) | 1000 | WebSocket |
| Read traffic | 3000 | HTTP GET (snapshot, leaderboard, bid history) |
| Side auction bidders | 200 | HTTP POST |
| Background settlement poll | 50 | HTTP GET /api/monitor/* (host role) |
| **Total VUs** | **~5250** | |

| SLA | Target |
|---|---|
| Hot bid p99 | ≤ 65ms |
| Read p99 | ≤ 300ms |
| WS delivery lag | ≤ 500ms |
| Settlement complete | within 10s of auction close |
| Zero DLQ entries | required |

Duration: 10 min sustained after 2-min ramp.

JMX file (to be created): `tests/pts/L4-combined/pts-4m1-full-mixed.jmx`

## Attribution Protocol

When L4-M1 p99 regresses vs L1-C1:
1. Check `auction_bid_decision_latency_seconds` histogram in `/metrics` — is the engine itself slow?
2. Check `outbox_lag_seconds` — is relay backpressure reaching the bid path?
3. Check `runtime.NumGoroutine` trend — WS goroutine accumulation?
4. Check PostgreSQL `pg_stat_activity` for lock waits — read/write DB contention?
5. Isolate the offending layer by re-running the matching L2 sub-test with L4 VU counts.

## Prerequisites

- L3-S1 and L3-S2 must pass.
- L4 requires a larger session pool: bidder + viewer + host sessions combined.
- Run on ECS or a machine matching production spec — do not run L4 on a dev laptop.
- Follow `docs/current/evidence-policy.md` for all L4 evidence classification.
