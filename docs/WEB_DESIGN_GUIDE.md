# Ember 前端设计规范 (精简版)

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

### 3.2 按钮（Button）

- 主操作：`.btn-ember`
- 次操作：白底描边（`bg-white border border-gray-200`）
- 图标按钮必须有 `aria-label`
- 可点击元素必须有 `cursor-pointer`

### 3.3 输入（Input）

- 默认：浅灰底 + 细边框
- Focus：白底 + 品牌色边框/光圈
- 输入框不得仅靠 placeholder 传递语义，必须有可见 label（筛选区强制）

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

---

## 6. 表格与分页规范

- 表头：`bg-gray-50` + 中灰文本 + 中等字重。
- 表格容器：`overflow-hidden`，避免边角溢出。
- 分页条：底部独立区域，`border-t border-gray-100 bg-gray-50/50 p-6`。
- 推荐分页布局：`total, sizes, prev, pager, next, jumper`。

---

## 7. 无障碍与动效

- 尊重 `prefers-reduced-motion`（全局兜底）。
- 图标按钮必须提供 `aria-label`。
- 禁止 emoji 作为 UI 图标。
- 颜色不是唯一状态表达（需文字/图标辅助）。

---

## 8. 反模式（禁止）

- 标题与筛选并排且造成大面积留白
- 单筛选项强行使用多列布局导致画面松散
- 同一筛选区同时出现“查询 + 刷新”重复动作
- 无 label，仅靠 placeholder 的筛选输入
- 同类页面按钮对齐方式不一致

---

## 9. 页面开发检查清单

1. 是否使用统一列表骨架（Header Card + Table Card）？
2. 筛选输入是否有可见 label、统一高度（42px）？
3. 查询/重置按钮是否右对齐且顺序固定？
4. 如场景复杂，是否需要展示“已生效筛选”并支持清空？
5. 是否删除了重复语义操作（例如刷新）？
6. 表格/分页是否采用统一容器样式？
7. 是否通过移动端与桌面端断点检查？
