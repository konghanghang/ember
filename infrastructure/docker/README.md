# Docker 目录说明

本目录只说明这里放了什么，不重复维护一份完整部署手册。

## 目录内容

- [`docker-compose.yml`](./docker-compose.yml) - 标准 Compose 部署文件
- [`.env.example`](./.env.example) - Compose 环境变量模板
- `initdb/` - PostgreSQL 首启 SQL 子目录（compose 挂载到 `/docker-entrypoint-initdb.d/`，仅 `.sql` 文件）

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
# 按 .env 注释填入密钥（POSTGRES_PASSWORD / JWT_SECRET / CONFIG_ENCRYPTION_KEY / INTERNAL_API_SECRET）
docker compose pull
docker compose up -d
```

如需启用 Bot：在 `.env` 填入 Telegram 相关变量，然后：

```bash
docker compose --profile bot up -d
```

## 说明

- compose 默认启动 `postgres`、`ember-api`、`ember-web`；`ember-bot` 通过 `profiles: ["bot"]` 控制，默认不启动
- `EMBER_API_IMAGE` / `EMBER_WEB_IMAGE` / `EMBER_BOT_IMAGE` 在 compose 中已钉版默认值，每次发版会同步更新；生产环境建议在 `.env` 显式覆盖以避免依赖默认值漂移
- `DATABASE_URL` 默认由 compose 按 `POSTGRES_USER/PASSWORD/DB` 自动拼接到内置 postgres；指向独立 DB 时在 `.env` 显式提供完整 DSN 即可覆盖
- 这个目录的路径和文件名属于部署入口的一部分，改动前先同步更新 runbooks

## `initdb/` 子目录

PostgreSQL 容器首次启动（`postgres_data` 卷为空）时会按字典序执行 `/docker-entrypoint-initdb.d/` 下的所有 `.sql` / `.sh` / `.sql.gz` 文件。compose 把本目录下的 `initdb/` 挂载到该路径，里面**只允许放需要在空库初始化时执行的 SQL**：

- 当前包含 `infrastructure/database/` 顶层 baseline + 顶层增量 SQL
- 不允许放 README、`.md`、`.sh`、子目录等任何非 SQL 文件，否则 PG 启动日志会 warn 或被误执行
- `infrastructure/database/` 仍是 SQL migration 真相目录，本目录是首启专用副本

### 新增 / 同步迁移

每次在 `infrastructure/database/` 新增顶层 SQL，必须**同步复制**一份到 `infrastructure/docker/initdb/`，文件名保持完全一致：

```bash
cp infrastructure/database/<NEW_SQL>.sql infrastructure/docker/initdb/
```

被 baseline 吸收并归档到 `archive/` 的旧文件，**也要从本目录删除**，避免空库初始化时重复执行。

> 这些文件**仅影响首次空库初始化**。已存在的 `postgres_data` 卷不会再执行本目录下的 SQL；schema 升级请按 `infrastructure/database/README.md` 流程手工执行。
