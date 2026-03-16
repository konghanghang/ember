# Ember MVP - 核心用户管理系统 - Design Document

> 历史文档：本文档描述的是 Ember 早期基于 Next.js 15 单体架构的 MVP 设计，不代表当前系统实现。
> 当前系统事实请以 [system-architecture.md](../system-architecture.md) 和 `docs/reference/`、`docs/runbooks/` 为准。

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

## Phase 2: 用户面板与订阅系统设计 ✅ 已实施

**更新时间**: 2025-12-13
**背景**: 基于 MVP 真实用户反馈设计
**状态**: 已完成开发并上线（2025-12-13）

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

**Phase 2 设计文档完成，已实施（2025-12-13）。**
# Ember MVP - 核心用户管理系统 - Requirements Document

Ember 的最小可行产品（MVP）版本，专注于核心功能：邀请码注册、用户管理、到期控制。砍掉了所有非必要功能（MoviePilot、邮件通知、批量操作、操作审计等），目标是 2 周内上线一个可用的系统。

## Core Features

### 1. 管理员认证
- 管理员使用用户名 + 密码登录
- 使用 JWT Token 认证（7 天有效期）
- Token 存储在 localStorage
- 保护所有管理后台路由

### 2. 邀请码管理
- 生成随机 8 位邀请码（如 `a7K9bX2Q`）
- 设置邀请码属性：
  - 最大使用次数（默认 1 次）
  - 可选的过期时间
  - 默认账号到期天数（如 30 天）
- 查看邀请码列表（显示使用情况）
- 删除未使用的邀请码

### 3. 用户注册
- 用户通过邀请码注册
- 填写：用户名、密码、邮箱
- 系统自动：
  - 验证邀请码有效性
  - 调用 Emby API 创建用户
  - 保存用户信息到数据库
  - 设置账号到期时间
- 注册成功后返回 Emby 服务器地址

### 4. 用户管理
- 查看用户列表：
  - 用户名、邮箱
  - 创建时间
  - 到期时间（显示剩余天数）
  - 账号状态（正常/已禁用）
- 搜索用户（按用户名）
- 延长账号到期时间
- 禁用/启用账号（同步到 Emby）
- 删除用户（同时删除 Emby 用户）

### 5. 账号到期处理
- 定时任务（每天凌晨 2:00）：
  - 扫描已到期账号
  - 调用 Emby API 禁用账号
  - 更新数据库状态
- 手动触发到期检查（管理后台）

### 6. 基础配置
- Emby 服务器配置（URL + API Key）
- 连接状态检测
- 默认用户权限（固定配置，不可修改）

## User Stories

### 管理员
- **作为管理员**，我要登录系统，以便管理用户
- **作为管理员**，我要生成邀请码，以便让新用户注册
- **作为管理员**，我要查看用户列表，以便了解用户状态
- **作为管理员**，我要延长用户到期时间，以便给用户续期
- **作为管理员**，我要禁用/删除用户，以便管理不活跃用户

### 用户
- **作为用户**，我要使用邀请码注册，以便获得 Emby 账号
- **作为用户**，我要知道账号何时到期，以便及时续期

## Acceptance Criteria

### 管理员登录
- [ ] 使用错误密码登录失败，返回错误提示
- [ ] 使用正确密码登录成功，返回 JWT Token
- [ ] Token 过期后自动跳转登录页
- [ ] 未登录访问管理页面自动跳转登录

### 邀请码生成
- [ ] 生成的邀请码为 8 位随机字符串（大小写字母+数字）
- [ ] 邀请码不重复
- [ ] 设置最大使用次数后，使用次数达到上限时邀请码失效
- [ ] 设置过期时间后，过期邀请码无法使用

### 用户注册
- [ ] 使用无效邀请码注册失败
- [ ] 用户名已存在时注册失败
- [ ] 注册成功后在 Emby 能看到新用户
- [ ] 注册成功后数据库有对应记录
- [ ] 邀请码使用次数正确更新

### 用户管理
- [ ] 用户列表正确显示所有用户
- [ ] 搜索功能能找到匹配的用户
- [ ] 延长到期时间后，数据库和显示都更新
- [ ] 禁用账号后，Emby 用户无法登录
- [ ] 删除用户后，Emby 用户也被删除

### 账号到期
- [ ] 定时任务每天执行一次
- [ ] 到期账号被正确禁用
- [ ] 未到期账号不受影响
- [ ] 手动触发检查功能正常

## Non-functional Requirements

### 性能要求
- 用户列表加载时间 < 1 秒（1000 用户以内）
- 注册流程完成时间 < 3 秒
- Emby API 调用失败时有重试机制（最多 3 次）

### 安全要求
- 管理员密码使用 bcrypt 加密存储（cost=10）
- JWT Token 使用强随机密钥
- 所有 API 调用验证 Token
- Emby API Key 不在前端暴露

### 兼容性要求
- 支持 PostgreSQL 14+
- 支持 Emby Server 4.7+
- 前端兼容现代浏览器（Chrome 90+, Firefox 88+, Safari 14+）

### 可靠性要求
- Emby API 调用失败时，事务回滚（不创建半成品用户）
- 定时任务异常不影响系统运行
- 数据库连接失败时有友好提示

### 可维护性要求
- 代码使用 TypeScript 严格模式
- 关键操作有日志记录（登录、注册、删除用户）
- 配置通过环境变量管理，不硬编码

## Out of Scope (明确不做的功能)

以下功能**不在 MVP 范围内**，等 MVP 上线后根据反馈再决定：

- ❌ 邮件通知（欢迎邮件、到期提醒）
- ❌ MoviePilot 集成
- ❌ 批量操作（批量删除、批量续期）
- ❌ 权限配置模板（使用固定默认权限）
- ❌ 用户自助面板（见 Phase 2 规划）
- ❌ 操作审计日志（只记录基础日志）
- ❌ 设备管理（查看/踢出设备）
- ❌ 统计仪表盘（用户数、活跃度）
- ❌ Webhook 支持
- ❌ 多管理员
- ❌ 数据库备份功能

## Phase 2 规划（基于真实用户反馈）

**背景**：MVP 已上线，根据真实用户反馈规划 Phase 2 功能。

**状态**：✅ 已完成（2025-12-13）

---

### 优先级 P0：用户自助面板 ✅

**需求来源**：5 位用户强烈要求

**功能描述**：
用户可以使用 Emby 账号密码登录 Ember，查看自己的账号信息并管理个人设置。

**核心功能**：

1. **用户认证**
   - 使用 Emby 账号密码登录（不是 Ember 独立密码）
   - 验证账号是否有效（未过期、未禁用）
   - 生成用户专用 JWT Token（user-token cookie，与管理员 auth-token 分离）
   - Token 有效期 7 天

2. **账号信息查看**
   - 显示：用户名、邮箱、Emby ID
   - 显示：到期时间、剩余天数
   - 显示：账号状态（正常/已禁用）
   - 显示：Emby 服务器地址

3. **密码和邮箱管理**
   - 修改密码（同步到 Emby，验证当前密码后允许修改）
   - 修改邮箱（仅更新本地数据库，Emby API 不支持修改邮箱）

**新增路由**：
- `/user/login` - 用户登录页
- `/user/dashboard` - 用户仪表盘
- `/user/subscriptions` - 用户订阅列表（Phase 2.2）
- `/user/subscriptions/new` - 提交新订阅（Phase 2.2）

**验收标准**：
- [x] 用户可以用 Emby 账号密码登录 Ember
- [x] 用户可以查看自己的账号信息（用户名、邮箱、到期时间）
- [x] 用户可以查看剩余天数
- [x] 用户可以修改密码，修改后立即在 Emby 生效
- [x] 用户可以修改邮箱（仅本地数据库，Emby 不支持）
- [x] 已过期用户无法登录
- [x] 已禁用用户无法登录
- [x] 用户只能看到自己的数据（路由保护）
- [x] 用户与管理员权限完全隔离（通过 role 字段区分）

**开发时间估算**：2 天 ✅

---

### 优先级 P0：MoviePilot 订阅集成 ✅

**需求来源**：用户主要使用场景，强烈需求

**功能描述**：
用户可以提交影视订阅请求，管理员审核后自动提交到 MoviePilot。

**核心功能**：

1. **用户提交订阅**
   - 方式一：输入 TMDB ID（快速，推荐）
   - 方式二：搜索影视名称（未来优化）
   - 必填信息：
     - 类型（movie / tv）
     - 影视名称
     - TMDB ID
   - 可选信息：
     - 用户备注
   - 提交后状态为 "pending"（待审核）

2. **用户管理订阅**
   - 查看自己的订阅列表（按提交时间倒序）
   - 删除 pending 状态的订阅（已审核的不可删除）
   - 查看订阅状态：pending / approved / rejected

3. **管理员审核订阅**
   - 查看所有待审核订阅（可按状态筛选）
   - 显示用户信息、影视名称、TMDB ID
   - 审核通过：调用 MoviePilot API 创建订阅
   - 审核拒绝：订阅状态变为 rejected

4. **MoviePilot API 集成**
   - 使用 OAuth2 认证（用户名密码 → access_token）
   - 调用 `POST /api/v1/subscribe/` 创建订阅
   - 参数映射：
     - movie → "电影"
     - tv → "电视剧"
   - 错误处理：API 失败时显示明确错误信息

**订阅状态机**：
```
pending → (approve) → approved (immutable)
        ↘ (reject) → rejected (terminal)
```

**新增路由**：
- `/user/subscriptions` - 用户订阅列表
- `/user/subscriptions/new` - 提交新订阅
- `/admin/subscriptions` - 管理员审核页面

**验收标准**：
- [x] 用户可以输入 TMDB ID 提交订阅
- [x] 用户可以查看自己的订阅列表
- [x] 用户可以删除 pending 状态的订阅
- [x] 用户无法删除 approved/rejected 状态的订阅
- [x] 管理员可以查看所有待审核订阅
- [x] 管理员可以审核通过订阅（调用 MP API）
- [x] 管理员可以拒绝订阅
- [x] MP API 调用成功后订阅状态变为 approved
- [x] MP API 调用失败时显示明确错误信息
- [x] 所有关键操作记录到日志表

**开发时间估算**：3 天（2 天订阅系统 + 1 天 MP 集成）✅

---

### 优先级 P1：未来增强功能（等 Phase 2.1 完成后决定）

- **邮件通知**：到期提醒（用户可能更喜欢 Telegram/微信）
- **批量操作**：管理员批量续期/删除（取决于用户量）
- **统计仪表盘**：用户活跃度分析（取决于数据需求）
- **Jellyfin 集成**：有用户使用 Jellyfin 而不是 Emby（取决于反馈）

## Technical Constraints

### 必须使用的技术
- **框架**: Next.js 15 (App Router)
- **语言**: TypeScript 5.x (严格模式)
- **数据库**: PostgreSQL 16+
- **ORM**: Prisma
- **认证**: 自实现 JWT（不使用 NextAuth.js）
- **UI**: shadcn/ui + Tailwind CSS
- **定时任务**: node-cron

### 部署方式
- Docker 单容器部署
- 使用 docker-compose 管理服务

### 数据库设计约束
- 只需要 4 张表：
  1. `admins` - 管理员表
  2. `invites` - 邀请码表
  3. `users` - 用户表
  4. `logs` - 简单日志表（可选）

## Success Metrics

MVP 成功的标准：

1. **功能完整性**: 6 个核心功能全部实现并测试通过
2. **开发周期**: 2 周内完成（10 个工作日）
3. **代码质量**: TypeScript 编译无错误，无明显性能问题
4. **可部署性**: 能通过 `docker-compose up` 一键部署
5. **可用性**: 能完成完整的用户注册-使用-到期流程

## Development Plan

### Week 1 (Day 1-5)
- **Day 1-2**: 项目初始化 + 数据库设计 + 管理员登录
- **Day 3-4**: 邀请码生成 + 用户注册（含 Emby 集成）
- **Day 5**: 用户列表 + 延长到期时间

### Week 2 (Day 6-10)
- **Day 6-7**: 禁用/删除用户 + 定时任务
- **Day 8**: Docker 部署 + 配置管理
- **Day 9-10**: 测试 + Bug 修复 + 文档

## Risks and Mitigation

### 风险 1: Emby API 不稳定
- **影响**: 用户注册/删除失败
- **缓解**: 实现重试机制，失败时回滚数据库操作

### 风险 2: 时间估算不准
- **影响**: 2 周内无法完成
- **缓解**: 已砍掉所有非核心功能，保留最小功能集

### 风险 3: PostgreSQL 数据丢失
- **影响**: 用户数据丢失
- **缓解**: 提醒用户配置 PostgreSQL 数据卷持久化（docker-compose）

## Appendix

### Emby API 端点（需要使用）
- `POST /Users/New` - 创建用户
- `DELETE /Users/{userId}` - 删除用户
- `POST /Users/{userId}/Policy` - 更新用户权限
- `GET /Users` - 获取用户列表

### 默认用户权限配置
```json
{
  "IsAdministrator": false,
  "IsDisabled": false,
  "EnableAllFolders": true,
  "EnabledFolders": [],
  "EnableRemoteAccess": true,
  "EnableLiveTvAccess": false,
  "EnableContentDeletion": false,
  "EnableContentDownloading": false,
  "EnableSyncTranscoding": false,
  "EnableMediaPlayback": true,
  "EnableAudioPlaybackTranscoding": true,
  "EnableVideoPlaybackTranscoding": true,
  "EnablePlaybackRemuxing": true
}
```
