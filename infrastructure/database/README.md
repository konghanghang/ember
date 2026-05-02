# 数据库迁移（PostgreSQL）

本目录存放 Ember 项目的数据库 schema 真相源。

当前唯一现行入口：

- `20260502_00_schema_baseline.sql`：v1.4.0 截点合并 baseline，新装库初始化的全部内容
- `archive/`：仅供追溯，不参与任何运行时链路

数据库表名 / 列名 / 索引名统一使用 `snake_case`；历史 camelCase 列已在 v1.4.0 期间整体收口（脚本归档于 `archive/pre-20260502/20260423_00_legacy_camelcase_to_snake_case.sql`）。

## 使用方式

### 1. Docker 一键启动（推荐）

```bash
cd infrastructure/docker
docker compose up -d postgres
```

`docker-compose.yml` 把 `infrastructure/docker/initdb/` 挂载到 PostgreSQL 的 `/docker-entrypoint-initdb.d`。容器首次启动（数据卷为空）时，PG 自动执行挂载目录下的 SQL，本目录的 baseline 已在该子目录有同名副本，首启即完成 schema 初始化。

`archive/` 不挂载，不参与初始化。

### 2. 本地空库初始化

```bash
cd services/api && go run ./cmd/migrate
```

`cmd/migrate` 流程：`InitDB` → 按字典序执行 `infrastructure/database/` 顶层 `*.sql` → `VerifySchema` 自检 → `Bootstrap` 写入默认 admin / settings / plan_groups。

如果必须手工执行：

```bash
psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  -f infrastructure/database/20260502_00_schema_baseline.sql
```

### 3. 已有数据库升级

线上以 `AUTO_MIGRATE=false` 运行，不依赖 GORM 自动迁移。

- **已升级到 v1.4.0 的环境**：当前 baseline 中的 DDL 全部带 `IF NOT EXISTS`、DML 在已清洗数据上 0 命中，因此对已升级库执行也是 no-op，但通常情况下不需要重复执行。
- **停留在 v1.3.1 的环境**（仅历史参考）：线上 v1.4.0 已上线，该路径无现实需求；如需还原，可在 `archive/pre-20260502/` 内按字典序执行 24 个原始文件（含 `20260423_00_legacy_camelcase_to_snake_case.sql`）。

### 4. 历史追溯

```text
archive/
├─ README.md
├─ pre-20260415/   早期迁移，被首轮 baseline 覆盖
├─ pre-20260422/   v1.3.1 截点 baseline + 同期增量，被次轮 baseline 覆盖
└─ pre-20260502/   v1.4.0 截点旧 baseline + 23 个增量，被本轮合并 baseline 覆盖
```

普通使用者无需关注。排错或核对字段历史时，可在此查阅原始迁移 SQL；其余场景优先查 `git log`。

## 现行 baseline 说明

`20260502_00_schema_baseline.sql` 是 **合并式 baseline**：

- 内容由历史 24 份顶层 SQL（旧 baseline + 23 个增量）按字典序合并
- 行为等价于在新装空库上逐个执行这 24 个文件
- 各原始文件以 `-- ┌─ <filename>` / `-- └─ <filename>` 边界注释包裹，便于定位语句来源
- 包含 4 处来自历史增量的 DML（去重 / NULL 回填）；新装空库上均为 no-op
- deterministic seed（`settings` 5 条 + `plan_groups.DEFAULT`）继承自旧 baseline 段

为何选合并式而非严格 schema-only dump：本项目当前由单人维护、为开源做准备，没有 pg_dump 验证链路也能直接维护；合并方案在文本层面恒等于历史执行链路，无额外验证负担。多团队 / 多环境项目应优先考虑严格 dump 方案，详见 [`docs/runbooks/database-migration-baseline.md`](../../docs/runbooks/database-migration-baseline.md)。

## 添加新 migration（维护者视角）

### 文件命名

- 顶层新增：`YYYYMMDD_NN_<description>.sql`
- 必须幂等：DDL 用 `IF NOT EXISTS`、列用 `ADD COLUMN IF NOT EXISTS`、DML 用 `WHERE` 收敛

### Schema 命名

- 表 / 列 / 索引一律 `snake_case`
- Go 字段与 JSON 字段通过显式 tag 映射，不构成数据库列命名依据
- 历史 camelCase 列已收口，新增不允许扩散

### 必做事项

每次新增顶层 SQL，必须同步完成：

1. 复制到 `infrastructure/docker/initdb/`，保持文件名一致
2. 在 `services/api/internal/db/db.go` 的 `schemaFingerprintColumns` / `schemaFingerprintIndexes` 中追加该 migration 引入的代表性列 / 索引指纹（启动期 `VerifySchema` 据此 fail-fast）
3. 命名符合 `snake_case`
4. 在临时库回灌验证幂等

漏做第 2 步：API 启动期不会拦住缺该 migration 的环境，要等运行到第一次查询才报错。

### 何时再做下一轮 baseline

当顶层增量再次堆到不易维护、相关 schema 已稳定时：

1. 选定新截点 `YYYYMMDD`
2. 按字典序合并 `{当前 baseline} + {后续增量}` 生成 `<新截点>_00_schema_baseline.sql`
3. 旧 baseline + 全部增量整批移到 `archive/pre-<新截点>/`
4. 同步 `docker/initdb/`、本 README 的 baseline 文件名引用
5. `db.go` 的 fingerprint 持续有效，不需要清空

具体操作可参考 [`docs/runbooks/database-migration-baseline.md`](../../docs/runbooks/database-migration-baseline.md)。
