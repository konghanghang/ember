# 🔥 Ember

> A modern user management system for Emby media server

**Ember** 是一个基于 **Go + Next.js** 的现代化 Emby 用户管理系统，提供邀请码注册、用户管理、账号到期控制、MoviePilot 集成等功能。

[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8.svg)](https://go.dev/)
[![Next.js](https://img.shields.io/badge/next.js-15-black.svg)](https://nextjs.org/)

---

## ✨ 特性

- 🎫 **邀请码系统** - 灵活的邀请码生成和管理
- 👥 **用户管理** - 完整的用户 CRUD 和批量操作
- ⏰ **账号到期** - 自动禁用和延迟删除机制
- 🎬 **MoviePilot 集成** - 完整的搜索、订阅、状态查询
- 🔐 **JWT 认证** - 现代化的 Token 认证体系
- 📧 **邮件通知** - 自动发送欢迎和到期提醒
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
cd backend && go mod download
cd ../frontend && npm install

# 开发模式
make dev

# 生产构建
make build
```

---

## 📚 文档

完整的设计文档位于 [`docs/`](./docs/) 目录：

| 文档 | 描述 |
|------|------|
| [项目总览](./docs/00-summary.md) | 快速了解项目概况 |
| [需求设计](./docs/01-requirements.md) | 详细功能需求和决策记录 |
| [架构设计](./docs/02-architecture.md) | 系统架构和技术细节 |
| [数据库设计](./docs/03-database.md) | 数据库 Schema（待完成） |
| [API 文档](./docs/04-api.md) | API 接口规范（待完成） |

---

## 🛠️ 技术栈

### 后端
- **语言:** Go 1.22+
- **框架:** Gin
- **ORM:** GORM
- **数据库:** PostgreSQL
- **认证:** JWT

### 前端
- **框架:** Next.js 15 (App Router)
- **UI:** shadcn/ui + Tailwind CSS
- **语言:** TypeScript
- **状态管理:** React Query + Zustand

### 部署
- **容器化:** Docker (单镜像)
- **编排:** Docker Compose
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
├── backend/              # Go 后端
│   ├── cmd/             # 入口
│   ├── internal/        # 核心逻辑
│   │   ├── api/        # HTTP 处理
│   │   ├── service/    # 业务逻辑
│   │   ├── repository/ # 数据访问
│   │   └── client/     # 外部客户端
│   └── pkg/            # 公共库
│
├── frontend/            # Next.js 前端
│   ├── app/            # 页面路由
│   ├── components/     # UI 组件
│   └── lib/            # 工具函数
│
├── docs/               # 项目文档
├── Dockerfile          # 单镜像构建
├── docker-compose.yml  # 部署配置
└── Makefile           # 构建脚本
```

---

## ⚙️ 配置

### 环境变量

```bash
# 数据库
DATABASE_URL=postgres://user:pass@localhost:5432/ember

# JWT
JWT_SECRET=your-secret-key
JWT_ACCESS_EXPIRY=15m
JWT_REFRESH_EXPIRY=7d

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
