# 提案与设计

这里放“准备怎么做”的文档。它们可以指导实现，但不是当前系统的真相来源。

## 当前内容

- [API 目录重构提案](./api-directory-refactor.md)
- [计划文档盘点](./plan-inventory.md)
- [提案模板](./proposal-template.md)
- [`docs/plan/`](../plan/) - 进行中的功能规划与实施方案
- [`docs/specs/design.md`](../specs/design.md) - MVP 初始设计（历史资料，不代表当前实现）

## 当前剩余重点

`docs/plan/` 经过两轮清理后，当前主要剩余两类内容：

- 持续治理类：`docs/plan/design-system-governance.md`
- 尚未落地的 `embypulse-features` P1/P2 条目

具体状态见 [计划文档盘点](./plan-inventory.md)。

## 兼容说明

- `docs/plan/` 仍保留原路径，因为现有协作流程和 AI 指令都依赖这个位置。
- `docs/specs/` 仍保留为 `specs-workflow` 输出目录，不做路径迁移；其中 `design.md` 已明确降级为历史设计资料。

## 维护规则

- 提案一旦落地，稳定结论要提炼进 `docs/reference/` 或 `docs/runbooks/`。
- 提案如果已经完成或废弃，应移动到 `docs/archive/`，不要长期伪装成“现行文档”。
- 具体功能实现稿不要写在这里，放进 `docs/plan/` 并优先使用 [功能方案模板](../plan/plan-template.md)。
