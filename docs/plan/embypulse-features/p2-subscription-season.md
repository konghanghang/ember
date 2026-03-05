# P2-1: 求片分季支持（Subscription Season Support）

## 功能描述

支持电视剧按季求片，例如“权力的游戏 第 3 季”。

**优先级**：P2

---

## Ember 对齐要点

1. 复用现有 `subscriptions` 表，不新建 `media_subscriptions`
2. 与现有字段命名保持一致：`type/name/tmdbId`
3. 新增 `season` 字段时保持向后兼容（默认 `0` 表示整剧）
4. 去重语义将从“全局去重(type+tmdbId)”调整为“按季去重(type+tmdbId+season)”（属于可见行为变更，需发布说明）

---

## 数据模型设计（扩展 Subscription）

```go
type Subscription struct {
    ID        string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
    UserID    string    `json:"userId" gorm:"column:userId;type:varchar(25);not null;index"`
    Type      string    `json:"type" gorm:"column:type;type:varchar(10);not null"`
    Name      string    `json:"name" gorm:"column:name;size:255;not null"`
    TmdbID    string    `json:"tmdbId" gorm:"column:tmdbId;size:50;not null"`
    Season    int       `json:"season" gorm:"column:season;not null;default:0"` // 0=整剧
    CreatedAt time.Time `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
}
```

唯一索引建议：
- 由现有 `(type, tmdbId)` 升级为 `(type, tmdbId, season)`

迁移注意：
- 需要显式删除旧唯一索引并创建新索引（仅靠 AutoMigrate 不可靠）

---

## API 端点设计

沿用现有创建接口：`POST /api/v1/subscriptions`

```json
{
  "type": "TV",
  "name": "Game of Thrones",
  "tmdbId": "1399",
  "season": 3
}
```

- 不传 `season` 时，后端默认 `0`（整剧）

---

## 核心实现建议

1. 扩展 `CreateSubscriptionRequest`，新增 `season` 可选字段
2. 去重逻辑改为 `type + tmdbId + season`
3. MoviePilot 推送时（第一版确定策略）：
- `season=0` 不传季参数
- `season>0` 先降级为整剧订阅

4. 需同步扩展 `MoviePilotClient` 请求结构：
- 当前 `SubscribeRequest` 仅支持 `type/name/tmdbid`，无 `season` 字段
- 第一版仅在 Ember 侧记录 `season`，推送时在 `mpError` 标注“季参数未透传（已降级整剧）”
- 后续若上游 MoviePilot API 明确支持季参数，再升级为透传

---

## 验证清单

- [ ] 同剧不同季可分别提交
- [ ] 同剧同季重复提交被拦截
- [ ] 未传季数时行为与当前一致
- [ ] 审批/拒绝流程不受影响

**预计工作量**：1-2 天
