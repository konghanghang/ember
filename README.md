# Ember

Ember 是一个面向 Emby 的用户管理系统，采用 Monorepo 管理 API、Web 和 Telegram Bot，覆盖注册登录、账号生命周期、兑换码、支付、求片订阅、播放排行与 Bot 通知等完整链路。

[![Test](https://github.com/konghanghang/ember/actions/workflows/test.yml/badge.svg)](https://github.com/konghanghang/ember/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.23-blue.svg)](https://go.dev/)
[![Vue](https://img.shields.io/badge/vue-3.x-green.svg)](https://vuejs.org/)
[![Python](https://img.shields.io/badge/python-3.11-blue.svg)](https://python.org/)

测试覆盖率请查看 GitHub Actions 中的 [Test workflow](https://github.com/konghanghang/ember/actions/workflows/test.yml) 运行结果。

## 快速导航

- 第一次了解项目：先看 [系统架构](./docs/system-architecture.md)
- 准备开始开发：先看 [开发指南](./docs/reference/development-guide.md)
- 准备部署或本地拉起完整环境：看 [部署指南](./docs/runbooks/deployment.md)
- 查完整文档入口：看 [文档中心](./docs/README.md)
- 只关心某个服务：看 [API 服务文档](./services/api/README.md)、[Web 服务文档](./services/web/README.md)、[Bot 服务文档](./services/bot/README.md)

## 适用对象

这个仓库主要适合以下几类场景作为统一入口：

- API、Web、Bot 的日常开发与联调
- Emby、TMDB、MoviePilot、Stripe、Telegram 集成排查
- Docker Compose 部署、环境变量配置与发布前验证
- 业务流程、账号状态流转和系统边界梳理

## 核心能力

- 用户注册与登录：支持开放注册或兑换码注册，支持邮箱验证码
- Emby 账号生命周期管理：试用、续期、过期封禁、管理员手动启停
- 兑换码系统：同时用于注册门控与续期
- 付费方案与 Stripe 一次性支付
- 求片订阅：TMDB 搜索、管理员审批、MoviePilot 自动下载
- 播放排行榜与用户画像能力
- Telegram Bot：通知、审批、账号绑定、续期与查询

## 快速开始

### 开发模式

1. 阅读 [开发指南](./docs/reference/development-guide.md)，按最短阅读路径建立项目认知。
2. 按改动范围进入对应服务文档：`services/api`、`services/web`、`services/bot`。
3. 按 [测试指南](./docs/runbooks/testing.md) 执行最小验证动作。

### Docker Compose 部署模式

最小部署：PostgreSQL + API + Web（默认不启动 Bot）。需要 Docker / Docker Compose。

1. 克隆仓库并进入 Docker 目录：

   ```bash
   git clone https://github.com/konghanghang/ember.git
   cd ember/infrastructure/docker
   cp .env.example .env
   ```

2. 在 Linux / macOS / WSL / Git Bash 下生成必填密钥（输出粘到 `.env` 对应行）：

   ```bash
   echo "POSTGRES_PASSWORD=$(openssl rand -hex 16)"
   echo "JWT_SECRET=$(openssl rand -hex 32)"
   echo "CONFIG_ENCRYPTION_KEY=$(openssl rand -hex 32)"
   echo "INTERNAL_API_SECRET=$(openssl rand -hex 32)"
   ```

3. 拉镜像并启动：

   ```bash
   docker compose pull
   docker compose up -d
   ```

4. 首次登录：

   ```bash
   docker compose logs ember-api | grep "临时口令"
   ```

   浏览器打开 `http://localhost`，使用 `admin` + 日志中的临时口令登录，按提示完成首次改密。

5. （可选）启用 Telegram Bot：在 `.env` 补齐 `TELEGRAM_BOT_TOKEN` / `TELEGRAM_WEBHOOK_SECRET`（生成：`openssl rand -hex 32`）/ `WEBHOOK_URL`，然后 `docker compose --profile bot up -d`。

升级到新版本：

```bash
docker compose pull
docker compose up -d
```

`ember-api` 启动期会内嵌自动迁移已应用之外的 SQL，无需手工操作。日志带 `[Migrate]` 前缀。

更细的配置与排障：[部署指南](./docs/runbooks/deployment.md) / [部署环境与配置](./docs/runbooks/deployment-environment.md) / [部署排障](./docs/runbooks/deployment-troubleshooting.md)。

## 技术栈

- 后端：Go 1.23 + Gin + GORM + PostgreSQL 15
- 前端：Vue 3 + TypeScript + Element Plus + Tailwind CSS
- Bot：Python 3.11 + python-telegram-bot + FastAPI
- 基础设施：Docker + Docker Compose + Nginx

## 仓库地图

- `services/api/`：Go API 服务，负责用户、账号、支付、兑换码、订阅、排行等核心业务接口
- `services/web/`：Vue 管理后台与用户控制台
- `services/bot/`：Telegram Bot 服务，负责通知、审批和账号相关交互
- `infrastructure/`：数据库、Docker Compose、Nginx 与部署资源
- `docs/`：唯一文档中心，包含架构、参考资料、操作手册、方案与归档

## 本地开发与验证

- 环境准备与阅读顺序：见 [开发指南](./docs/reference/development-guide.md)
- API 最小验证：`cd services/api && go vet ./... && go test ./... && go build ./...`
- Web 最小验证：`cd services/web && npm ci && npm run build`
- Bot 最小验证：`cd services/bot && pip install -r requirements.txt && python -m py_compile main.py`
- 需要手工回归的场景：见 [测试指南](./docs/runbooks/testing.md)

## 开发协作

- 开发入口与阅读顺序：见 [开发指南](./docs/reference/development-guide.md)
- 项目治理与目录边界：见 [项目治理经验](./docs/reference/project-governance-guide.md)
- API 约束：见 [API 开发与目录规范](./docs/reference/api-development-conventions.md) 与 [API 响应规范](./docs/reference/api-response-standard.md)
- 前端页面与视觉规范：见 [Web 设计规范](./docs/reference/web-design-guide.md)
- 多 Agent 协作：见 [多 Agent 协作指南](./docs/reference/multi-agent-collaboration-guide.md)

## 文档入口

- [系统架构](./docs/system-architecture.md) - 当前系统的核心真相来源
- [开发指南](./docs/reference/development-guide.md) - 开发时的最短阅读路径
- [文档中心](./docs/README.md) - 统一导航，按参考资料、操作手册、方案草稿、归档分类
- [部署指南](./docs/runbooks/deployment.md)
- [测试指南](./docs/runbooks/testing.md)
- [API 服务文档](./services/api/README.md)
- [Web 服务文档](./services/web/README.md)
- [Bot 服务文档](./services/bot/README.md)

## License

本项目基于 [Apache License 2.0](./LICENSE) 开源。
