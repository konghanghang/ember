# Ember - 项目总览

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
│ 前端：Next.js 15                    │
│       - App Router                  │
│       - shadcn/ui + Tailwind CSS   │
│                                     │
│ 后端：Go 1.22+                      │
│       - Gin (Web 框架)              │
│       - GORM (ORM)                  │
│       - JWT (认证)                  │
│                                     │
│ 数据库：PostgreSQL                  │
│                                     │
│ 部署：单 Docker 镜像                │
│       - 多阶段构建                  │
│       - 单端口 (8080)               │
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
├── backend/              # Go 后端
│   ├── cmd/
│   ├── internal/
│   │   ├── api/         # HTTP 层
│   │   ├── service/     # 业务逻辑
│   │   ├── repository/  # 数据访问
│   │   ├── model/       # 数据模型
│   │   ├── client/      # 外部客户端 (Emby/MoviePilot)
│   │   └── scheduler/   # 定时任务
│   └── pkg/             # 公共库
│
├── frontend/            # Next.js 前端
│   ├── app/
│   │   ├── (auth)/     # 登录注册
│   │   ├── admin/      # 管理后台
│   │   └── user/       # 用户面板
│   ├── components/
│   └── lib/
│
├── Dockerfile           # 单镜像构建
├── docker-compose.yml
└── README.md
```

---

## 🎯 路由设计

### API 路由 (Go/Gin)

```
/api/v1
├── /auth
│   ├── POST /login          # 登录
│   ├── POST /refresh        # 刷新 Token
│   └── POST /logout         # 登出
│
├── /admin (需要管理员权限)
│   ├── /users
│   │   ├── GET    /         # 用户列表
│   │   ├── GET    /:id      # 用户详情
│   │   ├── PUT    /:id      # 更新用户
│   │   ├── DELETE /:id      # 删除用户
│   │   └── POST   /:id/extend # 延长到期
│   │
│   ├── /invites
│   │   ├── GET    /         # 邀请码列表
│   │   ├── POST   /         # 创建邀请码
│   │   └── DELETE /:id      # 删除邀请码
│   │
│   └── /profiles
│       ├── GET    /         # 配置列表
│       ├── POST   /         # 创建配置
│       └── PUT    /:id      # 更新配置
│
├── /user (需要用户权限)
│   ├── GET  /profile        # 个人信息
│   ├── PUT  /password       # 修改密码
│   ├── GET  /devices        # 设备列表
│   └── DELETE /devices/:id  # 删除设备
│
├── /moviepilot
│   ├── GET  /search         # 搜索影片
│   ├── POST /subscribe      # 订阅影片
│   └── GET  /status/:id     # 查询状态
│
└── /register
    ├── GET  /validate/:code # 验证邀请码
    └── POST /              # 用户注册
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

```
┌────────────────────────────────┐
│   Docker Container (8080)      │
│                                │
│  ┌──────────────────────────┐ │
│  │   Go (Gin) Server        │ │
│  │   - API Routes (/api/*)  │ │
│  │   - Static Files (/*)    │ │
│  │   - Next.js Standalone   │ │
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
# 访问: http://localhost:8080
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
