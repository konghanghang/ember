# P2-3: 媒体库列表查询（Library List）

## 功能描述

获取 Emby 所有媒体库（Views），显示媒体库 ID、名称、类型，用于权限配置辅助。

**优先级**：P2（可选功能）⭐⭐

---

## API 端点设计

```
GET /api/libraries
Response:
{
  "data": [
    {
      "id": "abc123",
      "name": "电影",
      "type": "movies",
      "itemCount": 500
    },
    {
      "id": "def456",
      "name": "电视剧",
      "type": "tvshows",
      "itemCount": 200
    }
  ]
}
```

---

## 核心逻辑

```go
func (s *EmbyService) GetLibraries(ctx context.Context) ([]Library, error) {
    url := fmt.Sprintf("%s/Library/VirtualFolders?api_key=%s", s.baseURL, s.apiKey)

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, err
    }

    resp, err := s.httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var libraries []Library
    if err := json.NewDecoder(resp.Body).Decode(&libraries); err != nil {
        return nil, err
    }

    return libraries, nil
}
```

---

## 实施清单

- [ ] 实现 Emby 媒体库查询
- [ ] 实现 API 端点
- [ ] 更新前端权限配置页面

**预计工作量**：0.5 天
