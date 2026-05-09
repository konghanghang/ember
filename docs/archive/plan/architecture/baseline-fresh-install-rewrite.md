# Baseline 重建对齐方案

> 状态：已归档（落地于 2026-05-09）
> 负责人：Ember
> 更新时间：2026-05-09

## 背景

`infrastructure/database/00000000_baseline_20260502.sql` 当前是**合并式 baseline**——按字典序把 24 份历史顶层 SQL（v1.3.1 截点 baseline + 23 个 v1.4.0 期间增量）原样拼接而成。

2026-05-09 全新部署排错过程中暴露了两个连锁事实：

1. **第一个表面 bug**：`plan_groups` 表带历史遗留 `id varchar(25) NOT NULL` 列，但 GORM 模型把主键迁到 `key` 后未同步 baseline，新空库部署在 baseline 自带 seed `INSERT` 处直接报 NOT NULL 约束（已通过 commit `700eb31` 单点修复）。
2. **更深层结构问题**：进一步对照 GORM 模型与 baseline 后发现，`plan_groups.id` 不是孤例。**整份 baseline 与运行中 prod schema、当前 GORM 模型严重脱节**。已识别的差异覆盖至少 6 张表、>15 个字段，包括列名不同、列存在性不同、类型/大小不同等多种情形。

`services/api/logs/app-2026-04-09.log` 中真实 PG 返回成功的查询：

```sql
SELECT ... item_source_type, item_name, play_count, ... FROM playback_rankings ...
-- 结果：[rows:0]，查询成功
```

如果 prod 库按 baseline 字段（`item_type` / `metric_value`）建表，这条 SQL 会直接 `column does not exist`。所以**生产 schema 跟 GORM 模型一致，跟 baseline 不一致**。

最可能的成因：项目早期开过 GORM `AutoMigrate`，让真实 schema 漂移到与模型一致；后来 `AUTO_MIGRATE=false` 锁死，但 baseline 是从某个更早的快照来的，没跟上模型与运行库的演进。

后果：当前 baseline 已**长期无法用于"新空库"路径**。任何尝试做 fresh install 的人都会在第一条业务查询时炸掉，但因为没人做 fresh install（已部署实例都靠 `schema_migrations` 跳过 baseline），所以一直没暴露。今天 `plan_groups.id` 是这条问题链路的第一个公开信号。

项目计划在完成所有检测后开源。开源后第一波尝试 fresh install 的外部用户**必然踩这条链**。开源前必须把 baseline 重建到能真正初始化出"和 prod 等价的可用 schema"。

## 目标

1. 把 `00000000_baseline_20260502.sql` 重建为**与运行中 prod schema 字段级等价**的 fresh-install baseline，让新空库执行后立刻能被 GORM 模型查询、写入。
2. 形态采用 fresh-install：所有语句在新空库上都是有效语义，不存在 no-op，不携带历史包袱。
3. 截点不变（仍代表 v1.4.0 截点的 schema 状态），文件名不变（仍是 `00000000_baseline_20260502.sql`）。
4. 同步更新 `infrastructure/database/README.md` 与 `docs/runbooks/database-migration-baseline.md`，把"合并式 baseline"段重写为"重建式 fresh-install baseline"，并明确"为何旧 baseline 实际不可用"作为开源前必读说明。

## 非目标

- 不改任何 GORM 模型：`services/api/internal/models/*.go` 在本方案中是只读真相源（与 dump 一起作为对照基准）。
- 不改启动期 Migrate / Bootstrap / VerifySchema 链路：`schema_migrations` 记账、advisory lock、新空库分支判断、rename 豁免、多份 baseline 防御保持不变。
- 不改 `services/api/internal/db/db.go` 的 `schemaFingerprintColumns` / `schemaFingerprintIndexes`：这些指纹仍然是当前模型与运行库的真相，不需要因 baseline 重建而调整。
- 不动 `archive/pre-20260502/` 目录：历史合并源已在归档，追溯路径不变。
- 不做下一轮 baseline 截点推进（不会出现 `00000000_baseline_20260509.sql`）。
- 不修线上 prod 库 schema：prod 库 schema 已经是正确的（漂移到模型一致状态），本方案是把 baseline 拉回到 prod，而不是把 prod 改造对齐 baseline。
- **不强求"旧 baseline 与新 baseline schema diff 零差异"**：已知它们必然不等价，等价是这次任务的反目标。

## 当前事实

- 现行 baseline：`infrastructure/database/00000000_baseline_20260502.sql`（1630 行；已通过 commit `700eb31` 修复 `plan_groups.id`）
- 现行 README：`infrastructure/database/README.md`（"现行 baseline 说明"段把合并式当作设计选择文档化）
- 现行 runbook：`docs/runbooks/database-migration-baseline.md`（"本轮选择合并式 baseline 的边界"段为本轮历史决策记录）
- GORM 模型：`services/api/internal/models/` 共 21 个模型文件，对应 24 张 `tableChecks`（含 `tv_calendar` 多表）
- 运行中 dev/prod PG 实例的实际 schema：与 GORM 模型一致（间接证据：日志中的 SQL 查询命中真实列名）
- 启动期 Migrate 链路：`services/api/internal/db/migrate.go`、对应回归测试 `migrate_test.go` 已覆盖五种分支判断
- 已识别的 baseline ↔ 模型差异（不完全清单，详见对照表附录）：
  - `users.expiry_date` ↔ `expires_at`（列名）
  - `users.emby_username` 多余 / 缺 `emby_disabled`
  - `users.emby_id` 类型 (255 vs 50)
  - `plans` 缺 `currency` / `sort_order`
  - `plans.description` 类型 (255 nullable vs 500)
  - `redemption_codes.note` ↔ `notes`（列名 + 类型）
  - `redemption_codes` 多余 `is_active` / 缺 `default_days`
  - `redemption_codes.code` / `redemptions.code` 长度 (50 vs 20)
  - `redemptions` 多余 `redeemed_at` / `old_expiry_date` / `new_expiry_date`
  - `playback_rankings.item_type` ↔ `item_source_type`（列名）
  - `playback_rankings.metric_value` ↔ `play_count`（列名）
  - `playback_rankings.item_key` / `item_name` 长度
  - `playback_rankings` 多余 `user_count`

## 方案设计

### 1. 用户可见行为

- **新空库部署**：行为变化（修复型）。`docker compose up -d` 触发 Migrate 进入"新空库"分支，按字典序跑 `00000000_baseline_20260502.sql`，schema 终态对齐 GORM 模型，启动后业务查询立刻可用。**这是当前"事实上的破窗"修复**。
- **老库升级**：行为不变。`schema_migrations` 已记账老 baseline → forward-only 分支识别新 baseline 文件名命中 `baselineFilenamePattern` → 走 rename 豁免，仅写记账行、不重跑 baseline。文件 checksum 会变（内容大改），但豁免逻辑判断与 checksum 无关。
- **CI / 测试**：`migrate_test.go` 现有用例继续有效（验证 schema 终态而非 baseline 文本形态）。

### 2. 数据与模型

不涉及 GORM 模型变更。重建 baseline 的所有 DDL 必须 1:1 对应：

- 优先级 1：dev/prod PG 实例的真实 schema（pg_dump --schema-only 输出）
- 优先级 2：GORM 模型当前定义（用于解释字段 / 索引语义）
- 优先级 3：`db.go` 的 `schemaFingerprint*` 列表（必须全部命中）
- 优先级 4：现行 baseline 中已被验证为与现状一致的部分（含已通过 commit 700eb31 修复的 `plan_groups`）

deterministic seed 必须完整保留：

- `settings` 5 条默认配置（`default_trial_days` / `registration_mode` / `notify_group_link` / `email_verification` / `stripe_allowed_payment_methods`）
- `plan_groups.DEFAULT` 默认套餐分组

### 3. 接口与边界

本方案不涉及任何 API / Internal API / webhook / 命令变更。

### 4. 关键流程

1. **拿到真相快照**：用户在能跑通业务查询的 PG 实例上跑 `pg_dump --schema-only --no-owner --no-privileges -d <db>` 并贴回结果；或授权我读取一份本地 dump 文件。
2. **三方对齐审计**：以 dump 为优先级 1，模型为优先级 2，逐表生成"权威字段表"，列出每个字段的最终决定值（类型、nullability、default、注释）和决定理由。每个差异点都要明确选择哪一方为准并说明依据。
3. **新 baseline 起草**：基于权威字段表写到 `00000000_baseline_20260502.sql.new`，结构按 fresh-install 形态分四段：
   - ENUM 类型（仅保留 `media_type` / `subscription_status`，并核对 prod 是否真的还在用）
   - `CREATE TABLE` 段（按字典序排列 24 张表）
   - `ALTER TABLE ... ADD CONSTRAINT` 段（PK / UNIQUE / FK，FK 当前未使用）
   - `CREATE INDEX` 段（含 partial / functional / 复合）
   - deterministic seed `INSERT` 段
4. **静态自检报告**：列出
   - 每张表 / 每个非 PK 索引 / 每个非平凡约束在新 baseline 中的位置
   - 每个被弃用的 baseline 段（边界注释、historical DML、camelCase rename DO 块、prod-dump 残留字段）的删除依据
   - 每个 fingerprint 列 / 索引在新 baseline 中是否命中
   - 每张表的字段集合是否与对应 GORM 模型的 `column:` tag 完全一致
5. **用户 PG 验证**：用户在干净 PG 实例上跑新 baseline，dump 后与"真相快照"做字典级 diff（`information_schema.columns` / `pg_indexes` / `pg_constraint` 三视图全 diff 零差异）。
6. **运行时验证**：用户在 `ember_new` 库上启动 `ember-api`，确认 `Migrate → VerifySchema → Bootstrap` 全程通过，且至少能成功响应一次 `/health` 与一次任意业务列表 API。
7. **文档同步**：
   - `infrastructure/database/README.md` 重写"现行 baseline 说明"段，从"合并式"切到"重建式 fresh-install"，新增"为何旧 baseline 不可用"小节作为开源前必读。
   - `docs/runbooks/database-migration-baseline.md` 中"本轮选择合并式 baseline 的边界"段保留为历史记录，追加 "2026-05-09 重建为 fresh-install 形态" 说明，补"重建动因：baseline ↔ prod 脱节"。
8. **提交 2**：把新 baseline + README + runbook + plan 文档 + plan 清单更新打成一个提交。

### 5. 失败路径与边界条件

- **dump 与模型存在剩余分歧**：极小概率（dev 库可能有手工补丁的脏字段）。处理方式：每一处先列入对照表，决策"以 dump 为准"或"以模型为准"，不留模糊。
- **dump 包含 prod-only 字段（如临时调试列）**：明确剔除，并在自检报告中标注。
- **fingerprint 列 / 索引在 dump 中缺失**：表明 prod 也缺，需要先单独立项补 migration 把 prod 拉齐，本方案暂停等待。
- **新 baseline 漏字段 / 漏索引**：用户 PG 双跑 diff 阶段会发现，按 diff 修订；任何遗漏都不允许进主干。
- **老库升级 checksum 不一致**：rename 豁免逻辑（`baselineFilenamePattern` 命中 + 业务核心表已存在 + baseline 未记账）忽略文件 checksum，故老库平稳。
- **迁移依赖顺序**：fresh-install 形态需要表创建顺序对依赖兼容；本仓库当前未使用外键，无强依赖。如未来引入外键，需补 `ALTER TABLE ADD CONSTRAINT FOREIGN KEY` 排在所有表创建之后。

## 影响范围

- **API**：无业务逻辑改动；新空库部署期间 Migrate 阶段读到的文件文本不同，**行为有差异**——这是本方案的目的，把"事实上不可用的 baseline"拉回"可用"。
- **Web**：无。
- **Bot**：无。
- **配置 / 部署**：无 docker-compose 或环境变量调整。
- **文档**：必须同步更新
  - `infrastructure/database/README.md`
  - `docs/runbooks/database-migration-baseline.md`
- **测试**：现有 `services/api/internal/db/migrate_test.go` 用例不需要改；如重建后引入新疑点（如 `Bootstrap` 在新形态下未覆盖的边界），按需补一两条断言。

## 验证方式

### 编译 / 测试

- `cd services/api && go build ./...`
- `cd services/api && go test ./internal/db/...`

### 静态自检（AI 端）

- 输出 24 表权威字段对照表（dump vs 模型 vs 旧 baseline → 决定值）
- 列出 fingerprint 命中检查（19 个 column / 索引必须全部出现）
- 列出 deterministic seed 检查（settings 5 条 + plan_groups 1 条）

### 手工验证（用户端 PG）

1. 起干净 PG，建空库 `ember_truth`，灌入 dump → dump schema 视图到 `truth.txt`
2. 起干净 PG，建空库 `ember_new`，跑新 baseline → dump schema 视图到 `new.txt`
3. `diff truth.txt new.txt` 必须为空
4. 在 `ember_new` 上启动 `ember-api`，确认 `Migrate → VerifySchema → Bootstrap → /health 200` 全链路通过
5. 至少调用一次 `/api/admin/users` 等业务接口，返回 200（即便 0 行数据）

## 落地后文档处理

- 完成后，本计划文档保留在 `docs/plan/architecture/` 直至下一次 baseline 压缩窗口；下一轮 baseline 实际推进时，把本计划与历史合并式决策记录一同移入 `docs/archive/plan/architecture/`。
- 把"baseline 必须与运行库 schema 字段级等价"这一稳定约束补进 `infrastructure/database/README.md` 的"添加新 migration（维护者视角）"小节，杜绝再次发生模型漂移而 baseline 不跟。
