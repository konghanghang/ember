# Ember 前端设计规范 (Design System)

本文档旨在通过标准化的设计语言，保持 Ember 项目 UI/UX 的一致性与现代感。本规范基于 **Vue 3 + Tailwind CSS** 技术栈。

## 1. 核心理念 (Core Philosophy)

*   **沉浸 (Cinematic):** 受到流媒体应用（如 Netflix）启发，使用深色背景、鲜艳的红色强调色和高品质的视觉元素（毛玻璃、渐变）。
*   **现代 (Modern):** 采用大圆角、柔和阴影、充足的留白和微交互动画。
*   **简洁 (Clean):** 减少不必要的边框和线条，通过层级和色彩区分内容。

---

## 2. 色彩系统 (Color Palette)

项目主要使用 Tailwind CSS 自定义配置。

### 品牌色 (Brand Colors)
*   **Ember Red (主色):** `bg-ember` / `text-ember` (`#E50914`)
    *   用于：主要按钮、高亮文本、Logo、加载动画。
*   **Brand Gradient:** `bg-gradient-to-r from-ember to-orange-500`
    *   用于：高级会员卡片、进度条、强调背景。

### 背景色 (Backgrounds)
*   **Page Background:** `bg-gray-50` (`#F9FAFB`) - 用于页面底色，营造层次感。
*   **Surface (Card):** `bg-white` (`#FFFFFF`) - 用于内容卡片。
*   **Dark Surface:** `bg-gray-900` - 用于 Hero 区域或深色模式强调块。

### 状态色 (Status)
*   **Success (成功/有效):** `text-green-500` / `bg-green-50`
*   **Warning (待审核):** `text-yellow-500` / `bg-yellow-50`
*   **Error (失败/过期):** `text-red-500` / `bg-red-50`
*   **Info (信息/中性):** `text-blue-500` / `bg-blue-50`

---

## 3. 排版 (Typography)

*   **字体:** 首选 `Plus Jakarta Sans`，备用系统字体。代码使用 `JetBrains Mono`。
*   **标题:**
    *   Page Title: `text-2xl font-bold text-gray-900`
    *   Section Title: `text-lg font-bold`
*   **正文:** `text-sm text-gray-600` (主要阅读颜色) 或 `text-gray-500` (次要信息)。

---

## 4. 组件规范 (Components)

### 卡片 (Cards)
抛弃传统的 `el-card` 默认样式，使用自定义 Tailwind 类：
```html
<div class="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden">
  <!-- Content -->
</div>
```
*   **圆角:** 统一使用 `rounded-2xl` (16px) 或 `rounded-xl` (12px)。
*   **边框:** 极细微的 `border border-gray-100`，避免使用深色边框。
*   **交互:** 悬停时添加阴影或边框颜色变化：`hover:shadow-md hover:border-ember/30 transition-all`。

### 按钮 (Buttons)
*   **主要按钮 (Primary):** 优先使用 `.btn-ember` 作为“品牌主按钮”的基础样式，再叠加布局类（宽度/圆角/间距等）。
    ```html
    <button class="btn-ember px-6 py-2 rounded-lg font-bold shadow-md hover:shadow-lg active:scale-95 cursor-pointer">
      主要操作
    </button>
    ```
    *   特点：品牌渐变、轻微上浮、红色阴影，适合表单提交与 CTA。

*   **次要按钮 (Secondary/Outline):**
    ```html
    <button class="px-4 py-2 bg-white border border-gray-200 text-gray-700 rounded-lg hover:bg-gray-50 transition-colors cursor-pointer">
      次要操作
    </button>
    ```

*   **图标按钮:**
    ```html
    <button aria-label="编辑" class="p-2 text-gray-400 hover:text-ember hover:bg-gray-100 rounded-lg transition-colors cursor-pointer">
      <el-icon><Edit /></el-icon>
    </button>
    ```

*   **按钮样式策略:**
    | 场景 | 方式 | 说明 |
    |------|------|------|
    | 主要操作（表单提交、CTA） | `.btn-ember` | 保持全站一致的“品牌主按钮” |
    | 页面内次要操作 | Tailwind 内联 | 取消/返回/筛选等不应抢主视觉 |
    | 语义化操作（危险/成功） | Tailwind 内联 | 删除用红、批准用绿，语义优先于品牌色 |
    | Element Plus 组件内 | 沿用现有主题覆盖 | Dialog footer 等保持一致即可 |

### 输入框 (Inputs)
使用 `.input-ember` 类或以下 Tailwind 组合，替代 Element Plus 默认样式：
```css
/* 自定义样式覆盖 */
:deep(.el-input__wrapper) {
  background-color: #f9fafb; /* bg-gray-50 */
  box-shadow: 0 0 0 1px #e5e7eb inset; /* border-gray-200 */
  border-radius: 0.75rem; /* rounded-xl */
}
:deep(.el-input__wrapper.is-focus) {
  background-color: white;
  box-shadow: 0 0 0 2px var(--ember-red) inset !important;
}
```
*   **特点:** 浅灰背景，聚焦时变白并显示品牌色光圈，带图标。

### 搜索框模式 (Search Input Pattern)
有些页面的搜索框需要更紧凑、更“像控制台工具”的感觉，并且希望完全掌控 icon、focus ring、间距。这类搜索框不使用 `.input-ember`，而是使用纯 Tailwind 的独立模式。

适用场景：
*   表格页/列表页的顶部搜索（例如用户管理、订阅管理等）
*   需要 `group-focus-within` 统一驱动 icon 与外框状态的输入

示例（来自用户管理页的模式）：
```html
<div class="relative w-full md:w-80 group">
  <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
    <el-icon class="text-gray-400 group-focus-within:text-ember transition-colors"><Search /></el-icon>
  </div>
  <input
    type="search"
    inputmode="search"
    autocomplete="off"
    aria-label="搜索用户名或邮箱"
    placeholder="搜索用户名或邮箱..."
    class="w-full pl-10 pr-4 py-2.5 bg-gray-50 border border-gray-200 rounded-xl text-sm outline-none focus:bg-white focus:border-ember focus:ring-4 focus:ring-ember/10 transition-all placeholder-gray-400"
  />
</div>
```
要求：
*   必须提供 `aria-label`（避免只有 placeholder）
*   icon 使用 `pointer-events-none`，避免遮挡点击
*   禁止使用 emoji 图标，统一使用 Element Plus 图标

---

## 5. 常用布局模式 (Patterns)

### 统计概览 (Stats Dashboard)
用于首页或仪表盘顶部。
*   **结构:** Grid 布局 (`grid-cols-1 md:grid-cols-3 gap-6`)。
*   **元素:** 左侧图标(带浅色背景) + 右侧数据(大字体粗体)。

### 媒体网格 (Media Grid)
用于展示电影/剧集海报。
*   **容器:** `grid grid-cols-2 md:grid-cols-4 lg:grid-cols-5 gap-6`。
*   **海报:** `aspect-[2/3]` 比例，`object-cover`。
*   **悬停:** 鼠标悬停显示操作遮罩 (Overlay) 和 放大效果 (`group-hover:scale-110`).
*   **状态标:** 右上角使用半透明毛玻璃标签 (`backdrop-blur-md`).

### 列表/表格 (Tables)
*   **表头:** 浅灰背景 `bg-gray-50`，深灰文字。
*   **行:** 悬停高亮 `hover:bg-red-50/30`。
*   **操作栏:** 优先显示图标按钮，次要操作折叠进 "更多" (`...`) 菜单。

---

## 6. 动效 (Animations)

适当使用微交互动效提升质感：

*   **Fade In Up:** 页面加载时内容向上浮现。
    ```css
    @keyframes fadeInUp {
      from { opacity: 0; transform: translateY(20px); }
      to { opacity: 1; transform: translateY(0); }
    }
    ```
*   **Pulse:** 用于状态指示点或骨架屏。
*   **Scale:** 按钮点击 (`active:scale-95`) 或海报悬停。

---

## 7. 开发检查清单 (Checklist)

在开发新页面时，请检查：
1.  [ ] 是否使用了 `rounded-2xl` 大圆角卡片？
2.  [ ] 按钮是否有 Hover 和 Active 态？
3.  [ ] 输入框是否有 Focus 品牌色高亮？
4.  [ ] 图标是否统一使用了 `@element-plus/icons-vue`？
5.  [ ] 页面加载是否有过渡动画？
6.  [ ] 是否适配了移动端 (Responsive)？
7.  [ ] 是否避免使用 emoji 作为 UI 图标？
8.  [ ] 图标按钮是否补齐 `aria-label`？
9.  [ ] 动效是否尊重 `prefers-reduced-motion`？
10. [ ] 可点击元素是否呈现合理的鼠标指针（pointer）？

---

> **注意:** 本规范旨在提供指导，实际开发中可根据具体场景灵活调整，但应保持整体视觉风格的统一。

## 8. 侧边栏与导航 (Navigation)

控制台侧边栏参考 `services/web/src/components/console/Sidebar.vue`。

*   **侧边栏容器:** `w-64 h-screen bg-white border-r border-gray-100 shadow-sm`
*   **Logo 区:** `h-16 px-6 border-b border-gray-50`
*   **导航区:** `flex-1 overflow-y-auto py-6 px-3 space-y-1`
*   **导航项:**
    *   默认态：`text-gray-600 hover:bg-gray-50 hover:text-gray-900 transition-colors cursor-pointer`
    *   激活态：`bg-ember/10 text-ember font-medium` + 左侧指示条 `w-1 h-8 bg-ember rounded-r-full`

## 9. 对话框与模态框 (Dialogs)

*   统一使用 `el-dialog`，并保持圆角和 header 分割线清晰。
*   推荐尺寸：
    *   小型：`width="400px"` 确认操作、简单表单
    *   中型：`width="520px"` 编辑表单、详情查看
*   表单布局：优先 `label-position="top"`，复杂表单用 `grid` 做分栏。

## 10. 徽章与状态标签 (Badges)

*   **基础标签（带边框）**：
    ```html
    <span class="text-[11px] px-2 py-0.5 rounded-full font-semibold bg-sky-50 text-sky-700 border border-sky-100">
      信息
    </span>
    ```
*   **语义色彩**：成功绿、警告黄、错误红、信息蓝，避免“只靠颜色表达含义”。

## 11. 空状态 (Empty States)

优先使用 `el-empty`，外层套标准卡片容器（圆角、细边框、轻阴影），并提供下一步操作入口（可选）。

## 12. 响应式策略 (Responsive)

*   断点：移动优先（默认 < `md` 单列，`md` 适度分栏，`lg` 桌面布局）。
*   常用模式：
    *   Header：`flex flex-col md:flex-row ... gap-4`
    *   Grid：`grid-cols-2 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6`

## 13. 无障碍 (Accessibility)

*   **必须**尊重 `prefers-reduced-motion`（全局兜底 + 局部避免过度动画）。
*   图标按钮（无文字）必须提供 `aria-label`。
*   禁止使用 emoji 作为 UI 图标（跨平台渲染不一致，且难以一致对齐）。
