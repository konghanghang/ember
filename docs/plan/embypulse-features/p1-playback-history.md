# P1-2: 播放历史查询（Playback History）

## 功能描述

分页查询播放历史，支持按用户、关键词、日期范围过滤，展示播放时间、设备、客户端和播放时长。

**优先级**：P1

---

## 前提条件

- Emby 已安装 Playback Reporting 插件
- Ember 已正确配置 `EMBY_URL` 与 `EMBY_API_KEY`

---

## Ember 对齐要点

1. 接口建议放在管理员域：`/api/v1/admin/playback-history`
2. `userId` 类型为 `string`
3. 数据源使用 `PlaybackActivity`（与现有排行榜一致）
4. 禁止直接拼接用户输入 SQL，必须白名单校验 + 严格转义

---

## API 端点设计

`GET /api/v1/admin/playback-history`

Query:
- `userId`（可选，string）
- `keyword`（可选）
- `startDate`（可选，`YYYY-MM-DD`）
- `endDate`（可选，`YYYY-MM-DD`）
- `page`（默认 1）
- `pageSize`（默认 20）

Response:

```json
{
  "data": [
    {
      "userId": "cuid_user",
      "username": "john",
      "itemName": "Game of Thrones S03E05",
      "itemType": "Episode",
      "playedAt": "2026-03-05T20:30:00Z",
      "deviceName": "iPhone 13",
      "clientName": "Emby for iOS",
      "playDuration": 3600,
      "playDurationFormatted": "1h 0m"
    }
  ],
  "total": 500,
  "page": 1,
  "pageSize": 20
}
```

---

## 核心逻辑建议

```go
func (s *PlaybackHistoryService) GetPlaybackHistory(ctx context.Context, req PlaybackHistoryRequest) (*PlaybackHistoryResponse, error)
```

实现要点：

1. 使用两条 SQL：
- `COUNT(*)` 统计总数
- 明细分页查询

2. 拼接条件时：
- `userId`、日期、分页参数必须做格式校验
- `keyword` 需限制字符集与长度（例如仅允许中英文、数字、空格、`-_.'`，长度 <= 100），并做单引号转义

3. 统一时长格式化函数：
- `< 1h` 显示 `Xm`
- `>= 1h` 显示 `Xh Ym`

4. 当插件不可用时返回明确错误：
- `Playback Reporting 查询失败`

---

## 前端页面设计

### 路由

```ts
{
  path: '/console/playback-history',
  name: 'console-playback-history',
  meta: { requiresAuth: true, role: 'admin' },
  component: () => import('@/views/admin/PlaybackHistoryView.vue')
}
```

### 页面能力

- 用户筛选
- 关键词搜索
- 日期范围
- 分页表格

---

## 验证清单

- [x] 条件筛选结果正确
- [x] 总数与分页数据一致
- [x] SQL 注入输入不会导致异常查询
- [x] 插件不可用时返回可读错误

**预计工作量**：2-3 天
