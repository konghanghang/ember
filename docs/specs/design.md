# Ember MVP - 核心用户管理系统 - Design Document

## Overview

Ember MVP 采用 **Next.js 15 全栈单体架构**，所有功能在一个应用中实现。

**核心设计原则**：
1. **数据结构优先**：4 张表，清晰的关系，零冗余
2. **消除特殊情况**：固定用户权限，不搞"可配置"
3. **简洁实用**：Server Actions 直接操作数据库，无中间层
4. **事务安全**：Emby API 调用失败时回滚数据库

**技术栈**：
- Next.js 15 App Router + TypeScript 5.x
- Prisma ORM + PostgreSQL 16
- shadcn/ui + Tailwind CSS
- Docker 单容器部署

---

## Architecture

### 系统分层

```
┌─────────────────────────────────────┐
│   Next.js 15 Application            │
│                                     │
│  ┌───────────────────────────────┐ │
│  │  Presentation Layer           │ │  ← React Server Components
│  │  (app/*)                      │ │
│  └───────────────────────────────┘ │
│               │                     │
│               ▼                     │
│  ┌───────────────────────────────┐ │
│  │  Business Logic Layer         │ │  ← Server Actions
│  │  (app/actions/*)              │ │     (直接操作数据库)
│  └───────────────────────────────┘ │
│               │                     │
│               ▼                     │
│  ┌───────────────────────────────┐ │
│  │  Data Layer                   │ │  ← Prisma Client
│  │  (lib/db.ts)                  │ │
│  └───────────────────────────────┘ │
└─────────────────┬───────────────────┘
                  │
                  ▼
         ┌────────────────┐
         │  PostgreSQL    │
         └────────────────┘
                  │
         ┌────────┴────────┐
         ▼                 ▼
    ┌────────┐        ┌────────┐
    │  Emby  │        │  SMTP  │
    │ Server │        │(未来)  │
    └────────┘        └────────┘
```

### 目录结构

```
ember/
├── app/
│   ├── (auth)/
│   │   ├── login/page.tsx              # 管理员登录
│   │   └── register/page.tsx           # 用户注册
│   │
│   ├── (admin)/
│   │   ├── layout.tsx                  # 管理后台布局（验证登录）
│   │   ├── dashboard/page.tsx          # 仪表盘（可选）
│   │   ├── users/page.tsx              # 用户列表
│   │   ├── users/[id]/page.tsx         # 用户详情
│   │   ├── invites/page.tsx            # 邀请码管理
│   │   └── settings/page.tsx           # 系统设置
│   │
│   ├── actions/
│   │   ├── auth.ts                     # 认证 Server Actions
│   │   ├── users.ts                    # 用户管理 Server Actions
│   │   ├── invites.ts                  # 邀请码 Server Actions
│   │   └── cron.ts                     # 定时任务 Server Actions
│   │
│   └── api/
│       └── cron/check-expiry/route.ts  # Cron Job 端点（可选）
│
├── components/
│   ├── ui/                             # shadcn/ui 组件
│   ├── admin/
│   │   ├── user-table.tsx              # 用户列表表格
│   │   ├── invite-form.tsx             # 邀请码生成表单
│   │   └── user-detail-card.tsx        # 用户详情卡片
│   └── shared/
│       ├── navbar.tsx                  # 导航栏
│       └── auth-guard.tsx              # 登录验证组件
│
├── lib/
│   ├── db.ts                           # Prisma 客户端单例
│   ├── auth.ts                         # JWT 工具函数
│   ├── emby.ts                         # Emby API 客户端
│   ├── utils.ts                        # 通用工具函数
│   └── cron.ts                         # 定时任务（node-cron）
│
├── prisma/
│   ├── schema.prisma                   # 数据库 Schema
│   └── migrations/                     # 迁移文件
│
└── types/
    ├── auth.ts                         # 认证类型
    ├── user.ts                         # 用户类型
    └── invite.ts                       # 邀请码类型
```

---

## Data Models

### Prisma Schema

```prisma
// prisma/schema.prisma

generator client {
  provider = "prisma-client-js"
}

datasource db {
  provider = "postgresql"
  url      = env("DATABASE_URL")
}

// 管理员表
model Admin {
  id        String   @id @default(cuid())
  username  String   @unique
  password  String   // bcrypt hash, cost=10
  createdAt DateTime @default(now())
  updatedAt DateTime @updatedAt

  @@map("admins")
}

// 邀请码表
model Invite {
  id            String    @id @default(cuid())
  code          String    @unique // 8位随机字符串
  maxUses       Int       @default(1) // 最大使用次数
  usedCount     Int       @default(0) // 已使用次数
  expiresAt     DateTime? // 可选的过期时间
  defaultDays   Int       @default(30) // 默认账号有效天数
  createdAt     DateTime  @default(now())

  // 关系：一个邀请码可以创建多个用户
  users         User[]

  @@map("invites")
  @@index([code])
}

// 用户表
model User {
  id           String    @id @default(cuid())
  username     String    @unique
  email        String
  embyId       String    @unique // Emby用户ID（Emby返回）
  inviteCode   String    // 使用的邀请码
  expiresAt    DateTime? // 账号到期时间（null=永久有效）
  isActive     Boolean   @default(true) // 账号状态
  createdAt    DateTime  @default(now())
  updatedAt    DateTime  @updatedAt

  // 关系：属于某个邀请码
  invite       Invite    @relation(fields: [inviteCode], references: [code])

  @@map("users")
  @@index([username])
  @@index([embyId])
  @@index([expiresAt]) // 定时任务查询到期用户
}

// 操作日志表（简化版，可选）
model Log {
  id        String   @id @default(cuid())
  action    String   // "admin_login", "create_user", "delete_user", etc.
  targetId  String?  // 目标对象ID（如用户ID）
  details   Json?    // 额外信息（JSON格式）
  createdAt DateTime @default(now())

  @@map("logs")
  @@index([action])
  @@index([createdAt])
}
```

### 数据关系图

```
┌─────────┐
│ Admin   │  (管理员)
└─────────┘
  - 不关联其他表
  - 只用于认证

┌─────────┐       ┌──────────┐
│ Invite  │ 1 ──< │   User   │  (一个邀请码创建多个用户)
└─────────┘   N   └──────────┘
  - code (唯一)      - embyId (唯一)
  - maxUses          - inviteCode (外键)
  - usedCount        - expiresAt (到期时间)

┌─────────┐
│  Log    │  (独立日志表)
└─────────┘
  - 不关联其他表
  - 只记录操作历史
```

### 核心字段设计说明

| 字段 | 设计决策 | 原因 |
|------|----------|------|
| `Invite.code` | 8位随机字符串（大小写+数字） | 易读、唯一性高、不易猜测 |
| `Invite.maxUses` | 默认 1 | 常见场景：一码一用 |
| `User.embyId` | 存储 Emby 返回的用户ID | Emby 是 Single Source of Truth |
| `User.expiresAt` | `DateTime?`（可为 null） | null = 永久有效，简化逻辑 |
| `User.isActive` | Boolean | 禁用后不删除数据，方便恢复 |
| `Admin.password` | bcrypt hash | 不存储明文，cost=10 平衡安全和性能 |

---

## Components and Interfaces

### Server Actions API

所有 Server Actions 使用 `'use server'` 指令，直接在服务端执行。

#### `app/actions/auth.ts`

```typescript
'use server'

import { prisma } from '@/lib/db'
import { signToken, verifyPassword, hashPassword } from '@/lib/auth'
import { cookies } from 'next/headers'

// 管理员登录
export async function adminLogin(data: { username: string; password: string }) {
  const admin = await prisma.admin.findUnique({
    where: { username: data.username }
  })

  if (!admin || !(await verifyPassword(data.password, admin.password))) {
    throw new Error('用户名或密码错误')
  }

  const token = signToken({ id: admin.id, username: admin.username })

  // 记录日志
  await prisma.log.create({
    data: { action: 'admin_login', targetId: admin.id }
  })

  return { token }
}

// 验证Token（中间件使用）
export async function verifyToken(token: string) {
  // 验证JWT token
  // 返回 { id, username } 或抛出异常
}
```

#### `app/actions/invites.ts`

```typescript
'use server'

import { prisma } from '@/lib/db'
import { generateInviteCode } from '@/lib/utils'

// 生成邀请码
export async function createInvite(data: {
  maxUses?: number
  expiresAt?: Date
  defaultDays?: number
}) {
  const code = generateInviteCode(8) // 8位随机码

  const invite = await prisma.invite.create({
    data: {
      code,
      maxUses: data.maxUses || 1,
      expiresAt: data.expiresAt,
      defaultDays: data.defaultDays || 30
    }
  })

  await prisma.log.create({
    data: { action: 'create_invite', targetId: invite.id }
  })

  return invite
}

// 获取邀请码列表
export async function getInvites() {
  return prisma.invite.findMany({
    include: { users: true }, // 包含使用该邀请码的用户
    orderBy: { createdAt: 'desc' }
  })
}

// 删除邀请码
export async function deleteInvite(id: string) {
  // 检查是否已被使用
  const invite = await prisma.invite.findUnique({
    where: { id },
    include: { users: true }
  })

  if (invite.usedCount > 0) {
    throw new Error('邀请码已被使用，无法删除')
  }

  await prisma.invite.delete({ where: { id } })
}

// 验证邀请码
export async function validateInvite(code: string) {
  const invite = await prisma.invite.findUnique({ where: { code } })

  if (!invite) throw new Error('邀请码不存在')
  if (invite.usedCount >= invite.maxUses) throw new Error('邀请码已达使用上限')
  if (invite.expiresAt && invite.expiresAt < new Date()) throw new Error('邀请码已过期')

  return invite
}
```

#### `app/actions/users.ts`

```typescript
'use server'

import { prisma } from '@/lib/db'
import { embyClient } from '@/lib/emby'
import { add } from 'date-fns'

// 用户注册
export async function registerUser(data: {
  username: string
  password: string
  email: string
  inviteCode: string
}) {
  // 1. 验证邀请码
  const invite = await validateInvite(data.inviteCode)

  // 2. 使用事务：创建 Emby 用户 + 保存数据库
  return await prisma.$transaction(async (tx) => {
    // 2.1 创建 Emby 用户
    const embyUser = await embyClient.createUser({
      username: data.username,
      password: data.password
    })

    // 2.2 保存到数据库
    const user = await tx.user.create({
      data: {
        username: data.username,
        email: data.email,
        embyId: embyUser.Id,
        inviteCode: data.inviteCode,
        expiresAt: add(new Date(), { days: invite.defaultDays })
      }
    })

    // 2.3 更新邀请码使用次数
    await tx.invite.update({
      where: { code: data.inviteCode },
      data: { usedCount: { increment: 1 } }
    })

    // 2.4 记录日志
    await tx.log.create({
      data: { action: 'create_user', targetId: user.id }
    })

    return user
  })
}

// 获取用户列表
export async function getUsers(params?: { search?: string }) {
  return prisma.user.findMany({
    where: params?.search ? {
      username: { contains: params.search, mode: 'insensitive' }
    } : undefined,
    include: { invite: true },
    orderBy: { createdAt: 'desc' }
  })
}

// 延长到期时间
export async function extendExpiry(userId: string, days: number) {
  const user = await prisma.user.findUnique({ where: { id: userId } })
  if (!user) throw new Error('用户不存在')

  const newExpiresAt = add(user.expiresAt || new Date(), { days })

  await prisma.user.update({
    where: { id: userId },
    data: { expiresAt: newExpiresAt }
  })

  await prisma.log.create({
    data: {
      action: 'extend_expiry',
      targetId: userId,
      details: { days }
    }
  })
}

// 禁用/启用用户
export async function toggleUserStatus(userId: string) {
  const user = await prisma.user.findUnique({ where: { id: userId } })
  if (!user) throw new Error('用户不存在')

  // 同步到 Emby
  await embyClient.setUserPolicy(user.embyId, {
    IsDisabled: !user.isActive
  })

  // 更新数据库
  await prisma.user.update({
    where: { id: userId },
    data: { isActive: !user.isActive }
  })

  await prisma.log.create({
    data: {
      action: user.isActive ? 'disable_user' : 'enable_user',
      targetId: userId
    }
  })
}

// 删除用户
export async function deleteUser(userId: string) {
  const user = await prisma.user.findUnique({ where: { id: userId } })
  if (!user) throw new Error('用户不存在')

  await prisma.$transaction(async (tx) => {
    // 删除 Emby 用户
    await embyClient.deleteUser(user.embyId)

    // 删除数据库记录
    await tx.user.delete({ where: { id: userId } })

    // 记录日志
    await tx.log.create({
      data: {
        action: 'delete_user',
        targetId: userId,
        details: { username: user.username, embyId: user.embyId }
      }
    })
  })
}
```

#### `app/actions/cron.ts`

```typescript
'use server'

import { prisma } from '@/lib/db'
import { embyClient } from '@/lib/emby'

// 检查并禁用过期账号
export async function checkExpiredUsers() {
  const now = new Date()

  // 查找已到期但仍启用的用户
  const expiredUsers = await prisma.user.findMany({
    where: {
      expiresAt: { lte: now },
      isActive: true
    }
  })

  for (const user of expiredUsers) {
    try {
      // 禁用 Emby 用户
      await embyClient.setUserPolicy(user.embyId, { IsDisabled: true })

      // 更新数据库
      await prisma.user.update({
        where: { id: user.id },
        data: { isActive: false }
      })

      // 记录日志
      await prisma.log.create({
        data: {
          action: 'auto_disable_expired',
          targetId: user.id
        }
      })
    } catch (error) {
      console.error(`Failed to disable user ${user.username}:`, error)
    }
  }

  return { disabled: expiredUsers.length }
}
```

---

### 前端组件

#### `components/admin/user-table.tsx`

```typescript
'use client'

import { getUsers, extendExpiry, toggleUserStatus, deleteUser } from '@/app/actions/users'
import { useState } from 'react'

export function UserTable() {
  const [users, setUsers] = useState([])
  const [search, setSearch] = useState('')

  // 使用 Server Action 获取数据
  async function loadUsers() {
    const data = await getUsers({ search })
    setUsers(data)
  }

  // 延长到期
  async function handleExtend(userId: string, days: number) {
    await extendExpiry(userId, days)
    await loadUsers()
  }

  // 禁用/启用
  async function handleToggle(userId: string) {
    await toggleUserStatus(userId)
    await loadUsers()
  }

  // 删除
  async function handleDelete(userId: string) {
    if (confirm('确定删除该用户？')) {
      await deleteUser(userId)
      await loadUsers()
    }
  }

  return (
    <div>
      <input
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        placeholder="搜索用户名..."
      />
      <table>
        {/* 用户列表渲染 */}
      </table>
    </div>
  )
}
```

---

## Error Handling

### 错误处理策略

#### 1. Server Actions 错误

```typescript
// 统一错误格式
export class AppError extends Error {
  constructor(
    public message: string,
    public code: 'VALIDATION' | 'AUTH' | 'EMBY_API' | 'DATABASE'
  ) {
    super(message)
  }
}

// Server Action 中使用
export async function createUser(data: any) {
  try {
    // ...
  } catch (error) {
    if (error instanceof AppError) {
      throw error
    }
    throw new AppError('创建用户失败', 'DATABASE')
  }
}

// 前端捕获
try {
  await createUser(data)
} catch (error) {
  toast.error(error.message)
}
```

#### 2. Emby API 错误

```typescript
// lib/emby.ts

export class EmbyClient {
  private async request(endpoint: string, options?: RequestInit) {
    const url = `${this.baseUrl}${endpoint}`

    // 重试机制：最多3次
    for (let i = 0; i < 3; i++) {
      try {
        const res = await fetch(url, {
          ...options,
          headers: {
            'X-Emby-Token': this.apiKey,
            ...options?.headers
          }
        })

        if (!res.ok) {
          throw new AppError(`Emby API 错误: ${res.statusText}`, 'EMBY_API')
        }

        return await res.json()
      } catch (error) {
        if (i === 2) throw error // 最后一次重试失败，抛出异常
        await new Promise(r => setTimeout(r, 1000 * (i + 1))) // 指数退避
      }
    }
  }
}
```

#### 3. 数据库事务回滚

```typescript
// 用户注册失败时回滚
export async function registerUser(data: any) {
  return await prisma.$transaction(async (tx) => {
    // 创建 Emby 用户
    const embyUser = await embyClient.createUser(data)
    // ↑ 如果这里失败，事务自动回滚，数据库不会有脏数据

    // 保存到数据库
    const user = await tx.user.create({ data })

    return user
  })
}
```

---

## Testing Strategy

### MVP 阶段测试策略（简化）

#### 1. 手动测试（主要方式）

**测试清单**：
- [ ] 管理员登录（正确密码 / 错误密码）
- [ ] 生成邀请码（验证唯一性）
- [ ] 用户注册（验证 Emby 用户创建成功）
- [ ] 用户列表（验证搜索功能）
- [ ] 延长到期时间（验证数据库更新）
- [ ] 禁用/启用用户（验证 Emby 同步）
- [ ] 删除用户（验证 Emby 用户也被删除）
- [ ] 定时任务（手动触发，验证过期用户被禁用）

#### 2. 集成测试（可选）

```typescript
// __tests__/user.test.ts

import { registerUser } from '@/app/actions/users'
import { prisma } from '@/lib/db'

describe('User Registration', () => {
  it('should create user in both Emby and database', async () => {
    const data = {
      username: 'testuser',
      password: 'password123',
      email: 'test@example.com',
      inviteCode: 'TEST1234'
    }

    const user = await registerUser(data)

    expect(user.username).toBe('testuser')
    expect(user.embyId).toBeTruthy()

    // 清理
    await prisma.user.delete({ where: { id: user.id } })
  })
})
```

#### 3. E2E 测试（Phase 2）

MVP 阶段**不做** E2E 测试，等功能稳定后再考虑。

---

## Deployment

### Docker 部署

#### Dockerfile

```dockerfile
FROM node:20-alpine AS deps
WORKDIR /app
COPY package*.json ./
RUN npm ci

FROM node:20-alpine AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN npx prisma generate
RUN npm run build

FROM node:20-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production

COPY --from=builder /app/public ./public
COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static
COPY --from=builder /app/prisma ./prisma
COPY --from=builder /app/node_modules/.prisma ./node_modules/.prisma

EXPOSE 3000
CMD ["node", "server.js"]
```

#### docker-compose.yml

```yaml
version: '3.8'

services:
  ember:
    build: .
    ports:
      - "3000:3000"
    environment:
      - DATABASE_URL=postgresql://postgres:password@postgres:5432/ember
      - JWT_SECRET=${JWT_SECRET}
      - EMBY_URL=${EMBY_URL}
      - EMBY_API_KEY=${EMBY_API_KEY}
    depends_on:
      - postgres

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_PASSWORD: password
      POSTGRES_DB: ember
    volumes:
      - postgres_data:/var/lib/postgresql/data
    ports:
      - "5432:5432"

volumes:
  postgres_data:
```

---

## Security

### 安全措施

1. **JWT Token**：
   - 7 天有效期
   - 使用强随机密钥（至少 32 字节）
   - 存储在 localStorage（仅管理员使用，XSS 风险可控）

2. **密码加密**：
   - bcrypt，cost=10
   - 不存储明文密码

3. **Emby API Key**：
   - 通过环境变量配置
   - 不在前端暴露

4. **HTTPS**：
   - 生产环境强制 HTTPS（Nginx/Caddy 配置）

5. **SQL 注入防护**：
   - Prisma 自动防护

---

## Performance

### 性能优化

1. **数据库索引**：
   - `users.username`（搜索）
   - `users.expiresAt`（定时任务）
   - `invites.code`（验证）

2. **Server Components**：
   - 默认使用 Server Components
   - 减少客户端 JavaScript

3. **缓存**：
   - MVP 阶段不实现缓存
   - Phase 2 考虑 Redis

---

## Migration Plan

### 数据库迁移

```bash
# 初始化
npx prisma migrate dev --name init

# 后续迁移
npx prisma migrate dev --name add_xxx

# 生产部署
npx prisma migrate deploy
```

### 初始化管理员

```typescript
// prisma/seed.ts

import { PrismaClient } from '@prisma/client'
import bcrypt from 'bcryptjs'

const prisma = new PrismaClient()

async function main() {
  const password = await bcrypt.hash('admin123', 10)

  await prisma.admin.upsert({
    where: { username: 'admin' },
    update: {},
    create: {
      username: 'admin',
      password
    }
  })
}

main()
```

---

## Phase 2: 用户面板与订阅系统设计

**更新时间**: 2025-12-13
**背景**: 基于 MVP 真实用户反馈设计

### Phase 2.1: 用户认证与面板

#### 数据库 Schema 变更

**无需修改现有表**，直接使用 MVP 的 User 表。

**关键设计决策**：
- ✅ 用户使用 Emby 账号密码登录（不存储 Ember 独立密码）
- ✅ 直接调用 Emby API 验证用户
- ✅ 无需在 User 表新增 password 字段

#### 认证机制设计

**统一 Token 系统**：

| 配置项      | 值                                     |
|-------------|---------------------------------------|
| Cookie 名称 | auth-token                            |
| 有效期      | 7 天                                  |
| httpOnly    | true                                  |
| secure      | production 环境启用                    |

**JWT Payload 统一格式**：

```typescript
// 管理员
{
  id: string          // Admin.id
  username: string
  role: 'admin'
}

// 用户
{
  id: string          // User.id
  username: string
  embyId: string
  role: 'user'
}
```

**设计理由**：
- ✅ **简化逻辑**：一个 cookie，一套验证逻辑
- ✅ **安全隔离**：通过 role 字段区分权限（middleware 一个 if 判断）
- ✅ **消除特殊情况**：不需要两套 token 管理代码

#### Emby API 新增方法

**lib/emby.ts** 新增：

```typescript
class EmbyClient {
  // 用户认证（Phase 2 新增）
  async authenticateUser(username: string, password: string): Promise<{
    Id: string
    Name: string
    ServerId: string
  }> {
    // POST /Users/AuthenticateByName
    // Body: { Username, Pw }
    // 返回 { User: { Id, Name }, AccessToken, ServerId }
  }

  // 修改用户密码（Phase 2 新增）
  async updateUserPassword(userId: string, currentPwd: string, newPwd: string): Promise<void> {
    // 1. 先用 authenticateUser() 验证当前密码
    // 2. 调用 POST /Users/{userId}/Password
    // Body: { CurrentPw, NewPw }
  }
}
```

**注意**：删除了 `updateUserEmail()` 方法。经查阅 Emby API 文档，不支持直接修改用户邮箱。邮箱只存储在本地数据库中。

#### Server Actions 设计

**新文件**: `app/actions/user-auth.ts`

```typescript
'use server'

import { prisma } from '@/lib/db'
import { embyClient } from '@/lib/emby'
import { signToken } from '@/lib/auth'
import { cookies } from 'next/headers'

// 用户登录
export async function userLogin(data: {
  username: string
  password: string
}) {
  // 1. 调用 Emby API 验证用户
  const embyUser = await embyClient.authenticateUser(data.username, data.password)

  // 2. 查询本地数据库（通过 embyId）
  const user = await prisma.user.findUnique({
    where: { embyId: embyUser.Id }
  })

  if (!user) {
    throw new Error('用户不存在（未通过 Ember 注册）')
  }

  // 3. 验证账号是否有效
  if (!user.isActive) {
    throw new Error('账号已禁用，请联系管理员')
  }

  if (user.expiresAt && user.expiresAt < new Date()) {
    throw new Error('账号已过期，请联系管理员续期')
  }

  // 4. 生成 JWT Token
  const token = signToken({
    id: user.id,
    username: user.username,
    embyId: user.embyId,
    role: 'user' // 区分管理员和用户
  })

  // 5. 设置 cookie（httpOnly, secure，与管理员使用相同的 cookie 名称）
  cookies().set('auth-token', token, {
    httpOnly: true,
    secure: process.env.NODE_ENV === 'production',
    maxAge: 7 * 24 * 60 * 60 // 7 天
  })

  // 6. 记录日志
  await prisma.log.create({
    data: {
      action: 'user_login',
      targetId: user.id,
      details: { username: user.username }
    }
  })

  return { success: true, user: { id: user.id, username: user.username } }
}

// 获取当前用户信息
export async function getUserAuth() {
  const token = cookies().get('auth-token')?.value
  if (!token) return null

  try {
    const payload = verifyToken(token)
    if (payload.role !== 'user') return null // 只允许用户 token

    const user = await prisma.user.findUnique({
      where: { id: payload.id }
    })

    return user
  } catch {
    return null
  }
}

// 用户登出
export async function userLogout() {
  cookies().delete('auth-token')
  return { success: true }
}

// 修改密码
export async function updateUserPassword(data: {
  currentPassword: string
  newPassword: string
}) {
  const user = await getUserAuth()
  if (!user) {
    throw new Error('未登录')
  }

  // 验证新密码强度
  if (data.newPassword.length < 6) {
    throw new Error('密码至少 6 个字符')
  }

  // 调用 Emby API 修改密码
  await embyClient.updateUserPassword(
    user.embyId,
    data.currentPassword,
    data.newPassword
  )

  // 记录日志
  await prisma.log.create({
    data: {
      action: 'user_change_password',
      targetId: user.id
    }
  })

  return { success: true }
}

// 修改邮箱（仅更新本地数据库，Emby 不支持修改邮箱）
export async function updateUserEmail(email: string) {
  const user = await getUserAuth()
  if (!user) {
    throw new Error('未登录')
  }

  // 验证邮箱格式
  if (!/^[\w-\.]+@([\w-]+\.)+[\w-]{2,4}$/.test(email)) {
    throw new Error('邮箱格式不正确')
  }

  // 更新本地数据库
  await prisma.user.update({
    where: { id: user.id },
    data: { email }
  })

  // 记录日志
  await prisma.log.create({
    data: {
      action: 'user_change_email',
      targetId: user.id,
      details: { newEmail: email }
    }
  })

  return { success: true }
}
```

#### 路由保护（Middleware）

**middleware.ts** 修改（简化为一套逻辑）：

```typescript
export const config = {
  matcher: ['/admin/:path*', '/user/:path*', '/login', '/user/login']
}

export async function middleware(request: NextRequest) {
  const { pathname } = request.nextUrl

  // 统一获取 token
  const token = request.cookies.get('auth-token')?.value

  // 验证 token
  let payload: { id: string; username: string; role: 'admin' | 'user'; embyId?: string } | null = null
  if (token) {
    try {
      payload = verifyToken(token)
    } catch {
      // Token 无效或过期，清除 cookie
      const response = NextResponse.redirect(new URL('/login', request.url))
      response.cookies.delete('auth-token')
      return response
    }
  }

  // 管理员路由保护
  if (pathname.startsWith('/admin')) {
    if (!payload || payload.role !== 'admin') {
      return NextResponse.redirect(new URL('/login?redirect=' + pathname, request.url))
    }
  }

  // 用户路由保护
  if (pathname.startsWith('/user') && pathname !== '/user/login') {
    if (!payload || payload.role !== 'user') {
      return NextResponse.redirect(new URL('/user/login?redirect=' + pathname, request.url))
    }
  }

  // 已登录用户访问登录页，重定向到对应的仪表盘
  if (pathname === '/login' && payload?.role === 'admin') {
    return NextResponse.redirect(new URL('/admin/users', request.url))
  }

  if (pathname === '/user/login' && payload?.role === 'user') {
    return NextResponse.redirect(new URL('/user/dashboard', request.url))
  }

  return NextResponse.next()
}
```

#### 目录结构变更

```diff
 ember/
 ├── app/
 │   ├── (auth)/
 │   │   ├── login/page.tsx              # 管理员登录
 │   │   └── register/page.tsx           # 用户注册
 │   │
++│   ├── (user)/                          # Phase 2 新增
++│   │   ├── layout.tsx                   # 用户布局
++│   │   ├── login/page.tsx               # 用户登录
++│   │   ├── dashboard/page.tsx           # 用户仪表盘
++│   │   └── subscriptions/               # Phase 2.2 订阅系统
 │   │
 │   ├── (admin)/
++│   │   ├── subscriptions/page.tsx       # Phase 2.2 管理员审核
 │   │
 │   ├── actions/
 │   │   ├── auth.ts                     # 管理员认证
++│   │   ├── user-auth.ts                 # Phase 2 用户认证
++│   │   └── subscriptions.ts             # Phase 2.2 订阅管理
 │   │
 ├── lib/
++│   ├── moviepilot.ts                    # Phase 2.2 MoviePilot API
```

---

### Phase 2.2: MoviePilot 订阅集成

#### 数据库 Schema 变更

**新增 Subscription 表**：

```prisma
// prisma/schema.prisma

// 订阅状态枚举
enum SubscriptionStatus {
  PENDING  // 待审核
  APPROVED // 已批准
  REJECTED // 已拒绝
}

// 媒体类型枚举
enum MediaType {
  MOVIE // 电影
  TV    // 电视剧
}

model Subscription {
  id        String             @id @default(cuid())
  userId    String             // 关联用户
  type      MediaType          // 媒体类型（电影/电视剧）
  name      String             // 影视名称
  tmdbId    String             // TMDB ID（唯一标识）
  status    SubscriptionStatus @default(PENDING) // 订阅状态
  note      String?            // 用户备注（可选）
  createdAt DateTime           @default(now())
  updatedAt DateTime           @updatedAt

  // 关系：属于某个用户
  user User @relation(fields: [userId], references: [id], onDelete: Cascade)

  @@map("subscriptions")
  @@index([userId])
  @@index([status])
  @@index([createdAt])
}

// 修改 User 表（新增关系）
model User {
  // ... 现有字段
  subscriptions Subscription[] // Phase 2 新增
}
```

**数据库迁移**：

```bash
npx prisma migrate dev --name add_subscriptions
npx prisma generate
```

#### MoviePilot API 客户端

**新文件**: `lib/moviepilot.ts`

```typescript
export class MoviePilotClient {
  private baseUrl: string
  private username: string
  private password: string

  constructor() {
    this.baseUrl = process.env.MOVIEPILOT_URL!.replace(/\/$/, '')
    this.username = process.env.MOVIEPILOT_USERNAME!
    this.password = process.env.MOVIEPILOT_PASSWORD!
  }

  // 获取 Access Token（每次都重新登录，不缓存）
  private async login(): Promise<string> {
    const response = await fetch(`${this.baseUrl}/api/v1/login/access-token`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
      },
      body: new URLSearchParams({
        username: this.username,
        password: this.password,
      }),
    })

    if (!response.ok) {
      throw new Error(`MoviePilot 登录失败: ${response.statusText}`)
    }

    const data = await response.json()
    return data.access_token
  }

  // 创建订阅
  async createSubscription(data: {
    type: 'movie' | 'tv'
    name: string
    tmdbid: string
  }) {
    // 每次调用都重新登录获取 token
    const token = await this.login()

    const response = await fetch(`${this.baseUrl}/api/v1/subscribe/`, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        type: data.type === 'movie' ? '电影' : '电视剧',
        name: data.name,
        tmdbid: parseInt(data.tmdbid),
      }),
    })

    if (!response.ok) {
      const error = await response.text()
      throw new Error(`MoviePilot API 错误: ${error}`)
    }

    return await response.json()
  }
}

export const moviepilotClient = new MoviePilotClient()
```

#### 订阅 Server Actions

**新文件**: `app/actions/subscriptions.ts`

```typescript
'use server'

import { prisma } from '@/lib/db'
import { getUserAuth } from './user-auth'
import { getCurrentAdmin } from './auth'
import { moviepilotClient } from '@/lib/moviepilot'

// 用户提交订阅
export async function createSubscription(data: {
  type: 'movie' | 'tv'
  name: string
  tmdbId: string
  note?: string
}) {
  // 1. 获取当前用户
  const user = await getUserAuth()
  if (!user) {
    throw new Error('未登录')
  }

  // 2. 验证用户是否有效
  if (!user.isActive) {
    throw new Error('账号已禁用，无法提交订阅')
  }

  if (user.expiresAt && user.expiresAt < new Date()) {
    throw new Error('账号已过期，无法提交订阅')
  }

  // 3. 创建订阅记录
  const subscription = await prisma.subscription.create({
    data: {
      userId: user.id,
      type: data.type,
      name: data.name,
      tmdbId: data.tmdbId,
      note: data.note,
      status: 'PENDING'
    }
  })

  // 4. 记录日志
  await prisma.log.create({
    data: {
      action: 'create_subscription',
      targetId: subscription.id,
      details: {
        userId: user.id,
        username: user.username,
        name: data.name,
        tmdbId: data.tmdbId
      }
    }
  })

  return { success: true, subscription }
}

// 用户查看自己的订阅列表
export async function getUserSubscriptions() {
  const user = await getUserAuth()
  if (!user) {
    throw new Error('未登录')
  }

  const subscriptions = await prisma.subscription.findMany({
    where: { userId: user.id },
    orderBy: { createdAt: 'desc' }
  })

  return { success: true, subscriptions }
}

// 用户删除订阅（只能删除 pending）
export async function deleteSubscription(id: string) {
  const user = await getUserAuth()
  if (!user) {
    throw new Error('未登录')
  }

  const subscription = await prisma.subscription.findUnique({
    where: { id }
  })

  if (!subscription) {
    throw new Error('订阅不存在')
  }

  if (subscription.userId !== user.id) {
    throw new Error('无权操作此订阅')
  }

  if (subscription.status !== 'PENDING') {
    throw new Error('只能删除待审核的订阅')
  }

  await prisma.subscription.delete({
    where: { id }
  })

  // 记录日志
  await prisma.log.create({
    data: {
      action: 'delete_subscription',
      targetId: id,
      details: { name: subscription.name }
    }
  })

  return { success: true }
}

// 管理员查看所有订阅
export async function getAllSubscriptions(params?: {
  status?: string
  search?: string
}) {
  const admin = await getCurrentAdmin()
  if (!admin) {
    throw new Error('未授权')
  }

  const subscriptions = await prisma.subscription.findMany({
    where: {
      status: params?.status,
      ...(params?.search && {
        OR: [
          { name: { contains: params.search, mode: 'insensitive' } },
          { user: { username: { contains: params.search, mode: 'insensitive' } } }
        ]
      })
    },
    include: { user: true },
    orderBy: { createdAt: 'desc' }
  })

  return { success: true, subscriptions }
}

// 管理员审核通过
export async function approveSubscription(id: string) {
  const admin = await getCurrentAdmin()
  if (!admin) {
    throw new Error('未授权')
  }

  const subscription = await prisma.subscription.findUnique({
    where: { id },
    include: { user: true }
  })

  if (!subscription) {
    throw new Error('订阅不存在')
  }

  if (subscription.status !== 'PENDING') {
    throw new Error('订阅已处理')
  }

  try {
    // 调用 MoviePilot API
    await moviepilotClient.createSubscription({
      type: subscription.type as 'movie' | 'tv',
      name: subscription.name,
      tmdbid: subscription.tmdbId
    })

    // 更新状态
    await prisma.subscription.update({
      where: { id },
      data: { status: 'APPROVED', updatedAt: new Date() }
    })

    // 记录日志
    await prisma.log.create({
      data: {
        action: 'approve_subscription',
        targetId: id,
        details: {
          subscriptionId: id,
          userId: subscription.userId,
          username: subscription.user.username,
          name: subscription.name,
          tmdbId: subscription.tmdbId
        }
      }
    })

    return { success: true }
  } catch (error) {
    console.error('审核订阅失败：', error)
    return {
      success: false,
      error: error instanceof Error ? error.message : '未知错误'
    }
  }
}

// 管理员拒绝
export async function rejectSubscription(id: string, reason?: string) {
  const admin = await getCurrentAdmin()
  if (!admin) {
    throw new Error('未授权')
  }

  const subscription = await prisma.subscription.findUnique({
    where: { id }
  })

  if (!subscription) {
    throw new Error('订阅不存在')
  }

  if (subscription.status !== 'PENDING') {
    throw new Error('订阅已处理')
  }

  await prisma.subscription.update({
    where: { id },
    data: { status: 'REJECTED', updatedAt: new Date() }
  })

  // 记录日志
  await prisma.log.create({
    data: {
      action: 'reject_subscription',
      targetId: id,
      details: { reason }
    }
  })

  return { success: true }
}
```

#### 环境变量配置

**`.env`** 新增：

```bash
# MoviePilot 配置
MOVIEPILOT_URL="http://localhost:3001"
MOVIEPILOT_USERNAME="admin"
MOVIEPILOT_PASSWORD="your-password"
```

---

## Phase 2 架构图

```
┌─────────────────────────────────────┐
│   Next.js 15 Application            │
│                                     │
│  ┌───────────────────────────────┐ │
│  │  User Panel (Phase 2)         │ │
│  │  - /user/login                │ │
│  │  - /user/dashboard            │ │
│  │  - /user/subscriptions        │ │
│  └───────────────────────────────┘ │
│               │                     │
│               ▼                     │
│  ┌───────────────────────────────┐ │
│  │  Server Actions               │ │
│  │  - user-auth.ts               │ │
│  │  - subscriptions.ts           │ │
│  └───────────────────────────────┘ │
│               │                     │
│               ▼                     │
│  ┌───────────────────────────────┐ │
│  │  Data Layer                   │ │
│  │  - User, Subscription 表      │ │
│  └───────────────────────────────┘ │
└─────────────────┬───────────────────┘
                  │
         ┌────────┴────────┐
         ▼                 ▼
    ┌────────┐        ┌────────────┐
    │  Emby  │        │ MoviePilot │
    │ Server │        │            │
    └────────┘        └────────────┘
         ↑                 ↑
         │                 │
  用户认证/密码修改    订阅创建
```

---

## Phase 2 技术要点

### 认证隔离
- 管理员和用户使用不同的 cookie（auth-token vs user-token）
- JWT Payload 包含 role 字段区分身份
- 中间件分别保护 /admin/* 和 /user/* 路由

### 密码同步
- 用户密码存储在 Emby，Ember 不存储
- 修改密码时调用 Emby API
- 验证当前密码后允许修改新密码

### 订阅状态机
```
pending → (approve) → approved (immutable)
        ↘ (reject) → rejected (terminal)
```
- pending: 用户可删除
- approved/rejected: 不可修改/删除

### MoviePilot 集成
- OAuth2 认证：用户名密码 → access_token
- API 调用：POST /api/v1/subscribe/
- 参数映射：movie → "电影", tv → "电视剧"
- 错误处理：API 失败时显示明确错误

---

**Phase 2 设计文档完成，等待实施。**
