# Ember - 架构设计文档

> **项目名称：Ember**
> 日期：2025-12-06
> 版本：v2.0（Next.js 全栈）

---

## 系统架构概览

### 方案 A：Vercel 部署（推荐）

```
┌─────────────────────────────────────────────┐
│         Vercel Edge Network                 │
│                                             │
│  ┌────────────────────────────────────────┐ │
│  │         Next.js 15 Server              │ │
│  │                                        │ │
│  │  ┌──────────────┐  ┌────────────────┐ │ │
│  │  │Server Actions│  │Server Components│ │ │
│  │  │(推荐)        │  │                 │ │ │
│  │  └──────────────┘  └────────────────┘ │ │
│  │         │                   │          │ │
│  │         ▼                   ▼          │ │
│  │  ┌─────────────────────────────────┐  │ │
│  │  │     Business Logic Layer        │  │ │
│  │  │  - lib/services/                │  │ │
│  │  │    - user-service.ts            │  │ │
│  │  │    - invite-service.ts          │  │ │
│  │  │  - lib/clients/                 │  │ │
│  │  │    - emby.ts                    │  │ │
│  │  │    - moviepilot.ts              │  │ │
│  │  │    - email.ts                   │  │ │
│  │  └─────────────────────────────────┘  │ │
│  │                   │                    │ │
│  │                   ▼                    │ │
│  │  ┌─────────────────────────────────┐  │ │
│  │  │   Data Access Layer (Prisma)    │  │ │
│  │  │   - lib/db.ts                   │  │ │
│  │  └─────────────────────────────────┘  │ │
│  └────────────────────────────────────────┘ │
└─────────────────────┼────────────────────────┘
                      │
                      ▼
        ┌──────────────────────────┐
        │   PostgreSQL Database    │
        │   (Vercel Postgres or    │
        │    Supabase)             │
        └──────────────────────────┘
                      │
        ┌─────────────┼─────────────┐
        │             │             │
        ▼             ▼             ▼
   ┌────────┐   ┌────────┐   ┌──────────┐
   │  Emby  │   │MoviePil│   │   SMTP   │
   │ Server │   │  ot    │   │  Server  │
   └────────┘   └────────┘   └──────────┘
```

### 方案 B：Docker 自建部署

```
┌─────────────────────────────────────────────┐
│       Docker Container (Port 3000)          │
│                                             │
│  ┌────────────────────────────────────────┐ │
│  │     Next.js 15 Standalone Server       │ │
│  │     (Server Actions + API Routes)      │ │
│  └────────────────────────────────────────┘ │
└─────────────────────┼────────────────────────┘
                      │
                      ▼
        ┌──────────────────────────┐
        │   PostgreSQL Container   │
        └──────────────────────────┘
```

---

## Next.js 全栈部署架构

### 构建流程（Docker）

```
┌─────────────────────────────────────────────┐
│          Multi-Stage Dockerfile             │
├─────────────────────────────────────────────┤
│                                             │
│  Stage 1: Dependencies                      │
│  ┌─────────────────────────────────────┐   │
│  │  FROM node:20-alpine AS deps        │   │
│  │  WORKDIR /app                       │   │
│  │  COPY package*.json ./              │   │
│  │  RUN npm ci                         │   │
│  └─────────────────────────────────────┘   │
│                                             │
│  Stage 2: Build                             │
│  ┌─────────────────────────────────────┐   │
│  │  FROM node:20-alpine AS builder     │   │
│  │  WORKDIR /app                       │   │
│  │  COPY --from=deps /app/node_modules │   │
│  │  COPY . .                           │   │
│  │  RUN npx prisma generate            │   │
│  │  RUN npm run build                  │   │
│  │  → Output: .next/standalone         │   │
│  └─────────────────────────────────────┘   │
│                                             │
│  Stage 3: Runner                            │
│  ┌─────────────────────────────────────┐   │
│  │  FROM node:20-alpine AS runner      │   │
│  │  WORKDIR /app                       │   │
│  │  ENV NODE_ENV=production            │   │
│  │  COPY --from=builder /app/public    │   │
│  │  COPY --from=builder /app/.next     │   │
│  │  EXPOSE 3000                        │   │
│  │  CMD ["node", "server.js"]          │   │
│  └─────────────────────────────────────┘   │
└─────────────────────────────────────────────┘
```

### Server Actions 架构（推荐）

**Next.js Server Actions 优势：**

- ✅ 端到端类型安全（TypeScript）
- ✅ 无需手动定义 API 路由
- ✅ 自动错误处理和重试
- ✅ 更好的性能（减少网络往返）
- ✅ 内置表单处理

```typescript
// app/actions/user-actions.ts
'use server'

import { prisma } from '@/lib/db'
import { embyClient } from '@/lib/emby'

export async function createUser(data: CreateUserInput) {
  // 直接访问数据库
  const user = await prisma.user.create({ data })

  // 直接调用 Emby API
  await embyClient.createUser(user)

  return user
}

// 客户端直接调用
import { createUser } from '@/app/actions/user-actions'

function RegisterForm() {
  async function handleSubmit(formData: FormData) {
    const result = await createUser(formData) // 自动序列化
  }
}
```

### Docker Compose 配置

```yaml
# docker-compose.yml
version: '3.8'

services:
  ember:
    build: .
    ports:
      - "3000:3000"
    environment:
      - DATABASE_URL=postgresql://user:pass@postgres:5432/ember
      - NEXTAUTH_SECRET=${NEXTAUTH_SECRET}
      - EMBY_URL=${EMBY_URL}
      - EMBY_API_KEY=${EMBY_API_KEY}
    depends_on:
      - postgres

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_PASSWORD: yourpassword
      POSTGRES_DB: ember
    volumes:
      - postgres_data:/var/lib/postgresql/data

volumes:
  postgres_data:
```

---

## 项目目录结构

```
ember/
├── app/                          # Next.js App Router
│   ├── (auth)/                  # 认证页面组（共享 layout）
│   │   ├── login/
│   │   │   └── page.tsx
│   │   └── register/
│   │       └── page.tsx
│   │
│   ├── (admin)/                 # 管理后台（需要 admin 权限）
│   │   ├── layout.tsx          # 管理后台布局
│   │   ├── dashboard/
│   │   │   └── page.tsx
│   │   ├── users/
│   │   │   ├── page.tsx
│   │   │   └── [id]/
│   │   │       ├── page.tsx    # 用户详情
│   │   │       └── edit/
│   │   │           └── page.tsx
│   │   ├── invites/
│   │   │   └── page.tsx
│   │   ├── profiles/
│   │   │   └── page.tsx
│   │   └── settings/
│   │       └── page.tsx
│   │
│   ├── (user)/                  # 用户面板
│   │   ├── layout.tsx
│   │   ├── profile/
│   │   │   └── page.tsx
│   │   ├── devices/
│   │   │   └── page.tsx
│   │   └── moviepilot/
│   │       └── page.tsx
│   │
│   ├── actions/                 # Server Actions（推荐）
│   │   ├── auth-actions.ts     # 认证相关 actions
│   │   ├── user-actions.ts     # 用户管理 actions
│   │   ├── invite-actions.ts   # 邀请码 actions
│   │   └── moviepilot-actions.ts
│   │
│   ├── api/                     # API Routes（可选，用于 Webhook）
│   │   ├── auth/
│   │   │   └── [...nextauth]/
│   │   │       └── route.ts    # NextAuth.js 路由
│   │   ├── webhooks/
│   │   │   ├── emby/
│   │   │   │   └── route.ts
│   │   │   └── moviepilot/
│   │   │       └── route.ts
│   │   └── cron/               # Vercel Cron Jobs
│   │       ├── check-expiry/
│   │       │   └── route.ts
│   │       └── cleanup/
│   │           └── route.ts
│   │
│   ├── layout.tsx               # 根布局
│   ├── page.tsx                 # 首页
│   └── globals.css              # 全局样式
│
├── components/                   # React 组件
│   ├── ui/                      # shadcn/ui 组件
│   │   ├── button.tsx
│   │   ├── input.tsx
│   │   ├── table.tsx
│   │   └── ...
│   ├── admin/                   # 管理后台组件
│   │   ├── user-table.tsx
│   │   ├── invite-form.tsx
│   │   ├── stats-card.tsx
│   │   └── profile-form.tsx
│   ├── user/                    # 用户面板组件
│   │   ├── profile-card.tsx
│   │   ├── device-list.tsx
│   │   └── moviepilot-search.tsx
│   └── shared/                  # 共享组件
│       ├── header.tsx
│       ├── sidebar.tsx
│       └── footer.tsx
│
├── lib/                          # 工具库和服务
│   ├── db.ts                    # Prisma 客户端初始化
│   ├── auth.ts                  # 认证工具（NextAuth 或 JWT）
│   │
│   ├── services/                # 业务逻辑层
│   │   ├── user-service.ts
│   │   ├── invite-service.ts
│   │   └── profile-service.ts
│   │
│   ├── clients/                 # 外部 API 客户端
│   │   ├── emby.ts             # Emby API 封装
│   │   ├── moviepilot.ts       # MoviePilot API 封装
│   │   └── email.ts            # 邮件服务（Nodemailer）
│   │
│   ├── cron/                    # 定时任务（仅 Docker 部署）
│   │   ├── index.ts
│   │   ├── expiry-check.ts
│   │   └── cleanup.ts
│   │
│   ├── validators/              # Zod 验证 Schema
│   │   ├── user.ts
│   │   ├── invite.ts
│   │   └── auth.ts
│   │
│   └── utils.ts                 # 通用工具函数
│
├── prisma/                       # Prisma 配置
│   ├── schema.prisma            # 数据库 Schema
│   ├── migrations/              # 数据库迁移文件
│   └── seed.ts                  # 初始数据脚本
│
├── types/                        # TypeScript 类型定义
│   ├── user.ts
│   ├── invite.ts
│   ├── profile.ts
│   └── index.ts
│
├── config/                       # 配置文件
│   └── site.ts                  # 站点配置
│
├── public/                       # 静态资源
│   ├── images/
│   └── favicon.ico
│
├── .env.example                  # 环境变量示例
├── .env.local                    # 本地环境变量（不提交）
├── next.config.js                # Next.js 配置
├── tailwind.config.ts            # Tailwind 配置
├── tsconfig.json                 # TypeScript 配置
├── package.json
├── Dockerfile                    # Docker 构建文件
├── docker-compose.yml            # Docker Compose 配置
└── README.md
```

---

## 认证流程详解

### JWT Token 结构

**Access Token (短期):**
```json
{
  "sub": "user_id",
  "role": "admin|user",
  "exp": 1234567890,  // 15 分钟后过期
  "iat": 1234567000
}
```

**Refresh Token (长期):**
```json
{
  "sub": "user_id",
  "type": "refresh",
  "exp": 1234972800,  // 7 天后过期
  "iat": 1234567000
}
```

### 认证流程

```
┌──────┐                 ┌────────┐                ┌──────────┐
│Client│                 │Backend │                │ Database │
└──┬───┘                 └───┬────┘                └────┬─────┘
   │                         │                          │
   │ 1. POST /api/auth/login │                          │
   │ {username, password}    │                          │
   ├────────────────────────>│                          │
   │                         │ 2. 验证用户名密码          │
   │                         ├─────────────────────────>│
   │                         │<─────────────────────────┤
   │                         │ 3. 生成 Access + Refresh  │
   │ 4. 返回 Tokens          │                          │
   │<────────────────────────┤                          │
   │ {accessToken,           │                          │
   │  refreshToken}          │                          │
   │                         │                          │
   │ 5. 带 Access Token      │                          │
   │    访问保护路由          │                          │
   ├────────────────────────>│ 6. 验证 Token            │
   │                         │                          │
   │ 7. 返回数据             │                          │
   │<────────────────────────┤                          │
   │                         │                          │
   │ 8. Access Token 过期     │                          │
   │ POST /api/auth/refresh  │                          │
   │ {refreshToken}          │                          │
   ├────────────────────────>│ 9. 验证 Refresh Token    │
   │                         │                          │
   │ 10. 返回新 Access Token │                          │
   │<────────────────────────┤                          │
```

---

## 数据流示例

### 用户注册流程

```
┌────────┐   ┌─────────┐   ┌────────┐   ┌──────┐   ┌──────┐
│Frontend│   │ Backend │   │Database│   │ Emby │   │ SMTP │
└───┬────┘   └────┬────┘   └───┬────┘   └──┬───┘   └──┬───┘
    │             │             │           │          │
    │ 1. 用户填写  │             │           │          │
    │   注册表单   │             │           │          │
    │             │             │           │          │
    │ 2. POST     │             │           │          │
    │  /register  │             │           │          │
    ├────────────>│             │           │          │
    │             │ 3. 验证邀请码 │           │          │
    │             ├────────────>│           │          │
    │             │<────────────┤           │          │
    │             │ 4. 创建 Emby │           │          │
    │             │    用户      │           │          │
    │             ├───────────────────────>│          │
    │             │<─────────────────────────┤          │
    │             │ 5. 保存用户  │           │          │
    │             │    数据      │           │          │
    │             ├────────────>│           │          │
    │             │ 6. 发送欢迎  │           │          │
    │             │    邮件      │           │          │
    │             ├──────────────────────────────────>│
    │ 7. 返回成功  │             │           │          │
    │<────────────┤             │           │          │
```

---

## Next.js 全栈优势总结

### 为什么选择 Next.js 而非 Go + Next.js？

**用户规模考虑：**
- 预期用户：< 2000 人
- 并发需求：低（用户管理操作频率低）
- 性能瓶颈：数据库查询，而非应用层

**开发效率：**
| 指标 | Go + Next.js | Next.js 全栈 | 优势 |
|------|-------------|-------------|------|
| 开发时间 | 5-6 周 | 2.5-3 周 | **节省 50%** |
| 语言数量 | 2 (Go + TS) | 1 (TypeScript) | **统一技术栈** |
| 代码库 | 2 个 | 1 个 | **简化维护** |
| 部署复杂度 | 多阶段构建 | 单镜像 | **简化部署** |
| 类型安全 | 需手动同步 | 端到端自动 | **减少错误** |

**性能对比：**
- Next.js 轻松支持 2000-5000 用户
- Server Actions 减少网络往返
- 主要瓶颈在数据库，而非 Node.js vs Go

**未来扩展性：**
如果真的遇到性能瓶颈（极少可能）：
1. 添加 Redis 缓存
2. 优化数据库索引
3. 使用 Vercel Edge Functions
4. 渐进式迁移部分重负载 API 到 Go

---

## 下一步：数据库设计

现在 Next.js 全栈架构已经清晰，接下来需要设计数据库 Schema。

**待设计的表：**
1. `users` - 用户表
2. `invites` - 邀请码表
3. `profiles` - 权限配置表
4. `audit_logs` - 操作日志表
5. `notifications` - 通知队列表
6. `sessions` - 会话表（NextAuth.js 或可选）

**Prisma 优势：**
- ✅ 类型安全的查询
- ✅ 自动迁移管理
- ✅ 优雅的关系定义
- ✅ TypeScript 类型自动生成
