# 用户画像总览实现方案

> 状态：草稿
> 负责人：Codex
> 更新时间：2026-03-22

## 背景

当前系统已经支持“单个用户画像”，但管理员仍然缺少按用户维度的总览入口：

- 只能先知道某个用户，再进入画像页，不利于发现高活跃或异常用户。
- 播放历史偏记录视角，适合审计，不适合横向比较用户活跃度。
- 既然已经有单用户画像能力，就应该补一个聚合列表，把“发现问题”和“下钻分析”串起来。

## 目标

本方案要实现：

1. 提供管理员侧“用户画像总览”列表页，按用户聚合展示播放活跃度。
2. 提供稳定的聚合接口，支持时间窗口、关键词、排序和分页。
3. 打通总览页到单用户画像页、播放历史页的联动入口。

## 非目标

本次明确不做：

- 不新增数据库表、字段、索引或配置项。
- 不做趋势图、导出、标签配置化、画像规则后台配置。
- 不把所有画像细节塞进列表页，不在总览页展示完整设备/客户端/记录详情。

## 当前事实

- 相关文档：`docs/system-architecture.md`、`docs/reference/web-design-guide.md`
- 相关服务/页面：`services/api/internal/services/playback/profile.go`、`services/web/src/views/admin/UserPlaybackProfileView.vue`、`services/web/src/views/admin/PlaybackHistoryView.vue`
- 当前行为：管理员可查看单个用户画像，播放历史页可跳转到单用户画像。
- 现有限制：没有按用户聚合的总览页，管理员无法先看到“谁最活跃、谁最值得下钻”。

## 方案设计

### 1. 用户可见行为

- 新增管理端页面：`用户画像总览`
- 页面默认按用户聚合展示累计播放时长、播放次数、活跃天数、最近播放、峰值时段和标签摘要。
- 页面支持：
  - 时间窗口：`7d / 30d / 90d / all`
  - 用户名关键词搜索
  - 排序字段切换
  - 分页
- 每行支持：
  - `查看画像`
  - `查看播放历史`

### 2. 数据与模型

> 本次不涉及数据模型变更。

### 3. 接口与边界

- 新增管理员接口：`GET /api/v1/admin/playback-profiles`
- 请求参数：
  - `range=7d|30d|90d|all`
  - `keyword`
  - `sortBy=totalDuration|playCount|activeDays|lastPlayedAt`
  - `sortOrder=asc|desc`
  - `page`
  - `pageSize`
- 响应字段统一使用 `data`，每项包含：
  - `userId`
  - `username`
  - `range`
  - `totalPlayCount`
  - `totalPlayDuration`
  - `totalPlayDurationFormatted`
  - `activeDays`
  - `lastPlayedAt`
  - `peakHour`
  - `peakHourLabel`
  - `badges`
- 响应额外包含：
  - `total`
  - `page`
  - `pageSize`
  - `summary.userCount`
  - `summary.totalPlayCount`
  - `summary.totalPlayDuration`
  - `summary.totalPlayDurationFormatted`
- 服务边界：
  - 聚合列表逻辑独立于单用户画像接口
  - 不对每个用户循环调用单用户画像接口，避免 N+1 查询

### 4. 关键流程

1. 前端请求聚合接口，并带上时间窗口、关键词、排序和分页参数。
2. 后端读取指定时间窗口内的 `PlaybackActivity` 数据，并映射本地 `users`。
3. 服务按用户聚合播放时长、次数、活跃天数和峰值时段，并生成标签摘要。
4. 服务按指定排序字段排序后分页返回。
5. 前端列表页展示摘要信息，并支持进入单用户画像和播放历史。

### 5. 失败路径与边界条件

- 时间窗口非法：返回 `400`
- 关键词非法：返回 `400`
- 当前时间窗口内无播放数据：返回空列表和零摘要，不报错
- 非本地用户的播放记录：总览页默认忽略，只展示能映射到本地用户的数据
- 兼容性约束：现有单用户画像页、播放历史页和已有接口保持不变

## 影响范围

- API：有，新增聚合接口与类型定义
- Web：有，新增总览页、菜单入口、路由和列表页交互
- Bot：无
- 配置/部署：无
- 文档：更新 `docs/system-architecture.md`

## 验证方式

### 编译/测试

- `cd services/api && go test ./internal/services/playback ./internal/handlers`
- `cd services/api && go build ./...`
- `cd services/web && npm run build`

### 手工验证

- 管理员进入总览页，默认按累计播放时长倒序展示用户列表
- 切换 `7d / 30d / 90d / all` 后，总览页摘要与表格联动变化
- 搜索用户名后，总览列表与顶部摘要同步收敛
- 从总览页点 `查看画像` 可进入单用户画像页
- 从总览页点 `查看播放历史` 可带用户名跳到播放历史页

## 落地后文档处理

- 将新增页面、接口和菜单职责补充到 `docs/system-architecture.md`
- 方案稳定后移入 `docs/archive/plan/`
