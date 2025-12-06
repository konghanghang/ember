# 🔥 Ember

> A modern user management system for Emby media server

**Ember** 是一个基于 **Next.js 15 全栈**的现代化 Emby 用户管理系统，采用**全栈单体架构**设计，提供邀请码注册、用户管理、账号到期控制、MoviePilot 集成等功能。

[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Next.js](https://img.shields.io/badge/next.js-15-black.svg)](https://nextjs.org/)
[![TypeScript](https://img.shields.io/badge/typescript-5.x-blue.svg)](https://www.typescriptlang.org/)
[![Prisma](https://img.shields.io/badge/prisma-5.x-2D3748.svg)](https://www.prisma.io/)

---

## ✨ 特性（MVP 版本）

> 当前处于 MVP 开发阶段，专注于核心功能

- 🎫 **邀请码系统** - 随机生成邀请码，设置使用次数和到期时间
- 👥 **用户管理** - 用户列表、搜索、延长到期、禁用/删除
- ⏰ **账号到期** - 定时任务自动禁用过期账号
- 🔐 **JWT 认证** - 管理员登录，Token 7 天有效
- 🎨 **现代 UI** - 基于 shadcn/ui 的精美界面
- 🐳 **单镜像部署** - 一个 Docker 镜像包含所有

---

## 🚀 快速开始

### 使用 Docker Compose（推荐）

```bash
# 克隆仓库
git clone https://github.com/yourusername/ember.git
cd ember

# 启动服务
docker compose up -d

# 访问应用
open http://localhost:8080
```

### 从源码构建

```bash
# 安装依赖
npm install

# 开发模式
npm run dev

# 生产构建
npm run build
```

---

## 📚 文档

### 开发文档（以此为准）

**当前开发的需求和设计文档位于 [`docs/specs/`](./docs/specs/)**：

| 文档 | 描述 | 状态 |
|------|------|------|
| [需求文档](./docs/specs/requirements.md) | MVP 核心功能需求 | ✅ 已完成 |
| [设计文档](./docs/specs/design.md) | 数据库 Schema + API 设计 | 🚧 进行中 |
| [任务拆分](./docs/specs/tasks.md) | 开发任务列表 | ⏳ 待开始 |

### 设计草稿（已归档）

**以下文档是设计阶段的草稿，包含完整的 Phase 1/2/3 规划，仅供参考**：

| 文档 | 描述 |
|------|------|
| [项目总览](./docs/00-summary.md) | 设计阶段的项目概况（归档） |
| [需求设计](./docs/01-requirements.md) | 完整需求规划（归档） |
| [架构设计](./docs/02-architecture.md) | 系统架构设计（归档） |
| [技术选型](./docs/tech-stack-decision.md) | 技术栈决策记录（归档） |

---

## 🛠️ 技术栈

### 全栈框架
- **框架:** Next.js 15 (App Router)
- **语言:** TypeScript 5.x
- **ORM:** Prisma
- **数据库:** PostgreSQL 16.x
- **认证:** NextAuth.js v5 或自实现 JWT

### UI 层
- **组件库:** shadcn/ui + Radix UI
- **样式:** Tailwind CSS
- **图标:** Lucide Icons
- **状态管理:** React Query + Zustand (可选)

### 工具库
- **验证:** Zod
- **邮件:** Nodemailer
- **定时任务:** node-cron
- **日期处理:** date-fns

### 部署
- **推荐:** Vercel (一键部署)
- **自建:** Docker (单容器)
- **反向代理:** Nginx / Caddy (可选)

---

## 📋 功能路线图

### Phase 1: MVP ✅ (设计中)

- [x] 项目架构设计
- [x] 技术选型确定
- [ ] 数据库 Schema 设计
- [ ] API 接口设计
- [ ] 核心功能开发
  - [ ] 管理员认证
  - [ ] 用户管理 CRUD
  - [ ] 邀请码系统
  - [ ] 账号到期管理
  - [ ] 邮件通知

### Phase 2: 增强功能

- [ ] MoviePilot 完整集成
- [ ] 批量操作
- [ ] 设备管理
- [ ] 客户端过滤
- [ ] 操作审计
- [ ] Webhook 支持

### Phase 3: 可选功能

- [ ] 引荐系统
- [ ] Telegram 通知
- [ ] 多管理员
- [ ] UI 定制

---

## 🏗️ 项目结构

```
ember/
├── app/                  # Next.js App Router
│   ├── (auth)/          # 认证页面组
│   │   ├── login/
│   │   └── register/
│   ├── (admin)/         # 管理后台
│   │   ├── dashboard/
│   │   ├── users/
│   │   ├── invites/
│   │   └── settings/
│   ├── (user)/          # 用户面板
│   │   ├── profile/
│   │   └── devices/
│   ├── actions/         # Server Actions
│   └── api/             # API Routes (可选)
│
├── components/          # UI 组件
│   ├── ui/             # shadcn/ui 组件
│   ├── admin/          # 管理后台组件
│   └── user/           # 用户面板组件
│
├── lib/                 # 工具库
│   ├── db.ts           # Prisma 客户端
│   ├── auth.ts         # 认证工具
│   ├── emby.ts         # Emby API 客户端
│   └── email.ts        # 邮件服务
│
├── prisma/             # Prisma 配置
│   ├── schema.prisma   # 数据库 Schema
│   └── migrations/     # 迁移文件
│
├── types/              # TypeScript 类型
├── docs/               # 项目文档
├── Dockerfile          # Docker 构建
└── docker-compose.yml  # 部署配置
```

---

## ⚙️ 配置

### 环境变量

```bash
# 数据库
DATABASE_URL=postgres://user:pass@localhost:5432/ember

# NextAuth.js (如使用)
NEXTAUTH_SECRET=your-secret-key
NEXTAUTH_URL=http://localhost:3000

# 或自实现 JWT
JWT_SECRET=your-secret-key

# Emby
EMBY_URL=https://your-emby.com
EMBY_API_KEY=your-api-key

# SMTP
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=your-email@gmail.com
SMTP_PASS=your-password

# MoviePilot (可选)
MOVIEPILOT_URL=https://your-moviepilot.com
MOVIEPILOT_TOKEN=your-token
```

---

## 🤝 贡献

目前项目处于 **设计阶段**，欢迎：

- 💡 功能建议和需求讨论
- 🐛 Bug 报告
- 📖 文档改进
- 🎨 UI/UX 反馈

### 开发流程

```bash
# 1. Fork 项目
# 2. 创建功能分支
git checkout -b feature/amazing-feature

# 3. 提交更改
git commit -m 'Add amazing feature'

# 4. 推送到分支
git push origin feature/amazing-feature

# 5. 创建 Pull Request
```

---

## 📄 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情

---

## 🙏 致谢

- [Wizarr](https://github.com/wizarrrr/wizarr) - UI 设计灵感
- [jfa-go](https://github.com/hrfee/jfa-go) - 功能参考
- [shadcn/ui](https://ui.shadcn.com/) - UI 组件库

---

## 📞 联系方式

- **作者:** Kong Hang
- **项目主页:** https://github.com/yourusername/ember
- **问题反馈:** https://github.com/yourusername/ember/issues

---

<div align="center">

**[文档](./docs/)** • **[问题](../../issues)** • **[讨论](../../discussions)**

Made with ❤️ by Kong Hang

</div>
