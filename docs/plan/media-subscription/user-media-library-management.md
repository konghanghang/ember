# 用户媒体库管理实现方案

> 状态：草稿
> 负责人：Ember
> 更新时间：2026-05-27

## 背景

这个问题为什么现在要解决：

- Ember 当前已经有 `plan_groups`、`users.plan_group`、`plans.plan_group` 和注册兑换码 `registration_plan_group`，它们已经天然构成“用户业务分组 / 套餐分组”。
- 用户可见媒体库当前主要依赖 Emby 用户 Policy，缺少 Ember 内的显式模板、继承和自助偏好入口。
- 如果只在 Ember 网页端隐藏媒体库，不同步 Emby `Policy`，用户进入 Emby 客户端仍能看到被隐藏的库，权限模型不成立。
- 每个用户都维护一套媒体库授权表会造成重复数据和维护成本；按 `planGroup` 配媒体库模板，用户只存自定义偏好，结构更清楚。
- 用户已经可以在 Telegram Bot 中完成绑定、查询账号、兑换续期和求片订阅，媒体库显示偏好也应支持 Bot 私聊自助修改。

## 目标

本方案要实现：

1. 管理员可以为每个 `planGroup` 配置媒体库模板，作为该分组用户的媒体库上限。
2. 用户默认继承所属 `planGroup` 的媒体库模板。
3. 用户可以在网页控制台和 Telegram Bot 私聊中，在分组模板范围内选择自己实际显示哪些媒体库。
4. 用户未自定义时使用分组模板；用户自定义后使用用户偏好与分组模板的交集。
5. 支付套餐继续绑定 `planGroup`，媒体库模板也绑定 `planGroup`，统一套餐权益和资源库权益边界。
6. 最终可见媒体库必须同步到 Emby 用户 `Policy`，保证 Ember 与 Emby 客户端表现一致。
7. 管理员可以在用户管理列表一键把历史用户当前 Emby 媒体库权限同步为对应分组模板或用户偏好。
8. 将 `planGroup` 管理从支付中心的次级 tab 提升为更显眼的“用户分组 / 权益模板”管理入口，引导管理员先建分组和权益模板，再创建用户、付费计划和邀请码。

## 非目标

本次明确不做：

- 不使用 `role` 作为媒体库分组；`role` 只表示 `admin / user` 权限身份。
- 不让普通用户选择所属 `planGroup` 模板之外的媒体库。
- 不在 Telegram 群聊中开放媒体库修改能力，只允许私聊。
- 不做按设备、客户端、时段或网络环境区分的媒体库策略。
- 不改 Emby 媒体库本身的目录、扫描、命名或元数据。
- 不新增独立“媒体库管理”菜单；媒体库模板和 Emby 权益模板统一挂在“用户分组 / 权益模板”入口下。
- 首版不做单用户专属授权上限；管理员如需给某个用户特殊库，应把用户移动到合适的 `planGroup` 或新建专用分组。

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：
  - `docs/system-architecture.md`
  - `docs/reference/web-design-guide.md`
  - `docs/reference/api-endpoint-catalog.md`
  - `docs/reference/data-model-reference.md`
- 相关服务 / 页面 / 模型：
  - `services/api/internal/models/plan_group.go`
  - `services/api/internal/models/user.go`
  - `services/api/internal/models/plan.go`
  - `services/api/internal/integrations/emby/library.go`
  - `services/api/internal/integrations/emby/emby.go`
  - `services/api/internal/services/payment/plan_groups.go`
  - `services/api/internal/services/auth/service.go`
  - `services/api/internal/services/telegram/service.go`
  - `services/api/internal/handlers/telegram.go`
  - `services/web/src/views/admin/PlanGroupsView.vue`
  - `services/web/src/views/admin/UsersView.vue`
  - `services/web/src/views/console/AccountCenterView.vue`
  - `services/bot/app/handlers/telegram_handler.py`
  - `services/bot/app/clients/api_client.py`
  - `services/bot/app/server.py`
- 当前行为：
  - `EmbyService.GetLibraries()` 已能读取 Emby 媒体库列表。
  - `EmbyService.GetUserPolicyRaw()`、`PatchUserPolicyFields()` 和内部 `setUserPolicyRaw()` 已能读取和更新 Emby 用户 Policy。
  - `PlanGroup` 已承载套餐分组；`User.planGroup` 为空时跟随默认分组。
  - `Plan.planGroup` 已限制用户可购买套餐；支付履约会校验用户分组与套餐分组匹配。
  - 注册兑换码可绑定 `registrationPlanGroup`，注册后写入用户 `planGroup`。
  - 历史注册邀请码曾支持通过模板用户复制 `EnableAllFolders`、`EnabledFolders`、`ExcludedSubFolders` 等媒体库策略字段，本方案要求废弃该机制。
  - 用户通过 `users.emby_id` 关联 Emby 用户，通过 `users.telegram_id` 关联 Telegram 账号。
  - Bot 已有私聊命令、按钮回调、Internal API 客户端和账号绑定链路。
- 现有限制：
  - `planGroup` 还没有媒体库模板。
  - 用户没有网页端或 Telegram 端自助调整媒体库显示偏好的入口。
  - Emby 当前 Policy 不是 Ember 本地可审计、可继承、可重建的权限真相。

## 方案设计

### 1. 用户可见行为

- 管理员侧：
  - 在主导航中提供更显眼的“用户分组 / 权益模板”入口，承载现有 `PlanGroupsView` 的升级版。
  - 在用户分组管理中配置每个 `planGroup` 的基本信息、媒体库模板和 Emby 权益模板。
  - 在用户管理列表中查看用户所属分组、媒体库模板状态和是否存在用户自定义偏好。
  - 在用户管理列表中提供历史用户媒体库权限“一键同步”操作。
  - 在用户管理的单个用户操作中支持调整用户所属 `planGroup`、清除用户媒体库偏好、从 Emby 当前 Policy 同步为用户偏好。
  - 在用户管理中提供“禁用 / 恢复 Emby 访问”操作，写入 `users.emby_access_disabled`；该操作不改变 `users.is_active`，不影响 Ember Web 登录。
  - 支付中心不再作为 `planGroup` 的主维护入口；支付中心中的分组 tab 保留跳转或兼容入口，避免历史路径失效。
- 用户网页端：
  - 用户在账号中心查看自己可访问的媒体库。
  - 用户只能在所属 `planGroup` 模板内选择实际显示哪些媒体库。
  - 用户未修改过时展示“跟随分组模板”；一旦保存偏好，则使用用户自定义偏好。
  - 用户可以清除自定义偏好，重新跟随分组模板。
- Telegram Bot 端：
  - 新增私聊命令 `/libraries`。
  - Bot 返回当前可选媒体库列表，按钮显示当前启用状态和是否为自定义偏好。
  - 用户点击按钮后立即切换该媒体库显示状态，并刷新消息。
  - Bot 端提供“恢复分组默认”按钮。
  - Bot 端不做“暂存后统一保存”，避免引入会话状态、多实例一致性和重启丢失问题。
- 哪些现有行为必须保持不变：
  - 用户登录、续期、封禁、密码重置、求片订阅、媒体质量盘点现有能力不受影响。
  - 支付套餐仍按 `plans.plan_group` 和用户有效 `planGroup` 匹配。
  - 过期 / Emby 禁用用户的 Emby 封禁语义优先级不变，不能因为媒体库偏好更新绕过封禁。
- 前端约束：
  - 前端实现必须遵守 Ember 风格。
  - 设计与交互基线以 `docs/reference/web-design-guide.md` 为准。
  - 分组模板配置复用并升级现有 `PlanGroupsView`，但入口从支付中心次级 tab 提升为管理员主导航中的“用户分组 / 权益模板”。
  - 支付中心如果继续展示“套餐分组”tab，应只作为跳转入口或兼容壳，不维护第二套分组编辑 UI。
  - 用户侧默认放在账号中心，不新增控制台菜单，不把解释型文案堆进概览页。
  - 若存在搜索、筛选、保存按钮、空状态、弹窗表单，必须遵守现有控件基线。

### 2. 数据与模型

新增本地持久化模型，避免只依赖 Emby 当前 Policy 临时推断。

- 新增 `plan_group_media_libraries` 表，记录分组媒体库模板：
  - `id`
  - `plan_group_key`
  - `library_id`
  - `library_name`
  - `library_type`
  - `sort_order`
  - `created_at`
  - `updated_at`
  - 唯一约束：`(plan_group_key, library_id)`
  - 逻辑关联：`plan_group_key` 对应 `plan_groups.key`；系统不创建数据库外键，分组无业务引用且允许删除时，由服务层在同一事务中随分组一并清理。
- 新增 `plan_group_emby_policy_templates` 表，记录分组 Emby 权益模板：
  - `plan_group_key`
  - `simultaneous_stream_limit`
  - `enable_content_downloading`
  - `enable_live_tv_access`
  - `enable_sync_transcoding`
  - `enable_audio_playback_transcoding`
  - `enable_video_playback_transcoding`
  - `enable_playback_remuxing`
  - `enable_remote_access`
  - `created_at`
  - `updated_at`
  - 主键 / 唯一约束：`plan_group_key`
  - 逻辑关联：`plan_group_key` 对应 `plan_groups.key`，每个分组必须有且只有一条权益模板；系统不创建数据库外键，分组无业务引用且允许删除时，由服务层在同一事务中随分组一并清理。
  - 首版只托管上述字段，不做 Emby 全量 Policy 编辑器。
- Emby Policy 字段托管边界：
  - 分组可配置字段：
    - `SimultaneousStreamLimit`
    - `EnableContentDownloading`
    - `EnableLiveTvAccess`
    - `EnableSyncTranscoding`
    - `EnableAudioPlaybackTranscoding`
    - `EnableVideoPlaybackTranscoding`
    - `EnablePlaybackRemuxing`
    - `EnableRemoteAccess`
  - 系统强制字段：
    - `IsAdministrator=false`：永远由系统固定，不能进入分组配置。
    - `EnableContentDeletion=false`：首版固定禁止，不能让用户删除媒体。
  - 首版不开放字段：
    - `EnableUserPreferenceAccess`
    - `EnableLiveTvManagement`
    - `EnableMediaConversion`
    - `EnableSubtitleManagement`
    - `MaxParentalRating`
    - `BlockedTags`
    - `AllowedTags`
  - 不开放字段的处理规则：
    - 创建新 Emby 用户时，按现有默认策略或 Emby 默认值处理。
    - 后续按 `planGroup` 同步 Policy 时不主动修改这些字段，避免误伤管理员在 Emby 端的特殊设置。
    - 如果字段属于系统强制字段，则每次同步都写回强制值。
  - 并发播放数字段兼容规则：
    - 数据库统一只存 `simultaneous_stream_limit`，不把 Emby 版本差异暴露到本地模型。
    - 写 Emby Policy 时读取当前 Raw Policy 自动判断字段名。
    - 如果 Raw Policy 存在 `SimultaneousStreamLimit`，写入 `SimultaneousStreamLimit`。
    - 如果 Raw Policy 不存在 `SimultaneousStreamLimit` 但存在 `MaxActiveSessions`，写入 `MaxActiveSessions`。
    - 如果两个字段都不存在，记录 WARNING，不因未知字段阻断其他托管字段同步。
    - 该规则沿用当前 `ApplyEmberDefaultUserPolicy` 中对不同 Emby 版本的兼容思路。
- 新增 `user_media_library_preferences` 表，记录用户自定义显示偏好：
  - `id`
  - `user_id`
  - `library_id`
  - `enabled`
  - `created_at`
  - `updated_at`
  - 唯一约束：`(user_id, library_id)`
- 新增 `users.emby_access_disabled` 字段，记录管理员显式禁用 Emby 访问的业务意图：
  - 类型：`boolean NOT NULL DEFAULT false`
  - 该字段只控制 Emby `Policy.IsDisabled`，不控制 Ember Web 登录。
  - `users.is_active` 继续保持现有 Ember 本地登录拦截语义；本次不新增、不重定义 Ember 本地登录禁用能力。
  - `users.emby_disabled` 继续作为 Emby 端禁用状态缓存，只在 `ApplyEffectiveUserPolicy` 成功后更新，不作为禁用意图字段。
- 新增 `emby_policy_sync_tasks` 表，记录分组模板变更后的逐用户 Emby Policy 同步任务，以及单用户本地变更后的同步失败处理记录：
  - `id`
  - `batch_id`：分组级批量同步时必填；单用户失败处理记录为空。
  - `user_id`
  - `emby_id`
  - `plan_group_key`
  - `reason`
  - `status`：`pending` / `processing` / `synced` / `failed`
  - `attempts`
  - `last_error`
  - `next_retry_at`：`pending` 任务的可领取时间；单用户人工处理型 `failed` 任务可为空。
  - `created_at`
  - `updated_at`
  - 唯一约束：使用 PostgreSQL partial unique index `UNIQUE (user_id) WHERE status IN ('pending', 'processing')`，避免同一用户堆积重复未完成任务；最终 Policy 每次全量重算，不需要同一用户并存多个待同步任务。
  - migration 中应明确写成 partial unique index，例如 `CREATE UNIQUE INDEX IF NOT EXISTS uq_emby_policy_sync_tasks_user_active ON emby_policy_sync_tasks (user_id) WHERE status IN ('pending', 'processing');`，不能写成普通唯一索引。
  - 任务只记录同步元数据，不存完整 Emby Policy，避免把外部权限快照长期固化在本地。
  - 提交闸门：创建新的媒体库 / 权益模板同步任务前，若目标用户范围内已存在 `pending` 或 `processing` 任务，后端必须返回 409，不写入新的模板或偏好，提示等待当前同步完成。
  - `failed` / `partial_failed` 属于终态，不阻塞后续新提交；管理员可以重试失败项，也可以提交新模板，由新批次按当前状态全量重算收敛。
  - 单用户 `batch_id IS NULL` 的 `failed` 任务表示“本地已提交但 Emby 未同步成功，需要管理员人工处理或手动重试”，不会被 worker 自动消费。
  - 任意一次 `ApplyEffectiveUserPolicy` 成功后，后端必须把同一用户历史单用户失败状态收口为 `synced`，避免旧失败继续污染用户侧状态。
  - 该策略让同一用户不会同时处于多个未完成 Policy 同步中，避免新批次缺任务、旧批次挂进度和最终状态难解释。
- 新增 `emby_policy_sync_batches` 表，记录一次分组级模板变更对应的同步批次：
  - `id`
  - `plan_group_key`
  - `reason`
  - `status`：`pending` / `processing` / `synced` / `partial_failed` / `failed`
  - `total_count`
  - `pending_count`
  - `processing_count`
  - `synced_count`
  - `failed_count`
  - `created_by`
  - `created_at`
  - `updated_at`
  - `finished_at`
  - 批次用于前端展示进度、失败数量和重试入口；逐用户任务通过 `batch_id` 归属到同一次模板变更。
- 新增 Emby Policy 同步 worker：
  - 首版使用 API 进程内 cron/worker 处理 `emby_policy_sync_tasks`，不依赖前端轮询触发真实同步。
  - worker 按 `next_retry_at <= now()` 领取 `pending` 任务，并通过 `FOR UPDATE SKIP LOCKED` 避免多副本重复处理。
  - 任务领取后写为 `processing`，记录 `attempts` 和 `updated_at`；处理成功写为 `synced`，失败写为 `failed` 并记录脱敏后的 `last_error`。
  - 分组批次 `failed` 任务重试时重新置为 `pending`，更新 `next_retry_at`，再次执行时必须调用 `ApplyEffectiveUserPolicy` 全量重算当前有效 Policy，不复用旧快照。
  - 单用户 `batch_id IS NULL` 的 `failed` 任务不进入 worker 自动重试；管理员手动重试接口直接按当前用户状态调用 `ApplyEffectiveUserPolicy`。
  - worker 启动时或每轮执行前应回收超时 `processing` 任务：超过处理超时时间仍未更新的任务重新置为 `pending`，并增加 attempts / last_error，避免进程崩溃后任务永久卡死。
  - 批次进度不依赖长期维护计数字段的正确性；查询批次详情时以 `emby_policy_sync_tasks` 当前状态聚合为准，并同步更新 `emby_policy_sync_batches` 摘要字段和最终状态。
  - 当批次下任务全部终态后，批次状态收口为 `synced` 或 `partial_failed`；全部失败时可收口为 `failed`。
  - 管理端短轮询只查询批次进度，不承担执行任务的职责。
- 不再新增 `user_media_library_grants`：
  - 媒体库上限来自用户有效 `planGroup` 的模板。
  - 用户偏好只能在分组模板范围内做减法，不能扩大权限。
- 是否需要迁移：
  - 需要 SQL migration，放在 `infrastructure/database/`。
  - 迁移脚本必须幂等，写清新增表、索引、约束和是否回填。
  - 本系统不创建数据库外键；跨表关系通过服务层校验、删除前引用检查和同事务显式清理保证。
  - 需要删除线上 `redemption_codes.template_user_id` 字段；该字段属于待废弃兼容债，不为了保留历史模板用户复制语义继续设计迁移通道。
  - 删除前必须做上线检查：盘点仍可用于注册的历史邀请码中是否存在 `template_user_id`，确认这些邀请码后续只按 `registration_plan_group` 和 `planGroup` 模板生效；如仍需特殊权益，由管理员在上线前调整到合适分组。
  - 删除 `template_user_id` 的 migration 必须同步处理相关索引 / 约束，并确认代码、API DTO、前端表单和测试均不再引用该字段。
  - 历史 `users.plan_group IS NULL` 的记录回填为迁移执行时 `plan_groups.is_default = true` 的唯一分组 key；已有非空值保持不变。
  - 历史 `redemption_codes.registration_plan_group IS NULL` 的记录回填为迁移执行时 `plan_groups.is_default = true` 的唯一分组 key；已有非空值保持不变。
  - 如果迁移执行时不存在默认分组，或存在多个默认分组，migration 必须失败并提示先修复 `plan_groups` 默认分组数据。
  - 新增 `users.emby_access_disabled` 后，迁移只把“当前未过期且 `emby_disabled=true`”的用户回填为 `emby_access_disabled=true`，尽量保留历史手动 / 异常 Emby 禁用意图。
  - 当前已过期用户不因 `emby_disabled=true` 回填 `emby_access_disabled`；这些用户的 Emby 禁用继续由过期状态驱动，避免续期后仍被手动禁用卡住。
  - `plans.plan_group` 当前已是必填语义，迁移只校验其引用的分组存在，不改已有值。
  - 分阶段收紧 DB 约束：先回填历史数据并上线 API 必填校验；确认线上无空值后，再将 `users.plan_group` 与 `redemption_codes.registration_plan_group` 改为 `NOT NULL`。
- 默认初始化规则：
  - 首次安装或迁移后必须存在一个可用默认 `planGroup`，例如 `DEFAULT`。
  - 默认 `planGroup` 必须同时拥有一条 `plan_group_emby_policy_templates` 记录。
  - 默认权益模板使用当前代码硬编码默认值：
    - `simultaneous_stream_limit = 3`
    - `enable_content_downloading = false`
    - `enable_live_tv_access = false`
    - `enable_sync_transcoding = false`
    - `enable_audio_playback_transcoding = false`
    - `enable_video_playback_transcoding = false`
    - `enable_playback_remuxing = true`
    - `enable_remote_access = true`
  - 默认同步到 Emby Policy 时还必须强制：
    - `IsAdministrator=false`
    - `EnableContentDeletion=false`
  - 默认媒体库模板可以为空；系统没有配置 Emby 或还没有同步媒体库时，不阻断首次安装。
  - 管理员完成 Emby 配置后，可在“用户分组 / 权益模板”中为默认分组配置媒体库模板。
  - 新建 `planGroup` 时必须同时创建一条默认 Emby 权益模板；管理员可以随后调整。
- 有效分组计算：
  - 新建用户、编辑用户、创建付费计划、创建邀请码时必须显式选择 `planGroup`。
  - 历史用户 `users.plan_group` 为空时，migration 回填当前唯一默认分组 key；已有值保持不变，后续不再依赖“空值跟随默认”的隐式语义。
  - 分组不存在时，用户侧媒体库管理不可用，管理员侧提示重新绑定有效分组。
- 最终 Emby Policy 计算规则：
  - `groupLibraryIds = plan_group_media_libraries[effectivePlanGroup]`
  - 如果用户没有任何 `user_media_library_preferences` 记录：`finalLibraryIds = groupLibraryIds`
  - 如果用户已有偏好记录：`finalLibraryIds = groupLibraryIds ∩ enabledPreferences`
  - 写入 Emby 时设置 `EnableAllFolders = false`
  - 写入 Emby 时设置 `EnabledFolders = finalLibraryIds`
  - 同时按 `plan_group_emby_policy_templates[effectivePlanGroup]` 写入播放、转码、下载、远程访问等权益字段。
  - 保留 `ExcludedSubFolders` 等非本功能管理字段，不做无关清空。
- 统一 Emby Policy 写入口：
  - 新增领域级能力，例如 `ApplyEffectiveUserPolicy(userId, reason)`，作为普通用户 Emby Policy 的唯一写入口。
  - 该入口负责读取当前用户、有效 `planGroup`、分组媒体库模板、分组 Emby 权益模板、用户媒体库偏好和当前 Emby Raw Policy。
  - 该入口合成最终托管字段：
    - `IsDisabled`：这是 Emby `Policy.IsDisabled`，只表示 Emby 账号是否禁用，不表示 Ember 本地登录账号是否禁用。
    - `IsDisabled` 计算公式：`user.IsExpired() || user.emby_access_disabled`。
    - 用户过期时 Emby 禁用；续期、兑换后如果 `emby_access_disabled=false`，则 Emby 可解禁。
    - 管理员显式禁用 Emby 访问时写 `users.emby_access_disabled=true`；管理员显式恢复 Emby 访问时写 `false`，但若用户仍过期，最终 `IsDisabled` 仍为 `true`。
    - `users.is_active` 不参与 Emby `IsDisabled` 计算；它继续只表示 Ember 本地登录账号是否启用。
    - 本功能不新增、不扩大 Ember 本地登录禁用能力；媒体库偏好修改也不得改变本地登录鉴权语义。
    - `IsAdministrator=false`
    - `EnableContentDeletion=false`
    - 媒体库字段：`EnableAllFolders=false`、`EnabledFolders=finalLibraryIds`
    - 权益字段：来自 `plan_group_emby_policy_templates`
  - 该入口保留所有非托管字段，不做全量重置。
  - 该入口成功后负责更新本地 `users.emby_disabled` 为最终 `IsDisabled`；`users.emby_disabled` 是 Emby 端禁用状态缓存，不作为 Ember 登录禁用字段。
  - 禁止新链路继续直接调用 `SetUserPolicy`、`ApplyEmberDefaultUserPolicy` 或散落 patch `GetUserPolicyRaw` / `setUserPolicyRaw` 来修改普通用户 Policy。
  - 管理员 Emby 账号绑定、只读查询、密码更新不属于该入口范围；它们不修改 Policy。
- 用户偏好语义：
  - 没有任何 preferences 记录表示“跟随分组模板”。
  - 一旦用户保存偏好，preferences 作为用户启用集合的完整快照。
  - 用户关闭全部媒体库时，写入当前分组模板内全部库且 `enabled=false`，避免与“无记录=跟随默认”混淆。
  - 清除偏好时删除该用户所有 preferences，重新继承分组模板。
- 分组模板变更语义：
  - 分组新增媒体库：未自定义用户自动可见；已自定义用户默认不可见，除非用户自己启用。
  - 分组移除媒体库：所有用户立即失去该库可见性，即使 preferences 中残留 enabled 也不生效。
  - 分组模板保存后，需要同步该分组下受影响用户的 Emby Policy。
- 分组删除约束：
  - 默认分组不能删除。
  - 仍被 `users.plan_group` 引用时不能删除。
  - 仍被 `plans.plan_group` 引用时不能删除。
  - 仍被 `redemption_codes.registration_plan_group` 引用时不能删除。
  - 无上述业务引用时，允许删除分组，并在同一事务中同步清理 `plan_group_media_libraries`、`plan_group_emby_policy_templates`、`emby_policy_sync_batches` 和 `emby_policy_sync_tasks` 中该分组相关记录。
  - `plan_group_media_libraries`、`plan_group_emby_policy_templates`、`emby_policy_sync_batches` 和 `emby_policy_sync_tasks` 本身不作为阻止删除的业务引用，只作为分组从属配置或同步过程记录。
  - 系统不依赖数据库外键级联删除；删除分组时必须由服务层显式完成从属数据清理。

### 3. 接口与边界

新增 API 与 Internal API 均使用 camelCase 字段，列表响应使用 `data` 字段。

#### 管理员 API

- `GET /api/v1/admin/media-libraries`
  - 返回 Emby 当前媒体库列表。
  - 响应：`{ data: MediaLibraryOption[] }`
- `GET /api/v1/admin/plan-groups/:key/media-libraries`
  - 返回某个分组的媒体库模板。
  - 响应：`{ data: PlanGroupMediaLibrarySettings }`
- `PUT /api/v1/admin/plan-groups/:key/media-libraries`
  - 保存分组媒体库模板。
  - 请求：`{ libraryIds: string[] }`
  - 响应：`{ data: EmbyPolicySyncBatchCreated }`
  - 行为：
    - 校验分组存在。
    - 校验该分组目标用户当前没有未完成 Emby Policy 同步任务；若存在则返回 409，不写入模板。
    - 校验所有 `libraryIds` 存在于 Emby 当前媒体库列表。
    - 更新 `plan_group_media_libraries`。
    - 创建一条 `emby_policy_sync_batches`，并为该分组下已绑定 Emby 的用户批量创建 `emby_policy_sync_tasks`。
    - 返回 `batchId`、受影响用户数和初始同步状态，前端据此展示同步进度。
- `GET /api/v1/admin/plan-groups/:key/emby-policy-template`
  - 返回某个分组的 Emby 权益模板。
  - 响应：`{ data: PlanGroupEmbyPolicyTemplate }`
- `PUT /api/v1/admin/plan-groups/:key/emby-policy-template`
  - 保存分组 Emby 权益模板。
  - 请求：`PlanGroupEmbyPolicyTemplateUpdateRequest`
  - 响应：`{ data: EmbyPolicySyncBatchCreated }`
  - 行为：
    - 校验分组存在。
    - 校验该分组目标用户当前没有未完成 Emby Policy 同步任务；若存在则返回 409，不写入模板。
    - 校验同时播放数等字段范围。
    - 更新 `plan_group_emby_policy_templates`。
    - 创建一条 `emby_policy_sync_batches`，并为该分组下已绑定 Emby 的用户批量创建 `emby_policy_sync_tasks`。
    - 返回 `batchId`、受影响用户数和初始同步状态，前端据此展示同步进度。
- `GET /api/v1/admin/emby-policy-sync-batches/:id`
  - 查询分组级 Emby Policy 同步批次进度。
  - 响应：`{ data: EmbyPolicySyncBatchDetail }`
  - 行为：
    - 返回批次状态、总数、待处理数、处理中数、成功数、失败数和失败用户摘要。
    - 批次完成后前端停止轮询；失败项保留重试入口。
- `POST /api/v1/admin/emby-policy-sync-batches/:id/retry-failed`
  - 重试某个批次下失败的逐用户同步任务。
  - 响应：`{ data: EmbyPolicySyncBatchCreated }`
  - 行为：
    - 只重试该批次下 `failed` 状态的任务。
    - 重试前检查目标用户当前是否已有 `pending` 或 `processing` 同步任务；若存在则返回 409，不修改旧 failed 任务。
    - 重试时再次调用 `ApplyEffectiveUserPolicy` 全量重算当前有效 Policy，不复用旧快照。
    - 若原分组已不存在或用户已不再属于该分组，任务统一标记为 `failed`，并在 `last_error` 记录原因；不新增 `skipped` 状态。
- `POST /api/v1/admin/plan-groups/:key/media-libraries/sync-preview`
  - 预览某个分组下历史用户当前 Emby 媒体库权限，用于初始化或修正分组媒体库模板。
  - 请求：`{ userIds?: string[] }`
  - 响应：`{ data: MediaLibrarySyncPreviewResult }`
  - 行为：
    - 只读取该分组下已绑定 Emby 的用户 Policy，不写入本地模板、不写入 preferences、不调用 Emby 更新接口。
    - 按用户生成媒体库集合；`EnableAllFolders=true` 时按当前 Emby 全部媒体库作为该用户候选集合。
    - 返回候选集合、每个集合覆盖的用户数、是否全组一致、差异用户和读取失败摘要。
    - 后端不得在 preview 阶段自动取并集、交集或首个用户作为模板。
- `POST /api/v1/admin/plan-groups/:key/media-libraries/sync-apply`
  - 应用管理员确认后的历史媒体库同步结果。
  - 请求：`MediaLibrarySyncApplyRequest`
  - 响应：`{ data: MediaLibrarySyncApplyResult }`
  - 行为：
    - 校验该分组目标用户当前没有未完成 Emby Policy 同步任务；若存在则返回 409，不写入模板或 preferences。
    - 必须由前端提交管理员确认后的 `libraryIds` 作为分组媒体库模板，不能复用后端隐式推断结果。
    - 可选提交 `preferenceUserIds`，表示将这些差异用户当前 Emby Policy 写入 `user_media_library_preferences` 作为个人偏好。
    - 未提交到 `preferenceUserIds` 的差异用户后续跟随新的分组模板。
    - 写入分组模板后创建 Emby Policy 同步批次和逐用户同步任务，返回 `batchId`。
    - 单个差异用户 preferences 写入失败不回滚整批，但必须在结果中返回失败项。
- `DELETE /api/v1/admin/users/:id/media-libraries/preferences`
  - 管理员清除单个用户自定义偏好，使其重新跟随所属分组模板。
  - 响应：`{ data: AdminUserDetail }`
  - 行为：
    - 用户存在未完成 Emby Policy 同步任务时返回 409，不删除 preferences。
    - 删除 preferences 后在事务外触发 Emby Policy 同步。
    - 同步失败时创建单用户 `failed` 处理记录，并在响应中返回同步失败状态，供用户管理列表展示失败和重试入口。
- `POST /api/v1/admin/users/:id/media-libraries/sync`
  - 管理员从该用户当前 Emby Policy 同步为用户自定义偏好。
  - 行为：
    - 用户存在未完成 Emby Policy 同步任务时返回 409，不写 preferences。
- `POST /api/v1/admin/users/:id/emby-policy-sync/retry`
  - 管理员手动重试单个用户当前有效 Emby Policy 同步。
  - 响应：`{ data: AdminUserDetail }`
  - 行为：
    - 用户存在 `pending` 或 `processing` 同步任务时返回 409，不发起重试。
    - 后端按当前用户、当前 `planGroup`、当前模板和当前偏好调用 `ApplyEffectiveUserPolicy`，不复用旧失败快照。
    - 重试成功后将该用户历史单用户 `failed` 同步任务收口为 `synced`，用户列表不再显示旧失败。
    - 重试失败时保留 / 新增 `failed` 任务并记录脱敏后的 `last_error`，继续由管理员人工处理。
- `PUT /api/v1/admin/users/:id/emby-access`
  - 管理员显式禁用或恢复单个用户的 Emby 访问。
  - 请求：`{ disabled: boolean }`
  - 响应：`{ data: AdminUserDetail }`
  - 行为：
    - 更新 `users.emby_access_disabled`。
    - 本地提交后触发 `ApplyEffectiveUserPolicy(userId, "admin_emby_access_update")`。
    - 如果用户仍过期，即使 `disabled=false`，最终 Emby `IsDisabled` 仍为 `true`。
    - 不修改 `users.is_active`，不影响 Ember Web 登录。

#### 用户网页 API

- `GET /api/v1/user/media-libraries`
  - 返回当前登录用户所属分组模板、当前启用状态和是否自定义。
  - 响应：`{ data: UserMediaLibrarySettings }`
- `PUT /api/v1/user/media-libraries`
  - 用户保存自己的展示偏好。
  - 请求：`{ enabledLibraryIds: string[] }`
  - 行为：
    - 当前用户存在未完成 Emby Policy 同步任务时返回 409，不写 preferences。
    - 只允许提交当前分组模板内的库。
    - 保存 preferences 完整快照。
    - 本地提交后触发 Emby Policy 同步；同步失败时创建单用户 `failed` 处理记录，并向前端返回同步失败状态，提示联系管理员处理。
- `DELETE /api/v1/user/media-libraries/preferences`
  - 用户清除自定义偏好，重新跟随所属分组模板。
  - 行为：
    - 当前用户存在未完成 Emby Policy 同步任务时返回 409，不删除 preferences。
    - 删除 preferences 后在事务外触发 Emby Policy 同步。
    - 同步失败时创建单用户 `failed` 处理记录，并向前端返回同步失败状态，提示联系管理员处理。

#### Telegram Internal API

- `POST /api/v1/internal/telegram/media-libraries`
  - 请求：`{ telegramId: number }`
  - 返回：`{ data: UserMediaLibrarySettings }`
- `PUT /api/v1/internal/telegram/media-libraries/:libraryId/toggle`
  - 请求：`{ telegramId: number }`
  - 行为：
    - 通过 `telegramId` 找到绑定用户。
    - 用户存在未完成 Emby Policy 同步任务时返回 409，不写 preferences。
    - 校验目标 `libraryId` 在当前分组模板范围内。
    - 切换该库 enabled 状态。
    - 若此前用户未自定义，先以分组模板生成完整偏好快照，再执行 toggle。
    - 本地提交后触发 Emby Policy 同步；同步失败时创建单用户 `failed` 处理记录，并返回同步失败状态供 Bot 展示，Bot 提示联系管理员处理。
    - 返回切换后的完整设置，供 Bot 刷新消息。
- `DELETE /api/v1/internal/telegram/media-libraries/preferences`
  - 请求：`{ telegramId: number }`
  - 行为：
    - 用户存在未完成 Emby Policy 同步任务时返回 409，不删除 preferences。
    - 清除用户偏好，本地提交后触发 Emby Policy 同步。
    - 同步失败时创建单用户 `failed` 处理记录，并返回同步失败状态供 Bot 展示，Bot 提示联系管理员处理。

#### DTO 建议

- `MediaLibraryOption`
  - `id`
  - `name`
  - `type`
  - `itemCount`
- `PlanGroupMediaLibrarySettings`
  - `planGroupKey`
  - `planGroupName`
  - `libraries: MediaLibraryOption[]`
  - `libraryCount`
  - `affectedUserCount`
- `PlanGroupEmbyPolicyTemplate`
  - `planGroupKey`
  - `planGroupName`
  - `simultaneousStreamLimit`
  - `enableContentDownloading`
  - `enableLiveTvAccess`
  - `enableSyncTranscoding`
  - `enableAudioPlaybackTranscoding`
  - `enableVideoPlaybackTranscoding`
  - `enablePlaybackRemuxing`
  - `enableRemoteAccess`
  - `affectedUserCount`
- `UserMediaLibraryItem`
  - `id`
  - `name`
  - `type`
  - `itemCount`
  - `inGroupTemplate`
  - `enabled`
- `UserMediaLibrarySettings`
  - `userId`
  - `embyId`
  - `planGroup`
  - `planGroupName`
  - `customized`
  - `libraries: UserMediaLibraryItem[]`
  - `templateCount`
  - `enabledCount`
  - `policySyncStatus`：`synced` / `pending` / `processing` / `failed`，用于网页端和 Bot 明确提示 Emby Policy 是否已经同步完成。
  - `pendingSyncTaskId`：存在单用户 `pending` / `processing` / `failed` 任务时返回对应任务 ID；字段名保留现有 API 兼容语义，`failed` 时表示管理员处理入口。
- `AdminUserDetail`
  - 基于现有用户详情 / 用户列表 DTO 扩展，供用户管理页使用。
  - 必须包含：
    - `id`
    - `username`
    - `isActive`：Ember 本地登录账号状态。
    - `embyAccessDisabled`：管理员显式禁用 Emby 访问的业务意图，对应 `users.emby_access_disabled`。
    - `embyDisabled`：Emby 端禁用状态同步缓存，对应 `users.emby_disabled`。
    - `isExpired`：用户是否已过期，用于解释 Emby `IsDisabled` 来源。
    - `planGroup`
    - `planGroupName`
    - `effectivePlanGroup`
    - `effectivePlanGroupName`
    - `policySyncStatus`：`synced` / `pending` / `processing` / `failed`，用于用户管理列表展示单用户 Emby Policy 同步状态。
    - `policySyncTaskId`：存在单用户 `pending` / `processing` / `failed` 任务时返回对应任务 ID，供重试入口和排障定位使用。
    - `policySyncLastError`：最近一次单用户同步失败的脱敏错误摘要；无失败时为空。
    - `policySyncUpdatedAt`：最近一次同步状态更新时间，用于展示失败发生时间和判断状态新旧。
- `MediaLibrarySyncPreviewResult`
  - `planGroupKey`
  - `totalUsers`
  - `scannedUsers`
  - `consistent`
  - `candidates: MediaLibrarySyncCandidate[]`
  - `differenceUsers: MediaLibrarySyncDifferenceUser[]`
  - `failedItems: MediaLibrarySyncFailedItem[]`
- `MediaLibrarySyncCandidate`
  - `libraryIds`
  - `libraries: MediaLibraryOption[]`
  - `userCount`
  - `sourceUserIds`
- `MediaLibrarySyncDifferenceUser`
  - `userId`
  - `username`
  - `embyId`
  - `libraryIds`
  - `libraries: MediaLibraryOption[]`
- `MediaLibrarySyncApplyRequest`
  - `libraryIds`
  - `preferenceUserIds`
- `MediaLibrarySyncApplyResult`
  - `planGroupKey`
  - `templateLibraryCount`
  - `preferenceWritten`
  - `failed`
  - `failedItems: MediaLibrarySyncFailedItem[]`
  - `batchId`
  - `affectedUserCount`
- `EmbyPolicySyncBatchCreated`
  - `batchId`
  - `planGroupKey`
  - `reason`
  - `status`
  - `affectedUserCount`
- `EmbyPolicySyncBatchDetail`
  - `batchId`
  - `planGroupKey`
  - `reason`
  - `status`
  - `totalCount`
  - `pendingCount`
  - `processingCount`
  - `syncedCount`
  - `failedCount`
  - `failedItems: EmbyPolicySyncFailedItem[]`

字段命名可在实现时按现有 API 习惯微调，但必须保持统一 camelCase。

### 4. 关键流程

#### 管理员配置分组模板

1. 管理员在“用户分组 / 权益模板”中打开某个 `planGroup` 的模板配置。
2. API 读取 Emby 媒体库列表和当前 `plan_group_media_libraries`。
3. 前端展示可勾选媒体库，并标记当前模板项。
4. 管理员保存模板集合。
5. 后端校验分组、媒体库存在性、未完成同步任务和模板集合合法性。
6. 后端更新分组模板。
7. 后端创建 Emby Policy 同步批次和逐用户同步任务，保存接口返回 `batchId`。
8. 前端刷新分组模板和受影响用户数量，并按 `batchId` 轮询同步进度。
9. 同步完成后前端停止轮询；存在失败项时展示失败数量和重试入口。

#### 管理员配置 Emby 权益模板

1. 管理员在“用户分组 / 权益模板”中打开某个 `planGroup` 的 Emby 权益模板配置。
2. API 读取当前 `plan_group_emby_policy_templates`。
3. 前端展示同时播放数、下载、Live TV、转码、remux、远程访问等字段。
4. 管理员保存权益模板。
5. 后端校验字段范围、分组存在性和未完成同步任务。
6. 后端更新权益模板。
7. 后端创建 Emby Policy 同步批次和逐用户同步任务，保存接口返回 `batchId`。
8. 前端刷新权益模板和受影响用户数量，并按 `batchId` 轮询同步进度。
9. 同步完成后前端停止轮询；存在失败项时展示失败数量和重试入口。

#### 用户管理中的单用户操作

1. 管理员在用户管理列表查看用户所属分组、是否自定义媒体库偏好。
2. 管理员编辑用户时必须提交明确 `planGroup`；如需改变用户资源库上限，调整为另一个有效 `planGroup`，后端不再接受空值清空为 `NULL`。
3. 用户 `planGroup` 改变后，后端提交本地分组变更，并在事务外触发 Emby Policy 同步。
4. 管理员可清除该用户偏好，让用户重新跟随新分组模板。
5. 管理员可从用户当前 Emby Policy 同步为该用户自定义偏好，用于历史接管中的个别修正。
6. 管理员可禁用或恢复该用户 Emby 访问；后端只更新 `emby_access_disabled` 并重算 Emby Policy，不修改 `is_active`。
7. 当用户存在单用户 `failed` 同步任务时，用户管理列表展示“同步失败”，并提供“重试 Emby 同步”操作。
8. 管理员重试成功后，后端收口该用户历史 `failed` 状态；重试失败继续保留失败状态和错误摘要，等待管理员处理。

#### 历史用户一键同步

1. 管理员在用户管理列表或套餐分组管理中点击“一键同步媒体库权限”。
2. 前端先调用 `sync-preview`，说明该操作只会读取当前 Emby 用户媒体库权限，不会立即写入模板或用户偏好。
3. 后端读取该分组下已绑定 Emby 的用户 Policy，并按用户生成媒体库集合。
4. API 返回候选媒体库集合、每个集合覆盖的用户数、是否全组一致、差异用户和读取失败摘要。
5. 如果同一分组内所有用户媒体库集合完全一致，前端允许管理员一键确认，并在确认后调用 `sync-apply` 写入 `plan_group_media_libraries`。
6. 如果同一分组内用户媒体库集合不一致，系统不得自动聚合；前端必须进入人工确认：
   - 管理员选择某个用户的媒体库集合作为分组模板；或
   - 管理员手工勾选媒体库作为分组模板。
7. 管理员确认分组模板后，前端把明确的 `libraryIds` 提交给 `sync-apply`。
8. 后端确认该分组目标用户当前没有未完成同步任务；如果存在则返回 409，要求管理员等待当前同步完成。
9. 差异用户可以由管理员选择写入 `user_media_library_preferences` 作为个人偏好；前端通过 `preferenceUserIds` 明确提交，未选择写入个人偏好的用户后续跟随分组模板。
10. `sync-apply` 写入分组模板后创建 Emby Policy 同步批次和逐用户同步任务，返回 `batchId` 供前端展示进度。
11. 如果按单个用户同步，后端只读取指定用户 Emby Policy，写入该用户 preferences 完整快照，不反推分组模板。
12. 单个用户读取或写入失败时记录失败项，继续处理下一个用户。
13. API 返回预览或应用阶段各自的总数、候选集合、差异用户、成功数、失败数和失败摘要。

#### 新用户创建初始化

1. 管理员先创建或确认 `planGroup`，配置媒体库模板和 Emby 权益模板。
2. 管理员再创建用户、付费计划或邀请码，并选择对应 `planGroup`。
3. 注册或后台创建链路确定用户有效 `planGroup`。
4. 后端创建 Emby 用户，不再执行旧的默认 Policy 收敛链路。
5. 注册链路不再读取邀请码模板用户；注册权益只来自 `registrationPlanGroup` 对应的 `planGroup` 模板。
6. 后端通过统一入口 `ApplyEffectiveUserPolicy(userId, "user_created")` 读取该 `planGroup` 的媒体库模板和 Emby 权益模板，并同步到新用户 Emby Policy。
7. 新用户初始化期间不再单独调用 `ApplyEmberDefaultUserPolicy`、`SetUserPolicy` 或邀请码模板用户复制逻辑。
8. 不为新用户写 preferences；无 preferences 表示跟随分组模板。
9. 如果分组没有媒体库模板，用户创建可以成功，但后端记录 WARNING，用户侧显示暂无可用媒体库；是否阻断注册由后续运营规则另定。

#### 创建 / 编辑用户、付费计划和邀请码

1. 后台创建和编辑用户时，前端必须要求选择 `planGroup`；后端不接受空 `planGroup`，不再允许把 `planGroup` 清空为 `NULL`。
2. 创建或编辑付费计划时，前端必须要求选择 `planGroup`；后端继续校验该分组存在。
3. 创建或编辑注册邀请码时，前端必须要求选择 `registrationPlanGroup`；后端不再接受空值作为“跟随默认分组”。
4. 邀请码的 `templateUserId` 机制明确废弃：新建 / 编辑接口不再接收该字段，前端不再展示模板用户选择器。
5. 注册链路删除 `applyTemplatePolicyIfNeeded` 对邀请码模板用户的调用；注册权益只来自 `registrationPlanGroup` 对应的媒体库模板和 Emby 权益模板。
6. 旧邀请码如果历史上 `registrationPlanGroup` 为空，迁移时回填当前唯一默认分组 key；回填后新建 / 编辑均必须携带明确分组。
7. 旧邀请码如果历史上存在 `template_user_id`，不再保留为运行时字段，也不为模板用户复制语义设计兼容迁移；上线前只做有效邀请码影响检查，需要特殊权益的由管理员提前调整到合适分组。需要历史追溯时应在 migration 前导出或依赖数据库备份 / 审计日志。
8. 新建用户、付费计划、邀请码的表单默认可预选当前默认分组，但提交 payload 必须显式携带分组 key。

#### 用户网页端修改显示偏好

1. 用户打开账号中心媒体库设置。
2. API 返回所属分组模板、当前启用状态和是否自定义。
3. 用户勾选或取消勾选媒体库。
4. 用户保存。
5. 后端校验当前用户没有未完成 Emby Policy 同步任务；如存在则返回 409，提示等待当前同步完成。
6. 后端校验提交集合必须是当前分组模板子集。
7. 后端写入 preferences 完整快照，本地提交后在事务外触发 Emby Policy 同步。
8. 用户选择“恢复分组默认”时，后端删除 preferences，本地提交后在事务外触发 Emby Policy 同步。

#### Telegram 端修改显示偏好

1. 用户在 Bot 私聊发送 `/libraries`。
2. Bot 调用 Internal API 查询绑定用户媒体库设置。
3. Bot 发送媒体库按钮列表，按钮 callbackData 使用 `lib:toggle:<token>`：
   - `token` 优先使用经过 base64url 编码并通过长度校验的 `libraryId`。
   - 如果编码后超过 Telegram callbackData 长度限制，则使用 Bot 侧短 token 映射，映射带 TTL，并在回调时重新向 API 校验真实 `libraryId` 是否仍在分组模板范围内。
4. 用户点击某个媒体库按钮。
5. Bot 调用 toggle Internal API。
6. API 校验 Telegram 绑定、未完成同步任务、分组模板范围和 Emby 绑定。
7. API 切换 preferences，本地提交后触发 Emby Policy 同步。
8. Bot 编辑原消息，展示最新状态。

#### Emby Policy 同步

1. 所有普通用户 Policy 写入必须调用统一入口 `ApplyEffectiveUserPolicy(userId, reason)`。
2. 服务层读取目标用户当前完整 Emby Policy。
3. 只修改本功能托管字段：`IsDisabled`、`IsAdministrator`、`EnableContentDeletion`、`EnableAllFolders`、`EnabledFolders` 和 `plan_group_emby_policy_templates` 明确管理的权益字段。
4. 通过完整 Policy POST 回 Emby。
5. 不使用只带少量字段的 `SetUserPolicy` 更新媒体库权限，避免误伤其他策略字段。
6. 单用户操作沿用当前系统风格，采用“本地事务先提交 + 事务外同步 + 失败可观察”的策略：
   - 适用范围：用户分组变更、用户网页保存偏好、Telegram toggle、清除单个用户偏好、后台 Emby 启停单个用户、过期检查封禁用户、支付 / 兑换 Emby 解封单个用户。
   - 事务内只做本地校验和本地状态写入，不在事务内调用 Emby，避免数据库事务持有外部网络 I/O。
   - 本地事务提交后，事务外调用 `ApplyEffectiveUserPolicy`；关键账号状态变更入口使用 `ApplyEffectiveUserPolicyOrRecordFailure` 或等价封装，确保 Emby 写入失败会落单用户 `failed` 处理记录。
   - Emby 同步成功后更新本地 `users.emby_disabled` 等同步缓存字段。
   - Emby 同步失败时不回滚已经提交的本地业务状态；创建 `batch_id` 为空、`status=failed` 的单用户 `emby_policy_sync_tasks`，记录 ERROR 日志，并交由管理员人工处理或手动重试。
   - 网页端 / Bot 端需要按状态明确提示：`pending` / `processing` 时提示等待同步；`failed` 时提示“Emby 同步失败，请联系管理员处理”；不能让用户误以为 Emby 已立即生效或正在自动重试。
   - 后台 Emby 访问启停、过期封禁、支付 / 兑换 Emby 解封继续保持现有本地业务状态先提交语义；Emby 侧失败必须记录为可观察的失败状态，并提供管理员处理入口；不改变 Ember 本地登录状态。
7. 分组级批量操作采用“本地模板先提交 + 创建同步任务 + 逐用户重试”的策略：
   - 适用范围：分组媒体库模板保存、分组 Emby 权益模板保存、批量历史同步写入分组模板。
   - 事务内锁定目标分组，写入模板变更。
   - 查出有效分组等于该 `planGroup` 且已绑定 Emby 的用户，创建 `emby_policy_sync_batches` 并批量创建归属该批次的 `emby_policy_sync_tasks`。
   - 保存接口返回 `batchId`、受影响用户数和初始状态；前端通过批次查询接口轮询进度。
   - 本地事务提交后，API 进程内 Emby Policy 同步 worker 逐个领取任务并调用 `ApplyEffectiveUserPolicy`。
   - 单个用户同步失败只标记该用户任务为 `failed`，不回滚已提交的分组模板。
   - 管理端必须展示任务总数、成功数、失败数和可重试入口。
   - 首版使用管理端短轮询展示进度，不引入 WebSocket 或 SSE；批次完成后停止轮询。
8. 同步日志记录 `userId`、`embyId`、`planGroup`、模板数量、启用数量、`isDisabled`、`reason` 和 `taskId`，不记录 API Key、Token 或完整 Policy。

#### 现有 Policy 写入口迁移

以下链路必须迁移到统一入口，不能再直接写局部 Policy：

1. Emby 用户创建后的旧默认权限收敛，必须改为调用 `ApplyEffectiveUserPolicy(userId, "user_created")`。
2. 邀请码注册后的分组权益模板应用。
3. 后台创建用户后的权限同步。
4. 后台 Emby 访问启用 / 停用用户。
5. 过期检查 cron 封禁用户。
6. 支付成功或兑换续期后的解封。
7. 用户切换 `planGroup`。
8. 分组媒体库模板变更。
9. 分组 Emby 权益模板变更。
10. 用户网页端媒体库偏好变更。
11. Telegram 媒体库偏好变更。

### 5. 失败路径与边界条件

- 用户未绑定 Emby：网页端和 Bot 端都返回明确业务提示，不写库、不调用 Emby Policy。
- Telegram 未绑定账号：Internal API 返回与现有 Bot 账号能力一致的通用错误，不暴露账号枚举信息。
- 用户有效 `planGroup` 不存在：用户侧媒体库管理不可用，管理员侧提示重新绑定有效分组。
- 分组未配置媒体库模板：用户侧展示空状态；Bot 提示暂无可管理媒体库。
- 用户提交分组模板外媒体库：后端拒绝，不写 preferences，不调用 Emby。
- Emby 媒体库已删除或重命名：以 `libraryId` 为准；列表展示使用最新 Emby 名称，本地 `library_name` 仅做审计快照。
- 分组模板缩小：后端必须同步所有受影响用户，最终 Emby `EnabledFolders` 不能包含被撤销的库。
- Emby 权益模板同步：只能写入托管字段和系统强制字段；首版不开放字段不得被清空、覆盖或从其他模板复制。
- 分组缺少 Emby 权益模板：视为数据不完整；管理员侧提示补齐模板，用户创建和分组切换不得继续。
- 默认分组缺失或默认分组缺少权益模板：启动期 / schema 校验应给出明确错误，管理端也要提示初始化异常。
- 删除 `planGroup`：有用户、付费计划或邀请码引用时必须拒绝；无业务引用时才允许删除，并在同一事务中清理分组媒体库模板、Emby 权益模板、同步批次和同步任务；系统不使用数据库外键级联删除。
- 创建或编辑用户 / 付费计划未提交 `planGroup`，或邀请码未提交 `registrationPlanGroup`：后端返回 400，不再隐式使用默认分组；前端可默认选中默认分组，但必须显式提交。
- 旧邀请码缺少 `registration_plan_group`：migration 必须回填当前唯一默认分组 key；若默认分组不存在或存在多个默认分组，migration 应失败并提示先修复默认分组数据。
- 历史用户缺少 `plan_group`：migration 必须回填当前唯一默认分组 key；若默认分组不存在或存在多个默认分组，migration 应失败并提示先修复默认分组数据。
- 旧邀请码存在 `template_user_id`：字段删除后运行时不再可见；上线前必须检查仍可注册的邀请码并确认后续权益只来自 `registration_plan_group`，如需追溯必须先导出。
- 用户取消全部媒体库：后端允许保存为空集合，Emby 端应看不到任何媒体库；网页端保存时必须二次确认，Telegram 端点击最后一个启用库时必须先出现确认按钮，不能直接关闭。
- 单用户 Emby Policy 同步失败：本地业务变更已经提交，必须创建单用户 `failed` 处理记录；网页端 / Bot 提示“Emby 同步失败，请联系管理员处理”，不能返回假成功，也不能承诺自动重试。
- 分组级批量同步失败：分组模板变更仍以本地数据库为准；失败用户必须留在同步任务表中，管理端展示失败数量和重试入口。
- 存在未完成同步任务时提交新模板或偏好：后端返回 409，不写入新模板、不写入 preferences、不创建新批次；前端提示等待当前同步完成。
- 重试失败任务时目标用户已有未完成同步任务：后端返回 409，不修改旧 failed 任务，避免旧批次重试或单用户手动重试与 active task 冲突。
- 单用户 `failed` 重试成功：后端必须将该用户历史单用户失败任务收口为 `synced`，避免旧失败继续让用户列表、网页端或 Bot 显示同步失败。
- 同步 worker 未运行：批次查询会持续显示 `pending`；部署检查必须确认 API cron/worker 已注册并有日志输出，不能只依赖前端轮询。
- 同步 worker 处理中崩溃：超时 `processing` 任务必须被后续 worker 回收为 `pending` 并继续重试，不能永久卡在 `processing`。
- 多副本并发执行 worker：必须通过 `FOR UPDATE SKIP LOCKED` 或等价数据库锁保证同一任务不会被多个副本同时执行。
- 绕过统一 Policy 入口：实现阶段禁止新增普通用户 Policy 直接写入；测试需要覆盖关键调用方都走 `ApplyEffectiveUserPolicy`。
- 历史用户 `sync-preview` 读取失败：单个用户失败不影响整批；结果必须返回失败用户数量和原因摘要，便于管理员单独处理。
- 历史用户 `sync-apply` 写入失败：单个差异用户 preferences 写入失败不回滚分组模板；结果必须返回失败项，便于管理员单独处理。
- 历史用户分组同步集合不一致：不得自动取并集、交集或首个用户；必须由管理员选择模板来源或手工确认模板。
- 并发修改：分组模板保存、用户分组变更、用户偏好保存可能同时发生，服务层必须用事务和行锁保证最终状态符合 `groupTemplate ∩ preferences`。
- 过期 / Emby 访问禁用用户：媒体库偏好修改不能把 Emby `IsDisabled` 改回 false，必须按 `user.IsExpired() || user.emby_access_disabled` 保留 Emby 禁用结果；该状态不代表 Ember 本地登录禁用。
- 兼容性约束：
  - 现有注册模板用户 Policy 复制链路明确废弃，并通过 SQL migration 删除 `redemption_codes.template_user_id`。
  - 不破坏现有支付套餐分组匹配和履约链路。
  - 不破坏现有过期检查、续期解封和 Emby 访问启停链路；不新增 Ember 本地登录禁用链路。
  - 不破坏媒体质量盘点读取媒体库列表的现有接口。

## 影响范围

涉及的子系统：

- API：有
  - 新增分组媒体库模板模型、分组 Emby 权益模板模型、用户偏好模型、Emby Policy 同步批次 / 任务模型、管理员 API、用户 API、Telegram Internal API。
  - 扩展 PlanGroup 服务、UserService、AuthService、TelegramService 和 Emby Policy patch 能力。
  - 新增足够日志，覆盖分组模板同步、用户偏好同步和 Emby 写入失败。
- Web：有
  - 将现有套餐分组管理升级为“用户分组 / 权益模板”主入口。
  - 支付中心原套餐分组 tab 改为跳转入口或兼容壳，避免维护两套分组 UI。
  - 用户分组管理中的媒体库模板和 Emby 权益模板配置入口。
  - 用户管理列表中的分组归属、偏好状态、Emby 访问状态、批量同步、清除偏好和 Emby 访问启停入口。
  - 用户账号中心中的媒体库显示偏好入口。
  - API 类型与请求层扩展。
- Bot：有
  - 新增 `/libraries` 命令。
  - 新增媒体库按钮回调和恢复分组默认按钮。
  - 新增 Bot API client 方法和消息格式化。
- 配置/部署：无新增环境变量。
- 运行时任务：有
  - API 进程内新增 Emby Policy 同步 worker / cron，用于处理 `emby_policy_sync_tasks`。
  - 部署不新增环境变量，但启动日志和 cron 注册检查必须能确认 worker 已启用。
- 数据库：有
  - 新增 `plan_group_media_libraries`、`plan_group_emby_policy_templates`、`user_media_library_preferences`、`emby_policy_sync_batches`、`emby_policy_sync_tasks` 和对应索引 / 约束。
  - 新增 `users.emby_access_disabled`，用于记录管理员显式禁用 Emby 访问的业务意图。
  - 初始化默认分组及默认 Emby 权益模板。
- 文档：需要更新
  - `docs/system-architecture.md`
  - `docs/reference/data-model-reference.md`
  - `docs/reference/api-endpoint-catalog.md`
  - 如前端页面职责新增入口，同步 `docs/reference/web-information-architecture.md`

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`
- `cd services/web && npm run build`
- `cd services/bot && python -m compileall app`

按改动补充针对性测试：

- API 服务层：
  - 所有普通用户 Emby Policy 写入都通过 `ApplyEffectiveUserPolicy`。
  - `ApplyEffectiveUserPolicy` 能合成媒体库、权益模板和 Emby `IsDisabled` 状态，且不改变 Ember 本地登录鉴权语义。
  - `ApplyEffectiveUserPolicy` 使用 `user.IsExpired() || user.emby_access_disabled` 计算 Emby `IsDisabled`，不读取 `users.is_active` 作为 Emby 禁用来源。
  - 管理员显式禁用 Emby 访问会写入 `users.emby_access_disabled=true` 并同步 Emby `IsDisabled=true`。
  - 管理员显式恢复 Emby 访问会写入 `users.emby_access_disabled=false`；若用户仍过期，最终 Emby `IsDisabled` 仍为 `true`。
  - `users.emby_disabled` 只作为同步缓存，在 `ApplyEffectiveUserPolicy` 成功后更新为最终 `IsDisabled`。
  - 首次安装 / migration 后存在默认 `planGroup` 和默认 Emby 权益模板。
  - 默认 Emby 权益模板字段值与计划定义一致。
  - `simultaneous_stream_limit` 写 Emby 时会按 Raw Policy 自动选择 `SimultaneousStreamLimit` 或 `MaxActiveSessions`。
  - `IsAdministrator` 和 `EnableContentDeletion` 每次同步都写为 false。
  - 首版不开放字段在同步时保持原值，不被覆盖。
  - 新建 `planGroup` 时自动创建默认 Emby 权益模板。
  - 默认 `planGroup` 不允许删除。
  - 被用户、付费计划或邀请码引用的 `planGroup` 不允许删除。
  - 无业务引用的 `planGroup` 删除时，同一事务内同步清理媒体库模板、Emby 权益模板、同步批次和同步任务。
  - 创建用户、编辑用户、创建 / 编辑付费计划缺少显式 `planGroup` 时返回 400。
  - 管理员编辑用户传空 `planGroup` 时返回 400，不再把 `users.plan_group` 清空为 `NULL`。
  - 创建邀请码缺少显式 `registrationPlanGroup` 时返回 400。
  - migration 将历史空 `users.plan_group` 回填为当前唯一默认分组 key，非空值保持不变。
  - migration 将历史空 `redemption_codes.registration_plan_group` 回填为当前唯一默认分组 key，非空值保持不变。
  - migration 在默认分组不存在或存在多个默认分组时失败，不写入错误 fallback。
  - migration 新增 `users.emby_access_disabled`，并仅将当前未过期且 `emby_disabled=true` 的用户回填为 `true`；已过期用户继续由过期状态驱动 Emby 禁用。
  - 回填完成后 DB 约束可收紧为非空，API 层持续保持必填校验。
  - 注册链路不再调用 `applyTemplatePolicyIfNeeded`。
  - 新建 / 编辑邀请码请求不再接收 `templateUserId`。
  - 上线前检查仍可注册的邀请码中是否存在 `template_user_id`，并确认不再依赖模板用户复制语义。
  - `redemption_codes.template_user_id` 删除后模型、DTO、查询筛选和前端不再引用该字段。
  - 分组模板保存正常路径。
  - 分组 Emby 权益模板保存正常路径。
  - 分组 Emby 权益模板保存后创建 Emby Policy 同步批次，并为该分组下用户创建逐用户同步任务。
  - 分组媒体库模板保存后创建 Emby Policy 同步批次，并为该分组下用户创建逐用户同步任务。
  - 分组模板保存接口返回 `batchId`、受影响用户数和初始同步状态。
  - 同一分组存在未完成 Emby Policy 同步任务时，再次保存媒体库模板或权益模板返回 409，且不写入新模板。
  - 用户存在未完成 Emby Policy 同步任务时，网页端保存偏好、恢复默认和 Telegram toggle 返回 409，且不写 preferences。
  - 后台 Emby 访问启停接口只修改 `users.emby_access_disabled`，不修改 `users.is_active`。
  - 后台 Emby 访问启停、用户分组变更、过期检查封禁、支付履约和兑换续期在 Emby Policy 写入失败时都会创建单用户 `failed` 处理记录，管理端可见并可手动重试。
  - `users.is_active=false` 仍保持现有 Ember 登录拦截语义，但不参与 Emby `IsDisabled` 计算。
  - 同步批次查询接口返回总数、待处理数、处理中数、成功数、失败数和失败用户摘要。
  - 同步批次失败项可重试，重试时重新按当前用户和分组状态全量计算 Policy。
  - 同步批次失败项重试前若同一用户已有 `pending` / `processing` 任务，接口返回 409，且旧 failed 任务保持不变。
  - 同步任务逐用户成功 / 失败状态记录。
  - 批次同步任务失败后可由管理员重试，重试时重新置为 `pending`。
  - 单用户 `batch_id IS NULL` 的 `failed` 任务不会被 worker 自动领取，只作为管理员可见的人工处理记录。
  - 管理员单用户 Emby 同步重试接口按当前用户状态调用 `ApplyEffectiveUserPolicy`，成功后将该用户历史单用户 `failed` 收口为 `synced`。
  - 管理员单用户 Emby 同步重试失败时继续保留 / 新增 `failed` 记录，并保存脱敏 `lastError`。
  - Emby Policy 同步 worker 能领取 `pending` 任务、置为 `processing`、成功后置为 `synced`。
  - Emby Policy 同步 worker 处理批次任务失败时记录 `lastError` 并置为 `failed`，后续由管理员重试接口重新置为 `pending`。
  - Emby Policy 同步 worker 使用 `FOR UPDATE SKIP LOCKED` 或等价机制避免多副本重复处理同一任务。
  - Emby Policy 同步 worker 能回收超时 `processing` 任务，避免进程崩溃后任务永久卡死。
  - 批次查询接口以任务表状态聚合进度，并在全部任务终态后把批次收口为 `synced` / `partial_failed` / `failed`。
  - 用户未自定义时继承分组模板。
  - 用户自定义后使用 `groupTemplate ∩ enabledPreferences`。
  - 用户清除 preferences 后重新继承分组模板。
  - 用户切换 `planGroup` 后按新分组模板重算 Emby Policy。
  - 过期封禁、后台 Emby 访问停用、支付 / 兑换 Emby 解封不会覆盖媒体库和权益模板字段，也不会引入 Ember 本地登录禁用。
  - 注册和后台创建用户按有效 `planGroup` 模板初始化 Emby Policy。
  - 分组模板为空时用户创建不写 preferences，用户侧展示空状态。
  - 历史用户 `sync-preview` 只读取 Emby Policy，不写入分组模板、preferences 或 Emby Policy。
  - 历史用户 `sync-preview` 返回候选集合、覆盖用户数、是否一致、差异用户和失败摘要。
  - 历史用户同分组媒体库集合一致时，必须由管理员确认后通过 `sync-apply` 写入分组模板。
  - 历史用户同分组媒体库集合不一致时不会自动聚合，并要求 `sync-apply` 显式提交 `libraryIds`。
  - 历史用户 `sync-apply` 可按 `preferenceUserIds` 将差异用户写入个人 preferences。
  - 历史用户 `sync-apply` 写入分组模板后创建 Emby Policy 同步批次和逐用户任务。
  - `EnableAllFolders=true` 同步为当前全部媒体库候选。
  - `EnableAllFolders=false` 同步为 `EnabledFolders` 候选。
  - 批量同步中单个用户失败不影响其他用户。
  - 用户提交分组模板外媒体库被拒绝。
  - Emby Policy patch 只改媒体库字段，不误伤其他字段。
  - Emby Policy patch 只改权益模板托管字段，不误伤管理员权限、删除权限、标签和家长控制字段。
  - 单用户 Emby 同步失败时，本地业务状态已提交，系统创建 `failed` 人工处理记录，并在响应中返回 `policySyncStatus=failed`，不产生“Emby 已生效”或“已进入自动重试”的假成功。
- Telegram 服务：
  - 未绑定 Telegram 返回通用错误。
  - 已绑定用户查询媒体库设置。
  - toggle 只能切换分组模板内媒体库。
  - 恢复分组默认会删除 preferences，并在事务外触发 Emby 同步。
- Web：
  - “用户分组 / 权益模板”主入口可直接进入并管理分组。
  - 支付中心原套餐分组入口不会出现第二套编辑 UI，只跳转或兼容到新入口。
  - 用户分组媒体库模板配置入口渲染、保存、空状态。
  - 用户分组 Emby 权益模板配置入口渲染、保存、默认值。
  - 分组模板保存后能看到 Emby Policy 同步任务进度、失败数量和重试入口。
  - 用户管理列表能区分展示 Ember 账号状态、Emby 访问禁用意图和 Emby 同步缓存状态，避免把 `isActive`、`embyAccessDisabled`、`embyDisabled` 混成一个状态。
  - 用户管理列表在单用户 `policySyncStatus=failed` 时展示“同步失败”和“重试 Emby 同步”入口。
  - 用户管理列表单用户 Emby 同步重试成功后清除旧失败展示，重试失败后继续保留失败状态和错误摘要。
  - 用户管理列表批量同步入口、确认和结果展示。
  - 用户管理列表清除用户偏好入口。
  - 用户偏好入口渲染、保存、恢复默认、无模板状态。
- Bot：
  - `/libraries` 私聊可用，群聊拒绝。
  - 按钮回调后刷新消息。
  - 恢复分组默认按钮可用。
  - 媒体库偏好写入本地成功但 Emby 同步失败时提示联系管理员处理，不提示已进入自动重试。
  - API 失败时展示错误，不改变本地 Bot 状态。

### 手工验证

- 管理员给默认分组配置两个媒体库，新注册用户登录 Emby 只能看到这两个库。
- 首次安装后无需手工建表数据即可看到默认分组和默认 Emby 权益模板。
- 创建 / 编辑用户、创建 / 编辑付费计划、创建 / 编辑邀请码时必须选择 `planGroup`，表单默认可选中默认分组但提交必须携带分组 key。
- 管理员从主导航进入“用户分组 / 权益模板”，先创建分组和权益模板，再创建用户、付费计划和邀请码。
- 管理员修改 VIP 分组同时播放数或转码权限后，该分组用户 Emby Policy 同步变化。
- 管理员从支付中心历史分组入口进入时，不会进入旧的独立编辑面板，而是跳转或复用新的用户分组入口。
- 管理员给 VIP 分组配置更多媒体库，将用户切到 VIP 后 Emby 可见库同步变化。
- 用户网页端取消其中一个库，刷新 Emby 客户端后只看到剩余库。
- 用户 Telegram 私聊 `/libraries`，点击按钮后 Emby 可见库同步变化。
- 用户恢复分组默认后，Emby 可见库回到所属分组模板。
- 管理员从分组模板撤销某个库后，用户网页端和 Telegram 端都不能再选择该库。
- 用户取消全部库后，Emby 客户端不展示媒体库，续期、登录和账号信息仍正常。
- 过期用户修改媒体库偏好后，Emby 账号仍保持封禁状态。

## 落地后文档处理

落地后应同步处理：

- 将稳定数据模型和关系同步到 `docs/reference/data-model-reference.md`。
- 将新增 API / Internal API 同步到 `docs/reference/api-endpoint-catalog.md`。
- 将 `planGroup` 媒体库模板、用户偏好和 Emby Policy 同步规则同步到 `docs/system-architecture.md`。
- 如新增前端入口说明，更新 `docs/reference/web-information-architecture.md`。
- 主体功能完成、测试通过、文档同步后，将本方案移入 `docs/archive/plan/media-subscription/`。
