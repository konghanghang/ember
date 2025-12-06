# Ember - 架构设计文档

> **项目名称：Ember**
> 日期：2025-12-06
> 版本：v1.0

---

## 系统架构概览

```
┌─────────────────────────────────────────────┐
│           Docker Container (Port 8080)       │
│                                              │
│  ┌────────────────────────────────────────┐ │
│  │         Go (Gin) Web Server            │ │
│  │                                        │ │
│  │  ┌──────────────┐  ┌────────────────┐ │ │
│  │  │  API Routes  │  │  Static Files  │ │ │
│  │  │  /api/*      │  │  /*            │ │ │
│  │  └──────────────┘  └────────────────┘ │ │
│  │         │                   │          │ │
│  │         ▼                   ▼          │ │
│  │  ┌─────────────────────────────────┐  │ │
│  │  │     Business Logic Layer        │  │ │
│  │  │  - User Service                 │  │ │
│  │  │  - Invite Service               │  │ │
│  │  │  - Emby Client                  │  │ │
│  │  │  - MoviePilot Client            │  │ │
│  │  │  - Email Service                │  │ │
│  │  └─────────────────────────────────┘  │ │
│  │                   │                    │ │
│  │                   ▼                    │ │
│  │  ┌─────────────────────────────────┐  │ │
│  │  │     Data Access Layer (GORM)    │  │ │
│  │  └─────────────────────────────────┘  │ │
│  └────────────────────────────────────────┘ │
│                     │                        │
└─────────────────────┼────────────────────────┘
                      │
                      ▼
        ┌──────────────────────────┐
        │   PostgreSQL Database    │
        │   (Separate Container)   │
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

---

## 单镜像部署架构

### 构建流程

```
┌─────────────────────────────────────────────┐
│          Multi-Stage Dockerfile             │
├─────────────────────────────────────────────┤
│                                             │
│  Stage 1: Build Frontend                    │
│  ┌─────────────────────────────────────┐   │
│  │  FROM node:20-alpine                │   │
│  │  WORKDIR /app/frontend              │   │
│  │  COPY frontend/ .                   │   │
│  │  RUN npm ci                         │   │
│  │  RUN npm run build                  │   │
│  │  → Output: .next/standalone         │   │
│  └─────────────────────────────────────┘   │
│                                             │
│  Stage 2: Build Backend                     │
│  ┌─────────────────────────────────────┐   │
│  │  FROM golang:1.22-alpine            │   │
│  │  WORKDIR /app/backend               │   │
│  │  COPY backend/ .                    │   │
│  │  RUN go mod download                │   │
│  │  RUN go build -o server cmd/main.go │   │
│  │  → Output: server (binary)          │   │
│  └─────────────────────────────────────┘   │
│                                             │
│  Stage 3: Final Image                       │
│  ┌─────────────────────────────────────┐   │
│  │  FROM alpine:latest                 │   │
│  │  COPY --from=stage1 .next/standalone│   │
│  │  COPY --from=stage2 server          │   │
│  │  EXPOSE 8080                        │   │
│  │  CMD ["./server"]                   │   │
│  └─────────────────────────────────────┘   │
└─────────────────────────────────────────────┘
```

### 路由设计

**Go (Gin) 路由分配：**

```go
router := gin.Default()

// API 路由（优先匹配）
api := router.Group("/api/v1")
{
    api.POST("/auth/login", authHandler.Login)
    api.POST("/auth/refresh", authHandler.Refresh)

    // 需要认证的路由
    authorized := api.Group("")
    authorized.Use(authMiddleware.JWTAuth())
    {
        authorized.GET("/users", userHandler.List)
        authorized.POST("/invites", inviteHandler.Create)
        // ... 其他 API
    }
}

// 静态文件（Next.js 输出）
router.NoRoute(func(c *gin.Context) {
    // 托管 Next.js standalone 输出
    c.File("./frontend/.next/standalone" + c.Request.URL.Path)
})
```

### 端口映射

```yaml
# docker-compose.yml
services:
  emby-manager:
    ports:
      - "8080:8080"   # 单一端口
    environment:
      - DATABASE_URL=postgres://...
      - JWT_SECRET=...
```

---

## 项目目录结构

```
ember/
├── backend/                    # Go 后端
│   ├── cmd/
│   │   └── server/
│   │       └── main.go        # 程序入口
│   │
│   ├── internal/
│   │   ├── api/               # HTTP 处理器
│   │   │   ├── handler/
│   │   │   │   ├── auth_handler.go
│   │   │   │   ├── user_handler.go
│   │   │   │   ├── invite_handler.go
│   │   │   │   └── moviepilot_handler.go
│   │   │   ├── middleware/
│   │   │   │   ├── auth.go
│   │   │   │   ├── cors.go
│   │   │   │   └── logger.go
│   │   │   └── router/
│   │   │       └── router.go
│   │   │
│   │   ├── service/           # 业务逻辑
│   │   │   ├── user_service.go
│   │   │   ├── invite_service.go
│   │   │   ├── auth_service.go
│   │   │   ├── emby_service.go
│   │   │   └── moviepilot_service.go
│   │   │
│   │   ├── repository/        # 数据访问
│   │   │   ├── user_repo.go
│   │   │   ├── invite_repo.go
│   │   │   └── audit_repo.go
│   │   │
│   │   ├── model/             # 数据模型
│   │   │   ├── user.go
│   │   │   ├── invite.go
│   │   │   ├── profile.go
│   │   │   └── audit.go
│   │   │
│   │   ├── client/            # 外部客户端
│   │   │   ├── emby/
│   │   │   │   ├── client.go
│   │   │   │   └── types.go
│   │   │   └── moviepilot/
│   │   │       ├── client.go
│   │   │       └── types.go
│   │   │
│   │   ├── notification/      # 通知服务
│   │   │   ├── email.go
│   │   │   └── template.go
│   │   │
│   │   ├── scheduler/         # 定时任务
│   │   │   ├── cron.go
│   │   │   └── jobs/
│   │   │       ├── expiry_check.go
│   │   │       └── cleanup.go
│   │   │
│   │   └── config/            # 配置
│   │       └── config.go
│   │
│   ├── pkg/                   # 公共库
│   │   ├── jwt/
│   │   │   └── jwt.go
│   │   ├── validator/
│   │   │   └── validator.go
│   │   └── utils/
│   │       ├── hash.go
│   │       └── random.go
│   │
│   ├── migrations/            # 数据库迁移（可选）
│   │   └── 001_init.sql
│   │
│   ├── go.mod
│   └── go.sum
│
├── frontend/                   # Next.js 前端
│   ├── app/
│   │   ├── (auth)/            # 认证相关页面
│   │   │   ├── login/
│   │   │   │   └── page.tsx
│   │   │   └── register/
│   │   │       └── page.tsx
│   │   │
│   │   ├── admin/             # 管理后台
│   │   │   ├── layout.tsx
│   │   │   ├── dashboard/
│   │   │   │   └── page.tsx
│   │   │   ├── users/
│   │   │   │   ├── page.tsx
│   │   │   │   └── [id]/
│   │   │   │       └── page.tsx
│   │   │   ├── invites/
│   │   │   │   └── page.tsx
│   │   │   ├── profiles/
│   │   │   │   └── page.tsx
│   │   │   └── settings/
│   │   │       └── page.tsx
│   │   │
│   │   ├── user/              # 用户面板
│   │   │   ├── layout.tsx
│   │   │   ├── profile/
│   │   │   │   └── page.tsx
│   │   │   ├── devices/
│   │   │   │   └── page.tsx
│   │   │   └── moviepilot/
│   │   │       └── page.tsx
│   │   │
│   │   ├── layout.tsx
│   │   └── page.tsx
│   │
│   ├── components/            # UI 组件
│   │   ├── ui/               # shadcn/ui 组件
│   │   ├── admin/            # 管理后台组件
│   │   │   ├── user-table.tsx
│   │   │   ├── invite-form.tsx
│   │   │   └── stats-card.tsx
│   │   └── user/             # 用户面板组件
│   │       ├── profile-card.tsx
│   │       └── device-list.tsx
│   │
│   ├── lib/                  # 工具函数
│   │   ├── api.ts           # API 客户端
│   │   ├── auth.ts          # 认证工具
│   │   └── utils.ts         # 通用工具
│   │
│   ├── types/                # TypeScript 类型
│   │   ├── user.ts
│   │   ├── invite.ts
│   │   └── api.ts
│   │
│   ├── next.config.js
│   ├── package.json
│   └── tailwind.config.ts
│
├── Dockerfile                 # 单镜像构建
├── docker-compose.yml         # 完整部署配置
├── Makefile                   # 构建脚本
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

## 下一步：数据库设计

现在技术架构已经清晰，我们需要设计数据库 Schema。

**需要讨论的表：**
1. users - 用户表
2. invites - 邀请码表
3. profiles - 权限配置表
4. audit_logs - 操作日志表
5. notifications - 通知队列表
6. refresh_tokens - Refresh Token 存储表（可选）

准备好讨论数据库设计了吗？
