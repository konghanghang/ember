# 🔥 Ember

> A modern user management system for Emby media server

**Ember** 是一个现代化的 Emby 用户管理系统，采用 **Monorepo 微服务架构**，提供邀请码注册、用户管理、账号到期控制、MoviePilot 集成、Telegram Bot 等功能。

[![Test](https://github.com/konghanghang/ember/actions/workflows/test.yml/badge.svg)](https://github.com/konghanghang/ember/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.23-blue.svg)](https://go.dev/)
[![Vue](https://img.shields.io/badge/vue-3.x-green.svg)](https://vuejs.org/)
[![Python](https://img.shields.io/badge/python-3.11-blue.svg)](https://python.org/)

---

## 🏗️ 项目架构

本项目采用 **Monorepo 微服务架构**，将所有服务集中管理，便于代码共享和统一部署。

### 目录结构

```
ember/
├── services/                     # 微服务目录
│   ├── api/                     # Go API 服务
│   │   ├── cmd/                 # 命令入口
│   │   ├── internal/            # 内部包
│   │   ├── configs/             # 配置文件
│   │   └── Dockerfile
│   ├── web/                     # Vue 3 前端（🚧 开发中）
│   │   ├── src/                 # 源代码
│   │   ├── components/          # 组件
│   │   └── Dockerfile
│   └── bot/                     # Python Telegram Bot（🚧 待实现）
│       ├── handlers/            # 命令处理器
│       ├── services/            # 业务逻辑
│       └── Dockerfile
├── infrastructure/              # 基础设施配置
│   ├── docker/                 # Docker 相关
│   │   └── docker-compose.yml
│   ├── nginx/                  # Nginx 配置
│   └── database/               # 数据库脚本
├── docs/                       # 项目文档
│   ├── MIGRATION-GUIDE.md      # 迁移指南
│   └── specs/                  # 规范文档
├── Makefile                    # 统一命令入口
└── README.md                   # 本文档
```

### 技术栈

- **后端**: Go 1.23 + Gin + GORM + PostgreSQL
- **前端**: Vue 3 + Vite + TypeScript + Element Plus（🚧 开发中）
- **Bot**: Python 3.11 + python-telegram-bot（🚧 待实现）
- **基础设施**: Docker + Docker Compose + Nginx

### 架构优势

- **统一管理** - 所有服务在同一仓库，版本同步，易于维护
- **服务分离** - 各服务独立部署和扩展
- **简化部署** - 一键部署所有服务，环境配置统一
- **开发友好** - 统一的开发环境和工具链

---

## ✨ 核心功能

### Phase 1: 核心用户管理（✅ 已完成）

- 🎫 **邀请码系统** - 随机生成邀请码，设置使用次数和到期时间
- 👥 **用户管理** - 用户列表、搜索、延长到期、禁用/删除
- ⏰ **账号到期** - 定时任务自动禁用过期账号
- 🔐 **管理员认证** - JWT 认证，Token 7 天有效

### Phase 2: 用户面板与订阅系统（✅ 已完成）

- 👤 **用户认证与面板** - 用户登录、个人仪表盘、修改密码和邮箱
- 📺 **订阅管理** - 提交订阅请求、管理员审核、状态跟踪
- 🎬 **MoviePilot 集成** - 自动调用 API 创建订阅、同步失败提示

### Phase 3: 架构重构（🚧 进行中）

- ✅ **Go 后端** - Gin + GORM 实现，健康检查 API 完成
- 🚧 **Vue 3 前端** - 重写管理后台和用户面板（开发中）
- 🚧 **Telegram Bot** - 用户注册、信息查询、订阅管理（待实现）

---

## 🚀 快速开始

### 环境要求

- Go 1.23+
- Node.js 18+（前端开发）
- Python 3.11+（Bot 开发）
- PostgreSQL 14+
- Docker + Docker Compose（推荐）

### 1. 克隆项目

```bash
git clone https://github.com/konghanghang/ember.git
cd ember
```

### 2. 初始化项目

```bash
# 使用 Makefile 初始化
make init

# 或手动创建配置文件
cp .env.example .env
cp services/api/.env.example services/api/.env
# 编辑 .env 文件填入必要配置
```

### 3. 启动服务

#### 使用 Docker 镜像（推荐）

```bash
# 进入 docker 目录
cd infrastructure/docker

# 配置环境变量
cp .env.example .env
# 编辑 .env 文件填入实际配置

# 拉取镜像并启动
docker-compose pull
docker-compose up -d

# 查看服务状态
docker-compose ps
docker-compose logs -f
```

#### 使用源码构建

```bash
# 构建并启动所有服务
make docker-up

# 或直接使用 docker-compose
docker-compose -f infrastructure/docker/docker-compose.yml up -d
```

#### 开发模式

```bash
# 启动数据库
docker-compose -f infrastructure/docker/docker-compose.yml up -d postgres

# 启动 Go API 服务
make dev-api
# 或
cd services/api && go run cmd/server/main.go

# 启动 Vue 前端（待实现）
make dev-web

# 启动 Python Bot（待实现）
make dev-bot
```

### 4. 访问服务

- **API 服务**: http://localhost:8080
- **健康检查**: http://localhost:8080/health
- **Web 前端**: http://localhost:3000（待实现）

---

## 🔧 开发工具

### 使用 Makefile

```bash
make help          # 显示所有可用命令
make init          # 初始化项目
make setup         # 安装依赖
make build         # 构建所有服务
make dev-api       # 启动 API（开发模式）
make test-api      # 运行测试
make docker-up     # 启动 Docker 服务
make docker-down   # 停止 Docker 服务
make clean         # 清理项目
```

### 构建服务

```bash
# 构建 Go API
make build-api

# 构建所有服务
make build
```

### 运行测试

```bash
# 测试 Go API
make test-api

# 测试健康检查
make test-health
```

---

## 📚 文档

### 核心文档

- [迁移指南](./docs/MIGRATION-GUIDE.md) - Next.js → Vue 3 + Go + Python 完整迁移方案
- [开发规范](./CLAUDE.md) - AI 协作指南和开发规范
- [Docker 构建指南](./docs/DOCKER-BUILD-GUIDE.md) - Docker 镜像构建和发布流程
- [API 文档](./services/api/README.md) - Go API 服务文档

### 服务文档

- [API 服务](./services/api/README.md) - Go 后端架构和设计决策
- [Web 前端](./services/web/README.md) - Vue 3 前端开发指南
- [Telegram Bot](./services/bot/README.md) - Python Bot 功能设计

---

## 📈 项目状态

### ✅ 已完成

- [x] Go 后端基础框架（Gin + GORM）
- [x] GORM 数据模型（保留业务语义设计）
- [x] 数据库连接层（GORM ORM）
- [x] 健康检查 API
- [x] Monorepo 架构搭建
- [x] Docker + Docker Compose 配置
- [x] GitHub Actions CI/CD 流程
- [x] 多服务 Docker 镜像构建
- [x] Makefile 统一命令

### 🚧 进行中

- [ ] Go API 完整实现（登录、用户管理、邀请码等）
- [ ] Vue 3 前端开发
- [ ] JWT 中间件
- [ ] API 文档生成

### ⏳ 待开始

- [ ] Python Telegram Bot
- [ ] 数据迁移脚本
- [ ] 监控和日志（Prometheus + Grafana）
- [ ] 完整测试覆盖

---

## 🤝 贡献指南

### 开发流程

1. Fork 项目
2. 创建功能分支 (`git checkout -b feature/新功能`)
3. 提交更改 (`git commit -m 'feat: 添加新功能'`)
4. 推送分支 (`git push origin feature/新功能`)
5. 创建 Pull Request

### Git 提交规范

- `feat:` 新功能
- `fix:` 修复问题
- `docs:` 文档更新
- `refactor:` 代码重构
- `test:` 测试相关
- `chore:` 构建/工具变动

---

## 📋 最近更新

### v3.0.0 (2026-02-10) - 架构重构

- ✨ **Monorepo 架构** - 采用微服务 Monorepo 架构
- ✨ **Go 后端** - 使用 Go + Gin + GORM 重写后端
- ✨ **项目结构** - services/ 目录管理所有微服务
- ✨ **基础设施** - infrastructure/ 集中管理 Docker 配置
- ✨ **Makefile** - 统一命令入口，简化开发流程
- 📚 **文档完善** - 迁移指南、服务文档

### v2.0.0 (2024-12) - MVP 完成

- ✅ 核心用户管理功能
- ✅ 订阅系统和 MoviePilot 集成
- ✅ Next.js 15 全栈实现

[查看完整更新日志](./CHANGELOG.md)

---

## 📞 问题反馈

如有问题或建议，请通过项目 Issues 提交反馈

---

## ⚖️ 免责声明

本项目仅供学习交流使用。我们不存储任何影视文件，只提供用户管理服务。请支持正版！

---

<div align="center">

**[文档](./docs/)** • **[问题](../../issues)** • **[讨论](../../discussions)**

Made with ❤️ by Kong Hang

*架构参考: [NextNewEP](https://github.com/konghanghang/nextnewep)*

</div>
