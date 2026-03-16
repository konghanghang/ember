# Ember

Ember 是一个面向 Emby 的用户管理系统，采用 Monorepo 组织 API、Web 和 Telegram Bot，覆盖注册登录、账号生命周期、兑换码、支付、求片订阅、播放排行和 Bot 通知等能力。

[![Test](https://github.com/konghanghang/ember/actions/workflows/test.yml/badge.svg)](https://github.com/konghanghang/ember/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.23-blue.svg)](https://go.dev/)
[![Vue](https://img.shields.io/badge/vue-3.x-green.svg)](https://vuejs.org/)
[![Python](https://img.shields.io/badge/python-3.11-blue.svg)](https://python.org/)

## 文档入口

- [文档中心](./docs/README.md) - 统一导航，按参考资料、操作手册、方案草稿、归档分类
- [系统架构](./docs/system-architecture.md) - 当前系统的核心真相来源
- [开发指南](./docs/reference/development-guide.md) - 开发时的最短阅读路径
- [API 服务文档](./services/api/README.md)
- [Web 服务文档](./services/web/README.md)
- [Bot 服务文档](./services/bot/README.md)

## 技术栈

- 后端：Go 1.23 + Gin + GORM + PostgreSQL
- 前端：Vue 3 + TypeScript + Element Plus + Tailwind CSS
- Bot：Python 3.11 + python-telegram-bot + FastAPI
- 基础设施：Docker + Docker Compose + Nginx

## 仓库分区

- `services/`：API、Web、Bot
- `infrastructure/`：Docker、数据库、Nginx 等部署资源
- `docs/`：唯一文档中心

## 验证方式

- 后端编译：`go build ./...`
- 前端构建：`npm run build`
- 具体环境准备、部署和测试步骤见 [文档中心](./docs/README.md)
