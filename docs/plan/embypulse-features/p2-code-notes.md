# P2-4: 兑换码备注字段（Redemption Code Notes）

## 功能描述

为兑换码添加备注字段（如"给朋友 A"、"活动奖励"），方便追踪兑换码用途。

**优先级**：P2（可选功能）⭐⭐

---

## 数据模型设计

```go
// RedemptionCode 扩展（添加 notes 字段）
type RedemptionCode struct {
    // ... 现有字段 ...
    Notes string `gorm:"column:notes;size:500" json:"notes"` // 新增：备注
}
```

---

## API 端点设计

```
POST /api/redemption-codes
Body:
{
  "code": "VIP2026",
  "duration": 30,
  "maxUses": 10,
  "notes": "给朋友 A 的 VIP 兑换码"  // 新增
}

GET /api/redemption-codes
Response:
{
  "data": [
    {
      "id": 1,
      "code": "VIP2026",
      "notes": "给朋友 A 的 VIP 兑换码",
      ...
    }
  ]
}
```

---

## 实施清单

- [ ] 扩展 RedemptionCode 模型
- [ ] 编写数据库迁移脚本
- [ ] 更新兑换码创建 API
- [ ] 更新前端兑换码管理页面

**预计工作量**：0.5 天
