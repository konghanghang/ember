# Ember 前端设计规范

本规范用于统一 Ember（Vue 3 + Tailwind + Element Plus）的视觉与交互，优先解决“跨页面一致性”和“可维护性”。

## 1. 核心原则

- 一致性优先：同类页面必须共享同一布局骨架、按钮语义、输入反馈。
- 信息分层：标题区、筛选区、数据区分层，不混杂。
- 实用主义：避免重复操作与装饰性复杂度，优先可读、可扫、可点。

---

## 2. 视觉基线

### 2.1 色彩

- 品牌主色：`ember`（`#E50914`），用于主按钮和焦点强调。
- 页面底色：`bg-gray-50`。
- 内容容器：`bg-white`。
- 状态色：成功绿、警告黄、错误红、信息蓝。

### 2.2 字体与文本

- 字体：`Plus Jakarta Sans`（代码字体 `JetBrains Mono`）。
- 页面标题：`text-2xl font-bold text-gray-900`。
- 说明文案：`text-sm text-gray-500/600`。

### 2.3 尺寸常量（推荐）

- 输入高度：`42px`
- 圆角：卡片 `rounded-2xl`，输入/按钮 `rounded-xl`
- 边框：`border-gray-100/200`
- 焦点光圈：`ring-4` + `ember/10`

---

## 3. 基础组件规范

### 3.1 卡片（Card）

```html
<div class="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden"></div>
```

- 不使用厚重边框；通过留白和层级区分信息。
- 操作卡片默认使用白底容器（`bg-white` + `border-gray-100`），通过标题、标签、按钮和局部强调色表达重点。
- 非状态型信息区不要使用大面积深色整卡（如整块黑底）作为默认强调方式，避免同一页面出现第二套视觉语义。

### 3.2 按钮（Button）

- 主操作：`.btn-ember`
- 次操作：白底描边（`bg-white border border-gray-200`）
- 同一页面的主操作按钮语义必须统一，默认全部使用品牌主色体系（`ember`）。
- 禁止在同一页面同时混用“品牌主按钮”和无明确语义来源的黑色主按钮。
- 若一个区块存在“主操作 + 次操作”，主操作使用 `.btn-ember`，次操作使用白底描边，不反转语义。
- 页面级主 CTA、表单提交、确认保存优先使用 `.btn-ember`。
- 图标操作按钮、筛选区重置按钮、返回按钮等局部动作使用 Tailwind 内联样式，不强行套 `.btn-ember`。
- 深色或高强调背景中的反色按钮属于局部特例，可以保留白底样式，但不能扩散成通用主按钮基线。
- 图标按钮必须有 `aria-label`
- 可点击元素必须有 `cursor-pointer`

### 3.3 输入（Input）

- 默认：浅灰底 + 细边框
- Focus：白底 + 品牌色边框/光圈
- 输入框不得仅靠 placeholder 传递语义，必须有可见 label（筛选区强制）
- 基于 Element Plus 的输入框，边框、圆角、焦点光圈统一落在 `.el-input__wrapper` 层，不要再给内部 `input` 元素额外添加边框或阴影，避免出现双层输入框。
- `input-ember` 这类输入样式类，只负责统一外层容器表现；内部 `input` 应保持透明、无边框、无额外阴影。

---

## 4. 列表页标准骨架（必须）

管理后台列表页统一采用两段结构：

1. `Header Card`：页面标题 + 统计 + 筛选工具条  
2. `Table Card`：表格 + 分页条

示例骨架：

```html
<div class="space-y-6">
  <div class="bg-white p-6 rounded-2xl border border-gray-100 shadow-sm">...</div>
  <div class="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden">...</div>
</div>
```

---

## 5. 搜索与筛选规范（重点）

### 5.1 搜索输入模式

- 统一使用 `group-focus-within` 驱动 icon 与输入焦点态。
- icon 必须 `pointer-events-none`，避免遮挡点击。
- 必须提供 `aria-label`。
- 搜索框是页面级交互入口，保持自定义 Tailwind 写法，不要强行统一为 `.input-ember`。
- 日期类筛选控件也按同一视觉基线处理：`42px` 高度、`rounded-xl` 圆角、默认浅灰底 + inset 细边框、focus 时使用 `ember` 边框和光圈。
- 单日期选择器使用紧凑宽度，宽度只覆盖 `YYYY-MM-DD` 这类单值展示，不要做成接近日期范围选择器的长度。
- 日期范围选择器必须比单日期更宽，并完整展示开始/结束日期；不要把单日期和日期范围混用同一宽度。
- 自定义日期图标时，禁止和组件内置图标并存，避免出现双图标或重复占位。

### 5.2 筛选工具条模式

适用于用户管理、兑换历史、支付记录等列表页。

- 标题在上，筛选卡片在下，禁止左右并排导致大面积空白。
- 筛选卡片默认一层：筛选字段 + 操作按钮（查询/重置）。
- “已生效筛选标签”是可选增强，不是默认要求。
- 筛选按钮组统一右对齐：
  - 桌面端右对齐
  - 移动端换行后也保持按钮组靠右
  - 顺序固定：左 `重置`，右 `查询`
- 若存在“查询”，默认不再放“刷新”（避免重复语义）。

### 5.3 条件数量分级

- `1` 个条件：紧凑模式（字段固定宽度 + 右侧按钮）
- `2~4` 个条件：网格模式（`grid-cols-1 md:grid-cols-2 2xl:grid-cols-4`）
- `5~8` 个条件：主区保留高频，其他放“更多筛选（折叠）”
- `>8` 个条件：使用“高级筛选抽屉/弹层”

### 5.4 已生效筛选反馈（可选）

- 默认不展示该区块，避免筛选区视觉噪音。
- 当筛选条件较多、用户容易忘记当前条件时，可开启该反馈。
- 若开启，建议支持“清空单项”和“重置全部”。

### 5.5 可复制模板（Vue + Tailwind）

用于后台列表页筛选区的标准实现。默认按钮右对齐，兼容单条件和多条件扩展。

```vue
<template>
  <div class="mt-4 rounded-2xl border border-gray-200 bg-gray-50/60 p-3 md:p-4">
    <div class="flex flex-col xl:flex-row xl:items-end gap-3">
      <!-- 字段区：单条件时可用 lg:max-w-md，2~4 条件用 grid-cols-2/4 -->
      <div class="grid grid-cols-1 md:grid-cols-2 2xl:grid-cols-4 gap-3 flex-1">
        <div class="space-y-1.5">
          <label class="text-xs font-semibold tracking-wide text-gray-500">关键词</label>
          <div class="relative w-full group">
            <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
              <el-icon class="text-gray-400 group-focus-within:text-ember transition-colors"><Search /></el-icon>
            </div>
            <input
              v-model="filters.keyword"
              type="search"
              inputmode="search"
              autocomplete="off"
              aria-label="按关键词筛选"
              placeholder="输入关键词"
              class="filter-input w-full pl-10 pr-4"
              @keyup.enter="handleSearch"
            />
          </div>
        </div>

        <div class="space-y-1.5">
          <label class="text-xs font-semibold tracking-wide text-gray-500">到期晚于</label>
          <div class="relative w-full group">
            <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none z-10">
              <el-icon class="text-gray-400 group-focus-within:text-ember transition-colors"><Calendar /></el-icon>
            </div>
            <el-date-picker
              v-model="filters.expiresAfter"
              type="date"
              value-format="YYYY-MM-DD"
              placeholder="选择日期"
              clearable
              class="w-full filter-date"
              @change="handleSearch"
            />
          </div>
        </div>
      </div>

      <!-- 按钮区：统一右对齐 -->
      <div class="flex items-center gap-2 self-end xl:ml-auto xl:shrink-0">
        <button
          @click="handleReset"
          class="px-4 py-2.5 text-sm text-gray-700 bg-white border border-gray-200 hover:bg-gray-100 rounded-xl transition-colors cursor-pointer"
        >
          重置
        </button>
        <button
          @click="handleSearch"
          class="btn-ember px-4 py-2.5 text-sm rounded-xl font-semibold shadow-sm hover:shadow-md active:scale-[0.99] cursor-pointer inline-flex items-center gap-1.5"
        >
          <el-icon><Search /></el-icon>
          查询
        </button>
      </div>
    </div>

  </div>
</template>

<style scoped>
.filter-input {
  background-color: #f9fafb;
  border: 1px solid #e5e7eb;
  border-radius: 0.75rem;
  height: 42px;
  line-height: 1.2;
  font-size: 0.875rem;
  color: #111827;
  outline: none;
  transition: all 0.2s ease;
}

.filter-input::placeholder {
  color: #9ca3af;
}

.filter-input:hover {
  background-color: #ffffff;
}

.filter-input:focus {
  background-color: #ffffff;
  border-color: var(--ember-red);
  box-shadow: 0 0 0 4px rgba(229, 9, 20, 0.1);
}

:deep(.filter-date .el-input__wrapper) {
  height: 42px;
  min-height: 42px;
  background-color: #f9fafb !important;
  border-radius: 0.75rem;
  box-shadow: 0 0 0 1px #e5e7eb inset !important;
  transition: all 0.2s ease;
}

:deep(.filter-date .el-input__wrapper.is-focus) {
  background-color: #ffffff !important;
  box-shadow:
    0 0 0 1px var(--ember-red) inset,
    0 0 0 4px rgba(229, 9, 20, 0.1) !important;
}

:deep(.filter-date .el-input__inner) {
  height: 100%;
  padding-left: 2.5rem;
  font-size: 0.875rem;
}
</style>
```

模板使用说明：

- `1` 个条件：将字段区容器改为 `w-full lg:max-w-md`，避免大面积空白。
- 若已有“查询”，不要再放“刷新”按钮。
- 新增条件优先追加到字段区，不先加按钮。
- “已生效筛选”默认关闭，仅在复杂筛选场景按需开启。
- 单日期选择器优先用于“某一天 / 某一周 / 某一月”的单值查询；日期范围选择器只用于真正需要开始时间和结束时间的场景。
- 单日期和日期范围都必须与普通输入框保持同样的圆角、边框阴影和高度，不允许因为组件默认样式而出现直角、双层边框或尺寸漂移。

---

## 6. 表格与分页规范

- 表头：`bg-gray-50` + 中灰文本 + 中等字重。
- 表格容器：`overflow-hidden`，避免边角溢出。
- 分页条：底部独立区域，`border-t border-gray-100 bg-gray-50/50 p-6`。
- 推荐分页布局：`total, sizes, prev, pager, next, jumper`。

---

## 7. 导航与侧边栏规范

- 桌面端控制台导航采用固定侧边栏：`w-64`、白底、右边框、轻阴影。
- 移动端导航采用抽屉覆盖层，在 `lg` 以下显示遮罩与侧滑面板，不做第二套独立导航结构。
- 导航项统一使用 `px-3 py-2.5 rounded-lg`，默认态为 `text-gray-600 hover:bg-gray-50 hover:text-gray-900`。
- 激活态统一使用 `bg-ember/10 text-ember font-medium`，允许保留左侧红色指示条作为辅助强调。
- 所有导航 `router-link`、菜单开关按钮、关闭按钮都必须显式带 `cursor-pointer`。
- 图标无文字时必须提供 `aria-label`；带可见标题的导航项可不重复补 `aria-label`。

## 8. 对话框、徽章与空状态

### 8.1 对话框

- 常用宽度基线：
  - 小型确认：`400px`
  - 常规编辑：`520px`
  - 复杂表单：`680px`
- 默认使用 `align-center`，容器圆角保持 `rounded-2xl`。
- 表单优先 `label-position="top"`，避免左对齐标签压缩横向空间。
- 开关、状态切换等组合控件优先放进 `border border-gray-200 rounded-xl bg-gray-50` 容器，避免裸露控件漂浮。

### 8.2 徽章与状态标签

- 状态标签优先使用浅底语义色，不直接用高饱和纯色块作为默认态。
- 推荐映射：
  - 成功/有效：`bg-green-50 text-green-700`
  - 警告/待处理：`bg-amber-50 text-amber-700`
  - 错误/过期：`bg-red-50 text-red-700`
  - 信息/中性：`bg-sky-50 text-sky-700` 或 `bg-gray-100 text-gray-600`
- 若标签承担更强状态提示，可追加细边框：`border border-{color}-100`。
- 允许搭配状态点或图标，但颜色不能成为唯一状态表达。

### 8.3 空状态

- 空状态优先放在白底卡片或白底区块内，不把 `el-empty` 直接裸放在页面底色上。
- 默认结构：容器 + `el-empty` + 一句补充说明；需要引导动作时再增加次要按钮。
- 空状态文案保持克制，先说明“当前没有什么”，再说明“下一步能做什么”。

## 9. 响应式策略

- 断点基线：
  - 默认：手机单列
  - `md`：双列或更宽表头布局
  - `lg`：桌面控制台，侧边栏固定展开
  - `xl`：更高密度网格和更宽内容区
- 页头、筛选区、操作区默认按“移动优先”书写：先纵向堆叠，再在 `md`/`lg` 做横向展开。
- 控制台布局只在 `lg` 切换侧边栏模式；不要在多个断点频繁切换信息架构。
- 数据卡片、海报网格、统计块等组件，优先通过列数变化适配，而不是缩小到难以点击的尺寸。
- 移动端抽屉、下拉、弹层必须保证关闭入口始终可见。

## 10. 无障碍与动效

- 必须尊重 `prefers-reduced-motion`，并提供全局兜底，而不是只靠单个组件自觉处理。
- 图标按钮必须提供 `aria-label`。
- 可点击元素必须有 `cursor-pointer`，避免交互性表达依赖猜测。
- 禁止 emoji 作为 UI 图标；统一使用组件图标、SVG、CSS 徽章或文字。
- 颜色不是唯一状态表达，必须配合文字、图标、标签或位置关系。
- 不要移除焦点可见性；若自定义样式覆盖默认焦点环，必须提供等价替代。
- 移动端菜单开关、关闭按钮等核心操作的无障碍文案应与界面语言一致。

## 11. 反模式（禁止）

- 标题与筛选并排且造成大面积留白
- 单筛选项强行使用多列布局导致画面松散
- 同一筛选区同时出现“查询 + 刷新”重复动作
- 无 label，仅靠 placeholder 的筛选输入
- 同类页面按钮对齐方式不一致
- 侧边栏、抽屉、图标按钮缺失可点击态或无障碍标注
- 把 proposal 里的临时决策长期当成现行规范，而不提炼进 `docs/reference/`

## 12. 页面开发检查清单

1. 是否使用统一列表骨架（Header Card + Table Card）？
2. 筛选输入是否有可见 label、统一高度（42px）？
3. 查询/重置按钮是否右对齐且顺序固定？
4. 如场景复杂，是否需要展示“已生效筛选”并支持清空？
5. 是否删除了重复语义操作（例如刷新）？
6. 表格/分页是否采用统一容器样式？
7. 是否通过移动端与桌面端断点检查？
8. 侧边栏、抽屉、顶部操作区是否遵守同一套导航和关闭模式？
9. 同一页面的主操作按钮、推荐操作区、表单提交按钮是否保持统一颜色语义？
10. 对话框宽度、表单布局、状态标签是否符合当前规范，而不是各写一套？
11. 图标按钮是否补了 `aria-label`，可点击元素是否补了 `cursor-pointer`？
12. 是否错误使用大面积深色卡片制造强调，导致页面出现第二套视觉系统？
