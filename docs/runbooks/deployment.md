# 部署指南

这份文档只回答一件事：如何把 Ember 跑起来。环境变量细节、排障和发布流程已经拆出去，不再塞在一个文件里。

## 适用范围

- 使用 [`infrastructure/docker/docker-compose.yml`](../../infrastructure/docker/docker-compose.yml) 部署 API、Web、Bot 和 PostgreSQL
- 默认使用 GHCR 预构建镜像
- 适合单机或小规模环境的标准部署

## 最短路径

1. 进入 Docker 目录并准备环境变量。

```bash
cd infrastructure/docker
cp .env.example .env
```

2. 按 [部署环境与配置](./deployment-environment.md) 补齐必填项，尤其是：
   - `DATABASE_URL`
   - `JWT_SECRET`
   - `CONFIG_ENCRYPTION_KEY`
   - `ADMIN_PASSWORD`
   - `EMBY_URL`
   - `EMBY_API_KEY`
   - `INTERNAL_API_SECRET`
   - `TELEGRAM_BOT_TOKEN`
   - `TELEGRAM_WEBHOOK_SECRET`
   - `WEBHOOK_URL`

3. 决定数据库迁移策略。
   - 空数据库首次启动：可直接用当前 compose，PostgreSQL 会执行 `infrastructure/database/` 顶层 baseline SQL 和 baseline 之后的顶层增量 migration。
   - 已有数据库升级：先按 [`infrastructure/database/README.md`](../../infrastructure/database/README.md) 手动执行顶层增量 SQL，再启动服务。
   - 如果当前数据库版本停留在 `v1.2.13` 对应阶段，升级到当前版本前至少要顺序执行：
     - `infrastructure/database/20260416_01_subscription_status_and_review_fields.sql`
     - `infrastructure/database/20260418_01_media_gaps.sql`

4. 拉取镜像并启动。

```bash
docker compose pull
docker compose up -d
```

5. 做最小验证。

```bash
docker compose ps
curl http://localhost:8080/health
curl http://localhost:8000/health
```

6. 打开浏览器访问 `http://localhost`，确认 Web 首页可用。

## 部署模式

### 模式 A：预构建镜像

这是默认模式，也是当前推荐路径。`docker-compose.yml` 已经指向：

- `ghcr.io/konghanghang/ember-api:latest`
- `ghcr.io/konghanghang/ember-web:latest`
- `ghcr.io/konghanghang/ember-bot:latest`

适用场景：

- 线上部署
- 测试环境快速拉起
- 不需要本地修改 Dockerfile 的情况

### 模式 B：本地构建镜像

如果你正在验证未发布代码，可以在 `docker-compose.yml` 中：

1. 注释对应服务的 `image:`
2. 取消注释 `build:`
3. 执行本地构建

```bash
docker compose build
docker compose up -d
```

镜像构建细节见 [Docker 构建指南](./docker-build-guide.md)。

## 最小验收清单

- `postgres`、`ember-api`、`ember-web`、`ember-bot` 均为 `Up`
- `GET http://localhost:8080/health` 返回 200
- `GET http://localhost:8000/health` 返回 200
- `http://localhost` 可打开前端页面
- API 日志中没有持续刷屏的数据库连接错误
- 若开启管理员初始化，日志中没有“跳过 admin 初始化”的警告

## 不在本文展开的内容

- 环境变量、管理员初始化、迁移策略：见 [部署环境与配置](./deployment-environment.md)
- 故障排查、备份恢复、恢复动作：见 [部署排障](./deployment-troubleshooting.md)
- 镜像构建方法：见 [Docker 构建指南](./docker-build-guide.md)
- 分支、Tag、Release 发布流程：见 [发布流程](./release-process.md)

## 相关文档

- [操作手册总览](./README.md)
- [部署环境与配置](./deployment-environment.md)
- [部署排障](./deployment-troubleshooting.md)
- [Docker 目录说明](../../infrastructure/docker/README.md)
