# P0-3: 权限模板机制（Permission Template）

## 功能描述

创建兑换码时绑定模板用户 ID，新用户注册时自动复制模板用户权限。支持媒体库访问、下载权限、家长控制等全部权限的自动配置。

**核心价值**：
- 简化权限配置流程
- 确保权限一致性
- 支持多种用户类型（VIP、普通、试用等）

**优先级**：P0（强烈推荐）⭐⭐⭐⭐⭐

---

## 数据模型设计

### 扩展 Redemption Code 表

```go
// RedemptionCode 兑换码（扩展）
type RedemptionCode struct {
    // ... 现有字段 ...
    TemplateUserID *uint  `gorm:"column:template_user_id;index" json:"templateUserId"` // 新增：模板用户 ID
    TemplateUser   *User  `gorm:"foreignKey:TemplateUserID" json:"templateUser,omitempty"`
}
```

---

## API 端点设计

### 1. 创建兑换码（扩展）

```
POST /api/redemption-codes
Body:
{
  "code": "VIP2026",
  "duration": 30,
  "maxUses": 10,
  "templateUserId": 5  // 新增：模板用户 ID
}

Response:
{
  "message": "兑换码创建成功",
  "data": {
    "id": 1,
    "code": "VIP2026",
    "templateUserId": 5,
    "templateUser": {
      "id": 5,
      "username": "vip_template",
      "embyUsername": "vip_template"
    }
  }
}
```

### 2. 获取模板用户列表

```
GET /api/users/templates
Response:
{
  "data": [
    {
      "id": 5,
      "username": "vip_template",
      "embyUsername": "vip_template",
      "description": "VIP 用户模板"
    },
    {
      "id": 6,
      "username": "trial_template",
      "embyUsername": "trial_template",
      "description": "试用用户模板"
    }
  ]
}
```

---

## 核心逻辑实现

### 兑换码使用时复制权限

```go
// services/api/internal/services/redemption_service.go

func (s *RedemptionService) UseCode(ctx context.Context, code string, userID uint) error {
    // 1. 查找兑换码
    var redemptionCode models.RedemptionCode
    if err := s.db.WithContext(ctx).
        Preload("TemplateUser").
        Where("code = ?", code).
        First(&redemptionCode).Error; err != nil {
        return err
    }

    // 2. 验证兑换码有效性
    // ... 现有验证逻辑 ...

    // 3. 获取用户
    var user models.User
    if err := s.db.WithContext(ctx).First(&user, userID).Error; err != nil {
        return err
    }

    // 4. 如果有模板用户，复制权限
    if redemptionCode.TemplateUserID != nil {
        if err := s.copyPermissionsFromTemplate(ctx, &user, redemptionCode.TemplateUser); err != nil {
            return err
        }
    }

    // 5. 延长到期时间
    // ... 现有逻辑 ...

    return nil
}

// copyPermissionsFromTemplate 从模板用户复制权限
func (s *RedemptionService) copyPermissionsFromTemplate(ctx context.Context, targetUser *models.User, templateUser *models.User) error {
    // 1. 获取模板用户的 Emby 权限
    templateEmbyUser, err := s.embyService.GetUser(ctx, templateUser.EmbyUserID)
    if err != nil {
        return err
    }

    // 2. 获取目标用户的 Emby 用户
    targetEmbyUser, err := s.embyService.GetUser(ctx, targetUser.EmbyUserID)
    if err != nil {
        return err
    }

    // 3. 复制权限配置
    targetEmbyUser.Policy = templateEmbyUser.Policy
    targetEmbyUser.Configuration = templateEmbyUser.Configuration

    // 4. 更新到 Emby
    if err := s.embyService.UpdateUser(ctx, targetEmbyUser); err != nil {
        return err
    }

    return nil
}
```

### Emby 服务扩展

```go
// services/api/internal/services/emby_service.go

type EmbyUserPolicy struct {
    IsAdministrator              bool     `json:"IsAdministrator"`
    IsHidden                     bool     `json:"IsHidden"`
    IsDisabled                   bool     `json:"IsDisabled"`
    EnableAllFolders             bool     `json:"EnableAllFolders"`
    EnabledFolders               []string `json:"EnabledFolders"`
    EnableContentDownloading     bool     `json:"EnableContentDownloading"`
    EnableMediaPlayback          bool     `json:"EnableMediaPlayback"`
    EnablePublicSharing          bool     `json:"EnablePublicSharing"`
    MaxParentalRating            int      `json:"MaxParentalRating"`
    BlockedTags                  []string `json:"BlockedTags"`
    EnableRemoteAccess           bool     `json:"EnableRemoteAccess"`
    EnableLiveTvAccess           bool     `json:"EnableLiveTvAccess"`
    EnableLiveTvManagement       bool     `json:"EnableLiveTvManagement"`
    EnableSharedDeviceControl    bool     `json:"EnableSharedDeviceControl"`
    EnableRemoteControlOfOtherUsers bool  `json:"EnableRemoteControlOfOtherUsers"`
}

type EmbyUserConfiguration struct {
    PlayDefaultAudioTrack        bool     `json:"PlayDefaultAudioTrack"`
    SubtitleLanguagePreference   string   `json:"SubtitleLanguagePreference"`
    DisplayMissingEpisodes       bool     `json:"DisplayMissingEpisodes"`
    GroupedFolders               []string `json:"GroupedFolders"`
    SubtitleMode                 string   `json:"SubtitleMode"`
    DisplayCollectionsView       bool     `json:"DisplayCollectionsView"`
    EnableLocalPassword          bool     `json:"EnableLocalPassword"`
    OrderedViews                 []string `json:"OrderedViews"`
    LatestItemsExcludes          []string `json:"LatestItemsExcludes"`
    MyMediaExcludes              []string `json:"MyMediaExcludes"`
    HidePlayedInLatest           bool     `json:"HidePlayedInLatest"`
}

type EmbyUser struct {
    Id            string                 `json:"Id"`
    Name          string                 `json:"Name"`
    Policy        EmbyUserPolicy         `json:"Policy"`
    Configuration EmbyUserConfiguration  `json:"Configuration"`
}

// GetUser 获取 Emby 用户详细信息
func (s *EmbyService) GetUser(ctx context.Context, embyUserID string) (*EmbyUser, error) {
    url := fmt.Sprintf("%s/Users/%s?api_key=%s", s.baseURL, embyUserID, s.apiKey)

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, err
    }

    resp, err := s.httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var user EmbyUser
    if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
        return nil, err
    }

    return &user, nil
}

// UpdateUser 更新 Emby 用户
func (s *EmbyService) UpdateUser(ctx context.Context, user *EmbyUser) error {
    url := fmt.Sprintf("%s/Users/%s?api_key=%s", s.baseURL, user.Id, s.apiKey)

    body, err := json.Marshal(user)
    if err != nil {
        return err
    }

    req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
    if err != nil {
        return err
    }
    req.Header.Set("Content-Type", "application/json")

    resp, err := s.httpClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
        return fmt.Errorf("Emby API error: %d", resp.StatusCode)
    }

    return nil
}
```

---

## 前端页面设计

### 兑换码创建页面（扩展）

```vue
<!-- services/web/src/views/redemption-codes/create.vue -->
<template>
  <el-form :model="form" label-width="120px">
    <!-- 现有字段 -->
    <el-form-item label="兑换码">
      <el-input v-model="form.code" />
    </el-form-item>
    <el-form-item label="有效期（天）">
      <el-input-number v-model="form.duration" :min="1" />
    </el-form-item>
    <el-form-item label="最大使用次数">
      <el-input-number v-model="form.maxUses" :min="1" />
    </el-form-item>

    <!-- 新增：模板用户选择 -->
    <el-form-item label="权限模板">
      <el-select v-model="form.templateUserId" placeholder="选择权限模板（可选）">
        <el-option label="无模板" :value="null" />
        <el-option
          v-for="template in templates"
          :key="template.id"
          :label="`${template.username} - ${template.description}`"
          :value="template.id"
        />
      </el-select>
      <div class="form-tip">
        选择模板后，使用此兑换码的用户将自动获得模板用户的所有权限配置
      </div>
    </el-form-item>

    <el-form-item>
      <el-button type="primary" @click="createCode">创建兑换码</el-button>
    </el-form-item>
  </el-form>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { redemptionCodeApi, userApi } from '@/api'

const form = ref({
  code: '',
  duration: 30,
  maxUses: 1,
  templateUserId: null
})

const templates = ref([])

const fetchTemplates = async () => {
  const res = await userApi.getTemplates()
  templates.value = res.data
}

const createCode = async () => {
  await redemptionCodeApi.create(form.value)
  ElMessage.success('兑换码创建成功')
}

onMounted(() => {
  fetchTemplates()
})
</script>

<style scoped>
.form-tip {
  font-size: 12px;
  color: #909399;
  margin-top: 5px;
}
</style>
```

---

## 验证方式

### 1. 数据库迁移

```bash
cd services/api
go run cmd/migrate/main.go create add_template_user_to_redemption_codes
```

迁移脚本：
```go
func up(tx *gorm.DB) error {
    return tx.Exec(`
        ALTER TABLE redemption_codes
        ADD COLUMN template_user_id INTEGER REFERENCES users(id);

        CREATE INDEX idx_redemption_codes_template_user_id
        ON redemption_codes(template_user_id);
    `).Error
}
```

### 2. 创建模板用户

```bash
# 在 Emby 中手动创建模板用户
# 1. 创建用户 "vip_template"
# 2. 配置所有需要的权限（媒体库访问、下载权限等）
# 3. 在 Ember 中关联该用户
```

### 3. API 测试

```bash
# 获取模板用户列表
curl "http://localhost:8080/api/users/templates" \
  -H "Authorization: Bearer <admin_token>"

# 创建带模板的兑换码
curl -X POST http://localhost:8080/api/redemption-codes \
  -H "Authorization: Bearer <admin_token>" \
  -d '{
    "code": "VIP2026",
    "duration": 30,
    "maxUses": 10,
    "templateUserId": 5
  }'

# 使用兑换码（验证权限复制）
curl -X POST http://localhost:8080/api/redemption-codes/use \
  -H "Authorization: Bearer <user_token>" \
  -d '{"code": "VIP2026"}'
```

### 4. 验证权限复制

1. 创建一个新用户
2. 使用带模板的兑换码
3. 登录 Emby 验证该用户的权限配置
4. 确认媒体库访问、下载权限等与模板用户一致

---

## 实施清单

- [ ] 扩展 RedemptionCode 模型（添加 template_user_id 字段）
- [ ] 编写数据库迁移脚本
- [ ] 实现权限复制逻辑
- [ ] 扩展 Emby 服务（GetUser、UpdateUser）
- [ ] 实现模板用户列表 API
- [ ] 扩展兑换码创建 API
- [ ] 更新前端兑换码创建页面
- [ ] 创建模板用户（VIP、普通、试用等）
- [ ] 编写单元测试
- [ ] 更新系统架构文档

**预计工作量**：2-3 天
