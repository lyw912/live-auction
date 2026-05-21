# 15 · Startup Checklist

## How To Use This Folder

正式开工时，把整个 `docs/design-v2-industrial/` 复制到新仓库。新仓库开发只以这个文件夹为设计来源。

不要再回看：

- `docs/design-v1-initial/REVISIONS-*`
- 旧 spec/ADR
- 任何目录外设计草稿

如果发现本文件夹内部矛盾，以编号靠后的治理文档优先：

```text
12-engineering-rules.md
10-test-gates.md
05-api-contracts.md
03-domain-model-and-rules.md
02-architecture.md
```

## First Day Checklist

- [ ] 复制本文件夹到新仓库 `docs/design-v2-industrial/`
- [ ] 创建 `docs/evidence/`
- [ ] 创建 `docs/perf/`
- [ ] 创建 `docs/demo/`
- [ ] 创建 `.env.example`
- [ ] 创建 docker compose: PostgreSQL, Redis, MinIO
- [ ] 选定 Go HTTP router
- [ ] 选定 PC UI library
- [ ] 建立 migration 工具
- [ ] 建立测试命令
- [ ] 建立 CI 最小流程
- [ ] 把 `12-engineering-rules.md` 加入 PR 模板或 review checklist

## Before Writing Bid Code

- [ ] migration includes auctions, rules, bids, events, outbox, idempotency
- [ ] error code package exists
- [ ] money type uses integer cents
- [ ] rule validation unit tests pass
- [ ] idempotency status constants exist
- [ ] transaction helper can set lock_timeout and statement_timeout
- [ ] structured logging has trace_id

## Before Writing WebSocket Code

- [ ] event envelope frozen
- [ ] ws ticket endpoint designed
- [ ] Redis history key contract frozen
- [ ] snapshot response contract frozen
- [ ] client recovery state machine implemented in tests or storybook
- [ ] backpressure close reason defined

## Before Frontend Integration

- [ ] `05-api-contracts.md` paths and response shapes are stable
- [ ] H5 state matrix accepted
- [ ] PC rule form validation mirrors backend
- [ ] recovery UI disables bid CTA
- [ ] no fake success animation

## Before Demo

- [ ] all P0 gates in `10-test-gates.md` have evidence records
- [ ] diagnostic page has real data
- [ ] demo flow documented using `templates/demo-flow.md`
- [ ] known limits written
- [ ] no unmeasured performance number in README/slides

## Before Writing Any Performance Claim

- [ ] Linux native or documented equivalent environment
- [ ] k6 script path committed
- [ ] raw output saved
- [ ] 3 runs recorded
- [ ] DB/Redis/runtime metrics recorded
- [ ] bottleneck explained
- [ ] claim text references baseline file

## Red Flags

Stop and write an ADR if someone proposes:

- Redis as price/winner truth.
- Direct WS publish without outbox.
- Client-side hammer.
- Background timer as close authority.
- Redis Lua P0 implementation.
- Fake dashboard data.
- Performance numbers copied from internet.
- Chat/presence sharing auction queue.
- Multi-instance deployment claim without shared fanout design.

## Minimum PR Template

```text
Design section:
P0/P1/P2:
Truth source affected:
Idempotency needed:
Events/outbox needed:
Recovery behavior:
Tests:
Evidence record:
Known limits:
```
