# 数据库迁移 Baseline 与归档收口方案

> 状态：已完成
> 负责人：Ember
> 更新时间：2026-05-01
>
> **后续动态**：`cmd/migrate` 工具已在 v1.4.x 由「启动期内嵌自动迁移方案」收口删除，本地空库初始化改由 `go run ./cmd/server` 接管，生产升级路径精简为 `docker compose pull && up -d`。详见 [数据库迁移自动应用方案](./database-migration-auto-apply.md)。

## 背景

`infrastructure/database/` 顶层正在持续累积迁移 SQL。当前数量还没到不可维护，但目录已经开始同时承担三种职责：

- 空数据库首次初始化入口
- 已有数据库手工增量升级入口
- 历史 schema 变更追溯入口

如果继续无限平铺，运维和发布说明会越来越难维护；如果为了“目录整齐”直接把旧 SQL 挪走，又会破坏现有初始化和升级链路。

## 目标

本方案要实现：

1. 为 `infrastructure/database/` 建立可长期维护的 baseline + 增量迁移结构
2. 明确旧迁移何时允许归档，以及归档前必须满足的条件
3. 保持现有空库初始化、已有库升级、发布提醒三条链路可用

## 非目标

本次明确不做：

- 不改业务表结构，不新增模型字段或索引
- 不引入新的 migration 执行器或数据库版本管理工具
- 不做“只为路径整齐”的批量迁移文件搬运

## 当前事实

以当前代码和现行文档为准，现状如下：

- 相关目录：`infrastructure/database/`
- 相关部署入口：`infrastructure/docker/docker-compose.yml`
- 相关文档：`infrastructure/database/README.md`、`docs/runbooks/deployment.md`、`docs/runbooks/deployment-environment.md`、`docs/runbooks/release-process.md`
- 当前行为：
  - 现行迁移入口已经演进为 `infrastructure/database/20260422_00_schema_baseline.sql` + baseline 之后的顶层增量 migration。
  - `pre-20260415` 与 `pre-20260422` 历史 migration 已移动到 `infrastructure/database/archive/` 下对应目录，仅保留追溯价值。
  - API 启动期不再调用 `AutoMigrate`，启动会执行 `VerifySchema` 校验表、关键列和关键索引。
  - 本地空库初始化统一走 `cd services/api && go run ./cmd/migrate`，该工具会执行顶层 baseline + 增量 migration，再跑 `VerifySchema` 自检。
- 现有限制：
  - 线上长期以 `AUTO_MIGRATE=false` 运行，不能依赖 GORM 自动迁移
  - SQL migration 现行规则要求放在 `infrastructure/database/`
  - 顶层历史文件一旦直接迁移到别处，空库初始化和文档示例路径都会失真

## 方案设计

### 1. 用户可见行为

- 不改变业务功能和 API 行为
- 空数据库首次部署仍可通过 compose 初始化
- 已有数据库升级仍保持“显式执行 SQL，再启动服务”的流程
- 未被新 baseline 覆盖的增量迁移，继续按现行命名规则放在顶层

### 2. 数据与模型

> 本次不涉及数据模型变更。

本次只调整迁移资产组织方式和使用规则：

- 新增一份 schema baseline SQL，放在 `infrastructure/database/` 顶层
- baseline 之前的历史迁移在验证通过后移动到 `infrastructure/database/archive/<baseline-cutoff>/`
- baseline 之后的增量迁移继续保留在 `infrastructure/database/` 顶层

本轮实际收口结果：

- baseline 截点：`20260414_01_add_redemption_code_registration_plan_group.sql`
- baseline 文件：`20260415_00_schema_baseline.sql`
- 历史归档目录：`infrastructure/database/archive/pre-20260415/`
- baseline 验证：已在远端临时库 `ember_baseline_verify_20260415` 完整回灌
- 有意归一化差异：补齐 `idx_ranking_lookup` 与 `uq_redemptions_user_code`

后续演进：

- `schema-deployment-and-baseline-cleanup` 已将现行 baseline 收口到 `20260422_00_schema_baseline.sql`，并同步了 initdb 隔离、`VerifySchema` fail-fast、容器启动边界和部署文档。
- 因此本文不再承担当前迁移规则说明职责，当前规则以 `infrastructure/database/README.md`、`docs/runbooks/deployment-environment.md` 和 `docs/system-architecture.md` 为准。

### 3. 接口与边界

- 不新增 API、Internal API、webhook 或 CLI 接口
- 调整的是迁移文件目录契约：
  - 顶层：仅保留当前 baseline 和 baseline 之后仍需执行的增量迁移
  - `archive/`：仅保留已被 baseline 完整覆盖的历史迁移，供追溯使用
- 发布与部署文档必须继续指向“顶层可执行入口”，不能要求运维去 archive 手工挑文件

### 4. 关键流程

1. 选定 `20260414_01_add_redemption_code_registration_plan_group.sql` 作为首个 baseline 截点
2. 从现网可用 schema 导出 `schema-only` 基线初稿，并补齐 deterministic seed
3. 额外补齐源库缺失但迁移契约仍要求存在的两条索引
4. 在远端临时库完整回灌 baseline，确认空库可独立初始化
5. 将 baseline 之前的历史迁移整体移动到 `archive/`，保留原文件名和时间顺序

### 5. 失败路径与边界条件

- baseline 漏掉索引、约束或初始化数据：不得归档旧迁移，继续以现有平铺结构为准
- 发布脚本仍只扫描顶层 `*.sql`，而新流程又把可执行迁移挪进子目录：会导致升级提醒失真，必须先调整脚本或保持顶层入口不变
- 文档只更新 README、不更新部署和发布说明：会造成运维执行路径和仓库事实不一致
- 兼容性约束：不能破坏现有空数据库初始化流程，不能让已有数据库升级依赖 archive 中的历史 SQL

## 影响范围

涉及的子系统：

- API：无功能变更
- Web：无
- Bot：无
- 配置/部署：有，需要调整数据库初始化与迁移说明
- 文档：需要更新 `infrastructure/database/README.md`、`docs/runbooks/deployment.md`、`docs/runbooks/deployment-environment.md`、`docs/runbooks/release-process.md`

## 验证方式

### 编译/测试

- 本方案阶段不涉及编译产物变更，无需额外构建命令

### 手工验证

- 已完成：远端临时库 `ember_baseline_verify_20260415` 仅执行 baseline，成功创建核心表、索引、约束和 deterministic seed。
- 已完成：与现网源库做 `schema-only` diff，确认差异仅为有意补齐的 `idx_ranking_lookup` 和 `uq_redemptions_user_code`。
- 已完成：`infrastructure/database/README.md` 已收口当前 baseline + 增量 migration 规则。
- 已完成：`docs/system-architecture.md` 已同步启动期不再调用 `AutoMigrate`、本地空库初始化入口和 `VerifySchema` 约束。

## 落地后文档处理

落地后应同步处理：

- 将新的迁移组织规则提炼到 `infrastructure/database/README.md`
- 将部署入口和升级流程的稳定结论同步到 `docs/runbooks/deployment.md` 与 `docs/runbooks/deployment-environment.md`
- 如果发布提醒规则变化，更新 `docs/runbooks/release-process.md`
- 当前方案已完成，稳定事实已被后续 schema 收口方案和现行架构文档接管；后续只需在文档治理阶段移入 `docs/archive/plan/architecture/`。
