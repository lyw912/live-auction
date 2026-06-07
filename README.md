# Live Auction

直播竞拍全栈系统，目标是实现一条可演示、可测试、可追问的工程链路：

```text
商品上架 -> 规则配置 -> 开拍 -> 实时出价 -> 动态排名
-> 自动延时/封顶成交 -> 订单生成 -> mock 支付/历史记录
-> 监控诊断与故障恢复
```

## Design Source

当前开发、压测、评审先读 [docs/current/README.md](docs/current/README.md)。官方题目原文 `抖音电商AI全栈课题-直播竞拍全栈系统（宣讲版）.md` 永远不改；[docs/design-v2-industrial](docs/design-v2-industrial) 是历史设计基线和产品/UX/工程约束库，不再是热竞价架构的唯一来源。

当前优先级：

1. `docs/current/performance-correctness-contract.md`
2. `docs/current/architecture.md`
3. `docs/current/evidence-policy.md`
4. `docs/current/runtime-profiles.md`
5. `docs/current/document-map.md`
6. 官方题目原文
7. `docs/design-v2-industrial/*` 中仍适用的产品、UI、恢复、诊断和工程规则

## Tech Route

- Backend: Go modular monolith, `net/http` + `chi`.
- Hot bidding target: Redis live decision state + Kafka durable decision WAL/fence + PostgreSQL settlement/audit/order truth.
- Realtime: transactional outbox, Redis history/snapshot, browser WebSocket hub.
- Frontend: React + TypeScript + Vite.
- PC console: Arco Design.
- Mobile H5: custom auction UI.
- Local infra: Docker Compose with PostgreSQL, Redis, Kafka, MinIO, Prometheus and Grafana.
- Testing: Go tests, Playwright E2E, local smoke, PTS/JMeter cloud pressure, correctness verifier and fault-injection evidence.

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
  current/            current architecture, PTS-1B contract and evidence policy
  archive/            historical era/progress maps
  design-v2-industrial/ historical design baseline and UI/product constraints
  evidence/           historical gate evidence
  perf/               measured performance baselines
  demo/               demo scripts and notes
  adr/                architecture decisions
```

## First Setup

Follow [docs/setup_guide.md](docs/setup_guide.md). Do not start bid, settlement or realtime implementation until the local database, migrations, env file and test commands are in place.

Current runtime profile:

- `.env.example`: local/demo profile on the same Redis hot ledger + Kafka ACK engine; admission enabled.
- PTS-1B scripts: same engine, admission disabled, Kafka required.

Formal PTS-1B runs should follow [tests/pts/MANIFEST.md](tests/pts/MANIFEST.md), not hand-edited env guesses.

## Engineering Rules

- For the current PTS-1B hot manual-bid path, Redis is the live atomic decision state only under the Kafka WAL/fence and reconciliation contract.
- Kafka is the durable ordered decision log/fence for current hot-engine decisions.
- PostgreSQL remains settlement, audit, order and durable query truth. It is not the synchronous hot-row decision point for the PTS-1B target.
- WebSocket is delivery/recovery, never client-side truth.
- Client time never decides close time or winner.
- Every executable bid attempt is idempotent.
- Every auction state mutation writes an immutable event and outbox record.
- Every diagnostic panel must have a real producer.
- No performance number is documented without raw baseline evidence.
- HTTP status is not the auction outcome. Inspect `ENGINE_*`, durability status and settlement status.
