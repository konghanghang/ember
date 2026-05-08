# 数据库 Migration Baseline 操作手册

本手册只解决一件事：当 `infrastructure/database/` 顶层迁移已经堆到不适合继续平铺时，如何安全生成 baseline，并把被完整覆盖的旧迁移归档。

如果你只是给已有数据库补一个新 migration，不需要看这份文档，直接看 [`infrastructure/database/README.md`](../../infrastructure/database/README.md)。

> **本文档不是日常升级流程**：自 v1.4.x 起，日常升级由 `ember-api` 启动期 Migrate 阶段自动应用未应用 SQL（详见 `infrastructure/database/README.md` 的"自动迁移与 schema_migrations"章节）。本文仅在生成下一轮 baseline 时按隔离库验证流程使用。

## 当前状态

最近一轮 baseline 已于 `2026-05-02` 落地（v1.4.0 截点）：

- 基线文件：`infrastructure/database/20260502_00_schema_baseline.sql`（合并式 baseline，非 pg_dump schema-only）
- 历史归档目录：`infrastructure/database/archive/pre-20260502/`
- 吸收范围：上一轮 baseline `20260422_00_schema_baseline.sql` + 23 个 v1.4.0 期间顶层增量
- deterministic seed：5 条默认 settings（`default_trial_days` / `registration_mode` / `notify_group_link` / `email_verification` / `stripe_allowed_payment_methods`）+ `plan_groups.DEFAULT`，继承自旧 baseline 段

历史几轮 baseline 截点：

- v1.3.0 截点：`archive/pre-20260415/`
- v1.3.1 截点：`archive/pre-20260422/`
- v1.4.0 截点：`archive/pre-20260502/`

后续如果再次做 baseline，起点应是”当前顶层 baseline + baseline 之后新增的顶层 migration”，不要再把 `archive/` 当成现行执行链路。

## 本轮选择合并式 baseline 的边界

v1.4.0 截点这一轮采用了”按字典序文本拼接”的合并式 baseline，跳过了下文第二、三、四步（隔离库回灌、pg_dump 导出、双库 schema diff）。判断依据：

- 项目当前由单人维护、为开源做准备，无现成 PG 实例时 pg_dump 路径不便维护
- 23 个增量在新装空库上的执行结果与”按字典序逐个执行 24 个文件”恒等
- 合并文件保留各原始 SQL 的 DO 块、IF NOT EXISTS、historical DML（去重 / NULL 回填）；新装空库上 DML 均 0 命中，no-op
- 用 `services/api/internal/db/db.go` 的 `schemaFingerprintColumns` / `schemaFingerprintIndexes` 做兜底交叉验证，确认所有列 / 索引 fingerprint 都被合并文件引入

适用边界：

- 自用 / 单人维护 / 开源前的 baseline 收口可走合并式
- 多团队 / 多环境项目，或无法在文本层面证明”合并 == 字典序执行”时，仍应回归严格 dump 路径（下文第二至六步）

下面的步骤主要记录首个 baseline 的实际生成过程，针对严格 dump 路径；后续再做下一轮 baseline 时，按同样原则套到”当前顶层现行链路”上，不要回退到 `archive/` 时代的平铺迁移链。

## 适用场景

满足下面 3 个条件时，再做 baseline：

- 顶层迁移已经形成明显堆积
- 计划选定的截点之前，相关 schema 已经稳定
- 你能提供一个隔离的空数据库用于验证，而不是直接拿现网库冒险

## 前置约束

- baseline 生成前，不要先移动任何历史 migration
- baseline 不是简单的“把旧 SQL 拼在一起”
- `archive/` 只用于追溯，不是新的执行入口
- 基线生成成功前，`infrastructure/database/` 顶层仍然是唯一有效迁移入口
- 数据库新增设计规则仍然是：表名 / 列名 / 索引名统一使用 `snake_case`

## 命名约束

- baseline 必须忠实反映截点时的真实 schema，不要为了“顺手统一风格”在 baseline 过程中偷偷重命名历史列
- 历史 schema 中已经存在的 camelCase 列属于遗留兼容范围，只有单独立项的 schema 收口任务才允许改名
- baseline 之后新增的 migration 仍然必须遵守 `snake_case` 规则，不能继续扩散历史 camelCase 命名
- Go / GORM 字段与 JSON 字段命名不构成数据库列命名依据；写 migration 或手工 SQL 时只认数据库真实列名

## baseline 文件命名与老库升级路径

下一轮 baseline 文件名继续沿用 `00000000_baseline_<新截点>.sql`（全 0 前缀让 baseline 永远字典序最先且文件名一眼可辨）。`services/api/internal/db/migrate.go` 的 `baselineFilenamePattern` 同时识别新命名格式与历史命名格式 `YYYYMMDD_NN_schema_baseline.sql`，确保升级路径自动覆盖。

老库重启时启动期 Migrate 自动处理 baseline 重命名：

- 老 baseline 已记账（如历史命名 `20260502_00_schema_baseline.sql`）+ 目录里换成新 baseline（如全 0 命名 `00000000_baseline_<新截点>.sql`）+ 新 baseline 不在 `schema_migrations` 中 → 视为"等价 schema 快照"，**仅写记账行不执行 SQL**，避免重复跑 baseline 中 `CREATE TYPE` / `ADD CONSTRAINT` 等不带 `IF NOT EXISTS` 的 DDL 让 API fail-fast
- 这个豁免**只在 forward-only 分支生效**（`schema_migrations` 已有记账时）；新空库分支不受影响，baseline 仍然按字典序被真的执行
- 目录里同时存在两份 baseline → 启动期 fail-fast，要求把老 baseline 移到 `archive/pre-<日期>/`

回归测试：

- `TestRunMigrate_BaselineRenameOnExistingDB_ShouldNotReexecute`：老库 baseline 重命名豁免
- `TestRunMigrate_MultipleBaselinesCoexist_FailFast`：多份 baseline 共存防御
- `TestRunMigrate_EmptyDBWithBaseline_AppliesBaseline`：新空库下 baseline 必须真的被执行

下一轮 baseline 切换发版时不需要写过渡 SQL，部署者也不需要手工跑任何记账迁移命令——直接 `docker compose pull && up -d` 即可。

## 第一步：选定截点

先明确一份截止文件，例如：

- 截止 migration：`20260418_01_media_gaps.sql`
- 目标 baseline：`20260422_00_schema_baseline.sql`
- 目标归档目录：`infrastructure/database/archive/pre-20260422/`

截点选择标准：

- 截点前的 schema 已经稳定，不会立刻被重做
- 截点前如果包含持久化初始化数据，这些数据也必须被 baseline 覆盖
- 截点前如果包含仅对已有数据有意义的回填或清洗逻辑，不要原样搬进空库 baseline

## 第二步：整理截点前 migration 清单

在 `infrastructure/docker/` 目录下执行，先生成顶层 migration 文件清单：

```bash
for f in ../database/*.sql; do basename "$f"; done | sort > /tmp/ember-migrations-all.txt
awk '1; $0=="20260414_01_add_redemption_code_registration_plan_group.sql" { exit }' /tmp/ember-migrations-all.txt > /tmp/ember-migrations-cutoff.txt
```

检查 `/tmp/ember-migrations-cutoff.txt`，确认：

- 只包含顶层 migration 文件
- 最后一行就是截点文件
- 没把 README 或未来 baseline 文件混进去

## 第三步：在隔离空库完整执行截点前 migration

下面命令假设你已经有可用的 `postgres` 容器，并且当前目录是 `infrastructure/docker/`。

先创建临时数据库：

```bash
docker compose exec -T postgres dropdb --if-exists -U postgres ember_baseline_source
docker compose exec -T postgres createdb -U postgres ember_baseline_source
```

按顺序执行截点前 migration：

```bash
while IFS= read -r file; do
  docker compose exec -T postgres \
    psql -v ON_ERROR_STOP=1 -U postgres -d ember_baseline_source \
    -f "/docker-entrypoint-initdb.d/$file"
done < /tmp/ember-migrations-cutoff.txt
```

这里的目的不是跑业务服务，而是得到一个“只通过历史 migration 建出来的目标 schema”。

## 第四步：导出 schema，生成 baseline 初稿

先导出纯 schema：

```bash
docker compose exec -T postgres \
  pg_dump -U postgres -d ember_baseline_source \
  --schema-only --no-owner --no-privileges \
  > ../database/20260422_00_schema_baseline.sql
```

这一步只会导出 schema，不会带业务数据。这样做是对的，因为 baseline 不应该把线上脏数据、业务记录或环境特定配置一起打进去。

## 第五步：补齐 deterministic seed 数据

纯 `schema-only` dump 还不够。当前历史 migration 里至少有两类持久化初始化数据，空库 baseline 必须显式补进去：

- `settings.email_verification=false`
- `plan_groups.DEFAULT`

建议在 baseline 文件末尾追加一个独立的 seed 段，保持幂等，例如：

```sql
-- Deterministic bootstrap data carried by pre-baseline migrations.

INSERT INTO settings ("key", "value", "isEncrypted", "updatedByUserId", "updatedAt")
VALUES ('email_verification', 'false', false, NULL, now())
ON CONFLICT ("key") DO NOTHING;

INSERT INTO plan_groups (key, name, description, "isDefault", "sortOrder", "createdAt", "updatedAt")
VALUES ('DEFAULT', '默认分组', '系统默认套餐分组', true, 10, now(), now())
ON CONFLICT (key) DO NOTHING;
```

判断规则：

- 这类 seed 必须是稳定、确定、环境无关的数据
- 不要把 `settings` 整张表导成 data dump，那会把环境配置一并带进去
- 不要把面向历史脏数据修复的 `UPDATE` / `DELETE` 原样塞进空库 baseline
- 不要借这个步骤顺手把历史 camelCase 列改成 `snake_case`；命名收口是单独 migration 议题，不是 baseline seed 议题

当前明确不应直接搬进空库 baseline 的逻辑包括：

- `20260304_01_add_redemptions_user_code_unique.sql` 里的历史去重和 `usedCount` 校准
- `20260314_03_add_payment_expires_at.sql` 里的历史数据回填
- `20260329_01_add_subscription_season.sql` 里的历史 TV 订阅回填

这些逻辑属于“已有数据库升级时的数据修复”，不是空数据库 baseline 的职责。

## 第六步：验证 baseline

至少做下面 4 轮验证：

### 1. 空库只跑 baseline

创建一个新的临时数据库，只执行 baseline：

```bash
docker compose exec -T postgres dropdb --if-exists -U postgres ember_baseline_verify
docker compose exec -T postgres createdb -U postgres ember_baseline_verify
docker compose exec -T postgres \
  psql -v ON_ERROR_STOP=1 -U postgres -d ember_baseline_verify \
  -f /docker-entrypoint-initdb.d/20260422_00_schema_baseline.sql
```

检查表、索引、约束和 deterministic seed 是否完整。

### 2. 空库跑 baseline + 截点后增量 migration

在 `ember_baseline_verify` 上继续执行截点后的增量 migration，确认结果与当前完整链路一致。

### 3. 与历史链路做 schema 对比

建议分别导出 `ember_baseline_source` 和 `ember_baseline_verify` 的 schema，再做 diff：

```bash
docker compose exec -T postgres \
  pg_dump -U postgres -d ember_baseline_source \
  --schema-only --no-owner --no-privileges \
  > /tmp/ember-baseline-source-schema.sql

docker compose exec -T postgres \
  pg_dump -U postgres -d ember_baseline_verify \
  --schema-only --no-owner --no-privileges \
  > /tmp/ember-baseline-verify-schema.sql

diff -u /tmp/ember-baseline-source-schema.sql /tmp/ember-baseline-verify-schema.sql
```

如果这里还有差异，先修 baseline，不要急着归档旧文件。

### 4. 文档与发布入口核对

确认下面几处都已经指向顶层可执行 SQL，而不是旧历史目录：

- `infrastructure/database/README.md`
- `docs/runbooks/deployment.md`
- `docs/runbooks/deployment-environment.md`
- `docs/runbooks/release-process.md`

## 第七步：归档旧 migration

只有前面的验证都通过，才允许归档截点之前的历史文件。

建议结构：

```text
infrastructure/database/
├─ 20260422_00_schema_baseline.sql
├─ 20260424_01_xxx.sql
├─ 20260420_01_xxx.sql
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

归档时遵守下面规则：

- 整批移动，不要零散挑文件
- 原文件名保持不变
- 归档后顶层只保留 baseline 和 baseline 之后的增量 migration
- 归档完成后，立即更新 README 和相关 runbook

## 何时停止，不要继续推进

出现下面任一情况，就别继续做 baseline：

- 截点前的 schema 仍在反复变
- baseline 无法覆盖确定性 seed 数据
- 你只能从现网脏数据倒推 schema，拿不到隔离空库
- 发布、部署、README 入口还没收口，团队对新的执行路径也没共识

这时候继续做，只会把目录搞乱。

## 相关文档

- [数据库迁移说明](../../infrastructure/database/README.md)
- [部署指南](./deployment.md)
- [部署环境与配置](./deployment-environment.md)
- [发布流程](./release-process.md)
- [数据库迁移 Baseline 与归档收口方案](../archive/plan/architecture/database-migration-baseline-and-archive.md)
