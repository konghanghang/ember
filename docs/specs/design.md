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

## Future Improvements (Phase 2)

MVP 完成后，可以考虑：

1. **邮件通知**：Nodemailer + 模板系统
2. **MoviePilot 集成**：新增 MoviePilot API 客户端
3. **批量操作**：用户列表批量选择
4. **用户自助面板**：用户登录、查看到期时间
5. **Redis 缓存**：缓存用户列表、邀请码列表
6. **Webhook**：用户注册/到期事件通知

---

**设计文档完成，等待审核。**
