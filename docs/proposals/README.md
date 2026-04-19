# 提案与设计

这里放“准备怎么做”的文档。它们可以指导实现，但不是当前系统的真相来源。

## 当前内容

- [计划文档盘点](./plan-inventory.md)
- [提案模板](./proposal-template.md)
- [`docs/plan/`](../plan/) - 进行中的功能规划与实施方案
- [`docs/archive/mvp/design.md`](../archive/mvp/design.md) - MVP 初始设计（历史资料，不代表当前实现）

## 当前剩余重点

`docs/plan/` 经过整理后，当前主要剩余内容为：

- 进行中的功能实施稿
- 功能方案模板与后续新增实施稿入口

最近一次归档已将注册码绑定套餐分组方案移入 `docs/archive/plan/billing-redemption/`，并将缺集管理与精准补集方案移入 `docs/archive/plan/media-subscription/`；当前 `docs/plan/` 继续保留仍在推进中的实施稿。

具体状态见 [计划文档盘点](./plan-inventory.md)。

## 目录边界

放在 `docs/proposals/`：

- 文档治理、目录治理、流程治理提案
- 盘点清单、重构策略、收口方案
- 跨阶段决策文档，还没下沉到具体接口/页面/模型实施稿

放在 `docs/plan/`：

- 已明确到接口、模型、页面、路由、验收方式的实施稿
- 准备直接进入开发的功能方案

不该继续留在这两个目录：

- 已完成且只剩历史追溯价值：移入 `docs/archive/`
- 已稳定为现行规则或系统事实：提炼进 `docs/reference/`、`docs/runbooks/` 或 `docs/system-architecture.md`

## 兼容说明

- `docs/plan/` 仍保留原路径，因为现有协作流程和 AI 指令都依赖这个位置。
- 已完成的治理类提案会移入 `docs/archive/`，例如 [API 目录重构提案](../archive/proposal/api-directory-refactor.md)、[前端设计系统治理提案](../archive/proposal/design-system-governance.md)。

## 维护规则

- 提案一旦落地，稳定结论要提炼进 `docs/reference/` 或 `docs/runbooks/`。
- 提案如果已经完成或废弃，应移动到 `docs/archive/`，不要长期伪装成“现行文档”。
- 具体功能实现稿不要写在这里，放进 `docs/plan/` 并优先使用 [功能方案模板](../plan/plan-template.md)。
- 持续治理类文档如果已经不再指导决策，只剩历史记录，也要退出当前目录，不要长期占位。
