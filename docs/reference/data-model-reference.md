# Ember 数据模型参考

> 本文档承接 Ember 当前系统的完整数据模型字典、字段语义和关系说明。
> 数据库 schema 的 SQL 真相仍以 `infrastructure/database/` 顶层 migration 为准；本文件用于协作、排查和接口语义理解。

## 1. 核心说明

- 表名、列名、索引名统一使用 `snake_case`
- Go / GORM 字段继续使用 `CamelCase`，通过显式 `gorm:"column:..."` 映射到数据库列
- API / 前端 JSON 字段继续使用 `camelCase`
- 线上长期以 `AUTO_MIGRATE=false` 运行，不依赖 GORM 自动迁移

## 2. 数据模型

### 2.1 User

**表名**: `users` | **文件**: `models/user.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID 主键 |
| Username | string(50) | username | 大小写不敏感唯一；SQL 层由 `uq_users_username_lower` 维护 |
| Role | string(10) | role | `"admin"` 或 `"user"` |
| Password | string | password | bcrypt hash（JSON 隐藏） |
| Email | string(255) | email | 非空邮箱大小写不敏感唯一；SQL 层由 `uq_users_email_lower` 维护 |
| EmbyID | string(50) | embyId | Emby 用户 ID |
| EmbyDisabled | bool | embyDisabled | Emby 端 `Policy.IsDisabled` 同步缓存；只在有效 Policy 同步成功后更新 |
| EmbyAccessDisabled | bool | embyAccessDisabled | 管理员显式禁用 Emby 访问的业务意图；不影响 Ember Web 登录 |
| TelegramID | *int64 | telegramId | Telegram 绑定 ID（唯一，可空） |
| PlanGroup | *string | planGroup | 用户绑定的套餐分组 key；新增 / 编辑用户必须显式写入有效分组，历史空值仅作为兼容态按系统默认分组读取 |
| ExpiresAt | *time.Time | expiresAt | 到期时间（nil=永不过期）|
| IsActive | bool | isActive | 管理员手动开关 |
| CreatedAt | time.Time | createdAt | 自动 |
| UpdatedAt | time.Time | updatedAt | 自动 |

**方法**：
- `SetPassword(pwd)` / `CheckPassword(pwd)` — bcrypt
- `IsExpired()` — `ExpiresAt != nil && ExpiresAt < now`
- `IsAdmin()` — `Role == "admin"`

**设计要点**：
- `IsActive` 是 Ember Web 登录账号开关，不参与 Emby `Policy.IsDisabled` 计算
- Emby `Policy.IsDisabled` 由 `User.IsExpired() || EmbyAccessDisabled` 计算
- `EmbyDisabled` 是同步缓存，不是业务意图字段
- `User` 只承载 `users` 表真实列；`planGroupName`、`effectivePlanGroup` 等展示态字段只存在于 `services/user` 查询 DTO，不再混入持久化模型
- `Username` / `Email` 的唯一性以 `infrastructure/database/` SQL 为准；GORM tag 不表达 `lower(...)` 函数唯一索引，避免误导 AutoMigrate 语义

### 2.2 RedemptionCode

**表名**: `redemption_codes` | **文件**: `models/redemption_code.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| Code | string(20) | code | 唯一，16 字符 hex |
| MaxUses | int | maxUses | 最大使用次数（默认 1）|
| UsedCount | int | usedCount | 已使用次数（默认 0）|
| ExpiresAt | *time.Time | expiresAt | 码本身的过期时间 |
| DefaultDays | int | defaultDays | 每次兑换授予的天数（默认 30）|
| RegistrationPlanGroup | string(50) | registrationPlanGroup | 注册场景专用套餐分组 key（必填；仅注册时生效，续期忽略） |
| Notes | string(500) | notes | 备注（可选，用于记录用途或来源） |
| CreatedAt | time.Time | createdAt | 自动 |

**方法**：`IsValid()` — `UsedCount < MaxUses && (ExpiresAt == nil || ExpiresAt > now)`

**双重角色**：
- `registration_mode = "invite"` 时：注册门控（必须提供码才能注册）
- 已注册用户：续期工具（兑换码延长有效期）

### 2.3 Redemption（兑换历史）

**表名**: `redemptions` | **文件**: `models/redemption.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| UserID | string(25) | userId | 用户 ID（有索引）|
| Code | string(20) | code | 使用的兑换码 |
| Days | int | days | 兑换天数 |
| CreatedAt | time.Time | createdAt | 自动 |

### 2.4 Setting（系统配置）

**表名**: `settings` | **文件**: `models/setting.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| Key | string(100) | key | 主键 |
| Value | text | value | 值或密文 |
| IsEncrypted | bool | isEncrypted | 是否为加密存储 |
| UpdatedByUserID | *string(25) | updatedByUserId | 最后修改人（可空） |
| UpdatedAt | time.Time | updatedAt | 自动 |

**设计要点**：
- `settings` 已扩展为设置中心的运行期存储层
- 配置解析优先级固定为：数据库覆盖值 > 环境变量 > 代码默认值
- 敏感配置可加密落库，不通过 API 明文回显
- 配置定义显式声明“空值语义”：关闭功能 / 回退到上游配置 / 跟随外部服务默认行为，不再靠模糊文案猜
- 只读部署边界项同时声明“只读原因”和“缺失影响”，前端直接展示，不再让管理员自己猜为什么不能改

**当前已托管或接入统一解析的配置项**：
- 业务配置：`registration_mode`、`default_trial_days`、`notify_group_link`、`telegram_welcome_message_template`、`email_verification`、`registration_allowed_email_domains`、`stripe_allowed_payment_methods`
- 媒体集成：`EMBY_URL`、`EMBY_API_KEY`、`NEXT_PUBLIC_EMBY_URL`（历史键名，数据库配置项）、`TMDB_API_KEY`、`MOVIEPILOT_URL`、`MOVIEPILOT_API_KEY`
- 邮件服务：`SMTP_HOST`、`SMTP_PORT`、`SMTP_USERNAME`、`SMTP_PASSWORD`、`SMTP_FROM`、`EMAIL_CODE_EXPIRY_MINUTES`、`EMAIL_CODE_DAILY_LIMIT`、`EMAIL_CODE_IP_DAILY_LIMIT`
- 通知：`BOT_NOTIFY_URL`、`TELEGRAM_ADMIN_CHAT_ID`、`telegram_approval_admin_ids`、`TELEGRAM_GROUP_CHAT_ID`
- 只读展示：`DATABASE_URL`、`JWT_SECRET`、`INTERNAL_API_SECRET`、`ADMIN_USERNAME`、`ADMIN_PASSWORD`、`TELEGRAM_BOT_TOKEN`、`TELEGRAM_WEBHOOK_SECRET`、`WEBHOOK_URL`、`PORT` 等

### 2.5 Subscription（订阅求片）

**表名**: `subscriptions` | **文件**: `models/subscription.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| UserID | string(25) | userId | 用户 ID |
| Type | MediaType | type | `"MOVIE"` 或 `"TV"` |
| Name | string(255) | name | 媒体名称 |
| TmdbID | string | tmdbId | TMDB ID |
| Season | int | season | 季号，`0` 表示整剧 |
| PosterPath | *string(500) | posterPath | 海报 URL |
| Status | SubscriptionStatus | status | `PENDING`/`APPROVED`/`REJECTED`/`INGESTED` |
| Note | *string | note | 用户备注 |
| RetryFromID | *string(25) | retryFromId | 拒绝后重新发起时指向上一条 `REJECTED` 订阅，可空 |
| MpError | *string(500) | mpError | MoviePilot 同步错误 |
| RejectReason | *string | rejectReason | 管理员拒绝原因 |
| ReviewSource | *SubscriptionReviewSource | reviewSource | 审核通过来源：`MANUAL` / `AUTO_QUOTA`；待审核或已拒绝时为空 |
| ReviewedAt | *time.Time | reviewedAt | 审核时间（通过/拒绝） |
| IngestedAt | *time.Time | ingestedAt | 真实入库时间（由 Emby webhook 或管理员校验命中后收口写入） |
| CreatedAt | time.Time | createdAt | 自动 |
| UpdatedAt | time.Time | updatedAt | 自动 |

**状态流转**：
- 用户提交前必须满足：`embyId` 非空且 `embyDisabled=false`
- 用户创建后默认进入 `PENDING`；若命中所属 `PlanGroup.subscriptionAutoApproveDailyLimit` 的当日额度，则可直接进入 `APPROVED`
- 管理员审核通过后转 `APPROVED`，并记录 `reviewedAt`
- 管理员拒绝后转 `REJECTED`，必须写入 `rejectReason` 与 `reviewedAt`
- 用户可基于自己的 `REJECTED` 记录重新发起一条新的 `PENDING` 订阅，新记录写入 `retryFromId`，原拒绝记录保持历史不改写
- Emby 真实入库事件命中已通过订阅后转 `INGESTED`，并写入 `ingestedAt`
- 对历史漏回写记录，管理员可主动触发 Emby 校验；只有命中真实资源时，`APPROVED` 才能收口为 `INGESTED`

**审核来源语义**：
- `MANUAL`：管理员显式审批通过
- `AUTO_QUOTA`：系统按套餐分组的每日自动通过额度直接批准
- `REJECTED` / `PENDING` 记录不保留通过来源

**唯一约束**：
- `subscriptions` 不再使用全局唯一索引 `uk_subscription_media`
- 活跃状态唯一索引 `uq_subscriptions_active_media` 只约束 `status IN ('PENDING','APPROVED','INGESTED')` 的 `(type, tmdbId, season)`
- `REJECTED` 历史记录不占用唯一位，允许同一作品在被拒绝后重新提交，但任意时刻同一作品仍只能存在一条活跃订阅

### 2.6 SubscriptionAdminNotification（订阅管理员通知）

**表名**: `subscription_admin_notifications` | **文件**: `models/subscription_admin_notification.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| SubscriptionID | string(25) | subscriptionId | 关联订阅 ID |
| AdminTelegramID | int64 | adminTelegramId | 接收审批消息的 Telegram user_id |
| ChatID | int64 | chatId | Telegram 消息所在 chat ID |
| MessageID | *int64 | messageId | Telegram 消息 ID；发送失败时可为空 |
| HasPhoto | bool | hasPhoto | 原消息是否为 photo caption，用于选择编辑 caption 还是 text |
| DeliveryStatus | string(20) | deliveryStatus | `sent`/`send_failed`/`edit_failed`/`deleted` |
| FailureReason | *string(500) | failureReason | 最近一次发送或编辑失败摘要 |
| CreatedAt | time.Time | createdAt | 自动 |
| UpdatedAt | time.Time | updatedAt | 自动 |

**设计要点**：
- 一条订阅可对应多条管理员 Telegram 审批消息。
- Bot 发送订阅审批私聊后把每条消息引用返回 API，API 写入本表。
- Web 后台或 Telegram 任一端审批成功后，API 按 `subscriptionId` 查询本表并调用 Bot 批量编辑消息为最终状态。
- 编辑失败只更新 `deliveryStatus`，不回滚 `subscriptions` 的审批状态。

### 2.7 EmailVerification（邮箱验证码）

**表名**: `email_verifications` | **文件**: `models/email_verification.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| Email | string(255) | email | 索引 |
| Code | string(6) | code | 6 位验证码（JSON 隐藏）|
| Type | string(20) | type | 验证码类型：`register`/`reset`/`change_email`（索引）|
| IP | string(45) | ip | 请求 IP（索引，JSON 隐藏）|
| ExpiresAt | time.Time | expiresAt | 过期时间 |
| CreatedAt | time.Time | createdAt | 自动 |

### 2.8 Plan（付费方案）

**表名**: `plans` | **文件**: `models/plan.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| Name | string(100) | name | 方案名称 |
| Description | string(500) | description | 描述 |
| Days | int | days | 天数 |
| Price | int64 | price | 价格（分）|
| Currency | string(3) | currency | 币种（当前支持 `"usd"` / `"hkd"` / `"cny"`）|
| PlanGroup | string(50) | planGroup | 套餐所属分组 key（由应用层校验其存在性与删除约束）|
| IsActive | bool | isActive | 是否启用（默认 true，DELETE 接口仅置为 false 作为软删除）|
| SortOrder | int | sortOrder | 排序（默认 0）|
| CreatedAt | time.Time | createdAt | 自动 |
| UpdatedAt | time.Time | updatedAt | 自动 |

**设计要点**：
- `Plan` 只承载 `plans` 表真实列；`planGroupName` 这类 join 后的展示字段由 `services/payment` 查询 DTO 承载，避免普通 `First/Find` 误查不存在列

### 2.8.1 PlanGroup

**表名**: `plan_groups` | **文件**: `models/plan_group.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| Key | string(50) | key | 分组稳定标识，主键 |
| Name | string(100) | name | 分组展示名称 |
| Description | string(500) | description | 分组说明 |
| IsDefault | bool | isDefault | 是否默认分组（全局唯一） |
| SortOrder | int | sortOrder | 排序 |
| SubscriptionAutoApproveDailyLimit | int | subscriptionAutoApproveDailyLimit | 每个用户每天可自动通过的订阅数；`0` 表示全部走人工审核 |
| CreatedAt | time.Time | createdAt | 自动 |
| UpdatedAt | time.Time | updatedAt | 自动 |

### 2.8.2 PlanGroupMediaLibrary

**表名**: `plan_group_media_libraries` | **文件**: `models/media_library_policy.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID 主键 |
| PlanGroupKey | string(50) | planGroupKey | 对应 `plan_groups.key`，服务层维护逻辑关联 |
| LibraryID | string(100) | libraryId | Emby 媒体库 ID |
| LibraryName | string(255) | libraryName | 媒体库名称快照；展示时可被 Emby 最新名称覆盖 |
| LibraryType | string(50) | libraryType | Emby 媒体库类型快照 |
| SortOrder | int | sortOrder | 模板内排序 |
| CreatedAt | time.Time | createdAt | 自动 |
| UpdatedAt | time.Time | updatedAt | 自动 |

**约束**：
- `uq_plan_group_media_libraries_group_library` 保证 `(plan_group_key, library_id)` 唯一
- 不创建数据库外键；删除分组时由服务层在同一事务清理从属记录
- Emby 系统生成的 `boxsets` 合集入口不属于管理员主动配置的媒体库；服务层读取和同步时会过滤该类型

### 2.8.3 PlanGroupEmbyPolicyTemplate

**表名**: `plan_group_emby_policy_templates` | **文件**: `models/media_library_policy.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| PlanGroupKey | string(50) | planGroupKey | 主键，对应 `plan_groups.key` |
| SimultaneousStreamLimit | int | simultaneousStreamLimit | 同时播放限制；写 Emby 时兼容 `SimultaneousStreamLimit` / `MaxActiveSessions` |
| EnableContentDownloading | bool | enableContentDownloading | 是否允许下载 |
| EnableLiveTvAccess | bool | enableLiveTvAccess | 是否允许 Live TV |
| EnableSyncTranscoding | bool | enableSyncTranscoding | 是否允许同步转码 |
| EnableAudioPlaybackTranscoding | bool | enableAudioPlaybackTranscoding | 是否允许音频转码 |
| EnableVideoPlaybackTranscoding | bool | enableVideoPlaybackTranscoding | 是否允许视频转码 |
| EnablePlaybackRemuxing | bool | enablePlaybackRemuxing | 是否允许 remux |
| EnableRemoteAccess | bool | enableRemoteAccess | 是否允许远程访问 |
| CreatedAt | time.Time | createdAt | 自动 |
| UpdatedAt | time.Time | updatedAt | 自动 |

**设计要点**：
- 每个 `planGroup` 必须有且只有一条模板
- 系统强制 `IsAdministrator=false`、`EnableContentDeletion=false`，不进入模板配置
- 新建分组时服务层同步创建默认权益模板

### 2.8.4 UserMediaLibraryPreference

**表名**: `user_media_library_preferences` | **文件**: `models/media_library_policy.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID 主键 |
| UserID | string(25) | userId | 用户 ID |
| LibraryID | string(100) | libraryId | Emby 媒体库 ID |
| Enabled | bool | enabled | 用户是否启用该库 |
| CreatedAt | time.Time | createdAt | 自动 |
| UpdatedAt | time.Time | updatedAt | 自动 |

**设计要点**：
- 无记录表示跟随分组模板
- 一旦保存偏好，记录是当前分组模板内所有库的完整快照；关闭全部库会写入全 false 快照
- 最终可见库为 `planGroup` 模板与启用偏好的交集

### 2.8.5 EmbyPolicySyncBatch / EmbyPolicySyncTask

**表名**: `emby_policy_sync_batches`、`emby_policy_sync_tasks` | **文件**: `models/media_library_policy.go`

`emby_policy_sync_batches` 记录一次分组模板变更的同步批次，包含 `planGroupKey`、`reason`、`status`、总数 / 待处理 / 处理中 / 成功 / 失败计数、创建人和完成时间。

`emby_policy_sync_tasks` 记录单个用户的同步任务，包含 `batchId`、`userId`、`embyId`、`planGroupKey`、`reason`、`status`、`attempts`、`lastError`、`nextRetryAt`。

**约束**：
- `uq_emby_policy_sync_tasks_user_active` 是 partial unique index：`UNIQUE (user_id) WHERE status IN ('pending', 'processing')`
- 当前第一阶段模板保存后会创建批次 / 任务并同步执行；完整后台 worker、历史 preview/apply 接管仍按计划后续收口

### 2.9 Payment（支付记录）

**表名**: `payments` | **文件**: `models/payment.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| UserID | string(25) | userId | 用户 ID（索引）|
| PlanID | string(25) | planId | 方案 ID（索引）|
| StripeSessionID | string | stripeSessionId | Stripe 会话（唯一）|
| StripePaymentIntentID | string | stripePaymentIntentId | Stripe 支付意向 |
| CheckoutURL | string | checkoutUrl | 待支付订单复用的 Stripe Checkout 链接 |
| Amount | int64 | amount | 金额（分）|
| Currency | string | currency | 支付币种快照 |
| Days | int | days | 购买天数 |
| Status | PaymentStatus | status | `pending`/`completed`/`expired`/`failed` |
| ExpiresAt | *time.Time | expiresAt | 本地待支付订单过期时间（默认 30 分钟） |
| CreatedAt | time.Time | createdAt | 自动 |
| UpdatedAt | time.Time | updatedAt | 自动 |

### 2.10 PlaybackRanking（播放排行快照）

**表名**: `playback_rankings` | **文件**: `models/playback_ranking.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| BatchID | string(32) | batchId | 同一次生成的排行榜批次 ID（当前使用 26 位 ULID） |
| Period | RankingPeriod | period | `"daily"` 或 `"weekly"` |
| Category | RankingCategory | category | `"media_movie"` 或 `"media_episode"` |
| Rank | int | rank | 排名 |
| ItemKey | string(128) | itemKey | 稳定聚合键（电影使用 `ItemId`；剧集使用回查 Emby 条目详情得到的 `SeriesId`） |
| ItemSourceType | string(32) | itemSourceType | 聚合键来源（如 `movie_item` / `series` / `episode_item`） |
| ItemName | string(500) | itemName | 媒体名称 |
| PlayCount | int | playCount | 播放次数 |
| Duration | int64 | duration | 总时长（秒）|
| SnapshotAt | time.Time | snapshotAt | 快照时间 |
| PeriodStart | time.Time | periodStart | 周期开始 |
| PeriodEnd | time.Time | periodEnd | 周期结束 |
| CreatedAt | time.Time | createdAt | 自动 |

### 2.11 ClientBlacklist（客户端黑名单）

**表名**: `client_blacklists` | **文件**: `models/client_blacklist.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| ClientName | string(100) | clientName | 客户端名称 |
| NormalizedClientName | string(100) | normalizedClientName | 归一化名称（唯一索引） |
| Reason | string(255) | reason | 黑名单原因 |
| CreatedAt | time.Time | createdAt | 自动 |

### 2.12 DeviceAction（设备操作日志）

**表名**: `device_actions` | **文件**: `models/device_action.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| DeviceID | string(100) | deviceId | 设备 ID（索引） |
| UserID | string(25) | userId | 用户 ID（索引） |
| OperatorID | *string(25) | operatorId | 操作者用户 ID（可空，用于后台审计） |
| ClientName | string(100) | clientName | 客户端名 |
| Action | string(50) | action | 操作类型（blacklist/unblacklist/logout） |
| Note | string(255) | note | 备注 |
| CreatedAt | time.Time | createdAt | 自动 |

### 2.13 TelegramBindCode（Telegram 绑定验证码）

**表名**: `telegram_bind_codes` | **文件**: `models/telegram_bind_code.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| UserID | string(25) | userId | 用户 ID（索引） |
| Code | string(6) | code | 6 位绑定验证码 |
| ExpiresAt | time.Time | expiresAt | 过期时间（默认 5 分钟） |
| CreatedAt | time.Time | createdAt | 自动 |

### 2.14 TVCalendar（追剧日历）

**表名**: `tv_calendar_sources` | **文件**: `models/tv_calendar.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| TmdbID | string(50) | tmdbId | TMDB 剧集 ID（唯一） |
| SeriesID | string(50) | seriesId | Emby SeriesId |
| ShowName | string(255) | showName | 剧名 |
| PosterURL | string(500) | posterUrl | 海报地址 |
| Overview | text | overview | 剧集简介 |
| EmbyStatus | string(20) | embyStatus | Emby 识别状态，当前主要使用 `continuing` |
| LastEpisodeIngestedAt | *time.Time | lastEpisodeIngestedAt | 最近一次新剧集入库时间（用于轻量同步活跃剧） |
| LastSyncedAt | *time.Time | lastSyncedAt | 最近周历同步时间 |
| CreatedAt | time.Time | createdAt | 自动 |
| UpdatedAt | time.Time | updatedAt | 自动 |

**表名**: `tv_calendar_items` | **文件**: `models/tv_calendar.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| TmdbID | string(50) | tmdbId | TMDB 剧集 ID（联合唯一索引） |
| SeriesID | string(50) | seriesId | Emby SeriesId（可空） |
| Season | int | season | 季号（联合唯一索引） |
| Episode | int | episode | 集号（联合唯一索引） |
| AirDate | time.Time | airDate | 播出日期（UTC 00:00:00） |
| EpisodeName | string(255) | episodeName | 集标题 |
| Overview | text | overview | 单集简介 |
| Status | string(20) | status | `ready/missing/upcoming/today` |
| EmbyItemID | string(50) | embyItemId | Emby 集条目 ID（可空） |
| LastChecked | time.Time | lastChecked | 最近同步时间 |
| CreatedAt | time.Time | createdAt | 自动 |
| UpdatedAt | time.Time | updatedAt | 自动 |

**表名**: `tv_calendar_subscriptions` | **文件**: `models/tv_calendar.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| UserID | string(25) | userId | 用户 ID（联合唯一索引） |
| TmdbID | string(50) | tmdbId | TMDB 剧集 ID（联合唯一索引） |
| ShowName | string(255) | showName | 剧名 |
| PosterURL | string(500) | posterUrl | 海报地址 |
| CreatedAt | time.Time | createdAt | 自动 |

**表名**: `tmdb_cache` | **文件**: `models/tv_calendar.go`

| 字段 | 类型 | 列名 | 说明 |
|------|------|------|------|
| ID | string(25) | id | CUID |
| CacheKey | string(255) | cacheKey | 缓存键（唯一） |
| CacheValue | text | cacheValue | TMDB JSON 响应 |
| ExpiresAt | time.Time | expiresAt | 过期时间 |
| CreatedAt | time.Time | createdAt | 自动 |

### 2.15 数据关系

```
User (1) ──→ (N) Redemption     （兑换历史）
User (1) ──→ (N) Subscription   （求片记录）
Subscription (1) ──→ (N) SubscriptionAdminNotification（Telegram 管理员审批消息）
User (1) ──→ (N) Payment        （支付记录）
User (1) ──→ (N) TelegramBindCode（临时绑定验证码）
User (1) ──→ (1) Emby User      （外部 Emby 账号，通过 EmbyID 关联）

Plan (1) ──→ (N) Payment        （方案关联）
PlanGroup (1) ──→ (N) RedemptionCode（注册套餐分组引用）
RedemptionCode ──→ Redemption   （码被使用时生成记录）
Setting                         （全局 KV 配置，无外键）
EmailVerification               （独立验证码，无外键）
PlaybackRanking                 （独立排行快照，无外键）
ClientBlacklist ──→ DeviceAction（按 clientName 审计）
User (1) ──→ (N) TVCalendarSubscription（用户追剧订阅）
TVCalendarSource (1) ──→ (N) TVCalendarItem（按 tmdbId 关联）
TVCalendarSubscription (N) ──→ (1) TVCalendarSource（按 tmdbId 关联）
TMDBCache（独立缓存表）

FailedEmbyAsyncOp               （Emby 写操作补偿队列，cron 重试，无外键）
StripeWebhookEvent              （Stripe webhook event.id 去重表，无外键）
MediaGapScan                    （缺集扫描持久化记录，advisory lock 配套，无外键）
```

### 2.16 FailedEmbyAsyncOp（Emby 写操作补偿队列）

| 字段 | 类型 | 列名 | 备注 |
|---|---|---|---|
| ID | string | id | cuid |
| Origin | enum | origin | `payment_unban` / `redemption_unban` / `register_cleanup` |
| OriginRefID | string | originRefId | 业务侧引用 ID（paymentId / redemptionId / embyUserId） |
| EmbyUserID | string | embyUserId | 待操作的 Emby 账号 |
| Action | enum | action | `unban` / `delete` |
| Payload | *string | payload | 可选 JSON 文本（保留扩展字段，本批未使用） |
| Retries | int | retries | 已重试次数 |
| NextAttemptAt | time.Time | nextAttemptAt | 下次重试时间，cron 按 `<= now()` 拉取 |
| LastError | *string | lastError | 最近一次失败的脱敏错误（最多 500 字符） |

**契约**：
- 业务侧（`payment.fulfillPayment` / `redemption.RedeemCode` / `auth.register`）在事务外异步调 Emby 失败时，通过 `services/account/EmbyCompensation.EnsureUnbanned` / `EnsureDeleted` 入队
- cron `emby-async-compensation @every 10m` 每轮拉取上限 50 条，按 `(origin, action)` 路由到 emby service；成功删除该行
- 失败按指数退避 30s/2m/10m/1h/6h/24h，retries > 6 时仍保留行并写 ERROR 日志告警，需运维介入

### 2.17 StripeWebhookEvent（Stripe webhook 去重表）

| 字段 | 类型 | 列名 | 备注 |
|---|---|---|---|
| EventID | string | eventId | Stripe `event.id`（`evt_xxx`），主键 |
| EventType | string | eventType | Stripe `event.type` |
| Livemode | bool | livemode | Stripe `event.livemode` |
| ReceivedAt | time.Time | receivedAt | 首次收到时间 |
| ProcessedAt | *time.Time | processedAt | 业务分发完成时间 |
| Status | enum | status | `received` / `processed` / `skipped` / `failed` |
| Error | *string | errorMessage | 业务分发失败时的脱敏错误 |

**契约**：
- `HandleWebhook` 在签名验证后、业务分发前 `INSERT ... ON CONFLICT (eventId) DO NOTHING`；`RowsAffected=0` 表示重复事件，直接 200 不再分发
- 业务分发完成后单事务回写 `processedAt / status / errorMessage`
- 处理 `checkout.session.completed` / `async_payment_succeeded` / `async_payment_failed` / `checkout.session.expired` 标记为 `processed`，其余事件类型标记 `skipped`

### 2.18 MediaGapScan（缺集扫描持久化记录）

| 字段 | 类型 | 列名 | 备注 |
|---|---|---|---|
| ID | string | id | cuid |
| Status | enum | status | `running` / `success` / `failed` |
| NodeID | string | nodeId | 承担本次扫描的节点（`hostname/pid`） |
| StartedAt | time.Time | startedAt | 扫描开始时间 |
| FinishedAt | *time.Time | finishedAt | 扫描结束时间 |
| ErrorMessage | *string | errorMessage | 失败时的脱敏错误（最多 500 字符） |

**契约**：
- 扫描入口 `mediaGapScanManager.Start` 先尝试 `pg_try_advisory_lock(scanLockKey)`；锁被占返回 `ErrMediaGapScanInProgress` → handler 映射 409
- 拿到锁后写一条 `running` 记录，扫描结束在 `defer` 中写终态并释放 advisory lock
- advisory lock 绑定在持有连接的 PG session 上：进程 crash 时 PG 端会回收锁；`running` 记录留到 cron 清理时仍保留以便排查孤儿
- cron `media-gap-scans-cleanup @weekly` 删除 7 天之前的 `success / failed` 记录

### 2.19 BotRuntimeLock（Bot polling 单实例租约锁）

**表名**: `bot_runtime_locks` | **文件**: `models/bot_runtime_lock.go`

| 字段 | 类型 | 列名 | 说明 |
|---|---|---|---|
| Name | string | name | 锁名主键，当前固定为 `telegram_polling` |
| OwnerID | string | ownerId | 持锁 Bot 实例标识（hostname + pid + uuid） |
| ExpiresAt | time.Time | expiresAt | 租约到期时间；续租失败或实例 crash 后允许其他实例接管 |
| CreatedAt | time.Time | createdAt | 创建时间 |
| UpdatedAt | time.Time | updatedAt | 最近续租时间 |
