# Ember - 项目总览

> ⚠️ **此文档已归档（设计草稿）**
>
> 这是设计阶段的项目总览文档，包含完整的 Phase 1/2/3 规划。
>
> **实际开发以 [`docs/specs/`](./specs/) 为准，当前只开发 MVP 版本。**

---

> **最后更新：** 2025-12-06
> **状态：** 设计阶段 → 准备进入数据库设计

---

## 📋 项目信息

| 项目 | 信息 |
|------|------|
| **名称** | Ember |
| **定位** | 现代化 Emby 用户管理系统 |
| **替代** | embyboss (解决其稳定性问题) |
| **参考** | Wizarr (UI 设计) + jfa-go (功能设计) |

---

## ✅ 已确定的决策

### 技术栈

```
┌─────────────────────────────────────┐
│           技术栈总览                │
├─────────────────────────────────────┤
│ 全栈框架：Next.js 15                │
│       - App Router                  │
│       - Server Actions              │
│       - Server Components           │
│       - TypeScript 5.x              │
│                                     │
│ 数据层：Prisma ORM                  │
│       - PostgreSQL 16.x             │
│       - 类型安全的查询              │
│                                     │
│ 认证：NextAuth.js v5                │
│       - 或自实现 JWT                │
│                                     │
│ UI：shadcn/ui + Tailwind CSS        │
│                                     │
│ 部署：Vercel 或 Docker 单容器       │
└─────────────────────────────────────┘
```

### 核心功能

**Phase 1 (MVP - 必须有):**
- ✅ 管理员认证系统
- ✅ Emby API 集成
- ✅ 邀请码生成和验证
- ✅ 用户注册流程
- ✅ 用户管理 (CRUD + 批量操作)
- ✅ 账号到期管理 (自动禁用 + 30天后删除)
- ✅ 权限配置模板
- ✅ 用户自助面板
- ✅ 邮件通知系统
- ✅ 基础统计和监控

**Phase 2 (增强功能):**
- ✅ MoviePilot 完整集成 (搜索、订阅、状态)
- ✅ 批量操作功能
- ✅ 设备管理和会话控制
- ✅ 客户端过滤 (禁止爬虫)
- ✅ 操作审计日志
- ✅ 数据库自动备份
- ✅ Webhook 支持

**Phase 3 (可选):**
- ⚠️ 引荐系统
- ⚠️ Telegram Bot 通知
- ⚠️ 多管理员权限
- ⚠️ UI 定制

### 功能细节决策

| 功能 | 决策 |
|------|------|
| **邀请码格式** | 随机生成 8-12 位 (如 `a7k9Bx2Q`) |
| **到期处理** | 立即禁用 Emby 账号 + 30天后删除数据 |
| **MoviePilot** | 完整集成 - 搜索、订阅、状态查询 |
| **认证方式** | JWT (Access 15分钟 + Refresh 7天) |
| **密码策略** | 最小 8 位 + 数字字母混合 |
| **部署方式** | 单 Docker 镜像 (Go 托管 Next.js 静态文件) |

---

## 📁 项目结构

```
ember/
├── app/                        # Next.js App Router
│   ├── (auth)/                # 认证页面组 (layout 共享)
│   │   ├── login/
│   │   └── register/
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
│   ├── actions/               # Server Actions（推荐）
│   │   ├── auth-actions.ts
│   │   ├── user-actions.ts
│   │   └── invite-actions.ts
│   │
│   ├── api/                   # API Routes（可选）
│   │   └── webhooks/
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
├── Dockerfile                 # Docker 构建
├── docker-compose.yml
└── README.md
```

---

## 🎯 路由设计

### Server Actions（推荐方式）

```typescript
// app/actions/auth-actions.ts
'use server'
export async function login(data: LoginInput)
export async function logout()

// app/actions/user-actions.ts
'use server'
export async function getUsers()
export async function createUser(data: CreateUserInput)
export async function updateUser(id: string, data: UpdateUserInput)
export async function deleteUser(id: string)
export async function extendExpiry(id: string, days: number)

// app/actions/invite-actions.ts
'use server'
export async function getInvites()
export async function createInvite(data: CreateInviteInput)
export async function validateInvite(code: string)
export async function deleteInvite(id: string)

// app/actions/moviepilot-actions.ts
'use server'
export async function searchMovies(query: string)
export async function subscribe(movieId: string)
export async function getSubscriptionStatus(id: string)
```

### API Routes（可选，用于 Webhook）

```
/api
├── /webhooks
│   ├── POST /emby           # Emby Webhook
│   └── POST /moviepilot     # MoviePilot Webhook
│
└── /cron (Vercel Cron)
    ├── GET /check-expiry    # 检查到期账号
    └── GET /cleanup         # 清理已删除账号
```

### 前端路由 (Next.js)

```
/                           # 首页
/login                      # 登录
/register?code=xxx          # 注册

/admin                      # 管理后台
  ├── /dashboard           # 仪表盘
  ├── /users               # 用户管理
  ├── /invites             # 邀请码管理
  ├── /profiles            # 权限配置
  └── /settings            # 系统设置

/user                       # 用户面板
  ├── /profile             # 个人信息
  ├── /devices             # 设备管理
  └── /moviepilot          # MoviePilot 订阅
```

---

## 🗄️ 数据库设计 (待完成)

**核心表：**
- [ ] users - 用户表
- [ ] invites - 邀请码表
- [ ] profiles - 权限配置表
- [ ] audit_logs - 操作日志表
- [ ] notifications - 通知队列表
- [ ] refresh_tokens - Token 存储表 (可选)

---

## 🔐 认证流程

```
1. 用户登录 → 返回 Access Token (15分钟) + Refresh Token (7天)
2. Access Token 存储在 localStorage
3. Refresh Token 存储在 httpOnly Cookie
4. 每次请求带上 Access Token (Authorization Header)
5. Access Token 过期 → 自动使用 Refresh Token 获取新的
6. Refresh Token 过期 → 重新登录
```

---

## 📦 部署架构

### 方案 A：Vercel（推荐）

```
┌─────────────────────────────────┐
│    Vercel Edge Network          │
│                                 │
│  Next.js 15 App                 │
│  ├── Server Components          │
│  ├── Server Actions             │
│  └── API Routes                 │
└─────────────────────────────────┘
              │
              ▼
    ┌──────────────────┐
    │ PostgreSQL       │
    │ (Vercel Postgres │
    │  或 Supabase)    │
    └──────────────────┘
```

**启动命令：**
```bash
vercel deploy
```

### 方案 B：Docker 自建

```
┌────────────────────────────────┐
│   Docker Container (3000)      │
│                                │
│  ┌──────────────────────────┐ │
│  │   Next.js Server         │ │
│  │   - Server Actions       │ │
│  │   - API Routes           │ │
│  │   - Static Files         │ │
│  └──────────────────────────┘ │
└────────────────────────────────┘
              │
              ▼
    ┌──────────────────┐
    │   PostgreSQL     │
    └──────────────────┘
```

**启动命令：**
```bash
docker compose up -d
# 访问: http://localhost:3000
```

---

## 📅 开发计划

### 当前阶段：设计阶段

- [x] 项目命名确定
- [x] 技术栈选型
- [x] 功能需求梳理
- [x] 架构设计完成
- [ ] **下一步：数据库 Schema 设计** ⬅️ 我们在这里

### 后续阶段

1. **数据库设计** (0.5天)
   - 表结构设计
   - 关系和索引
   - 迁移策略

2. **API 接口设计** (0.5天)
   - 详细接口定义
   - 请求/响应格式
   - 错误码规范

3. **前端组件设计** (0.5天)
   - 页面布局
   - 组件拆分
   - 状态管理

4. **项目初始化** (1天)
   - 创建项目结构
   - 配置开发环境
   - 基础框架搭建

5. **核心功能开发** (2-3周)
   - Phase 1 功能实现
   - 测试和调试

6. **增强功能开发** (1-2周)
   - Phase 2 功能实现
   - MoviePilot 集成

7. **测试和部署** (3-5天)
   - 单元测试
   - 集成测试
   - Docker 镜像构建
   - 文档完善

---

## 📚 相关文档

1. **emby-manager-design.md** - 详细功能需求和决策记录
2. **architecture-design.md** - 系统架构详细设计
3. **PROJECT-SUMMARY.md** - 本文档（项目总览）
4. **database-schema.md** - 数据库设计（待创建）
5. **api-specification.md** - API 接口文档（待创建）

---

## 🎨 Logo 设计方向

```
🔥 Ember

或

E  ← 火焰图标 + 首字母 E
```

配色建议：
- 主色：橙红色渐变 (#FF6B35 → #FF8E53)
- 辅色：深灰色 (#2C3E50)
- 背景：白色/深色模式

---

## 🤝 贡献指南

目前处于设计阶段，欢迎：
- ✅ 功能建议
- ✅ 架构讨论
- ✅ UI/UX 反馈

---

## 📝 变更记录

- **2025-12-06**
  - 项目命名确定为 Ember
  - 完成技术选型
  - 完成架构设计
  - 准备进入数据库设计阶段

---

**下一步：开始详细的数据库 Schema 设计**
