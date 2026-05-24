# P3-02 Relay Shard Ownership

Date: 2026-05-24 Asia/Shanghai

Status: `DONE_WITH_WINDOWS_LOCAL_EVIDENCE`

Raw output roots:

- `docs/perf/raw/p3-relay-shard-lease-20260524-0515/`
- `docs/perf/raw/p3-relay-owner-kill-202605240531/`

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

Owner-kill pressure gate:

```text
pnpm exec node tests/load/run-p3-relay-owner-kill.mjs
```

Raw output:

- `docs/perf/raw/p3-relay-owner-kill-202605240531/`
- `leases-before-kill.txt`: shard `13` live owner was `p3-relay-a`.
- `killed-owner.txt`: runner killed `p3-relay-a`.
- `leases-after-kill.txt`: shard `13` live owner became `p3-relay-b`.
- `leases-final.txt`: shard `13` remained owned by `p3-relay-b`.
- `bid-pressure-owner-kill.log`: 120 rps for 30 seconds, 3600 iterations, 0 HTTP errors, 100% business envelopes.
- `outbox-after-drain.txt`: 3600 `PUBLISHED` rows for `auc_live`.
- `outbox-drain-poll.csv`: pending rows reached 0 before the first 5 second poll.

An earlier attempted run, `p3-relay-owner-kill-202605240524`, found a harness gap: Windows Node `spawn(..., { shell: true })` killed only the shell wrapper, leaving `p3-outboxrelay.exe` alive. Its raw directory was cleaned by P3-09 because the retained evidence is this summarized finding, not the failed raw bundle. The runner now starts server and relay binaries with `shell: false` and asserts the live owner before and after kill.

## Remaining Work

This remains Windows local evidence, not a final capacity claim. It proves relay shard lease failover and outbox drain under local bid pressure. The next P3 loop should attack realtime fanout and hot/cold multi-room isolation before claiming broader multi-instance realtime behavior.
