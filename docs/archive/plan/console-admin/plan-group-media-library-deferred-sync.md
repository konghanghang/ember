# PlanGroup 媒体库延迟同步与单用户手动下发实现方案

> 状态：已完成，已归档
> 负责人：Ember
> 更新时间：2026-07-09

## 落地状态

- 已实现 `PlanGroup` 媒体库模板的 deferred / batch 两种保存模式，以及用户侧、管理员侧单用户“同步到 Emby”入口。
- 已引入 `media_library_template_version` / `applied_media_library_template_version` 与 `out_of_sync` 状态语义，分组列表、用户列表和账号中心已按版本差正确展示。
- 已补 API / Web 回归测试，并补齐“模板无变化时不递增版本、不误建批次、不误打脏状态”的集成测试。
- 已完成 `go test ./internal/app`、`go test ./internal/services/policy`、`go build ./...` 和全仓 `scripts/test/all.sh` 验证。
- 稳定结论已同步到 `docs/system-architecture.md`，本稿现在只保留历史追溯价值。

## 背景

这个问题为什么现在要解决：

- 当前管理员修改 `PlanGroup` 的媒体库模板后，后端会立即为该分组所有已绑定 Emby 的普通用户创建同步批次，并由 worker 逐个下发 Emby Policy。
- 当一个分组下用户较多时，管理员只是微调模板，也会立刻触发整组同步；同步耗时长、操作成本高，而且后续模板修正会被 `pending/processing` 任务闸门阻塞。
- 现有系统缺少“只保存模板、稍后再同步”的能力，也缺少“用户自己同步当前模板”或“管理员按单用户同步”的明确入口，导致管理员只能在“全量同步”与“不改模板”之间二选一。
- 如果只加按钮而不补状态模型，系统仍会把“模板已更新但 Emby 尚未下发”的用户显示为“已同步”，管理员和用户都会被误导。

## 目标

本方案要实现：

1. 管理员修改 `PlanGroup` 媒体库模板时，可以选择“仅保存模板”或“保存并同步现有用户”。
2. 用户可以在网页控制台主动把“当前有效媒体库设置”同步到 Emby，而不必先改动偏好。
3. 管理员可以在用户管理页对单个用户手动触发“同步到 Emby”。
4. 系统能够准确区分“同步中 / 同步失败 / 待同步 / 已同步”，不再把延迟下发状态误报为已同步。

## 非目标

本次明确不做：

- 不改变 `PlanGroup` Emby 权益模板的现有保存语义；`/plan-groups/:key/emby-policy-template` 仍保持“保存即创建同步批次”。
- 不修改 Telegram Bot 媒体库交互；本次只覆盖 Web 控制台和管理员后台。
- 不放开“分组存在活动同步任务时继续改模板”的能力；避免旧批次读取到新模板，导致保存但未授权的模板被提前下发。
- 不做 Emby 端实时对账或全量回扫；首版只追踪本方案上线后的模板变更与同步状态。
- 不重做现有“历史用户媒体库同步”预览/应用流程；`sync-preview` / `sync-apply` 继续保留。

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：
  - `docs/system-architecture.md`
  - `docs/reference/web-design-guide.md`
  - `docs/archive/plan/media-subscription/user-media-library-management.md`
- 相关服务/页面/模型：
  - `services/api/internal/services/policy/media_library_settings.go`
  - `services/api/internal/services/policy/effective_policy.go`
  - `services/api/internal/handlers/media_library_policy.go`
  - `services/api/internal/app/routes.go`
  - `services/api/internal/services/user/admin.go`
  - `services/api/internal/services/payment/plan_groups.go`
  - `services/api/internal/models/plan_group.go`
  - `services/api/internal/models/user.go`
  - `services/web/src/views/admin/PlanGroupsView.vue`
  - `services/web/src/views/admin/UsersView.vue`
  - `services/web/src/views/console/AccountCenterView.vue`
- 当前行为：
  - `UpdatePlanGroupMediaLibraries` 每次保存模板都会删除旧模板、写入新模板，并立即调用 `createBatchWithTasks` 创建整组 Emby Policy 同步批次。
  - 用户保存媒体库偏好、恢复默认偏好时，会立刻调用 `ApplyEffectiveUserPolicy` 把当前结果写回 Emby。
  - 管理员用户页已有“从 Emby 读取当前偏好”和“重试 Emby Policy 同步”入口，但没有一个常驻的“把当前模板/当前偏好同步到 Emby”动作。
  - 用户控制台只有“保存偏好”和“恢复默认”，没有“当前设置未变，仅重新同步到 Emby”的独立动作。
- 现有限制：
  - 分组模板修改与批量 Emby 下发是硬绑定关系，管理员无法只改模板。
  - 单用户重新下发当前有效 Policy 的能力存在于后端实现里，但前端只把它作为失败重试使用，语义不清。
  - `policySyncStatus` 当前仅由任务表状态推导；没有活动任务或单用户失败记录时默认返回 `synced`，无法表达“模板已更新但未下发”。
  - 分组列表的同步状态同样只看任务表，不能识别“分组内存在待手动同步用户”。

## 方案设计

### 1. 用户可见行为

- 管理员在“用户分组 / 权益模板”中的媒体库模板弹窗新增两个明确动作：
  - `仅保存模板`
  - `保存并同步现有用户`
- 选择“仅保存模板”时：
  - 只更新 `PlanGroup` 媒体库模板，不创建批量同步任务。
  - 弹窗关闭后提示“模板已保存，N 个用户待同步”。
  - 分组列表同步状态显示为 `待同步`，而不是 `已同步`。
- 选择“保存并同步现有用户”时：
  - 保持现有批量同步流程。
  - 管理端继续展示批次进度、失败用户和失败重试入口。
- 用户控制台账号中心新增“同步到 Emby”按钮：
  - 不修改当前勾选状态。
  - 直接把当前有效媒体库设置下发到 Emby。
  - 若当前使用的是分组默认模板，用户也能主动同步，不需要先人为制造偏好变化。
- 管理员用户页新增常驻“同步到 Emby”动作：
  - 对所有已绑定 Emby 的普通用户展示，不再只在失败时出现。
  - 该动作直接重算并下发该用户当前有效 Policy。
- 管理员用户页现有“同步媒体库偏好”入口保留，但文案改为“从 Emby 读取当前偏好”，明确它是 Emby -> Ember 回读，而不是 Ember -> Emby 下发。
- 哪些现有行为必须保持不变：
  - `保存并同步现有用户` 的批量处理、批次详情和失败重试能力保持不变。
  - 用户保存偏好、恢复默认偏好仍然立即同步 Emby，不改原有成功/失败处理语义。
  - `sync-preview` / `sync-apply` 仍用于历史用户媒体库权限收口，不被本方案替代。
- 前端约束：
  - 前端实现必须遵守 Ember 风格。
  - 设计与交互基线以 `docs/reference/web-design-guide.md` 为准。
  - `PlanGroupsView` 继续沿用现有弹窗和批次展示骨架，不另起第二套分组管理 UI。
  - `UsersView` 与 `AccountCenterView` 的新增按钮、状态标签和提示文案必须复用现有按钮语义、标签语义和空状态基线。

### 2. 数据与模型

- 新增 `plan_groups.media_library_template_version`：
  - 类型：`bigint NOT NULL DEFAULT 1`
  - 含义：记录该分组媒体库模板当前版本；每次保存模板都递增。
- 新增 `users.applied_media_library_template_version`：
  - 类型：`bigint NOT NULL DEFAULT 1`
  - 含义：记录该用户最后一次成功下发到 Emby 的媒体库模板版本。
- 版本字段的适用范围：
  - 本次只用于媒体库模板的“延迟同步”判定。
  - 不扩展到 `PlanGroup` Emby 权益模板保存语义。
  - 后续若要把 Emby 权益模板也改成延迟同步，可再引入更高层的统一有效 Policy 版本。
- 版本回写策略：
  - `PlanGroup` 媒体库模板保存成功后，无论是否立即批量同步，都更新 `media_library_template_version`。
  - 任意一次 `ApplyEffectiveUserPolicy` 成功后，统一把 `users.applied_media_library_template_version` 更新为该用户当前有效 `PlanGroup` 的 `media_library_template_version`。
  - 这样注册、支付履约、管理员改分组、手动单用户同步、用户保存偏好、用户恢复默认、批量任务成功等路径都能自动收口版本。
- 是否需要迁移：
  - 需要 SQL migration，放在 `infrastructure/database/`。
  - 首版迁移只做字段新增和默认值回填：
    - 所有现有 `plan_groups.media_library_template_version = 1`
    - 所有现有 `users.applied_media_library_template_version = 1`
  - 不在 migration 中直接访问 Emby 做历史状态核对；本方案只保证上线后新的模板变更能被准确追踪。
- 本次不新增新表：
  - 继续复用现有 `emby_policy_sync_batches` / `emby_policy_sync_tasks` 表管理批量任务和单用户失败记录。

### 3. 接口与边界

- 修改 `PUT /api/v1/admin/plan-groups/:key/media-libraries`
  - 请求体新增字段：
    - `libraryIds: string[]`
    - `applyToExistingUsers?: boolean`
  - 行为变更：
    - `true`：保存模板并创建批量同步任务
    - `false`：仅保存模板，不创建批量同步任务
- `PUT /api/v1/admin/plan-groups/:key/media-libraries` 响应改为更明确的结果结构，避免把“未创建批次”的情况伪装成批次结果：
  - `mode: "deferred" | "batch"`
  - `status: "out_of_sync" | "pending" | "processing" | "synced" | "partial_failed" | "failed"`
  - `batchId?: string`
  - `affectedUserCount: number`
  - `outOfSyncUserCount?: number`
- 新增 `POST /api/v1/admin/users/:id/emby-policy-sync/apply-current`
  - 语义：对单个用户重新应用当前有效 Emby Policy。
  - 返回：更新后的 `UserInfo`
  - 现有 `POST /api/v1/admin/users/:id/emby-policy-sync/retry` 保留为兼容入口，内部复用同一服务方法。
- 新增 `POST /api/v1/user/emby-policy-sync/apply-current`
  - 语义：当前登录用户重新应用自己的当前有效 Emby Policy。
  - 返回：最新的 `UserMediaLibrarySettings`
- 状态枚举扩展：
  - `EmbyPolicySyncStatus` 新增 `out_of_sync`
  - 适用范围：
    - 分组列表 `policySyncStatus`
    - 用户列表 `policySyncStatus`
    - 用户控制台 `UserMediaLibrarySettings.policySyncStatus`
- 状态判定优先级统一为：
  - `processing`
  - `pending`
  - `failed`
  - `out_of_sync`
  - `synced`
- 现有调用方受影响：
  - `PlanGroupsView` 需要处理 `mode=deferred` 的响应，而不是总是假设有 `batchId`
  - `UsersView` 需要新增“同步到 Emby”动作并调整旧动作文案
  - `AccountCenterView` 需要新增“同步到 Emby”动作

### 4. 关键流程

按顺序写清主链路，不需要贴代码：

1. 管理员在 `PlanGroupsView` 修改某个分组的媒体库模板。
2. 后端校验该分组当前没有 `pending/processing` 任务；若存在活动任务，仍返回 409，避免旧批次读取到新模板。
3. 后端在事务内保存新的媒体库模板，并将 `plan_groups.media_library_template_version` 递增。
4. 若 `applyToExistingUsers=false`：
   - 事务提交后直接返回 `mode=deferred`
   - 不创建 batch/task
   - 该分组用户随后通过版本差识别为 `out_of_sync`
5. 若 `applyToExistingUsers=true`：
   - 在事务内继续创建批量同步任务
   - worker 逐个执行 `ApplyEffectiveUserPolicy`
   - 每个用户成功后更新 `users.applied_media_library_template_version`
6. 用户在账号中心点击“同步到 Emby”时：
   - 后端直接调用 `ApplyEffectiveUserPolicy`
   - 成功后把 `applied_media_library_template_version` 更新到当前分组版本
   - 返回最新 `UserMediaLibrarySettings`
7. 管理员在用户页点击“同步到 Emby”时：
   - 后端复用单用户重算逻辑
   - 成功后更新该用户的 `UserInfo` 与同步状态
8. 用户列表、分组列表、账号中心在读取同步状态时：
   - 先看任务状态
   - 若没有活动/失败任务，再比较版本
   - 分组或用户版本落后时显示 `out_of_sync`

### 5. 失败路径与边界条件

- 分组存在活动批量任务时再次保存模板：
  - 返回 409
  - 包括“仅保存模板”也不能绕过闸门
  - 原因是旧批次当前读取的是最新模板，若放行会导致未授权的模板被部分提前下发
- 用户或管理员手动同步失败：
  - 保持现有单用户失败记录语义
  - 前端提示“Emby 同步失败”
  - 用户状态显示为 `failed`
- 批量同步部分成功：
  - 成功用户更新 `applied_media_library_template_version`
  - 失败用户保持旧版本
  - 批次终态为 `partial_failed`
  - 失败用户后续显示 `failed`，重试成功后再收口
- 用户无 Emby 绑定：
  - 不展示或禁用“同步到 Emby”动作
  - 不参与 `out_of_sync` 判定
- 历史状态兼容：
  - migration 不尝试判断“上线前是否已经手动漂移”
  - 上线时所有现有用户统一视为版本已对齐
  - 本方案只对上线后的模板变更负责
- 兼容性约束：
  - 不能破坏现有批次进度查询和失败重试接口
  - 不能把“从 Emby 读取偏好”和“向 Emby 下发当前设置”混成同一动作
  - 不能因为新增 `out_of_sync` 状态而让旧页面把未知状态渲染为空白

## 影响范围

涉及的子系统：

- API：有，涉及：
  - `PlanGroup` 媒体库模板保存接口
  - 单用户手动同步接口
  - 状态判定逻辑
  - `ApplyEffectiveUserPolicy` 成功后的版本回写
- Web：有，涉及：
  - `PlanGroupsView`
  - `UsersView`
  - `AccountCenterView`
  - `types/api.ts` 状态枚举与响应结构
- Bot：无
- 配置/部署：无新增环境变量；需要新增 SQL migration
- 文档：需要更新：
  - `docs/system-architecture.md`
  - 如接口最终稳定，补充 `docs/reference/` 中的接口或数据模型文档

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`
- `cd services/web && npm run test`
- `cd services/web && npm run build`

按改动补充针对性测试：

- API：
  - `applyToExistingUsers=false` 时只保存模板、不创建 batch
  - 用户/分组状态对 `out_of_sync` 的判定
  - 单用户手动同步成功后版本回写
  - 批量同步部分失败时，成功用户版本前进、失败用户版本保持落后
- Web：
  - `PlanGroupsView` 两个保存动作的分支行为
  - `UsersView` 单用户“同步到 Emby”动作与旧动作文案
  - `AccountCenterView` `out_of_sync` 状态和“同步到 Emby”按钮

### 手工验证

- 在某个用户较多的分组中修改媒体库模板并选择“仅保存模板”：
  - 分组模板保存成功
  - 不创建新批次
  - 页面提示待同步用户数
- 选择“仅保存模板”后进入用户控制台：
  - 当前用户状态显示为 `待同步`
  - 点击“同步到 Emby”后状态恢复为 `已同步`
- 在管理员用户页对单个用户点击“同步到 Emby”：
  - 用户行状态从 `待同步` 变为 `已同步`
  - 不影响同分组其他用户状态
- 在同一分组中选择“保存并同步现有用户”：
  - 仍然创建批次
  - 批次进度、失败用户列表和失败重试入口保持可用
- 在某个分组已有活动批次时再次尝试保存模板：
  - 后端返回 409
  - 前端明确提示“该分组有同步任务未完成，稍后再保存”

## 落地后文档处理

已同步处理：

- 稳定结论已同步到 `docs/system-architecture.md`
  - `PlanGroup` 媒体库模板保存支持 deferred/batch 两种模式
  - 用户与分组的媒体库同步状态新增 `out_of_sync`
  - 单用户手动下发当前有效 Emby Policy 的入口与职责
- 接口、状态字段和版本字段已稳定落地到现行代码与类型定义
- 本稿已迁入 `docs/archive/plan/console-admin/`
