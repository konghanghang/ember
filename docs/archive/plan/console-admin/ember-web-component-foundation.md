# Ember Web 组件基建收口方案

> 状态：已归档
> 负责人：Ember
> 更新时间：2026-04-15

## 背景

这个问题为什么现在要解决：

- `services/web` 已经形成了 Ember 自己的视觉基线，但大部分样式和页面骨架还直接写在各个 view 里，复用主要停留在 `btn-ember`、`input-ember` 这类样式类层面。
- 后台列表页已经出现稳定的重复模式：`Header Card + Filter Panel + Table Card + Pagination`，但仍在 `UsersView`、`PaymentsView`、`PlaybackHistoryView`、`RedemptionCodesView`、`UserPlaybackProfilesView` 等页面里重复拼装。
- `filter-input`、`filter-date`、`filter-select`、`filter-date-range` 等筛选样式在多个页面各自维护，后续继续加页面时只会继续复制 scoped CSS，维护成本会持续上升。
- 当前 `services/web/src/views` 共有 30 个 `.vue` 文件、约 11800 行；`services/web/src/components` 只有 12 个 `.vue` 文件、约 1763 行。共享 UI 结构明显沉淀不足。

## 目标

本方案要实现：

1. 在 `services/web` 内建立一套可复用的 Ember 前端基础组件层，优先覆盖后台与控制台高频页面骨架、筛选控件、表格容器和弹窗表单。
2. 把重复的样式实现从页面级 scoped CSS 收口到稳定组件或全局基线，减少“每个文件自己定义一套”的扩散。
3. 保持现有用户可见行为和路由职责不变，只收口结构、样式与交互基线，为后续页面迭代提供统一拼装能力。

## 非目标

本次明确不做：

- 不在首期引入完整第三方 design system 或重写 Element Plus。
- 不把所有页面一次性重构成“万能配置驱动页面”，避免过度抽象。
- 不优先抽取强业务、强视觉特例页面为通用组件，例如 `TVCalendarView` 这种特例页面先做内部拆分，不强行定义为全局基建。
- 不修改后端 API、数据库模型或用户权限模型。

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：
  - `docs/system-architecture.md`
  - `docs/reference/web-design-guide.md`
- 相关目录与文件：
  - `services/web/src/assets/base.css`
  - `services/web/src/components/common/DefaultAvatar.vue`
  - `services/web/src/components/profile/PlaybackProfileContent.vue`
  - `services/web/src/views/admin/UsersView.vue`
  - `services/web/src/views/admin/PaymentsView.vue`
  - `services/web/src/views/admin/PlaybackHistoryView.vue`
  - `services/web/src/views/admin/RedemptionCodesView.vue`
  - `services/web/src/views/admin/UserPlaybackProfilesView.vue`
  - `services/web/src/views/admin/PlansView.vue`
  - `services/web/src/views/admin/PlanGroupsView.vue`
  - `services/web/src/views/admin/PaymentCenterView.vue`
  - `services/web/src/views/admin/RedemptionCenterView.vue`
  - `services/web/src/views/LoginView.vue`
  - `services/web/src/views/ForgotPasswordView.vue`
  - `services/web/src/views/user/RegisterView.vue`
- 当前行为：
  - 全局已存在 `btn-ember`、`input-ember` 和部分 Element Plus 覆盖样式，见 `services/web/src/assets/base.css`。
  - 页面级仍存在大量重复骨架：
    - `bg-white p-6 rounded-2xl border border-gray-100 shadow-sm`
    - `bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden`
    - `rounded-2xl border border-gray-200 bg-gray-50/60`
  - 列表页表头样式 `:header-cell-style="{ background: '#f9fafb', color: '#6b7280', fontWeight: '600' }"` 在多个页面直接重复。
  - 筛选控件相关样式在多个页面重复出现：
    - `services/web/src/views/admin/UsersView.vue`
    - `services/web/src/views/admin/PaymentsView.vue`
    - `services/web/src/views/admin/PlaybackHistoryView.vue`
    - `services/web/src/views/admin/RedemptionCodesView.vue`
    - `services/web/src/views/admin/RedemptionHistoryView.vue`
    - `services/web/src/views/admin/UserPlaybackProfilesView.vue`
    - `services/web/src/components/profile/PlaybackProfileContent.vue`
  - 已有正向样板是 `services/web/src/components/profile/PlaybackProfileContent.vue`，同一套展示主体已被 user/admin 两个页面共享。
- 现有限制：
  - 新页面仍要手动复制 header、filter、table、dialog 结构。
  - scoped CSS 总量已偏高，探索期统计约 1500 行样式仍散落在 views 中。
  - 当前组件目录按业务分组较多，但缺少明确的 Ember UI 基建层，导致“结构可复用”和“样式可复用”都没有稳定入口。
- 已完成项：
  - 已建立 `services/web/src/components/ember/` 目录，并落下 10 个基础组件：
    - `layout/EmberPageHeaderCard.vue`
    - `layout/EmberFilterPanel.vue`
    - `layout/EmberSegmentTabs.vue`
    - `filters/EmberSearchInput.vue`
    - `filters/EmberSelectField.vue`
    - `filters/EmberDateField.vue`
    - `filters/EmberDateRangeField.vue`
    - `data-display/EmberTableCard.vue`
    - `data-display/EmberMetricCard.vue`
    - `forms/EmberFormDialog.vue`
  - 已完成首批和扩展页面迁移：
    - `services/web/src/views/admin/PaymentsView.vue`
    - `services/web/src/views/admin/UsersView.vue`
    - `services/web/src/views/admin/PlaybackHistoryView.vue`
    - `services/web/src/views/admin/RedemptionCodesView.vue`
    - `services/web/src/views/admin/UserPlaybackProfilesView.vue`
  - 已完成第二批页面迁移与容器收口：
    - `services/web/src/views/admin/PaymentCenterView.vue`
    - `services/web/src/views/admin/RedemptionCenterView.vue`
    - `services/web/src/views/admin/RedemptionHistoryView.vue`
    - `services/web/src/views/admin/PlansView.vue`
    - `services/web/src/views/admin/PlanGroupsView.vue`
  - 已完成第三批页面收口：
    - `services/web/src/views/admin/DevicesView.vue`
    - `services/web/src/views/console/RenewalCenterView.vue`
    - `services/web/src/views/admin/MediaQualityView.vue`
  - 已完成第四批基线与控制台收口：
    - `services/web/src/assets/base.css`
      - 已统一 `form-select`、`form-date` 与 dialog chrome 基线
    - `services/web/src/components/ember/forms/EmberFormDialog.vue`
      - 已接入统一 dialog chrome
    - `services/web/src/views/console/LibraryView.vue`
      - 已接入 `EmberPageHeaderCard` 与 `EmberSegmentTabs`
    - `services/web/src/views/console/RankingsView.vue`
      - 已接入 `EmberPageHeaderCard` 与 `EmberSegmentTabs`
    - `services/web/src/views/console/SubscriptionsView.vue`
      - 已接入 `EmberPageHeaderCard` 与 `EmberSegmentTabs`
    - `services/web/src/views/console/NewSubscriptionView.vue`
      - 已接入统一 dialog chrome 与表单字段基线
  - 已完成第五批轻量组件与目录清理：
    - `services/web/src/components/ember/feedback/EmberEmptyStateCard.vue`
    - `services/web/src/components/profile/PlaybackProfileContent.vue`
      - 已统一空状态与简单统计卡
    - `services/web/src/views/console/DashboardView.vue`
      - 已统一服务器入口相关空状态
    - `services/web/src/components/ember/data-display/EmberMetricCard.vue`
      - 已补足 `valueClass`，覆盖非大数字型统计值
    - `services/web/src/components/HelloWorld.vue`
      - 已删除无引用脚手架残留
  - 上述迁移完成后均已执行 `cd services/web && npm run build` 验证通过。
- 收口结论：
  - 已落地的组件目录、职责边界与页面骨架规则已同步到 `docs/system-architecture.md` 与 `docs/reference/web-design-guide.md`。
  - 认证页边界已经明确：`LoginView`、`ForgotPasswordView`、`RegisterView` 不接入当前后台基建层，继续保留页面级特例实现。
  - `form-number` 已在 `services/web/src/assets/base.css` 内沉淀为全局数字输入基线，本期不再额外抽 Ember 数字输入组件。
  - 后续扩展页面结论已经明确：
    - `services/web/src/views/admin/DevicesView.vue`：已完成标准后台列表页收口，页头、统计卡、筛选区与主表格已接入 `EmberPageHeaderCard`、`EmberFilterPanel`、`EmberTableCard` 和 `EmberMetricCard`；黑名单卡片与操作日志保留页面内实现。
    - `services/web/src/views/admin/MediaQualityView.vue`：已完成公共壳层收口，页头与媒体库筛选区已接入 Ember 基建；报告主体、分布统计、汇总表与抽屉明细继续保留强业务特例实现。
    - `services/web/src/views/console/RenewalCenterView.vue`：已完成局部收口，续费方式 tabs、历史记录 tabs 与历史记录表格壳层已接入 `EmberSegmentTabs` 与 `EmberTableCard`；购买卡片与兑换码区继续保留业务特例实现。
  - 额外探索结论已经明确：
    - 表单基线统一已完成：`form-select`、`form-date` 和 dialog chrome 已收口到 `base.css` 与 `EmberFormDialog`。
    - 控制台页头与 tabs 收口已完成：`LibraryView`、`RankingsView`、`SubscriptionsView` 已复用 `EmberPageHeaderCard` 与 `EmberSegmentTabs`。
    - 空状态组件化已完成：已补 `EmberEmptyStateCard`，并覆盖 `PlaybackProfileContent`、`DashboardView`、`NewSubscriptionView`、`SubscriptionsView`、`RenewalCenterView` 等高重复场景。
    - `PlaybackProfileContent`、`MediaQualityView` 的简单统计卡已统一到 `EmberMetricCard`。
    - 强业务海报卡片不适合现在抽成通用卡片：`SubscriptionsView`、`LibraryView`、`NewSubscriptionView` 的海报卡片交互和信息密度差异较大，继续硬抽只会制造大量布尔分支。
    - `DashboardView`、`AccountCenterView`、`SettingsView`、`TVCalendarView` 继续保持页面级特例，不纳入下一轮 Ember 基建通用层。
    - `services/web/src/components/HelloWorld.vue` 已完成删除，不再作为清理尾项保留。

## 方案设计

### 1. 用户可见行为

- 新增能力：
  - 新增 Ember 基础组件层，供后台与控制台高频页面复用。
  - 后续新页面优先通过基础组件拼装，而不是从空白模板重新写一套结构和样式。
- 修改现有行为：
  - 页面内部实现方式从“view 内直接堆 Tailwind + scoped CSS”改为“基础组件 + 必要 slot + 少量局部样式”。
- 哪些现有行为必须保持不变：
  - 路由结构不变。
  - 接口调用方式不变。
  - 用户可见文案、筛选语义、表格字段、弹窗行为默认保持不变。
  - 现有 `Element Plus` 交互能力继续可用，只统一 Ember 风格外观和骨架。
- 前端约束：
  - 前端实现必须遵守 Ember 风格。
  - 设计与交互基线以 `docs/reference/web-design-guide.md` 为准。
  - 列表页默认采用 `Header Card + Table Card` 骨架，不再在每个页面重新发明第二套布局。
  - 主操作按钮、输入框、筛选区、分页区、弹窗表单必须统一到 Ember 组件层或全局基线。
  - 若存在偏离规范的特例，必须单独写明原因、范围和收口条件。

### 2. 数据与模型

> 本次不涉及数据模型变更。

### 3. 接口与边界

- API / Internal API：
  - 不新增后端 API。
  - 不修改现有请求参数和响应字段。
- Web 组件边界：
  - 在 `services/web/src/components/` 下新增 Ember 基建目录，建议为 `services/web/src/components/ember/`。
  - 目录按职责分层，而不是按页面来源分层：
    - `layout/`
    - `filters/`
    - `data-display/`
    - `forms/`
    - `feedback/`
  - 页面 view 继续保留业务状态、接口调用、路由和数据编排。
  - Ember 基础组件只承载稳定 UI 契约，不侵入业务接口和 store。
- 组件职责建议：
  - `EmberPageHeaderCard`
    - 负责标题、描述、统计 badge、右侧 actions/tabs slot。
  - `EmberFilterPanel`
    - 负责筛选卡片容器、字段区布局、按钮区对齐。
  - `EmberTableCard`
    - 负责表格容器、统一表头样式、分页区和 empty/loading slot。
  - `EmberMetricCard`
    - 负责摘要统计卡的基础结构和视觉基线。
  - `EmberSegmentTabs`
    - 负责支付中心、兑换中心这种页内分段 tabs。
  - `EmberSearchInput`
    - 负责搜索 icon、focus 态、回车触发和可访问标签。
  - `EmberDateField`
    - 负责单日期选择器的 Ember 基线。
  - `EmberDateRangeField`
    - 负责日期范围选择器和统一 popper 样式。
  - `EmberSelectField`
    - 负责带图标或无图标的下拉选择器样式。
  - `EmberFormDialog`
    - 负责通用弹窗表单容器、footer 按钮区和统一内边距。
- 哪些调用方会受影响：
  - 已完成迁移页面：
    - `services/web/src/views/admin/UsersView.vue`
    - `services/web/src/views/admin/PaymentsView.vue`
    - `services/web/src/views/admin/PlaybackHistoryView.vue`
    - `services/web/src/views/admin/RedemptionCodesView.vue`
    - `services/web/src/views/admin/UserPlaybackProfilesView.vue`
    - `services/web/src/views/admin/PaymentCenterView.vue`
    - `services/web/src/views/admin/RedemptionCenterView.vue`
    - `services/web/src/views/admin/RedemptionHistoryView.vue`
    - `services/web/src/views/admin/PlansView.vue`
    - `services/web/src/views/admin/PlanGroupsView.vue`
  - 当前明确保留特例方向的页面：
    - `services/web/src/views/console/TVCalendarView.vue`
    - `services/web/src/views/LoginView.vue`
    - `services/web/src/views/ForgotPasswordView.vue`
    - `services/web/src/views/user/RegisterView.vue`
  - 后续扩展边界已明确：
    - `services/web/src/views/admin/DevicesView.vue`
      - 已完成标准后台列表页收口，不再作为待迁移页面。
    - `services/web/src/views/admin/MediaQualityView.vue`
      - 已完成公共壳层统一，继续保留报告主体特例实现。
    - `services/web/src/views/console/RenewalCenterView.vue`
      - 已完成 tabs 与历史记录表格骨架收口，继续保留购买与兑换核心卡片特例实现。
  - 如后续继续扩展，应复用同一套组件边界到更多控制台页面，而不是新增第二套骨架；本方案内已盘点的追加页面已完成收口判断与实现。

### 3.1 认证页边界结论

- 当前认证页 **不接入** 现有 Ember 后台基建层：
  - `services/web/src/views/LoginView.vue`
  - `services/web/src/views/ForgotPasswordView.vue`
  - `services/web/src/views/user/RegisterView.vue`
- 原因：
  - 现有 `EmberPageHeaderCard`、`EmberFilterPanel`、`EmberTableCard`、`EmberFormDialog`、`EmberSegmentTabs` 是为后台/控制台高频骨架设计，不适合认证页的页面壳层。
  - 认证页当前承担居中卡片、背景氛围、步骤流、验证码/Turnstile 等独立交互，不属于“后台列表页 + 弹窗表单”这套边界。
  - 三个认证页已经共享 `btn-ember`、`input-ember` 和统一视觉基线；继续强接后台基建只会制造错边界，而不是提升复用。
- 收口结论：
  - 认证页继续遵守 Ember 风格，但保留为页面级特例实现。
  - 若后续认证页之间重复继续增大，应单独抽 `auth` 领域组件壳层，而不是污染当前后台基建层。

### 3.2 追加探索页面结论

- `services/web/src/views/admin/DevicesView.vue`
  - 结论：**已完成接入** Ember 基建，不再保留第二套标准后台列表骨架。
  - 原因：
    - 页面同时具备页头、统计卡、筛选区、列表表格和分页，边界与 `UsersView`、`PaymentsView` 这类已迁移后台列表页一致。
    - 设备列表主链路已完成统一收口，只保留黑名单卡片和操作日志这类局部特例。
  - 收口方向：
    - 页头、统计卡、筛选区和主表格已接入 Ember 基建。
    - 黑名单卡片和操作日志区继续保留页面内实现。
- `services/web/src/views/admin/MediaQualityView.vue`
  - 结论：**已完成公共壳层收口**，继续保留报告主体特例实现。
  - 原因：
    - 页面主体是“媒体库报告 + 分布统计 + 汇总表 + 明细抽屉”的复合报告页，不是标准 CRUD 列表页。
    - 强行套用通用列表骨架只会把特例页面做僵，不会提升边界质量。
  - 收口方向：
    - 页头与媒体库筛选壳层已统一到 Ember 基建。
    - 报告主体和抽屉明细结构继续保留页面内实现。
- `services/web/src/views/console/RenewalCenterView.vue`
  - 结论：**已完成局部接入** Ember 基建，不做整页重构。
  - 原因：
    - 购买方案卡片和兑换码输入区属于控制台业务特例，保留页面内实现更符合边界。
    - 续费方式 tabs、历史记录 tabs、支付/兑换记录表格壳层已完成稳定收口。
  - 收口方向：
    - `EmberSegmentTabs` 已用于收口 tabs。
    - `EmberTableCard` 已用于收口历史记录表格和分页区。

### 3.3 后续探索清单

- 已完成：表单基线统一
  - 目标：
    - 将 `form-select`、`form-date`、dialog header/body/footer 的重复样式统一沉到稳定基线。
    - 减少 `UsersView`、`PlansView`、`RedemptionCodesView`、`NewSubscriptionView` 这类页面内重复 scoped CSS。
  - 实际结果：
    - 已补全 `base.css` 中的表单字段全局基线。
    - 已收口 `EmberFormDialog`，将 dialog chrome 变为默认能力。
- 已完成：控制台页头与 tabs 继续统一
  - 目标：
    - 继续扩展 `EmberPageHeaderCard` 与 `EmberSegmentTabs` 的覆盖面，减少控制台内第二套头部和 tabs 写法。
  - 实际落地页面：
    - `services/web/src/views/console/LibraryView.vue`
    - `services/web/src/views/console/RankingsView.vue`
    - `services/web/src/views/console/SubscriptionsView.vue`
  - 补充说明：
    - `services/web/src/views/admin/SessionsView.vue` 仍可只收页头，不建议强套列表页骨架。
- 已完成：补轻量 empty state 组件
  - 目标：
    - 为 dashed empty state 提供统一容器，减少重复 Tailwind 组合。
  - 实际覆盖位置：
    - `services/web/src/components/profile/PlaybackProfileContent.vue`
    - `services/web/src/views/console/DashboardView.vue`
    - `services/web/src/views/console/NewSubscriptionView.vue`
    - `services/web/src/views/console/SubscriptionsView.vue`
    - `services/web/src/views/console/RenewalCenterView.vue`
- 已完成：统计卡继续复用 `EmberMetricCard`
  - 目标：
    - 把已共享页面内部仍手写的简单统计卡继续统一到现有组件。
  - 实际落地位置：
    - `services/web/src/components/profile/PlaybackProfileContent.vue`
    - `services/web/src/views/admin/MediaQualityView.vue`
- 已完成：局部对话框统一
  - 目标：
    - 处理 `NewSubscriptionView` 这类还保留裸 `el-dialog` 和局部下拉样式的页面。
  - 边界：
    - 保留业务内容高度定制，但 dialog chrome 和字段样式已不再页面私有化。
- 明确保留特例，不继续抽象：
  - `services/web/src/views/console/DashboardView.vue`
  - `services/web/src/views/console/AccountCenterView.vue`
  - `services/web/src/views/admin/SettingsView.vue`
  - `services/web/src/views/console/TVCalendarView.vue`
  - `services/web/src/views/console/SubscriptionsView.vue` / `services/web/src/views/console/LibraryView.vue` / `services/web/src/views/console/NewSubscriptionView.vue` 的海报卡片主体
  - 原因：
    - 这些页面要么页面级编排过重，要么视觉和交互边界差异大，不适合收成单一通用组件。

### 4. 关键流程

按顺序写清主链路，不需要贴代码：

1. 先把当前重复的页面骨架、筛选样式、表格外壳和弹窗表单整理成稳定契约，而不是直接复制现有 view。
2. 在 `services/web/src/components/ember/` 内建立基础组件目录和组件边界。
3. 优先收口全局样式基线：
   - 保留 `base.css` 中的 token、按钮、输入框、日期范围 popper 等稳定基础能力。
   - 把重复出现在多个页面中的筛选输入、日期、选择器样式迁移到基础组件或全局可复用类。
4. 先改造高重复后台页面：
   - `UsersView`
   - `PaymentsView`
   - `PlaybackHistoryView`
   - `RedemptionCodesView`
   - 当前已完成以上 4 个页面，并额外完成 `UserPlaybackProfilesView`
5. 验证基础组件是否足够覆盖：
   - 单关键词筛选
   - 多字段筛选
   - 单日期/日期范围
   - 表格 + 分页
   - 弹窗表单
   - 当前已在上述已迁移页面中完成覆盖验证
6. 再扩展到剩余后台页面，并收口重复的 tabs、metric cards、empty states。
   - 当前已完成 `PaymentCenterView`、`RedemptionCenterView`、`RedemptionHistoryView`、`PlansView`、`PlanGroupsView`
7. 最后清理失效的 scoped CSS、重复 class 组合和不再需要的局部样式类。
   - 当前后台高重复页面、控制台页头 / tabs、表单基线、empty state、统计卡与前端残留清理均已完成。
   - 本方案内列出的收口项已经完成，不再继续扩展本方案主体。
   - 若后续继续推进其他页面，仅按新问题另开扩展计划。

### 5. 失败路径与边界条件

- 组件抽象过头：如果一个组件开始依赖大量布尔开关和分支拼接，说明边界错了，应拆成更小的基础组件，而不是继续堆参数。
- 强业务页面误抽为通用层：`TVCalendarView`、部分高度定制首页模块应优先做页面内拆分，不直接并入基础组件层。
- 与 Element Plus 冲突：基础组件只覆盖 Ember 明确需要统一的外观层，不重复包裹所有 Element Plus 能力。
- 迁移中断：允许新旧写法短期并存，但必须明确“谁已经迁移、谁还未迁移”，避免半套基建长期悬空。
- 兼容性约束：
  - 不能改变现有页面职责和路由归属。
  - 不能为了组件化引入另一套视觉语言。
  - 不能把 view 内的业务逻辑搬进通用组件，导致边界倒置。

## 影响范围

涉及的子系统：

- API：无
- Web：有
  - 新增 Ember 组件基建目录
  - 重构后台与控制台部分页面结构
  - 收口 `base.css` 与局部页面样式职责
- Bot：无
- 配置/部署：无
- 文档：已更新
  - `docs/system-architecture.md`
  - `docs/reference/web-design-guide.md`

## 验证方式

### 编译/测试

- `cd services/web && npm run build`
- 已于 2026-04-15 再次执行，当前构建通过。

按改动补充针对性验证：

- 基础组件渲染与 slot 组合
- 关键页面迁移后是否仍可正常编译
- 日期选择器、下拉选择器、弹窗表单样式是否仍符合 Ember 基线

### 手工验证

- 用户管理页迁移后，筛选区、表格、分页和新建/编辑弹窗行为保持不变。
- 支付记录页迁移后，带图标的筛选输入、下拉框和表头样式保持一致。
- 播放历史页迁移后，日期范围筛选与表格容器不出现尺寸漂移或双层边框。
- 兑换码页迁移后，复杂筛选区和多个弹窗表单仍能复用同一套 Ember 组件骨架。
- 用户画像总览页迁移后，统计卡、日期范围和列表骨架继续遵守 Ember 风格。
- 支付中心、兑换中心迁移后，页内 tabs 切换统一落到 `EmberSegmentTabs`，不再保留第二套手写标签容器。
- 兑换历史、付费方案、套餐分组迁移后，header / table / dialog 骨架继续保持原行为与原字段语义。
- 认证页继续保留页面级特例实现，但维持 `btn-ember`、`input-ember` 和 Ember 视觉基线，不引入第二套表单语言。
- `DevicesView` 已完成标准后台列表页收口，且保留黑名单卡片与操作日志特例边界。
- `RenewalCenterView` 已完成 tabs 与历史记录表格骨架收口，且保留购买卡片与兑换码区特例边界。
- `MediaQualityView` 已完成页头与媒体库筛选壳层收口，报告主体继续保留页面内特例实现。
- `LibraryView`、`RankingsView`、`SubscriptionsView` 已完成页头与 tabs 收口。
- 表单基线统一、empty state 组件化、统计卡统一和目录清理已完成。

## 落地后文档处理

落地后应同步处理：

- 已同步 `docs/system-architecture.md`
  - `services/web` 组件目录边界
  - Web 共享组件层职责
- 已同步 `docs/reference/web-design-guide.md`
  - 页面骨架、筛选控件、表单容器命名和使用规则
- 本方案内已补齐认证页“保留特例”结论，并补齐 `DevicesView`、`MediaQualityView`、`RenewalCenterView` 的页面边界结论。
- 归档结论：
  - 本方案主体已经完成，且本轮盘点的后续收口项已全部实际落地。
  - 后续若继续推进其他控制台或后台页面的接入，应新开扩展计划，不再继续堆叠到本方案。
  - 当前可以直接移入 `docs/archive/plan/console-admin/`。
