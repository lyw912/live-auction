# Design v2 Industrial · 开工基线

> 日期：2026-05-22  
> 用途：这是直播竞拍系统第二版、可复制到新仓库直接开工的完整设计文档集。正式开发只参考本文件夹和本文件夹内整理后的项目简报，不再回看 v1.x 修订包。

## 入口顺序

1. `00-project-brief.md`：项目目标、官方范围、评分拆解。
2. `01-scope-and-roadmap.md`：P0/P1/P2 分期、取舍、开工顺序。
3. `02-architecture.md`：总体架构、技术选型、模块边界。
4. `03-domain-model-and-rules.md`：竞拍领域模型、状态机、规则矩阵。
5. `04-data-and-storage.md`：PostgreSQL/Redis/MinIO schema、约束、真源边界。
6. `05-api-contracts.md`：REST/WS API、错误码、幂等契约。
7. `06-realtime-and-recovery.md`：WebSocket、outbox relay、history/snapshot、背压。
8. `07-frontend-ux.md`：PC/H5 页面、状态矩阵、交互门禁。
9. `08-observability-and-ops.md`：诊断页、指标、告警、异常恢复。
10. `09-performance-and-benchmark.md`：压测纪律、脚本设计、环境要求。
11. `10-test-gates.md`：P0/P1 必过测试与失败解释。
12. `11-implementation-plan.md`：开工准备、里程碑、任务依赖。
13. `12-engineering-rules.md`：开发铁律、禁止项、审查清单。
14. `13-risk-register.md`：盲区、陷阱、反驳与兜底。
15. `14-evidence-and-references.md`：外部证据、论据、可追问口径。
16. `16-industrial-p2-p3-roadmap.md`：P1 完成后的工业级 P2/P3+ 后续路线图、调研依据、取舍和门禁。
17. `17-local-stress-and-p3-execution-plan.md`：Windows-first 压测、mock 边界、周期性瓶颈发现和 P3 执行细则。
18. `templates/`：正式开发时需要持续填写的模板。

## 一句话方案

用 PostgreSQL 作为竞拍事实真源，用行锁序列化同一 auction 的所有金钱相关写入；用 durable idempotency 防重复；用 immutable events + outbox relay 把提交后的状态可靠投递到 Redis history/snapshot 和 WebSocket；客户端只展示服务端权威状态，断线或 gap 后通过 history/snapshot 恢复。

## 不再争论的决策

- P0 不做微服务，不做真实直播推流，不做真实支付，不做 AI 热路径。
- Redis Lua 不进入 v1 实现范围，只作为 P2 design-only。
- 所有性能数字必须先有 baseline 报告，不能写猜测值。
- Chat、保证金、讲解中、用户加入广播是完整度功能，不是技术亮点。
- 竞拍正确性、可恢复实时、异常可诊断、实测纪律才是最终亮点。
- P1 完成后，后续工业化工作以 `16-industrial-p2-p3-roadmap.md` 为准：先补 auth/ACL/room/rate-limit/payment hardening，再做 multi-instance realtime 和最终 baseline。

## 复制到新仓库后

1. 保留本目录结构。
2. 先按 `11-implementation-plan.md` 初始化仓库、服务、数据库迁移和测试框架。
3. 每实现一个 P0 模块，补 `templates/evidence-record.md`。
4. 每次写入性能数字前，必须先补 `templates/perf-baseline.md`。
5. 任何新设计若违反 `12-engineering-rules.md`，必须先写 ADR 并通过 review。
