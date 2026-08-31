# Ember API 端点目录

> 本文档承接 Ember 当前 HTTP / Internal API 的完整端点目录，用于协作、排查和调用面盘点。
> 返回格式与字段命名约定以 [API 响应规范](./api-response-standard.md) 为准。

## 1. 公开路由（无需认证）

| 方法 | 路径 | 用途 |
|------|------|------|
| POST | `/api/v1/login` | 登录 |
| GET | `/api/v1/login/protection-config` | 登录页公开保护配置（Turnstile 开关 / Site Key / Hostname） |
| POST | `/api/v1/user/register` | 注册（code/emailCode 可选）|
| POST | `/api/v1/register/send-code` | 发送邮箱验证码 |
| POST | `/api/v1/forgot-password/send-code` | 发送密码重置验证码 |
| POST | `/api/v1/forgot-password/reset` | 通过验证码重置密码 |
| GET | `/api/v1/register/mode` | 获取注册模式（响应字段：`mode`、`defaultTrialDays`、`emailVerification`、`allowedEmailDomains: string[]`；空数组表示不限制注册邮箱域名）|
| GET | `/api/v1/register/code/:code/validate` | 验证注册场景兑换码（会校验绑定的 `registrationPlanGroup` 仍存在） |
| POST | `/api/v1/webhooks/stripe` | Stripe Webhook 回调 |
| POST | `/api/v1/webhooks/emby?token=` | Emby 入库 Webhook（追剧日历） |

## 2. 统一认证路由（admin + user 共享，需 JWT）

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | `/api/v1/subscriptions` | 我的订阅 |
| POST | `/api/v1/subscriptions/check-existing` | 创建前检测库内是否已存在资源 |
| POST | `/api/v1/subscriptions` | 创建订阅（支持可选 `season`，`0` 表示整剧；命中套餐分组当日额度时会直接自动通过） |
| POST | `/api/v1/subscriptions/:id/resubmit` | 基于自己的 `REJECTED` 订阅重新发起，必须提交本次 `note`；命中套餐分组当日额度时会直接自动通过 |
| DELETE | `/api/v1/subscriptions/:id` | 删除订阅 |
| GET | `/api/v1/tmdb/search?query=&type=` | TMDB 搜索（需 JWT，服务端缓存；响应 `{data,total}`） |
| GET | `/api/v1/tmdb/tv/:id/seasons` | TMDB 剧集季列表（需 JWT，服务端缓存） |
| GET | `/api/v1/profile` | 个人信息 |
| GET | `/api/v1/profile/analytics` | 当前登录用户画像（支持 `range` 或 `startDate/endDate`） |
| PUT | `/api/v1/password` | 修改密码 |
| POST | `/api/v1/email/send-code` | 发送邮箱变更验证码到新邮箱（请求体 `{newEmail}`，必填合法邮箱；与 `PUT /api/v1/email` 共用 `change_email` 限流） |
| PUT | `/api/v1/email` | 修改邮箱（请求体 `{newEmail, code}`，`code` 必填 6 位） |
| POST | `/api/v1/redeem` | 通用兑换续期 |
| GET | `/api/v1/redeem/:code/validate` | 续期兑换码预验证（忽略 `registrationPlanGroup`） |
| GET | `/api/v1/redemptions` | 当前登录账号的兑换历史 |
| POST | `/api/v1/telegram/bindcode` | 生成 Telegram 绑定验证码 |
| DELETE | `/api/v1/telegram/unbind` | 解除 Telegram 绑定 |
| GET | `/api/v1/emby/config` | Emby 配置 |
| GET | `/api/v1/media/stats` | 媒体统计（`data` 字段使用 `movieCount/seriesCount/episodeCount`） |
| GET | `/api/v1/media/latest` | 最新入库 |
| GET | `/api/v1/media/posters/:itemId` | 最近入库封面代理（需登录） |
| GET | `/api/v1/user/media-libraries` | 当前登录用户媒体库偏好与分组模板 |
| PUT | `/api/v1/user/media-libraries` | 保存当前登录用户媒体库偏好（请求体 `{enabledLibraryIds}`） |
| DELETE | `/api/v1/user/media-libraries/preferences` | 清除当前登录用户媒体库偏好，恢复分组默认 |
| GET | `/api/v1/rankings/latest` | 最新整期排行（`period`） |
| GET | `/api/v1/rankings/history` | 按日期查询整期历史排行（`period` + `date`） |
| GET | `/api/v1/plans` | 当前登录用户可购方案列表（认证兼容别名，按用户有效套餐分组过滤） |
| GET | `/api/v1/payments/plans` | 当前登录用户可购方案列表（按用户有效套餐分组过滤） |
| POST | `/api/v1/payments/checkout` | Stripe 结账 |
| GET | `/api/v1/payments` | 我的支付记录 |
| GET | `/api/v1/tv-calendar/global` | 全局追剧周历 |
| GET | `/api/v1/tv-calendar/following` | 我的关注周历 |
| GET | `/api/v1/tv-calendar` | 追剧日历 |
| GET | `/api/v1/tv-calendar/subscriptions` | 我的关注列表 |
| POST | `/api/v1/tv-calendar/subscriptions` | 关注剧集 |
| DELETE | `/api/v1/tv-calendar/subscriptions/:tmdbId` | 取消关注剧集 |

## 3. 用户路由（需认证 + role=user）

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | `/api/v1/user/profile` | 个人信息 |
| PUT | `/api/v1/user/password` | 修改密码 |
| POST | `/api/v1/user/email/send-code` | 发送邮箱变更验证码到新邮箱（请求体 `{newEmail}`，必填合法邮箱） |
| PUT | `/api/v1/user/email` | 修改邮箱（请求体 `{newEmail, code}`，`code` 必填 6 位） |
| POST | `/api/v1/user/redeem` | 兑换续期 |
| GET | `/api/v1/user/redeem/:code/validate` | 续期兑换码预验证（忽略 `registrationPlanGroup`） |
| GET | `/api/v1/user/redemptions` | 我的兑换历史 |
| GET | `/api/v1/user/emby/config` | Emby 服务器地址 |
| GET | `/api/v1/user/media/stats` | 媒体库统计 |
| GET | `/api/v1/user/subscriptions` | 我的订阅 |
| POST | `/api/v1/user/subscriptions` | 创建订阅（命中套餐分组当日额度时会直接自动通过） |
| POST | `/api/v1/user/subscriptions/:id/resubmit` | 基于自己的 `REJECTED` 订阅重新发起，必须提交本次 `note`；命中套餐分组当日额度时会直接自动通过 |
| DELETE | `/api/v1/user/subscriptions/:id` | 删除订阅 |

## 4. 管理员路由（需认证 + role=admin）

管理员路由支持两类 Bearer 凭证：管理员 JWT，或设置中心生成的全局 Admin API Key。API Key 没有真实用户身份语义，不能访问统一认证路由、用户路由或 Internal API。涉及凭证本身的 `external-api-key` 和 `p115-accounts` 管理接口只允许管理员 JWT，Admin API Key 调用时返回 `403`。

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | `/api/v1/admin/current` | 当前管理员信息 |
| GET | `/api/v1/admin/emby-users` | Emby 用户候选列表（查询参数 `query` 必填且至少 2 个字符，`limit` 可选；返回 `data`） |
| PUT | `/api/v1/admin/current/emby-binding` | 管理员自助绑定 Emby 账号（请求体 `{embyId}`，404/409/502 错误语义见 `docs/system-architecture.md` §5.1） |
| DELETE | `/api/v1/admin/current/emby-binding` | 管理员解除 Emby 关联（仅清本地 `emby_id`，不动 Emby 用户） |
| GET | `/api/v1/admin/users` | 用户列表（支持按有效 `planGroup` 过滤；历史空分组兼容归入默认分组） |
| POST | `/api/v1/admin/users` | 后台创建普通用户（显式指定 `planGroup` 与 `expiresAt` / `neverExpire`） |
| GET | `/api/v1/admin/users/:id` | 用户详情 |
| GET | `/api/v1/admin/users/:id/profile` | 用户画像（支持 `range` 或 `startDate/endDate`） |
| PUT | `/api/v1/admin/users/:id` | 更新用户 |
| PUT | `/api/v1/admin/users/:id/extend` | 延长有效期 |
| PUT | `/api/v1/admin/users/:id/toggle` | 切换激活状态 |
| PUT | `/api/v1/admin/users/:id/reset-password` | 重置密码 |
| DELETE | `/api/v1/admin/users/:id` | 删除用户 |
| DELETE | `/api/v1/admin/users/:id/media-libraries/preferences` | 清除单个用户媒体库偏好 |
| POST | `/api/v1/admin/users/:id/media-libraries/sync` | 从 Emby 当前 Policy 同步为用户偏好 |
| POST | `/api/v1/admin/users/:id/emby-policy-sync/retry` | 管理员重试单个用户当前有效 Emby Policy 同步 |
| PUT | `/api/v1/admin/users/:id/emby-access` | 管理员显式禁用或恢复用户 Emby 访问（请求体 `{disabled}`，不改变 `isActive`） |
| GET | `/api/v1/admin/redemption-codes` | 兑换码列表（支持 `code` / `status` / `registrationPlanGroup` / `showAll` 过滤） |
| POST | `/api/v1/admin/redemption-codes` | 创建兑换码（必须提交 `registrationPlanGroup`） |
| POST | `/api/v1/admin/redemption-codes/batch` | 批量创建兑换码（必须提交 `registrationPlanGroup`） |
| PUT | `/api/v1/admin/redemption-codes/:id` | 更新兑换码（必须提交 `registrationPlanGroup`） |
| DELETE | `/api/v1/admin/redemption-codes/:id` | 删除兑换码 |
| GET | `/api/v1/admin/configs` | 获取设置中心全部配置（定义 + 当前值 + 来源） |
| PATCH | `/api/v1/admin/configs/:key` | 更新单项配置 |
| POST | `/api/v1/admin/configs/:group/test` | 测试指定配置组 |
| GET | `/api/v1/admin/external-api-key` | 查询全局 Admin API Key 是否已启用（只返回 `configured`） |
| POST | `/api/v1/admin/external-api-key` | 生成或轮换全局 Admin API Key；响应只在本次返回 `apiKey` 明文 |
| DELETE | `/api/v1/admin/external-api-key` | 禁用全局 Admin API Key，清空 `external_api_key_hash` |
| GET | `/api/v1/admin/redemptions` | 全部兑换历史（支持 `username` / `userId` / `code` 过滤） |
| GET | `/api/v1/admin/p115-accounts` | 115 账号概要列表（返回 `data`，不返回 Cookie；仅管理员 JWT） |
| POST | `/api/v1/admin/p115-accounts` | 创建 `pending + disabled` 账号，Cookie 只写；source 同时提交 `embyPathPrefix/sourceRootId`，playback 提交 `targetParentId`（仅管理员 JWT） |
| GET | `/api/v1/admin/p115-accounts/:id` | 查询单个 115 账号概要，不返回 Cookie（仅管理员 JWT） |
| PUT | `/api/v1/admin/p115-accounts/:id/cookie` | 替换 Cookie，并重置为 `pending + disabled`（仅管理员 JWT） |
| POST | `/api/v1/admin/p115-accounts/:id/validate` | 只读验证当前 Cookie；成功进入 `active` 但不自动启用（仅管理员 JWT） |
| PUT | `/api/v1/admin/p115-accounts/:id/enabled` | 设置启用状态；启用要求账号已验证为 `active`（仅管理员 JWT） |
| PUT | `/api/v1/admin/p115-accounts/:id/source-location` | 更新 source 账号的 `embyPathPrefix/sourceRootId`；playback 调用返回 400（仅管理员 JWT） |
| GET | `/api/v1/admin/subscriptions` | 全部订阅 |
| PUT | `/api/v1/admin/subscriptions/:id/approve` | 审批通过 |
| PUT | `/api/v1/admin/subscriptions/:id/reject` | 审批拒绝（请求体必须携带 `reason`） |
| PUT | `/api/v1/admin/subscriptions/:id/ingest` | 校验 Emby 已入库后收口（仅 `APPROVED` 可用） |
| PUT | `/api/v1/admin/subscriptions/:id/redispatch` | 重试 MoviePilot 自动订阅创建（仅 `APPROVED + mpError` 可用） |
| POST | `/api/v1/admin/subscriptions/:id/manual-search` | 手动补偿下载候选搜索；整剧订阅必须提交 `season` |
| POST | `/api/v1/admin/subscriptions/:id/manual-dispatch` | 下发管理员选定的 MoviePilot 候选资源；整剧订阅必须提交搜索时使用的 `season`，订阅继续等待入库 webhook 收口 |
| DELETE | `/api/v1/admin/subscriptions/:id` | 删除订阅 |
| GET | `/api/v1/admin/sessions` | 活跃会话 |
| GET | `/api/v1/admin/playback-history` | 播放历史查询 |
| GET | `/api/v1/admin/playback-profiles` | 用户画像总览（支持 `range` 或 `startDate/endDate`，以及 `keyword/sortBy/sortOrder/page/pageSize`） |
| GET | `/api/v1/admin/media-quality/libraries` | 媒体库列表（质量盘点）；过滤系统生成的 `boxsets` 合集入口 |
| GET | `/api/v1/admin/media-quality/libraries/:libraryId` | 媒体库质量报告（支持 `force/page/pageSize`） |
| POST | `/api/v1/admin/media-quality/libraries/:libraryId/scan` | 触发媒体库质量扫描 |
| GET | `/api/v1/admin/media-quality/libraries/:libraryId/groups/:groupId/details` | 低画质汇总项下钻明细（支持 `force/page/pageSize`） |
| GET | `/api/v1/admin/media-quality/posters/:itemId` | 媒体质量封面代理 |
| GET | `/api/v1/admin/devices` | 设备列表 |
| GET | `/api/v1/admin/devices/stats` | 设备统计 |
| GET | `/api/v1/admin/devices/actions` | 设备操作日志 |
| GET | `/api/v1/admin/devices/blacklist` | 黑名单列表 |
| POST | `/api/v1/admin/devices/blacklist` | 添加黑名单 |
| DELETE | `/api/v1/admin/devices/blacklist/:clientName` | 移除黑名单 |
| POST | `/api/v1/admin/devices/logout/:deviceId` | 强制注销设备 |
| POST | `/api/v1/admin/devices/blacklist/logout-all` | 批量注销黑名单设备 |
| GET | `/api/v1/admin/media-libraries` | Emby 当前媒体库列表，用于配置分组模板；过滤系统生成的 `boxsets` 合集入口 |
| GET | `/api/v1/admin/plan-groups` | 用户分组 / 权益模板列表（包含每日自动通过订阅额度） |
| POST | `/api/v1/admin/plan-groups` | 创建用户分组，并创建默认 Emby 权益模板；支持设置每日自动通过订阅额度 |
| PUT | `/api/v1/admin/plan-groups/:key` | 更新用户分组 / 切换默认分组 / 调整每日自动通过订阅额度 |
| DELETE | `/api/v1/admin/plan-groups/:key` | 删除用户分组；无业务引用时同步清理从属模板和同步记录 |
| GET | `/api/v1/admin/plan-groups/:key/media-libraries` | 查询分组媒体库模板 |
| PUT | `/api/v1/admin/plan-groups/:key/media-libraries` | 保存分组媒体库模板并同步该分组用户 Policy |
| POST | `/api/v1/admin/plan-groups/:key/media-libraries/sync-preview` | 历史用户媒体库权限预览 |
| POST | `/api/v1/admin/plan-groups/:key/media-libraries/sync-apply` | 应用历史用户媒体库权限同步结果 |
| GET | `/api/v1/admin/plan-groups/:key/emby-policy-template` | 查询分组 Emby 权益模板 |
| PUT | `/api/v1/admin/plan-groups/:key/emby-policy-template` | 保存分组 Emby 权益模板并同步该分组用户 Policy |
| GET | `/api/v1/admin/emby-policy-sync-batches/:id` | 查询 Emby Policy 同步批次进度 |
| POST | `/api/v1/admin/emby-policy-sync-batches/:id/retry-failed` | 重试某个同步批次中的失败任务 |
| GET | `/api/v1/admin/plans` | 方案列表（支持 `planGroup` 过滤） |
| POST | `/api/v1/admin/plans` | 创建方案 |
| PUT | `/api/v1/admin/plans/:id` | 更新方案 |
| DELETE | `/api/v1/admin/plans/:id` | 下架方案（软删除） |
| GET | `/api/v1/admin/payments` | 全部支付记录 |
| GET | `/api/v1/admin/system/info` | 系统统计 |
| POST | `/api/v1/admin/system/test-emby` | 测试 Emby 连接 |
| GET | `/api/v1/admin/media-gaps/scan-status` | 查询缺集扫描后台任务状态 |
| POST | `/api/v1/admin/media-gaps/scan` | 异步触发缺集扫描 |
| POST | `/api/v1/admin/media-gaps/:id/search` | 搜索缺集候选资源 |
| POST | `/api/v1/admin/media-gaps/:id/dispatch` | 下发缺集候选资源，请求 MoviePilot 下载入口时携带 `tmdbid` |
| POST | `/api/v1/admin/media-gaps/:id/ignore` | 手动忽略缺集工单 |
| POST | `/api/v1/admin/tv-calendar/sync` | 手动同步追剧日历 |
| POST | `/api/v1/admin/tv-calendar/refresh` | 手动刷新追剧日历 |
| POST | `/api/v1/admin/cron/check-expired` | 手动执行过期检查 |
| POST | `/api/v1/admin/cron/generate-ranking` | 手动生成排行 |
| POST | `/api/v1/admin/rankings/preview` | 排行预览 |
| GET | `/api/v1/admin/rankings/library-allowlist` | 读取排行榜媒体库 allowlist 与当前 Emby 媒体库列表 |
| PUT | `/api/v1/admin/rankings/library-allowlist` | 保存排行榜媒体库 allowlist（请求体 `{libraryIds}`；空数组表示统计全部媒体库） |

追剧日历同步接口说明：

- `POST /api/v1/admin/tv-calendar/sync`：请求体可选，默认同步 `[0,1]`（当前周 + 下周）
- `tmdbId` 可选，传入时只同步单剧
- `weekOffsets` 可选，仅支持 `-1/0/1`
- `force=true` 时跳过轻量活跃剧筛选，并强制刷新 TMDB 缓存
- `POST /api/v1/admin/tv-calendar/refresh` 仍保留，内部复用同步逻辑，作为兼容入口
- Emby 入库 webhook 在保留 TV Calendar 点亮逻辑的同时，额外回写 `subscriptions`：电影按 `tmdbId` 命中 `APPROVED` 电影订阅；剧集优先按 webhook 自带 `tmdbId + season` 命中指定季订阅，若 webhook 未携带剧集主 TMDB ID，则回退用 `seriesId` 向 Emby 查询主剧 `ProviderIds`，优先走 `Items?Ids=`，未命中时再尝试 `/Items/{id}`，同时允许 `season=0` 的整剧订阅在任意季首个真实剧集入库时转为 `INGESTED`

## 5. 内部服务路由（InternalAuth 中间件，Bot 调用）

| 方法 | 路径 | 用途 |
|------|------|------|
| PUT | `/api/v1/internal/subscriptions/:id/approve` | 审批通过 |
| PUT | `/api/v1/internal/subscriptions/:id/reject` | 审批拒绝（请求体必须携带 `reason`） |
| GET | `/api/v1/internal/settings/:key` | 读取内部配置（仅允许访问统一配置层中已注册的非敏感 key；未知 key 返回 404） |
| GET | `/api/v1/internal/media/stats` | 读取内部媒体统计（Bot 复用；`data` 字段使用 `movieCount/seriesCount/episodeCount`） |
| GET | `/api/v1/internal/tmdb/search?query=&type=` | Bot 使用的 TMDB 搜索代理（InternalAuth，服务端缓存；响应 `{data,total}`） |
| GET | `/api/v1/internal/tmdb/tv/:id/seasons` | Bot 使用的 TMDB 剧集季列表代理（InternalAuth，服务端缓存） |
| POST | `/api/v1/internal/telegram/bind` | Bot 校验并绑定账号 |
| POST | `/api/v1/internal/telegram/info` | Bot 查询账号信息 |
| POST | `/api/v1/internal/telegram/redeem` | Bot 兑换续期码 |
| POST | `/api/v1/internal/telegram/reset-password` | Bot 重置账号密码 |
| POST | `/api/v1/internal/telegram/subscribe` | Bot 创建求片订阅 |
| POST | `/api/v1/internal/telegram/media-libraries` | Bot 查询绑定用户媒体库偏好 |
| PUT | `/api/v1/internal/telegram/media-libraries/:libraryId/toggle` | Bot 切换单个媒体库显示状态 |
| DELETE | `/api/v1/internal/telegram/media-libraries/preferences` | Bot 恢复分组默认媒体库偏好 |
| POST | `/api/v1/internal/telegram/reject-request/enqueue` | Bot 入队拒绝待确认记录（请求体必须携带 `chatId`、`adminUserId`、`subscriptionId`） |
| POST | `/api/v1/internal/telegram/reject-request/pop` | Bot 弹出拒绝待确认记录（请求体必须携带 `chatId`、`adminUserId`，且操作者必须与入队记录一致） |

## 6. API 响应格式约定

- **列表**：`{data: [], total, page, pageSize, totalPages}`
- **单个对象**：直接返回对象或 `{user: object}`
- **成功操作**：`{message: "xxx"}`
- **错误**：`{error: "xxx"}`（400/401/404/500）
- **字段命名**：camelCase
