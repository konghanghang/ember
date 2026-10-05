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
| DELETE | `/api/v1/subscriptions/:id` | 原子取消本人 PENDING 订阅；本人已审核返回 409，不存在/非本人返回 404 |
| GET | `/api/v1/tmdb/search?query=&type=` | TMDB 搜索（需 JWT，服务端缓存；响应 `{data,total}`） |
| GET | `/api/v1/tmdb/tv/:id/seasons` | TMDB 剧集季列表（需 JWT，服务端缓存） |
| GET | `/api/v1/profile` | 个人信息 |
| GET | `/api/v1/profile/analytics` | 当前登录用户画像（支持 `range` 或 `startDate/endDate`） |
| PUT | `/api/v1/password` | 修改密码 |
| POST | `/api/v1/email/send-code` | 发送邮箱变更验证码到新邮箱（请求体 `{newEmail}`，必填合法邮箱；与 `PUT /api/v1/email` 共用 `change_email` 限流） |
| PUT | `/api/v1/email` | 修改邮箱（请求体 `{newEmail, code}`，`code` 必填 6 位） |
| POST | `/api/v1/redeem` | 通用兑换续期 |
| GET | `/api/v1/redeem/:code/validate` | 续期兑换码可用性预验证；实际兑换只续 `registrationPlanGroup` 指定组，拒绝迁移失效码 |
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
| GET | `/api/v1/rankings/latest` | 最新已生成的整期排行（`period`）；当前周期生成后立即可读，空榜也有批次 |
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

排行榜响应保留 `movies/episodes` 与原字段名；`periodStart/periodEnd` 展示实际覆盖日期，零点排他上界折算为前一天。`snapshotAt` 是业务时区下带 offset 的 RFC3339 生成时间；`cutoffAt` 兼容字段现在表示生成时分。数据库查询周期未因此改变，详细边界见 [Playback Reporting 合同](./playback-reporting-api-contract.md#61-ember-快照存储与展示合同)。

## 3. 用户路由（需认证 + role=user）

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | `/api/v1/user/profile` | 个人信息 |
| PUT | `/api/v1/user/password` | 修改密码 |
| POST | `/api/v1/user/email/send-code` | 发送邮箱变更验证码到新邮箱（请求体 `{newEmail}`，必填合法邮箱） |
| PUT | `/api/v1/user/email` | 修改邮箱（请求体 `{newEmail, code}`，`code` 必填 6 位） |
| POST | `/api/v1/user/redeem` | 兑换续期 |
| GET | `/api/v1/user/redeem/:code/validate` | 续期兑换码可用性预验证；实际兑换只续 `registrationPlanGroup` 指定组，拒绝迁移失效码 |
| GET | `/api/v1/user/redemptions` | 我的兑换历史 |
| GET | `/api/v1/user/p115-account` | 当前用户个人 115 playback 安全摘要；不返回 Cookie、内部目录 ID、Provider User-Agent 或 owner |
| POST | `/api/v1/user/p115-account` | 仅提交 `{cookie}` 创建 `pending + disabled` 个人账号；客户端类型由后端派生 |
| PUT | `/api/v1/user/p115-account/cookie` | 仅提交 `{cookie}` 替换凭证，清空旧 Provider/目录并回到 `pending + disabled` |
| POST | `/api/v1/user/p115-account/validate` | 显式验证当前 Cookie；成功进入 `active + disabled` |
| PUT | `/api/v1/user/p115-account/directory` | 提交已有目录路径，由后端解析并原子保存 path/内部 ID |
| PUT | `/api/v1/user/p115-account/concurrency` | 保存 `1..100` 且不超过当前正数套餐 `SimultaneousStreamLimit` 的配置值 |
| PUT | `/api/v1/user/p115-account/enabled` | 启停个人账号；启用时原子复验完整状态和当前套餐 |
| DELETE | `/api/v1/user/p115-account` | 幂等解绑并写入无凭证 `revoked` tombstone；保留 transfer provenance |
| GET | `/api/v1/user/p115-usage` | 本人播放归因、pending 与小时/自然日转存用量；Redis 故障时计数为 `null` |
| GET | `/api/v1/user/emby/config` | Emby 服务器地址 |
| GET | `/api/v1/user/media/stats` | 媒体库统计 |
| GET | `/api/v1/user/subscriptions` | 我的订阅 |
| POST | `/api/v1/user/subscriptions` | 创建订阅（命中套餐分组当日额度时会直接自动通过） |
| POST | `/api/v1/user/subscriptions/:id/resubmit` | 基于自己的 `REJECTED` 订阅重新发起，必须提交本次 `note`；命中套餐分组当日额度时会直接自动通过 |
| DELETE | `/api/v1/user/subscriptions/:id` | 同统一入口：原子取消本人 PENDING，已审核 409，不存在/非本人 404 |

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
| GET | `/api/v1/admin/p115-accounts` | 管理员全局 115 账号概要列表（返回 `data`，排除个人/revoked；共享 playback 同时返回 Redis 用量可用性与计数） |
| POST | `/api/v1/admin/p115-accounts` | 创建 `pending + disabled` 管理员账号，Cookie 只写；source 提交 `embyPathPrefix/sourceRootId`，playback 可在验证后通过路径配置（仅管理员 JWT） |
| GET | `/api/v1/admin/p115-accounts/:id` | 查询单个 115 账号概要，不返回 Cookie（仅管理员 JWT） |
| PUT | `/api/v1/admin/p115-accounts/:id/cookie` | 替换 Cookie，并重置为 `pending + disabled`（仅管理员 JWT） |
| POST | `/api/v1/admin/p115-accounts/:id/validate` | 只读验证当前 Cookie；成功进入 `active` 但不自动启用（仅管理员 JWT） |
| PUT | `/api/v1/admin/p115-accounts/:id/enabled` | 设置启用状态；启用要求账号已验证为 `active`（仅管理员 JWT） |
| PUT | `/api/v1/admin/p115-accounts/:id/source-location` | 更新 source 账号的 `embyPathPrefix/sourceRootId`；playback 调用返回 400（仅管理员 JWT） |
| PUT | `/api/v1/admin/p115-accounts/:id/playback-config` | 为已验证共享 playback 完整提交 `{targetParentPath,maxConcurrentStreams}`；路径解析与版本条件成立后原子保存（仅管理员 JWT） |
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
| GET | `/api/v1/admin/plan-groups` | 用户分组 / 权益模板列表（含订阅额度、`personal|system` 115 路由和小时/每日转存额度） |
| POST | `/api/v1/admin/plan-groups` | 创建用户分组与默认 Emby 权益模板；115 策略缺省为 `personal / 5 / 10` |
| PUT | `/api/v1/admin/plan-groups/:key` | 更新用户分组、默认分组、订阅额度和 115 路由/转存额度 |
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
| GET | `/api/v1/admin/media-gaps` | 工单明细；`status=OPEN` 为未收口，`ALL` 或省略为全部，支持指定工单状态 |
| GET | `/api/v1/admin/media-gaps/grouped` | 聚合列表；省略状态默认 `OPEN`，`ALL` / 指定终态可查历史；计数、摘要和分页同口径，超界页码回退 |
| GET | `/api/v1/admin/media-gaps/scan-status` | 当前进程最近任务状态：`idle/running/succeeded/partial/failed`；`failures` 含 `tmdbId/seriesName/season?/reason`，原因脱敏 |
| POST | `/api/v1/admin/media-gaps/scan` | 异步扫描；可传 `tmdbId` 和 `force` 定向重扫失败剧集 |
| POST | `/api/v1/admin/media-gaps/:id/search` | 搜索缺集候选资源；回写遇到并发状态变化返回 409，前端清候选并刷新 |
| POST | `/api/v1/admin/media-gaps/:id/dispatch` | 工单级跨副本互斥下发；REQUESTED 重发需 `retry=true` 与当前 `expectedUpdatedAt`（RFC3339）；争锁/版本/状态冲突返回 409，不表示撤回远端请求。结果未知保持 REQUESTED，明确业务拒绝为 DISPATCH_FAILED |
| POST | `/api/v1/admin/media-gaps/:id/ignore` | 手动忽略缺集工单 |
| DELETE | `/api/v1/admin/media-gaps/:id` | 仅删除已入库/已忽略工单；缺失 404、未收口/状态冲突 409；成功 `data.deletedCount` |
| POST | `/api/v1/admin/media-gaps/batch-delete` | `{ "ids": [...] }`，1–100 个非空 ID，去重后事务删除；任一缺失/未收口则整批回滚，参数非法 400；成功 `data.deletedCount` |
| POST | `/api/v1/admin/tv-calendar/sync` | 手动同步追剧日历 |
| POST | `/api/v1/admin/tv-calendar/refresh` | 手动刷新追剧日历 |
| POST | `/api/v1/admin/cron/check-expired` | 手动执行过期检查 |
| POST | `/api/v1/admin/cron/generate-ranking` | 手动生成排行 |
| POST | `/api/v1/admin/rankings/preview` | 排行预览 |
| GET | `/api/v1/admin/rankings/library-allowlist` | 读取排行榜媒体库 allowlist 与当前 Emby 媒体库列表 |
| PUT | `/api/v1/admin/rankings/library-allowlist` | 保存排行榜媒体库 allowlist（请求体 `{libraryIds}`；空数组表示统计全部媒体库） |

手动生成排行榜的 `type` 默认为 `daily`，支持 `daily/weekly`；可选 `start/end` 必须同时提供，格式为 `YYYY-MM-DD` 且包含首尾日期。API 按 `CRON_TIMEZONE` 转为 `[开始日零点,结束日次日零点)`，例如 `start=2026-09-29&end=2026-09-29` 完整统计 29 日，不包含 30 日零点；未提供日期时继续生成当前自然日 / 周。快照展示仍返回用户覆盖的结束日期，历史快照不重算。

排行榜媒体库配置 GET 只读返回有效 `libraryIds` 与 `invalidLibraryIds`；非空原配置全部失效时仍为 `allowAll: false`，不会自动清空。生成 / 预览只统计有效选择，全部失效产生空榜；正式生成继续保存并推送空榜。PUT 先验证全部 ID，只有合法全选才规范化为全库。新生成榜单的总时长统一只计电影 / 剧集，低于 60 秒和未入 Top 10 的有效条目也计入；旧快照不重算。

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
| POST | `/api/v1/internal/telegram/reject-request/peek` | 非破坏读取同一 `chatId + adminUserId` 最近点击的未过期记录，直接返回现有记录字段；无记录 404 |
| POST | `/api/v1/internal/telegram/reject-request/complete` | 提交 `pendingRequestId/chatId/adminUserId/reason`，事务内完成拒绝；返回 `subscriptionId/status/changed/rejectReason`，终态重放不重复通知 |
| POST | `/api/v1/internal/telegram/reject-request/pop` | 旧 Bot 升级过渡入口，仍弹出并删除记录；所有旧 Bot 退出且无需回滚后移除，新 Bot 禁止调用 |

## 6. API 响应格式约定

- **列表**：`{data: [], total, page, pageSize, totalPages}`
- **单个对象**：直接返回对象或 `{user: object}`
- **成功操作**：`{message: "xxx"}`
- **错误**：`{error: "xxx"}`（400/401/404/500）
- **字段命名**：camelCase

## 分组权益合同补充（2026-10-04）

| 方法 | 路径 | 权限与合同 |
| --- | --- | --- |
| GET | `/api/v1/user/entitlements` | 普通用户本人；返回 `{data,businessTimezone}`，逐项 `userId/planGroup/planGroupName/validityType/expiresAt`，包括仍保留的过期项 |
| GET | `/api/v1/admin/users/:id/entitlements` | 管理员查看指定用户权益，同一列表结构 |
| POST | `/api/v1/admin/users/:id/entitlements` | `operationId`（重试复用）、`planGroup`、`action=set\|extend\|revoke`；extend 提交正整数 `days`，set 提交 `validityType` 与限时 `expiresAt`；永久不填期限 |
| PUT | `/api/v1/admin/plan-groups/ranks` | `{ranks:{分组key:等级}}`，完整唯一非负整数映射；校验高组资源包含低组后保存并重算用户 |
| POST | `/api/v1/admin/payments/:id/resolve` | `{resolution:external_refund\|compensation,note,benefits?}`；退款只记录外部结果，补偿原子发放，已收口重复请求不重发 |

套餐接口使用唯一 `planGroup`、显式 `validityType` 和 `days`；`duration` 的天数为正整数，`permanent` 天数规范化为 0。创建必填分组和类型；编辑按合并后的字段校验。商品查询响应的 `planGroupName` 为当前分组名称，不修改订单快照。用户套餐列表仍使用 `data`，同一目录/价格附 `purchasable/purchaseReason`，不按当前组过滤。Profile 增加 `resourceAccessGranted`；`isExpired` 表示本地重算后的无权益状态，不是前端实时到期计算。

管理员权益设置的无偏移时间 `YYYY-MM-DD HH:mm:ss` 按 `CRON_TIMEZONE` 解析，也接受带偏移 RFC3339；权益列表的 `businessTimezone` 用于输入标签及显示，不使用浏览器时区猜测。支付记录增加权益快照、`paidAt`、人工原因和处理结果；新状态 `paid_review/resolved` 与 `completed` 一样阻止重复付款发放。

商品创建/编辑明确拒绝旧 `benefits` 参数（包括空数组与 null）；支付历史及人工补偿仍使用多项快照。用户编辑接口仅提交 `planGroup` 时表示将当前组权益迁到目标组，保留原期限和状态；目标已持有返回 409，原组无权益返回 400，同组重试不重复迁移。仍需新增、延长或撤销权益时使用权益接口。编辑当前组期限可提交 `expiresAt`（RFC3339 或按 `CRON_TIMEZONE` 解释的 `YYYY-MM-DD HH:mm:ss`），设永久提交 `clearExpiresAt=true`；两者不能同时提交。用户中心将换组与改期限分开保存，继续保留单独换组的期限迁移语义。

### 兑换码权益有效期合同（2026-10-05）

创建、批量创建、编辑兑换码均接收 `validityType`（`duration` / `permanent`）。兼容未传类型的旧请求，按 `duration` 处理；限时要求 `defaultDays > 0`，永久要求 `defaultDays=0`，非法组合返回 400。`registrationPlanGroup` 继续表示新老用户统一获得的权益分组，`expiresAt` 仅表示码本身的截止时间。

兑换码列表与校验响应、兑换历史增加 `validityType`。用户兑换及 Telegram Internal 兑换成功响应同样携带 `validityType`：永久返回 `days=0/expiresAt=null` 和永久权益提示；限时仍返回所兑换组的新期限。列表仍使用 `data`。历史记录保存实际兑换时的类型，编辑或删除码不改变已授予权益和历史显示；旧历史迁移为 `duration`，不从当前码反推。


### 编辑用户合并延期（2026-10-05）

`PUT /api/v1/admin/users/:id` 增加可选 `extendDays`（正整数）与 `operationId`（延期时必填，最多 64 字符）。延期与邮箱/账号状态编辑在同一用户行锁事务内提交，复用权益审计键保证同一用户同一操作号不重复累加。不得同时提交 `planGroup`、`expiresAt` 或 `clearExpiresAt=true`；不传期限字段即保持原权益。原独立延期 API 保留兼容，仅移除用户列表上的独立按钮。永久已拥有或分组配置不可用沿用权益服务的拒绝规则，返回 400，失败不部分保存资料。
