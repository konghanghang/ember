# 控制台与后台 UI 一致性收口优化方案

> 状态：进行中
> 负责人：Ember
> 更新时间：2026-04-20

## 背景

这个问题为什么现在要解决：

- Ember Web 已经建立了 `components/ember/` 基础组件层和 `docs/reference/web-design-guide.md` 设计规范，但控制台与后台页面的落地程度不一致，同类页面仍存在多套骨架和交互语义。
- 当前前端问题主要集中在页头、筛选区、表格分页、弹窗和可达性细节，没有形成“缺少设计系统”的问题，而是“已有规范没有收口到位”。
- 如果继续在页面内局部手写样式和骨架，后续每次微调 UI 都要逐页返工，维护成本会持续上升。

## 目标

本方案要实现：

1. 将控制台与后台高频页面统一收口到 Ember 前端基础组件和现有设计规范，减少重复骨架与样式分叉。
2. 优先修复真实影响使用体验的 UI 问题，尤其是 hover 依赖、交互可达性、分页与空状态不一致。
3. 在不改动业务接口和既有页面职责的前提下，形成稳定、可重复应用的后台页面基线。

## 非目标

本次明确不做：

- 不重做首页、营销页或品牌视觉，不进行整站配色和风格重设计。
- 不新增新的前端状态管理方案、路由结构或第二套基础组件体系。
- 不因为 UI 收口去改动现有 API 契约、字段结构或后端业务流程。
- 不追求一次性清理所有页面，只优先覆盖高频后台与控制台页面。

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：
  - `docs/system-architecture.md`
  - `docs/reference/web-design-guide.md`
- 相关服务/页面/模型：
  - `services/web/src/components/ember/layout/EmberPageHeaderCard.vue`
  - `services/web/src/components/ember/layout/EmberFilterPanel.vue`
  - `services/web/src/components/ember/data-display/EmberTableCard.vue`
  - `services/web/src/components/ember/forms/EmberFormDialog.vue`
  - `services/web/src/components/ember/filters/EmberSearchInput.vue`
  - `services/web/src/components/ember/filters/EmberSelectField.vue`
  - `services/web/src/components/ember/filters/EmberDateField.vue`
  - `services/web/src/components/ember/filters/EmberDateRangeField.vue`
  - `services/web/src/views/admin/SessionsView.vue`
  - `services/web/src/views/admin/SettingsView.vue`
  - `services/web/src/views/admin/MediaQualityView.vue`
  - `services/web/src/views/console/SubscriptionsView.vue`
  - `services/web/src/views/console/NewSubscriptionView.vue`
  - `services/web/src/assets/base.css`
- 当前行为：
  - 后台与控制台部分页面已经接入 Ember 基础组件，但仍有页面在 view 内手写页头、筛选区、表格容器、分页区和弹窗。
  - `SubscriptionsView` 的主要操作一度依赖 hover 浮层暴露，移动端和键盘用户可发现性较弱。
  - 输入、下拉、日期筛选控件同时存在 `base.css` 全局样式和基础组件局部样式两套实现。
- 已完成项：
  - `SessionsView` 已切到 `EmberPageHeaderCard` + `EmberEmptyStateCard`，并补充刷新按钮与进度条语义。
  - `SubscriptionsView` 已移除 hover-only 关键操作，改为卡片底部常驻操作区。
  - `NewSubscriptionView` 已接入 `EmberPageHeaderCard`、`EmberSearchInput`、`EmberSegmentTabs`、`EmberFormDialog`。
  - `MediaQualityView` 已将汇总表格与分页区收进 `EmberTableCard`，并补齐图片替代文本与操作按钮语义。
  - `SettingsView` 已将页头收进 `EmberPageHeaderCard`，并为左侧分组切换补充 tab / tabpanel 语义。
  - `base.css` 已抽出共享字段 token，用于统一高度、圆角、背景和 focus ring。
  - `2026-04-20` 已完成本轮修改文件系统性审查，并执行 `cd services/web && npm run build` 验证构建通过。
  - `MediaQualityView` 已通过 `EmberTableCard` header slot 补回“低画质清单（汇总）”主表标题语义。
  - `SettingsView` 已补 roving tabindex、方向键切换和 `Home / End` 快捷键，左侧分组 tabs 键盘交互闭环。
  - `EmberSegmentTabs` 已收口为基于 `radiogroup / radio` 的单选分段控件，补齐 roving tabindex、左右方向键和 `Home / End`，并要求调用点补业务化 `aria-label`。
  - `SessionsView` 与 `NewSubscriptionView` 已拆开失败态和空态，并移除页面级重复错误提示，避免一次失败被多路重复渲染。
  - `NewSubscriptionView` 搜索链路已补 latest-only 保护，旧请求不再覆盖新查询结果或错误态。
  - `SubscriptionsView` 已区分“全量为空”和“筛选后为空”的空态文案与动作。
  - `EmberSearchInput`、`EmberSelectField`、`EmberDateField`、`EmberDateRangeField` 已改为消费共享 field token，降低组件 scoped CSS 与 `base.css` 的样式漂移风险。
  - `docs/reference/web-design-guide.md` 与 `docs/system-architecture.md` 已同步本轮稳定结论。
- 剩余项：
  - `SettingsView` 内部字段区仍保留局部输入样式，尚未进一步和全局表单基线完全收口。
  - 组件样式已改为消费共享 token，但筛选基础组件仍保留 scoped CSS 承载结构性细节，后续要继续评估是否还能进一步下沉到统一基线。
- 现有限制：
  - 同类页面的视觉节奏和交互反馈已有明显收口，但基础组件和全局样式仍未完成单一来源治理。
  - 某些复杂页面内部字段布局仍然保留历史实现，后续继续收口时要避免误伤业务表单逻辑。
  - 当前只完成了静态审查和构建验证，移动端、键盘导航和真实图片失败场景还没有做浏览器级手工走查。

## 方案设计

### 1. 用户可见行为

- 新增能力：
  - 控制台与后台高频页面统一使用同一套页头、筛选区、表格分页和空状态骨架。
  - 关键操作在触屏、键盘和鼠标场景下都可被明确发现和触发。
  - 页面中的输入、下拉、日期筛选控件保持一致的高度、圆角、边框与焦点态。
- 修改现有行为：
  - 将仍在页面内手写的通用骨架替换为 Ember 基础组件。
  - 将依赖 hover 的操作区改为常驻或可聚焦显式入口。
- 哪些现有行为必须保持不变：
  - 页面路由归属、业务功能、接口调用和查询条件语义保持不变。
  - 后台与控制台现有主流程不因 UI 收口而改变。
  - 特例页面如首页重视觉模块和高度定制页面可以保留局部实现，但不能扩散为通用基线。
- 前端约束：
  - 前端实现必须遵守 Ember 风格。
  - 设计与交互基线以 `docs/reference/web-design-guide.md` 为准。
  - 后台列表页默认遵守“Header Card + Filter Panel + Table Card + Pagination”骨架。
  - 若存在偏离规范的特例，必须单独写明原因、范围和收口条件。

### 2. 数据与模型

> 本次不涉及数据模型变更。

### 3. 接口与边界

- 新增或修改哪些 API / Internal API / webhook / 命令：
  - 本次不新增 API，不修改现有接口结构。
- 请求参数与响应字段怎么变：
  - 不变。
- 哪些调用方会受影响：
  - `services/web/src/views/admin/` 下的高频后台页面。
  - `services/web/src/views/console/` 下的高频控制台页面。
  - `services/web/src/components/ember/` 基础组件层。
- 边界约束：
  - 基础组件层只负责骨架、样式与交互基线，不承载业务查询逻辑。
  - 页面内保留业务编排、数据请求、筛选状态和表格列定义。
  - `base.css` 只保留稳定的全局 token 与通用基线，不再和基础组件重复定义同类控件外观。

### 4. 关键流程

按顺序写清主链路，不需要贴代码：

1. 先按页面职责梳理后台与控制台高频页面，区分“必须收口的重复骨架”和“允许保留的业务特例”。
2. 将仍在 view 内手写的页头、筛选区、表格分页和标准弹窗优先替换为 Ember 基础组件。
3. 修复关键页面的可达性问题，包括 hover-only 操作、图标按钮语义、焦点态和按钮语义属性。
4. 收敛输入、下拉、日期筛选等控件的样式来源，避免全局样式和组件局部样式重复维护。
5. 对典型列表页、表单弹窗页和控制台卡片页做回归验证，确认页面职责、查询行为和操作链路未被破坏。
6. 主体稳定后，将形成的稳定结论回写到设计规范和系统架构文档。

### 5. 失败路径与边界条件

- 页面存在高度定制布局：允许保留页面内实现，但必须明确它不是新的通用基线。
- 基础组件抽象无法覆盖局部场景：优先扩展现有 Ember 组件的 slot 或配置项，不新发明第二套骨架。
- hover 操作改为常驻后出现信息拥挤：优先通过次级按钮、菜单按钮或折叠菜单收口，而不是回退到 hover-only。
- 样式收口后局部页面视觉出现偏差：以 `docs/reference/web-design-guide.md` 为准逐页校正，不允许在页面内继续覆盖出第三套样式。
- 兼容性约束：
  - 不得破坏现有筛选条件、分页逻辑、查询触发条件和弹窗操作流程。
  - 不得因样式收口引入新的交互歧义，例如按钮主次语义反转、输入框双层边框、日期组件双图标。

## 影响范围

涉及的子系统：

- API：无
- Web：有
  - `components/ember/` 基础组件层
  - `views/admin/` 与 `views/console/` 的高频页面
  - `src/assets/base.css` 全局样式基线
- Bot：无
- 配置/部署：无
- 文档：需要更新
  - `docs/system-architecture.md`（若基础组件层职责边界或前端结构描述发生稳定变化）
  - `docs/reference/web-design-guide.md`（若沉淀出新的稳定规范或禁止项）

## 验证方式

### 编译/测试

- [x] `cd services/web && npm run build`

按改动补充针对性检查：

- [x] 控制台与后台典型页面的类型检查与构建通过
- [x] 基础组件替换后不存在未消费 slot 或模板编译错误

### 手工验证

本轮已完成静态与构建层验证，待下一轮继续补充页面级手工走查：

- [ ] 后台列表页在桌面端保持统一的页头、筛选区、表格和分页节奏。
- [ ] 典型控制台页面在移动端与桌面端都能发现和触发关键操作，不依赖 hover 才可见。
- [ ] 图标按钮、整卡点击、搜索框和筛选控件具备明确焦点态和必要语义。
- [x] `SettingsView` 左侧分组在键盘场景下支持 roving tabindex、方向键切换和 Home / End 快捷键。
- [x] `MediaQualityView` 主表保留明确的区块标题或等价标题语义，不出现无标题主表卡片。
- [ ] 输入、下拉、日期控件在不同页面中保持相同高度、圆角、边框与焦点光圈。
- [ ] 弹窗页在取消、确认、禁用态和加载态下保持统一 footer 节奏和按钮语义。

### 本轮审查结论

- `P0 / P1`
  - 本轮未发现会阻断合并的致命问题。
- `P2`
  - 本轮修复后，`MediaQualityView` 主表无标题、`SettingsView` 键盘交互不完整、`EmberSegmentTabs` 共享语义模型、`SessionsView` / `NewSubscriptionView` 假空态、重复报错、搜索竞态，以及 `SubscriptionsView` 筛选空态误导都已收口。
- `P3`
  - 基础筛选组件虽已切到共享 token，但 scoped CSS 仍保留结构性样式，后续还可继续压缩样式来源。
  - 本轮缺少浏览器级手工走查，移动端与真实键盘路径仍需补验证。

## 落地后文档处理

落地后应同步处理：

- 将稳定结论同步到 `docs/reference/web-design-guide.md`
  - 哪些后台页面必须优先复用 Ember 基础组件
  - hover-only 操作的限制与替代方案
  - 表单与筛选控件样式来源的唯一基线
- 若 `docs/system-architecture.md` 中的前端结构描述发生稳定变化，同步更新基础组件层职责说明。
- 归档条件：
  - `SettingsView` 内部字段区样式与基础表单基线完成收口。
  - 基础筛选组件与 `base.css` 的样式来源继续收敛到更稳定的单一来源。
  - 关键页面完成手工走查并确认无明显交互回归。
- 满足上述条件后移入 `docs/archive/plan/console-admin/`。
