# 数据库迁移自动应用方案

> 状态：已归档
> 负责人：Ember
> 更新时间：2026-05-17
>
> 演进说明：本方案设计时计划"先保持 initdb/ 与 ember-api 启动期 Migrate 双轨、稳定后再退役"；落地后由 [OSS 部署体验方案](./oss-deployment-experience.md) phase 2 提前评估并执行 initdb/ 退役（自动化测试已覆盖"新空库"分支、当前尚无活跃 OSS 部署）。下文"背景 / 当前事实"段保留方案设计时的现状快照不重写，"方案设计 / 验证方式"段中已过时的 initdb 表述按当前实现做了最小修订。
>
> 归档说明：启动期自动迁移已进入 `v1.5.0` / `v1.5.1`，`schema_migrations`、forward-only、backfill、checksum 防改写和 PG `initdb.d` 退役均已同步到系统架构、数据库 README 与部署 runbook。本方案只保留历史设计与决策追溯价值。

## 背景

当前 Ember 的数据库 schema 升级路径在 docker compose 部署场景下是断裂的：

- 部署者主流路径是 `docker compose pull && docker compose up -d`，但 Postgres 容器只在数据卷为空时执行一次 `infrastructure/docker/initdb/`，**之后任何新增 SQL 文件都不会被 PG 自动应用**。
- API 容器启动期 `VerifySchema` 会因为缺表 / 缺列 / 缺索引而 fail-fast，部署者必须手工连进 `postgres` 容器、按 `infrastructure/database/README.md` 顺序执行 baseline 之后的全部 SQL，再重启 API。
- 这条手工链路对个人 / 小团队部署者门槛过高，是当前升级体验的核心痛点；遗漏一条 SQL 就意味着 API 起不来或线上数据漂移。
- 现有 `services/api/cmd/migrate` 已经能从空库一次性应用全部 SQL，但**没有跟踪"哪些已应用"**，也没有任何容器生命周期对接，等于工具具备但通路没接通。

如果继续维持现状，每次发版都需要在发布说明里逐条提醒部署者手工跑 SQL；漏跑导致的故障只能在 API 启动失败后被动发现，恢复成本高。

## 目标

本方案要实现：

1. 部署者只需 `docker compose pull && docker compose up -d`，**所有未应用的顶层 SQL 在 API 服务真正对外提供之前自动按字典序应用完毕**，无需任何人工 SQL 操作。
2. 迁移执行器升级为 forward-only 幂等：重复启动只跑未应用的 SQL，不会重复执行历史 SQL，多次拉起不会互相干扰。
3. 单条 SQL 执行失败时，**API 进程 fail-fast 退出**；docker `restart: unless-stopped` 策略下容器进入 restart loop，但 advisory lock + checksum 保证每次重启都是同一个 fail-fast 退出，不会出现"半成品 schema + 跑起来的 API"。
4. 镜像版本与 schema 真相绑定：SQL 文件随镜像一起分发，部署者拉到的镜像 tag 就是当时的 schema 状态，消除"代码到了 v2，SQL 还停在 v1"的可能。

## 非目标

本次明确不做：

- **不引入 GORM AutoMigrate**：本方案是"按手写 SQL forward-only 跑"，schema 真相依旧是 `infrastructure/database/*.sql`，不让框架反射猜 DDL。
- **不实现 down migration**：错误用新增前向 SQL 修复，不维护逆向脚本（与 Flyway 默认行为对齐）。
- **不重构现有 SQL 文件命名 / 内容**：`YYYYMMDD_NN_description.sql` 命名规则、幂等性要求、archive 归档边界保持不变。
- **不拆独立的迁移容器**：迁移逻辑内嵌于 `cmd/server` 启动期，不新增 service、不调整 `depends_on`、不要求 `docker compose ≥ v2.17`；权衡见"方案设计 → 5. 失败路径与边界条件"。
- **不保留 `services/api/cmd/migrate` 工具**：迁移内嵌后该工具失去价值，本次一并删除；本地空库一步到位由 `go run ./cmd/server` 自然接管。
- **不处理 K8s 多副本部署**：docker compose 单副本场景下不存在竞态；多副本场景仅通过 PG advisory lock 留好挂钩位，不展开调度策略。
- **不在本方案中做 baseline 收口**：是否再做一轮 baseline、何时归档已应用的增量 SQL，留给独立的 baseline 收口任务。
- **不重做 `VerifySchema` 的指纹清单结构**：本方案保留 `schemaFingerprintColumns / schemaFingerprintIndexes`，并在 backfill 模式下复用该清单做"已应用"探测（详见"4. 关键流程"backfill 段落）；是否在记账机制稳定后把指纹清单收敛成"只校验 `schema_migrations` 表完整性"，留作后续优化任务。

## 当前事实

以当前代码和现行文档为准：

- 相关文档：
  - `infrastructure/database/README.md`：现行 SQL 真相与升级流程
  - `docs/runbooks/deployment-environment.md`：部署入口
  - `docs/system-architecture.md`：架构说明
- 相关服务 / 模块：
  - `services/api/cmd/migrate/main.go`：当前的"空库一次性应用"工具，本次将删除
  - `services/api/cmd/server/main.go`：当前启动序列 `InitDB → VerifySchema → Bootstrap → Start`
  - `services/api/internal/db/db.go`：`InitDB` / `VerifySchema` / `Bootstrap`，含 `schemaFingerprintColumns / schemaFingerprintIndexes`
  - `services/api/Dockerfile`：当前只编译并打包 `ember`（server）单一二进制；build context 为 `services/api/`，**不包含**仓库根的 `infrastructure/database/`
  - `.github/workflows/build-api.yml`：CI 构建入口，`context: ./services/api` / `file: ./services/api/Dockerfile`
  - `infrastructure/docker/docker-compose.yml`：`postgres` 挂载 `./initdb` 至 `/docker-entrypoint-initdb.d`，`ember-api` 仅 `depends_on: postgres healthy`
  - `infrastructure/docker/initdb/`：与 `infrastructure/database/` 顶层文件双源同步
- 当前行为：
  - 空库首次启动：PG initdb 自动跑 `initdb/` 全部 SQL，API 起来即就绪。
  - 已有数据库升级：PG initdb 不再触发；新增的顶层 SQL 必须由部署者手工执行；API 启动期 `VerifySchema` 缺一即拒。
  - `cmd/migrate` 不感知"已应用"，重复执行依赖每条 SQL 自身幂等。
- 现有限制：
  - `infrastructure/docker/initdb/` 仅服务于"数据卷为空"这一时刻，对升级链路完全无效。
  - `schemaFingerprintColumns / schemaFingerprintIndexes` 必须随每条新增 SQL 手工维护，遗漏后启动期才报错。
  - SQL 真相需要在 `infrastructure/database/` 与 `infrastructure/docker/initdb/` 两处保持同步，靠 `cp` 维系。
  - Dockerfile build context 为 `services/api/`，无法 COPY 仓库根的 `infrastructure/database/*.sql` 进镜像。

## 方案设计

### 1. 用户可见行为

新增：

- 部署者执行 `docker compose pull && docker compose up -d` 时，`ember-api` 容器启动期先完成所有未应用 SQL 的应用，再进入 HTTP 服务循环，部署者无需任何手工步骤。
- 启动日志中 migration 阶段以 `[Migrate]` 前缀标注，便于与 API 业务日志区分。

修改：

- `infrastructure/docker/initdb/` 不再承担"升级"职责；首启依旧由 PG initdb 跑 baseline，但增量升级路径改由 API 启动期 migrate 接管。本方案先**保持双轨**（initdb/ 仍存在），落地稳定后再瘦身（见"落地后文档处理"）。**[2026-05-04 更新]** 该子目录与 PG `initdb.d` 挂载已由 OSS 部署体验方案 phase 2 提前退役，schema 初始化与升级全部由启动期 Migrate 接管。
- `services/api/cmd/migrate` 工具被删除。本地空库一步到位由 `go run ./cmd/server` 自然接管：启动期 migrate 阶段在空库分支跑全部 SQL，与原 `cmd/migrate` 行为等价但更连贯。

必须保持不变：

- `cmd/server` 对外 HTTP API、Internal API、Bot webhook、Telegram 命令、cron 入口
- `docker compose` 现有 service 拓扑（不新增 service、不调整 `depends_on`、不要求 `docker compose ≥ v2.17`）
- 现有 SQL 文件命名、目录结构、archive 边界

### 2. 数据与模型

新增一张内部表，仅供迁移系统自用：

| 字段 | 类型 | 语义 |
|---|---|---|
| `filename` | TEXT PRIMARY KEY | 顶层 SQL 文件名（如 `20260427_04_bot_pending_reject_message_context.sql`） |
| `applied_at` | TIMESTAMPTZ NOT NULL DEFAULT now() | 应用时间 |
| `checksum` | TEXT NOT NULL | SQL 文件内容指纹（SHA-256 hex，按字节计算，不做行尾规范化） |

约束：

- 表由 `internal/db/migrate.go` 在执行前 `CREATE TABLE IF NOT EXISTS` 自建，不污染 baseline 与业务模型。
- 不通过 GORM 模型管理，不出现在 `services/api/internal/models/` 下。
- `checksum` 用于检测"已应用 SQL 文件被改写"的反常情况，发现即 fail-fast，不静默重应用。
- 仓库根 `.gitattributes` 强制 `*.sql text eol=lf`，确保所有 SQL 文件在工作树里都是 LF；这是 checksum 算法跨平台可重复的前提（Windows 开发者克隆后若行尾被改成 CRLF，本地与镜像内 hash 会不一致）。

不改任何现有业务表。

### 3. 接口与边界

`cmd/server` 启动序列变更：

- 原：`InitDB → VerifySchema → Bootstrap → Start`
- 新：`InitDB → Migrate → VerifySchema → Bootstrap → Start`

新增 `services/api/internal/db/migrate.go`（与 `db.go` 同包，可直接复用 `schemaFingerprint*`，无需对外导出）：

- 入口：包级函数 `Migrate() error`，由 `cmd/server` 在 `InitDB` 后立刻调用
- 输入：环境变量 `EMBER_MIGRATIONS_DIR`（容器内默认 `/app/migrations`，本地未设时按 `cmd/migrate` 原候选路径 fallback 到工作树 `infrastructure/database/`）
- 输出：返回 `error`；`cmd/server` 拿到非 nil 后 `log.Fatal` 退出
- 行为契约：见"关键流程"

修改 `services/api/Dockerfile`：

- builder 仅编译 `ember`（cmd/server）单一二进制（cmd/migrate 已删除）
- runtime 阶段 COPY `infrastructure/database/*.sql` 至镜像内 `/app/migrations/`
- 镜像内置 `ENV EMBER_MIGRATIONS_DIR=/app/migrations`
- **build context 上提至仓库根**：原因是 `infrastructure/database/*.sql` 位于仓库根，不在原 `services/api/` context 下；上提后 Dockerfile 内全部 COPY 路径加 `services/api/` 前缀

修改 `.github/workflows/build-api.yml`：

- `context: .`
- `file: ./services/api/Dockerfile`

仓库根新增 `.dockerignore`：限制上提后的 build context 只包含必要内容（`services/api/`、`infrastructure/database/` 顶层 SQL），排除 `services/web/`、`services/bot/`、`docs/`、`infrastructure/database/archive/`、`.git/`、`.github/`、各类 `node_modules` 等。

修改 `infrastructure/docker/docker-compose.yml`：

- 仅调整本地 build 注释里的 context：`context: ../../`、`dockerfile: services/api/Dockerfile`
- service 拓扑、`depends_on`、env 列表、健康检查均不变

不修改的接口：

- HTTP API、Internal API、Bot webhook、Telegram 命令、cron 入口
- `docker compose` service 拓扑

### 4. 关键流程

镜像构建期：

1. 编译 `ember`（cmd/server）
2. 拷贝仓库根 `infrastructure/database/*.sql` 到镜像内 `/app/migrations/`

部署期 `docker compose up -d`：

1. `postgres` 启动 → 健康检查通过
2. `ember-api` 启动 → `InitDB → Migrate → VerifySchema → Bootstrap → Start`

`cmd/server` 启动期 Migrate 阶段流程（封装在 `internal/db/migrate.go`，全过程日志带 `[Migrate]` 前缀）：

1. 用 `pg_try_advisory_lock(<int8 const>)` 尝试获取 PG advisory lock。常量在 `internal/db/migrate.go` 顶部 const `migrateAdvisoryLockKey` 声明，注释"占用，不要复用"。30s 超时窗口内每秒重试；超时仍抢不到则 fail-fast，提示"另一个 migrate 进程在运行；如确认无并发，`docker compose ps -a` 排查孤悬的 ember-api 容器后重启"
2. `CREATE TABLE IF NOT EXISTS schema_migrations (filename TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now(), checksum TEXT NOT NULL)`
3. 从 `EMBER_MIGRATIONS_DIR` 列出顶层 `.sql` 文件，按字典序排序
4. `SELECT filename, checksum FROM schema_migrations`，构造已应用集合
5. 根据数据库状态判断进入哪条分支（详见下方"启动期分支判断"）
6. 释放 advisory lock，正常返回；`cmd/server` 继续走 `VerifySchema → Bootstrap → Start`

**启动期分支判断**：

| 业务核心表存在 | `schema_migrations` 非空 | fingerprint 列/索引齐 | 缺失项关联 migration 是否都在目录顶层 | 分支 | 行为 |
|:-:|:-:|:-:|:-:|---|---|
| 否 | 否 | — | — | 新空库 | forward-only 跑全部目录 SQL，每条成功后写记账 |
| 是 | 否 | 是 | — | 老库 backfill | 把目录中全部 SQL 视为"已应用"灌入 `schema_migrations`，不实际执行 SQL（`INSERT ... ON CONFLICT DO NOTHING` 保证重入幂等） |
| 是 | 否 | 否 | 是 | 混合模式 | 缺失项关联的 migration → forward-only 跑（执行 SQL + 写记账）；其余 SQL → backfill 仅记账 |
| 是 | 否 | 否 | 否 | 老库 schema 不对齐 | fail-fast，提示"业务表存在但部分 fingerprint 列/索引缺失，且部分缺失项关联的 migration 文件不在 `infrastructure/database/` 顶层（通常意味着 schema 真的漂了，不是新增 SQL 漏同步）" |
| — | 是 | — | — | 正常 forward-only | 对每个文件：已应用且 checksum 一致 → 跳过；已应用但 checksum 不一致 → fail-fast 报告该文件并提示先排查行尾；未应用 → 在外层事务内执行 SQL + 同一事务写入记账行 |

业务核心表探测：检查 `users` / `plans` / `subscriptions` / `settings` 等代表性表是否存在。

fingerprint 探测：`schemaFingerprintColumns / schemaFingerprintIndexes` 中全部列/索引在数据库中都存在。`schemaFingerprintColumn / schemaFingerprintIndex` 的 `migration` 字段即"缺失项关联的 migration 文件名前缀"，拼上 `.sql` 后与目录顶层文件集合做交集即可判断是否进混合模式。

混合模式分支的目的：覆盖"业务核心表已存在但 fingerprint 不齐"的边界场景——例如手工预建了部分业务表、或老库已被部分人工增量覆盖但未写入 `schema_migrations`。此时业务核心表存在 + 部分 fingerprint 缺失 + 缺失项对应的 migration 仍在目录顶层，混合模式可以自动 forward-only 跑缺失的、backfill 其余的，让"一条命令完成升级"承诺仍然有效。

> 设计动机演进：本分支最初为覆盖"PG initdb baseline 漏同步"场景而设计；initdb/ 退役后该触发路径不再存在，分支保留用于上述边界场景与未来扩展。

老库 schema 不对齐分支保留为最后兜底：fingerprint 缺失 + 缺失项关联的 migration **不在目录顶层**（通常是已被合入 baseline 后归档到 `archive/`）。这种情况下数据库 schema 真的漂了，自动恢复不安全，必须人工对齐。

SQL 文件约束（与 README 同步）：

- 顶层 SQL 文件**不要自己写 `BEGIN` / `COMMIT`**，migrate 在外层包裹事务执行；嵌套事务在 PG 中表现为 savepoint，会让错误回滚行为变得难以预测。`DO $$ ... $$` 块仍允许（PG 视为单语句）
- 顶层 SQL 文件**不允许使用 `CREATE INDEX CONCURRENTLY` / `REINDEX CONCURRENTLY` / `VACUUM` / `ALTER SYSTEM` / `CREATE DATABASE`** 等 PG 不允许在事务内执行的 DDL。如不得不用，需放进 `archive/` 由人工执行，不进 forward-only 流。
- SQL 一旦被记账，文件内容不得再改写（含格式调整、注释新增、空白调整）；需修正只能新增前向 SQL

SQL 来源（运行时由 `EMBER_MIGRATIONS_DIR` 决定）：

- 容器内：`/app/migrations/`（镜像构建期 COPY 自仓库 `infrastructure/database/*.sql`，与镜像 tag 强绑定）
- 本地 `go run ./cmd/server`：fallback 到仓库工作树 `infrastructure/database/`（保留若干常见相对路径自适应）

这两份 SQL 在开发期可能不同步（开发者改了 SQL 但没构建新镜像）。生产 schema 真相**只认镜像内**那一份；本地行为只用于开发自查与首次空库初始化。

### 5. 失败路径与边界条件

失败行为：

- **单条 SQL 执行失败**：当前事务回滚，`schema_migrations` 不写入该文件；`cmd/server` 直接 `log.Fatal` 退出；docker `restart: unless-stopped` 策略下容器进入 restart loop。advisory lock + forward-only + checksum 保证每次重启都是同一个 fail-fast 退出，不会产生破坏；部署者通过 `docker logs ember-api --tail` 第一时间定位失败 SQL 文件名。

  恢复链路（生产）：镜像是只读的，无法 `docker exec` 删 `/app/migrations/` 内文件——必须"修复仓库 SQL → 重新构建并推送镜像 → `docker compose pull` → `up -d`"。如失败 SQL 不可修，按 forward-only 原则**追加一条新 SQL** 抵消错误效果，不允许修改原文件（修改原文件即破坏 checksum）。

  恢复链路（本地 `go run ./cmd/server`）：直接修仓库 SQL 后重跑；本地若需重置 `schema_migrations` 表，删表后下一轮启动按"启动期分支判断"重新选支。
- **checksum 不一致**：fail-fast，提示哪个文件、当前 checksum 与历史 checksum；先排查 SQL 行尾（`.gitattributes` 强制 LF）；不自动重应用，须按上一条恢复链路处理。
- **advisory lock 抢占失败 / 超时**：30s 重试窗口仍抢不到即报错退出。docker compose 单副本场景下通常意味着上一个 ember-api 容器孤悬未退出——`docker compose ps -a` 排查后重启即可恢复；多副本场景留作未来扩展。
- **数据库不可达**：与现有 `InitDB` 行为一致，连接失败即 fail-fast 退出。

启动期内嵌 vs 独立容器的权衡（设计选择记录）：

- 内嵌路径下 migration 失败导致 API restart loop（而非独立 ember-migrate 容器中的 `Created` 状态阻断）。restart loop 中 advisory lock + forward-only + checksum 让每次重试都是同一个 fail-fast 退出，不会产生破坏，但会刷日志。
- 选择内嵌的收益：
  - 部署模型更简单，不新增一次性 service；
  - 不依赖 `docker compose ≥ v2.17`；
  - 失败定位仍然干净（`[Migrate]` 日志前缀 + `docker logs ember-api` 直接看到失败 SQL 文件名）。
- 这是 Ember"个人 / 小团队 docker compose 单机部署"场景下的取舍，本方案选内嵌。

兼容性约束：

- 不能改写已被 PG initdb 应用过的 SQL 文件内容（这是 checksum 机制的隐含约束，应在 `infrastructure/database/README.md` 写明）。
- 不能让 `infrastructure/docker/initdb/` 与 `infrastructure/database/` 之间的双源同步规则变得更复杂；本方案至少应让"未来"删掉 `initdb/` 这一份成为可能。
- `VerifySchema` 暂时保留兜底语义，但允许后续重构为基于 `schema_migrations` 表完整性的检查。

回滚策略：

- 本方案上线后若出现严重问题，部署者可降级到旧版 `EMBER_API_IMAGE`，老镜像不知道 `schema_migrations` 表，但**该表仅作为新版自用**，不影响老 API 启动；老 API 仍走 `VerifySchema` 路径。
- 已应用 SQL 不会因为镜像降级被回退（forward-only 原则）。

## 影响范围

涉及的子系统：

- API：
  - 新增 `services/api/internal/db/migrate.go`：advisory lock + `schema_migrations` 自建表 + checksum + 四分支启动期判断（新空库 / 老库 backfill / 老库不对齐 / 正常 forward-only）全流程
  - `services/api/cmd/server/main.go`：启动序列追加 Migrate 阶段
  - 删除 `services/api/cmd/migrate/`
  - 新增对应自动化测试：forward-only / backfill / checksum 三项行为
- Web：无
- Bot：无
- 配置 / 部署：
  - `services/api/Dockerfile`：单二进制 + SQL 内嵌 + COPY 路径前缀调整（context 上提的连带改动）
  - `.github/workflows/build-api.yml`：build context 上提
  - 仓库根 `.dockerignore`：新增，限制 build context 范围
  - 仓库根 `.gitattributes`：新增 `*.sql text eol=lf`
  - `infrastructure/docker/docker-compose.yml`：本地 build 注释 context 调整；service 拓扑不变
  - `.env.example` 不需要新增变量
- 文档：
  - `infrastructure/database/README.md`：补充"自动迁移机制 + 不要修改已应用 SQL"约束；删除 `cmd/migrate` 工具相关段落
  - `docs/runbooks/deployment-environment.md`：升级流程更新为 `pull + up -d`
  - `docs/system-architecture.md`：在数据库章节补充 `schema_migrations` 与启动期 Migrate 阶段
  - `infrastructure/docker/initdb/` 双轨期间的同步规则可暂保留，待 backfill 在生产稳定后单独发起退役计划

## 验证方式

### 编译 / 测试

- `cd services/api && go build ./...`
- `cd services/api && go test ./...`（含 forward-only / backfill / checksum 三项行为的自动化测试）
- `cd infrastructure/docker && docker compose config`（compose 文件语法校验）

### 手工验证

- **空库首次部署（容器路径）**：清空数据卷 → `docker compose up -d` → API 启动期日志显示 `[Migrate]` 进入"新空库"分支（业务核心表不存在 + `schema_migrations` 为空，PG `initdb.d` 已退役不再预先跑 baseline）→ 按字典序 forward-only 跑全部目录 SQL → API 正常对外提供服务 → `SELECT count(*) FROM schema_migrations` 等于目录文件数。
- **空库首次开发（本地路径）**：本地完全空的 PG 库（无业务表）→ `go run ./cmd/server` → API 启动期日志显示 `[Migrate]` 进入"新空库"分支跑全部 SQL → API 正常对外提供服务。
- **已有数据库升级**：使用既有数据卷 → 拉新镜像（包含一条新增测试 SQL）→ `up -d` → API 启动期日志显示 `[Migrate]` 进入"正常 forward-only"分支仅执行新增 SQL → API 正常对外提供服务。
- **重复 up -d**：连续执行两次 `docker compose up -d` → API 第二次启动期 `[Migrate]` 阶段跑 0 条 SQL，正常进入 Start。
- **失败 SQL 触发 restart loop**：故意构造一条会失败的测试 SQL（在临时分支 / 临时镜像）→ API 启动期 `log.Fatal` → 容器进入 restart loop → `docker logs ember-api --tail` 第一时间看到失败 SQL 文件名 → 删除该 SQL 后再次 `up -d` 恢复。
- **checksum 防护**：手工修改一条已记账 SQL 文件内容 → API 启动期 fail-fast 并报告该文件 → 还原后恢复。
- **混合模式**：构造一份未应用 SQL 关联到一条新增 fingerprint，但其它 fingerprint 全齐 → API 启动期日志显示 `[Migrate]` 进入"混合模式"分支，缺失项关联的 SQL 走 forward-only，其余 SQL 走 backfill → `SELECT count(*) FROM schema_migrations` 等于目录文件数。
- **老库 schema 不对齐探测**：手工建一个缺失 fingerprint 且对应 migration 已不在目录顶层（仅在 archive/）的库 → `up -d` → fail-fast，提示"业务表存在但部分 fingerprint 列/索引缺失，且部分缺失项关联的 migration 文件不在目录顶层"。

## 落地后文档处理

落地后应同步处理：

- `infrastructure/database/README.md`：
  - 新增"自动迁移与 schema_migrations"章节，明确"已应用 SQL 不可改写"、"SQL 文件强制 LF / 不写 BEGIN/COMMIT" 与"禁用 CREATE INDEX CONCURRENTLY 等不能在事务内执行的 DDL" 约束
  - 移除"使用方式 → 2. 本地空库初始化 → cmd/migrate"段落，改为"本地空库一步到位由 `go run ./cmd/server` 自然接管"
  - 移除"已有数据库升级 → 手工执行 SQL"步骤
  - 保留"添加新 migration → 必做事项"中的 fingerprint 维护要求（仍用于 VerifySchema 兜底与混合模式探测）
- `docs/runbooks/deployment-environment.md` 更新升级章节，把流程精简为 `pull + up -d`
- `docs/system-architecture.md` 在数据库章节补充 `schema_migrations` 表与启动期 Migrate 阶段说明
- 仓库根落地 `.gitattributes` 与 `.dockerignore`（与方案设计同步，落地时校验 git 工作树确实生效）
- `infrastructure/docker/initdb/` 退役：已由 [OSS 部署体验方案](./oss-deployment-experience.md) phase 2 提前评估并执行退役（commit 见 OSS plan 的"Phase 2 落地记录"段落）。退役理由：自动化测试已覆盖"新空库"分支、当前尚无活跃 OSS 部署，PG `initdb.d` 双轨简化为单轨，schema 初始化与升级全部由启动期 Migrate 接管

归档条件：

- 上述文档同步完成
- 至少一次完整发版按本方案完成自动迁移
- `internal/db/migrate.go` 的 forward-only / backfill / checksum 三项行为有自动化测试覆盖

上述条件已满足，本计划已迁入 `docs/archive/plan/architecture/`。
