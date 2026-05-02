# 数据库迁移自动应用方案

> 状态：草稿
> 负责人：Ember
> 更新时间：2026-05-02

## 背景

当前 Ember 的数据库 schema 升级路径在 docker compose 部署场景下是断裂的：

- 部署者主流路径是 `docker compose pull && docker compose up -d`，但 Postgres 容器只在数据卷为空时执行一次 `infrastructure/docker/initdb/`，**之后任何新增 SQL 文件都不会被 PG 自动应用**。
- API 容器启动期 `VerifySchema` 会因为缺表 / 缺列 / 缺索引而 fail-fast，部署者必须手工连进 `postgres` 容器、按 `infrastructure/database/README.md` 顺序执行 baseline 之后的全部 SQL，再重启 API。
- 这条手工链路对个人 / 小团队部署者门槛过高，是当前升级体验的核心痛点；遗漏一条 SQL 就意味着 API 起不来或线上数据漂移。
- 现有 `services/api/cmd/migrate` 已经能从空库一次性应用全部 SQL，但**没有跟踪"哪些已应用"**，也没有任何容器生命周期对接，等于工具具备但通路没接通。

如果继续维持现状，每次发版都需要在发布说明里逐条提醒部署者手工跑 SQL；漏跑导致的故障只能在 API 启动失败后被动发现，恢复成本高。

## 目标

本方案要实现：

1. 部署者只需 `docker compose pull && docker compose up -d`，**所有未应用的顶层 SQL 在 API 启动前自动按字典序应用完毕**，无需任何人工 SQL 操作。
2. 迁移执行器升级为 forward-only 幂等：重复启动只跑未应用的 SQL，不会重复执行历史 SQL，多次拉起不会互相干扰。
3. 单条 SQL 执行失败时，迁移整体 exit 非 0，**`ember-api` 容器被 docker compose 的依赖条件阻断启动**，避免出现"半成品 schema + 跑起来的 API"。
4. 镜像版本与 schema 真相绑定：SQL 文件随镜像一起分发，部署者拉到的镜像 tag 就是当时的 schema 状态，消除"代码到了 v2，SQL 还停在 v1"的可能。
5. 现有空库初始化路径 (`go run ./cmd/migrate`) 继续可用，行为对老用户无感。

## 非目标

本次明确不做：

- **不引入 GORM AutoMigrate**：本方案是"按手写 SQL forward-only 跑"，schema 真相依旧是 `infrastructure/database/*.sql`，不让框架反射猜 DDL。
- **不实现 down migration**：错误用新增前向 SQL 修复，不维护逆向脚本（与 Flyway 默认行为对齐）。
- **不重构现有 SQL 文件命名 / 内容**：`YYYYMMDD_NN_description.sql` 命名规则、幂等性要求、archive 归档边界保持不变。
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
  - `services/api/cmd/migrate/main.go`：当前的"空库一次性应用"工具
  - `services/api/internal/db/db.go`：`InitDB` / `VerifySchema` / `Bootstrap`
  - `services/api/Dockerfile`：当前只编译并打包 `ember`（server）单一二进制
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

## 方案设计

### 1. 用户可见行为

前置环境约束：

- 部署机的 `docker compose` 必须 ≥ v2.17（`service_completed_successfully` 依赖条件最早在该版本引入）。老版本 compose 会**静默忽略**该依赖，导致 API 在迁移失败时仍可能启动，违反本方案核心保证；部署文档需在头部声明该要求。

新增：

- 部署者执行 `docker compose pull && docker compose up -d` 时，新增的 SQL 自动应用到现有数据库，无需任何手工步骤。
- `ember-migrate` 容器作为一次性 service 出现在 `docker ps -a` 中，跑完即退出，退出码反映迁移结果。

修改：

- `infrastructure/docker/initdb/` 不再承担"升级"职责；首启依旧由 PG initdb 跑 baseline，但增量升级路径改由 `ember-migrate` 容器接管。本方案先**保持双轨**（initdb/ 仍存在），落地稳定后再瘦身（见"落地后文档处理"）。

必须保持不变：

- `services/api/cmd/server/main.go` 的启动顺序、API 行为、`VerifySchema` 校验语义。
- `services/api/cmd/migrate` 在本地空库场景下的使用方式 (`go run ./cmd/migrate`) 行为兼容，仅"已应用即跳过"是新增能力。
- 现有 SQL 文件命名、目录结构、archive 边界。

### 2. 数据与模型

新增一张内部表，仅供迁移系统自用：

| 字段 | 类型 | 语义 |
|---|---|---|
| `filename` | TEXT PRIMARY KEY | 顶层 SQL 文件名（如 `20260427_04_bot_pending_reject_message_context.sql`） |
| `applied_at` | TIMESTAMPTZ NOT NULL DEFAULT now() | 应用时间 |
| `checksum` | TEXT NOT NULL | SQL 文件内容指纹（SHA-256 hex，按字节计算，不做行尾规范化） |

约束：

- 表由 `ember-migrate` 在执行前 `CREATE TABLE IF NOT EXISTS` 自建，不污染 baseline 与业务模型。
- 不通过 GORM 模型管理，不出现在 `services/api/internal/models/` 下。
- `checksum` 用于检测"已应用 SQL 文件被改写"的反常情况，发现即 fail-fast，不静默重应用。
- 仓库根 `.gitattributes` 强制 `*.sql text eol=lf`，确保所有 SQL 文件在工作树里都是 LF；这是 checksum 算法跨平台可重复的前提（Windows 开发者克隆后若行尾被改成 CRLF，本地与镜像内 hash 会不一致）。

不改任何现有业务表。

### 3. 接口与边界

新增二进制 `ember-migrate`：

- 入口：`services/api/cmd/migrate/main.go`（基于现有同名工具升级，不新建独立目录）
- 输入：`EMBER_MIGRATIONS_DIR`、`DATABASE_URL`（语义不变）
- 输出：退出码 0 = 全部已应用或无新增；非 0 = 失败，并在 stderr 给出失败的 SQL 文件名
- 行为契约：见"关键流程"

修改 `services/api/Dockerfile`：

- builder 阶段同时编译 `ember`（server）与 `ember-migrate`
- runtime 阶段同时 COPY 两个二进制
- runtime 阶段 COPY `infrastructure/database/*.sql` 至镜像内 `/app/migrations/`
- 镜像内置 `ENV EMBER_MIGRATIONS_DIR=/app/migrations`

修改 `infrastructure/docker/docker-compose.yml`：

- 新增一次性 service `ember-migrate`：
  - 复用 `${EMBER_API_IMAGE}` 镜像
  - `command: ["/app/ember-migrate"]`
  - `depends_on: postgres { condition: service_healthy }`
  - `restart: "no"`
  - 与 `ember-api` 共用最小必要的环境变量（`DATABASE_URL`）
- 修改 `ember-api`：在 `depends_on` 中追加 `ember-migrate: { condition: service_completed_successfully }`

不修改的接口：

- HTTP API、Internal API、Bot webhook、Telegram 命令、cron 入口
- `cmd/server` 的启动期 `InitDB → VerifySchema → Bootstrap` 序列

### 4. 关键流程

镜像构建期：

1. 编译 `ember`、`ember-migrate` 两个二进制
2. 拷贝 `infrastructure/database/*.sql` 到镜像内固定路径

部署期 `docker compose up -d`：

1. `postgres` 启动 → 健康检查通过
2. `ember-migrate` 启动 → 应用所有未应用 SQL → 退出 0
3. `ember-api` 启动 → `InitDB → VerifySchema → Bootstrap`

`ember-migrate` 内部流程：

1. 连接数据库；用 `pg_try_advisory_lock(<int8 const>)` 尝试获取 PG advisory lock。常量在 `services/api/cmd/migrate/main.go` 顶部 const `migrateAdvisoryLockKey` 声明，注释"占用，不要复用"。30s 超时窗口内每秒重试；超时仍抢不到则 fail-fast，提示"另一个 migrate 进程在运行；如确认无并发，`docker compose ps -a` 排查孤悬容器并 `docker rm` 后重试"
2. `CREATE TABLE IF NOT EXISTS schema_migrations (...)`
3. 从 `EMBER_MIGRATIONS_DIR` 列出顶层 `.sql` 文件，按字典序排序
4. `SELECT filename, checksum FROM schema_migrations`，构造已应用集合
5. 对每个文件：
   - 已应用且 checksum 一致 → 跳过
   - 已应用但 checksum 不一致 → fail-fast，报错指明文件，提示先排查 SQL 行尾（`.gitattributes` 强制 LF）
   - 未应用 → 在外层事务内执行 SQL；成功后在同一事务内写入 `schema_migrations` 一行；提交
6. 释放 advisory lock，正常退出

SQL 文件约束（与 README 同步）：

- 顶层 SQL 文件**不要自己写 `BEGIN` / `COMMIT`**，migrate 在外层包裹事务执行；嵌套事务在 PG 中表现为 savepoint，会让错误回滚行为变得难以预测。`DO $$ ... $$` 块仍允许（PG 视为单语句）
- SQL 一旦被记账，文件内容不得再改写（含格式调整、注释新增、空白调整）；需修正只能新增前向 SQL

空库首启场景：

- PG initdb 已经把 `initdb/` 下的 SQL 执行完毕（与现状一致）
- `ember-migrate` 第一次启动时，自建 `schema_migrations`，进入 backfill 模式预写入跟踪表（首启不重复执行）

backfill 模式触发条件（必须**同时满足**）：

1. 业务核心表已存在（探测 `users` / `plans` / `subscriptions` 等若干代表性表）
2. `schema_migrations` 不存在或为空
3. 目录中**每条 SQL** 引入的代表性列 / 索引（基于 `services/api/internal/db/db.go` 的 `schemaFingerprintColumns` / `schemaFingerprintIndexes`）在数据库中**全部存在**

任意一项不满足 → fail-fast，提示"该库不是干净状态，请先按 `infrastructure/database/README.md` 手工对齐 schema 后再启动 migrate"。这一收紧避免老库手工漏跑某条 SQL 时，backfill 把缺失文件误标记为"已应用"导致永久静默漂移——这是设计上最致命的风险，必须以 fingerprint 兜底。

通过 backfill 触发条件的环境（含空库 + 已通过 initdb 全量应用的环境 + 已手工跑齐全部增量的老线上库）：把当前目录中的全部顶层 SQL 视为"已应用"灌入 `schema_migrations`，之后的增量进入正常 forward-only 路径。这一过程对**重入幂等**——重复启动不重复 backfill。

SQL 来源（运行时由 `EMBER_MIGRATIONS_DIR` 决定）：

- 容器内：`/app/migrations/`（镜像构建期 COPY 自仓库 `infrastructure/database/*.sql`，与镜像 tag 强绑定）
- 本地 `go run ./cmd/migrate`：仓库工作树 `infrastructure/database/`

这两份 SQL 在开发期可能不同步（开发者改了 SQL 但没构建新镜像）。生产 schema 真相**只认镜像内**那一份；本地行为只用于开发自查与首次空库初始化。

### 5. 失败路径与边界条件

失败行为：

- **单条 SQL 执行失败**：当前事务回滚，`schema_migrations` 不写入该文件；进程 exit 非 0；后续 SQL 不再执行；`ember-api` 容器因 `service_completed_successfully` 条件不满足而不启动；部署者通过 `docker compose logs ember-migrate` 定位失败 SQL。

  恢复链路（生产）：镜像是只读的，无法 `docker exec` 删 `/app/migrations/` 内文件——必须"修复仓库 SQL → 重新构建并推送镜像 → `docker compose pull` → `up -d`"。如失败 SQL 不可修，按 forward-only 原则**追加一条新 SQL** 抵消错误效果，不允许修改原文件（修改原文件即破坏 checksum）。

  恢复链路（本地 `cmd/migrate`）：直接修仓库 SQL 后重跑；本地若需重置 `schema_migrations` 表，删表后下一轮 migrate 进入 backfill。
- **checksum 不一致**：fail-fast，提示哪个文件、当前 checksum 与历史 checksum；先排查 SQL 行尾（`.gitattributes` 强制 LF）；不自动重应用，须按上一条恢复链路处理。
- **advisory lock 抢占失败 / 超时**：30s 重试窗口仍抢不到即报错退出。docker compose 单副本场景下通常意味着上一轮 migrate 容器孤悬未退出——`docker compose ps -a` 排查并 `docker rm` 后重试；多副本场景留作未来扩展。
- **数据库不可达**：与现有 `InitDB` 行为一致，连接失败即 exit 非 0。

兼容性约束：

- 不能破坏 `go run ./cmd/migrate` 在本地空库场景下的行为；新增的 backfill / 跟踪表行为对它同样适用，且应保证多次执行不报错。
- 不能改写已被 PG initdb 应用过的 SQL 文件内容（这是 checksum 机制的隐含约束，应在 `infrastructure/database/README.md` 写明）。
- 不能让 `infrastructure/docker/initdb/` 与 `infrastructure/database/` 之间的双源同步规则变得更复杂；本方案至少应让"未来"删掉 `initdb/` 这一份成为可能。
- `VerifySchema` 暂时保留兜底语义，但允许后续重构为基于 `schema_migrations` 表完整性的检查。

回滚策略：

- 本方案上线后若出现严重问题，部署者可降级到旧版 `EMBER_API_IMAGE`，老镜像不知道 `schema_migrations` 表，但**该表仅作为新版自用**，不影响老 API 启动；老 API 仍走 `VerifySchema` 路径。
- 已应用 SQL 不会因为镜像降级被回退（forward-only 原则）。

## 影响范围

涉及的子系统：

- API：`services/api/cmd/migrate/main.go` 增量化、引入 `schema_migrations` 自建表逻辑；`services/api/internal/db/` 可能新增极小的 lock / checksum 工具（若放在 db 包则需明示职责）。
- Web：无。
- Bot：无。
- 配置 / 部署：
  - `services/api/Dockerfile`：双二进制 + SQL 内嵌
  - `infrastructure/docker/docker-compose.yml`：新增 `ember-migrate` service，调整 `ember-api.depends_on`；要求部署机 `docker compose ≥ v2.17`（`service_completed_successfully` 依赖条件起始版本），低版本会静默忽略该依赖
  - 仓库根 `.gitattributes`：新增 `*.sql text eol=lf`，确保 SQL 文件跨平台 LF（checksum 跨平台可重复的前提）
  - `.env.example` 不需要新增变量
- 文档：
  - `infrastructure/database/README.md`：补充"自动迁移机制 + 不要修改已应用 SQL"约束
  - `docs/runbooks/deployment-environment.md`：升级流程更新为 "pull + up -d"
  - `docs/system-architecture.md`：在数据库章节补充 `schema_migrations` 与 `ember-migrate` 容器
  - `infrastructure/docker/initdb/` 双轨期间的同步规则可暂保留，待 backfill 在生产稳定后单独发起退役计划

## 验证方式

### 编译 / 测试

- `cd services/api && go build ./...`
- `cd services/api && go test ./...`
- `cd infrastructure/docker && docker compose config`（compose 文件语法校验）

### 手工验证

- **空库首次部署**：清空数据卷 → `docker compose up -d` → 观察 `ember-migrate` 进入 backfill 模式 → `ember-api` 正常启动 → `SELECT count(*) FROM schema_migrations` 等于目录文件数。
- **已有数据库升级**：使用既有数据卷 → 拉新镜像（包含一条新增测试 SQL）→ `up -d` → 观察 `ember-migrate` 仅执行新增 SQL → `ember-api` 正常启动。
- **重复 up -d**：连续执行两次 `docker compose up -d` → `ember-migrate` 第二次跑 0 条 SQL，正常退出 → `ember-api` 不重启或仅按 compose 默认行为重启。
- **失败 SQL 阻断 API**：故意构造一条会失败的测试 SQL（在临时分支 / 临时镜像）→ `ember-migrate` 退出非 0 → `ember-api` 因 `service_completed_successfully` 条件不满足保持 `Created` 状态 → 删除该 SQL 后再次 `up -d` 恢复。
- **checksum 防护**：手工修改一条已记账 SQL 文件内容 → `ember-migrate` fail-fast 并报告该文件 → 还原后恢复。
- **本地 `go run ./cmd/migrate`**：在本地空库与本地非空库分别执行 → 行为分别为"全部应用并记账" / "仅补未应用"，不报错。

## 落地后文档处理

落地后应同步处理：

- `infrastructure/database/README.md` 更新：
  - 新增"自动迁移与 schema_migrations"章节，明确"已应用 SQL 不可改写"与"SQL 文件强制 LF / 不写 BEGIN/COMMIT"约束
  - 移除手工执行升级 SQL 的步骤
- `docs/runbooks/deployment-environment.md` 更新升级章节，把流程精简为 `pull + up -d`，并在头部声明 `docker compose ≥ v2.17` 要求
- `docs/system-architecture.md` 在数据库章节补充 `ember-migrate` 容器与 `schema_migrations` 表
- 在 `infrastructure/docker/README.md` 写清 `ember-migrate` service 的语义、`docker compose ≥ v2.17` 要求与 SQL LF / 不写 BEGIN/COMMIT 约束
- 仓库根落地 `.gitattributes`（与方案设计同步，落地时校验 git 工作树确实生效）
- 待 `infrastructure/docker/initdb/` 退役时，单独发起一份收口计划，本方案不强行覆盖该步骤

归档条件：

- 上述 4 处文档同步完成
- 至少一次完整发版按本方案完成自动迁移
- `cmd/migrate` 的 forward-only / backfill / checksum 三项行为有自动化测试覆盖

满足以上条件后，本计划迁入 `docs/archive/plan/architecture/`。
