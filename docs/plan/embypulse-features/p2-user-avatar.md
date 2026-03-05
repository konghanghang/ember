# P2-2: 用户头像管理（User Avatar Management）

## 功能描述

管理员可为用户上传头像（URL 或本地文件），自动转换为 Base64 并推送到 Emby。

**优先级**：P2（可选功能）⭐⭐

---

## API 端点设计

```
POST /api/users/:userId/avatar
Body:
{
  "avatarUrl": "https://example.com/avatar.jpg"
}
或
Content-Type: multipart/form-data
Body: file

Response:
{
  "message": "头像更新成功"
}
```

---

## 核心逻辑

```go
func (s *UserService) UpdateAvatar(ctx context.Context, userID uint, avatarData []byte) error {
    // 1. 转换为 Base64
    base64Avatar := base64.StdEncoding.EncodeToString(avatarData)

    // 2. 获取用户
    var user models.User
    if err := s.db.WithContext(ctx).First(&user, userID).Error; err != nil {
        return err
    }

    // 3. 推送到 Emby
    return s.embyService.UpdateUserAvatar(ctx, user.EmbyUserID, base64Avatar)
}
```

---

## 实施清单

- [ ] 实现头像上传 API
- [ ] 实现 Emby 头像更新
- [ ] 更新前端用户管理页面

**预计工作量**：1 天
