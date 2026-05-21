# Live Auction

直播竞拍全栈系统，目标是实现一条可演示、可测试、可追问的工程链路：

```text
商品上架 -> 规则配置 -> 开拍 -> 实时出价 -> 动态排名
-> 自动延时/封顶成交 -> 订单生成 -> mock 支付/历史记录
-> 监控诊断与故障恢复
```

## Design Source

正式开发以 [docs/design-v2-industrial](docs/design-v2-industrial) 为唯一设计来源。若文档间有冲突，优先级按：

1. `12-engineering-rules.md`
2. `10-test-gates.md`
3. `05-api-contracts.md`
4. `03-domain-model-and-rules.md`
5. `02-architecture.md`

## Tech Route

- Backend: Go modular monolith, `net/http` + `chi`, PostgreSQL as source of truth.
- Realtime: transactional outbox, Redis history/snapshot, browser WebSocket hub.
- Frontend: React + TypeScript + Vite.
- PC console: Arco Design.
- Mobile H5: custom auction UI.
- Local infra: Docker Compose with PostgreSQL, Redis and MinIO.
- Testing: Go tests, Playwright E2E, k6 load tests, Toxiproxy chaos tests.

## Repository Layout

```text
backend/               Go backend service
  cmd/server/          server entrypoint
  internal/            bounded modules
  migrations/          goose SQL migrations
frontend/
  pc-console/          merchant/host PC console
  mobile-h5/           bidder mobile H5
tests/
  integration/         cross-module/backend tests
  e2e/                 Playwright flows
  load/                k6 scripts
  chaos/               Toxiproxy scenarios
infra/                 local Docker Compose and infra config
docs/
  design-v2-industrial/ design baseline
  evidence/           P0 gate evidence
  perf/               measured performance baselines
  demo/               demo scripts and notes
  adr/                architecture decisions
```

## First Setup

Follow [docs/setup_guide.md](docs/setup_guide.md). Do not start bid, settlement or realtime implementation until the local database, migrations, env file and test commands are in place.

## Engineering Rules

- PostgreSQL is the only authority for auction state, current price, winner, bid result, order state and idempotency result.
- Redis and WebSocket are projections/delivery channels, never money truth.
- Client time never decides close time or winner.
- Every executable bid attempt is idempotent.
- Every auction state mutation writes an immutable event and outbox record.
- Every diagnostic panel must have a real producer.
- No performance number is documented without raw baseline evidence.
