# ADR Index

> Status: ADR classification index, 2026-05-31.

This directory is no longer a flat current-decision set. Read
`docs/current/README.md` before applying any ADR to hot bidding.

## Current Or Still Applicable

| ADR | Current use | Caveat |
|---|---|---|
| `p2-01-session-auth-boundary.md` | session identity boundary | still applicable |
| `p2-02-room-membership-acl.md` | room membership and host ownership ACL | still applicable; hot path may cache reads but must preserve ACL semantics |
| `p2-03-room-context-routing.md` | room routing and multi-room product context | still applicable |
| `p2-04-bid-admission-control.md` | product admission/abuse protection | not used to prove PTS-1B downstream capacity when disabled |
| `p2-05-payment-provider-boundary.md` | fake provider boundary and payment idempotency | still applicable |
| `p2-06-security-abuse-diagnostics.md` | anomaly/diagnostic producers | still applicable |
| `p2-07-release-baseline-harness.md` | release evidence discipline | capacity details superseded by current PTS evidence policy |
| `p3-03-rejected-bid-realtime-policy.md` | public realtime noise policy | apply with current `ENGINE_*` outcome semantics |
| `p9-04-max-bid-pre-bid-decision.md` | max bid/pre-bid product semantics | PostgreSQL-row-lock implementation assumptions are historical for PTS-1B hot manual bids |

## Historical Or Superseded For Hot Bidding

| ADR | Classification | Current use |
|---|---|---|
| `p3-00-local-stress-discipline.md` | historical baseline discipline | keep measurement discipline; use current PTS manifest for cloud runs |
| `p3-01-centrifugo-borrowing-decision.md` | historical borrowing review | realtime design background |
| `p3-02-debezium-borrowing-decision.md` | historical borrowing review | outbox/CDC design background |
| `pts-02-hotspot-bidding-engine-redesign.md` | superseded/historical | explains route evolution from PG lane to Redis/Kafka; current contract is in `docs/current/architecture.md` |

## Rules

- Do not cite an ADR as current PTS-1B proof.
- If an ADR says PostgreSQL is the only bid decision truth, reinterpret it as
  original-era context. Current hot manual bidding is Redis live decision state,
  Kafka durable decision WAL/fence, and PostgreSQL settlement/audit/order truth.
- When adding a new ADR, include whether it is current authority, current
  product/background, or historical exploration.
