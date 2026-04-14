# Ember Web 组件基建收口方案

> 状态：草稿
> 负责人：Ember
> 更新时间：2026-04-14

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

## 方案设计

### 1. 用户可见行为

- 新增能力：
  - 新增 Ember 基础组件层，供后台、控制台和认证页复用。
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
  - 首批改造页面：
    - `services/web/src/views/admin/UsersView.vue`
    - `services/web/src/views/admin/PaymentsView.vue`
    - `services/web/src/views/admin/PlaybackHistoryView.vue`
    - `services/web/src/views/admin/RedemptionCodesView.vue`
  - 第二批复用页面：
    - `services/web/src/views/admin/RedemptionHistoryView.vue`
    - `services/web/src/views/admin/UserPlaybackProfilesView.vue`
    - `services/web/src/views/admin/PlansView.vue`
    - `services/web/src/views/admin/PlanGroupsView.vue`
    - `services/web/src/views/admin/PaymentCenterView.vue`
    - `services/web/src/views/admin/RedemptionCenterView.vue`
    - `services/web/src/views/LoginView.vue`
    - `services/web/src/views/ForgotPasswordView.vue`
    - `services/web/src/views/user/RegisterView.vue`

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
5. 验证基础组件是否足够覆盖：
   - 单关键词筛选
   - 多字段筛选
   - 单日期/日期范围
   - 表格 + 分页
   - 弹窗表单
6. 再扩展到剩余控制台/后台页面，并收口重复的 tabs、metric cards、empty states。
7. 最后清理失效的 scoped CSS、重复 class 组合和不再需要的局部样式类。

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
- 文档：需要更新
  - `docs/system-architecture.md`
  - 如组件命名、目录约定和页面骨架成为长期规则，应补充到 `docs/reference/web-design-guide.md`

## 验证方式

### 编译/测试

- `cd services/web && npm run build`

按改动补充针对性验证：

- 基础组件渲染与 slot 组合
- 关键页面迁移后是否仍可正常编译
- 日期选择器、下拉选择器、弹窗表单样式是否仍符合 Ember 基线

### 手工验证

- 用户管理页迁移后，筛选区、表格、分页和新建/编辑弹窗行为保持不变。
- 支付记录页迁移后，带图标的筛选输入、下拉框和表头样式保持一致。
- 播放历史页迁移后，日期范围筛选与表格容器不出现尺寸漂移或双层边框。
- 兑换码页迁移后，复杂筛选区和多个弹窗表单仍能复用同一套 Ember 组件骨架。
- 登录、注册、忘记密码页如接入基础表单组件后，视觉保持 Ember 风格，不引入第二套表单语言。

## 落地后文档处理

落地后应同步处理：

- 将稳定结论同步到 `docs/system-architecture.md`
  - `services/web` 组件目录边界
  - Web 共享组件层职责
- 将长期有效的页面骨架、筛选控件、表单容器命名和使用规则补充到 `docs/reference/web-design-guide.md`
- 这份方案在以下条件满足后移入 `docs/archive/plan/console-admin/`：
  - Ember 基础组件目录已建立
  - 首批高重复页面已完成迁移
  - 失效 scoped CSS 已完成清理
  - 现行文档已同步更新
