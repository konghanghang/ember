# system-architecture 文档拆分提案

> 状态：四阶段已落地，待收尾归档
> 负责人：Ember
> 更新时间：2026-05-08

## 问题

`docs/system-architecture.md` 在拆分前同时承担了入口架构文档、事实字典、运维手册和代码速查表四类职责，已经超出单文档可维护边界：

- 拆分前文档曾增长到 1600+ 行，继续追加会显著提高 AI 和人工加载成本
- 同一文档内混合系统边界、模型字段表、API 清单、环境变量表、部署细节，职责不清
- 章节已经出现编号重复和结构错位，说明维护方式开始退化为“追加式记录”
- 现有 `docs/reference/` 和 `docs/runbooks/` 已能承接部分内容，但主文档仍在重复持有这些事实

## 目标

这个提案要解决：

1. 将 `docs/system-architecture.md` 收口为入口级架构文档，只保留系统边界、主链路和索引
2. 将持续膨胀的事实字典和运维细节拆分到 `docs/reference/` 与 `docs/runbooks/`
3. 建立后续维护规则，避免主文档再次退化为“大一统文档”

## 不做的事

- 不在本提案中直接改业务代码或接口行为
- 不为了目录整齐做无意义批量搬迁；只处理职责重叠和真相源冲突

## 当前状态

用事实描述现状，不写空话：

- 当前主文档为 [docs/system-architecture.md](../system-architecture.md)，总长度约 834 行
- 主文档当前已收口为系统入口，仍包含系统概览、目录结构、后端服务、前端与 Bot 总览、定时任务、配置/部署摘要；完整数据模型、API 端点、Web 信息架构、Bot 细节与代码模式速查均已迁出
- 主文档中的全量环境变量表与部署细节已压缩为摘要，并指向 [docs/reference/configuration-reference.md](../reference/configuration-reference.md) 与相关 runbook
- 完整数据模型与 API 端点清单已迁移到 [docs/reference/data-model-reference.md](../reference/data-model-reference.md) 与 [docs/reference/api-endpoint-catalog.md](../reference/api-endpoint-catalog.md)
- 前端信息架构、Bot 细节与代码模式速查也已迁移到对应 `reference` 文档
- 文档内部的编号重复与尾部错位问题已收口
- 当前协作入口仍要求“先读 `docs/system-architecture.md`”，因此该文档必须继续保留为仓库入口，而不是被彻底替代

## 当前进度

### 已完成项

- 已完成当前主文档体量、章节结构和重复职责盘点
- 已确认 `docs/reference/` 与 `docs/runbooks/` 已有部分文档可直接承接拆出内容
- 已在 `docs/system-architecture.md` 补入文档职责说明，明确主文档只做系统入口
- 已修正主文档中的编号重复和尾部错位问题
- 已将环境变量与部署章节压缩为摘要，并改为指向现有 `reference` / `runbooks`
- 已新增 `docs/reference/data-model-reference.md`，承接完整数据模型字典
- 已新增 `docs/reference/api-endpoint-catalog.md`，承接完整 API 端点目录
- 已将主文档中的 `4. 数据模型` 与 `6. API 端点完整列表` 压缩为摘要入口
- 已新增 `docs/reference/web-information-architecture.md`，承接前端信息架构与共享组件层细节
- 已新增 `docs/reference/bot-architecture-reference.md`，承接 Bot 运行模式、端点、命令与环境变量说明
- 已将主文档中的 `3.1 Web 共享组件层`、`8. 前端架构`、`9. Telegram Bot 架构` 压缩为摘要入口
- 已新增代码模式承接文档；其后又按边界收口回 `docs/reference/api-development-conventions.md` 与 `docs/reference/api-response-standard.md`
- 已将主文档中的 `14. 代码模式速查` 压缩为摘要入口

### 剩余项

- 评估是否还需要继续压缩主文档体量
- 完成归档前的最后一轮引用盘点与事实确认
- 满足条件后将本提案移入 `docs/archive/`

## 提案内容

### 1. 核心原则

- `docs/system-architecture.md` 只做系统入口，不再承载全量枚举型事实
- 稳定事实字典放 `docs/reference/`
- 运维与部署操作细节放 `docs/runbooks/`
- 主文档保留摘要和索引，专题文档保留完整事实，二者职责不重叠
- 迁移后必须修正引用和入口，不能留下双份真相源

### 2. 具体调整

- 主文档保留：系统概览、技术栈、服务边界、核心目录结构、核心数据关系总览、关键状态流转、外部集成关系、部署拓扑总览、详细参考索引
- 主文档删除全量字段表、全量 API 清单、全量环境变量表、细粒度部署参数说明、代码模式速查
- `4. 数据模型` 拆到 `docs/reference/data-model-reference.md`
- `6. API 端点完整列表` 拆到 `docs/reference/api-endpoint-catalog.md`
- `8. 前端架构` 中页面地图、页面职责和共享组件细节拆到 `docs/reference/web-information-architecture.md`
- `9. Telegram Bot 架构` 详细内容拆到 `docs/reference/bot-architecture-reference.md`
- `11. 环境变量完整列表` 从主文档移除，统一并入现有 [docs/reference/configuration-reference.md](../reference/configuration-reference.md)
- `13. 部署` 的细节说明下沉到现有 `docs/runbooks/deployment*.md`
- `14. 代码模式速查` 初始拆到独立文档，后续再按边界并回 `docs/reference/api-development-conventions.md` 与 `docs/reference/api-response-standard.md`
- 更新 [docs/reference/README.md](../reference/README.md)，将新增专题文档纳入入口

### 3. 分阶段推进

#### 第一阶段

- 在主文档开头补“本文职责说明”
- 修正当前章节编号漂移和尾部错位问题
- 将环境变量章节改为简短配置分层说明，并链接到 `configuration-reference.md`
- 将部署章节改为部署拓扑与关键约束摘要，并链接到相关 runbook

#### 第二阶段

- 新增 `docs/reference/data-model-reference.md`
- 新增 `docs/reference/api-endpoint-catalog.md`
- 将主文档中的模型与 API 章节压缩为摘要 + 链接

#### 第三阶段

- 新增 `docs/reference/web-information-architecture.md`
- 新增 `docs/reference/bot-architecture-reference.md`
- 将主文档中的前端与 Bot 章节压缩为高层职责 + 链接

#### 第四阶段

- 新增代码模式承接文档（当前已并回 `docs/reference/api-development-conventions.md` 与 `docs/reference/api-response-standard.md`）
- 更新 `docs/reference/README.md`
- 检查并修正所有直接引用 `docs/system-architecture.md` 旧章节内容的文档
- 更新协作入口文档中对主文档职责的描述

## 风险与约束

- 风险 1：拆分后出现双份真相源
  - 控制方式：主文档只保留摘要，不保留完整表格和清单
- 风险 2：旧链接失效
  - 控制方式：迁移时同步修正文档内直接引用和 README 入口
- 风险 3：主文档拆得过碎，反而失去入口价值
  - 控制方式：保留高层边界、主流程和索引，不把主文档降格为“空跳转页”
- 兼容性要求：`docs/system-architecture.md` 必须继续作为 AI 和人工进入仓库时的首要系统入口，路径不变

## 影响范围

- 文档：
  - [docs/system-architecture.md](../system-architecture.md)
  - [docs/reference/README.md](../reference/README.md)
  - [docs/reference/configuration-reference.md](../reference/configuration-reference.md)
  - `docs/reference/data-model-reference.md`
  - `docs/reference/api-endpoint-catalog.md`
  - `docs/reference/web-information-architecture.md`
  - `docs/reference/bot-architecture-reference.md`
  - `docs/reference/api-development-conventions.md`
  - `docs/reference/api-response-standard.md`
  - `docs/runbooks/deployment.md` 及相关部署 runbook
  - 协作入口文档 `AGENTS.md`、`CLAUDE.md`、`GEMINI.md`
- 流程：
  - 后续系统边界类改动更新主文档
  - 字段/API/配置/部署细节改动更新对应专题文档
- 兼容性：
  - `docs/system-architecture.md` 路径保持不变
  - 主文档中原有高价值结论需要保留摘要，不可直接删空

## 完成标准

- `docs/system-architecture.md` 收口到 700 行以内
- 主文档不再包含全量数据模型字段表
- 主文档不再包含全量 API 端点清单
- 主文档不再包含全量环境变量表
- 主文档不再包含细粒度部署参数和代码模式速查
- `docs/reference/README.md` 已补齐新增专题文档入口
- 所有迁移涉及的直接引用路径已修正
- 文档编号不再重复、章节结构不再错位

## 归档条件

- 主文档拆分落地完成，新增专题文档已稳定承接各自职责
- 协作入口、reference 入口、runbook 入口均已同步到当前事实
- 后续维护规则已稳定执行，不再依赖本提案指导日常更新

## 落地后文档处理

- 若需要具体实施顺序和逐文档迁移清单，可继续补一份 `docs/plan/architecture/` 下的实施稿
- 拆分完成后，将稳定结论固化到 `docs/system-architecture.md`、`docs/reference/`、`docs/runbooks/`
- 本提案完成历史使命后，移入 `docs/archive/`
