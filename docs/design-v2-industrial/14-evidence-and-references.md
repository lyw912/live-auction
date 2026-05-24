# 14 · Evidence And References

## Evidence Policy

External references prove semantics, constraints, or mature patterns. They do not prove this project's performance. Performance must be measured locally.

## Primary References

| Source | URL | Fact Used | Project Consequence |
|---|---|---|---|
| PostgreSQL SELECT docs | https://www.postgresql.org/docs/current/sql-select.html | `SKIP LOCKED` skips locked rows and gives inconsistent view | queue claim only; no ordering proof |
| PostgreSQL explicit locking | https://www.postgresql.org/docs/current/explicit-locking.html | row locks serialize conflicting row updates | auction row is serialization point |
| PostgreSQL partial indexes | https://www.postgresql.org/docs/current/indexes-partial.html | partial unique index supports conditional uniqueness | one ACTIVE/narrating per room |
| MDN WebSocket constructor | https://developer.mozilla.org/en-US/docs/Web/API/WebSocket/WebSocket | browser constructor takes URL and protocols | no custom Authorization header |
| MDN protocol upgrade | https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/Protocol_upgrade_mechanism | `Sec-WebSocket-Protocol` negotiates subprotocol | ticket via subprotocol |
| Grafana k6 websockets | https://grafana.com/docs/k6/latest/javascript-api/k6-websockets/ | current module is `k6/websockets` | load scripts use new module |
| Stripe idempotency | https://docs.stripe.com/api/idempotent_requests | repeated key returns first result and checks parameters | durable result + request_hash |
| Redis scripting | https://redis.io/docs/latest/develop/programmability/eval-intro/ | Lua is atomic inside Redis | Lua not DB/WS consistency proof |
| Debezium outbox router | https://debezium.io/documentation/reference/stable/transformations/outbox-event-router.html | outbox event id/aggregate/payload pattern | event schema and dedupe |
| MDN performance.now | https://developer.mozilla.org/en-US/docs/Web/API/Performance/now | monotonic high-resolution time | UI countdown display only |
| Toxiproxy | https://github.com/Shopify/toxiproxy | programmable network faults | weak-network tests |

## Design Claims And Status

| Claim | Status | Evidence Needed |
|---|---|---|
| PG row lock serializes one auction writes | supported by PG docs | integration/concurrency tests |
| outbox avoids commit-then-crash lost publish | supported by pattern | kill-after-commit test |
| Redis history can recover short gaps | supported as pattern | recovery hit-ratio benchmark |
| snapshot fallback prevents permanent stale client | design | reconnect storm test |
| self WS hub can handle target watchers | unproven | k6 baseline |
| outbox hot table is acceptable | unproven | outbox burst report |
| bid path latency is acceptable | unproven | final-second bid baseline |
| UI animation does not block input | unproven | Playwright + longtask |

## Industrial Alternatives

| Area | Alternative | Why Not P0 |
|---|---|---|
| WS fanout | self hub pressure gates | bounded queues, slow close, reconnect, and snapshot fallback must be proven locally |
| Outbox | CDC/WAL/Debezium/pg_logical_emit_message | more moving parts; keep as future if polling fails |
| Scheduler | Asynq/Temporal | DB lease sufficient and auditable |
| Pub/Sub | NATS/Kafka | overkill for P0 single process |
| Observability | OTel | Prometheus/logs first |
| Hot path | Redis Lua reservation | reconciliation complexity |

## Local Evidence To Produce

Required files in implementation repo:

```text
docs/evidence/p0-gates/*.md
docs/perf/baseline-YYYY-MM-DD.md
docs/perf/outbox-burst-YYYY-MM-DD.md
docs/demo/demo-flow.md
docs/demo/known-limits.md
```

## Citation Rules For Final Materials

- Cite official docs for semantics.
- Cite local baseline for numbers.
- Do not cite internet benchmarks as your capacity.
- State hardware and workload with every number.
- State known limits next to claims.
