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
   - `POSTGRES_USER`
   - `POSTGRES_PASSWORD`
   - `DATABASE_URL`
   - `JWT_SECRET`
   - `CONFIG_ENCRYPTION_KEY`
   - `EMBY_URL`
   - `EMBY_API_KEY`
   - `INTERNAL_API_SECRET`
    - `TELEGRAM_BOT_TOKEN`
    - `TELEGRAM_WEBHOOK_SECRET`
    - `WEBHOOK_URL`
   - `EMBER_API_IMAGE`
   - `EMBER_WEB_IMAGE`
   - `EMBER_BOT_IMAGE`

   说明：
   - `ADMIN_PASSWORD` 不再是 compose 解析期必填；不填时 API 会在首启时生成临时管理员口令并要求首次登录改密。
   - `EMBY_URL` / `EMBY_API_KEY` 等媒体能力配置已托管到设置中心，若不准备启用相关能力，可以在首启后再补。

3. 决定数据库迁移策略。
   - 空数据库首次启动：可直接用当前 compose，PostgreSQL 会执行挂载到 `/docker-entrypoint-initdb.d` 的 `infrastructure/docker/initdb/` 子目录；该目录当前仅有 v1.4.0 截点合并 baseline。
   - 已有数据库升级：当前顶层无独立增量；如未来新增增量，按 [`infrastructure/database/README.md`](../../infrastructure/database/README.md) 手动执行后再启动服务。
   - 如果当前数据库版本停留在 `v1.3.1` 对应阶段（理论场景，v1.4.0 已上线），可参考 `infrastructure/database/archive/pre-20260502/` 内的 24 个原始文件按字典序执行。

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

这是默认模式，也是当前推荐路径。`docker-compose.yml` 现在要求通过环境变量显式指定固定镜像：

- `EMBER_API_IMAGE`
- `EMBER_WEB_IMAGE`
- `EMBER_BOT_IMAGE`

推荐写法示例：

```env
EMBER_API_IMAGE=ghcr.io/konghanghang/ember-api:v2026.04.30
EMBER_WEB_IMAGE=ghcr.io/konghanghang/ember-web:v2026.04.30
EMBER_BOT_IMAGE=ghcr.io/konghanghang/ember-bot:v2026.04.30
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

- `postgres`、`ember-api`、`ember-web`、`ember-bot` 均为 `Up`
- `GET http://localhost:8080/health` 返回 200
- `GET http://localhost:8000/health` 返回 200
- `http://localhost` 可打开前端页面
- API 日志中没有持续刷屏的数据库连接错误
- 若首次环境没有现成 admin，日志中能看到“默认管理员已创建”或“已生成临时口令并要求首次改密”的提示

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
