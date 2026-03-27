# 概览页与续费中心视觉统一重设计

> 状态：草稿
> 负责人：AI
> 更新时间：2026-03-19

## 背景

- Dashboard 使用 `bg-slate-900` 深色英雄卡，直接违反 `web-design-guide.md` 中"非状态型信息区不要使用大面积深色整卡"的禁令，形成独立于白底体系的第二套视觉语言
- RenewalCenter 状态卡片使用渐变背景（`bg-gradient-to-br`），续费方式 header 使用渐变（`from-amber-50 via-white to-rose-50`），不符合平坦白底卡片规范
- 两个页面大量使用 `rounded-3xl`（规范要求 `rounded-2xl`）、边框色 `slate-200`（规范要求 `gray-100`）
- 续费中心存在设计指令泄漏到用户可见文案的问题

## 目标

1. 消除 Dashboard 深色英雄卡，替换为白底 + ember 强调色卡片
2. 消除 RenewalCenter 所有渐变背景，状态区改用白底 + 彩色左边框
3. 全面统一卡片圆角为 `rounded-2xl`、边框为 `border-gray-100`
4. 所有主操作按钮使用 `.btn-ember`
5. 修复用户可见的设计指令泄漏文案

## 非目标

- 不抽取共享组件
- 不改动任何数据流、API 调用、路由逻辑
- 不改动 `<script setup>` 中的业务逻辑（除 computed 类名映射）
- 不改动其他页面

## 当前事实

- 相关文档：`docs/reference/web-design-guide.md`
- 相关页面：
  - `services/web/src/views/console/DashboardView.vue`（362 行）
  - `services/web/src/views/console/RenewalCenterView.vue`（647 行）
- 全局样式：`services/web/src/assets/base.css`（`.btn-ember`、`.input-ember` 已正确定义，可直接复用）
- 当前行为：两个页面功能完整，仅视觉风格与规范不一致
- 现有限制：不能破坏支付流程、兑换码流程、用户状态展示逻辑

## 方案设计

### 1. 用户可见行为

- Dashboard 英雄区从深色大卡片变为白底卡片，信息层级通过 ember 头像色、彩色标签、`.btn-ember` 按钮表达
- 续费中心状态区从渐变背景变为白底 + 彩色左边框（红/黄/绿），状态感知更直接
- 功能和交互完全不变

### 2. 数据与模型

> 本次不涉及数据模型变更。

### 3. 接口与边界

> 本次不涉及 API 变更。

### 4. 关键流程 -- DashboardView.vue

**4.1 英雄区**

| 元素 | 当前 | 改为 |
|------|------|------|
| 外层容器 | `bg-slate-900 text-white shadow-xl rounded-3xl` | `bg-white rounded-2xl border border-gray-100 shadow-sm` |
| 渐变装饰层 | radial-gradient div | 删除 |
| 用户头像 | `bg-white/10 text-white ring-1 ring-white/10 rounded-3xl` | `bg-ember/10 text-ember rounded-2xl` |
| 用户名 | `text-3xl font-semibold` 白色 | `text-2xl font-bold text-gray-900` |
| 角色标签 | `border-white/15 bg-white/10 text-white/85` | `bg-gray-100 text-gray-600 border border-gray-200` |
| 到期标签(过期) | `bg-red-400/20 text-red-200 ring-1 ring-red-300/20` | `bg-red-50 text-red-700 ring-1 ring-red-200` |
| 到期标签(正常) | `bg-amber-400/20 text-amber-100 ring-1 ring-amber-300/20` | `bg-amber-50 text-amber-700 ring-1 ring-amber-200` |
| 到期标签(永久) | `bg-emerald-400/20 text-emerald-200 ring-1 ring-emerald-300/20` | `bg-emerald-50 text-emerald-700 ring-1 ring-emerald-200` |
| Telegram(已绑) | `bg-sky-400/25 text-sky-100 ring-1 ring-sky-300/25` | `bg-sky-50 text-sky-700 ring-1 ring-sky-200` |
| Telegram(未绑) | `bg-white/10 text-slate-200` | `bg-gray-100 text-gray-500` |
| 状态子卡片 | `rounded-3xl border-white/10 bg-white/5 backdrop-blur` | `rounded-2xl border border-gray-100 bg-gray-50` |
| 状态标签文字 | `text-slate-400` | `text-gray-400` |
| 状态描述 | `text-slate-300` | `text-gray-500` |
| 续费按钮 | `bg-white text-slate-900 hover:bg-slate-100 rounded-2xl` | `.btn-ember rounded-xl` |

**4.2 脚本**

`membershipStatusTextClass`：`text-red-300` -> `text-red-600`，`text-sky-200` -> `text-sky-600`，`text-emerald-300` -> `text-emerald-600`

**4.3 统计卡片**

所有三张卡：`rounded-3xl border-slate-200` -> `rounded-2xl border-gray-100`；文字 `text-slate-*` -> `text-gray-*`

**4.4 帮助与资源**

外层 + 子卡片：`rounded-3xl` -> `rounded-2xl`；`slate-*` -> `gray-*`

**4.5 服务器连接**

外层 + 分割线 + 内嵌卡片：`rounded-3xl` -> `rounded-2xl`；`slate-*` -> `gray-*`

**4.6 快捷操作**

外层 + 操作按钮：`rounded-3xl` -> `rounded-2xl`；`slate-*` -> `gray-*`

**4.7 间距**

`space-y-8` -> `space-y-6`

### 5. 关键流程 -- RenewalCenterView.vue

**5.1 脚本修改**

`statusTone` computed：
- 删除 `panel` 属性
- 新增 `borderLeft` 属性：过期 `'border-l-red-500'`，即将到期 `'border-l-amber-500'`，有效 `'border-l-emerald-500'`

**5.2 状态卡片**

| 元素 | 当前 | 改为 |
|------|------|------|
| 外层 | `rounded-3xl bg-gradient-to-br` + `:class="statusTone.panel"` | `bg-white rounded-2xl border border-gray-100 shadow-sm border-l-4` + `:class="statusTone.borderLeft"` |
| 标题 | `text-3xl font-bold` | `text-2xl font-bold` |
| 剩余时长子卡 | `bg-white/90 border-white` | `bg-gray-50 border border-gray-100` |
| 推荐操作子卡 | `border-ember/10 ring-1 ring-ember/5` | `border border-gray-100` |
| 文案(第330行) | "优先选择一种续费方式，主操作统一使用品牌色，避免视觉分裂。" | "选择在线购买或使用兑换码，续费后时长直接叠加。" |

**5.3 续费方式区 header**

- `rounded-3xl` -> `rounded-2xl`
- header 渐变 `bg-gradient-to-r from-amber-50 via-white to-rose-50` -> `bg-gray-50/50`
- Tab 容器 `rounded-3xl` -> `rounded-2xl`
- 文案(第358行)：`"把在线支付和兑换码续期收口在一个切换区里，减少页面噪音。"` -> `"选择在线购买或使用兑换码完成续费"`

**5.4 方案卡片**

- `rounded-3xl bg-gradient-to-b from-white to-gray-50` -> `rounded-2xl bg-white`
- 购买按钮：`rounded-lg bg-ember` -> `.btn-ember rounded-xl`

**5.5 其他嵌套卡片**

- 第 400 行、第 468 行的 `rounded-3xl` -> `rounded-2xl`

### 6. 失败路径与边界条件

- 本次为纯视觉变更，无功能逻辑改动
- 不能破坏：支付跳转流程、兑换码逻辑、用户状态展示、分页、空状态
- `statusTone.panel` 删除后确保模板中无残留引用

## 影响范围

- API：无
- Web：DashboardView.vue、RenewalCenterView.vue
- Bot：无
- 配置/部署：无
- 文档：无需更新 `system-architecture.md`（非架构变更）

## 验证方式

### 编译/测试

- `cd services/web && npm run build`

### 手工验证

1. 打开 Dashboard，确认：无深色卡片、全部 `rounded-2xl` + `border-gray-100`、续费按钮用 `.btn-ember`、头像用 ember 色
2. 打开续费中心，确认：白底 + 彩色左边框、无渐变、购买按钮用 `.btn-ember`、无设计指令泄漏文案
3. 测试过期/正常/永久三种用户状态下两个页面的展示
4. 对照 `web-design-guide.md` 第 9 节检查清单

## 落地后文档处理

- 本方案落地后移入 `docs/archive/`
- 无需更新 `docs/reference/` 或 `docs/system-architecture.md`
