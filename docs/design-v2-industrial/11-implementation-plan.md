# 11 · Implementation Plan

## Pre-Start Checklist

Before coding:

- choose exact backend router and UI library;
- initialize monorepo layout;
- write docker compose for PG/Redis/MinIO;
- create migration tool setup;
- create seed data plan;
- create test command conventions;
- create `.env.example`;
- create CI skeleton;
- create docs/evidence folder in implementation repo.

## Suggested Repo Layout

```text
backend/
  cmd/server/
  internal/gateway/
  internal/auction/
  internal/order/
  internal/realtime/
  internal/outbox/
  internal/scheduler/
  internal/observability/
  internal/storage/
  migrations/
frontend/
  pc-console/
  mobile-h5/
tests/
  integration/
  e2e/
  load/
  chaos/
infra/
  docker-compose.yml
docs/
  design-v2-industrial/
  evidence/
  perf/
```

## Milestone 0 · Foundation

Deliver:

- config/env loader;
- DB connection/pool;
- Redis client;
- MinIO client;
- structured logger;
- error code package;
- trace_id middleware;
- migration runner;
- seed users/room/items.

Exit:

- app starts;
- health endpoint;
- migration up/down;
- tests run in CI/local.

## Milestone 1 · Domain And CRUD

Deliver:

- users/rooms/items tables;
- upload-url flow;
- auctions/rules DRAFT/SCHEDULED;
- rule validation matrix;
- PC APIs for item/rule/list.

Exit:

- rule validation tests;
- cap reachability tests;
- PC can create item and schedule auction.

## Milestone 2 · Bid Truth Path

Deliver:

- idempotency_records;
- bid command pipeline;
- auction row lock;
- bids/events/outbox write;
- cap/extension/cancel/order;
- payment mock.

Exit:

- bid integration tests;
- duplicate idempotency tests;
- concurrent-final-second test;
- cancel-cap-race test.

## Milestone 3 · Outbox And Projection

Deliver:

- outbox_events/outbox_delivery;
- sharded ordered relay;
- Redis snapshot/history;
- DEAD handling and anomaly.

Exit:

- kill-after-commit test;
- outbox-order test;
- outbox-poison test;
- snapshot can rebuild from DB.

## Milestone 4 · WebSocket

Deliver:

- ws ticket endpoint;
- browser-compatible WS connect;
- room hub;
- event envelope;
- recovery by last_seq;
- bounded send queues;
- slow-consumer close.

Exit:

- ws-auth-browser test;
- forged-room test;
- history/snapshot recovery test;
- slow-consumer test.

## Milestone 5 · Scheduler And Diagnostics

Deliver:

- scheduler_jobs worker;
- START/END/ORDER_EXPIRE/ANOMALY_SCAN;
- anomaly producers;
- diagnostic APIs.

Exit:

- scheduler crash test;
- end-job-vs-extend test;
- stuck auction anomaly demo;
- outbox diagnostic panel data.

## Milestone 6 · Frontend P0

Deliver:

- PC item/rule/start/cancel/order/diagnostic;
- H5 room/list/detail/bid/result/chat/recovery;
- UI state matrix;
- event-driven effects.

Exit:

- Playwright P0 state tests;
- demo flow works end-to-end;
- no fake diagnostic panels.

## Milestone 7 · P0 Gate Freeze

Deliver:

- all P0 gates green;
- evidence records filled;
- known limits documented.

Exit:

- ready for P1 metrics/benchmark.

## Milestone 8 · P1 Evidence

Deliver:

- Prometheus metrics;
- Grafana dashboard;
- k6 scripts;
- Toxiproxy scripts;
- baseline reports.

Exit:

- final materials may include only measured numbers.

## Daily Engineering Routine

1. Pick task from current milestone.
2. Check design doc section.
3. Write/adjust tests first for invariants.
4. Implement narrowly.
5. Run relevant tests.
6. Add evidence record if P0 gate touched.
7. Do not refactor unrelated modules.

## Definition Of Done

A feature is done only when:

- API contract implemented;
- DB constraints present;
- tests cover success and failure;
- logs contain trace_id;
- anomalies added if failure mode is operational;
- UI handles pending/recovering/error states;
- evidence record updated for gate features.

## Parallel Work Split

Safe parallel tracks:

- frontend static layout after API contracts fixed;
- DB migrations + backend domain tests;
- outbox/realtime after event schema fixed;
- diagnostics after anomaly schema fixed;
- k6 scripts after API/WS protocol stable.

Avoid parallel edits:

- bid transaction and schema by multiple people at once;
- WS protocol and H5 recovery state at once without contract freeze;
- rule state machine and scheduler start/end in separate uncoordinated branches.
