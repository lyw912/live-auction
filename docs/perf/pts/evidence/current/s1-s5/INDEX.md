# S1-S5 Current Evidence Index

> Status: current evidence map, 2026-06-05.
> Raw run folders under this directory are local evidence artifacts and can be
> large. Commit reviewed summaries and specific small evidence files only when
> needed; keep Prometheus dumps, OS snapshots, k6 JSON streams, and sampling-log
> exports out of normal commits unless a review explicitly needs them.

## Current Pass / Diagnostic Runs

| Scenario | Directory | Classification | Primary files |
|---|---|---|---|
| S1 final-second contention | `s1-final-second-contention-2MLCX7WG/` | `CURRENT_PASS` | `s1-review.md`, `l4b-invariant-gates.tsv`, `l4b-correctness.txt`, `postgres-summary.txt` |
| S1 strict-barrier diagnostic | `s1-diagnostic-strict-barrier-TGLBX7GG/` | `CURRENT_DIAGNOSTIC` | `s1-review.md`, `l4b-invariant-gates.tsv` |
| S2 long soak | `s2-long-soak-20260604T095720/` | `CURRENT_PASS` | `l4b-invariant-gates.tsv`, `postgres-summary.txt`, `metrics.prom` |
| S2 convergence drain | `s2-convergence-drain-20260604T1937/` | `CURRENT_PASS` | `l4b-invariant-gates.tsv`, `postgres-summary.txt`, `metrics.prom` |
| S2 capacity display | `s2-capacity-accepted-display200-p1-ecs-20260604T192002/` | `CURRENT_PASS` | `l4b-invariant-gates.tsv`, `pg-final-summary.txt`, `kafka-lag-final.txt` |
| S2 read interference display | `s2-read-display-postfix-ecs-15m-20260604T140509/` | `CURRENT_PASS` | `l4b-invariant-gates.tsv`, `postgres-summary.txt`, `metrics.prom` |
| S3 live-only fanout | `s3-live-only-fanout-XWLAX70G/` and `s3-pts-console-live-only-XWLAX70G/` | `CURRENT_PASS` | `postgres-summary.txt`, `metrics.prom`, `pts-console-api-list.md` |
| S3 mixed final burst | `s3-mixed-final-burst-20L8X79G/` and `s3-pts-console-mixed-20L8X79G/` | `CURRENT_PASS` | `postgres-summary.txt`, `metrics.prom`, `pts-console-api-list.md` |
| S4 Redis fault | `s4-redis-fault-20260604T203439/` | `CURRENT_PASS` | `l1c-gates.tsv`, `l4b-invariant-gates.tsv` |
| S4 backend/settlement crash | `s4-backend-settlement-crash-20260604T205252/` | `CURRENT_PASS` | `l1c-gates.tsv`, `l4b-invariant-gates.tsv` |
| S4 PostgreSQL fault | `s4-postgres-fault-20260604T205623/` | `CURRENT_PASS` | `l1c-gates.tsv`, `l4b-invariant-gates.tsv` |
| S4 Kafka fault, local | `s4-kafka-fault-local-20260604T205824/` | `CURRENT_PASS` | `l1c-gates.tsv`, `l4b-invariant-gates.tsv` |
| S4 Kafka fault, independent k6 | `s4-kafka-fault-independent-20260604T202510/` | `CURRENT_PASS` | `l4b-invariant-gates.tsv`, `postgres-summary.txt`, `metrics.prom` |
| S4 Redis FLUSHALL | `s4-redis-flush-20260604T210256/` | `CURRENT_PASS` | `l1c-gates.tsv`, `l4b-invariant-gates.tsv` |
| S4 Redis+Kafka fault | `s4-redis-kafka-fault-20260604T210834/` | `CURRENT_PASS` | `l1c-gates.tsv`, `l4b-invariant-gates.tsv` |
| S4 Redis partial network | `s4-redis-partial-network-20260604T224626/` | `CURRENT_PASS` | `l1c-gates.tsv`, `l4b-invariant-gates.tsv` |
| S5 clean reconnect | `s5-clean-reconnect-20260604T221312/` | `CURRENT_PASS` | `s5-k6-summary.json`, `run-env.json`, `recovery-after.csv` |
| S5 Toxiproxy reconnect | `s5-toxiproxy-reconnect-20260604T231925/` | `CURRENT_PASS` | `s5-k6-summary.json`, `run-env.json`, `toxiproxy-ws.json`, `recovery-after.csv` |

## Interpreting This Index

Use [`docs/current/test-strategy/s1-s5-pressure-and-gate-audit.md`](../../../../../current/test-strategy/s1-s5-pressure-and-gate-audit.md)
for the gate-by-gate explanation. Use
[`docs/current/test-strategy/s1-s5-debug-and-system-change-log.md`](../../../../../current/test-strategy/s1-s5-debug-and-system-change-log.md)
for the scenario narrative and boundaries.

Historical, failed, smoke, and preflight raw folders were moved to:

```text
docs/perf/pts/evidence/archive/legacy-unused-20260605/
```

That archive is ignored by Git by policy.
