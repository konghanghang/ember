# 数据库 Migration Baseline 操作手册

本手册只解决一件事：当 `infrastructure/database/` 顶层迁移已经堆到不适合继续平铺时，如何安全生成 baseline，并把被完整覆盖的旧迁移归档。

如果你只是给已有数据库补一个新 migration，不需要看这份文档，直接看 [`infrastructure/database/README.md`](../../infrastructure/database/README.md)。

## 当前状态

当前最近一轮 baseline 已于 `2026-04-22` 落地；首轮 baseline 信息保留在 archive 作为追溯：

- 基线文件：`infrastructure/database/20260422_00_schema_baseline.sql`
- 历史归档目录：`infrastructure/database/archive/pre-20260422/`
- 吸收范围：旧 baseline `20260415_00_schema_baseline.sql` + `20260416_01_subscription_status_and_review_fields.sql` + `20260418_01_media_gaps.sql`
- deterministic seed：5 条默认设置 + 默认套餐分组 `DEFAULT`

后续如果再次做 baseline，起点应是“当前顶层 baseline + baseline 之后新增的顶层 migration”，不要再把 `archive/` 当成现行执行链路。

下面的步骤主要记录首个 baseline 的实际生成过程；后续再做下一轮 baseline 时，按同样原则套到“当前顶层现行链路”上，不要回退到 `archive/` 时代的平铺迁移链。

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
- [数据库迁移 Baseline 与归档收口方案](../plan/architecture/database-migration-baseline-and-archive.md)
