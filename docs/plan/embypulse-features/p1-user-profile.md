# P1-3: 用户画像（User Profile Analytics）

## 功能描述

基于播放历史计算用户画像：
- 24 小时观影分布
- 设备分布
- 趣味勋章（修仙党、周末狂欢、Emby 肝帝）

**优先级**：P1

---

## 前提条件

- Emby 已安装 Playback Reporting 插件
- `PlaybackActivity` 数据可查询

---

## Ember 对齐要点

1. `userId` 使用 `string`，通过本地 `users` 表映射到 `embyId`
2. 优先复用播放统计能力（与排行榜同源）
3. 提供管理员视角和用户自身视角两类接口
4. 勋章规则可配置，避免硬编码到前端

---

## API 端点设计

### 1. 管理员查看任意用户画像

`GET /api/v1/admin/users/:userId/profile`

### 2. 当前用户查看自己的画像

`GET /api/v1/profile/analytics`

Response:

```json
{
  "data": {
    "hourlyDistribution": [
      { "hour": 0, "count": 5 },
      { "hour": 1, "count": 2 }
    ],
    "deviceDistribution": [
      { "deviceName": "iPhone", "count": 50 }
    ],
    "badges": [
      {
        "id": "night_owl",
        "name": "修仙党",
        "description": "凌晨 2-6 点观影次数达到阈值"
      }
    ],
    "totalPlayCount": 500,
    "totalPlayDuration": 360000,
    "favoriteGenre": null
  }
}
```

---

## 核心逻辑建议

```go
func (s *UserProfileService) GetUserProfile(ctx context.Context, userID string) (*UserProfile, error)
```

### 勋章规则（第一版）

- `night_owl`：凌晨 2-5 点播放次数 >= 10
- `weekend_warrior`：周末播放次数 >= 20
- `hardcore_viewer`：总播放时长 >= 100 小时

说明：
- 第一版优先保证稳定和可解释性
- `favoriteGenre` 若无可靠数据源可返回 `null`

---

## 前端页面设计

### 路由

```ts
{
  path: '/console/profile-analytics',
  name: 'console-profile-analytics',
  meta: { requiresAuth: true },
  component: () => import('@/views/console/ProfileAnalyticsView.vue')
}
```

### 页面模块

- 小时分布图
- 设备分布图
- 勋章墙
- 统计摘要

---

## 验证清单

- [ ] 指定用户与当前用户视图都可正常返回
- [ ] 24 小时分布包含 0-23 全量桶
- [ ] 勋章阈值命中逻辑可复现
- [ ] 无播放数据用户返回空画像而非报错

**预计工作量**：3-4 天
