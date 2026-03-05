# P2-2: 用户头像管理（User Avatar Management）

## 功能描述

管理员可为用户上传头像（URL 或文件），并同步到 Emby 用户头像。

**优先级**：P2

---

## Ember 对齐要点

1. 管理员接口：`/api/v1/admin/users/:id/avatar`
2. `userId` 类型为 `string`
3. 对齐上游服务实现（参考 emby-pulse）：`DELETE + POST /emby/Users/{id}/Images/Primary`
4. 上传前做图片大小与 MIME 校验

---

## API 端点设计

`POST /api/v1/admin/users/:id/avatar`

支持两种输入：

1. JSON
```json
{
  "avatarUrl": "https://example.com/avatar.jpg"
}
```

2. `multipart/form-data`
- `file`: 图片文件

Response:

```json
{
  "message": "头像更新成功"
}
```

---

## 核心逻辑建议

```go
func (s *UserService) UpdateAvatar(ctx context.Context, userID string, avatarData []byte, contentType string) error
```

关键步骤：

1. 校验用户存在且 `embyId` 非空
2. 校验图片格式与大小（例如 <= 2MB）
3. 转 Base64
4. 调用 Emby：
- `DELETE /emby/Users/{embyId}/Images/Primary`
- `POST /emby/Users/{embyId}/Images/Primary`

第一版固定策略：
- 上传体采用 Base64（二进制先编码后提交）
- 若后续发现特定 Emby 版本兼容问题，再补 raw bytes 兜底分支

---

## EmbyService 扩展建议

```go
func (s *EmbyService) UpdateUserAvatar(embyUserID string, imageBase64 []byte, contentType string) error
```

---

## 前端改动建议

在 `/console/users` 的用户管理表中新增：
- 上传头像按钮
- URL 填写入口
- 上传结果提示

---

## 验证清单

- [ ] URL 上传可用
- [ ] 本地文件上传可用
- [ ] 非图片文件会被拒绝
- [ ] Emby 同步失败时错误信息清晰

**预计工作量**：1 天
