# P2-3: 媒体库列表查询（Library List）

## 功能描述

获取 Emby 媒体库（VirtualFolders）列表，用于权限配置与质量盘点等功能的基础数据。

**优先级**：P2

---

## Ember 对齐要点

1. 管理员接口：`/api/v1/admin/libraries`
2. 复用 Emby `/emby/Library/VirtualFolders`
3. 返回字段标准化：`id/name/type/itemCount`
4. 兼容 Emby 不同版本字段名（`Guid`/`ItemId`）

---

## API 端点设计

`GET /api/v1/admin/libraries`

Response:

```json
{
  "data": [
    {
      "id": "abc123",
      "name": "电影",
      "type": "movies",
      "itemCount": 500
    }
  ]
}
```

---

## 核心逻辑建议

```go
func (s *EmbyService) GetLibraries() ([]Library, error)
```

实现步骤：

1. 请求 `/emby/Library/VirtualFolders`
2. 读取 `Guid`（缺失时回退 `ItemId`）作为 `id`
3. 读取 `Name`、`CollectionType`
4. `itemCount` 可选：
- 第一版允许返回 `0`
- 后续可按库查询补全

---

## 验证清单

- [ ] 媒体库列表可正常返回
- [ ] 字段映射稳定（不同 Emby 版本）
- [ ] 未配置 Emby 时返回清晰错误

**预计工作量**：0.5-1 天
