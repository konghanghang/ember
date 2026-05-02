# 部署环境与配置

这份文档只负责解释部署时必须准备什么，不重复写启动命令。

## Compose 事实

当前 [`docker-compose.yml`](../../infrastructure/docker/docker-compose.yml) 默认会启动四个服务：

- `postgres`
- `ember-api`
- `ember-web`
- `ember-bot`

这意味着：

- 如果你不准备启用 Telegram Bot，不能假装相关配置不存在
- 要么补齐 Bot 所需变量
- 要么在部署前手动注释 `ember-bot` 服务

## 必填变量

### 基础设施与 API 启动期变量

下列变量为 compose 解析期强制（`${X:?}`）：缺失任一项 `docker compose up` 直接拒绝启动：

| 变量 | 作用 |
|------|------|
| `POSTGRES_USER` | PostgreSQL 用户 |
| `POSTGRES_PASSWORD` | PostgreSQL 密码（禁止使用默认值）|
| `DATABASE_URL` | PostgreSQL 连接串（与 `POSTGRES_USER/PASSWORD` 保持一致）|
| `JWT_SECRET` | JWT 签名密钥（≥32 字符）|
| `CONFIG_ENCRYPTION_KEY` | 设置中心敏感值加密主密钥（≥32 字符）|
| `INTERNAL_API_SECRET` | API 与 Bot 的内部调用共享密钥 |

可选但推荐：

| 变量 | 作用 |
|------|------|
| `POSTGRES_DB` | 默认 `ember` |
| `ADMIN_USERNAME` | 默认 `admin` |
| `ADMIN_PASSWORD` | 首次启动管理员初始化密码（落地后立即在控制台改密）|
| `EMBY_WEBHOOK_TOKEN` | Emby Webhook 验签口令 |

### Bot 与 Webhook

默认 compose 会启动 `ember-bot`。**这组变量在 compose 解析期不再强制**（强制会让仅启动 postgres + ember-api 的部署也被拒绝），但 Bot 进程启动时若 `TELEGRAM_BOT_TOKEN` 缺失会自行退出：

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

## `.env.example` 的已知缺口

[`infrastructure/docker/.env.example`](../../infrastructure/docker/.env.example) 已覆盖启动期所有强制 env（含 `POSTGRES_USER` / `POSTGRES_PASSWORD` / `DATABASE_URL` / `JWT_SECRET` / `CONFIG_ENCRYPTION_KEY` / `INTERNAL_API_SECRET` / `ADMIN_*`），但仍不是"所有运行期能力都能开箱即用"的完整模板。

如果你希望首次启动后直接使用媒体相关能力，还需要额外准备一份设置中心初始化方案或在后台手动补齐 `EMBY_URL` / `EMBY_API_KEY` / `TMDB_API_KEY` / `MOVIEPILOT_*` / `BOT_NOTIFY_URL` 等运行期配置。

## 数据库迁移策略

### 空数据库首次部署

`infrastructure/docker/docker-compose.yml` 把 `infrastructure/docker/initdb/` 子目录挂到 PostgreSQL 容器的 `/docker-entrypoint-initdb.d`：

- 仅当数据卷为空时由 PG 镜像自动执行一次
- 当前仅包含 v1.4.0 截点合并 baseline `20260502_00_schema_baseline.sql`（含旧 baseline 与全部历史增量）
- 不再挂载 `infrastructure/database/` 顶层目录，避免 README / archive / 临时 SQL 被误执行
- `infrastructure/database/` 仍是 SQL migration 真相目录，新增顶层 SQL 必须同步到 `docker/initdb/`

### 已有数据库升级

API 启动期已不再调用 `AutoMigrate`（任何 `AUTO_MIGRATE` env 都会被忽略），且启动会执行 `VerifySchema` 三层校验：

1. **表存在**：覆盖 baseline 与"新建表"型 migration
2. **关键列**：覆盖"在已有表上 ADD COLUMN"型 migration
3. **关键索引**：覆盖"加 partial unique / 复合唯一"型 migration

任意一层缺失立即 fail-fast，并在错误信息里标注是哪条 migration 引入的：

1. 阅读 [`infrastructure/database/README.md`](../../infrastructure/database/README.md)
2. 按顺序手动执行 baseline 之后新增的顶层 SQL
3. 同步把新增 SQL 复制到 `infrastructure/docker/initdb/`，并在 `services/api/internal/db/db.go` 的 `schemaFingerprintColumns` / `schemaFingerprintIndexes` 追加该 migration 的列/索引指纹
4. 启动或重启 API；如启动日志报"数据库缺少必要的表/列/索引"，回到第 2 步检查漏跑哪条 SQL

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

升级期建议：先停 API → 跑 migration → 启动新 API，避免老路径与新路径并存的竞态（特别是 `telegram_bind_codes` 在 GenerateBindCode 路径上的事务+DELETE 与 ON CONFLICT 切换）。

### 本地空库快速搭建

仅本地开发可用：

```bash
cd services/api
go run ./cmd/migrate
```

会按字典序应用 `infrastructure/database/` 顶层与生产同源的 SQL（即 baseline + 后续增量），跑 `VerifySchema` 自检，再写入默认管理员 / 默认设置 / 默认套餐分组。**严禁在生产使用**。

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

- `DATABASE_URL` 已指向可访问的 PostgreSQL
- 所有密钥都不是示例值
- 如果保留 `ember-bot` 服务，Telegram 相关变量已补齐
- 如果是升级环境，手动 SQL 已执行完毕

## 相关文档

- [部署指南](./deployment.md)
- [部署排障](./deployment-troubleshooting.md)
- [配置参考](../reference/configuration-reference.md)
- [数据库迁移说明](../../infrastructure/database/README.md)
