# 提案与设计

这里放“准备怎么做”的文档。它们可以指导实现，但不是当前系统的真相来源。

## 当前内容

- [Ember 项目成熟度改进路线提案](./project-maturity-improvement-roadmap.md)
- [计划文档盘点](./plan-inventory.md)
- [提案模板](./proposal-template.md)
- [`docs/plan/`](../plan/) - 进行中的功能规划与实施方案
- [`docs/archive/mvp/design.md`](../archive/mvp/design.md) - MVP 初始设计（历史资料，不代表当前实现）

## 当前剩余重点

`docs/plan/` 经过整理后，当前主要剩余内容为：

- 8 份进行中或观察期的功能实施稿
- 功能方案模板与后续新增实施稿入口

最近一次归档已将 Gateway 透明代理与 Web 访问控制方案移入 `docs/archive/plan/architecture/`；此前已将前端工程质量、115 Size 解耦、前端页面布局与设计收口、订阅手动补偿下载、项目级日志级别、管理员 API Key、settings key cache 等方案陆续迁入归档。当前 `docs/plan/` 保留 8 份进行中或观察期实施稿与模板入口。

2026-08-31 已完成前端工程质量实施稿的代码、自动化验证和正式归档；真实浏览器验收未执行且未写成已通过，具体限制和剩余计划状态见 [计划文档盘点](./plan-inventory.md)。

同日 Gateway 透明代理与 Web 访问控制方案完成 v2.0.3 受控验收并按用户明确决定归档；各项实机证据、排除项和部署确认边界已保留在归档正文与稳定参考文档中。

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
- 已完成的治理类提案会移入 `docs/archive/`，例如 [API 目录重构提案](../archive/proposal/api-directory-refactor.md)、[前端设计系统治理提案](../archive/proposal/design-system-governance.md)、[system-architecture 文档拆分提案](../archive/proposal/system-architecture-document-split.md)。

## 维护规则

- 提案一旦落地，稳定结论要提炼进 `docs/reference/` 或 `docs/runbooks/`。
- 提案如果已经完成或废弃，应移动到 `docs/archive/`，不要长期伪装成“现行文档”。
- 具体功能实现稿不要写在这里，放进 `docs/plan/` 并优先使用 [功能方案模板](../plan/plan-template.md)。
- 持续治理类文档如果已经不再指导决策，只剩历史记录，也要退出当前目录，不要长期占位。
