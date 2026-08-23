# Docker 目录说明

本目录只说明这里放了什么，不重复维护一份完整部署手册。

## 目录内容

- [`docker-compose.yml`](./docker-compose.yml) - 标准 Compose 部署文件
- [`.env.example`](./.env.example) - Compose 环境变量模板

## 这个目录解决什么问题

- 提供 API、Web、Bot、PostgreSQL 的统一 Compose 启动入口
- 支持直接使用 GHCR 预构建镜像
- 也支持切换到本地 `build:` 方式验证未发布代码

## Playback Gateway

- Compose 服务名为 `ember-gateway`，但不新增镜像名。
- Gateway 使用 `profiles: ["gateway"]`，避免当前默认旧镜像不认识新子命令时破坏既有部署；启用时执行 `docker compose --profile gateway up -d`。
- `ember-api` 与 `ember-gateway` 复用同一 `EMBER_API_IMAGE` 和同一 `ember` 二进制，分别运行 `api` / `gateway` 子命令。
- 两个服务仍使用独立容器、健康检查和日志卷；禁止在一个容器内后台启动两个进程。
- Gateway 只映射到宿主机 `127.0.0.1:${PLAYBACK_GATEWAY_PORT:-8090}`；公网 HTTPS 由外部反向代理负责。

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
# 按 .env 注释填入密钥（POSTGRES_PASSWORD / JWT_SECRET / CONFIG_ENCRYPTION_KEY / INTERNAL_API_SECRET）
docker compose pull
docker compose up -d
```

如需启用 Bot：在 `.env` 填入 Telegram 相关变量，然后：

```bash
docker compose --profile bot up -d
```

如需启用 Playback Gateway：先确认 `EMBER_API_IMAGE` 已包含 `ember gateway`，并在设置中心配置内部 Emby 地址/API Key，然后：

```bash
docker compose --profile gateway up -d
```

## 说明

- compose 默认启动 `postgres`、`ember-api`、`ember-web`；`ember-gateway` 与 `ember-bot` 分别通过 `gateway` / `bot` profile 控制，默认不启动
- `EMBER_API_IMAGE` / `EMBER_WEB_IMAGE` / `EMBER_BOT_IMAGE` 在 compose 中已钉版默认值，每次发版会同步更新；生产环境建议在 `.env` 显式覆盖以避免依赖默认值漂移
- `DATABASE_URL` 默认由 compose 按 `POSTGRES_USER/PASSWORD/DB` 自动拼接到内置 postgres；指向独立 DB 时在 `.env` 显式提供完整 DSN 即可覆盖
- `ember-api` 与 `ember-gateway` 必须使用同一个 `EMBER_API_IMAGE`，不允许独立漂移版本
- 这个目录的路径和文件名属于部署入口的一部分，改动前先同步更新 runbooks

## 数据库初始化与升级

不再挂载 PG `initdb.d`：schema 初始化与升级**全部由 `ember-api` 启动期 Migrate 阶段**接管。

- **新空库首次部署**：业务核心表不存在 + `schema_migrations` 为空 → 进入"新空库"分支，按字典序 forward-only 跑全部 `infrastructure/database/` 顶层 SQL
- **已有数据库升级**：直接 `docker compose pull && up -d`，启动期 Migrate 自动按 forward-only 应用未应用 SQL
- 启动失败时容器进入 restart loop，`docker logs ember-api --tail` 第一时间看到失败 SQL 文件名

详见 [`infrastructure/database/README.md`](../database/README.md) 的「自动迁移与 schema_migrations」章节。
