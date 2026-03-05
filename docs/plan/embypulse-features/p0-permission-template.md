# P0-3: 权限模板机制（Permission Template）

## 功能描述

在创建兑换码时绑定模板用户（`templateUserId`），用户使用该兑换码注册后，自动继承模板权限（媒体库访问、下载、转码、分级等）。

**优先级**：P0

---

## Ember 对齐要点

1. `templateUserId` 类型使用 `*string`，关联 `users.id`
2. 兑换码接口复用现有 `/api/v1/admin/redemption-codes`
3. 权限复制发生在注册流程（`AuthService.RegisterUser` 的 invite 分支）
4. 禁止整包复制策略，必须白名单字段复制

---

## 数据模型设计

### 扩展 `redemption_codes`

```go
type RedemptionCode struct {
    ID             string     `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
    Code           string     `json:"code" gorm:"column:code;uniqueIndex;size:20;not null"`
    MaxUses        int        `json:"maxUses" gorm:"column:maxUses;not null;default:1"`
    UsedCount      int        `json:"usedCount" gorm:"column:usedCount;not null;default:0"`
    ExpiresAt      *time.Time `json:"expiresAt,omitempty" gorm:"column:expiresAt"`
    DefaultDays    int        `json:"defaultDays" gorm:"column:defaultDays;not null;default:30"`
    TemplateUserID *string    `json:"templateUserId,omitempty" gorm:"column:templateUserId;size:25;index"`
    CreatedAt      time.Time  `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
}
```

---

## API 端点设计

### 1. 创建兑换码（扩展）

`POST /api/v1/admin/redemption-codes`

```json
{
  "maxUses": 10,
  "defaultDays": 30,
  "expiresAt": "2026-12-31T00:00:00Z",
  "templateUserId": "cuid_template_user"
}
```

### 2. 更新兑换码（扩展）

`PUT /api/v1/admin/redemption-codes/:id`

支持更新 `templateUserId`（可置空）。

### 3. 模板用户列表（管理员）

`GET /api/v1/admin/user-templates`

返回可作为模板的用户列表（建议只展示启用且非过期用户）。
说明：使用独立路径避免与现有 `GET /api/v1/admin/users/:id` 同层路由冲突。

---

## 核心逻辑实现

### 注册链路（invite 模式）

```go
// AuthService.RegisterUser
// 在创建 Emby 用户成功后，若 redemptionCode.TemplateUserID != nil，则复制模板权限到新用户
func (s *AuthService) applyTemplatePolicyIfNeeded(newEmbyID string, templateUserID *string) error
```

行为边界（避免破坏现有用户可见行为）：
- 仅在“邀请码注册”链路应用模板权限
- 已注册用户走 `RedeemCode` 续期时忽略 `templateUserId`

### 白名单复制策略（必须）

建议仅复制以下字段：
- `EnableAllFolders`
- `EnabledFolders`
- `ExcludedSubFolders`
- `EnableContentDownloading`
- `EnableSyncTranscoding`
- `EnableVideoPlaybackTranscoding`
- `EnablePlaybackRemuxing`
- `EnableAudioPlaybackTranscoding`
- `MaxParentalRating`

禁止复制：
- `IsAdministrator`
- `IsDisabled`
- 任何与安全审计/锁定相关字段

---

## EmbyService 扩展建议

当前已有：
- `GetUserByID`（简版）
- `SetUserPolicy`
- `ApplyEmberDefaultUserPolicy`

建议新增：

```go
// GetUserPolicyRaw 获取完整策略 map（内部可复用现有 getUserPolicyRaw）
func (s *EmbyService) GetUserPolicyRaw(embyUserID string) (map[string]any, error)

// PatchUserPolicyFields 仅按白名单字段覆盖目标用户策略
func (s *EmbyService) PatchUserPolicyFields(targetUserID string, sourcePolicy map[string]any, fields []string) error
```

---

## 前端改动建议

在管理员兑换码页面新增：
- `templateUserId` 下拉选择（可选）
- 模板用户搜索（按用户名）
- 模板信息展示（有效期、角色、备注）

路由沿用现有：`/console/redemption-codes`
并同步更新前端类型：`CreateRedemptionCodeRequest` / `UpdateRedemptionCodeRequest` / `RedemptionCode`

---

## 验证清单

- [ ] 绑定模板的兑换码创建成功
- [ ] 使用该码注册后权限正确继承
- [ ] 模板用户不存在时注册失败并返回清晰错误
- [ ] 不会复制管理员权限
- [ ] 未绑定模板的兑换码行为与当前一致

**预计工作量**：3-5 天
