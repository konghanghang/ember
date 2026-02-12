# 开放注册 + 兑换码续期 系统改造方案（第一期）

## Context

当前系统采用"邀请码注册"模式——用户必须持有有效邀请码才能注册。这适合内测阶段，但不适合长期运营。

本次改造目标：转为"可配置注册模式 + 统一兑换码"体系。管理员可在"开放注册"和"需要兑换码注册"之间切换。兑换码统一用于两种场景：作为注册门控（invite 模式下），以及作为已注册用户的续期工具。系统每日自动封禁过期 Emby 账号，但保留本项目登录能力。

**核心挑战**：当前普通用户没有本地密码，完全依赖 Emby API 认证（`auth.go:42-43`）。Emby 账号被封禁后用户无法登录。必须引入本地密码存储。

**第二期规划**（不在本次范围）：Email 验证系统（SMTP 配置、验证邮件、确认链接、重发机制）。

---

## 1. 数据模型变更

### 1.1 User 模型修改
**文件**: `services/api/internal/models/user.go`

变更点：
- `Password` 字段扩展为所有用户通用（注册时存储 bcrypt hash）
- 新增 `EmbyDisabled bool` — 追踪 Emby 是否被定时任务封禁
- `Email` 字段添加唯一约束（防滥用基础措施）
- 移除 `InviteCode` 字段

```go
type User struct {
    ID           string     `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
    Username     string     `json:"username" gorm:"column:username;uniqueIndex;size:50;not null"`
    Role         string     `json:"role" gorm:"column:role;size:10;not null;default:user"`
    Password     string     `json:"-" gorm:"column:password"`
    Email        string     `json:"email,omitempty" gorm:"column:email;size:255;uniqueIndex"`
    EmbyID       string     `json:"embyId,omitempty" gorm:"column:embyId;size:50;index"`
    EmbyDisabled bool       `json:"embyDisabled" gorm:"column:embyDisabled;default:false;not null"`
    ExpiresAt    *time.Time `json:"expiresAt,omitempty" gorm:"column:expiresAt"`
    IsActive     bool       `json:"isActive" gorm:"column:isActive;default:true;not null"`
    CreatedAt    time.Time  `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
    UpdatedAt    time.Time  `json:"updatedAt" gorm:"column:updatedAt;autoUpdateTime"`
}
```

已有方法保留不变：`BeforeCreate`(CUID)、`IsExpired`、`IsAdmin`、`SetPassword`(bcrypt)、`CheckPassword`。

### 1.2 RedemptionCode 模型（统一码，替换 Invite）
**新文件**: `services/api/internal/models/redemption_code.go`
**删除**: `services/api/internal/models/invite.go`

同一个模型服务两种场景：
- `registration_mode = "invite"` 时：作为注册门控，初始有效期 = `defaultDays`
- 已注册用户兑换时：延长有效期 `defaultDays` 天

```go
type RedemptionCode struct {
    ID          string     `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
    Code        string     `json:"code" gorm:"column:code;uniqueIndex;size:20;not null"`
    MaxUses     int        `json:"maxUses" gorm:"column:maxUses;not null;default:1"`
    UsedCount   int        `json:"usedCount" gorm:"column:usedCount;not null;default:0"`
    ExpiresAt   *time.Time `json:"expiresAt,omitempty" gorm:"column:expiresAt"`
    DefaultDays int        `json:"defaultDays" gorm:"column:defaultDays;not null;default:30"`
    CreatedAt   time.Time  `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
}
// 表名: redemption_codes
```

方法：
- `BeforeCreate`：CUID 生成（与 Invite 一致）
- `IsValid() bool`：`UsedCount < MaxUses && (ExpiresAt == nil || ExpiresAt > now)`

### 1.3 Redemption 模型（兑换历史）
**新文件**: `services/api/internal/models/redemption.go`

```go
type Redemption struct {
    ID        string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
    UserID    string    `json:"userId" gorm:"column:userId;size:25;index;not null"`
    Code      string    `json:"code" gorm:"column:code;size:20;not null"`
    Days      int       `json:"days" gorm:"column:days;not null"`
    CreatedAt time.Time `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
}
// 表名: redemptions
```

### 1.4 Setting 模型（系统配置）
**新文件**: `services/api/internal/models/setting.go`

```go
type Setting struct {
    Key       string    `json:"key" gorm:"column:key;type:varchar(50);primaryKey"`
    Value     string    `json:"value" gorm:"column:value;size:500;not null"`
    UpdatedAt time.Time `json:"updatedAt" gorm:"column:updatedAt;autoUpdateTime"`
}
// 表名: settings
```

默认配置项（启动时 seed）：
- `default_trial_days`: `"7"` — 开放注册时的默认试用天数
- `registration_mode`: `"open"` — 注册模式，可选 `"open"` 或 `"invite"`

---

## 2. 后端服务层变更

### 2.1 auth.go — 认证服务
**文件**: `services/api/internal/services/auth.go`

**RegisterUserRequest 结构变更**：

```go
// 旧
type RegisterUserRequest struct {
    Username   string `json:"username" binding:"required,min=3,max=50"`
    Password   string `json:"password" binding:"required,min=6"`
    Email      string `json:"email" binding:"required,email"`
    InviteCode string `json:"inviteCode" binding:"required"`
}

// 新
type RegisterUserRequest struct {
    Username string `json:"username" binding:"required,min=3,max=50"`
    Password string `json:"password" binding:"required,min=6"`
    Email    string `json:"email" binding:"required,email"`
    Code     string `json:"code"` // 不再 required，rename from inviteCode
}
```

**注册流程伪代码**：

```
func RegisterUser(req):
    // 1. 读取注册模式
    mode = settingService.GetRegistrationMode()  // "open" or "invite"

    if mode == "invite":
        if req.Code == "":
            return error("当前为邀请注册模式，请提供兑换码")
        code, err = codeService.ValidateCode(req.Code)
        if err: return err  // "兑换码不存在" 或 "兑换码已失效"
        defaultDays = code.DefaultDays
    else:  // "open"
        defaultDays = settingService.GetDefaultTrialDays()  // 默认 7

    // 2. 检查用户名重复（不变）
    // 3. 检查邮箱重复（新增，email 有唯一索引）
    if emailExists: return error("邮箱已被注册")

    // 4. 创建 Emby 用户（不变）
    // 5. 计算有效期
    expiresAt = time.Now().AddDate(0, 0, defaultDays)

    // 6. 创建本地用户（新增本地密码存储）
    user = User{Username, Role:"user", Email, EmbyID, ExpiresAt, IsActive:true}
    user.SetPassword(req.Password)  // 新增！
    db.Create(&user)

    // 7. invite 模式：使用兑换码 + 记录兑换历史
    if mode == "invite":
        codeService.UseCode(req.Code)
        db.Create(&Redemption{UserID: user.ID, Code: req.Code, Days: defaultDays})

    // 8. 签发 Token（不变）
```

**登录流程伪代码**：

```
func Login(req):
    // 1. 查找用户（不变）
    user = db.Where("username = ?", req.Username).First()
    if not found: return error("用户名或密码错误")

    // 2. Admin 路径：本地密码校验（不变）
    if user.IsAdmin():
        if !user.CheckPassword(req.Password):
            return error("用户名或密码错误")
        goto ISSUE_TOKEN

    // 3. 普通用户 — 核心变化：
    //    - 移除 IsActive 检查（第36-38行）
    //    - 移除 IsExpired() 检查（第39-41行）
    //    过期用户可登录，看到过期提示 + 兑换入口

    // 3a. 有本地密码 → 本地校验
    if user.Password != "":
        if !user.CheckPassword(req.Password):
            return error("用户名或密码错误")
        goto ISSUE_TOKEN

    // 3b. 无本地密码（存量用户迁移）→ Emby 认证
    embyUser, err = embyService.AuthenticateUser(user.Username, req.Password)
    if err: return error("用户名或密码错误")
    if embyUser.ID != user.EmbyID: return error("用户信息不匹配")

    // 3c. Emby 认证成功，补存本地 hash（迁移）
    user.SetPassword(req.Password)
    db.Save(&user)  // 失败不阻塞登录，只记日志

ISSUE_TOKEN:
    token = GenerateToken(user.ID, user.Username, user.Role)
    return LoginResponse{Token, User}
```

### 2.2 setting.go（系统配置服务，新建）
**新文件**: `services/api/internal/services/setting.go`

```go
type SettingService struct{}

func (s *SettingService) GetSetting(key string) string
func (s *SettingService) SetSetting(key, value string) error
func (s *SettingService) GetAllSettings() ([]models.Setting, error)
func (s *SettingService) GetDefaultTrialDays() int    // 读取 + strconv.Atoi，默认 7
func (s *SettingService) GetRegistrationMode() string // 读取，默认 "open"
```

`SetSetting` 需要对特定 key 进行值校验：
- `registration_mode`：必须为 `"open"` 或 `"invite"`
- `default_trial_days`：必须为正整数

### 2.3 redemption_code.go（替换 invite.go）
**新文件**: `services/api/internal/services/redemption_code.go`
**删除**: `services/api/internal/services/invite.go`

```go
type RedemptionCodeService struct{}

type CreateRedemptionCodeRequest struct {
    MaxUses     int        `json:"maxUses" binding:"required,min=1"`
    DefaultDays int        `json:"defaultDays" binding:"required,min=1"`
    ExpiresAt   *time.Time `json:"expiresAt"`
}

type GetRedemptionCodesRequest struct {
    Page     int  `form:"page" binding:"omitempty,min=1"`
    PageSize int  `form:"pageSize" binding:"omitempty,min=1"`
    ShowAll  bool `form:"showAll"`
}

type GetRedemptionCodesResponse struct {
    Data       []models.RedemptionCode `json:"data"`
    Total      int64                   `json:"total"`
    Page       int                     `json:"page"`
    PageSize   int                     `json:"pageSize"`
    TotalPages int                     `json:"totalPages"`
}

func (s *RedemptionCodeService) CreateRedemptionCode(req *CreateRedemptionCodeRequest) (*models.RedemptionCode, error)
func (s *RedemptionCodeService) GetRedemptionCodes(req *GetRedemptionCodesRequest) (*GetRedemptionCodesResponse, error)
func (s *RedemptionCodeService) DeleteRedemptionCode(id string) error
func (s *RedemptionCodeService) ValidateCode(code string) (*models.RedemptionCode, error)
func (s *RedemptionCodeService) UseCode(code string) error
func (s *RedemptionCodeService) generateCode(length int) (string, error) // crypto/rand → hex，16 chars
```

`GetRedemptionCodes` 分页逻辑与当前 `GetInvites` 完全一致：`ShowAll=false` 时过滤掉已用完/已过期的码。

### 2.4 redemption.go（兑换服务，新建）
**新文件**: `services/api/internal/services/redemption.go`

```go
type RedemptionService struct{}

type RedeemCodeRequest struct {
    Code string `json:"code" binding:"required"`
}

type RedeemCodeResponse struct {
    Message   string     `json:"message"`
    Days      int        `json:"days"`
    ExpiresAt *time.Time `json:"expiresAt"`
}

type GetRedemptionsRequest struct {
    Page     int `form:"page" binding:"omitempty,min=1"`
    PageSize int `form:"pageSize" binding:"omitempty,min=1"`
}

type GetRedemptionsResponse struct {
    Data       []models.Redemption `json:"data"`
    Total      int64               `json:"total"`
    Page       int                 `json:"page"`
    PageSize   int                 `json:"pageSize"`
    TotalPages int                 `json:"totalPages"`
}

// 管理员查看时，需要 JOIN 用户名
type RedemptionWithUser struct {
    models.Redemption
    Username string `json:"username"`
}

type GetAllRedemptionsRequest struct {
    Page     int    `form:"page" binding:"omitempty,min=1"`
    PageSize int    `form:"pageSize" binding:"omitempty,min=1"`
    UserID   string `form:"userId"`
}

type GetAllRedemptionsResponse struct {
    Data       []RedemptionWithUser `json:"data"`
    Total      int64                `json:"total"`
    Page       int                  `json:"page"`
    PageSize   int                  `json:"pageSize"`
    TotalPages int                  `json:"totalPages"`
}
```

**RedeemCode 核心逻辑伪代码**：

```
func RedeemCode(userID string, req *RedeemCodeRequest):
    // 1. 验证兑换码
    code, err = codeService.ValidateCode(req.Code)
    if err: return err

    // 2. 查找用户
    var user models.User
    db.Where("id = ?", userID).First(&user)

    // 3. 计算新 ExpiresAt（复用 user.go ExtendExpiry 逻辑）
    if user.ExpiresAt == nil || user.ExpiresAt.Before(time.Now()):
        newExpiry = time.Now().AddDate(0, 0, code.DefaultDays)
    else:
        newExpiry = user.ExpiresAt.AddDate(0, 0, code.DefaultDays)

    // 4. 数据库事务
    tx = db.Begin()

    // 4a. 更新用户有效期
    user.ExpiresAt = &newExpiry

    // 4b. 如果 Emby 被封禁，解封
    if user.EmbyDisabled:
        err = embyService.SetUserPolicy(user.EmbyID, {IsDisabled: false})
        if err:
            tx.Rollback()
            return error("Emby 解封失败，请稍后重试")
        user.EmbyDisabled = false

    // 4c. 保存用户
    tx.Save(&user)

    // 4d. 原子递增兑换码使用次数（防竞态）
    result = tx.Model(&RedemptionCode{}).
        Where("code = ? AND \"usedCount\" < \"maxUses\"", req.Code).
        Update("usedCount", gorm.Expr("\"usedCount\" + 1"))
    if result.RowsAffected == 0:
        tx.Rollback()
        return error("兑换码已失效")  // 竞态：被其他请求用完了

    // 4e. 创建兑换记录
    tx.Create(&Redemption{UserID: userID, Code: req.Code, Days: code.DefaultDays})

    // 4f. 提交
    tx.Commit()

    return RedeemCodeResponse{
        Message:   fmt.Sprintf("兑换成功，有效期已延长 %d 天", code.DefaultDays),
        Days:      code.DefaultDays,
        ExpiresAt: &newExpiry,
    }
```

**原子性策略**：事务包含三个 DB 操作（更新 user、递增 usedCount、创建 redemption）。Emby API 调用在事务内、commit 之前。Emby API 失败 → 回滚，无副作用。Emby 成功但 DB commit 失败（极端情况）→ Emby 被解封但本地未更新，下次 cron 会重新封禁，系统最终一致。

### 2.5 user.go — 密码同步
**文件**: `services/api/internal/services/user.go`

两处修改：

**`UpdatePassword`（当前 line 226-247）**：
```
// 当前：验证旧密码 via Emby → 更新 Emby 密码
// 新增：Emby 更新成功后，同步本地 hash
err = embyService.UpdateUserPassword(user.EmbyID, req.NewPassword)
if err: return error(...)

// 新增以下两行
user.SetPassword(req.NewPassword)
db.Save(&user)
```

**`ResetPassword`（当前 line 166-181）**：
```
// 当前：直接调用 embyService.UpdateUserPassword
// 新增：同步本地 hash
err = embyService.UpdateUserPassword(user.EmbyID, newPassword)
if err: return error(...)

// 新增以下两行
user.SetPassword(newPassword)
db.Save(&user)
```

### 2.6 system.go — 定时任务改造
**文件**: `services/api/internal/services/system.go`

**CheckExpiredUsers 改造**：

```go
// 旧查询条件（line 88）：
db.Where("\"expiresAt\" < NOW() AND \"isActive\" = ?", true).Find(&expiredUsers)

// 新查询条件：
db.Where("\"expiresAt\" < NOW() AND \"embyDisabled\" = ?", false).Find(&expiredUsers)

// 旧更新逻辑（line 122-124）：
user.IsActive = false

// 新更新逻辑：
user.EmbyDisabled = true
// 不再修改 IsActive
```

**SystemInfo 结构变更**：

```go
// 旧
type SystemInfo struct {
    UserCount       int64 `json:"userCount"`
    ActiveUserCount int64 `json:"activeUserCount"`
    InviteCount     int64 `json:"inviteCount"`
}

// 新
type SystemInfo struct {
    UserCount           int64 `json:"userCount"`
    ActiveUserCount     int64 `json:"activeUserCount"`
    RedemptionCodeCount int64 `json:"redemptionCodeCount"`
}

// GetSystemInfo 中（line 47-48）：
// 旧: db.Model(&models.Invite{}).Count(&info.InviteCount)
// 新: db.Model(&models.RedemptionCode{}).Count(&info.RedemptionCodeCount)
```

---

## 3. Handler + 路由变更

### 3.1 Handler 变更

| 文件 | 变更 |
|------|------|
| `handlers/auth.go` | 注册：Code 改为可选；登录：返回中包含过期状态 |
| `handlers/redemption_code.go`（新建，替换 `handlers/invite.go`） | 管理员 CRUD 兑换码 |
| `handlers/user.go` | 新增 `RedeemCode`、`ValidateRedeemCode`、`GetRedemptions` |
| `handlers/setting.go`（新建） | 管理员配置 CRUD + 公开 GetRegistrationMode |
| `handlers/system.go` | SystemInfo 字段更名（自动跟随 struct） |

### 3.2 路由变更
**文件**: `services/api/cmd/server/main.go`

```
公开路由:
  POST /api/v1/user/register              # 注册（Code 可选，根据 registration_mode）
  GET  /api/v1/register/mode              # 获取当前注册模式（前端据此显示/隐藏兑换码输入框）

移除:
  GET  /api/v1/invites/:code/validate

用户路由（需认证，不限过期状态）:
  POST /api/v1/user/redeem                 # 兑换码兑换
  GET  /api/v1/user/redeem/:code/validate  # 兑换码预验证
  GET  /api/v1/user/redemptions            # 我的兑换历史

管理员路由:
  GET    /api/v1/admin/redemption-codes          # 兑换码列表
  POST   /api/v1/admin/redemption-codes          # 创建兑换码
  DELETE /api/v1/admin/redemption-codes/:id      # 删除兑换码
  GET    /api/v1/admin/settings                  # 获取所有配置
  PUT    /api/v1/admin/settings/:key             # 更新配置
  GET    /api/v1/admin/redemptions               # 全部兑换历史
```

### 3.3 各端点请求/响应格式

| 端点 | 请求 | 成功响应 (200) |
|------|------|---------------|
| `POST /user/redeem` | `{"code": "xxx"}` | `{"message": "兑换成功...", "days": 30, "expiresAt": "..."}` |
| `GET /user/redeem/:code/validate` | 路径参数 | `RedemptionCode` 对象 |
| `GET /user/redemptions` | `?page=1&pageSize=10` | `{data:[], total, page, pageSize, totalPages}` |
| `GET /register/mode` | 无 | `{"mode": "open", "defaultTrialDays": 7}` 或 `{"mode": "invite"}` |
| `GET /admin/redemption-codes` | `?page=1&pageSize=10&showAll=false` | `{data:[], total, page, pageSize, totalPages}` |
| `POST /admin/redemption-codes` | `{"maxUses": 5, "defaultDays": 30}` | `RedemptionCode` 对象 |
| `DELETE /admin/redemption-codes/:id` | 路径参数 | `{"message": "删除成功"}` |
| `GET /admin/settings` | 无 | `Setting[]` 数组 |
| `PUT /admin/settings/:key` | `{"value": "open"}` | `Setting` 对象 |
| `GET /admin/redemptions` | `?page=1&pageSize=10&userId=xx` | `{data:[], total, page, pageSize, totalPages}` |

### 3.4 自动调度初始化
**文件**: `services/api/cmd/server/main.go`

**环境变量**：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `CRON_SCHEDULE` | `0 2 * * *` | cron 表达式，默认每天凌晨2点 |
| `CRON_TIMEZONE` | `Asia/Shanghai` | 时区 |
| `CRON_ENABLED` | `true` | 是否启用定时任务 |

**集成方式**（在 `db.InitDB()` 之后、`r.Run()` 之前）：

```go
import "github.com/robfig/cron/v3"

// 定时任务初始化
cronEnabled := os.Getenv("CRON_ENABLED")
if cronEnabled == "" {
    cronEnabled = "true"
}

if cronEnabled == "true" {
    schedule := os.Getenv("CRON_SCHEDULE")
    if schedule == "" {
        schedule = "0 2 * * *"
    }
    tzName := os.Getenv("CRON_TIMEZONE")
    if tzName == "" {
        tzName = "Asia/Shanghai"
    }
    tz, err := time.LoadLocation(tzName)
    if err != nil {
        log.Printf("时区解析失败，使用 UTC：%v", err)
        tz = time.UTC
    }

    systemService := services.NewSystemService()
    c := cron.New(cron.WithLocation(tz))
    c.AddFunc(schedule, func() {
        log.Println("[Cron] 开始检查过期用户...")
        result, err := systemService.CheckExpiredUsers()
        if err != nil {
            log.Printf("[Cron] 检查失败：%v", err)
            return
        }
        log.Printf("[Cron] 完成，封禁 %d/%d 个用户", result.DisabledCount, result.TotalExpired)
    })
    c.Start()
    defer c.Stop()
    log.Printf("定时任务已启用：%s (%s)", schedule, tzName)
}
```

---

## 4. 前端变更

设计策略：**Dashboard 分区降级** — 用户面板不拆分页面，根据过期状态做渐进式降级。

### 4.1 注册页面改造
**文件**: `services/web/src/views/user/RegisterView.vue`

- `onMounted` 调用 `GET /api/v1/register/mode` 获取注册模式
- `"open"` 模式：只显示用户名、密码、邮箱（邮箱必填）
- `"invite"` 模式：额外显示兑换码输入框（必填，带预验证）
- 用 `v-if` 控制兑换码字段显隐，不需要两套 UI

### 4.2 用户面板 — 双态 Dashboard
**文件**: `services/web/src/views/user/DashboardView.vue`

核心状态判断：
```ts
const isExpired = computed(() => {
  if (!user.value.expiresAt) return false
  return new Date(user.value.expiresAt) < new Date()
})
```

**页面结构（从上到下）：**

**① 状态 Banner**：
- 过期：`el-alert type="warning"` 橙色，"你的账号已过期，Emby 访问已暂停。请使用兑换码续期。"
- 活跃：`el-alert type="success"` 绿色，"有效期至 xxxx-xx-xx（剩余 N 天）"

**② 兑换码区域**（`panel-clean` 卡片）：
- 过期时：白底红色左边框，兑换码输入框 + 兑换按钮，视觉最醒目
- 活跃时：收缩为 `el-collapse` 折叠面板，文案 "提前续期"

**③ 媒体统计**：
- 过期时：加 `opacity-40 pointer-events-none`，覆盖 "🔒 续期后自动恢复"
- 活跃时：正常 3 列布局（当前样式不变）
- 数据请求：活跃时才拉取 mediaStats 和 embyConfig

**④⑤ 账号信息 + 安全设置**：始终可见，不变。到期时间字段增加颜色标记：
- 过期：红色
- 即将过期（<7天）：黄色
- 正常：绿色

**兑换成功后**：刷新 profile 数据，banner 切换为绿色，媒体统计恢复，展示成功提示。

### 4.3 管理后台 — 兑换码管理
**文件**: `services/web/src/views/admin/InvitesView.vue` → 重命名为 `RedemptionCodesView.vue`

- 表头/文案从"邀请码"改为"兑换码"
- 创建对话框不变（maxUses, defaultDays, expiresAt）
- API 调用指向新端点

### 4.4 管理后台 — 系统设置增强
**文件**: `services/web/src/views/admin/SettingsView.vue`

页面结构（从上到下）：
- **系统配置区域**（新增）：`el-form` 行内布局
  - `registration_mode`：`el-select` 下拉选择 open / invite
  - `default_trial_days`：`el-input-number`
  - 保存按钮
- **统计卡片**：`inviteCount` → `redemptionCodeCount`
- **系统操作**：不变

### 4.5 管理后台侧边栏 + 路由
**文件**: `services/web/src/views/admin/Layout.vue`
- 菜单项 "邀请码管理" → "兑换码管理"

**文件**: `services/web/src/router/index.ts`
- `/admin/invites` → `/admin/redemption-codes`

### 4.6 首页文案更新
**文件**: `services/web/src/views/HomeView.vue`
- "自助注册系统" 卡片文案：从"通过邀请码注册"改为"直接注册体验"
- 首页 hero 区域的"从邀请注册到求片订阅"改为"从注册到求片订阅"

### 4.7 API 层变更

**`api/auth.ts`**：
```typescript
// 移除
export function validateInviteCode(code: string): Promise<ValidateInviteResponse>

// 新增
export function getRegistrationMode(): Promise<RegistrationModeResponse>

// register() 签名不变，但 RegisterRequest.code 现在可选
```

**`api/user.ts`** 新增：
```typescript
export function redeemCode(data: RedeemCodeRequest): Promise<RedeemCodeResponse>
export function validateRedeemCode(code: string): Promise<RedemptionCode>
export function getRedemptions(params?: {page?: number; pageSize?: number}): Promise<RedemptionListResponse>
```

**`api/admin.ts`** 变更：
```typescript
// 移除 getInvites(), createInvite(), deleteInvite()

// 新增：兑换码管理
export function getRedemptionCodes(params?): Promise<RedemptionCodeListResponse>
export function createRedemptionCode(data: CreateRedemptionCodeRequest): Promise<RedemptionCode>
export function deleteRedemptionCode(id: string)

// 新增：系统配置
export function getSettings(): Promise<Setting[]>
export function updateSetting(key: string, data: UpdateSettingRequest): Promise<Setting>

// 新增：兑换历史
export function getAllRedemptions(params?): Promise<RedemptionListResponse>
```

### 4.8 类型定义变更

**`types/api.ts`** 完整变更：

```typescript
// ====== 移除 ======
// Invite, InviteListResponse, ValidateInviteResponse, CreateInviteRequest

// ====== 修改 ======
export interface RegisterRequest {
  username: string
  password: string
  email: string
  code?: string  // 原 inviteCode，改为可选
}

export interface UserInfo {
  id: string
  username: string
  role: UserRole
  email?: string
  embyId?: string
  embyDisabled?: boolean  // 新增
  expiresAt?: string
  isActive: boolean
  createdAt: string
}

export interface SystemInfoResponse {
  success: boolean
  info: {
    userCount: number
    activeUserCount: number
    redemptionCodeCount: number  // 原 inviteCount
  }
}

// ====== 新增 ======
export interface RedemptionCode {
  id: string
  code: string
  maxUses: number
  usedCount: number
  defaultDays: number
  expiresAt?: string | null
  createdAt: string
}

export interface RedemptionCodeListResponse {
  data: RedemptionCode[]
  total: number
  page: number
  pageSize: number
  totalPages: number
}

export interface CreateRedemptionCodeRequest {
  maxUses: number
  defaultDays: number
  expiresAt?: string | null
}

export interface Redemption {
  id: string
  userId: string
  code: string
  days: number
  createdAt: string
  username?: string  // admin 查看时 JOIN 得到
}

export interface RedemptionListResponse {
  data: Redemption[]
  total: number
  page: number
  pageSize: number
  totalPages: number
}

export interface RedeemCodeRequest {
  code: string
}

export interface RedeemCodeResponse {
  message: string
  days: number
  expiresAt: string
}

export interface RegistrationModeResponse {
  mode: 'open' | 'invite'
  defaultTrialDays?: number  // 仅 open 模式包含
}

export interface Setting {
  key: string
  value: string
  updatedAt: string
}

export interface UpdateSettingRequest {
  value: string
}
```

### 4.9 Store 变更

**`store/auth.ts`**：
- 移除 `validateInvite` 方法及其 return 暴露
- `register` 的参数类型因 `RegisterRequest` 变化自动适配
- 不需要新增方法——兑换操作直接在组件中调用 `api/user.ts`

---

## 5. 数据库迁移

### 5.1 GORM 自动迁移
启动时自动：
- 新建 `redemption_codes`、`redemptions`、`settings` 表
- User 表新增 `embyDisabled` 列，Email 加唯一索引

### 5.2 默认配置 Seed
**文件**: `services/api/internal/db/db.go`

```go
db.DB.FirstOrCreate(&Setting{Key: "default_trial_days"}, Setting{Value: "7"})
db.DB.FirstOrCreate(&Setting{Key: "registration_mode"}, Setting{Value: "open"})
```

### 5.3 存量用户密码迁移
无需脚本。登录时自动完成：`Password == ""` → Emby 认证 → 成功后补存本地 hash。

### 5.4 旧表清理
在 `db.go` 中 AutoMigrate 后添加：`db.Migrator().DropTable("invites")`

---

## 6. 新增依赖

```bash
cd services/api && go get github.com/robfig/cron/v3
```

---

## 7. 验证方案

### 7.1 编译验证
```bash
cd services/api && go build ./...
cd services/web && npm run build
```

### 7.2 功能验证清单
1. **开放注册**：`registration_mode=open`，无需码，直接注册获得 N 天试用
2. **邀请注册**：`registration_mode=invite`，必须输入兑换码，试用天数 = 码的 defaultDays
3. **登录（正常/过期/存量迁移）**：过期用户可登录，看到过期提示；存量用户首次登录自动迁移密码
4. **兑换续期**：输入有效码 → ExpiresAt 延长 + Emby 解封 + 记录历史
5. **定时任务**：自动封禁过期 Emby 账号，不影响本地登录
6. **管理员设置**：可切换注册模式、修改默认试用天数
7. **兑换码管理**：管理员 CRUD 兑换码
8. **改密码**：本地 hash + Emby 密码同步

---

## 8. 关键文件清单

### 后端（20 文件：8 新建 + 9 修改 + 3 删除）

**新建**:
- `services/api/internal/models/redemption_code.go`
- `services/api/internal/models/redemption.go`
- `services/api/internal/models/setting.go`
- `services/api/internal/services/redemption_code.go`
- `services/api/internal/services/redemption.go`
- `services/api/internal/services/setting.go`
- `services/api/internal/handlers/redemption_code.go`
- `services/api/internal/handlers/setting.go`

**修改**:
- `services/api/internal/models/user.go` — 新增 EmbyDisabled，Email 唯一索引，移除 InviteCode
- `services/api/internal/services/auth.go` — 注册/登录流程重构
- `services/api/internal/services/user.go` — 密码同步
- `services/api/internal/services/system.go` — 定时任务改造
- `services/api/internal/handlers/auth.go` — 注册 handler 更新
- `services/api/internal/handlers/user.go` — 新增兑换 handler
- `services/api/internal/handlers/system.go` — SystemInfo 字段更新
- `services/api/internal/db/db.go` — 新模型迁移 + 配置 seed + 旧表清理
- `services/api/cmd/server/main.go` — 路由更新 + cron 初始化

**删除**:
- `services/api/internal/models/invite.go`
- `services/api/internal/services/invite.go`
- `services/api/internal/handlers/invite.go`

### 前端（12 文件修改/重命名）

- `services/web/src/views/user/RegisterView.vue` — 根据注册模式显示/隐藏兑换码
- `services/web/src/views/user/DashboardView.vue` — 双态设计：兑换区域 + 过期降级
- `services/web/src/views/admin/InvitesView.vue` → `RedemptionCodesView.vue`
- `services/web/src/views/admin/SettingsView.vue` — 添加配置编辑
- `services/web/src/views/admin/Layout.vue` — 菜单文案更新
- `services/web/src/views/HomeView.vue` — 首页文案更新（移除邀请码相关描述）
- `services/web/src/api/auth.ts`
- `services/web/src/api/user.ts`
- `services/web/src/api/admin.ts`
- `services/web/src/types/api.ts`
- `services/web/src/store/auth.ts`
- `services/web/src/router/index.ts`

---

## 9. 实施顺序

严格按编译依赖排序，每一步完成后必须能 `go build ./...` 通过。

### 阶段 A：后端 Models

| 步骤 | 操作 | 说明 |
|------|------|------|
| 1 | 新建 `models/redemption_code.go` | struct + TableName + BeforeCreate + IsValid |
| 2 | 新建 `models/redemption.go` | struct + TableName + BeforeCreate |
| 3 | 新建 `models/setting.go` | struct + TableName |
| 4 | 修改 `models/user.go` + 同步修改 `services/auth.go` | **必须同步**：移除 InviteCode 字段会导致 auth.go 编译失败 |

### 阶段 B：后端 Services

| 步骤 | 操作 | 说明 |
|------|------|------|
| 5 | 新建 `services/setting.go` | 配置 CRUD |
| 6 | 新建 `services/redemption_code.go` | 兑换码 CRUD（从 invite.go 迁移逻辑） |
| 7 | 修改 `services/auth.go` | 注册（open/invite 分支）+ 登录（本地密码优先 + 存量迁移） |
| 8 | 新建 `services/redemption.go` | 兑换核心逻辑（事务） |
| 9 | 修改 `services/user.go` | UpdatePassword + ResetPassword 同步本地 hash |
| 10 | 修改 `services/system.go` | CheckExpiredUsers 改查询条件 + SystemInfo 改字段 |

### 阶段 C：后端 Handlers + 路由

| 步骤 | 操作 | 说明 |
|------|------|------|
| 11 | 新建 `handlers/redemption_code.go` | Create + List + Delete |
| 12 | 新建 `handlers/setting.go` | GetSettings + UpdateSetting + GetRegistrationMode |
| 13 | 修改 `handlers/user.go` | 新增 RedeemCode + ValidateRedeemCode + GetRedemptions |
| 14 | 修改 `handlers/auth.go` | 跟随 service 层变化（无结构性改动） |
| 15 | 修改 `handlers/system.go` | 自动跟随 SystemInfo struct 变化 |
| 16 | 修改 `db/db.go` | AutoMigrate 新模型 + seed settings + DropTable invites |
| **16.5** | **`go get github.com/robfig/cron/v3`** | **必须在步骤 17 之前** |
| 17 | 修改 `cmd/server/main.go` | 替换路由 + cron 初始化 |

### 阶段 D：后端清理

| 步骤 | 操作 | 说明 |
|------|------|------|
| 18 | 删除 `models/invite.go` + `services/invite.go` + `handlers/invite.go` | 确认无残留引用 |
| 19 | `go build ./...` 最终验证 | |

### 阶段 E：前端

| 步骤 | 操作 | 说明 |
|------|------|------|
| 20 | 修改 `types/api.ts` | 类型基础，后续文件依赖 |
| 21 | 修改 `api/auth.ts` | 移除 validateInviteCode，新增 getRegistrationMode |
| 22 | 修改 `api/user.ts` | 新增兑换相关 API |
| 23 | 修改 `api/admin.ts` | 替换 invite → redemption-code，新增 settings + redemptions |
| 24 | 修改 `store/auth.ts` | 移除 validateInvite |
| 25 | 修改 `router/index.ts` | /admin/invites → /admin/redemption-codes |
| 26 | 修改 `RegisterView.vue` | onMounted 获取模式 + v-if 兑换码字段 |
| 27 | 修改 `DashboardView.vue` | 双态设计（核心前端改动） |
| 28 | 重命名 `InvitesView.vue` → `RedemptionCodesView.vue` | 文案 + API 端点替换 |
| 29 | 修改 `SettingsView.vue` | 新增配置编辑表单 |
| 30 | 修改 `Layout.vue` | 菜单文案更新 |
| 31 | 修改 `HomeView.vue` | 首页文案更新 |
| 32 | `npm run build` 最终验证 | |

**⚠️ 关键依赖提醒**：
- 步骤 4 和步骤 7 必须同步完成（InviteCode 字段移除会破坏 auth.go 编译）
- 步骤 16.5（go get cron）必须在步骤 17 之前
- 前端步骤 20 是所有后续前端步骤的基础

---

## 10. 边界情况与错误消息

### 10.1 完整错误消息清单

| 端点 | 错误消息 | HTTP | 触发条件 |
|------|----------|------|----------|
| `POST /login` | 用户名或密码错误 | 401 | 用户不存在 / 密码错误 |
| `POST /login` | 用户信息不匹配 | 401 | 存量迁移时 EmbyID 不匹配 |
| `POST /user/register` | 当前为邀请注册模式，请提供兑换码 | 400 | invite 模式下未传 code |
| `POST /user/register` | 兑换码不存在 | 400 | code 不在数据库中 |
| `POST /user/register` | 兑换码已失效 | 400 | code 已用完或过期 |
| `POST /user/register` | 用户名已存在 | 400 | username 重复 |
| `POST /user/register` | 邮箱已被注册 | 400 | email 重复（新增） |
| `POST /user/register` | 创建 Emby 用户失败 | 400 | Emby API 错误 |
| `POST /user/redeem` | 兑换码不存在 | 400 | code 不存在 |
| `POST /user/redeem` | 兑换码已失效 | 400 | code 已用完/过期/竞态被抢 |
| `POST /user/redeem` | Emby 解封失败，请稍后重试 | 500 | SetUserPolicy API 失败 |
| `POST /user/redeem` | 兑换失败，请稍后重试 | 500 | 事务中 DB 操作失败 |
| `GET /user/redeem/:code/validate` | 兑换码不存在 | 400 | code 不存在 |
| `GET /user/redeem/:code/validate` | 兑换码已失效 | 400 | code 无效 |
| `POST /admin/redemption-codes` | 创建兑换码失败 | 500 | DB 创建失败 |
| `DELETE /admin/redemption-codes/:id` | 兑换码不存在 | 500 | ID 不存在 |
| `PUT /admin/settings/:key` | 配置项不存在 | 404 | key 不在 settings 表 |
| `PUT /admin/settings/:key` | 无效的注册模式，必须为 open 或 invite | 400 | 值非法 |
| `PUT /admin/settings/:key` | 无效的试用天数 | 400 | 不是正整数 |

### 10.2 边界情况处理

**Case 1：用户兑换码时已经活跃（未过期）**

正常处理。ExpiresAt 从当前到期时间向后叠加 N 天。`EmbyDisabled` 为 false 时跳过 Emby 解封步骤。这是"提前续期"的正常场景。

**Case 2：管理员在用户注册过程中切换 registration_mode**

注册是同步请求，函数入口读取 mode 后基于该快照执行。
- `open → invite`：用户请求无 code，返回"当前为邀请注册模式，请提供兑换码"。前端刷新后显示 code 输入框。
- `invite → open`：用户请求带了 code，但 open 模式忽略 code。用户成功注册，有效期由系统配置决定。无副作用。

**Case 3：Cron 在用户兑换过程中运行**

不冲突。用户兑换时如果已过期，其 `EmbyDisabled=true`，cron 查询 `embyDisabled=false` 不会命中该用户。兑换完成后 ExpiresAt 在未来，cron 的 `expiresAt < NOW()` 也不会命中。安全。

**Case 4：两个请求同时兑换同一个 maxUses=1 的码**

通过 `WHERE usedCount < maxUses` 条件更新解决竞态：
```go
result = tx.Model(&RedemptionCode{}).
    Where("code = ? AND \"usedCount\" < \"maxUses\"", req.Code).
    Update("usedCount", gorm.Expr("\"usedCount\" + 1"))
if result.RowsAffected == 0:
    tx.Rollback()
    return error("兑换码已失效")
```
数据库行锁保证只有一个请求成功，另一个 RowsAffected=0 回滚。

**Case 5：存量用户从未登录新系统，Emby 已被 cron 封禁**

该用户 `Password==""` + `EmbyDisabled=true`。登录走 Emby 认证分支，但 Emby 已禁用，认证失败。

解决：管理员通过 `PUT /admin/users/:id/reset-password` 重置密码，此接口同步存储本地 hash。之后用户走本地密码分支登录，不再依赖 Emby。

**Case 6：Email 唯一约束冲突**

在 `Create` 之前显式检查 email 是否已存在，返回明确的"邮箱已被注册"。
