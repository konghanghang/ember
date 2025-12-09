# 🔧 Bug 修复总结

> **修复日期**: 2025-12-08 ~ 2025-12-09
> **修复人员**: Kong Hang (with Claude Code)
> **修复方案**: 方案 B - 完整修复所有问题

---

## 📋 修复清单

| 问题编号 | 问题描述 | 严重程度 | 状态 |
|---------|---------|---------|------|
| #1 | 注册流程事务错误 | 🔴 严重 | ✅ 已修复 |
| #2 | 删除用户事务错误 | 🔴 严重 | ✅ 已修复 |
| #3 | 用户状态同步不一致 | 🟡 中等 | ✅ 已修复 |
| #4 | 延期功能状态同步不一致 | 🟡 中等 | ✅ 已修复 |
| #5 | 邀请码并发安全问题 | 🟡 中等 | ✅ 已修复 |
| #6 | 管理员密码安全隐患 | 🔴 严重 | ✅ 已修复 |

**总修复时间**: ~5 小时
**修改文件数**: 11 个

---

## 🔴 问题 #1：注册流程事务错误

### 问题描述

`registerUser()` 函数中，Emby API 调用被包裹在 Prisma 事务内：

```typescript
await prisma.$transaction(async (tx) => {
  // ❌ Emby API 调用（Prisma 无法回滚外部 API）
  const embyUser = await embyClient.createUser({...})

  // ✅ 数据库操作（可以回滚）
  await tx.user.create({...})
})
```

**后果**：
- Emby 用户创建成功，但数据库失败 → 留下孤儿 Emby 用户
- 用户无法用相同用户名重新注册（Emby 提示"用户名已存在"）
- 邀请码未扣减，但 Emby 资源已消耗

### 修复方案

采用 **"Emby 先行 + 补偿删除"** 模式：

```typescript
let embyUserId: string | null = null

try {
  // 1. 先执行 Emby 操作（事务外）
  const embyUser = await embyClient.createUser({...})
  embyUserId = embyUser.Id

  // 2. 再执行数据库事务
  const user = await prisma.$transaction(async (tx) => {
    await tx.user.create({...})
    await tx.invite.update({...})
    return newUser
  })

  return { success: true, user }

} catch (error) {
  // 3. 补偿删除：如果数据库失败，清理 Emby 用户
  if (embyUserId) {
    try {
      await embyClient.deleteUser(embyUserId)
    } catch (cleanupError) {
      // 4. 清理失败，记录日志供人工介入
      await prisma.log.create({
        action: 'orphan_emby_user',
        details: { embyId: embyUserId, ... }
      })
    }
  }

  throw error
}
```

**优势**：
- ✅ Emby 失败 → 数据库不受影响
- ✅ 数据库失败 → 自动清理 Emby 用户
- ✅ 清理失败 → 记录日志，可追溯

**修改文件**: `app/actions/users.ts` (第 13-150 行)

---

## 🔴 问题 #2：删除用户事务错误

### 问题描述

`deleteUser()` 函数同样将 Emby API 调用放在事务内，导致：
- Emby 删除成功，数据库失败 → 留下孤儿数据库记录
- 数据库记录显示用户存在，但 Emby 已无此用户

### 修复方案

采用 **"Emby 先删 + 异常记录"** 模式：

```typescript
// 1. 先删除 Emby 用户（失败则直接返回）
try {
  await embyClient.deleteUser(user.embyId)
} catch (embyError) {
  return { success: false, error: 'Emby 用户删除失败' }
}

// 2. Emby 删除成功后，再删除数据库记录
try {
  await prisma.$transaction(async (tx) => {
    await tx.user.delete({ where: { id: userId } })
    await tx.log.create({ action: 'delete_user', ... })
  })

  return { success: true }

} catch (dbError) {
  // 3. 数据库删除失败，记录孤儿数据库记录
  await prisma.log.create({
    action: 'orphan_db_record',
    details: { userId, embyId: user.embyId, ... }
  })

  return { success: false, error: '数据库记录删除失败，请联系管理员' }
}
```

**选择 Emby 先删的原因**：
- 孤儿数据库记录比孤儿 Emby 用户更容易清理
- 数据库记录无实际影响，Emby 用户会消耗资源

**修改文件**: `app/actions/users.ts` (第 321-407 行)

---

## 🟡 问题 #3：用户状态同步不一致

### 问题描述

`toggleUserStatus()` 函数在 Emby 更新成功但数据库失败时，导致状态不一致：
- Emby 用户已禁用，数据库显示启用
- 管理员界面显示"正常"，但用户无法登录 Emby

### 修复方案

采用 **"Emby 先行 + 补偿回滚"** 模式：

```typescript
// 1. 先更新 Emby 状态
try {
  await embyClient.setUserPolicy(user.embyId, { IsDisabled: targetStatus })
} catch (embyError) {
  return { success: false, error: 'Emby 状态更新失败' }
}

// 2. 再更新数据库
try {
  await prisma.$transaction(async (tx) => {
    await tx.user.update({ data: { isActive: targetStatus } })
    await tx.log.create({ action: 'toggle_user_status', ... })
  })

  return { success: true }

} catch (dbError) {
  // 3. 数据库失败，尝试回滚 Emby 状态
  try {
    await embyClient.setUserPolicy(user.embyId, { IsDisabled: !targetStatus })
  } catch (rollbackError) {
    // 4. 回滚失败，记录同步冲突
    await prisma.log.create({
      action: 'sync_conflict',
      details: { embyStatus, dbStatus, ... }
    })
  }

  return { success: false, error: '数据库更新失败' }
}
```

**修改文件**: `app/actions/users.ts` (第 263-367 行)

---

## 🟡 问题 #4：延期功能状态同步不一致

### 问题描述

`extendExpiry()` 函数在延期已禁用用户时，会自动重新启用 Emby 用户。但如果数据库更新失败，会出现：
- Emby 用户已启用，数据库仍显示禁用
- 用户可以登录 Emby，但管理界面显示"已禁用"

### 修复方案

与问题 #3 类似，采用 **"Emby 先行 + 补偿回滚"** 模式：

```typescript
// 1. 如果需要重新启用，先更新 Emby
if (shouldReactivate) {
  try {
    await embyClient.setUserPolicy(user.embyId, { IsDisabled: false })
  } catch (embyError) {
    return { success: false, error: 'Emby 重新启用失败' }
  }
}

// 2. 再更新数据库
try {
  await prisma.$transaction(async (tx) => {
    await tx.user.update({
      data: {
        expiresAt: newExpiresAt,
        ...(shouldReactivate && { isActive: true })
      }
    })
    await tx.log.create({ action: 'extend_expiry', ... })
  })

  return { success: true }

} catch (dbError) {
  // 3. 如果已重新启用 Emby，尝试回滚到禁用
  if (shouldReactivate) {
    try {
      await embyClient.setUserPolicy(user.embyId, { IsDisabled: true })
    } catch (rollbackError) {
      await prisma.log.create({ action: 'sync_conflict', ... })
    }
  }

  return { success: false, error: '数据库更新失败' }
}
```

**修改文件**: `app/actions/users.ts` (第 196-313 行)

---

## 🟡 问题 #5：邀请码并发安全问题

### 问题描述

在高并发场景下，同一个邀请码可能被多个请求同时使用：

1. 请求 A 验证邀请码（剩余 1 次）✅
2. 请求 B 验证邀请码（剩余 1 次）✅
3. 请求 A 更新 `usedCount` → 1
4. 请求 B 更新 `usedCount` → 2（超额使用）

**后果**：单次邀请码被用两次，管理员控制失效

### 修复方案

采用 **乐观锁模式**，使用数据库原子操作：

```typescript
await prisma.$transaction(async (tx) => {
  // 1. 原子操作：仅在有剩余次数时更新
  const updateResult = await tx.invite.updateMany({
    where: {
      code: data.inviteCode,
      usedCount: { lt: invite.maxUses }, // 确保还有剩余次数
      ...(invite.expiresAt && { expiresAt: { gte: new Date() } })
    },
    data: {
      usedCount: { increment: 1 }
    }
  })

  // 2. 检查是否更新成功
  if (updateResult.count === 0) {
    throw new Error('邀请码已用尽或已过期（并发冲突）')
  }

  // 3. 继续创建用户
  await tx.user.create({...})
})
```

**关键点**：
- 使用 `updateMany` 而不是 `update`（支持条件检查）
- 在 WHERE 条件中检查 `usedCount < maxUses`
- 更新失败（`count === 0`）立即抛错，触发事务回滚

**修改文件**: `app/actions/users.ts` (第 60-115 行)

---

## ✅ 新增功能

### 1. Docker 部署支持

**新增文件**：
- `Dockerfile` - 多阶段构建，优化镜像体积
- `docker-compose.yml` - 一键部署（应用 + 数据库）
- `.dockerignore` - 排除不必要的文件

**特性**：
- 三阶段构建（deps → builder → runner）
- 使用 Next.js standalone 输出（减小镜像体积 60%+）
- 自动运行数据库迁移和初始化
- 健康检查支持

### 2. 健康检查 API

**新增文件**: `app/api/health/route.ts`

**功能**：
- 检查应用状态
- 检查数据库连接
- 用于 Docker healthcheck 和监控

**示例响应**：
```json
{
  "status": "ok",
  "timestamp": "2025-12-08T...",
  "database": "connected"
}
```

### 3. 部署文档

**新增文件**: `docs/DEPLOYMENT.md`

**内容**：
- 快速部署指南
- 环境变量配置说明
- 常见问题排查
- 安全建议
- 维护操作

---

## 📊 修复效果

### 数据一致性保证

| 场景 | 修复前 | 修复后 |
|------|--------|--------|
| Emby 成功，数据库失败 | ❌ 孤儿 Emby 用户 | ✅ 自动清理 Emby 用户 |
| Emby 失败，数据库成功 | ❌ 数据库有记录但 Emby 无用户 | ✅ 事务回滚，数据库无记录 |
| 状态同步失败 | ❌ Emby 和数据库状态不一致 | ✅ 自动回滚 Emby 状态 |
| 邀请码并发使用 | ❌ 单次邀请码被用两次 | ✅ 原子操作保证唯一 |

### 可观测性提升

所有异常情况都有日志记录：
- `orphan_emby_user` - Emby 孤儿用户（需人工删除）
- `orphan_db_record` - 数据库孤儿记录（需人工删除）
- `sync_conflict` - 状态同步冲突（需人工核对）

**查询日志示例**：

```sql
SELECT * FROM "Log"
WHERE action IN ('orphan_emby_user', 'orphan_db_record', 'sync_conflict')
ORDER BY "createdAt" DESC;
```

---

## 🧪 测试建议

### 1. 补偿删除测试

```bash
# 模拟数据库失败
1. 注册用户时关闭数据库
2. 检查 Emby 是否创建了用户
3. 检查日志是否记录了孤儿用户
```

### 2. 并发测试

```bash
# 使用 Apache Bench 测试邀请码并发
ab -n 10 -c 10 -p register.json \
   -T application/json \
   http://localhost:3000/api/register
```

### 3. 状态回滚测试

```bash
# 模拟数据库失败
1. 禁用用户时关闭数据库
2. 检查 Emby 状态是否回滚到原状态
3. 检查日志是否记录了同步冲突
```

---

## 🔐 问题 #6：管理员密码安全隐患（新增修复）

### 问题描述

**原始设计的安全隐患**：

1. **默认密码硬编码**
   - 管理员密码硬编码为 `admin123`（`prisma/seed.ts`）
   - 所有环境都使用相同密码
   - 无法在初始化时自定义密码

2. **无法修改密码**
   - 系统未提供修改密码功能
   - 密码一旦泄露无法补救

3. **登录页面暴露密码**
   - 登录页面显示 "默认账号：admin / admin123"
   - 公网部署时等同于公开管理员密码

**后果**：
- ❌ 公网部署极度危险（任何人都知道密码）
- ❌ 账号被入侵后无法修改密码
- ❌ 违反安全最佳实践

### 修复方案

采用 **方案 A（修改密码功能）+ 方案 B（环境变量配置）**：

#### 方案 A：添加修改密码功能

**新增文件**：
- `app/actions/auth.ts` - 添加 `updateAdminPassword()` 函数
- 更新 `app/admin/settings/page.tsx` - 添加修改密码表单

**功能特性**：
```typescript
export async function updateAdminPassword(data: {
  currentPassword: string
  newPassword: string
}) {
  // 1. 验证当前密码
  const isValid = await verifyPassword(currentPassword, admin.password)
  if (!isValid) return { error: '当前密码错误' }

  // 2. 验证新密码长度（至少 6 个字符）
  if (newPassword.length < 6) return { error: '新密码太短' }

  // 3. 加密并更新
  const hashed = await bcrypt.hash(newPassword, 10)
  await prisma.admin.update({ data: { password: hashed } })

  // 4. 记录日志
  await prisma.log.create({ action: 'update_password', ... })

  return { success: true }
}
```

**UI 界面**：
- 位置：系统设置 → 修改密码
- 表单字段：当前密码、新密码、确认新密码
- 前端验证：密码一致性检查
- 实时反馈：成功/失败提示

#### 方案 B：环境变量配置初始密码

**修改文件**：`prisma/seed.ts`

**实现**：
```typescript
async function main() {
  // 优先使用环境变量，否则使用默认密码
  const plainPassword = process.env.ADMIN_DEFAULT_PASSWORD || 'admin123'
  const password = await bcrypt.hash(plainPassword, 10)

  await prisma.admin.upsert({
    where: { username: 'admin' },
    create: { username: 'admin', password },
  })

  // 显示警告
  if (plainPassword === 'admin123') {
    console.log('⚠️  警告：当前使用默认密码 "admin123"')
    console.log('   生产环境请在 .env 中设置 ADMIN_DEFAULT_PASSWORD')
  }
}
```

**使用方法**：
```bash
# .env 文件
ADMIN_DEFAULT_PASSWORD="your-secure-password"

# 重新初始化
npm run db:seed
```

#### 方案 C：移除登录页面的密码提示

**修改文件**：`app/(auth)/login/page.tsx`

**变更**：
- ❌ 删除：`<p>默认账号：admin / admin123</p>`
- ✅ 保留：登录表单和返回首页链接

**理由**：
- 公开显示默认密码是严重安全隐患
- 强制管理员妥善保管密码
- 符合生产环境安全标准

---

### 完整的密码管理策略

#### 部署前配置（推荐）

```bash
# .env 文件
ADMIN_DEFAULT_PASSWORD="$(openssl rand -base64 16)"
```

**优势**：
- ✅ 不同环境使用不同密码
- ✅ 避免使用弱密码
- ✅ 密码不暴露在代码中

#### 首次登录后修改（必须）

**步骤**：
1. 登录管理后台（用户名：`admin`）
2. 导航到"系统设置" → "修改密码"
3. 填写当前密码和新密码
4. 点击"修改密码"

**密码策略**：
- 最小长度：6 个字符
- 推荐：12+ 个字符，包含大小写字母、数字、特殊字符
- 禁止：弱密码如 `123456`、`password`、`admin`

#### 密码重置（管理员忘记密码）

如果管理员忘记密码，可以通过数据库重置：

```bash
# 方法 1：重新运行 seed（会重置为环境变量中的密码）
npm run db:seed

# 方法 2：直接更新数据库（需要生成 bcrypt hash）
# 生成新密码的 hash
node -e "console.log(require('bcryptjs').hashSync('new-password', 10))"

# 更新数据库
docker compose exec postgres psql -U postgres ember -c \
  "UPDATE \"Admin\" SET password = 'YOUR_HASH_HERE' WHERE username = 'admin';"
```

---

### 安全性对比

| 场景 | 修复前 | 修复后 |
|------|--------|--------|
| **公网部署** | ❌ 默认密码 `admin123`，无法修改 | ✅ 环境变量配置 + 可修改 |
| **登录页面** | ❌ 显示默认密码提示 | ✅ 无密码提示 |
| **密码泄露** | ❌ 无法补救 | ✅ 立即修改密码 |
| **多环境部署** | ❌ 所有环境同一密码 | ✅ 每个环境独立密码 |
| **密码强度** | ❌ 弱密码 `admin123` | ✅ 支持强密码 |

---

### 修改文件列表

1. **prisma/seed.ts** - 支持环境变量配置初始密码
2. **app/actions/auth.ts** - 添加 `updateAdminPassword()` 函数
3. **app/admin/settings/page.tsx** - 添加修改密码表单
4. **app/(auth)/login/page.tsx** - 移除密码提示
5. **.env.example** - 添加 `ADMIN_DEFAULT_PASSWORD` 配置
6. **docs/DEPLOYMENT.md** - 更新密码管理文档

---

## 🚀 后续优化建议

### P0 - 高优先级

- [ ] 添加分布式锁（Redis）支持更高并发
- [ ] 实现幂等性（避免重复提交）
- [ ] 添加请求限流（防止暴力攻击）

### P1 - 中优先级

- [ ] 监控告警（Sentry / Prometheus）
- [ ] 自动清理孤儿数据脚本
- [ ] 补充单元测试和集成测试

### P2 - 低优先级

- [ ] Emby 操作队列化（异步处理）
- [ ] 数据库读写分离
- [ ] CDN 加速静态资源

---

## 📝 总结

本次修复解决了 **6 个核心问题**，包括：
- 2 个严重的数据一致性问题
- 3 个中等的状态同步和并发问题
- 1 个严重的安全隐患（密码管理）

所有修复均遵循以下原则：
1. **最终一致性** - 确保 Emby 和数据库最终状态一致
2. **补偿机制** - 失败时自动尝试恢复
3. **可观测性** - 所有异常情况都有日志追踪
4. **实用主义** - 解决实际问题，不过度设计
5. **安全第一** - 生产环境部署的安全性保障

**修复后的系统具备生产环境部署条件。**

---

**修复人员**: Kong Hang (with Claude Code)
**修复日期**: 2025-12-08 ~ 2025-12-09
**下一步**: 部署到生产环境并监控运行状况
