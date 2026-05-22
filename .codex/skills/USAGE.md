# Skills Usage

把本目录下的 skill 文件夹复制到新仓库的 `.codex/skills/`：

```text
docs/design-v2-industrial/skills/live-auction-v2-navigator
docs/design-v2-industrial/skills/live-auction-v2-plan-review
docs/design-v2-industrial/skills/live-auction-v2-code-review
docs/design-v2-industrial/skills/live-auction-v2-ui-review
docs/design-v2-industrial/skills/live-auction-v2-perf-review
docs/design-v2-industrial/skills/live-auction-v2-ship-gate
docs/design-v2-industrial/skills/live-auction-v2-tiktok-judge
docs/design-v2-industrial/skills/live-auction-v2-tiktok-test-attacker
docs/design-v2-industrial/skills/live-auction-v2-tiktok-product-auditor
```

复制后目录应类似：

```text
.codex/skills/live-auction-v2-navigator/SKILL.md
.codex/skills/live-auction-v2-plan-review/SKILL.md
...
docs/design-v2-industrial/README.md
```

这些 skills 默认从新仓库根目录读取 `docs/design-v2-industrial/`。如果文档放在其他路径，使用前先告诉 Codex 定版设计目录。

触发建议：

- 让 Codex 熟悉方案：`使用 live-auction-v2-navigator 梳理当前任务应该读哪些设计文档`
- 开工前审设计：`使用 live-auction-v2-plan-review 审查 bid/outbox/WS 方案`
- 代码变更后 review：`使用 live-auction-v2-code-review review 本次 diff`
- UI 评审：`使用 live-auction-v2-ui-review 审查 H5 竞拍页`
- 压测/性能：`使用 live-auction-v2-perf-review 审查 benchmark 设计`
- 提交/演示前：`使用 live-auction-v2-ship-gate 做 release gate`
- 评委拷打：`使用 live-auction-v2-tiktok-judge 像 TikTok 十年资深工程师/面试官一样审 P0`
- 测试攻击：`使用 live-auction-v2-tiktok-test-attacker 设计并运行恶意/边界/事故场景`
- 产品验收：`使用 live-auction-v2-tiktok-product-auditor 对照范围和代码逐项查验是否降级实现`
