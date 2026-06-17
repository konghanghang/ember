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

2. 按 [部署环境与配置](./deployment-environment.md) 补齐必填项：
   - `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB`
   - `JWT_SECRET` / `CONFIG_ENCRYPTION_KEY` / `INTERNAL_API_SECRET`

   不再硬性要求（compose 已就位默认或自动拼接）：

   - `DATABASE_URL`：缺省由 compose 按 `POSTGRES_USER/PASSWORD/DB` 自动拼接到内置 postgres；指向独立 DB 时显式提供
   - `EMBER_API_IMAGE` / `EMBER_WEB_IMAGE` / `EMBER_BOT_IMAGE`：compose 中已钉版默认值，随每次发版同步更新
   - `ADMIN_PASSWORD`：未填时 API 首启会生成临时管理员口令并要求首次登录改密
   - `EMBY_URL` / `EMBY_API_KEY` 等媒体能力配置已托管到设置中心，可在首启后补
   - 启用 Bot 时再填：`TELEGRAM_BOT_TOKEN` / `TELEGRAM_WEBHOOK_SECRET` / `WEBHOOK_URL`

3. 数据库迁移由 `ember-api` 启动期内嵌自动应用，部署者不再需要任何手工 SQL。
   - 空数据库首次启动：进入"新空库"分支，按字典序 forward-only 跑全部 `infrastructure/database/` 顶层 SQL 完成初始化
   - 已有数据库升级：当前直接升级支持起点是 `2026-06-05` / v1.6.0 截点；支持窗口内直接 `docker compose pull && up -d`，启动期 Migrate 自动按 forward-only 应用未应用的顶层 SQL。旧于该截点且未执行过已归档增量的数据库，需先人工对齐或先升到中间版本。详见 [`infrastructure/database/README.md`](../../infrastructure/database/README.md) 的"自动迁移与 schema_migrations"章节。

4. 拉取镜像并启动。

不启用 Bot（默认）：

```bash
docker compose pull
docker compose up -d
```

启用 Bot：

```bash
docker compose pull
docker compose --profile bot up -d
```

5. 做最小验证。

```bash
docker compose ps
curl http://localhost:8080/health
# 启用 Bot 时再加：curl http://localhost:8000/health
```

6. 打开浏览器访问 `http://localhost`，确认 Web 首页可用。

## 部署模式

### 模式 A：预构建镜像

这是默认模式，也是当前推荐路径。`docker-compose.yml` 中 `EMBER_API_IMAGE` / `EMBER_WEB_IMAGE` / `EMBER_BOT_IMAGE` 已钉版默认值（随每次发版同步更新），开箱可拉起。

生产环境建议在 `.env` 中显式覆盖避免依赖默认值漂移：

```env
EMBER_API_IMAGE=ghcr.io/konghanghang/ember-api:v1.6.1
EMBER_WEB_IMAGE=ghcr.io/konghanghang/ember-web:v1.6.1
EMBER_BOT_IMAGE=ghcr.io/konghanghang/ember-bot:v1.6.1
```

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

- `postgres`、`ember-api`、`ember-web` 均为 `Up`
- `GET http://localhost:8080/health` 返回 200
- `http://localhost` 可打开前端页面
- API 日志中没有持续刷屏的数据库连接错误
- 若首次环境没有现成 admin，日志中能看到“默认管理员已创建”或“已生成临时口令并要求首次改密”的提示
- 启用 Bot 时（`docker compose --profile bot up -d`）：`ember-bot` 也为 `Up`，`GET http://localhost:8000/health` 返回 200

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
