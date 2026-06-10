# Live Auction 评审文档入口

> 本目录只保留面向评委/答辩的最终设计、S1-S5 评测材料和运行说明。

## 推荐阅读顺序

1. [judge/01-final-review.md](judge/01-final-review.md)  
   项目总览、核心亮点、工程边界和答辩口径。
2. [design/01-architecture.md](design/01-architecture.md)  
   Redis Lua 热决策、Kafka WAL、PostgreSQL 结算真相和 Reconciler 闭环。
3. [s1-s5/00-overview.md](s1-s5/00-overview.md)  
   S1-S5 评测场景、指标、工具链和证据口径。
4. [judge/02-s1-s5-gate-mapping.md](judge/02-s1-s5-gate-mapping.md)  
   决策路径、35 条校验门禁和代码机制映射。
5. [judge/03-demo-script.md](judge/03-demo-script.md)  
   演示流程。

## 目录结构

| 目录 | 内容 |
|---|---|
| `design/` | 最新架构合同、性能/正确性边界、运行配置、证据策略、告警 runbook |
| `s1-s5/` | S1 绝杀、S2 稳态、S3 围观、S4 故障、S5 重连的评测设计 |
| `judge/` | 面向评委的总览报告、校验门禁映射和演示脚本 |
| `setup_guide.md` | 本地启动和环境配置 |

## 证据边界

`docs/` 不承载大体积原始压测文件、截图、录制脚本或临时调试记录。如需复跑证据，以 `tests/pts/`、`tests/load/`、`tests/chaos/` 中的脚本和 [s1-s5/12-readiness-checklist.md](s1-s5/12-readiness-checklist.md) 为准。
