# PTS Report 4I3CX7UG Review

Date: 2026-05-29

## Verdict

`FAIL / HARNESS_GAP`

This run cannot be used as successful PTS-1 final-burst evidence.

The Alibaba PTS report shows `POST PTS-1 hotspot bid` executed `148802` times,
not approximately `1000` one-shot bid attempts. The prepared JMX has
`LoopController.loops=1`, so the run likely used platform-level looping,
different uploaded assets, or a PTS setting that overrode the intended one-shot
semantics.

## PTS Summary

| Metric | Value |
|---|---:|
| Report id | `4I3CX7UG` |
| Window | `2026-05-29 19:54:33` to `2026-05-29 20:00:33` |
| Agents | 2 |
| `POST PTS-1 hotspot bid` samples | 148802 |
| Bid avg RT | 115.56 ms |
| Bid p90 | 256 ms |
| Bid p99 | 309 ms |
| Bid max | 941 ms |
| Bid success rate | 0.56% |

## Correctness Gate Result

Post-run gate failed P0:

| Gate | Result | Meaning |
|---|---|---|
| `redis_engine_seq_matches_settlement` | FAIL | PG engine seq did not converge to the settled Redis/Kafka engine ledger. |
| `kafka_offset_matches_engine_order` | FAIL | Kafka offset order did not preserve `engine_seq`. |
| `no_non_terminal_settlements` | FAIL | Settlement left failed rows. |
| `redis_kafka_pg_accepted_match` | FAIL | Accepted/sold Kafka ledger rows did not match PG accepted bids. |

Supporting state:

```text
auction seq=1 engine_seq=2 accepted_bid_count=1
settlement_total=826
settlement_accepted_or_sold=59
open_or_failed_settlements=824
engine paused: REDIS_ENGINE_DB_BEHIND_REDIS
```

The first durable failure shape was `engine seq gap redis=N db_next=3` after
Kafka offset order diverged from Redis `engine_seq`.

## Root Cause

This run exposed two separate issues:

1. The PTS harness did not run the intended one-shot PTS-1 workload.
2. The L4B ledger path allowed concurrent HTTP requests to append Redis engine
   decisions to Kafka out of `engine_seq` order. Kafka then delivered messages
   by offset, while PG settlement correctly required contiguous `engine_seq`.
   Once a future seq was read before the missing earlier seq, settlement marked
   it failed and the auction fell behind Redis.

This is a correctness failure, not just a performance bottleneck.

## Fix Applied After Review

The hot path now waits for the Redis pending-decision minimum seq before
appending a result to Kafka. This preserves the invariant:

```text
Kafka offset order for one auction == Redis engine_seq order
```

A concurrent integration test was added to assert that simultaneous Redis
ledger decisions append to the ledger in `engine_seq` order.

## Retest Requirements

Before rerunning PTS-1:

1. Restart the backend with the fixed binary.
2. Reset/seed with `SESSION_COUNT=1000`.
3. Upload the current JMX and CSV.
4. In PTS UI set:
   - pressure mode: virtual users;
   - maximum VU: `1000`;
   - duration: `6 minutes`;
   - ramp-up: `1 minute`;
   - specify loop count: `Yes: 1`.
5. After the run, reject the report unless `POST PTS-1 hotspot bid` is close to
   `1000` samples.
6. Run the post-run correctness gate before making any performance claim.
