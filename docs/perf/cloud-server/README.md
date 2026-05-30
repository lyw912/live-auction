# 云服务器性能测试工作区

本目录是云服务器性能测试、瓶颈归因、架构重构论证和评委答辩的唯一入口。历史文件仍保留在 `docs/perf/pts/`、`docs/perf/raw/` 和 `tests/pts/`，本目录只做结构化索引、复盘、重新设计和下一轮 P0 计划。

## 背景

当前云服务器为阿里云 ECS，8c / 32G，服务部署在河源地域。PTS 压力源应使用阿里云 VPC 内网、华南2（河源），目标地址为 `172.16.179.112:18080`。公网 IP `47.113.223.90` 只用于外部访问和运维，不作为同地域 PTS 主压测路径。

最新已完成 PTS 报告：

- ReportId: `3IVNW7TF`
- 报告文件: `Jmeter压测报告-3IVNW7TF-20260528014811.pdf`
- PTS 明细: `docs/perf/pts/evidence/archive/historical/after-3IVNW7TF-review/pts-sampling-logs/`
- 服务侧证据: `docs/perf/pts/evidence/archive/historical/after-3IVNW7TF-review/`

## 文档地图

1. [00-context-and-evidence.md](00-context-and-evidence.md)
   - 当前云服务器、服务、PTS、数据库、Redis、outbox 证据汇总。
2. [01-methodology-and-research.md](01-methodology-and-research.md)
   - 重新调研后的方法论：SRE、USE、RED、k6/PTS、outbox、Kafka/Centrifugo。
3. [02-p0-performance-test-design.md](02-p0-performance-test-design.md)
   - 从 P0 重新开始的完整性能测试设计。
4. [03-workload-matrix-and-pts-config.md](03-workload-matrix-and-pts-config.md)
   - PTS/JMeter/k6 工作负载矩阵、参数、数据集和验收口径。
5. [04-bottleneck-deep-dive-tasks.md](04-bottleneck-deep-dive-tasks.md)
   - 每个已发现问题和建议拆成可独立深挖的任务。
6. [05-architecture-decision-records.md](05-architecture-decision-records.md)
   - outbox、Centrifugo、Kafka/NATS、Redis、PostgreSQL truth 的技术决策。
7. [06-judge-defense-playbook.md](06-judge-defense-playbook.md)
   - 面向大厂开发、运维、产品、用户、评委的地狱拷问清单和答辩证据。
8. [07-execution-runbook.md](07-execution-runbook.md)
   - 云服务器下一轮 P0 压测执行手册。
9. [08-evidence-index.md](08-evidence-index.md)
   - 所有证据、命令、报告、原始文件和缺口索引。

## 当前工程判断

本轮压测已经证明：

- HTTP 层 100% 成功不等于竞拍实时链路健康。
- PTS 汇总显示 `643 TPS`、`P99 153ms`，但数据库侧 `outbox_delivery` 积压超过 30 万。
- `auction_admission_enabled=0`，压力确实打到后端。
- Redis publish pipeline 很快，机器 CPU、内存、磁盘没有被打满。
- 主瓶颈在 app-owned outbox relay 的消费模型、单 auction/shard 顺序发布和积压治理。

因此下一步不是先宣传容量，也不是先引入 Centrifugo。下一步是从 P0 性能测试纪律重新开始，先证明脚本、数据、业务结果、服务侧证据都可信，再重构 outbox relay 并跑同等压力回归。
