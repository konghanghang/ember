# 部署排障

这份文档只保留部署期最常见的检查动作和恢复动作。

## 先做最小检查

```bash
cd infrastructure/docker
docker compose ps
docker compose logs --tail=100 ember-api
docker compose logs --tail=100 ember-bot
curl http://localhost:8080/health
curl http://localhost:8000/health
```

如果这几步都看不懂，就别急着改配置，先把报错抄清楚。

## 常见问题

### 1. `postgres` 不健康，API 起不来

检查：

- `docker compose ps` 中 `postgres` 是否为 `healthy`
- `DATABASE_URL` 是否真的指向当前 PostgreSQL
- 宿主机或数据库侧的认证是否允许当前连接

辅助命令：

```bash
docker compose logs postgres
docker compose exec postgres psql -U postgres -d ember -c '\dt'
```

### 2. API 健康检查失败

检查：

- `ember-api` 日志是否有数据库连接错误
- `JWT_SECRET`、`CONFIG_ENCRYPTION_KEY` 是否为空
- `EMBY_URL`、`EMBY_API_KEY` 是否写成了占位值

辅助命令：

```bash
docker compose logs --tail=200 ember-api
curl http://localhost:8080/health
```

### 3. 管理员没有自动创建

优先看 API 日志里是否出现：

- “跳过 admin 初始化”
- `ADMIN_USERNAME` / `ADMIN_PASSWORD` 缺失

结论很直接：

- `ADMIN_PASSWORD` 没配：不会创建管理员
- 数据库里已有 admin：不会重复创建

### 4. Web 能打开，但页面请求 API 失败

检查：

- `ember-api` 是否真的已启动
- 前端容器日志是否有静态资源或代理配置异常

辅助命令：

```bash
docker compose logs --tail=100 ember-web
curl http://localhost:8080/health
```

### 5. Bot 401 或收不到 Telegram 回调

检查：

- `TELEGRAM_WEBHOOK_SECRET` 是否与 Telegram 侧一致
- `WEBHOOK_URL` 是否是公网可访问地址
- `INTERNAL_API_SECRET` 是否与 API 侧完全一致

辅助命令：

```bash
docker compose logs --tail=200 ember-bot
curl http://localhost:8000/health
```

本地联调不要在这里反复试，直接去 [Cloudflared 本地联调](./cloudflared-local-testing.md)。

### 6. 某些功能页面能打开，但能力不可用

这通常不是“服务没起”，而是功能配置没补齐。

典型场景：

- TMDB 搜索 / 追剧日历：缺 `TMDB_API_KEY`
- MoviePilot 同步：缺 `MOVIEPILOT_*`
- Telegram 通知：缺 `TELEGRAM_*` 或 `WEBHOOK_URL`

先对照 [部署环境与配置](./deployment-environment.md) 和 [配置参考](../reference/configuration-reference.md) 补齐再说。

## 恢复动作

### 只重启单个服务

```bash
docker compose restart ember-api
docker compose restart ember-bot
docker compose restart ember-web
```

### 全量重启

```bash
docker compose down
docker compose up -d
```

### 拉取最新镜像后重启

```bash
docker compose pull
docker compose up -d
```

### 本地构建镜像后重启

```bash
docker compose build
docker compose up -d
```

### 删除数据卷重建

```bash
docker compose down -v
docker compose up -d
```

这一步会删 PostgreSQL 数据。没备份就别装勇敢。

## 备份与恢复

### 备份

```bash
docker compose exec postgres pg_dump -U postgres ember > backup.sql
```

### 恢复

```bash
cat backup.sql | docker compose exec -T postgres psql -U postgres ember
```

## 何时停止排障，直接修配置

出现下面任一情况，就别继续“看日志碰运气”了：

- `.env` 里仍有 `your-...` 这类占位值
- 依赖的外部地址不可达
- 生产升级环境却没执行 SQL 迁移
- Webhook 地址是内网地址，却想接公网回调

## 相关文档

- [部署指南](./deployment.md)
- [部署环境与配置](./deployment-environment.md)
- [数据库 Migration Baseline](./database-migration-baseline.md)
- [Cloudflared 本地联调](./cloudflared-local-testing.md)
- [数据库迁移说明](../../infrastructure/database/README.md)
