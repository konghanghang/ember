# 兑换码"一人一码一次"限制 + 兑换历史 UI 实现方案

## Context

当前系统的兑换码机制存在业务逻辑漏洞：**同一用户可以多次使用同一个兑换码进行续期**。例如，一个 `maxUses=10` 的兑换码，可能被单个用户消耗完所有次数，而不是预期的"10个不同用户各使用一次"。

此外，虽然后端已实现兑换历史查询 API（`GET /api/v1/user/redemptions` 和 `GET /api/v1/admin/redemptions`），但前端完全没有提供查询入口，导致用户和管理员无法查看兑换记录。

**本次改造目标**：
1. 在应用层增加"一人一码一次"校验，拒绝重复兑换
2. 为用户端和管理端添加兑换历史查询 UI

---

## 背景：当前兑换码使用逻辑

### 数据模型

**RedemptionCode（兑换码）**：
```go
type RedemptionCode struct {
    ID          string     // CUID
    Code        string     // 兑换码字符串（唯一索引）
    MaxUses     int        // 最大使用次数
    UsedCount   int        // 已使用次数
    ExpiresAt   *time.Time // 过期时间（可选）
    DefaultDays int        // 延长天数
    CreatedAt   time.Time
}
```

**Redemption（兑换历史）**：
```go
type Redemption struct {
    ID        string    // CUID
    UserID    string    // 用户 ID（有索引）
    Code      string    // 兑换码字符串
    Days      int       // 本次延长天数
    CreatedAt time.Time
}
```

### 当前兑换流程

**文件**：`services/api/internal/services/redemption.go:RedeemCode()`

**伪代码**：
```
func RedeemCode(userID string, req *RedeemCodeRequest):
    // 1. 验证兑换码
    code, err = codeService.ValidateCode(req.Code)
    if err: return err  // "兑换码不存在" 或 "兑换码已失效"

    // 2. 查找用户
    var user models.User
    db.Where("id = ?", userID).First(&user)

    // 3. 计算新 ExpiresAt
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

**原子性策略**：
- 事务包含三个 DB 操作（更新 user、递增 usedCount、创建 redemption）
- Emby API 调用在事务内、commit 之前
- Emby API 失败 → 回滚，无副作用
- Emby 成功但 DB commit 失败（极端情况）→ Emby 被解封但本地未更新，下次 cron 会重新封禁，系统最终一致

**当前问题**：
- ❌ 没有检查 `redemptions` 表中是否已存在 `(userId, code)` 记录
- ❌ 同一用户可以多次兑换同一个码，直到 `usedCount >= maxUses`

---

## 1. 后端改造：应用层去重校验

### 1.1 核心逻辑变更

**文件**：`services/api/internal/services/redemption.go`

**修改位置**：`RedeemCode` 函数（第 60-131 行）

**变更策略**：使用 `SELECT FOR UPDATE` 行锁保证并发安全，在事务内完成去重检查。

**完整流程**（修改后）：
```go
func RedeemCode(userID string, req *RedeemCodeRequest):
    // 1. 查找用户（移到事务外，减少事务持有时间）
    var user models.User
    if err := db.DB.Where("id = ?", userID).First(&user).Error; err != nil {
        return nil, errors.New("用户不存在")
    }

    // 2. 开启事务
    tx := db.DB.Begin()
    if tx.Error != nil {
        return nil, ErrRedeemFailed
    }

    // 3. 【核心变更】锁定兑换码行 + 验证有效性
    var code models.RedemptionCode
    err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
        Where("code = ?", req.Code).
        First(&code).Error
    if err != nil {
        tx.Rollback()
        if err == gorm.ErrRecordNotFound {
            return nil, ErrRedemptionCodeNotFound
        }
        return nil, ErrRedeemFailed
    }

    // 验证兑换码有效性（在锁的保护下）
    if !code.IsValid() {
        tx.Rollback()
        return nil, ErrRedemptionCodeInvalid
    }

    // 4. 【新增】在事务内检查用户是否已使用过此兑换码
    var existingRedemption models.Redemption
    err = tx.Where("\"userId\" = ? AND code = ?", userID, req.Code).
        First(&existingRedemption).Error
    if err == nil {
        // 找到记录 → 用户已兑换过
        tx.Rollback()
        return nil, ErrRedemptionDuplicate
    }
    if err != gorm.ErrRecordNotFound {
        // 数据库查询错误
        tx.Rollback()
        return nil, ErrRedeemFailed
    }

    // 5. 在事务内重新查询用户（获取最新状态，避免并发丢失更新）
    var userInTx models.User
    if err := tx.Where("id = ?", user.ID).First(&userInTx).Error; err != nil {
        tx.Rollback()
        return nil, errors.New("用户不存在")
    }

    // 6. 计算新有效期（基于事务内的最新值）
    now := time.Now().UTC()
    var newExpiry time.Time
    if userInTx.ExpiresAt == nil || userInTx.ExpiresAt.Before(now) {
        newExpiry = now.AddDate(0, 0, code.DefaultDays)
    } else {
        newExpiry = userInTx.ExpiresAt.AddDate(0, 0, code.DefaultDays)
    }

    // 7. 准备更新字段
    updates := map[string]interface{}{
        "expiresAt": newExpiry,
    }

    // 8. 如果 Emby 被封禁，解封
    needUnban := userInTx.EmbyDisabled && userInTx.IsActive
    if needUnban {
        embyService := NewEmbyService()
        if err := embyService.SetUserPolicy(userInTx.EmbyID, EmbyUserPolicy{IsDisabled: false}); err != nil {
            tx.Rollback()
            return nil, ErrEmbyUnbanFailed
        }
        updates["embyDisabled"] = false
    }

    // 9. 更新用户（只更新必要字段）
    if err := tx.Model(&models.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
        tx.Rollback()
        return nil, ErrRedeemFailed
    }

    // 10. 递增兑换码使用次数（已在锁的保护下，但仍需条件更新）
    result := tx.Model(&models.RedemptionCode{}).
        Where("code = ? AND \"usedCount\" < \"maxUses\"", req.Code).
        Update("usedCount", gorm.Expr("\"usedCount\" + 1"))
    if result.Error != nil {
        tx.Rollback()
        return nil, ErrRedeemFailed
    }
    if result.RowsAffected == 0 {
        tx.Rollback()
        return nil, ErrRedemptionCodeInvalid
    }

    // 11. 创建兑换记录
    if err := tx.Create(&models.Redemption{
        UserID: user.ID,
        Code:   req.Code,
        Days:   code.DefaultDays,
    }).Error; err != nil {
        tx.Rollback()
        return nil, ErrRedeemFailed
    }

    // 12. 提交事务
    if err := tx.Commit().Error; err != nil {
        return nil, ErrRedeemFailed
    }

    return &RedeemCodeResponse{
        Message:   fmt.Sprintf("兑换成功，有效期已延长 %d 天", code.DefaultDays),
        Days:      code.DefaultDays,
        ExpiresAt: &newExpiry,
    }, nil
```

**关键改动点**：
1. **移除独立的 `ValidateCode` 调用**：改为在事务内用 `SELECT FOR UPDATE` 锁定并验证
2. **行锁保护**：`Clauses(clause.Locking{Strength: "UPDATE"})` 锁定 `redemption_codes` 表的对应行
3. **去重检查在锁内**：在行锁的保护下查询 `redemptions` 表，保证原子性
4. **用户查询前置 + 事务内重查**：事务外先查询用户（获取 EmbyID 等基本信息），事务内重新查询获取最新状态（避免并发丢失更新）
5. **有效期计算在事务内**：基于事务内查询到的最新 `expiresAt` 计算新值，解决同一用户并发兑换不同码时的叠加问题
6. **部分字段更新**：使用 `Updates()` 只更新 `expiresAt` 和 `embyDisabled`，避免覆盖管理员并发修改的其他字段（如邮箱、状态）
7. **修正 embyDisabled 更新逻辑**：在调用 Emby API 之前判断是否需要解封，解封成功后将 `embyDisabled: false` 加入 `updates` map

### 1.2 错误定义与 HTTP 映射

**文件 1**：`services/api/internal/services/errors.go`

**新增错误**：
```go
var (
    // ... 现有错误定义
    ErrRedemptionDuplicate = errors.New("你已经使用过此兑换码")
)
```

**文件 2**：`services/api/internal/handlers/user.go`

**修改位置**：`RedeemCode` handler（第 355-374 行）

**变更点**：在 `switch` 语句中新增 `ErrRedemptionDuplicate` 的 400 映射

**修改后的代码**：
```go
func (h *UserHandler) RedeemCode(c *gin.Context) {
    userID, _ := c.Get("userID")
    var req services.RedeemCodeRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
        return
    }

    resp, err := h.redemptionService.RedeemCode(userID.(string), &req)
    if err != nil {
        switch {
        case errors.Is(err, services.ErrRedemptionCodeNotFound),
             errors.Is(err, services.ErrRedemptionCodeInvalid),
             errors.Is(err, services.ErrRedemptionDuplicate):  // 新增
            c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        case errors.Is(err, services.ErrEmbyUnbanFailed),
             errors.Is(err, services.ErrRedeemFailed):
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        default:
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        }
        return
    }

    c.JSON(http.StatusOK, resp)
}
```

**文件 3**：`services/api/internal/handlers/telegram.go`

**修改位置**：`RedeemByTelegram` handler（第 117-138 行）

**说明**：该 handler 处理 Telegram Bot 的兑换请求，与 `user.go` 的 `RedeemCode` 类似，都需要映射新增的 `ErrRedemptionDuplicate` 错误。

**变更点**：在 `switch` 语句中新增 `ErrRedemptionDuplicate` 的 400 映射

**修改后的代码**：
```go
func (h *TelegramHandler) RedeemByTelegram(c *gin.Context) {
    var req services.TelegramRedeemRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
        return
    }

    result, err := h.telegramService.RedeemByTelegram(req.TelegramID, req.Code)
    if err != nil {
        statusCode := http.StatusInternalServerError
        switch {
        case errors.Is(err, services.ErrTelegramNotBound),
             errors.Is(err, services.ErrRedemptionCodeNotFound),
             errors.Is(err, services.ErrRedemptionCodeInvalid),
             errors.Is(err, services.ErrRedemptionDuplicate):  // 新增
            statusCode = http.StatusBadRequest
        case errors.Is(err, services.ErrRedeemFailed),
             errors.Is(err, services.ErrEmbyUnbanFailed):
            statusCode = http.StatusInternalServerError
        }
        c.JSON(statusCode, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, result)
}
```

### 1.3 并发安全性分析

**SELECT FOR UPDATE 的工作原理**：
1. 事务 A 执行 `SELECT ... FOR UPDATE WHERE code = 'ABC123'`，获取该行的排他锁
2. 事务 B 同时尝试 `SELECT ... FOR UPDATE WHERE code = 'ABC123'`，被阻塞，等待事务 A 释放锁
3. 事务 A 完成所有操作（检查去重、更新、插入）并提交，释放锁
4. 事务 B 获取锁，此时 `redemptions` 表中已有事务 A 插入的记录，去重检查失败，回滚

**并发场景验证**：

| 场景 | 事务 A | 事务 B | 结果 |
|------|--------|--------|------|
| 同一用户同时兑换同一码 | 锁定 `code='ABC'` → 检查通过 → 插入 | 等待锁 → 检查失败（已有记录） | A 成功，B 返回"你已经使用过此兑换码" ✅ |
| 不同用户同时兑换同一码 | 锁定 `code='ABC'` → 检查通过 → 插入 | 等待锁 → 检查通过 → 插入 | A 成功，B 成功（如果 `usedCount < maxUses`）✅ |
| 同一码被用完的边界 | 锁定 → `usedCount=9` → 更新为 10 | 等待锁 → `usedCount=10` → 条件更新失败 | A 成功，B 返回"兑换码已失效" ✅ |

**性能影响**：
- 只有同时兑换**同一个码**的请求会排队
- 不同码的请求完全并行，无锁竞争
- **单个事务的锁持有时间**：约 10-50ms（包含数据库操作 + Emby API 调用）
- **并发排队等待时间**：取决于并发数，N 个并发请求 = N × 锁持有时间（最坏情况）
- **端到端延迟（用户感知）**：锁持有时间 + 排队等待时间
  - 无并发：< 100ms
  - 10 并发：最后一个请求可能等待 500ms
  - 30 并发：最后一个请求可能等待 1.5 秒
  - 100 并发：最后一个请求可能等待 5 秒
- 预期 QPS：单个兑换码 20-100 req/s，全局无限制
- 事务内操作增加了锁等待时间，但保证了并发安全
- **设计取舍（明确）**：优先保证一致性和”Never break userspace”，接受同一码高并发时串行排队，不将 Emby 调用移出锁
- **用户体验预期（同一码并发）**：并发量越高，排队越明显；高并发场景下端到端延迟可能达到秒级甚至十几秒，这是预期行为而非故障

**边界情况处理**：

**Case 1：存量数据（用户在改动前已多次兑换同一码）**
- 查询会找到第一条记录，拒绝后续兑换 ✅
- 历史记录保留，不影响数据完整性
- 无需数据迁移或清理

**Case 2：注册时使用兑换码**
- 注册流程（`auth.go:RegisterUser`）会在 `invite` 模式下插入 `redemption` 记录
- 用户注册后再次兑换同一码 → 被拒绝 ✅ 符合预期

**Case 3：Emby API 调用失败**
- 在事务内调用 Emby API，失败时回滚整个事务
- 不会出现"数据库已更新但 Emby 未解封"的不一致状态
- 用户可以重试兑换

**Case 4：管理员并发修改用户信息**
- 用户查询在事务外，管理员可能在兑换过程中修改用户邮箱、状态等字段
- 使用 `Updates()` 只更新 `expiresAt` 和 `embyDisabled`，不会覆盖其他字段
- 避免"丢失更新"问题

**Case 5：同一用户并发使用不同兑换码（已接受风险）**
- 本方案通过锁定 `redemption_codes` 行保证“同一码”的并发安全
- 对“同一用户 + 不同兑换码”的并发请求不额外加用户行锁，理论上存在低概率的有效期叠加竞态
- **本次决策**：该场景定义为非目标（用户主要为手动操作路径，概率低），不在本期增加额外复杂度
- 如后续出现真实案例，再评估增加用户行锁或原子续期 SQL

**Case 5：同一用户并发兑换不同码**
- 两个请求锁定不同的 `redemption_code` 行，可以并发执行
- 但都会在事务内查询同一个用户，获取最新的 `expiresAt`
- 第一个事务提交后，第二个事务查询到的 `expiresAt` 已经是更新后的值
- 有效期正确叠加，不会丢失天数
- **注意**：如果两个事务几乎同时开始，可能都基于旧值计算，导致只叠加一次。这是 Read Committed 隔离级别的固有限制，但概率极低（需要精确到毫秒级的并发）

---

## 2. 前端改造：兑换历史 UI

### 2.1 用户端：Dashboard 新增兑换历史区块

**文件**：`services/web/src/views/console/DashboardView.vue`

**位置**：在"账号设置"区块（Account Management）之后、`el-dialog`（续期弹窗）之前插入新区块。

**重要**：该区块仅对普通用户显示（`v-if="!authStore.isAdmin"`），因为 `GET /api/v1/user/redemptions` 接口使用了 `UserOnly()` 中间件，管理员调用会返回 403。

**UI 结构**：
```vue
<!-- 兑换历史（仅普通用户显示） -->
<div v-if="!authStore.isAdmin" class="bg-white rounded-2xl border border-gray-100 shadow-sm p-6">
  <div class="flex items-center justify-between mb-6">
    <div>
      <h2 class="text-xl font-semibold text-gray-900">兑换历史</h2>
      <p class="text-sm text-gray-500 mt-1">查看你的兑换码使用记录</p>
    </div>
    <el-button :icon="Refresh" @click="fetchRedemptions" :loading="redemptionsLoading">
      刷新
    </el-button>
  </div>

  <el-table
    :data="redemptions"
    v-loading="redemptionsLoading"
    style="width: 100%"
    :header-cell-style="{ backgroundColor: '#f9fafb' }"
  >
    <el-table-column prop="code" label="兑换码" width="180" />
    <el-table-column prop="days" label="延长天数" width="120">
      <template #default="{ row }">
        <el-tag type="success">{{ row.days }} 天</el-tag>
      </template>
    </el-table-column>
    <el-table-column prop="createdAt" label="兑换时间" width="200">
      <template #default="{ row }">
        {{ formatDate(row.createdAt) }}
      </template>
    </el-table-column>
  </el-table>

  <div class="mt-4 flex justify-end bg-gray-50/50 p-4 rounded-lg">
    <el-pagination
      v-model:current-page="redemptionPage"
      v-model:page-size="redemptionPageSize"
      :total="redemptionTotal"
      :page-sizes="[5, 10, 20]"
      layout="total, sizes, prev, pager, next"
      @current-change="fetchRedemptions"
      @size-change="fetchRedemptions"
    />
  </div>
</div>
```

**数据管理**：
```typescript
// 新增 ref
const redemptions = ref<Redemption[]>([])
const redemptionsLoading = ref(false)
const redemptionPage = ref(1)
const redemptionPageSize = ref(10)
const redemptionTotal = ref(0)

// 新增函数
const fetchRedemptions = async () => {
  redemptionsLoading.value = true
  try {
    const res = await getRedemptions({
      page: redemptionPage.value,
      pageSize: redemptionPageSize.value
    })
    redemptions.value = res.data
    redemptionTotal.value = res.total
  } catch (error) {
    // 全局拦截器已处理错误提示，这里不需要重复弹窗
  } finally {
    redemptionsLoading.value = false
  }
}

// 在 onMounted 中调用（仅普通用户）
onMounted(() => {
  // ... 现有逻辑
  if (!authStore.isAdmin) {
    fetchRedemptions()
  }
})

// 兑换成功后刷新历史
const handleRedeem = async () => {
  // ... 现有逻辑
  try {
    await redeemCode({ code: redeemForm.value.code })  // 修正：使用 redeemForm.value.code
    ElMessage.success('兑换成功')
    showRenewDialog.value = false
    redeemForm.value.code = ''
    await refreshAll()  // 刷新用户信息
    fetchRedemptions()  // 新增：刷新兑换历史
  } catch (error) {
    // 全局拦截器已处理错误提示
  }
}
```

**日期格式化**：
```typescript
const formatDate = (dateString: string) => {
  return new Date(dateString).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })
}
```

**类型导入**：
```typescript
import type { Redemption } from '@/types/api'
import { getRedemptions } from '@/api/user'
import { useAuthStore } from '@/store/auth'

// 在 setup 中获取 authStore
const authStore = useAuthStore()
```

### 2.2 管理端：新增兑换历史视图

**新建文件**：`services/web/src/views/admin/RedemptionHistoryView.vue`

**参考模板**：复用 `RedemptionCodesView.vue` 的结构，替换数据源和列定义。

**核心代码**：
```vue
<template>
  <div class="p-6 space-y-6">
    <!-- Header -->
    <div class="bg-white rounded-2xl border border-gray-100 shadow-sm p-6">
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-bold text-gray-900">兑换历史</h1>
          <p class="text-sm text-gray-500 mt-1">查看所有用户的兑换码使用记录</p>
        </div>
        <div class="flex items-center gap-3">
          <span class="text-sm text-gray-500">
            共 <span class="font-semibold text-gray-900">{{ total }}</span> 条记录
          </span>
          <el-button :icon="Refresh" @click="fetchData" :loading="loading">
            刷新
          </el-button>
        </div>
      </div>
    </div>

    <!-- 筛选器 -->
    <div class="bg-white rounded-2xl border border-gray-100 shadow-sm p-6">
      <el-form :inline="true">
        <el-form-item label="用户 ID">
          <el-input
            v-model="queryParams.userId"
            placeholder="输入用户 ID 筛选"
            clearable
            style="width: 200px"
          />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handleSearch">搜索</el-button>
          <el-button @click="handleReset">重置</el-button>
        </el-form-item>
      </el-form>
    </div>

    <!-- 表格 -->
    <div class="bg-white rounded-2xl border border-gray-100 shadow-sm p-6">
      <el-table
        :data="tableData"
        v-loading="loading"
        style="width: 100%"
        :header-cell-style="{ backgroundColor: '#f9fafb' }"
      >
        <el-table-column prop="username" label="用户名" width="150" />
        <el-table-column prop="code" label="兑换码" width="180" />
        <el-table-column prop="days" label="延长天数" width="120">
          <template #default="{ row }">
            <el-tag type="success">{{ row.days }} 天</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="createdAt" label="兑换时间" width="200">
          <template #default="{ row }">
            {{ formatDate(row.createdAt) }}
          </template>
        </el-table-column>
        <el-table-column prop="userId" label="用户 ID" min-width="200">
          <template #default="{ row }">
            <code class="text-xs text-gray-600">{{ row.userId }}</code>
          </template>
        </el-table-column>
      </el-table>

      <!-- 分页 -->
      <div class="mt-4 flex justify-end bg-gray-50/50 p-4 rounded-lg">
        <el-pagination
          v-model:current-page="queryParams.page"
          v-model:page-size="queryParams.pageSize"
          :total="total"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next"
          @current-change="fetchData"
          @size-change="fetchData"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { getAllRedemptions } from '@/api/admin'
import type { Redemption } from '@/types/api'

const tableData = ref<Redemption[]>([])
const loading = ref(false)
const total = ref(0)
const queryParams = ref({
  page: 1,
  pageSize: 10,
  userId: ''
})

const fetchData = async () => {
  loading.value = true
  try {
    const params: any = {
      page: queryParams.value.page,
      pageSize: queryParams.value.pageSize
    }
    if (queryParams.value.userId) {
      params.userId = queryParams.value.userId
    }
    const res = await getAllRedemptions(params)
    tableData.value = res.data
    total.value = res.total
  } catch (error) {
    // 全局拦截器已处理错误提示
  } finally {
    loading.value = false
  }
}

const handleSearch = () => {
  queryParams.value.page = 1
  fetchData()
}

const handleReset = () => {
  queryParams.value.userId = ''
  queryParams.value.page = 1
  fetchData()
}

const formatDate = (dateString: string) => {
  return new Date(dateString).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })
}

onMounted(() => {
  fetchData()
})
</script>
```

### 2.3 路由配置

**文件**：`services/web/src/router/index.ts`

**新增路由**（在 `/console` 的 `children` 数组中）：
```typescript
{
  path: 'redemption-history',
  name: 'console-redemption-history',
  component: () => import('../views/admin/RedemptionHistoryView.vue'),
  meta: { requiresAuth: true, role: 'admin' }
}
```

**位置**：建议放在 `redemption-codes` 路由之后。

### 2.4 侧边栏菜单

**文件**：`services/web/src/components/console/Sidebar.vue`

**修改位置**：管理控制台菜单组（`role: 'admin'` 的 group）

**新增菜单项**：
```typescript
{
  title: '兑换历史',
  path: '/console/redemption-history',
  icon: Document // 或 List
}
```

**位置**：放在"兑换码管理"之后、"付费方案"之前。

**图标导入**：
```typescript
import { Document } from '@element-plus/icons-vue'
```

---

## 3. 实施顺序

### 阶段 A：后端改造（10 分钟）

| 步骤 | 文件 | 操作 |
|------|------|------|
| 1 | `services/api/internal/services/errors.go` | 新增 `ErrRedemptionDuplicate` 错误定义 |
| 2 | `services/api/internal/services/redemption.go` | 重构 `RedeemCode` 函数：移除 `ValidateCode` 调用，改为事务内 `SELECT FOR UPDATE` + 去重检查（约 30 行改动） |
| 3 | `services/api/internal/handlers/user.go` | 在 `RedeemCode` handler 的 `switch` 中新增 `ErrRedemptionDuplicate` 映射到 400 |
| 4 | `services/api/internal/handlers/telegram.go` | 同上（如果该文件有兑换接口） |
| 5 | 编译验证 | `cd services/api && go build ./...` |

### 阶段 B：前端用户端（15 分钟）

| 步骤 | 文件 | 操作 |
|------|------|------|
| 4 | `services/web/src/views/console/DashboardView.vue` | 在"账号设置"区块后插入兑换历史区块（约 80 行代码） |
| 5 | 同上 | 新增 `redemptions` 相关 ref、`fetchRedemptions` 函数、`formatDate` 工具函数 |
| 6 | 同上 | 在 `onMounted` 中调用 `fetchRedemptions()`，在 `handleRedeem` 成功后调用 `fetchRedemptions()` |
| 7 | 编译验证 | `cd services/web && npm run build` |

### 阶段 C：前端管理端（20 分钟）

| 步骤 | 文件 | 操作 |
|------|------|------|
| 8 | `services/web/src/views/admin/RedemptionHistoryView.vue` | 新建文件，复制 `RedemptionCodesView.vue` 结构并修改（约 150 行代码） |
| 9 | `services/web/src/router/index.ts` | 在 `/console` 的 `children` 中新增 `redemption-history` 路由 |
| 10 | `services/web/src/components/console/Sidebar.vue` | 在管理控制台菜单组中新增"兑换历史"菜单项 |
| 11 | 编译验证 | `cd services/web && npm run build` |

---

## 4. 关键文件清单

### 后端（4 文件修改）
- `services/api/internal/services/errors.go` — 新增错误定义
- `services/api/internal/services/redemption.go` — 核心去重逻辑（SELECT FOR UPDATE）
- `services/api/internal/handlers/user.go` — HTTP 错误映射
- `services/api/internal/handlers/telegram.go` — HTTP 错误映射（如果有兑换接口）

### 前端（4 文件：1 新建 + 3 修改）
- `services/web/src/views/admin/RedemptionHistoryView.vue` — 新建管理端兑换历史视图
- `services/web/src/views/console/DashboardView.vue` — 用户端兑换历史区块
- `services/web/src/router/index.ts` — 新增路由
- `services/web/src/components/console/Sidebar.vue` — 新增菜单项

---

## 5. 验证方案

### 5.1 后端验证

**编译验证**：
```bash
cd services/api && go build ./...
```

**单元测试**（如果有）：
```bash
cd services/api && go test ./internal/services -v -run TestRedeemCode
```

**手动测试场景**（需要启动服务后在浏览器/Postman 中测试）：

**场景 1：正常兑换**
- 用户 A 首次兑换码 "TEST123"
- 预期：成功，返回 `{"message": "兑换成功，有效期已延长 30 天", ...}`

**场景 2：重复兑换（核心验证）**
- 用户 A 再次兑换同一个码 "TEST123"
- 预期：失败，返回 `{"error": "你已经使用过此兑换码"}`，HTTP 400

**场景 3：不同用户兑换同一码**
- 用户 B 兑换 "TEST123"（假设 `maxUses >= 2`）
- 预期：成功

**场景 4：并发测试**（可选，需要压测工具）
- 使用 Apache Bench 或 wrk 模拟 10 个并发请求，同一用户兑换同一码
- 预期：只有 1 个请求成功，其余 9 个返回"你已经使用过此兑换码"

### 5.2 前端验证

**用户端（DashboardView）**：
1. 登录普通用户账号
2. 进入 Dashboard（`/console/dashboard`）
3. 滚动到底部，确认"兑换历史"区块存在
4. 检查表格显示历史记录（如果有）
5. 点击"立即续期"，输入兑换码，成功后确认表格自动刷新

**管理端（RedemptionHistoryView）**：
1. 登录管理员账号
2. 侧边栏确认"兑换历史"菜单项存在（在"兑换码管理"之后）
3. 点击进入 `/console/redemption-history`
4. 检查表格显示所有用户的兑换记录，包含"用户名"列
5. 输入用户 ID 筛选，点击"搜索"，确认筛选生效
6. 点击"重置"，确认筛选清空

### 5.3 编译验证

```bash
# 后端
cd services/api && go build ./...

# 前端
cd services/web && npm run build
```

---

## 6. 错误消息清单

| 场景 | HTTP | 错误消息 |
|------|------|----------|
| 用户重复兑换同一码 | 400 | 你已经使用过此兑换码 |
| 兑换码不存在 | 400 | 兑换码不存在 |
| 兑换码已失效 | 400 | 兑换码已失效 |
| 数据库查询失败 | 500 | 兑换失败，请稍后重试 |

---

## 7. 数据库影响

**无需迁移**：
- 不新增表或字段
- 不修改索引
- 不添加唯一约束
- 现有 `redemptions` 表已包含所需数据（`userId`, `code`）

**存量数据兼容性**：
- 如果用户在改动前已多次兑换同一码，`redemptions` 表中会有多条记录
- 查询 `WHERE userId=? AND code=?` 会找到第一条记录，拒绝后续兑换
- 历史记录完整保留，不影响数据完整性
- **无需清理重复数据**

**事务隔离级别**：
- PostgreSQL 默认隔离级别为 Read Committed
- `SELECT FOR UPDATE` 在该隔离级别下正常工作
- 无需修改数据库配置

---

## 8. 性能考量

**后端**：
- 使用 `SELECT FOR UPDATE` 锁定兑换码行，在事务内完成所有操作
- **单个事务的锁持有时间**：约 10-50ms（包含数据库操作 + Emby API 调用）
- **并发场景下的排队等待**：只有同时兑换同一个码的请求会排队，不同码的请求完全并行
- **端到端延迟（用户感知）**：
  - 无并发：< 100ms
  - 低并发（< 10）：< 500ms
  - 中并发（10-30）：可能达到 1-2 秒
  - 高并发（> 30）：可能达到秒级甚至十几秒
- **验收阈值（建议）**：同一码 30 并发时，允许出现排队导致的秒级到十几秒延迟；只要最终满足”最多 1 次/用户/码、总次数不超限”即视为符合设计预期

**前端**：
- 用户端：Dashboard 加载时多一次 API 请求（`/api/v1/user/redemptions`）
- 管理端：新增独立页面，不影响现有页面性能
- 分页查询，单次返回 10-20 条记录，响应时间 < 100ms

---

## 9. 后续优化（不在本次范围）

1. **数据库索引优化**：如果兑换历史表增长到 10 万+ 记录，考虑为 `redemptions.code` 添加索引
2. **兑换码使用统计**：在兑换码管理页面显示"已被 X 个用户使用"
3. **导出功能**：管理端兑换历史支持导出 CSV
4. **实时通知**：用户兑换成功后，通过 WebSocket 推送通知给管理员
5. **不同兑换码并发叠加一致性**：若生产中出现同一用户并发兑换多个不同码导致续期叠加异常，再引入用户行锁或原子续期策略
