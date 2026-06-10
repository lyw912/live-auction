# Skills Usage

把本目录下的 skill 文件夹复制到新仓库的 `.codex/skills/`：

```text
.codex/skills/live-auction-v2-navigator
.codex/skills/live-auction-v2-plan-review
.codex/skills/live-auction-v2-code-review
.codex/skills/live-auction-v2-ui-review
.codex/skills/live-auction-v2-perf-review
.codex/skills/live-auction-v2-stress-attacker
.codex/skills/live-auction-v2-ship-gate
.codex/skills/live-auction-v2-tiktok-judge
.codex/skills/live-auction-v2-tiktok-test-attacker
.codex/skills/live-auction-v2-tiktok-product-auditor
```

复制后目录应类似：

```text
.codex/skills/live-auction-v2-navigator/SKILL.md
.codex/skills/live-auction-v2-plan-review/SKILL.md
...
docs/design-v2-industrial/README.md
docs/current/README.md
```

这些 skills 当前默认先读取 `docs/README.md`，再按任务读取 `docs/design/`、`docs/s1-s5/` 和 `docs/judge/`。旧的 `docs/current/`、`docs/perf/`、`docs/reviews/`、`docs/design-v2-industrial/` 已不再作为当前权威文档保留。

触发建议：

- 让 Codex 熟悉方案：`使用 live-auction-v2-navigator 梳理当前任务应该读哪些设计文档`
- 开工前审设计：`使用 live-auction-v2-plan-review 审查 bid/outbox/WS 方案`
- 代码变更后 review：`使用 live-auction-v2-code-review review 本次 diff`
- UI 评审：`使用 live-auction-v2-ui-review 审查 H5 竞拍页`
- 压测/性能：`使用 live-auction-v2-perf-review 审查 benchmark 设计`
- 压力攻击/找瓶颈：`使用 live-auction-v2-stress-attacker 主动打压 PG/outbox/WS 并归因`
- 提交/演示前：`使用 live-auction-v2-ship-gate 做 release gate`
- 评委拷打：`使用 live-auction-v2-tiktok-judge 像 TikTok 十年资深工程师/面试官一样审 P0`
- 测试攻击：`使用 live-auction-v2-tiktok-test-attacker 设计并运行恶意/边界/事故场景`
- 产品验收：`使用 live-auction-v2-tiktok-product-auditor 对照范围和代码逐项查验是否降级实现`

压测、性能审查、发布闸门、评委拷打类任务必须先确认 `docs/design/03-runtime-profiles.md`：

- `.env.example` 只代表本地 demo profile。
- PTS-1B 必须使用 `.env.pts1b.example` 或 `tests/pts/MANIFEST.md` 的 reset/preflight/verify 流程。
- PTS-1B 证据必须记录 `BID_ENGINE_MODE=redis_ledger`、`ADMISSION_ENABLED=false`、Redis 和 Kafka 配置来源。
