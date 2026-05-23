# P3-02 Relay Shard Ownership

Date: 2026-05-24 Asia/Shanghai

Status: `IMPLEMENTED_WITH_LOCAL_GATE`

Raw output root: `docs/perf/raw/p3-relay-shard-lease-20260524-0515/`

This is a correctness and ownership gate, not a final multi-instance capacity claim.

## Design

PostgreSQL remains auction truth. Relay shard ownership only decides which backend worker may claim outbox delivery rows.

Added:

- `outbox_relay_shard_leases(shard_id, owner_id, lease_until, acquired_at, renewed_at)`;
- `outbox_delivery.shard_id`, derived from `auction_id` when present;
- shard-aware ready and predecessor indexes;
- relay lease renewal with compare-and-take-over semantics:
  - same owner can renew;
  - another owner can take a shard only after `lease_until <= now()`;
  - idle workers only lease shards that currently have unfinished delivery rows.

The relay still publishes one event at a time through the existing Redis/history/WS path and still enforces same-auction head-of-line ordering.

## Gates

Focused test:

```text
go test ./internal/outbox -run TestRelayShardLeasePreventsDuplicateOwnersAndAllowsFailover -count=1 -v
```

Raw output:

- `docs/perf/raw/p3-relay-shard-lease-20260524-0515/relay-shard-failover-test.txt`

Full validation:

```text
go test -p 1 ./...
pnpm exec node tests/load/validate-k6-suite.mjs
git diff --check
```

## Result

- A contender worker cannot process an event while a live owner holds the target shard lease.
- After the owner lease is expired, the contender renews and processes the event.
- Existing outbox gates still pass: ordered publish, poison DEAD + gap notice, expired publishing reclaim, blocked head-of-line, 5000 pending claim, batch drain, snapshot rebuild.

## Remaining Work

This does not yet prove a two-process backend under live pressure. The next P3 loop should run an owner-kill burst with two backend processes and capture duplicate publish, relay lag, and shard owner diagnostics.
