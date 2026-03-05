# EmbyPulse 功能借鉴规划（Ember 对齐版）

## 📋 概述

本目录用于规划从 EmbyPulse 借鉴到 Ember 的功能。本文档已按 Ember 当前架构完成第一轮对齐修订，重点保证：

1. **不破坏现有行为**（Never break userspace）
2. **与现有数据模型一致**（`string` 主键 + camelCase 列名）
3. **与现有路由体系一致**（统一 ` /api/v1 ` + 前端 ` /console/* `）
4. **优先复用已有能力**（`EmbyService`、`QueryPlaybackStats`、`/admin/sessions` 等）

---

## ✅ 对齐基线（必须遵守）

### 数据与模型

- Ember 主键统一为 `string`（CUID），**不使用 `uint` 自增主键**
- GORM 字段必须显式 `gorm:"column:xxx"`，列名使用 camelCase
- 新增字段优先“向后兼容默认值”，避免影响旧数据读写

### API 与路由

- 后端统一前缀：`/api/v1`
- 管理员接口统一挂载：`/api/v1/admin/*`
- 用户鉴权接口统一挂载：`/api/v1/*`（`JWTAuth`）
- 前端页面统一挂载：`/console/*`

### 外部依赖

- 与播放记录相关功能（播放历史、用户画像）依赖 Emby Playback Reporting 插件
- 与追剧日历相关功能依赖 TMDB API Key
- 新 Webhook 需复用现有安全模式（token/secret 校验）

---

## 🎯 功能优先级（修订后）

### P0（建议先做）

1. [追剧日历（TV Calendar）](./p0-tv-calendar.md)
2. [客户端设备管理（Client Device Management）](./p0-device-management.md)
3. [权限模板机制（Permission Template）](./p0-permission-template.md)

### P1（高价值）

4. [媒体库质量盘点（Media Quality Insight）](./p1-media-quality.md)
5. [播放历史查询（Playback History）](./p1-playback-history.md)
6. [用户画像（User Profile Analytics）](./p1-user-profile.md)

### P2（可选）

7. [求片分季支持（Subscription Season Support）](./p2-subscription-season.md)
8. [用户头像管理（User Avatar Management）](./p2-user-avatar.md)
9. [媒体库列表查询（Library List）](./p2-library-list.md)
10. [兑换码备注字段（Redemption Code Notes）](./p2-code-notes.md)

---

## 📅 实施建议（修订后）

### 阶段 1：P0（2-3 周）

1. 追剧日历（7-10 天）
2. 设备管理（4-6 天）
3. 权限模板（3-5 天）

### 阶段 2：P1（1-2 周）

1. 播放历史（2-3 天）
2. 媒体质量（3-4 天）
3. 用户画像（3-4 天）

### 阶段 3：P2（3-5 天）

按需实施。

---

## ⚠️ 已知关键风险

1. **权限模板越权风险**：禁止整包复制模板用户权限，必须白名单复制
2. **播放历史注入风险**：禁止直接 `fmt.Sprintf` 拼接用户输入 SQL
3. **路由不一致风险**：禁止新增 ` /api/... ` 无版本前缀接口
4. **迁移方式偏差**：当前项目无 `cmd/migrate`，需按现有迁移机制扩展
5. **路由冲突风险**：Gin 中静态路由与参数路由同层注册顺序不当会冲突（如 `/users/:id` 与 `/users/templates`）
6. **索引迁移风险**：求片分季需显式迁移旧唯一索引，否则新去重语义不会生效
7. **上游能力边界**：分季推送第一版采用“降级整剧订阅”策略，并记录 `mpError`

---

## 📝 文档结构

```
docs/plan/embypulse-features/
├── README.md
├── p0-tv-calendar.md
├── p0-device-management.md
├── p0-permission-template.md
├── p1-media-quality.md
├── p1-playback-history.md
├── p1-user-profile.md
├── p2-subscription-season.md
├── p2-user-avatar.md
├── p2-library-list.md
└── p2-code-notes.md
```

---

## 🔗 相关文档

- [系统架构文档](../../SYSTEM-ARCHITECTURE.md)
- [API 响应标准](../../API-RESPONSE-STANDARD.md)
- [开发指南](../../development-guide.md)
