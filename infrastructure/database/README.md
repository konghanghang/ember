# 数据库迁移（PostgreSQL）

本目录存放 Ember 项目的数据库迁移 SQL。

当前规则很直接：

- `infrastructure/database/` 顶层文件是现行可执行迁移资产
- 当前顶层已收口为 `20260422_00_schema_baseline.sql` + 后续增量迁移
- `archive/pre-20260415/` 与 `archive/pre-20260422/` 仅保留已被 baseline 完整覆盖的历史迁移
- 已有数据库升级只执行 baseline 之后新增的顶层 SQL

## 当前目录职责

本目录当前同时承担三种职责：

- 空数据库首次初始化入口
- 已有数据库手工增量升级入口
- 历史 schema 变更追溯入口

这也是为什么不能为了“目录整齐”直接把旧 SQL 从顶层挪走。

## 使用方式

### 1. 空数据库首次初始化

标准入口：

```bash
cd services/api && go run ./cmd/migrate
```

`cmd/migrate` 会按字典序执行 `infrastructure/database/` 顶层 baseline + 全部后续增量 migration，
然后跑 `VerifySchema` 自检。这是当前唯一和 API 启动期 schema 约束完全一致的空库初始化方式。

如果必须手工执行 SQL，也必须执行：

1. `20260422_00_schema_baseline.sql`
2. baseline 之后的全部顶层增量 migration（见下节完整列表）

只执行 baseline 本身已经不够，API 启动时会因为缺少后续表 / 列 / 索引被 `VerifySchema` 拒绝。

当前现行 baseline `20260422_00_schema_baseline.sql` 包含：

- 当前完整 schema
- 5 条 deterministic 默认设置
- 默认套餐分组 `DEFAULT`
- 与历史迁移定义对齐但线上源库缺失的两条索引：
  - `idx_ranking_lookup`
  - `uq_redemptions_user_code`
- `2026-04-22`（`v1.3.1`）前已上线的订阅审核字段与 `media_gaps` 表结构

### 2. 生产 / 已有数据库升级

只执行 baseline 之后新增的顶层 SQL。

当前顶层 baseline 之后的增量 migration 为：

- `20260424_01_subscription_resubmission_after_rejection.sql`
- `20260425_01_baseline_normalization_indexes.sql`
- `20260425_02_telegram_bind_codes_user_unique.sql`
- `20260426_01_users_lower_unique_indexes.sql`
- `20260426_02_failed_emby_async_ops.sql`
- `20260426_03_stripe_webhook_events.sql`
- `20260426_04_payments_checkout_constraints.sql`
- `20260426_05_subscriptions_ingest_progress.sql`
- `20260426_06_media_gaps_dispatch_failed.sql`
- `20260426_07_media_gap_scans.sql`
- `20260426_08_playback_rankings_idempotency.sql`
- `20260426_09_media_quality_caches_inflight.sql`
- `20260426_10_device_actions_operator_id.sql`
- `20260426_11_tv_calendar_sources_sync_markers.sql`
- `20260426_12_bot_pending_reject_requests.sql`
- `20260426_13_schema_alignment.sql`
- `20260426_14_airdate_to_date.sql`
- `20260426_15_users_password_reset_required.sql`
- `20260426_16_subscriptions_note_not_null.sql`
- `20260427_01_bot_runtime_locks.sql`
- `20260427_02_media_gaps_ignore_reason_code.sql`

如果当前数据库还停留在 `v1.3.1` 对应阶段，升级到当前版本前需要从 `20260424_01_subscription_resubmission_after_rejection.sql` 开始顺序执行以上 SQL；已经执行过它们的环境不需要重复执行。

### 3. Docker 首次初始化（仅首次）

`infrastructure/docker/docker-compose.yml` 把 `infrastructure/docker/initdb/` 子目录挂载到 Postgres 的 `/docker-entrypoint-initdb.d`，**不再直接挂本目录**。原因：

- PG initdb.d 会按字典序执行其下所有 `.sql` / `.sh` / `.sql.gz`，本目录里的 README、archive、未来临时 SQL 都可能被误执行
- 改用专用子目录后，本目录仍是 SQL 真相，但首启执行链路收口在 `docker/initdb/`

执行行为：

- 只在数据库数据卷为空时执行一次
- 当前 `docker/initdb/` 包含顶层 baseline 和后续增量 migration
- `archive/` 不参与初始化
- 如果数据库已存在，只手工执行 baseline 之后新增的顶层 migration

**新增 / 同步迁移**：每次在本目录新增顶层 SQL，必须同步复制一份到 `infrastructure/docker/initdb/`：

```bash
cp infrastructure/database/<NEW_SQL>.sql infrastructure/docker/initdb/
```

被 baseline 吸收并归档到 `archive/` 的旧文件，也要从 `docker/initdb/` 删除。

### 4. 历史追溯

历史迁移文件保存在：

```text
infrastructure/database/archive/pre-20260415/
infrastructure/database/archive/pre-20260422/
```

这些文件只用于追溯，不再属于现行执行链路。

## 文件命名规则

- 迁移文件名统一使用 `YYYYMMDD_NN_description.sql`
- 新增 migration 默认继续放在 `infrastructure/database/` 顶层
- 单次迁移脚本必须保持幂等
- 归档历史迁移时，原文件名不允许改写
- baseline 文件继续使用同一命名规则，不额外引入特殊前缀

## 新增 migration 必做事项

每次在本目录新增顶层 SQL，必须同步完成以下三件事：

1. 复制到 `infrastructure/docker/initdb/`，保持文件名一致
2. 在 `services/api/internal/db/db.go` 的 `schemaFingerprintColumns` / `schemaFingerprintIndexes` 中追加该 migration 引入的代表性列 / 索引指纹（用于 API 启动期 `VerifySchema` fail-fast）
3. 在隔离临时库回灌验证幂等

漏做第 2 步：API 启动期不会拦住缺该 migration 的环境，要等运行到第一次查询才报错。

## Baseline 收口规则

当前目录已经过两轮 baseline 收口，现行执行链路如下：

- 顶层：保留当前 baseline 和 baseline 之后仍需执行的增量迁移
- `archive/`：只保留已被 baseline 完整覆盖的历史迁移，供追溯使用

示例结构：

```text
infrastructure/database/
├─ README.md
├─ 20260422_00_schema_baseline.sql
├─ 20260424_01_xxx.sql
├─ 20260426_01_xxx.sql
└─ archive/
   ├─ pre-20260415/
   │  ├─ 20260215_01_create_playback_rankings.sql
   │  ├─ 20260222_01_add_email_verification.sql
   │  └─ ...
   └─ pre-20260422/
      ├─ 20260415_00_schema_baseline.sql
      ├─ 20260416_01_subscription_status_and_review_fields.sql
      └─ 20260418_01_media_gaps.sql
```

边界约束：

- 顶层永远只保留现行可执行链路，不把历史文件和平铺增量混在一起
- 只有被最新 baseline 完整覆盖的旧迁移，才允许整体归档
- 归档动作必须按截点整批执行，不能零散挪几份文件
- 部署和运维执行入口始终以顶层可执行 SQL 为准，不能要求去 `archive/` 挑文件

## Baseline 生成方式

生成 baseline 时，不要手工拼接旧 migration，应该按下面的顺序做：

1. 选定稳定的迁移截点
2. 从当前有效 schema 导出 `schema-only` baseline 初稿
3. 补齐 deterministic seed 和与迁移契约对齐的缺失结构
4. 在隔离空库回灌 baseline，验证表、索引、约束和 seed
5. 将被完整覆盖的旧迁移整批归档到 `archive/<cutoff>/`

具体操作步骤看 [`docs/runbooks/database-migration-baseline.md`](../../docs/runbooks/database-migration-baseline.md)。

## 验证清单

准备归档旧迁移前，至少完成下面的检查：

1. 空数据库仅执行 baseline，可以完整创建核心表、索引、约束和必要初始化数据
2. 空数据库执行 baseline + baseline 后增量迁移，结果与当前完整迁移链路一致
3. 已有数据库从 baseline 截点之后的任一版本升级，只需要执行增量迁移
4. `infrastructure/database/README.md`、部署文档、发布说明中的执行入口一致
5. 发布提醒仍只面向顶层可执行迁移，不把 `archive/` 误当成上线升级清单
