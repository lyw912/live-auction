# Realtime Auction Load Model Reference

> Status: superseded design note, 2026-06-02.
> The current realtime test model is now integrated into
> `docs/current/test-strategy/README.md`, `s2-steady-auction-and-soak.md`,
> `s3-room-fanout.md`, and `s5-reconnect-recovery.md`.

This note originally argued why a mixed protocol `L2-P3` run was not enough to
prove a production-realistic realtime auction. That conclusion still stands,
but the plan names have changed:

| Former idea | Current scenario |
|---|---|
| steady interactive auction | S2 |
| fanout soak / 10k viewers | S3 |
| reconnect storm | S5 |
| fault during interaction | S4 follow-up layers |

## Current Model

Use S2/S3/S5 rather than a separate L2/L3/L4 ladder:

| Scenario | Connection model | Bid/message model | Primary proof |
|---|---|---|---|
| S2 steady auction | 2000-5000 viewers context, 5-15% active bidders | open arrival model, 20/s -> 60/s -> 100/s, 30-60 min local soak | decision p99 <= 100ms, bounded dropped iterations, no resource leak |
| S3 fanout | 2000 PTS WS cost variant or 10000 PTS headline | low accepted-update source, viewers are the variable | fanout publish-to-receive p99 <= 1s, connections held, RAM/conn |
| S5 reconnect | reconnecting cohort after weak-network drop | stale `last_seq` catch-up during ongoing updates | time-to-current-state, no seq gaps or duplicate notifications |

## Rationale Kept From The Old Note

- k6 arrival-rate executors are the right local tool for sustained open-model
  request arrival and expose overload through `dropped_iterations`.
- Closed VU loops are useful for active-user/fault behavior, but they can hide
  sustained arrival-rate overload because slow responses reduce offered rate.
- Fanout pressure is driven by accepted public updates multiplied by connected
  viewers, not by rejected bid attempts.
- A single hot auction has a real sequencing ceiling; WS gateways, Redis
  pub/sub topology, Kafka partitions, and settlement workers must be discussed
  as architecture constraints rather than dismissed as "just infrastructure."

For runnable assets, use `tests/pts/MANIFEST.md`.
