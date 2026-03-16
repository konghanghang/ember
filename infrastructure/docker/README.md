# Docker 目录说明

本目录只说明这里放了什么，不重复维护一份完整部署手册。

## 目录内容

- [`docker-compose.yml`](./docker-compose.yml) - 标准 Compose 部署文件
- [`.env.example`](./.env.example) - Compose 环境变量模板

## 这个目录解决什么问题

- 提供 API、Web、Bot、PostgreSQL 的统一 Compose 启动入口
- 支持直接使用 GHCR 预构建镜像
- 也支持切换到本地 `build:` 方式验证未发布代码

## 你现在应该看哪份文档

- 想知道怎么部署：看 [部署指南](/Users/konghang/data/me/github/ember/docs/runbooks/deployment.md)
- 想知道变量怎么填：看 [部署环境与配置](/Users/konghang/data/me/github/ember/docs/runbooks/deployment-environment.md)
- 想排障：看 [部署排障](/Users/konghang/data/me/github/ember/docs/runbooks/deployment-troubleshooting.md)
- 想知道镜像怎么构建：看 [Docker 构建指南](/Users/konghang/data/me/github/ember/docs/runbooks/docker-build-guide.md)
- 想知道怎么发版：看 [发布流程](/Users/konghang/data/me/github/ember/docs/runbooks/release-process.md)

## 最小使用方式

```bash
cd infrastructure/docker
cp .env.example .env
docker compose pull
docker compose up -d
```

## 说明

- 当前 compose 默认会启动 `postgres`、`ember-api`、`ember-web`、`ember-bot`
- 如果你不想启动某个服务，就直接改 `docker-compose.yml`，不要指望 README 帮你兜策略
- 这个目录的路径和文件名属于部署入口的一部分，改动前先同步更新 runbooks
