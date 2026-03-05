# P2-1: 求片分季支持（Subscription Season Support）

## 功能描述

支持按季度求片（如"权力的游戏 第 3 季"），主键设计：(userId, tmdbId, season) 联合唯一约束，自动检测 Emby 库存（按季度）。

**优先级**：P2（可选功能）⭐⭐⭐

---

## 数据模型设计

```go
// MediaSubscription 扩展（添加 season 字段）
type MediaSubscription struct {
    ID        uint      `gorm:"primaryKey;column:id" json:"id"`
    UserID    uint      `gorm:"column:user_id;not null;index" json:"userId"`
    TMDBId    int       `gorm:"column:tmdb_id;not null;index" json:"tmdbId"`
    Season    *int      `gorm:"column:season;index" json:"season"` // 新增：季数（NULL 表示整部剧集）
    MediaType string    `gorm:"column:media_type;size:20" json:"mediaType"`
    Title     string    `gorm:"column:title;size:255" json:"title"`
    Status    string    `gorm:"column:status;size:20;default:'pending'" json:"status"`
    CreatedAt time.Time `gorm:"column:created_at" json:"createdAt"`
}

// 联合唯一索引
// CREATE UNIQUE INDEX idx_subscription_unique ON media_subscriptions(user_id, tmdb_id, season);
```

---

## API 端点设计

```
POST /api/subscriptions
Body:
{
  "tmdbId": 1399,
  "mediaType": "tv",
  "title": "Game of Thrones",
  "season": 3  // 可选，不传表示整部剧集
}

Response:
{
  "message": "求片成功"
}
```

---

## 实施清单

- [ ] 扩展 MediaSubscription 模型
- [ ] 编写数据库迁移脚本
- [ ] 更新求片 API
- [ ] 更新库存检测逻辑（按季度）
- [ ] 更新前端求片页面

**预计工作量**：1-2 天
