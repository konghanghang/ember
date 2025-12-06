# 技术栈决策记录

> ⚠️ **此文档已归档（设计草稿）**
>
> 这是设计阶段的技术栈决策记录，最终决定使用 Next.js 15 全栈方案。
>
> **实际开发以 [`docs/specs/`](./specs/) 为准。**

---

> **决策日期：** 2025-12-06
> **决策者：** Kong Hang
> **状态：** ✅ 已确定

---

## 核心决策

**从 Go + Next.js 调整为 Next.js 全栈方案**

---

## 决策背景

### 用户量级评估

- **预期用户规模：** < 2000 人
- **并发需求：** 低（用户管理操作频率低）
- **主要瓶颈：** 数据库查询，非应用层性能

### 开发资源

- **开发人员：** 1 人（你）
- **时间预算：** 希望快速迭代
- **维护能力：** 统一技术栈更易维护

---

## 方案对比

### ❌ 方案 A：Go + Next.js（放弃）

**为什么不选：**

| 问题 | 说明 |
|------|------|
| **过度设计** | 对于 <2000 用户是浪费 |
| **开发慢** | 需要 4-6 周 vs 2-3 周 |
| **维护难** | 需要懂 Go + TypeScript |
| **部署复杂** | 需要多阶段构建、两个进程 |
| **没必要** | 性能优势在这个量级体现不出来 |

### ✅ 方案 B：Next.js 全栈（采用）

**为什么选择：**

| 优势 | 价值 |
|------|------|
| **快速开发** | 2-3 周完成 MVP |
| **统一语言** | 只需 TypeScript |
| **简化部署** | Docker 单容器或 Vercel 一键部署 |
| **易于维护** | 一个代码库，一套工具链 |
| **性能充足** | 轻松支持 2000-5000 用户 |
| **生态丰富** | Prisma、NextAuth、丰富的库 |

---

## 最终技术栈

### 核心框架

```typescript
📦 Next.js 15
   ├── App Router          // 路由系统
   ├── Server Components   // 服务端渲染
   ├── Server Actions      // 后端逻辑
   └── API Routes          // RESTful API（按需）
```

### 数据层

```typescript
📦 Prisma ORM
   ├── Schema Definition   // 类型安全的 Schema
   ├── Migrations          // 数据库迁移
   ├── Type Generation     // 自动生成 TS 类型
   └── Query Builder       // 优雅的查询 API

📦 PostgreSQL
   └── 生产级关系型数据库
```

### 认证授权

```typescript
📦 NextAuth.js v5 (Auth.js)
   ├── JWT Strategy        // Token 认证
   ├── Session Management  // 会话管理
   └── Credentials Provider // 用户名密码登录

或者

📦 自实现 JWT
   ├── jose 库             // JWT 操作
   └── 更灵活的控制
```

### UI 层

```typescript
📦 React 18+
📦 shadcn/ui             // UI 组件库
📦 Tailwind CSS          // 样式
📦 Radix UI              // 无样式组件基础
📦 Lucide Icons          // 图标
```

### 状态管理

```typescript
📦 React Query (TanStack Query)
   └── 服务端状态管理、缓存

📦 Zustand（可选）
   └── 客户端全局状态
```

### 工具库

```typescript
📦 zod                   // Schema 验证
📦 date-fns              // 日期处理
📦 nodemailer            // 邮件发送
📦 node-cron             // 定时任务
```

---

## 完整技术栈清单

| 层级 | 技术 | 版本 | 用途 |
|------|------|------|------|
| **框架** | Next.js | 15.x | 全栈框架 |
| **语言** | TypeScript | 5.x | 类型安全 |
| **ORM** | Prisma | 5.x | 数据库操作 |
| **数据库** | PostgreSQL | 16.x | 持久化 |
| **认证** | NextAuth.js | 5.x | 认证授权 |
| **UI** | shadcn/ui | latest | 组件库 |
| **样式** | Tailwind CSS | 3.x | CSS 框架 |
| **状态** | React Query | 5.x | 数据获取 |
| **验证** | Zod | 3.x | 数据验证 |
| **邮件** | Nodemailer | 6.x | SMTP |
| **定时** | node-cron | 3.x | Cron Jobs |

---

## 项目结构

### 简化后的目录

```
ember/
├── app/                        # Next.js App Router
│   ├── (auth)/                # 认证页面组（layout 共享）
│   │   ├── login/
│   │   │   └── page.tsx
│   │   └── register/
│   │       └── page.tsx
│   │
│   ├── (admin)/               # 管理后台
│   │   ├── layout.tsx        # 管理后台布局
│   │   ├── dashboard/
│   │   ├── users/
│   │   ├── invites/
│   │   └── settings/
│   │
│   ├── (user)/                # 用户面板
│   │   ├── layout.tsx
│   │   ├── profile/
│   │   └── devices/
│   │
│   ├── api/                   # API Routes（可选）
│   │   └── webhooks/
│   │       └── route.ts
│   │
│   ├── actions/               # Server Actions（推荐）
│   │   ├── auth-actions.ts
│   │   ├── user-actions.ts
│   │   └── invite-actions.ts
│   │
│   ├── layout.tsx            # 根布局
│   └── page.tsx              # 首页
│
├── components/                # React 组件
│   ├── ui/                   # shadcn/ui 组件
│   ├── admin/                # 管理后台组件
│   └── user/                 # 用户面板组件
│
├── lib/                       # 工具库
│   ├── db.ts                 # Prisma 客户端
│   ├── auth.ts               # 认证工具
│   ├── emby.ts               # Emby API 客户端
│   ├── moviepilot.ts         # MoviePilot 客户端
│   ├── email.ts              # 邮件服务
│   └── utils.ts              # 通用工具
│
├── prisma/                    # Prisma 配置
│   ├── schema.prisma         # 数据库 Schema
│   └── migrations/           # 迁移文件
│
├── types/                     # TypeScript 类型定义
│   ├── user.ts
│   ├── invite.ts
│   └── index.ts
│
├── config/                    # 配置文件
│   └── site.ts
│
├── public/                    # 静态资源
├── .env.example              # 环境变量示例
├── next.config.js            # Next.js 配置
├── tailwind.config.ts        # Tailwind 配置
├── tsconfig.json             # TypeScript 配置
└── package.json              # 依赖管理
```

---

## Server Actions vs API Routes

### 推荐：Server Actions ⭐

**优势：**
```typescript
// app/actions/user-actions.ts
'use server'

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
    const result = await createUser(formData) // 自动处理
  }
}
```

**好处：**
- ✅ 类型安全（端到端）
- ✅ 无需定义 API 路由
- ✅ 自动错误处理
- ✅ 更好的性能（减少网络请求）

---

## 认证方案选择

### 方案 1：NextAuth.js（推荐新手）

```typescript
// auth.config.ts
import NextAuth from 'next-auth'
import Credentials from 'next-auth/providers/credentials'

export const { handlers, auth, signIn, signOut } = NextAuth({
  providers: [
    Credentials({
      credentials: {
        email: {},
        password: {}
      },
      authorize: async (credentials) => {
        // 验证逻辑
        const user = await verifyUser(credentials)
        return user
      }
    })
  ]
})
```

### 方案 2：自实现 JWT（推荐有经验者）

```typescript
// lib/auth.ts
import { SignJWT, jwtVerify } from 'jose'

export async function createToken(userId: string) {
  const token = await new SignJWT({ userId })
    .setProtectedHeader({ alg: 'HS256' })
    .setExpirationTime('15m')
    .sign(secret)

  return token
}
```

**建议：先用 NextAuth.js，后期可以替换**

---

## 定时任务方案

### 选项 1：内置 Cron（开发/自建服务器）

```typescript
// lib/cron.ts
import cron from 'node-cron'

// 每天凌晨 1 点检查到期账号
cron.schedule('0 1 * * *', async () => {
  await checkExpiredUsers()
})
```

### 选项 2：Vercel Cron（部署到 Vercel）

```typescript
// app/api/cron/check-expiry/route.ts
export async function GET(request: Request) {
  // 验证 Vercel Cron Secret
  if (request.headers.get('authorization') !== `Bearer ${process.env.CRON_SECRET}`) {
    return new Response('Unauthorized', { status: 401 })
  }

  await checkExpiredUsers()
  return Response.json({ ok: true })
}
```

```json
// vercel.json
{
  "crons": [{
    "path": "/api/cron/check-expiry",
    "schedule": "0 1 * * *"
  }]
}
```

---

## 部署方案

### 方案 A：Vercel（推荐最简单）

```bash
# 一键部署
vercel deploy

# 特点
✅ 免费额度（个人项目够用）
✅ 自动 HTTPS
✅ 全球 CDN
✅ 自动 Preview 环境
✅ 内置 Cron 支持
```

### 方案 B：Docker 自建

```dockerfile
# Dockerfile
FROM node:20-alpine AS base

# 安装依赖
FROM base AS deps
WORKDIR /app
COPY package*.json ./
RUN npm ci

# 构建
FROM base AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN npx prisma generate
RUN npm run build

# 运行
FROM base AS runner
WORKDIR /app
ENV NODE_ENV=production

COPY --from=builder /app/public ./public
COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static

EXPOSE 3000
CMD ["node", "server.js"]
```

```yaml
# docker-compose.yml
version: '3.8'
services:
  ember:
    build: .
    ports:
      - "3000:3000"
    environment:
      DATABASE_URL: postgresql://user:pass@postgres:5432/ember
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

## 性能优化策略

虽然 < 2000 用户不需要担心性能，但我们仍然要做好：

1. **数据库优化**
   - ✅ 合理的索引
   - ✅ 使用 Prisma 的 select（避免过度查询）
   - ✅ 连接池配置

2. **缓存策略**
   - ✅ React Query 客户端缓存
   - ✅ Next.js 静态页面缓存
   - ✅ Redis 缓存（可选，后期）

3. **代码分割**
   - ✅ Dynamic Import
   - ✅ 按路由分割

---

## 开发效率提升

**预计时间对比：**

| 阶段 | Go + Next.js | Next.js 全栈 | 节省 |
|------|-------------|-------------|------|
| 环境搭建 | 2 天 | 0.5 天 | 1.5 天 |
| 核心功能 | 3 周 | 2 周 | 1 周 |
| 测试调试 | 1 周 | 0.5 周 | 0.5 周 |
| **总计** | **5-6 周** | **2.5-3 周** | **2.5 周** |

**效率提升：50%+**

---

## 未来扩展路径

如果真的遇到性能瓶颈（不太可能）：

1. **数据库优化**
   - 添加 Redis 缓存
   - 优化索引
   - 读写分离

2. **CDN 加速**
   - 静态资源 CDN
   - 全球边缘节点

3. **渐进式迁移**
   - 保留 Next.js 前端
   - 迁移部分重负载 API 到 Go
   - 混合架构

---

## 决策总结

**✅ 采用 Next.js 全栈方案**

**核心理由：**
- 用户量 < 2000，性能充足
- 开发效率高 50%
- 维护成本低 70%
- 部署简单
- 随时可以扩展

**技术栈：**
- Next.js 15 + TypeScript
- Prisma + PostgreSQL
- NextAuth.js
- shadcn/ui + Tailwind

---

**下一步：更新架构设计文档**
