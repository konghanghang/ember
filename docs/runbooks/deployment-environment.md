# 部署环境与配置

这份文档只负责解释部署时必须准备什么，不重复写启动命令。

## Compose 事实

当前 [`docker-compose.yml`](../../infrastructure/docker/docker-compose.yml) 默认启动三个服务：

- `postgres`
- `ember-api`
- `ember-web`

`ember-bot` 通过 `profiles: ["bot"]` 控制，`ember-gateway` 通过 `profiles: ["gateway"]` 控制，二者默认不启动。启用 Gateway 时使用包含统一入口的新镜像并执行 `docker compose --profile gateway up -d`。

`ember-gateway` 不对应第四个镜像：启用 profile 后，`ember-api` 与 `ember-gateway` 复用同一 `EMBER_API_IMAGE` 中的单个 `ember` 二进制，分别选择默认 `api` 与显式 `gateway` 子命令。profile 设计避免当前默认旧 Tag 因不认识新子命令而破坏既有部署。

## 必填变量

### 基础设施与 API 启动期变量

下列变量为 compose 解析期强制（`${X:?}`）：缺失任一项 `docker compose up` 直接拒绝启动：

| 变量 | 作用 |
|------|------|
| `POSTGRES_USER` | PostgreSQL 用户 |
| `POSTGRES_PASSWORD` | PostgreSQL 密码（禁止使用默认值）|
| `JWT_SECRET` | JWT 签名密钥（≥32 字符）|
| `CONFIG_ENCRYPTION_KEY` | 设置中心敏感值加密主密钥（已有环境保持原值；新部署推荐随机 ≥32 字节）|
| `INTERNAL_API_SECRET` | API 与 Bot 的内部调用共享密钥（≥32 字符，禁止使用示例占位值） |

`DATABASE_URL` 缺省时由 compose 按 `POSTGRES_USER/PASSWORD/DB` 自动拼接到内置 postgres；指向独立 DB 时在 `.env` 显式提供完整 DSN 即可覆盖。

可选但推荐：

| 变量 | 作用 |
|------|------|
| `POSTGRES_DB` | 默认 `ember` |
| `ADMIN_USERNAME` | 默认 `admin` |
| `ADMIN_PASSWORD` | 首次启动管理员初始化密码（落地后立即在控制台改密）|
| `EMBY_WEBHOOK_TOKEN` | Emby Webhook 验签口令 |
| `PLAYBACK_GATEWAY_PORT` | Gateway 宿主机回环映射端口，默认 `8081` |

### Bot 与 Webhook

`ember-bot` 通过 `profiles: ["bot"]` 控制默认不启动。这组变量在 compose 解析期不强制（profile 关闭时不会触发插值校验），Bot 进程启动时若 `TELEGRAM_BOT_TOKEN` 缺失会自行退出：

| 变量 | 作用 |
|------|------|
| `TELEGRAM_BOT_TOKEN` | Telegram Bot Token（启用 Bot 必填）|
| `TELEGRAM_WEBHOOK_SECRET` | Bot Webhook 验签密钥（webhook 模式必填）|
| `WEBHOOK_URL` | Telegram 对外回调地址（webhook 模式必填）|

## 常用可选变量

| 变量 | 默认值 / 说明 |
|------|---------------|
| `ADMIN_USERNAME` | 默认 `admin` |
| `TELEGRAM_ADMIN_CHAT_ID` | 管理员通知目标 |
| `TELEGRAM_GROUP_CHAT_ID` | 群推送目标，未填时回退到管理员 |
| `TMDB_API_KEY` | TMDB 搜索、追剧日历依赖 |
| `TURNSTILE_SECRET_KEY` | 登录 Turnstile 服务端校验密钥 |
| `MOVIEPILOT_URL` / `MOVIEPILOT_API_KEY` | 求片审批同步到 MoviePilot 时需要 |
| `CRON_ENABLED` | API 内置 Cron 开关 |
| `RANKING_CRON_ENABLED` | 播放排行 Cron 开关 |
| `RANKING_DAILY_SCHEDULE` / `RANKING_WEEKLY_SCHEDULE` | 排行定时表达式 |
| `TV_CALENDAR_SYNC_SCHEDULE` | 追剧日历同步表达式 |

更完整的配置边界见 [配置参考](../reference/configuration-reference.md)。

## 首次启动后必须补齐的运行期配置

下列配置不再依赖 compose 直接注入 `ember-api`，而是由设置中心数据库托管：

- `EMBY_URL`
- `EMBY_API_KEY`
- `TMDB_API_KEY`
- `MOVIEPILOT_URL`
- `MOVIEPILOT_API_KEY`
- `BOT_NOTIFY_URL`
- `CRON_*`
- `RANKING_*`
- `TV_CALENDAR_*`

这意味着：

- 新环境启动后，API 可以先起来；
- 但媒体、MoviePilot、Bot fire-and-forget、追剧日历和调度能力，要在设置中心补齐配置后才会真正可用；
- 不要再把这些项当成 compose 自动注入的前提变量。

Gateway 必须保持两个地址边界：`EMBY_URL` 是 API/Gateway 容器访问原始 Emby 的内部地址；`NEXT_PUBLIC_EMBY_URL` 是用户和播放器看到的 Gateway 公网 HTTPS 地址。二者指向同一公网 Gateway 会形成代理回环，原始 Emby 继续公开则会形成安全旁路。

API 固定使用容器内 `8080`，Gateway 固定监听容器内 `8081`；直接运行 `ember gateway` 时同样监听 `:8081`。`.env` 的 `PLAYBACK_GATEWAY_PORT` 只改变映射到 Gateway `8081` 的宿主机 `127.0.0.1` 端口，不进入 Go 配置。必须先在设置中心填写 `EMBY_URL/EMBY_API_KEY` 再启用 profile；配置错误时 Gateway 按启动合同 fail-fast 并由 Docker 重启。

API、Gateway 和 Bot 共用项目级 `LOG_LEVEL`，只接受 `info/debug` 并默认 `info`。Info 保留关键业务事件、失败和视频最终决策；Debug 额外输出安全请求摘要、正常 access log、参数化 SQL、Gateway 缓存/载体诊断和 Bot 应用调试信息。非法值回退 `info` 并记录一次固定警告；修改后重启目标服务生效。该变量不控制浏览器 console、Nginx、Docker logging driver 或 PostgreSQL 自身日志。

本地直接运行 `ember api/gateway` 时，entrypoint 会先加载 `EMBER_DOTENV` 指定文件；未指定时检查当前目录 `.env`、再检查 `services/api/.env`，之后才初始化日志。因此若日志启动行仍显示 `logLevel=info`，先确认 Debug 写入的是上述实际命中的文件，或直接在进程环境中导出 `LOG_LEVEL=debug`。

API 与 Gateway 都把日志写到 stdout 和按日文件，但文件前缀固定隔离：API 使用 `logs/api-YYYY-MM-DD.log`，Gateway 使用 `logs/gateway-YYYY-MM-DD.log`。Compose 分别挂载 `api_logs` 与 `gateway_logs`；直接在同一工作目录运行两个二进制进程时也不会写入同一个文件。Bot 继续使用自己的 stdout 与按日轮转文件。

## `.env.example` 的已知缺口

[`infrastructure/docker/.env.example`](../../infrastructure/docker/.env.example) 已覆盖启动期所有强制 env（含 `POSTGRES_USER` / `POSTGRES_PASSWORD` / `JWT_SECRET` / `CONFIG_ENCRYPTION_KEY` / `INTERNAL_API_SECRET` / `ADMIN_*`），但仍不是"所有运行期能力都能开箱即用"的完整模板。

如果你希望首次启动后直接使用媒体相关能力，还需要额外准备一份设置中心初始化方案或在后台手动补齐 `EMBY_URL` / `EMBY_API_KEY` / `TMDB_API_KEY` / `MOVIEPILOT_*` / `BOT_NOTIFY_URL` 等运行期配置。

## 数据库迁移策略

### 空数据库首次部署

不再依赖 PG `initdb.d`：`ember-api` 启动期 Migrate 阶段直接接管空库初始化：

- 探测到业务核心表不存在 + `schema_migrations` 为空 → 进入"新空库"分支
- 按字典序 forward-only 跑全部 `infrastructure/database/` 顶层 SQL（当前为 v1.6.0 截点 fresh-install baseline `00000000_baseline_20260605.sql`）
- `archive/` 不参与运行时链路

### 已有数据库升级

线上以 `AUTO_MIGRATE=false` 运行，不依赖 GORM 自动迁移。当前直接升级支持起点是 `2026-06-05` / v1.6.0 截点；从该截点之后的已支持版本升级，部署者只需：

```bash
docker compose pull
docker compose up -d
```

`ember-api` 启动期 Migrate 阶段会自动应用所有未应用的顶层 SQL（forward-only），日志带 `[Migrate]` 前缀。流程详见 [`infrastructure/database/README.md`](../../infrastructure/database/README.md) 的「自动迁移与 schema_migrations」章节。

不支持的直升场景：

- 旧于 `2026-06-05` 截点的数据库，如果没有执行过已归档到 `infrastructure/database/archive/pre-20260605/` 的增量，不承诺直接跳升到当前版本
- 启动期 Migrate 只扫描 `infrastructure/database/` 顶层 SQL，不会自动执行 `archive/`
- 不要把 `archive/` 整体搬回顶层补救；多份 baseline 会 fail-fast，历史回填脚本也可能不适合当前运行链路

这类旧库需要先人工对齐到 v1.6.0 截点 schema，或先升级到包含对应增量的中间版本，再进入当前升级路径。

启动期序列为 `InitDB → Migrate → VerifySchema → Bootstrap → Start`：

- **Migrate**：按 `schema_migrations` 记账表 + `pg_advisory_lock` 串行执行未应用 SQL
- **VerifySchema** 作为兜底，三层校验任意一层缺失立即 fail-fast：
  1. **表存在**：覆盖 baseline 与"新建表"型 migration
  2. **关键列**：覆盖"在已有表上 ADD COLUMN"型 migration
  3. **关键索引**：覆盖"加 partial unique / 复合唯一"型 migration

如启动失败：`docker logs ember-api --tail` 第一时间看到失败 SQL 文件名 → 按 forward-only 原则**追加一条新 SQL** 抵消错误效果，重新构建并推送镜像 → `docker compose pull && up -d` 恢复。**镜像是只读的，不允许 `docker exec` 进容器删 SQL 文件、也不允许修改原文件**（修改即破坏 checksum）。

### 上线前脏数据自查（v1.3.1 → v1.4.0 历史升级期）

> 以下表格记录 v1.3.1 → v1.4.0 升级期间需要预检的脏数据；v1.4.0 已上线、相关 migration 已归档到 `infrastructure/database/archive/pre-20260502/`。新装库与已升级环境无需关注，保留作为历史参考。未来新增增量时按相同结构在表内追加。

| 来源 migration | 自查 SQL | 期望结果 |
|---|---|---|
| `20260425_02_telegram_bind_codes_user_unique` | （migration 内置 CTE 自动去重，每个 userId 只保留 createdAt 最新一条；绑定码本就是 5 分钟短期凭据，无业务影响） | — |
| `20260426_01_users_lower_unique_indexes` | `SELECT lower(username), count(*) FROM users GROUP BY 1 HAVING count(*) > 1; SELECT lower(email), count(*) FROM users WHERE email IS NOT NULL AND email <> '' GROUP BY 1 HAVING count(*) > 1;` | 0 行；非 0 行需人工判定合并 / 重命名后再重跑 migration（migration 内置 `RAISE EXCEPTION` 预检，存在重复时会停止并附排查 SQL） |
| `20260426_04_payments_checkout_constraints` | `SELECT "userId", "planId", count(*) FROM payments WHERE status='pending' GROUP BY 1,2 HAVING count(*) > 1;` | 0 行；非 0 行需先把多余 pending 收口为 expired 再重跑 migration（migration 内置 `RAISE EXCEPTION` 预检并附收口 SQL） |

### 批次 2 新增运行期表（v1.4.0 期间新建）

下面三张表是 v1.4.0 期间新建的运行期表（来源 migration 已归档），运行时由业务 / cron 按需写入：

| 表 | 来源 migration | 用途 |
|---|---|---|
| `failed_emby_async_ops` | `20260426_02_failed_emby_async_ops` | 支付履约 / 兑换码 / 注册回滚等链路在事务外调 Emby 失败时的补偿队列 |
| `stripe_webhook_events` | `20260426_03_stripe_webhook_events` | Stripe webhook event.id 级别去重表 |
| `media_gap_scans` | `20260426_07_media_gap_scans` | 缺集扫描的持久化执行记录（配合 PG advisory lock 做跨副本互斥） |

### 批次 2 新增 cron 任务

API 启动期注册的常驻 cron（`CRON_ENABLED=true` 时启用）新增两项：

| Cron 名 | 调度 | 行为 | 失败处理 |
|---|---|---|---|
| `emby-async-compensation` | `@every 10m` | 拉取 `failed_emby_async_ops` 中 `nextAttemptAt <= now()` 的待补偿操作（每轮上限 50 条），按 origin/action 路由到 emby service；成功删除该行，失败指数退避（30s/2m/10m/1h/6h/24h），retries > 6 写 ERROR 日志告警 | 进程内 panic 由 cron runner 捕获；DB 异常仅记录错误日志，不影响其他 cron |
| `media-gap-scans-cleanup` | `@weekly` | 删除 `media_gap_scans` 表中 7 天之前的 `success / failed` 记录（`running` 不清理，便于排查孤儿） | 同上 |

升级期建议：在支持窗口内直接 `docker compose pull && up -d`，启动期 Migrate 阶段配合 advisory lock 保证串行；如失败立即 restart loop，不会出现"半成品 schema + 跑起来的 API"。

### 本地空库快速搭建

仅本地开发可用：

```bash
cd services/api
go run ./cmd/ember api
```

`ember api` 启动期 Migrate 阶段会探测到业务核心表不存在 + `schema_migrations` 为空 → 进入"新空库"分支按字典序应用 `infrastructure/database/` 顶层与生产同源的 SQL（即 baseline + 后续增量），跑 `VerifySchema` 自检，再写入默认管理员 / 默认设置 / 默认套餐分组。无参数同样默认 API。**严禁在生产使用本地 Go 启动方式**——生产路径走 docker compose `pull + up -d`。

如果当前数据库还停留在 `v1.2.13` 对应阶段，升级到当前版本前需要按顺序执行这两份 SQL；已经执行过它们的环境无需重复处理。

## 管理员初始化

API 启动时会检查是否已有 `role=admin` 的用户：

- 已存在：admin 初始化自动跳过
- 不存在：读取 `ADMIN_USERNAME` 与 `ADMIN_PASSWORD`
  - `ADMIN_PASSWORD` 已设置：用该密码创建首个管理员
  - `ADMIN_PASSWORD` 未设置：生成随机临时口令，写日志提示，并将该管理员标记为 `passwordResetRequired=true`

注意：

- `ADMIN_USERNAME` 在 compose 默认会回退为 `admin`
- `ADMIN_PASSWORD` 现在不是“不填就跳过初始化”，而是“不填则生成临时口令并要求首次登录立即改密”
- 该流程是幂等的，多次重启不会重复创建管理员

## 启用可选功能前的最低要求

### TMDB / 追剧日历 / 搜索

- `TMDB_API_KEY`
- Emby 可访问

### 登录 Turnstile 保护

- API 环境变量：
  - `TURNSTILE_SECRET_KEY`
- API 设置中心运行期配置：
  - `turnstile_login_enabled`
  - `turnstile_site_key`
  - `turnstile_expected_hostname`

说明：

- `turnstile_site_key` 是前端公开值，走设置中心即可。
- `TURNSTILE_SECRET_KEY` 是服务端校验密钥，必须保留在环境变量，不进入设置中心。

### Telegram Bot

- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_WEBHOOK_SECRET`
- `WEBHOOK_URL`
- `INTERNAL_API_SECRET`

本地联调见 [Cloudflared 本地联调](./cloudflared-local-testing.md)。

### MoviePilot

- `MOVIEPILOT_URL`
- `MOVIEPILOT_API_KEY`

未配置时，相关同步能力应视为关闭，而不是“自动降级成功”。

## 启动前自检

- `POSTGRES_USER` / `POSTGRES_PASSWORD` 已设置；显式提供 `DATABASE_URL` 时已指向可访问的 PostgreSQL
- 所有密钥都不是示例值
- 启用 `ember-bot` 时（`docker compose --profile bot`），Telegram 相关变量已补齐
- 如果是支持窗口内升级环境，无需任何手工 SQL：`docker compose pull && up -d` 后由 ember-api 启动期 Migrate 阶段自动应用未应用的顶层 SQL

## 相关文档

- [部署指南](./deployment.md)
- [部署排障](./deployment-troubleshooting.md)
- [配置参考](../reference/configuration-reference.md)
- [数据库迁移说明](../../infrastructure/database/README.md)
