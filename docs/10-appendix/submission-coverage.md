# 最终提交材料覆盖矩阵

父文档：[文档库入口](../README.md)
可信输入：`submission/` 和 `submission/championship-review-2026-06-10/`

## 覆盖原则

最终提交材料用于比赛提交和答辩叙事；本 `docs/` 文档库用于理解、讲解、追问防守和后续扩展。覆盖不意味着逐字搬运，而是把每个提交主题落到真实代码、闭环文档和证据地图。

## `第五组-李烨文-训练营结项文档.md`

| 提交主题 | 新文档位置 |
|---|---|
| 课题名称/团队/分工 | [项目总览](../00-project/00-overview.md) |
| 核心功能清单 | [产品范围](../00-project/01-product-scope.md), [领域模型](../02-domain/00-domain-model-and-rules.md) |
| 端到端使用流程 | [产品范围](../00-project/01-product-scope.md), [H5 闭环](../05-frontend/01-mobile-h5-closed-loop.md), [PC 闭环](../05-frontend/02-pc-console-closed-loop.md) |
| README/运行说明/目录结构 | [代码地图](code-map.md), [系统架构](../01-architecture/00-system-architecture.md) |
| 系统架构图 | [系统架构](../01-architecture/00-system-architecture.md) |
| AI 能力使用 | [AI 运营闭环](../03-backend/04-ai-ops-closed-loop.md) |
| 难点 1：最后一秒竞争 | [热出价闭环](../03-backend/01-bid-decision-closed-loop.md), [架构师拷问](../09-judge-defense/01-architect-defense.md) |
| 难点 2：分布式一致性 | [数据一致性](../01-architecture/01-data-consistency.md), [Kafka 结算](../03-backend/02-kafka-settlement-closed-loop.md), [结算 L4](../03-backend/settlement/00-index.md), [工程难点](../03-backend/05-engineering-difficulties.md) |
| 难点 3：Redis 丢失恢复 | [Redis 恢复](../03-backend/03-redis-loss-recovery.md), [恢复 L4](../03-backend/recovery/00-index.md), [工程难点](../03-backend/05-engineering-difficulties.md) |
| 难点 4：前端实时体验 | [WebSocket 恢复](../04-realtime/01-websocket-recovery-closed-loop.md), [WebSocket L4](../04-realtime/websocket/00-index.md), [H5 闭环](../05-frontend/01-mobile-h5-closed-loop.md), [H5 L4](../05-frontend/mobile-h5/00-index.md), [工程难点](../03-backend/05-engineering-difficulties.md) |
| 项目亮点/创新 | [项目总览](../00-project/00-overview.md), [答辩索引](../09-judge-defense/00-defense-index.md) |
| S1-S5 测试体系 | [证据映射](../07-performance-and-evidence/00-evidence-map.md), [测试/SRE 拷问](../09-judge-defense/02-sre-test-defense.md) |
| 文档库模板与可视化要求 | [模板覆盖说明](../00-project/03-template-coverage.md), [可视化覆盖矩阵](../00-project/04-visualization-map.md) |

## `8分钟提问追问备战手册.md`

| 评委角色 | 新文档位置 |
|---|---|
| 架构师 | [架构师拷问](../09-judge-defense/01-architect-defense.md) |
| 后端开发 | [热出价闭环](../03-backend/01-bid-decision-closed-loop.md), [数据一致性](../01-architecture/01-data-consistency.md) |
| 运维/测试 | [测试/SRE 拷问](../09-judge-defense/02-sre-test-defense.md), [风险矩阵](../08-tests-and-risk/00-risk-and-abuse-matrix.md) |
| 前端 | [H5 闭环](../05-frontend/01-mobile-h5-closed-loop.md), [WebSocket 恢复](../04-realtime/01-websocket-recovery-closed-loop.md) |
| 产品经理 | [产品拷问](../09-judge-defense/03-product-defense.md), [产品范围](../00-project/01-product-scope.md) |

## `20分钟答辩演示设计.md`

| 演示阶段 | 新文档位置 |
|---|---|
| 开场架构图 | [项目总览](../00-project/00-overview.md), [系统架构](../01-architecture/00-system-architecture.md) |
| 技术选型与工业对标 | [技术选型与工业对标](../01-architecture/02-technology-selection-and-benchmark.md) |
| 热路径 | [热出价闭环](../03-backend/01-bid-decision-closed-loop.md) |
| 持久性分层 | [数据一致性](../01-architecture/01-data-consistency.md), [Kafka 结算](../03-backend/02-kafka-settlement-closed-loop.md) |
| 失败关闭 | [Redis 恢复](../03-backend/03-redis-loss-recovery.md) |
| Live Demo | [产品范围](../00-project/01-product-scope.md), [PC 闭环](../05-frontend/02-pc-console-closed-loop.md), [H5 闭环](../05-frontend/01-mobile-h5-closed-loop.md) |
| 技术深挖 | [代码地图](code-map.md), [领域模型](../02-domain/00-domain-model-and-rules.md) |
| 正确性证据 | [证据映射](../07-performance-and-evidence/00-evidence-map.md) |
| 诚实边界 | [答辩索引](../09-judge-defense/00-defense-index.md) |

## `评委视角终审-直播竞拍全栈系统.md`

| 终审主题 | 新文档位置 | 当前修正 |
|---|---|---|
| 真实架构 | [系统架构](../01-architecture/00-system-architecture.md) | 当前 HEAD `ab0a41d` 重新核验 |
| Lua 工程亮点 | [热出价闭环](../03-backend/01-bid-decision-closed-loop.md), [领域模型](../02-domain/00-domain-model-and-rules.md) | 保留 absolute_end、engine_seq、幂等 |
| 分布式正确性 | [数据一致性](../01-architecture/01-data-consistency.md) | 不吹 Kafka EOS |
| AI 全栈轴 | [AI 运营闭环](../03-backend/04-ai-ops-closed-loop.md) | 强调 AI 不碰交易真相 |
| 测试与性能证据 | [证据映射](../07-performance-and-evidence/00-evidence-map.md) | 明确历史性能数字边界 |
| 缺陷与攻击面 | [风险矩阵](../08-tests-and-risk/00-risk-and-abuse-matrix.md) | H5 出价无超时已在当前代码修复 |

## `附录-决策路径等价性与校验门禁映射.md`

| 附录主题 | 新文档位置 |
|---|---|
| Lua vs PG legacy 定位 | [领域模型](../02-domain/00-domain-model-and-rules.md) |
| 分支等价性 | [热出价闭环](../03-backend/01-bid-decision-closed-loop.md) |
| 延时公式差异 | [领域模型](../02-domain/00-domain-model-and-rules.md), [风险矩阵](../08-tests-and-risk/00-risk-and-abuse-matrix.md) |
| 35 门禁意义 | [证据映射](../07-performance-and-evidence/00-evidence-map.md) |
| 基础设施/持久性门禁 | [数据一致性](../01-architecture/01-data-consistency.md), [观测运维](../06-observability/00-ops-observability.md) |
