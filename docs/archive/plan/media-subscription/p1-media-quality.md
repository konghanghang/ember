# P1-1: 媒体库质量盘点（Media Quality Insight）

## 功能描述

扫描指定媒体库（或全部媒体库），统计分辨率、编码、HDR 分布，并给出低画质资源清单供洗版参考。

**优先级**：P1

---

## Ember 对齐要点

1. 管理员接口：`/api/v1/admin/media-quality/*`
2. 缓存持久化使用 PostgreSQL，不使用进程内全局变量作为唯一缓存
3. 字段命名 camelCase，ID 类型 `string`
4. 扫描按媒体库维度执行，支持强制刷新；`libraryId=all` 支持全库汇总

---

## 数据模型设计

```go
type MediaQualityCache struct {
    ID         string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
    LibraryID  string    `json:"libraryId" gorm:"column:libraryId;size:50;uniqueIndex;not null"`
    Statistics string    `json:"statistics" gorm:"column:statistics;type:text;not null"`
    ExpiresAt  time.Time `json:"expiresAt" gorm:"column:expiresAt;index"`
    CreatedAt  time.Time `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
    UpdatedAt  time.Time `json:"updatedAt" gorm:"column:updatedAt;autoUpdateTime"`
}
```

---

## API 端点设计

### 1. 查询媒体库质量报告（管理员）

`GET /api/v1/admin/media-quality/libraries/:libraryId`

Query：
- `force`（可选，默认 `false`）
- `page`（可选，默认 `1`）
- `pageSize`（可选，默认 `20`，最大 `100`）

说明：`force=false` 时优先返回缓存。
低画质清单按“影片/剧集”汇总后再分页返回，避免单集展开导致噪音过大。

### 2. 触发媒体库质量扫描（管理员）

`POST /api/v1/admin/media-quality/libraries/:libraryId/scan`

说明：用于显式触发重扫，避免“GET 带副作用”。

### 3. 查询低画质汇总项明细（管理员）

`GET /api/v1/admin/media-quality/libraries/:libraryId/groups/:groupId/details`

Query：
- `force`（可选，默认 `false`）
- `page`（可选，默认 `1`）
- `pageSize`（可选，默认 `20`，最大 `100`）

Response:

```json
{
  "data": {
    "resolutionDistribution": [
      { "resolution": "4K", "count": 120 }
    ],
    "codecDistribution": [
      { "codec": "HEVC", "count": 80 }
    ],
    "hdrDistribution": [
      { "type": "HDR10", "count": 30 }
    ],
    "lowQualityItems": [
      {
        "id": "item_xxx",
        "groupId": "movie:item_xxx",
        "name": "Movie Name",
        "itemType": "Movie",
        "itemCount": 1,
        "posterItemId": "item_xxx",
        "resolution": "720p",
        "codec": "H.264",
        "bitrate": 2000
      }
    ],
    "lowQualityTotal": 1,
    "page": 1,
    "pageSize": 20,
    "scanAt": "2026-03-05T10:00:00Z"
  }
}
```

---

## 核心实现建议

```go
func (s *MediaQualityService) ScanLibraryQuality(ctx context.Context, libraryID string, force bool) (*QualityReport, error)
```

实现要点：

1. 先读缓存：`expiresAt > now` 且 `force=false` 直接返回
2. 再查 Emby 媒体条目，逐条提取 `MediaStreams`
3. 统计逻辑统一在服务层，不在 handler 拼装
4. 回写缓存时使用 upsert，避免重复行

---

## 前端页面设计

### 路由

```ts
{
  path: '/console/media-quality',
  name: 'console-media-quality',
  meta: { requiresAuth: true, role: 'admin' },
  component: () => import('@/views/admin/MediaQualityView.vue')
}
```

### 交互

- 选择媒体库
- 支持“全部媒体库”汇总分析
- 扫描/强制刷新
- 三组分布图 + 低画质汇总表格（分页）

---

## 验证清单

- [ ] 同一库重复扫描可命中缓存
- [ ] `force=true` 可绕过缓存
- [x] 无 `MediaStreams` 的条目不会导致接口失败
- [x] 低画质清单字段完整可展示

**预计工作量**：3-4 天
