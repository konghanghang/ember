# 数据库迁移（PostgreSQL）

本目录存放 Ember 项目的数据库 schema 真相源。

当前顶层现行入口：

- `00000000_baseline_20260605.sql`：v1.6.0 截点 fresh-install baseline，新装库初始化的全部内容；吸收 2026-05-02 baseline 以及 2026-05-04 至 2026-05-29 顶层增量，已按"全 0 前缀让 baseline 永远字典序最先"的命名约定收口
- `20260707_01_subscription_plan_group_auto_approval.sql`：套餐分组订阅自动审批额度与审核来源
- `20260707_02_plan_group_media_library_deferred_sync.sql`：媒体库模板版本与用户延迟同步标记
- `20260801_01_create_p115_accounts.sql`：管理员 115 源账号 / 播放账号、Cookie 密文和启用唯一约束
- `20260822_01_create_playback_transfer_tasks.sql`：保留式秒传任务、活动内容唯一约束和访问时间索引
- `archive/`：仅供追溯，不参与任何运行时链路

当前升级支持窗口：

- **支持起点**：`2026-06-05` / v1.6.0 截点，即当前顶层 baseline `00000000_baseline_20260605.sql` 对齐的 schema
- **新装库**：直接执行当前顶层 baseline，再执行后续顶层增量
- **已有库升级**：只保证从 `2026-06-05` 截点之后的已支持版本 forward-only 升级；旧于该截点的库如果没有执行过已归档增量，不承诺直接跳升到当前版本
- **历史 SQL**：`archive/` 中的 SQL 已退出运行链路，不能靠启动期 Migrate 自动补齐；需要追溯或人工修复时再查阅

数据库表名 / 列名 / 索引名统一使用 `snake_case`；历史 camelCase 列已在 v1.4.0 期间整体收口（脚本归档于 `archive/pre-20260502/20260423_00_legacy_camelcase_to_snake_case.sql`）。

## 自动迁移与 schema_migrations

API 启动期内嵌自动迁移：支持窗口内执行 `docker compose pull && docker compose up -d` 即可完成 schema 升级，部署者无需手工执行顶层 SQL。旧于当前支持起点的数据库按上文"当前升级支持窗口"处理。

启动期序列：`InitDB → Migrate → VerifySchema → Bootstrap → Start`，全过程日志带 `[Migrate]` 前缀。Migrate 阶段封装在 `services/api/internal/db/migrate.go`，靠一张内部表 `schema_migrations` 记账：

| 字段 | 类型 | 语义 |
|---|---|---|
| `filename` | TEXT PRIMARY KEY | 顶层 SQL 文件名（如 `20260427_04_bot_pending_reject_message_context.sql`） |
| `applied_at` | TIMESTAMPTZ NOT NULL DEFAULT now() | 应用时间 |
| `checksum` | TEXT NOT NULL | SQL 文件内容指纹（SHA-256 hex，按字节计算） |

启动期分支判断：

| 业务核心表存在 | `schema_migrations` 非空 | fingerprint 齐 | 缺失项关联 migration 都在目录顶层 | 分支行为 |
|:-:|:-:|:-:|:-:|---|
| 否 | 否 | — | — | **新空库**：forward-only 跑全部目录 SQL（含 baseline） |
| 是 | 否 | 是 | — | **老库 backfill**：仅记账，不执行 SQL |
| 是 | 否 | 否 | 是 | **混合模式**：缺失项关联的 SQL 跑 forward-only，其余记账 |
| 是 | 否 | 否 | 否 | **老库不对齐**：fail-fast，需人工对齐 schema |
| — | 是 | — | — | **正常 forward-only**：未应用即跑、checksum 不一致即拒；**未应用 baseline 仅记账不执行**（重命名豁免） |

并发保护：每次 Migrate 阶段用 `pg_try_advisory_lock` 抢独占锁，30s 超时窗口内重试；锁绑定到单条物理连接，避免连接池抢到不同连接导致 unlock 落空。

baseline 唯一性：所有分支共用一条防御——目录里同时存在多份 baseline 文件时启动期 fail-fast。baseline 表达"等价 schema 快照"，目录里必须只有一份；做新一轮 baseline 压缩时旧 baseline 必须先移到 `archive/pre-<日期>/`。

## SQL 文件硬约束

凡进入 `infrastructure/database/` 顶层的 SQL 文件，必须满足：

1. **不允许写 `BEGIN` / `COMMIT`**：Migrate 在外层包裹事务执行，嵌套事务在 PG 表现为 savepoint 会让回滚行为难以预测。`DO $$ ... $$` 块仍可用（PG 视为单语句）。
2. **不允许使用不能在事务内执行的 DDL**：`CREATE INDEX CONCURRENTLY` / `REINDEX CONCURRENTLY` / `VACUUM` / `ALTER SYSTEM` / `CREATE DATABASE` 等。如必须用，放进 `archive/` 由人工执行，不进 forward-only 流。
3. **必须幂等**：DDL 用 `IF NOT EXISTS`、列用 `ADD COLUMN IF NOT EXISTS`、DML 用 `WHERE` 收敛。
4. **强制 LF 行尾**：仓库根 `.gitattributes` 写明 `*.sql text eol=lf`。这是 checksum 跨平台稳定的前提；Windows 开发者克隆后若行尾被改成 CRLF，本地与镜像内 hash 会不一致。
5. **一旦被记账，文件内容不得再改写**：含格式调整、注释新增、空白调整。需修正只能新增前向 SQL（forward-only 原则）。

## 使用方式

### 1. Docker 一键启动（推荐）

```bash
cd infrastructure/docker
docker compose up -d
```

`ember-api` 启动期 Migrate 阶段自动接管 schema 初始化与升级：

- **空数据库（首次部署）**：业务核心表不存在 + `schema_migrations` 为空 → 进入"新空库"分支，按字典序 forward-only 跑全部目录 SQL，从空库一次性初始化 schema
- **已有数据库（升级）**：`schema_migrations` 已记账 → 走"正常 forward-only"分支，按需补齐当前顶层未应用 SQL；直接升级来源必须不早于当前支持起点

PG `initdb.d` 不再被挂载，无需手工 SQL，无需任何 SQL 副本目录同步。

### 2. 本地空库一步到位

```bash
cd services/api
go run ./cmd/server
```

`cmd/server` 启动期 Migrate 阶段会探测到业务核心表不存在 + `schema_migrations` 为空 → 进入"新空库"分支按字典序 forward-only 跑全部 SQL，再走 `VerifySchema` 自检与 `Bootstrap` 写入默认 admin / settings / plan_groups。

`EMBER_MIGRATIONS_DIR` 在容器内由镜像 ENV 注入为 `/app/migrations`；本地未设时按 `../../infrastructure/database` / `../infrastructure/database` / `infrastructure/database` 逐个 fallback 到仓库工作树。

### 3. 已有数据库升级

线上以 `AUTO_MIGRATE=false` 运行，**不依赖 GORM 自动迁移**。

升级流程：

```bash
docker compose pull
docker compose up -d
```

`ember-api` 启动期 Migrate 阶段会自动应用未应用的顶层 SQL；如失败则 `log.Fatal` 退出，容器进入 restart loop，部署者通过 `docker logs ember-api --tail` 第一时间看到失败 SQL 文件名。**镜像是只读的**，恢复时必须修复仓库 SQL → 重新构建并推送镜像 → `docker compose pull && up -d`；如失败 SQL 不可修，按 forward-only 原则**追加一条新 SQL** 抵消错误效果，不允许修改原文件。

升级边界：

- 当前直接升级支持起点是 `2026-06-05` / v1.6.0 截点
- 从该截点之后的版本升级时，Migrate 只需要处理当前顶层 baseline 之后新增的 forward-only SQL
- 从更旧版本直接跳升时，如果旧库没有执行过 `archive/pre-20260605/` 中已归档的增量，启动期 Migrate 不会自动补齐这些 SQL；此类环境必须先升级到支持起点对应 schema，或人工按归档 SQL 对齐后再升级
- 不要把 `archive/` 中的 SQL 直接整体搬回顶层：多份 baseline 会触发 fail-fast，历史回填 / seed / 兼容脚本也不一定适合当前运行链路

### 4. 历史追溯

```text
archive/
├─ README.md
├─ pre-20260415/   早期迁移，被首轮 baseline 覆盖
├─ pre-20260422/   v1.3.1 截点 baseline + 同期增量，被次轮 baseline 覆盖
├─ pre-20260502/   v1.4.0 截点旧 baseline + 23 个增量，被 2026-05-02 baseline 覆盖
└─ pre-20260605/   2026-05-02 baseline + 4 个增量，被当前 baseline 覆盖
```

普通使用者无需关注。排错、人工对齐旧库或核对字段历史时，可在此查阅原始迁移 SQL；其余场景优先查 `git log`。

## 现行 baseline 说明

`00000000_baseline_20260605.sql` 是 **fresh-install 形态的 baseline**：

- 内容基于 2026-05-02 fresh-install baseline 吸收 2026-05-04 至 2026-05-29 顶层 forward-only migration 整理而来，所有语句在新空库上都是有效语义，不携带历史升级包袱
- 表 / 主键 / 索引按字典序排列，便于后续做 schema diff 验证
- 末尾追加 deterministic seed（`settings` 6 条 + `plan_groups.DEFAULT` + 默认 Emby Policy 模板），跟应用层 `Bootstrap` 与分组模板默认行为一致

形态切换决策记录：[`docs/archive/plan/architecture/baseline-fresh-install-rewrite.md`](../../docs/archive/plan/architecture/baseline-fresh-install-rewrite.md)。

> 历史前身是 **合并式 baseline**（24 份历史顶层 SQL 按字典序拼接），在 2026-05-09 排错过程中暴露与运行库 schema 严重脱节，开源前一次性切换到 fresh-install 形态。合并源整批归档于 [`archive/pre-20260502/`](./archive/pre-20260502/)，仅供追溯。
>
> 2026-06-05 上线后，`00000000_baseline_20260502.sql` 与后续 4 条顶层增量已整批归档于 [`archive/pre-20260605/`](./archive/pre-20260605/)，当前顶层只保留新 baseline。自该截点起，项目不再承诺更旧数据库直接跳升时由 Migrate 自动回放归档 SQL；如需支持更早版本直升，必须另行恢复对应增量链或提供专门的 upgrade bridge SQL。

### baseline ↔ schema 真相对齐约束

baseline 必须与运行库 schema **字段级等价**，且与 `services/api/internal/models/` 下 GORM 模型保持一致。维护任何模型字段、索引、约束变更时，都必须同步在 `infrastructure/database/` 顶层补一条 SQL migration；漏补会让"老库 forward-only 升级"和"新空库 fresh-install"两条路径在某个时间点产生不可见漂移，下一次 baseline 重建时会以 PG diff 不一致的形式暴露出来。

## 添加新 migration（维护者视角）

### 文件命名

- 顶层新增：`YYYYMMDD_NN_<description>.sql`
- 必须满足上面"SQL 文件硬约束"全部 5 条

### Schema 命名

- 表 / 列 / 索引一律 `snake_case`
- Go 字段与 JSON 字段通过显式 tag 映射，不构成数据库列命名依据
- 历史 camelCase 列已收口，新增不允许扩散

### 必做事项

每次新增顶层 SQL，必须同步完成：

1. 在 `services/api/internal/db/db.go` 的 `schemaFingerprintColumns` / `schemaFingerprintIndexes` 中追加该 migration 引入的代表性列 / 索引指纹（启动期 `VerifySchema` 据此 fail-fast；混合模式分支也据此判断）
2. 命名符合 `snake_case`
3. 在临时库回灌验证幂等

漏做第 1 步：API 启动期 Migrate 在该 SQL 已被记账后不再触发分支判断（直接走正常 forward-only 路径跳过），后续如果 `VerifySchema` 兜底也漏检，运行到第一次查询才会报错——风险更高，**绝不允许**。

### 何时再做下一轮 baseline

当顶层增量再次堆到不易维护、相关 schema 已稳定时：

1. 选定新截点 `YYYYMMDD`
2. 按字典序合并 `{当前 baseline} + {后续增量}` 生成新 baseline，命名沿用 `00000000_baseline_<新截点>.sql`（全 0 前缀让 baseline 永远字典序最先且文件名一眼可辨）
3. 旧 baseline + 全部增量整批移到 `archive/pre-<新截点>/`（**目录顶层任何时刻必须只有一份 baseline**，多份共存启动期会 fail-fast）
4. 同步本 README 的 baseline 文件名引用
5. `db.go` 的 fingerprint 持续有效，不需要清空

做 baseline 压缩前必须先确认升级支持窗口：

- 默认策略：新的 baseline 截点就是新的直接升级支持起点
- 如果仍要支持更旧版本直接升级，就不能把该支持窗口内的增量全部移出顶层；必须保留老库升级需要执行的 forward-only SQL，或新增明确的 upgrade bridge SQL
- 更新 release notes / 部署文档时必须写清最低支持直升版本，避免使用者误以为 `archive/` 会被自动执行

老库重启时的行为（已写入 `migrate_test.go`）：
- 老 baseline 已记账、新 baseline 在目录中且未记账 → 新 baseline 视为"等价 schema 快照"仅记账不执行（`baselineFilenamePattern` 同时识别两种命名格式）
- 老库 baseline 重命名豁免对应 commit 的回归测试：`TestRunMigrate_BaselineRenameOnExistingDB_ShouldNotReexecute`
- 多份 baseline 共存防御：`TestRunMigrate_MultipleBaselinesCoexist_FailFast`

具体操作可参考 [`docs/runbooks/database-migration-baseline.md`](../../docs/runbooks/database-migration-baseline.md)。
