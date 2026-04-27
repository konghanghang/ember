# Schema 与部署基线收口方案

> 状态：可进入归档准备（主干已落地）
> 负责人：Ember
> 更新时间：2026-04-29

## 落地进度

截至当前仓库事实，本方案主干已落地：

- ✅ `AUTO_MIGRATE=false` 基线、`VerifySchema` fail-fast、`initdb/` 子目录隔离已落地
- ✅ `users` lower unique、`payments` partial unique、`schema_alignment`、`airDate -> date`、`payments.expiresAt` 推进已落地
- ✅ API / Web / Bot 容器非 root、`init: true`、`stop_grace_period`、Bot 健康依赖已落地
- ✅ compose 已收口为显式固定 `EMBER_API_IMAGE` / `EMBER_WEB_IMAGE` / `EMBER_BOT_IMAGE`，不再接受 floating `latest`
- ✅ API / Web / Bot 的 `.dockerignore` 已补齐，空库初始化与镜像构建边界已和文档对齐
- ✅ 空库初始化标准入口已收口为 `go run ./cmd/migrate`，数据库 README 已同步当前事实

当前剩余项以部署治理和文档归档为主：

- 部署环境盘点、baseline 精简归档和交叉引用收口仍待后续整理
- 备份恢复与镜像发布 runbook 已有落点，但仍可继续做更细颗粒度的运营补充

## 归档判断

- 当前可以进入归档准备，但暂不直接归档。
- 原因：代码与部署基线已经稳定，剩余事项主要是 runbook 与归档整理；更适合先提炼稳定结论，再退出 `docs/plan/`。

## 稳定结论

以下结论已经稳定，可视为当前基线，而不是临时整改步骤：

- 启动期不再调用 `AutoMigrate`；空库初始化统一走 `go run ./cmd/migrate` + `VerifySchema`。
- PostgreSQL 首启执行链路固定为 `infrastructure/docker/initdb/`，`infrastructure/database/` 顶层是 SQL 真相源，`archive/` 不参与初始化。
- compose 部署必须显式提供固定镜像、强制注入核心凭证，并以 `service_healthy` / `init: true` / `stop_grace_period` 作为容器基线。
- Docker 镜像默认采用非 root 运行；数据库 schema 校验、迁移复制和 archive 边界已经成为持续维护规则。

## 交叉引用

- 当前系统事实：
  - [docs/system-architecture.md](</Users/konghang/data/github/ember/docs/system-architecture.md>) §13 已收录部署基线、固定镜像、`initdb/`、`cmd/migrate`、`VerifySchema`
- 当前数据库入口：
  - [infrastructure/database/README.md](</Users/konghang/data/github/ember/infrastructure/database/README.md>) 已收录顶层 migration、`initdb/` 同步规则、archive 边界
- 当前部署 / 发布 / 备份 runbook：
  - [docs/runbooks/deployment.md](</Users/konghang/data/github/ember/docs/runbooks/deployment.md>)
  - [docs/runbooks/deployment-environment.md](</Users/konghang/data/github/ember/docs/runbooks/deployment-environment.md>)
  - [docs/runbooks/release-process.md](</Users/konghang/data/github/ember/docs/runbooks/release-process.md>)
  - [docs/runbooks/database-backup.md](</Users/konghang/data/github/ember/docs/runbooks/database-backup.md>)
- 当前盘点入口：
  - [docs/proposals/plan-inventory.md](</Users/konghang/data/github/ember/docs/proposals/plan-inventory.md>) 已把本方案标为“可进入归档准备”

## 退场说明

- 本文档后续不再承担“现行部署规则”说明职责；现行基线以 `docs/system-architecture.md`、`infrastructure/database/README.md` 与 `docs/runbooks/` 为准。
- 在以下条件同时满足后，可移入 `docs/archive/plan/architecture/`：
  - `baseline` 精简归档、冲突清单归档和 archive 交叉引用已完成
  - `docs/plan/README`、`docs/proposals/README`、`docs/proposals/plan-inventory.md` 已同步把本方案从现行实施稿入口移除
  - 文中已完成条目不再承担新的决策说明，只剩历史追溯价值

## 修订记录

- 2026-04-26：项目级规则确认 **不引入数据库 FK / 级联策略**。下列条目作废，一致性由 services 层显式 sweep 负责，发现孤儿数据走"应用层删除路径补全 + cron 兜底清理"：
  - 目标段第 7 项 "出 `20260425_02_foreign_keys.sql`"
  - §2 数据与模型 表中 `20260425_02_foreign_keys.sql` 一行
  - §1 用户可见行为 中"按 FK 策略级联清理或显式 SET NULL"描述
  - §4.6 foreign_keys.sql 整节
  - §5 失败路径与边界条件 中 "FK 补充时存在孤儿数据" 一项
  - 影响范围 中 "20260425_02_foreign_keys.sql" 一份
  - 验证方式 §手工验证 → foreign_keys 整节
  - 修复后验证清单中 "FK 补充前的孤儿数据清理报告归档" 一项
  - 落地后文档处理中 "FK 与级联策略"
  - 附录 P1-7 (DB) "缺 FK + 级联" → §4.6
- 后续修订：上述条目仅保留于本文档作为决策溯源，不再纳入实施。

## 背景

2026-04-25 系统性 review 在数据库 schema / 部署 / 基础设施层集中暴露多类硬伤，整体品味评分 🔴：

- `infrastructure/docker/docker-compose.yml` 默认 `AUTO_MIGRATE=${AUTO_MIGRATE:-true}`，与 CLAUDE.md 硬规则"线上长期 AUTO_MIGRATE=false"直接冲突。
- PG initdb.d 把整个 `infrastructure/database/` 目录挂进容器，README / archive / 多份 SQL 一锅塞进去，行为未定义；任何在该目录加 `.sh` 脚本都会被首启执行。
- `users.email` 是"可空 + 唯一索引"，但模型字段是非指针 `string` + 非 NOT NULL → 空字符串重复必触发 23505，第二个空邮箱用户注册必失败。
- 默认管理员 seed 用 env 长期注入 `ADMIN_PASSWORD`，无强制改密；compose 默认 `POSTGRES_USER=postgres` / `POSTGRES_PASSWORD=password` + `5432:5432` 对外暴露。
- baseline 多对重复唯一 / 普通索引（payments / media_quality_caches / tmdb_cache / tv_calendar_* / client_blacklists）。
- baseline 仍保留 `users.inviteCode` 列 + `idx_users_invite_code`，但模型早已不再有该字段。
- baseline `subscriptions.uk_subscription_media` 与 `20260424_01` 删除后再建 partial unique 冲突，新装环境多做一对"先建后删"。
- 模型 `User.TelegramID` `uniqueIndex` 与 SQL partial 不一致；AutoMigrate 启用时会重建非 partial 唯一索引。
- `media_gaps.tmdbId` 模型层 `index` + `uniqueIndex` 复合，AutoMigrate 会建额外冗余索引。
- `bot` 容器 `depends_on: ember-api` 没带 `condition: service_healthy`，API 慢启动时 Bot 第一次 internal 调用必失败。
- `playback_rankings` snake_case 列名与项目硬规则 camelCase 冲突（事实保留，但需文档豁免）。
- `tv_calendar_items.airDate` / `media_gaps.airDate` 用 `timestamptz` 但语义是"00:00:00 UTC date"，无 CHECK 约束跨 DST 漂移。
- `tmdb_cache` / `email_verifications` / `telegram_bind_codes` / `device_actions` 等大表无清理调度或调度未配齐。
- 缺 FK + 级联策略：删用户 / 兑换码 / plan group 后留孤儿，应用层零碎兜底。【作废 2026-04-26】项目级规则不引入 FK；该问题由 services 层显式 sweep + 必要时新增 cron 兜底解决，不在本方案范围内。
- 连接池 MaxIdle=10 + MaxOpen=100 失衡，单 API 节点可能直接打满 PG 默认 max_connections=100。
- `db.go` 在 Docker 启动期仍尝试 `godotenv.Load(".env")` 多条相对路径，每次启动打 warn。
- API/Web/Bot Dockerfile 全用 floating tag（`golang:1.24.13-alpine` / `nginx:alpine` / `alpine:latest`），构建结果不可复现；`.dockerignore` 缺位。
- compose 用 `version: '3.8'`，新版 docker compose 已忽略并 warning。
- `subscriptions.note` 接口必填、DB 仍允许空，双标。
- `payments.expiresAt` 仅有索引但无 cron 状态推进。
- 备份 / 恢复 runbook 缺位。

如果不收口，会出现"复制粘贴上线即 RCE/DB takeover"、"AutoMigrate 默认开导致双源真相"、"users.email 空串注册必 500"、"baseline 重复索引写放大"、"删用户后 redemptions/subscriptions 留孤儿"等真实可触发的安全 / 数据 / 性能事故。

## 目标

本方案要实现：

1. compose 默认 `AUTO_MIGRATE=false`；从 API 启动路径完全移除 GORM `AutoMigrate` 兜底入口（或挪到独立 `make db-automigrate` 子命令）
2. PG initdb.d 改为只挂"可执行 SQL"的子目录 `infrastructure/docker/initdb/`；README / archive 不参与首启
3. `users.email` 改为 partial unique（`WHERE email IS NOT NULL AND email <> ''`），并改 partial unique on `lower(email)`；模型字段改 `*string`
4. 默认管理员 seed 改为"env 注入仅首启 + 一次性临时口令 + 强制改密"；compose 移除 `ADMIN_PASSWORD` 默认值
5. compose Postgres 改为 `${POSTGRES_USER:?}` / `${POSTGRES_PASSWORD:?}` 强制 env 入参；端口改 `127.0.0.1:5432:5432` 或不发布
6. 出 `20260425_01_schema_alignment.sql`：清重复索引、删 `users.inviteCode` 死字段、`users.telegramId` partial 与模型对齐、subscriptions partial 唯一收口、media_gaps 模型 tag 对齐
7. ~~出 `20260425_02_foreign_keys.sql`：补 FK + 级联策略（CASCADE / SET NULL）~~【作废 2026-04-26】项目级规则不引入 FK
8. baseline 同步精简（清重复索引、删死字段、收口 partial 唯一）
9. compose `bot` 加 `condition: service_healthy`；同时 compose 显式声明 `ember-api` healthcheck
10. `tmdb_cache` / `email_verifications` / `telegram_bind_codes` / `device_actions` 全部补"按 expiresAt / createdAt 清理"调度，统一在 cron 中收口
11. `payments.expiresAt` 引入 cron 推进 `pending → expired`
12. 连接池 MaxOpen 调到 30，MaxIdle 调到 15；compose 显式声明 PG `max_connections=200` 或建议
13. `db.go` 启动期 `godotenv.Load` 改为仅在非容器环境调用（`EMBER_DOTENV` 或路径存在性判断）
14. API/Web/Bot Dockerfile 锁版本（pin tag + sha 可选）；新增 `.dockerignore`；Web/Bot 切非 root 用户
15. compose 删 `version: '3.8'`；补 `init: true / stop_grace_period`
16. `subscriptions.note` 在 resubmit 路径明确 NOT NULL（DB 改 NOT NULL 或保持但 resubmit 业务校验已收口）
17. `tv_calendar_items.airDate` / `media_gaps.airDate` 改 `date` 类型，避免 DST 漂移
18. README / 部署手册新增"备份与恢复 runbook"；`playback_rankings` snake_case 在文档豁免

## 非目标

本次明确不做：

- 不引入第三方 migration 执行器（仍是手写 SQL + 顺序执行）
- 不重构业务表结构（仅清理冗余 + 改 airDate 类型；FK 补齐已被项目级规则否决，参见修订记录）
- 不替换 PostgreSQL 大版本（保留 16.x）
- 不引入新的备份方案（仅写 runbook 引导）
- 不改 Dockerfile 的运行时基础镜像（仅锁版本 + 切非 root）
- 不改 `playback_rankings` 列名（成本过高，文档豁免）

## 当前事实

以当前代码和现行文档为准，现状如下：

- 相关文档：
  - `docs/system-architecture.md` §13、§11
  - `infrastructure/database/README.md`
  - `docs/runbooks/deployment.md`、`docs/runbooks/deployment-environment.md`、`docs/runbooks/release-process.md`
- 相关文件：
  - `infrastructure/docker/docker-compose.yml`
  - `infrastructure/database/20260415_00_schema_baseline.sql`
  - `infrastructure/database/2026*.sql`（baseline 之后增量）
  - `infrastructure/database/archive/pre-20260415/`
  - `services/api/Dockerfile`、`services/web/Dockerfile`、`services/bot/Dockerfile`
  - `services/api/internal/db/db.go`
  - `services/api/cmd/server/main.go`
  - `services/api/internal/models/*.go`
- 当前行为：
  - `db.go` 启动期 `db.AutoMigrate(...)`（受 `AUTO_MIGRATE` 控制）
  - compose 默认 `AUTO_MIGRATE=true` / `POSTGRES_PASSWORD=password` / `5432:5432`
  - PG initdb.d 挂 `../database`（含 README / archive）
  - `users.email`、`users.username` 大小写敏感唯一索引
  - 多对重复索引
- 现有限制：
  - 线上 `AUTO_MIGRATE=false`
  - SQL migration 必须放 `infrastructure/database/`
  - 所有 migration 必须幂等

## 方案设计

### 1. 用户可见行为

- 部署：`docker compose up` 不再包含明文默认凭证；强制 env 注入；不对外暴露 5432
- 首次启动：默认管理员临时口令打到日志，要求首次登录强制改密
- API：启动路径不再调用 GORM `AutoMigrate`；空库初始化必须先执行顶层 SQL
- 用户：第二个空邮箱注册不再 500
- 删除用户 / 兑换码 / plan group：~~相关数据按 FK 策略级联清理或显式 SET NULL~~【作废 2026-04-26】不引入 FK；级联清理由 services 层显式 sweep 负责
- DB 备份：运维参考 runbook 完成定期备份

### 2. 数据与模型

#### Migration 命名（baseline 之后顶层增量）

| 文件 | 用途 |
|---|---|
| `20260425_01_schema_alignment.sql` | 清重复索引、删死字段、收口 partial 唯一、对齐模型 tag |
| ~~`20260425_02_foreign_keys.sql`~~ | ~~补 FK + 级联策略~~【作废 2026-04-26】项目级规则不引入 FK |
| `20260425_03_users_email_username_partial_unique.sql` | `lower(email)` / `lower(username)` partial unique |
| `20260425_04_airdate_to_date.sql` | `tv_calendar_items.airDate` / `media_gaps.airDate` 改 `date` |
| `20260425_05_bigtable_cleanup_indexes.sql` | `tmdb_cache`、`email_verifications`、`telegram_bind_codes`、`device_actions` 上 `expiresAt / createdAt` 索引 |

baseline `20260415_00_schema_baseline.sql` 同步精简为最终状态；旧 baseline 移到 `archive/pre-20260425/` 历史保留。

#### 关键模型字段调整

- `User.Email`：`string` → `*string` + `gorm:"column:email"`（业务空值用 `nil`）
- `User.PlanGroup`：保持 `*string`，无变化
- 新增表 `failed_emby_provisions`（与 access-auth 计划共用，本计划仅声明放入 baseline 后增量）
- 新增表 `stripe_webhook_events`、`failed_emby_unbans`（与 billing-redemption 计划共用）
- 新增表 `media_gap_scans`、`bot_pending_reject_requests`（与 media-subscription / bot-telegram 计划共用）

> 注：上述跨计划的表 / 索引由各计划自行声明 SQL migration；本计划仅负责 baseline 同步与跨计划顺序协调。

### 3. 接口与边界

本计划不新增 API / Internal API / webhook，仅调整：

- compose env 入参契约：必须强制传入 `POSTGRES_USER` / `POSTGRES_PASSWORD` / `JWT_SECRET` / `INTERNAL_API_SECRET`
- `db.go` 启动期 `godotenv.Load` 仅在 `EMBER_DOTENV=path` 或非容器环境（无 `/.dockerenv`）调用
- Dockerfile 构建时锁基础镜像 tag（含小版本）
- PG initdb.d 挂载边界：仅挂 `infrastructure/docker/initdb/` 目录

### 4. 关键流程

#### 4.1 AUTO_MIGRATE 收敛

1. `docker-compose.yml` 改 `AUTO_MIGRATE=${AUTO_MIGRATE:-false}`
2. README 新增"`AUTO_MIGRATE=true` 仅本地空库可临时打开"说明
3. `cmd/server/main.go` 把 `AutoMigrate` 调用挪到独立子命令 `cmd/migrate/main.go`，启动路径完全不再调用
4. CI 添加 lint：禁止在 `cmd/server` 引用 `AutoMigrate`

#### 4.2 PG initdb.d 隔离

1. 新建 `infrastructure/docker/initdb/`
2. 顶层 SQL 通过 symlink 或 build-time 拷贝到该目录：
   - `20260415_00_schema_baseline.sql`
   - `20260425_*.sql`
3. compose 改 `volumes: - ../docker/initdb:/docker-entrypoint-initdb.d:ro`
4. README / archive 不再挂载到 PG 容器

#### 4.3 默认管理员 seed 收紧

1. 启动期检查 `ADMIN_PASSWORD`：
   - 已设置：仅在 `users` 表无 admin 时使用
   - 未设置：随机生成 16 位 base64 临时口令，写入 `settings(admin_initial_password)` + 日志 WARN
2. admin 首次登录强制改密：`users.passwordResetRequired=true`，登录后跳转改密页
3. `ADMIN_PASSWORD` 不再在 compose 默认填值

#### 4.4 compose 凭证 / 端口收紧

1. `POSTGRES_USER=${POSTGRES_USER:?POSTGRES_USER required}`
2. `POSTGRES_PASSWORD=${POSTGRES_PASSWORD:?POSTGRES_PASSWORD required}`
3. `ports: - "127.0.0.1:5432:5432"`（默认仅本机），README 说明"生产请走容器内网，不要暴露 5432"
4. `JWT_SECRET=${JWT_SECRET:?}` / `INTERNAL_API_SECRET=${INTERNAL_API_SECRET:?}`

#### 4.5 schema_alignment.sql

1. DROP 重复索引：`idx_payments_stripe_session`、`idx_media_quality_caches_library_id`、`idx_tmdb_cache_cache_key`、`uk_tv_calendar_episode`、`idx_tv_calendar_sources_tmdb_id`、`uk_tv_calendar_subscription`、`idx_client_blacklists_normalized_client_name` 等
2. DROP `idx_users_invite_code`、ALTER TABLE users DROP COLUMN inviteCode
3. DROP `uk_subscription_media`（baseline 已删的话本步幂等）
4. ALTER `users.telegramId` 唯一索引为 partial（`WHERE telegramId IS NOT NULL`）
5. 模型层 `User.TelegramID` 加 `gorm:"-"` 阻止 AutoMigrate 干扰

#### 4.6 foreign_keys.sql【作废 2026-04-26】

> 项目级规则不引入数据库 FK / 级联策略。本节内容保留作为决策溯源，**不再纳入实施**。
> 相关一致性问题（删用户后从表留孤儿）由 services 层显式 sweep 解决；如发现新场景需要补救，方向是"补全应用层 DELETE 路径 + 必要时新增 cron 兜底清理"，**不要补 FK**。

~~1. 补 FK：~~
   - ~~`redemptions.userId` → `users.id` ON DELETE CASCADE~~
   - ~~`redemption_codes.templateUserId` → `users.id` ON DELETE SET NULL~~
   - ~~`subscriptions.userId` → `users.id` ON DELETE CASCADE~~
   - ~~`payments.userId` → `users.id` ON DELETE CASCADE~~
   - ~~`payments.planId` → `plans.id` ON DELETE RESTRICT~~
   - ~~`device_actions.userId` → `users.id` ON DELETE SET NULL~~
   - ~~`tv_calendar_subscriptions.userId` → `users.id` ON DELETE CASCADE~~
   - ~~`users.planGroup` → `plan_groups.key` ON DELETE SET NULL~~
   - ~~其余按需补~~
~~2. 在补 FK 前先 `SELECT ... NOT IN (...)` 检测孤儿数据 → 输出报告；运维需先清理才能加 FK~~

#### 4.7 partial unique on lower(email/username)

1. `CREATE UNIQUE INDEX uq_users_email_lower ON users (lower(email)) WHERE email IS NOT NULL AND email <> ''`
2. 同理 `uq_users_username_lower`
3. migration 前先 `SELECT lower(email), count(*) FROM users GROUP BY 1 HAVING count(*) > 1` 输出冲突清单；运维合并后才允许加唯一索引

#### 4.8 airDate 改 date 类型

1. `ALTER TABLE tv_calendar_items ALTER COLUMN airDate TYPE date USING (airDate AT TIME ZONE 'UTC')::date`
2. 同理 `media_gaps.airDate`
3. 模型字段类型同步改为 `time.Time`（按 date 序列化）

#### 4.9 大表清理调度

1. cron 每日：
   - `DELETE FROM email_verifications WHERE "expiresAt" < now() - interval '1 day'`
   - `DELETE FROM telegram_bind_codes WHERE "expiresAt" < now() - interval '1 day'`
   - `DELETE FROM device_actions WHERE "createdAt" < now() - interval '90 days'`
   - `DELETE FROM tmdb_cache WHERE "expiresAt" < now() - interval '7 days'`（与追剧日历计划协同）
2. 旧 batchId 清理：`DELETE FROM playback_rankings WHERE "createdAt" < now() - interval '90 days'`
3. metric 输出每次删除条数

#### 4.10 payments.expiresAt 推进

1. cron 每 5 分钟：
   - `UPDATE payments SET status='expired', updatedAt=now() WHERE status='pending' AND "expiresAt" < now()`
2. 状态变更触发 metric

#### 4.11 连接池调整

1. `db.go`：MaxOpen=30、MaxIdle=15、MaxLifetime=1h、MaxIdleTime=10min
2. compose 给 PG 加 `command: ["postgres", "-c", "max_connections=200"]`
3. README 说明"生产至少 max_connections=200，单 API 副本 MaxOpen 建议 ≤ 30"

#### 4.12 godotenv 收敛

1. `db.go` 改：仅当 `EMBER_DOTENV=path` 设置时调用 `godotenv.Load(path)`
2. 容器内不调用任何相对路径加载

#### 4.13 Dockerfile 锁版本 + 非 root

1. API：`FROM golang:1.23.6-alpine3.20 AS builder` / `FROM alpine:3.20 AS runtime`
2. Web：`FROM node:22.13-alpine AS builder` / `FROM nginx:1.27.4-alpine AS runtime` + 切非 root（`nginx` 用户）
3. Bot：`FROM python:3.11.11-slim-bookworm` + 非 root 用户
4. 各服务新增 `.dockerignore`：排除 `.git`、`docs`、`node_modules`、`__pycache__`、`*.log`、`*.env*`

#### 4.14 compose 清理

1. 删 `version: '3.8'`
2. 各服务加 `init: true` + `stop_grace_period: 30s`
3. `bot` 容器：`depends_on: ember-api: { condition: service_healthy }`
4. `ember-api` healthcheck 显式声明（即使 Dockerfile 已写也要 compose 显式覆盖）

#### 4.15 备份 runbook

1. 新增 `docs/runbooks/database-backup.md`
2. 内容包含：
   - `pg_dump` 命令
   - 加密备份建议
   - 备份周期（建议每日 + 保留 7 天）
   - 恢复演练步骤

### 5. 失败路径与边界条件

- **运维忘记设 env 强制变量**：compose 启动失败 + 提示具体 env 名
- **首启 admin 临时口令落到日志**：明确文档说明"用完即改密"；不在 settings 表持久化明文（仅记录 hash 化版本？或落到 settings 后强制改密时清空）
- **partial unique on lower(email) 时存在冲突**：migration 主动失败并输出冲突报告
- ~~**FK 补充时存在孤儿数据**：migration 主动失败并输出孤儿清单~~【作废 2026-04-26】项目级规则不引入 FK
- **airDate 改 date 时存在非 UTC 00:00:00 数据**：USING 表达式自动归一
- **大表清理 cron 误删**：保留窗口足够长；metric 监控异常删除量
- **连接池 MaxOpen 调小后高并发短时不可用**：metric 监控连接等待
- **Dockerfile 锁版本后镜像无法获取**：CI 提前 mirror 到内网
- **`AUTO_MIGRATE=true` 被运维误开**：仅本地空库可用；线上 CI/CD 拒绝该值
- **PG initdb.d 切到子目录后老部署需要手动迁移**：README 提供升级步骤

## 影响范围

- API：
  - 修改：`db/db.go`、`cmd/server/main.go`
  - 新增：`cmd/migrate/main.go`（独立子命令）
- Web：
  - Dockerfile + .dockerignore
- Bot：
  - Dockerfile + .dockerignore
- 配置 / 部署：
  - `infrastructure/docker/docker-compose.yml`：默认值 / 端口 / depends_on / healthcheck / init
  - `infrastructure/docker/initdb/`：新目录
  - `infrastructure/database/20260425_01..05_*.sql`：5 份增量 migration（其中 `_02_foreign_keys.sql` 已于 2026-04-26 作废，实际落地为 4 份；编号占位保留）
  - baseline `20260415_00_schema_baseline.sql`：精简同步
  - `infrastructure/database/archive/pre-20260425/`：旧 baseline 归档
- 文档：
  - `docs/system-architecture.md` §13 / §11 改写
  - `infrastructure/database/README.md` 改写"目录边界 / 升级步骤 / archive 不参与初始化"
  - `docs/runbooks/database-backup.md`（新增）
  - `docs/runbooks/deployment.md` / `deployment-environment.md` / `release-process.md` 同步

## 验证方式

### 编译 / 测试

- `cd services/api && go build ./...`
- `cd services/api && go test ./internal/db/...`
- `docker compose -f infrastructure/docker/docker-compose.yml config`（语法校验）
- `docker compose ... up` 在测试主机空盘验证首启
- `psql -c "\d+ users"` / `\d+ payments` / `\d+ subscriptions` 等核对

### 手工验证

#### AUTO_MIGRATE 收敛
- 默认 compose up：API 启动日志无 AutoMigrate；故意删一列 → 启动正常但业务报错（证明依赖 SQL）
- `cmd/migrate` 子命令可在本地空库手动跑

#### initdb.d 隔离
- 在 `infrastructure/database/` 加 `bad.sh` → 首启不执行
- 在 `infrastructure/docker/initdb/` 加 `extra.sql` → 首启执行

#### 默认管理员 seed
- 不设 `ADMIN_PASSWORD` 启动：日志输出"临时口令: xxxx"
- admin 首次登录：强制改密页

#### compose 凭证
- 不设 `POSTGRES_PASSWORD`：compose up 失败 + 提示
- 设 `POSTGRES_PASSWORD=secret`：成功
- `netstat -plnt | grep 5432`：仅 127.0.0.1 listen

#### schema_alignment
- baseline 重灌 + 顶层增量：无重复索引；`\d+ users` 无 `inviteCode`
- AutoMigrate（如启用）不重建 partial 索引

#### foreign_keys【作废 2026-04-26】
- ~~删除 admin 用户：相关 redemptions/subscriptions/payments 按策略级联清理~~
- ~~删除 plan_group：跟随 user 的 planGroup 置空~~

#### partial unique on lower(email)
- 注册 `Alice@example.com` 后再注册 `alice@example.com`：第二次失败

#### airDate 类型
- 跨 DST 写入：DB 存 date，无时区漂移

#### 大表清理
- cron 触发：过期 email_verifications / telegram_bind_codes 被删；device_actions 90 天前被删

#### payments.expiresAt
- 创建 pending 31 分钟后 cron 触发：status=expired

#### 连接池
- 高并发 / 监控连接数：MaxOpen=30 不打满 PG 200

#### godotenv
- 容器启动日志：无 `.env load failed` warn
- 本地开发设 `EMBER_DOTENV=.env.local`：成功加载

#### Dockerfile 锁版本
- `docker images`：基础镜像 tag 显式版本
- 各服务以非 root 运行（`docker exec ... id`）

#### compose 清理
- `docker compose config`：无 `version` warning
- `docker compose up`：bot 在 ember-api healthy 之后启动

#### 备份 runbook
- 按 runbook 执行 `pg_dump` + 恢复测试通过

### 修复后验证清单

- [ ] `go build ./...` 与 `go test ./internal/db/...` 全绿
- [ ] 5 份 SQL migration 在临时库重灌通过
- [ ] baseline 精简后空库初始化路径通过
- [ ] ~~FK 补充前的孤儿数据清理报告归档~~【作废 2026-04-26】项目级规则不引入 FK
- [ ] partial unique 冲突清单归档
- [ ] compose `config` 无 warning
- [ ] PG 不对外暴露 5432
- [ ] Dockerfile 各基础镜像锁定版本，CI 镜像构建可复现
- [ ] `docs/runbooks/database-backup.md` 完成并经过一次恢复演练
- [ ] 关键日志含 `migration / cron / actor / requestId`

### 二次暴露检查清单

- [ ] sweep `compose` 文件中所有"硬编码默认凭证"位置（POSTGRES_*、ADMIN_*、JWT_*、SMTP_*）
- [ ] sweep `compose` 文件中所有"对外暴露端口"位置，确认仅必要项发布到 0.0.0.0
- [ ] sweep `models/*.go` 所有 `uniqueIndex` tag，确认与 SQL partial 一致
- [ ] sweep 所有 `gorm:"-"` 是否覆盖了"可能被 AutoMigrate 误重建"的字段
- [ ] sweep 所有 cron 任务，确认每张大表都有清理 owner
- [ ] sweep 所有 Dockerfile，确认锁版本 + .dockerignore + 非 root
- [ ] sweep `db.go` `godotenv.Load`、`init()` 等启动期副作用
- [ ] sweep 所有迁移文件命名是否符合 `YYYYMMDD_NN_*.sql` + 幂等
- [ ] 复核 archive/pre-20260415 与 archive/pre-20260425 的边界 / 归档 README

## 落地后文档处理

- 已提炼：
  - `docs/system-architecture.md` §13：`AUTO_MIGRATE=false`、`initdb/`、固定镜像、`cmd/migrate`、`VerifySchema`
  - `infrastructure/database/README.md`：顶层 migration 入口、`initdb/` 同步规则、archive 边界
  - `docs/runbooks/deployment-environment.md`：启动期不再调用 `AutoMigrate`、`VerifySchema` 校验与 `cmd/migrate` 用法
  - `docs/runbooks/database-backup.md` / `docs/runbooks/release-process.md`：备份恢复与镜像发布入口
- 归档前仍需补的收尾：
  - `baseline` 精简归档、冲突清单归档、archive 目录边界复核
  - `playback_rankings` snake_case 豁免与其余交叉引用继续核对
  - `docs/plan/README` / `docs/proposals/README` / `docs/proposals/plan-inventory.md` 状态保持一致
- 本方案完成归档准备后，移入 `docs/archive/plan/architecture/`
- 剩余更细颗粒度的部署治理转交后续 runbook / proposal，不再继续堆在本实施稿

## 附录：问题清单与本方案条目映射

| review 编号 | 问题 | 本方案条目 |
|---|---|---|
| P0-1 (DB) | AUTO_MIGRATE 默认 true | §4.1 |
| P0-2 (DB) | initdb.d 整目录挂载 | §4.2 |
| P0-3 (DB) | users.email 空串重复冲突 | §4.7 |
| P0-4 (DB) | 默认管理员明文 env | §4.3 |
| P0-5 (DB) | Postgres 默认弱凭证 + 端口暴露 | §4.4 |
| P1-1 (DB) | 模型与 baseline 多处对不齐 | §4.5 |
| P1-2 (DB) | baseline 重复索引 | §4.5 |
| P1-3 (DB) | uk_subscription_media 与 partial 冲突 | §4.5 |
| P1-4 (DB) | telegramId uniqueIndex 与 SQL partial 不一致 | §4.5 |
| P1-5 (DB) | README/archive 与 PG init 行为脱钩 | §4.2 |
| P1-6 (DB) | db.go 启动期 godotenv | §4.12 |
| P1-7 (DB) | 缺 FK + 级联 | ~~§4.6~~【作废 2026-04-26】项目级规则不引入 FK，由 services 层显式 sweep 解决 |
| P1-8 (DB) | media_gaps 模型 tag 冗余 | §4.5 |
| P1-9 (DB) | bot depends_on 无 healthy | §4.14 |
| P2-1 (DB) | playback_rankings snake_case | 文档豁免 |
| P2-2 (DB) | airDate 跨 DST 漂移 | §4.8 |
| P2-3 (DB) | 缺 expiresAt/createdAt 清理索引 | §4.9 |
| P2-4 (DB) | inviteCode 死字段 | §4.5 |
| P2-5 (DB) | 连接池失衡 | §4.11 |
| P2-6 (DB) | PG 大版本横跨 | 文档同步 |
| P2-7 (DB) | compose version 字段过时 | §4.14 |
| P2-8 (DB) | subscriptions.note 双标 | 二次暴露清单 |
| P2-9 (DB) | payments.expiresAt 无 cron | §4.10 |
| P2-10 (DB) | media_gaps.airDate 类型 | §4.8 |
| P3-1~P3-9 (DB) | 日志 / Dockerfile / 备份 / archive 边界 | §4.13 / §4.15 / 二次暴露清单 |
