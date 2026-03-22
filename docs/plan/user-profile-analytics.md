# 用户画像实现方案

> 状态：草稿
> 负责人：Codex
> 更新时间：2026-03-22

## 背景

当前管理端已经提供播放历史查询，但它只解决“查记录”，没有解决“看人”：

- 管理员无法快速判断某个用户最近一段时间的活跃程度、看片时段和设备偏好。
- 播放历史列表按记录平铺，适合审计，不适合作为用户分析入口。
- 后续如果要扩展播放分析、用户自助画像，也需要先把用户画像边界独立出来。

## 目标

本方案要实现：

1. 提供管理员查看任意用户画像的稳定接口和页面。
2. 基于现有 `PlaybackActivity` 数据输出最小可用画像指标。
3. 打通用户管理和播放历史到用户画像的联动入口。

## 非目标

本次明确不做：

- 不新增数据库表、字段、索引或配置项。
- 不做内容类型偏好、题材偏好、完播率等依赖额外媒体元数据的高级画像。
- 不新增独立侧边栏菜单，不重构整套“播放历史”导航结构。

## 当前事实

以当前代码和现行文档为准，现状如下：

- 相关文档：`docs/system-architecture.md`、`docs/reference/web-design-guide.md`
- 相关服务/页面：`services/api/internal/services/playback/history.go`、`services/api/internal/handlers/playback_history.go`、`services/web/src/views/admin/PlaybackHistoryView.vue`、`services/web/src/views/admin/UsersView.vue`
- 当前行为：管理员可按用户、关键词、日期范围查询播放历史明细。
- 现有限制：没有按用户聚合的播放画像能力，用户管理也无法下钻到播放分析。

## 方案设计

### 1. 用户可见行为

- 管理员在用户管理页可进入某个用户的“用户画像”页面。
- 用户画像页默认展示指定时间窗口内的播放摘要、活跃时段、设备/客户端分布、最近播放记录。
- 播放历史页增加“查看画像”入口，便于从记录直接跳转到对应用户画像。
- 现有播放历史查询行为保持不变，仍然可按原条件筛选和分页。

### 2. 数据与模型

> 本次不涉及数据模型变更。

### 3. 接口与边界

- 新增管理员接口：`GET /api/v1/admin/users/:id/profile`
- 新增统一认证接口：`GET /api/v1/profile/analytics`
- 两个接口统一支持 `range=7d|30d|90d|all`，默认 `30d`
- 响应字段统一使用 `data`，包含：
  - `userId`
  - `username`
  - `range`
  - `totalPlayCount`
  - `totalPlayDuration`
  - `totalPlayDurationFormatted`
  - `activeDays`
  - `averagePlayDuration`
  - `averagePlayDurationFormatted`
  - `lastPlayedAt`
  - `hourlyDistribution`
  - `deviceDistribution`
  - `clientDistribution`
  - `badges`
  - `recentRecords`
- 服务边界独立为 `UserPlaybackProfileService`，不把画像聚合逻辑塞进现有播放历史查询接口。

### 4. 关键流程

1. 前端通过本地 `userId` 请求用户画像接口，并可带时间窗口参数。
2. 后端读取本地 `users` 表，校验用户存在并映射 `embyId`。
3. 服务基于时间窗口拼接 `PlaybackActivity` 查询条件，分别计算摘要、分布和最近记录。
4. 服务构建勋章规则结果，统一输出格式化后的画像响应。
5. 前端渲染画像页面，并提供跳转到播放历史的联动入口。

### 5. 失败路径与边界条件

- 用户不存在：管理员接口返回 `404`，前端提示用户不存在。
- 用户存在但无播放数据：返回空画像结构，不报错，摘要为 `0`，数组为空。
- `range` 非法：返回 `400`，错误信息明确指出仅支持固定时间窗口。
- 用户未绑定 `embyId`：默认回退使用本地 `userId` 作为 `PlaybackActivity.UserId` 过滤值，保持与现有播放历史逻辑一致。
- 兼容性约束：现有播放历史接口、用户管理列表和管理端菜单路径保持不变。

## 影响范围

涉及的子系统：

- API：有，新增用户画像服务、handler、路由和类型导出。
- Web：有，新增用户画像页、路由、API 定义、用户管理入口和播放历史跳转。
- Bot：无。
- 配置/部署：无。
- 文档：更新 `docs/system-architecture.md`。

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`
- `cd services/web && npm run build`

### 手工验证

- 管理员从用户管理进入指定用户画像页，默认看到 `30d` 数据。
- 管理员切换 `7d / 30d / 90d / all`，摘要和分布同步变化。
- 无播放记录用户可正常打开画像页，页面展示空态而不是报错。
- 管理员从播放历史页点击“查看画像”可跳到对应用户画像页。

## 落地后文档处理

落地后应同步处理：

- 将新增接口、前端路由和页面职责补充到 `docs/system-architecture.md`
- 本方案在功能上线并稳定后移入 `docs/archive/plan/`
