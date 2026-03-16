# Ember 前端设计系统治理计划

## 背景与目标

Ember 前端基于 Vue 3 + Tailwind CSS + Element Plus 构建，已有设计规范文档 `docs/reference/WEB_DESIGN_GUIDE.md`，定义了色彩、排版、组件、布局、动效等核心规范。

**经过全面审查发现**，代码实现与设计文档的整体对齐度约 95%，基础设计质量优秀，但存在三类问题：

1. **文档缺失** — 代码中已有 6+ 种设计模式未被文档覆盖（侧边栏、对话框、徽章、空状态、响应式策略、无障碍）
2. **代码不一致** — 按钮样式三种写法混用、emoji 用作 UI 图标、图标容器大小不统一
3. **无障碍缺失** — 零 `prefers-reduced-motion` 支持、`cursor-pointer` 仅 2 个文件使用、ARIA 属性极少

**本方案实现**：
1. 补全设计规范文档，覆盖所有已实现的设计模式
2. 修复代码中的不一致问题，使代码严格遵循规范
3. 补充基础无障碍支持

**不做的事**：
- 不做暗黑模式（用户明确暂不考虑，虽然 tailwind.config.js 已配置 `darkMode: 'class'`）
- 不更换字体或色彩方案（现有 Plus Jakarta Sans + #E50914 品牌色完成度很高）
- 不引入新的组件库或设计令牌系统（保持现有 Tailwind + CSS 变量架构）
- 不做组件抽象重构（问题是一致性，不是架构）

**范围与约定**：
- 本计划仅覆盖前端 `services/web/` 与设计规范文档 `docs/reference/WEB_DESIGN_GUIDE.md`
- 文中所有路径均为“仓库根目录相对路径”，避免 `src/...` 这种不带上下文的歧义写法

---

## 一、审查发现详情

### 1.1 当前对齐度矩阵

| 维度 | 规范要求 | 代码实现 | 对齐度 | 问题 |
|------|---------|---------|--------|------|
| 品牌色 | `#E50914` / `bg-ember` | CSS 变量 + Tailwind 扩展完整 | ✅ 100% | 无 |
| 渐变色 | `from-ember to-orange-500` | DashboardView、HeroSection 正确使用 | ✅ 100% | 无 |
| 背景色 | `bg-gray-50` / `bg-white` / `bg-gray-900` | 全部精确匹配 | ✅ 100% | 无 |
| 状态色 | 绿/黄/红/蓝语义色 | 所有状态页面正确使用 | ✅ 100% | 无 |
| 字体 | Plus Jakarta Sans / JetBrains Mono | tailwind.config.js + base.css 完整定义 | ✅ 100% | 无 |
| 标题排版 | `text-2xl font-bold text-gray-900` | 所有页面一致 | ✅ 100% | 无 |
| 卡片 | `bg-white rounded-2xl border-gray-100 shadow-sm` | 32+ 处使用，高度一致 | ✅ 95% | Hero 卡片用了 `rounded-3xl` |
| **按钮** | Tailwind 内联样式 | `.btn-ember` / 内联 Tailwind / Element Plus 三种混用 | 🟡 85% | **需统一** |
| 输入框 | `.input-ember` + Element Plus 覆盖 | 大部分正确，搜索框用了原生 Tailwind | ✅ 90% | 轻微不一致 |
| 布局网格 | `grid-cols-1 md:grid-cols-3 gap-6` | 统计卡片、媒体网格完全符合 | ✅ 100% | 无 |
| 动效 | fadeInUp / pulse / scale | 9+ 处使用 animate-fade-in-up | ✅ 100% | 无 |
| **emoji 图标** | 未提及（应禁止） | RankingsView 使用 🥇🥈🥉⏱▶📅 | 🔴 不合格 | **需替换** |
| **无障碍** | 未提及 | 零 prefers-reduced-motion，2 处 aria | 🔴 缺失 | **需补充** |
| **cursor-pointer** | 未提及 | 仅 2 文件 5 处使用 | 🟡 不足 | **需补充** |

### 1.2 文档覆盖缺口

设计规范文档当前包含 7 个章节：
1. ✅ 核心理念
2. ✅ 色彩系统
3. ✅ 排版
4. ✅ 组件规范（卡片、按钮、输入框）
5. ✅ 常用布局模式（统计概览、媒体网格、表格）
6. ✅ 动效
7. ✅ 开发检查清单

**缺失章节**（代码中已有实现但文档未覆盖）：
- ❌ 侧边栏/导航模式
- ❌ 对话框/模态框样式
- ❌ 徽章/状态标签
- ❌ 空状态设计
- ❌ 响应式策略
- ❌ 无障碍规范
- ❌ 按钮样式策略（`.btn-ember` vs Tailwind 内联的使用场景区分）
- ❌ 搜索框模式（独立于 `.input-ember` 的搜索栏设计规范）

> **⚠️ 设计决策记录**：搜索框（如 UsersView 页头搜索栏）应保持自定义 Tailwind 写法（`bg-gray-50 rounded-xl focus:ring-4 focus:ring-ember/10` + 图标联动），**不应**统一为 `.input-ember`。搜索框是页面级交互入口，需要比表单输入框更高的视觉精致度和层次感。

### 1.3 各文件问题清单

| 文件 | 问题 | 严重度 |
|------|------|--------|
| `services/web/src/views/console/RankingsView.vue` | 🥇🥈🥉⏱▶📅 emoji 做 UI 图标 | 🔴 高 |
| `services/web/src/assets/base.css` | 无 `prefers-reduced-motion` | 🔴 高 |
| `services/web/src/views/console/DashboardView.vue` | Hero 按钮用白色背景而非 `.btn-ember`；Hero 卡片 `rounded-3xl` 未在规范中明确 | 🟡 中 |
| `services/web/src/views/console/SubscriptionsView.vue` | 按钮用内联 Tailwind `bg-ember text-white rounded-lg`，与 `.btn-ember` 不统一 | 🟡 中 |
| `services/web/src/views/admin/UsersView.vue` | 表格行操作按钮缺 `cursor-pointer`、缺 `aria-label` | 🟡 中 |
| `services/web/src/components/console/Sidebar.vue` | 导航项缺 `cursor-pointer`；缺 `aria-label` | 🟢 低 |
| `services/web/src/components/console/TopBar.vue` | 图标按钮缺 `aria-label` | 🟢 低 |
| `services/web/src/components/home/FeaturesSection.vue` | 图标容器用 `w-14 h-14`，与 DashboardView 不完全统一 | 🟢 低 |

---

## 二、实施方案

### 第一步：更新设计规范文档

**目标**：使 `docs/reference/WEB_DESIGN_GUIDE.md` 成为完整的设计系统参考，覆盖所有已实现的模式。

**修改文件**：`docs/reference/WEB_DESIGN_GUIDE.md`

#### 2.1.1 新增 §8「侧边栏与导航」

从 `services/web/src/components/console/Sidebar.vue` 提取已有模式，文档化为规范：

````markdown
## 8. 侧边栏与导航 (Navigation)

### 侧边栏 (Sidebar)
- **容器:** `w-64 h-screen bg-white border-r border-gray-100 shadow-sm`
- **Logo 区:** `h-16 px-6 border-b border-gray-50`
- **导航区:** `flex-1 overflow-y-auto py-6 px-3 space-y-1`

### 导航项 (Nav Items)
```html
<!-- 默认态 -->
<a class="flex items-center px-3 py-2.5 rounded-lg text-gray-600
          hover:bg-gray-50 hover:text-gray-900 transition-colors cursor-pointer">
  <el-icon class="mr-3"><Odometer /></el-icon>
  <span>仪表盘</span>
</a>

<!-- 激活态 -->
<a class="flex items-center px-3 py-2.5 rounded-lg
          bg-ember/10 text-ember font-medium relative">
  <!-- 左侧红色指示条 -->
  <span class="absolute left-0 top-1/2 -translate-y-1/2 w-1 h-8 bg-ember rounded-r-full"></span>
  <el-icon class="mr-3"><Odometer /></el-icon>
  <span>仪表盘</span>
</a>
```

### 移动端适配
- **触发:** `lg:hidden` 汉堡按钮
- **模式:** 抽屉覆盖 + `bg-gray-600/75` 半透明遮罩
- **动画:** `transform transition-transform duration-300`
````

#### 2.1.2 新增 §9「对话框与模态框」

从 `DashboardView.vue` 和 `UsersView.vue` 提取：

````markdown
## 9. 对话框 (Dialogs)

### 尺寸规范
| 类型 | 宽度 | 用途 |
|------|------|------|
| 小型 | `400px` | 确认操作、简单表单 |
| 中型 | `520px` | 编辑表单、详情查看 |
| 大型 | `680px` | 复杂表单、多步操作 |

### 样式
```html
<el-dialog width="520px" align-center class="rounded-2xl">
  <template #header>
    <span class="text-lg font-bold">标题</span>
  </template>
  <!-- 表单使用 label-position="top" -->
  <el-form label-position="top">
    ...
  </el-form>
</el-dialog>
```

### 表单布局
- 标签位置：`label-position="top"`（非左对齐）
- 多列布局：`grid grid-cols-1 md:grid-cols-2 gap-6`
- 开关控件容器：`border border-gray-200 rounded-xl bg-gray-50 p-4`
````

#### 2.1.3 新增 §10「徽章与状态标签」

从多个页面提取统一模式：

````markdown
## 10. 徽章与状态标签 (Badges)

### 基础样式
```html
<span class="inline-flex items-center text-xs px-3 py-1 rounded-full
             bg-{color}-50 text-{color}-700 font-medium">
  状态文本
</span>
```

### 带边框变体
```html
<span class="text-[11px] px-2 py-0.5 rounded-full font-semibold
             bg-{color}-50 text-{color}-700 border border-{color}-100">
  标签文本
</span>
```

### 语义色彩映射
| 状态 | 背景 | 文字 | 边框 |
|------|------|------|------|
| 成功/有效 | `bg-green-50` | `text-green-700` | `border-green-100` |
| 警告/待审 | `bg-amber-50` | `text-amber-700` | `border-amber-100` |
| 错误/过期 | `bg-red-50` | `text-red-700` | `border-red-100` |
| 信息/中性 | `bg-sky-50` | `text-sky-700` | `border-sky-100` |

### 状态指示点
```html
<!-- 在线状态（脉冲动画） -->
<span class="relative flex h-2.5 w-2.5">
  <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75"></span>
  <span class="relative inline-flex rounded-full h-2.5 w-2.5 bg-green-500"></span>
</span>
```
````

#### 2.1.4 新增 §11「空状态」

````markdown
## 11. 空状态 (Empty States)

使用 Element Plus 的 `el-empty` 组件，搭配品牌化容器：
```html
<div class="bg-white border border-gray-100 rounded-2xl p-8">
  <el-empty description="暂无数据">
    <template #description>
      <span class="text-gray-400 text-sm">暂无播放数据</span>
    </template>
  </el-empty>
</div>
```

- **文字色:** `text-gray-400 text-sm`
- **容器:** 沿用标准卡片样式
- **操作按钮（可选）:** 使用次要按钮引导用户操作
````

#### 2.1.5 新增 §12「响应式策略」

````markdown
## 12. 响应式设计 (Responsive)

### 断点策略（移动优先）
| 断点 | 像素 | 用途 |
|------|------|------|
| 默认 | <768px | 手机（单列布局） |
| `md` | ≥768px | 平板（两列布局） |
| `lg` | ≥1024px | 桌面（侧边栏展开） |
| `xl` | ≥1280px | 大屏（更多网格列） |

### 关键模式
- **侧边栏:** `lg` 断点切换（手机为抽屉，桌面为固定侧边栏）
- **网格:** `grid-cols-2 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6`
- **表头:** `flex flex-col md:flex-row justify-between items-start md:items-center gap-4`
- **文字大小:** `text-3xl md:text-5xl`（标题响应式缩放）
- **搜索框:** `hidden md:flex`（手机端隐藏，用图标替代）

### 容器最大宽度
- 内容区跟随侧边栏：`lg:pl-64`（左偏移侧边栏宽度）
- 不使用全局 `max-w` 容器，内容自然填充
````

#### 2.1.6 新增 §13「无障碍」

````markdown
## 13. 无障碍 (Accessibility)

### 强制要求
1. **动画尊重用户偏好:** 所有动画必须受 `prefers-reduced-motion` 控制
2. **可交互元素指针:** 所有可点击元素必须添加 `cursor-pointer`
3. **图标按钮标注:** 无文字的图标按钮必须添加 `aria-label`
4. **色彩对比度:** 正文文字与背景的对比度 ≥ 4.5:1

### 禁止事项
- ❌ 不使用 emoji 作为 UI 图标（跨平台渲染不一致）
- ❌ 不使用纯色彩区分状态（必须搭配文字或图标）
- ❌ 不使用 `outline: none` 移除焦点环（除非提供替代焦点样式）

### aria-label 使用场景
```html
<!-- 图标按钮 -->
<button aria-label="编辑用户" class="p-2 text-gray-400 hover:text-blue-600">
  <el-icon><Edit /></el-icon>
</button>

<!-- 移动端菜单按钮 -->
<button aria-label="打开导航菜单" class="lg:hidden p-2">
  <el-icon><Fold /></el-icon>
</button>
```
````

#### 2.1.7 新增 §4.1「搜索框」

搜索框是独立于 `.input-ember` 的 UI 模式，保持自定义 Tailwind 写法：

````markdown
### 搜索框 (Search Input)

搜索框作为页面级交互入口，使用比表单输入框更精致的自定义样式，**不使用 `.input-ember`**。

```html
<div class="relative w-full md:w-80 group">
  <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
    <el-icon class="text-gray-400 group-focus-within:text-ember transition-colors"><Search /></el-icon>
  </div>
  <input
    type="text"
    placeholder="搜索..."
    class="w-full pl-10 pr-4 py-2.5 bg-gray-50 border border-gray-200 rounded-xl text-sm
           outline-none focus:bg-white focus:border-ember focus:ring-4 focus:ring-ember/10
           transition-all placeholder-gray-400"
  />
</div>
```

**与 `.input-ember` 的区别：**
| 特性 | 搜索框 | `.input-ember` 表单输入框 |
|------|--------|--------------------------|
| 背景 | `bg-gray-50`（有层次感） | `#FFFFFF`（纯白） |
| 圆角 | `rounded-xl` | Element Plus 默认 |
| 聚焦 | `ring-4 ring-ember/10`（柔和光晕） | `box-shadow 3px`（品牌色描边） |
| 图标 | 自定义绝对定位 + 聚焦变色 | Element Plus prefix-icon |
| 用途 | 页面顶部搜索栏 | 表单字段（登录、编辑、兑换等） |
````

#### 2.1.8 更新现有 §4「组件规范」

在按钮部分补充 `.btn-ember` 和使用场景区分：

````markdown
### 按钮样式策略

| 场景 | 方式 | 示例 |
|------|------|------|
| 主要操作（表单提交、CTA） | `.btn-ember` CSS 类 | 登录、注册、兑换 |
| 页面内次要操作 | Tailwind 内联 | 取消、返回、筛选 |
| 图标操作按钮 | Tailwind 内联 | 编辑、删除、更多 |
| Element Plus 组件内 | 沿用主题覆盖 | Dialog 确认按钮 |
````

在图标容器部分补充大小规范：

````markdown
### 图标容器尺寸

| 尺寸 | 类名 | 用途 |
|------|------|------|
| 标准 | `w-12 h-12 rounded-xl` | 统计卡片、列表项图标 |
| 大号 | `w-14 h-14 rounded-2xl` | Hero 区域、特性展示 |
````

#### 2.1.8 更新 §7「开发检查清单」

在现有 6 项基础上增加无障碍检查项：

````markdown
7.  [ ] 可点击元素是否添加了 `cursor-pointer`？
8.  [ ] 图标按钮是否有 `aria-label`？
9.  [ ] 是否使用了 emoji 作为图标？（应禁止）
10. [ ] 按钮样式是否遵循了样式策略表？
````

---

### 第二步：修复代码不一致

#### 2.2.1 添加 `prefers-reduced-motion` 全局支持

**修改文件**：`services/web/src/assets/base.css`

在文件末尾添加：

```css
/* 无障碍：尊重用户减少动画偏好 */
@media (prefers-reduced-motion: reduce) {
  *,
  *::before,
  *::after {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
    scroll-behavior: auto !important;
  }
}
```

**原理**：这是一个全局兜底方案，当用户在系统设置中开启"减少动画"时，所有 CSS 动画和过渡都会被抑制。这比逐个组件添加 `motion-safe:` 前缀更简洁可靠。

---

#### 2.2.2 替换 emoji 图标为 SVG/CSS 方案

**修改文件**：`services/web/src/views/console/RankingsView.vue`

##### 排名奖牌 🥇🥈🥉 → CSS 圆形排名

当前实现（`medal(rank)` 返回 emoji）：
```typescript
function medal(rank: number): string {
  if (rank === 1) return '🥇'
  if (rank === 2) return '🥈'
  if (rank === 3) return '🥉'
  return `${rank}.`
}
```

替换方案：删除 `medal()`（不再返回 emoji），直接用 `item.rank` 渲染排名，并对 Top3 使用条件样式的徽章：

```html
<span v-if="item.rank <= 3"
      class="w-7 h-7 rounded-full flex items-center justify-center text-xs font-bold text-white"
      :class="[
        item.rank === 1 ? 'bg-amber-400' : '',
        item.rank === 2 ? 'bg-gray-400' : '',
        item.rank === 3 ? 'bg-amber-600' : ''
      ]">
  {{ item.rank }}
</span>
<span v-else class="w-7 text-sm font-semibold text-gray-400 text-center tabular-nums">
  {{ item.rank }}
</span>
```

色彩语义：
- 第 1 名：`bg-amber-400`（金色）
- 第 2 名：`bg-gray-400`（银色）
- 第 3 名：`bg-amber-600`（铜色）

##### 元数据符号 ⏱▶ → Element Plus 图标

当前实现（模板中直接拼接 ⏱▶ 文本）：
```html
⏱ {{ formatDuration(item.duration) }} · ▶ {{ item.playCount }} 次
```

替换为：
```html
<el-icon :size="12" class="mr-0.5"><Timer /></el-icon>
{{ formatDuration(item.duration) }}
<span class="mx-1 text-gray-300">·</span>
<el-icon :size="12" class="mr-0.5"><VideoPlay /></el-icon>
{{ item.playCount }} 次
```

注：项目在 `services/web/src/main.ts` 中全量注册了 `@element-plus/icons-vue`，因此可直接使用 `Timer`、`VideoPlay`、`Calendar` 等图标组件。

##### 日期符号 📅 → Element Plus 图标

当前实现（`rangeText` 计算属性拼接了 📅 emoji）：
```typescript
if (start !== '' && start === end) return `📅 ${start}`
if (start !== '' && end !== '') return `📅 ${start} ~ ${end}`
```

方案：在模板中使用 `<el-icon><Calendar /></el-icon>` + 文本，而非在计算属性中拼接 emoji 字符串。

---

#### 2.2.3 统一按钮样式

**修改文件及具体改动**：

##### DashboardView.vue

1. **Hero 区域续期按钮**（约第 191 行）：
   - 当前：`bg-white text-gray-900 rounded-xl font-bold hover:bg-gray-100 shadow-lg active:scale-95`
   - 保留：这是在深色背景上的反色按钮，属于特殊场景，保持白色但添加 `cursor-pointer`

2. **兑换码提交按钮**（约第 365 行）：
   - 当前：`bg-ember text-white rounded-xl font-bold hover:bg-red-700 shadow-lg`
   - 改为：使用 `.btn-ember` 类 + `rounded-xl cursor-pointer`，保持视觉一致

##### SubscriptionsView.vue

1. **管理按钮**（约第 181 行）：
   - 当前：`px-4 py-2 bg-ember text-white rounded-lg hover:bg-red-700`
   - 改为：使用 `.btn-ember` 类 + `px-4 py-2 rounded-lg cursor-pointer`

##### UsersView.vue

1. **操作图标按钮**：已基本符合规范（`p-2 text-gray-400 hover:text-{color}-600`），仅需添加 `cursor-pointer` 和 `aria-label`

---

#### 2.2.4 添加 `cursor-pointer`

**原则**：所有可点击元素必须呈现“可点击”的鼠标指针（pointer），避免用户误判交互性。

实现策略：
- 对于通过 `@click` 绑定点击事件的非语义元素（如 `div`/`span` 等），必须显式添加 `cursor-pointer`
- 对于 `<button>`/`<a>`/`<router-link>`，通常浏览器默认就是 pointer，但仍建议在项目内保持一致（避免组件库或 reset 样式覆盖导致行为差异）

**逐文件改动清单**：

| 文件 | 需要添加 `cursor-pointer` 的元素 |
|------|------|
| `services/web/src/views/console/DashboardView.vue` | Tab 切换按钮、续期按钮、兑换按钮 |
| `services/web/src/views/console/RankingsView.vue` | 周期切换按钮（日榜/周榜）、刷新按钮、预览按钮 |
| `services/web/src/views/console/SubscriptionsView.vue` | 管理按钮、海报卡片（如有点击事件） |
| `services/web/src/views/admin/UsersView.vue` | 图标操作按钮（编辑/延期/更多）、分页按钮 |
| `services/web/src/components/console/Sidebar.vue` | 所有导航项 `<router-link>` |
| `services/web/src/components/console/TopBar.vue` | 搜索按钮、社区链接、用户下拉菜单触发器 |

---

#### 2.2.5 统一图标容器大小

**规范定义**：
- **标准档**：`w-12 h-12 rounded-xl`（统计卡片、列表项）
- **大号档**：`w-14 h-14 rounded-2xl`（Hero 区域、特性展示）

**改动文件**：

| 文件 | 当前 | 改为 |
|------|------|------|
| `services/web/src/views/console/DashboardView.vue` 统计卡片 | `w-14 h-14 rounded-xl` | `w-12 h-12 rounded-xl`（统计场景用标准档）|
| `services/web/src/components/home/FeaturesSection.vue` 特性图标 | `w-14 h-14 rounded-2xl` | 保持不变（特性展示用大号档）|

---

#### 2.2.6 补充关键 ARIA 属性

**修改文件及改动**：

##### `services/web/src/views/admin/UsersView.vue`
```html
<!-- 编辑按钮 -->
<button aria-label="编辑用户" class="p-2 text-gray-400 hover:text-blue-600 ...">
<!-- 延期按钮 -->
<button aria-label="延期订阅" class="p-2 text-gray-400 hover:text-green-600 ...">
<!-- 更多操作 -->
<button aria-label="更多操作" class="p-2 text-gray-400 hover:text-gray-600 ...">
```

##### `services/web/src/components/console/Sidebar.vue`
```html
<!-- 移动端关闭按钮 -->
<button aria-label="关闭导航菜单" class="...">
```

##### `services/web/src/components/console/TopBar.vue`
```html
<!-- 移动端菜单按钮 -->
<button aria-label="打开导航菜单" class="lg:hidden ...">
<!-- 通知按钮（如有） -->
<button aria-label="查看通知" class="...">
```

---

## 三、执行顺序与依赖

```
┌─────────────────────────────────────────────┐
│  Step 1: 更新 WEB_DESIGN_GUIDE.md           │  ← 先立规矩
│  (文档更新，无代码依赖)                        │
└──────────────────┬──────────────────────────┘
                   │
┌──────────────────▼──────────────────────────┐
│  Step 2.1: base.css 添加 prefers-reduced-   │  ← 全局一行
│  motion (无依赖，独立改动)                     │
└──────────────────┬──────────────────────────┘
                   │
┌──────────────────▼──────────────────────────┐
│  Step 2.2: RankingsView 替换 emoji 图标       │  ← 最明显的视觉问题
│  (独立改动，不影响其他文件)                     │
└──────────────────┬──────────────────────────┘
                   │
┌──────────────────▼──────────────────────────┐
│  Step 2.3-2.6: 按钮统一 + cursor-pointer +   │  ← 可并行处理多个文件
│  图标容器 + ARIA (跨多文件，但改动独立)         │
└──────────────────┬──────────────────────────┘
                   │
┌──────────────────▼──────────────────────────┐
│  验证: npm run build + 浏览器视觉检查          │
└─────────────────────────────────────────────┘
```

---

## 四、涉及文件汇总

| 文件路径 | 改动类型 |
|---------|---------|
| `docs/reference/WEB_DESIGN_GUIDE.md` | 新增 6 个章节 + 更新 2 个现有章节 |
| `services/web/src/assets/base.css` | 添加 `prefers-reduced-motion` 媒体查询 |
| `services/web/src/views/console/RankingsView.vue` | 替换 emoji 图标、补齐可点击指针与图标语义 |
| `services/web/src/views/console/DashboardView.vue` | 统一按钮样式、补齐可点击指针、图标容器大小对齐 |
| `services/web/src/views/console/SubscriptionsView.vue` | 统一按钮样式、补齐可点击指针 |
| `services/web/src/views/admin/UsersView.vue` | 补齐可点击指针、aria-label |
| `services/web/src/components/console/Sidebar.vue` | 补齐可点击指针、aria-label |
| `services/web/src/components/console/TopBar.vue` | 补齐可点击指针、aria-label |
| `services/web/src/components/home/FeaturesSection.vue` | 确认图标容器大小（可能无需改动） |

---

## 五、验证方式

1. **编译验证**：
   ```bash
   cd services/web && npm run build
   ```
   确保构建成功（零错误）。如存在现有的 chunk size 提示类 warning，可记录但不作为阻断项。

2. **视觉比对**（由用户手动执行）：
   - 启动开发服务器，逐页检查：
     - `/console/dashboard` — 统计卡片、按钮
     - `/console/rankings` — 排名奖牌、元数据图标
     - `/console/subscriptions` — 媒体网格、按钮
     - `/console/users` — 表格操作按钮、搜索框
   - 检查移动端（375px）侧边栏抽屉、网格折叠

3. **无障碍检查**：
   - 浏览器 DevTools → Lighthouse → Accessibility 审计
   - 系统设置开启"减少动画" → 确认所有动画被抑制
   - Tab 键导航 → 确认焦点环可见

4. **跨平台一致性**：
   - 确认排名页不再有 emoji 渲染差异（尤其 Windows vs macOS）
