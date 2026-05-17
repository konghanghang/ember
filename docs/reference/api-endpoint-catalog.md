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
| POST | `/api/v1/subscriptions` | 创建订阅（支持可选 `season`，`0` 表示整剧） |
| POST | `/api/v1/subscriptions/:id/resubmit` | 基于自己的 `REJECTED` 订阅重新发起，必须提交本次 `note` |
| DELETE | `/api/v1/subscriptions/:id` | 删除订阅 |
| GET | `/api/v1/tmdb/search?query=&type=` | TMDB 搜索（需 JWT，服务端缓存） |
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
| GET | `/api/v1/media/stats` | 媒体统计 |
| GET | `/api/v1/media/latest` | 最新入库 |
| GET | `/api/v1/media/posters/:itemId` | 最近入库封面代理（需登录） |
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
| POST | `/api/v1/user/subscriptions` | 创建订阅 |
| POST | `/api/v1/user/subscriptions/:id/resubmit` | 基于自己的 `REJECTED` 订阅重新发起，必须提交本次 `note` |
| DELETE | `/api/v1/user/subscriptions/:id` | 删除订阅 |

## 4. 管理员路由（需认证 + role=admin）

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | `/api/v1/admin/current` | 当前管理员信息 |
| GET | `/api/v1/admin/emby-users` | Emby 用户候选列表（查询参数 `query` 必填且至少 2 个字符，`limit` 可选；返回 `data`） |
| PUT | `/api/v1/admin/current/emby-binding` | 管理员自助绑定 Emby 账号（请求体 `{embyId}`，404/409/502 错误语义见 `docs/system-architecture.md` §5.1） |
| DELETE | `/api/v1/admin/current/emby-binding` | 管理员解除 Emby 关联（仅清本地 `emby_id`，不动 Emby 用户） |
| GET | `/api/v1/admin/users` | 用户列表（支持按有效 `planGroup` 过滤；显式分组为空时自动归入默认分组） |
| POST | `/api/v1/admin/users` | 后台创建普通用户（显式指定 `planGroup` 与 `expiresAt` / `neverExpire`） |
| GET | `/api/v1/admin/users/:id` | 用户详情 |
| GET | `/api/v1/admin/users/:id/profile` | 用户画像（支持 `range` 或 `startDate/endDate`） |
| PUT | `/api/v1/admin/users/:id` | 更新用户 |
| PUT | `/api/v1/admin/users/:id/extend` | 延长有效期 |
| PUT | `/api/v1/admin/users/:id/toggle` | 切换激活状态 |
| PUT | `/api/v1/admin/users/:id/reset-password` | 重置密码 |
| DELETE | `/api/v1/admin/users/:id` | 删除用户 |
| GET | `/api/v1/admin/redemption-codes` | 兑换码列表（支持 `code` / `status` / `templateUserId` / `registrationPlanGroup` / `showAll` 过滤） |
| POST | `/api/v1/admin/redemption-codes` | 创建兑换码（支持可选 `registrationPlanGroup`） |
| POST | `/api/v1/admin/redemption-codes/batch` | 批量创建兑换码（支持可选 `registrationPlanGroup`） |
| PUT | `/api/v1/admin/redemption-codes/:id` | 更新兑换码（支持可选 `registrationPlanGroup`） |
| DELETE | `/api/v1/admin/redemption-codes/:id` | 删除兑换码 |
| GET | `/api/v1/admin/user-templates` | 模板用户列表 |
| GET | `/api/v1/admin/configs` | 获取设置中心全部配置（定义 + 当前值 + 来源） |
| PATCH | `/api/v1/admin/configs/:key` | 更新单项配置 |
| POST | `/api/v1/admin/configs/:group/test` | 测试指定配置组 |
| GET | `/api/v1/admin/redemptions` | 全部兑换历史（支持 `username` / `userId` / `code` 过滤） |
| GET | `/api/v1/admin/subscriptions` | 全部订阅 |
| PUT | `/api/v1/admin/subscriptions/:id/approve` | 审批通过 |
| PUT | `/api/v1/admin/subscriptions/:id/reject` | 审批拒绝（请求体必须携带 `reason`） |
| PUT | `/api/v1/admin/subscriptions/:id/ingest` | 校验 Emby 已入库后收口（仅 `APPROVED` 可用） |
| DELETE | `/api/v1/admin/subscriptions/:id` | 删除订阅 |
| GET | `/api/v1/admin/sessions` | 活跃会话 |
| GET | `/api/v1/admin/playback-history` | 播放历史查询 |
| GET | `/api/v1/admin/playback-profiles` | 用户画像总览（支持 `range` 或 `startDate/endDate`，以及 `keyword/sortBy/sortOrder/page/pageSize`） |
| GET | `/api/v1/admin/media-quality/libraries` | 媒体库列表（质量盘点） |
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
| GET | `/api/v1/admin/plan-groups` | 套餐分组列表 |
| POST | `/api/v1/admin/plan-groups` | 创建套餐分组 |
| PUT | `/api/v1/admin/plan-groups/:key` | 更新套餐分组 / 切换默认分组 |
| DELETE | `/api/v1/admin/plan-groups/:key` | 删除套餐分组 |
| GET | `/api/v1/admin/plans` | 方案列表（支持 `planGroup` 过滤） |
| POST | `/api/v1/admin/plans` | 创建方案 |
| PUT | `/api/v1/admin/plans/:id` | 更新方案 |
| DELETE | `/api/v1/admin/plans/:id` | 下架方案（软删除） |
| GET | `/api/v1/admin/payments` | 全部支付记录 |
| GET | `/api/v1/admin/system/info` | 系统统计 |
| POST | `/api/v1/admin/system/test-emby` | 测试 Emby 连接 |
| GET | `/api/v1/admin/media-gaps/scan-status` | 查询缺集扫描后台任务状态 |
| POST | `/api/v1/admin/media-gaps/scan` | 异步触发缺集扫描 |
| POST | `/api/v1/admin/tv-calendar/sync` | 手动同步追剧日历 |
| POST | `/api/v1/admin/tv-calendar/refresh` | 手动刷新追剧日历 |
| POST | `/api/v1/admin/cron/check-expired` | 手动执行过期检查 |
| POST | `/api/v1/admin/cron/generate-ranking` | 手动生成排行 |
| POST | `/api/v1/admin/rankings/preview` | 排行预览 |

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
| GET | `/api/v1/internal/media/stats` | 读取内部媒体统计（Bot 复用） |
| POST | `/api/v1/internal/telegram/bind` | Bot 校验并绑定账号 |
| POST | `/api/v1/internal/telegram/info` | Bot 查询账号信息 |
| POST | `/api/v1/internal/telegram/redeem` | Bot 兑换续期码 |
| POST | `/api/v1/internal/telegram/reset-password` | Bot 重置账号密码 |
| POST | `/api/v1/internal/telegram/subscribe` | Bot 创建求片订阅 |
| POST | `/api/v1/internal/telegram/reject-request/enqueue` | Bot 入队拒绝待确认记录（请求体必须携带 `chatId`、`adminUserId`、`subscriptionId`） |
| POST | `/api/v1/internal/telegram/reject-request/pop` | Bot 弹出拒绝待确认记录（请求体必须携带 `chatId`、`adminUserId`，且操作者必须与入队记录一致） |

## 6. API 响应格式约定

- **列表**：`{data: [], total, page, pageSize, totalPages}`
- **单个对象**：直接返回对象或 `{user: object}`
- **成功操作**：`{message: "xxx"}`
- **错误**：`{error: "xxx"}`（400/401/404/500）
- **字段命名**：camelCase
