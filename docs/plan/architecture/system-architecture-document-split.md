# system-architecture 文档拆分实现方案

> 状态：四阶段已落地，待收尾归档
> 负责人：Ember
> 更新时间：2026-05-08

## 背景

`docs/system-architecture.md` 在拆分前承担了过多职责，继续在单文档内追加会直接降低协作效率：

- 拆分前主文档曾增长到 1600+ 行，AI 和人工都很难快速抓到真正关键的系统边界
- 同一文档中同时存在架构入口、模型字典、API 清单、环境变量表、部署说明和代码速查，职责混杂
- 文档内部已经出现编号重复和尾部错位，说明维护方式开始退化为“追加式记录”
- 仓库内已经存在 `docs/reference/` 和 `docs/runbooks/`，但主文档仍在重复持有它们应承接的内容

如果不做拆分，`docs/system-architecture.md` 会继续膨胀，最终既不适合作为系统入口，也不适合作为稳定真相源。当前四阶段拆分已完成，本文保留为实施过程与归档前收尾说明。

## 目标

本方案要实现：

1. 将 `docs/system-architecture.md` 收口为入口级架构文档，只保留系统边界、主链路和索引
2. 将模型字典、API 清单、前端信息架构、Bot 细节、代码模式等可膨胀内容迁移到 `docs/reference/`
3. 将环境变量和部署细节统一收口到现有 `docs/reference/` 与 `docs/runbooks/`
4. 建立后续维护边界，避免主文档再次回到“大一统”状态

## 非目标

本次明确不做：

- 不修改任何业务代码、API 行为、数据库 schema 或部署脚本
- 不在本方案中直接归档历史文档；只做现行文档职责收口
- 不为了目录整齐批量重命名无关文档；只处理职责重叠和真相源冲突

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：
  - `docs/system-architecture.md`
  - `docs/proposals/system-architecture-document-split.md`
  - `docs/reference/configuration-reference.md`
  - `docs/reference/README.md`
  - `docs/runbooks/deployment.md`
  - `docs/runbooks/deployment-environment.md`
  - `docs/runbooks/deployment-troubleshooting.md`
- 当前行为：
  - 协作入口要求 AI 在首次进入仓库、跨模块改动或涉及状态流转 / 外部集成 / 部署入口时先读 `docs/system-architecture.md`
  - 主文档当前已收口为系统入口，完整数据模型、API 端点、Web 信息架构、Bot 细节和代码模式速查均已迁移到 `docs/reference/`
  - `configuration-reference.md` 与部署 runbook 已承接配置和部署细节，主文档只保留摘要与入口
- 现有限制：
  - `docs/system-architecture.md` 路径不能改变，必须继续作为系统入口
  - 迁移过程中不能留下双份真相源
  - 主文档不能被削成只有链接的空壳，仍需保留高层系统事实

## 方案设计

### 1. 用户可见行为

- AI 和人工进入仓库时，仍然先看 `docs/system-architecture.md`
- 主文档加载成本明显下降，重点更聚焦于系统边界、主流程和索引
- 需要字段、接口、配置、部署、前端页面细节时，按主文档中的索引跳转到对应专题文档
- 未被明确要求改变的现有协作路径保持不变：主文档路径不变，`reference` 和 `runbooks` 仍是稳定事实与操作手册入口

### 2. 数据与模型

> 本次不涉及数据模型变更。

### 3. 接口与边界

本次不新增代码接口，但会重定义文档边界：

- `docs/system-architecture.md`
  - 只保留系统概览、技术栈、服务边界、核心目录结构、核心数据关系总览、关键状态流转、外部集成关系、部署拓扑总览、详细参考索引
- `docs/reference/data-model-reference.md`
  - 承接全量数据模型字段表、模型设计要点、表关系细节
- `docs/reference/api-endpoint-catalog.md`
  - 承接全量 API 端点目录与路由分组
- `docs/reference/web-information-architecture.md`
  - 承接前端页面职责、页面地图、共享组件层边界
- `docs/reference/bot-architecture-reference.md`
  - 承接 Bot 通信模式、端点、命令处理器、运行边界
- `docs/reference/code-patterns.md`
  - 承接代码模式速查表
- `docs/reference/configuration-reference.md`
  - 统一承接主文档中的环境变量与配置分层细节
- `docs/runbooks/deployment*.md`
  - 统一承接部署与排障细节

边界约束：

- 主文档只写摘要，不再保留完整表格和清单
- 专题文档保留完整事实，但不重复系统边界总述
- 同一类事实只能有一个当前真相源

### 4. 关键流程

四阶段实施主链路已经完成，保留如下作为落地记录：

1. 在 `docs/system-architecture.md` 开头新增“本文职责说明”，明确它是系统入口文档，不再承载全量枚举型事实
2. 修正主文档当前章节编号重复和尾部错位问题，先把结构恢复到可维护状态
3. 将主文档中的环境变量与部署章节压缩为摘要，并改为指向现有 `configuration-reference.md` 与部署 runbook
4. 新增 `docs/reference/data-model-reference.md`，迁移 `4. 数据模型`
5. 新增 `docs/reference/api-endpoint-catalog.md`，迁移 `6. API 端点完整列表`
6. 新增 `docs/reference/web-information-architecture.md`，迁移 `3.1 Web 共享组件层` 与 `8. 前端架构` 中的页面级内容
7. 新增 `docs/reference/bot-architecture-reference.md`，迁移 `9. Telegram Bot 架构` 的详细内容
8. 新增 `docs/reference/code-patterns.md`，迁移 `14. 代码模式速查`
9. 更新 `docs/reference/README.md`，把新增专题文档纳入入口
10. 更新协作入口文档中的文档职责描述，确保入口说明与拆分后的事实一致
11. 全量检查交叉引用，删除主文档与专题文档之间的重复正文

### 5. 失败路径与边界条件

- 主文档被压得过薄：如果删到只剩链接页，立即回补系统边界、关键流程和外部集成摘要
- 出现双份真相源：如果同一表格或同一清单同时保留在主文档和专题文档，必须优先删除主文档中的完整版本，只保留摘要
- 专题文档职责重叠：如果新增文档与现有 `configuration-reference.md` 或部署 runbook 内容冲突，应优先合并到现有文档，而不是继续新建平行文档
- 旧链接失效：迁移正文时，所有被主文档删除的章节都必须补上新路径链接
- 兼容性约束：`docs/system-architecture.md` 的路径、入口角色和高层事实必须保持稳定，不能破坏现有协作入口

## 影响范围

涉及的子系统：

- API：无
- Web：无
- Bot：无
- 配置/部署：无代码变更；仅文档边界调整
- 文档：
  - `docs/system-architecture.md`
  - `docs/reference/README.md`
  - `docs/reference/configuration-reference.md`
  - `docs/runbooks/deployment.md`
  - `docs/runbooks/deployment-environment.md`
  - `docs/runbooks/deployment-troubleshooting.md`
  - `AGENTS.md` / `CLAUDE.md` / `GEMINI.md`
  - 新增：
    - `docs/reference/data-model-reference.md`
    - `docs/reference/api-endpoint-catalog.md`
    - `docs/reference/web-information-architecture.md`
    - `docs/reference/bot-architecture-reference.md`
    - `docs/reference/code-patterns.md`

## 验证方式

### 编译/测试

本次为纯文档治理改动，不涉及代码编译。

需要执行的文档一致性检查：

- 检查新增文档路径是否符合 `kebab-case`
- 检查 `docs/reference/README.md`、`docs/plan/README.md`、相关协作入口文档是否已同步入口
- 检查主文档到专题文档的交叉引用是否可达

### 手工验证

- 从零开始阅读：首次只打开 `docs/system-architecture.md`，确认能在 5 分钟内理解系统边界并找到专题文档入口
- 模型查阅：需要查询字段含义时，确认能从主文档跳到 `data-model-reference.md`
- 接口查阅：需要查路由时，确认能从主文档跳到 `api-endpoint-catalog.md`
- 配置查阅：需要查环境变量时，确认主文档不再重复列大表，而是指向 `configuration-reference.md`
- 部署查阅：需要看部署细节时，确认主文档只保留摘要，并能跳到对应 runbook
- 结构检查：确认主文档不存在重复编号、尾部错位和孤立章节

## 落地后文档处理

落地后应同步处理：

- 将稳定后的文档边界说明提炼回 `docs/system-architecture.md` 开头与 `docs/reference/README.md`
- 本实施稿在拆分全部完成、引用收口、主文档稳定后移入 `docs/archive/plan/architecture/`
- 对应的治理提案 [docs/proposals/system-architecture-document-split.md](../../proposals/system-architecture-document-split.md) 在不再指导决策后移入 `docs/archive/`
