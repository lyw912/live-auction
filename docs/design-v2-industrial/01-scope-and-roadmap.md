# 01 · Scope And Roadmap

## Scope Lock

P0 是必须按顺序完成的硬骨架：

```text
auth/schema
-> item/rule/order basic CRUD
-> auction state machine
-> bid transaction + idempotency
-> auction_events + outbox_events
-> outbox relay + Redis snapshot/history
-> WebSocket room + recovery
-> scheduler jobs
-> H5/PC full flow
-> P0 diagnostics
-> P0 failure gates
```

P1/P2 不得抢占 P0 正确性。任何“亮点”如果没有测试或诊断证据，视为未完成。

## P0

| ID | Deliverable | Done Evidence |
|---|---|---|
| P0-01 | repo/init/env/docker compose | one command starts PG/Redis/MinIO/backend/frontend |
| P0-02 | DB migrations | all core tables, constraints, indexes |
| P0-03 | auth/role/ACL | host/user API and room auth tests |
| P0-04 | item + image upload | MinIO presigned PUT flow |
| P0-05 | auction rule lifecycle | DRAFT/SCHEDULED/ACTIVE, freeze rules |
| P0-06 | bid command | row lock, idempotency, reject priority |
| P0-07 | cap/extension/cancel/order | integration tests |
| P0-08 | outbox relay | ordered delivery, poison handling |
| P0-09 | Redis snapshot/history | reconnect recovery test |
| P0-10 | WebSocket hub | browser ticket auth, backpressure |
| P0-11 | scheduler | start/end/order expire, lease recovery |
| P0-12 | PC console | item/rule/start/cancel/order/diagnostic |
| P0-13 | H5 room | list/detail/bid/result/recovery/chat |
| P0-14 | diagnostics | anomaly producers and drilldown |
| P0-15 | failure gates | all P0 tests green |

## P1

| ID | Deliverable | Entry Condition |
|---|---|---|
| P1-01 | Prometheus metrics | P0 diagnostics exist |
| P1-02 | Grafana dashboards | metrics real, not mock |
| P1-03 | k6 baseline suite | P0 flow stable |
| P1-04 | Toxiproxy weak-network tests | WS recovery stable |
| P1-05 | Redis/DB reconciliation checker | snapshot/event schema stable |
| P1-06 | UI performance trace | H5 event states complete |
| P1-07 | alert rules | anomaly semantics stable |

## P2

| Candidate | Keep Only If |
|---|---|
| Redis Lua reservation | PG baseline proves row lock path is bottleneck and reconciliation is designed |
| Centrifugo replacement | self hub misses WS go/no-go |
| WAL/CDC outbox | polling outbox hot-table test fails or becomes visible bottleneck |
| event replay | auction_events schema stable |
| pprof deep dive | k6 baseline exists |

## Build Order

1. **Foundation**: migrations, config, logging, errors, trace_id.
2. **Domain**: auction state machine, rules, validation, DB constraints.
3. **Money Path**: bid transaction, idempotency, order/payment.
4. **Realtime**: outbox relay, Redis projection, WS recovery.
5. **Schedulers**: start/end/order expire/anomaly scan.
6. **Frontend**: PC then H5, wired to real APIs.
7. **Diagnostics**: real panels only.
8. **Failure Tests**: concurrency, kill/restart, reconnect, poison, weak network.
9. **Benchmark**: only after correctness gates.

## Cut Rules

Cut immediately if it delays P0 gates:

- advanced animation;
- AI feature;
- Redis Lua implementation;
- OTel;
- NATS/Temporal;
- full microservices;
- advanced moderation/sensitive word engine;
- multi-room subscription in one WS connection;
- real payment/live SDK.

## Go/No-Go Dates

| Decision | Deadline | Criteria |
|---|---|---|
| self WS hub vs Centrifugo | May 27, 2026 18:00 Asia/Shanghai | reconnect, slow-consumer, memory, kill-after-commit tests |
| performance number inclusion | after first Linux native baseline | raw output + env + 3 runs |
| P1 start | after all P0 gates green | no blocker failures |
| demo freeze | after P0 demo + diagnostics + evidence records | no untriaged P0 risk |
