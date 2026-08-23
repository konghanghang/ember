# 部署指南

这份文档只回答一件事：如何把 Ember 跑起来。环境变量细节、排障和发布流程已经拆出去，不再塞在一个文件里。

## 适用范围

- 使用 [`infrastructure/docker/docker-compose.yml`](../../infrastructure/docker/docker-compose.yml) 部署 API、Gateway、Web、Bot 和 PostgreSQL
- 默认使用 GHCR 预构建镜像
- 适合单机或小规模环境的标准部署

## Playback Gateway 部署边界

当前 Compose 已包含可选 `gateway` profile：

- 保持一个 `EMBER_API_IMAGE` 和一个 `ember` 二进制；默认子命令为 `api`。
- `ember-api` 容器使用镜像默认命令（新镜像等价于 `ember api`），`ember-gateway` 容器复用同一镜像并运行 `ember gateway`。
- 两个容器分别维护生命周期、健康检查和日志卷，不能在一个容器里同时启动两个后台进程。
- Gateway 公网入口必须代理完整 Emby 请求面；`EMBY_URL` 保持为容器可访问的原始 Emby 内网地址，`NEXT_PUBLIC_EMBY_URL` 才指向 Gateway 的公网 HTTPS 地址。
- 原始 Emby 公网入口必须关闭或限制，否则本地 Token 撤销与用户状态门控可以被绕过。
- 不新增第四个镜像、Gateway Tag 或独立发布节奏；API 与 Gateway 使用同一镜像引用。

Gateway 固定监听容器内 `8081`；Compose 只把它映射到 `127.0.0.1:${PLAYBACK_GATEWAY_PORT:-8081}`，不会自动配置公网 TLS。部署者仍需在宿主机 Nginx/Caddy/Cloudflare Tunnel 中把完整 Emby 请求面代理到该回环端口，并关闭或限制原始 Emby 公网入口。

为兼容当前默认 Tag 中尚无统一入口的旧镜像，Gateway 不随普通 `docker compose up -d` 自动启动。启用前必须使用包含 `ember gateway` 的新镜像或本地构建镜像，再显式执行 `docker compose --profile gateway up -d`。

### 外部 Nginx 示例

下面只展示 Gateway 需要的代理边界，证书路径和 TLS 策略由部署者按现有环境补齐：

```nginx
map $http_upgrade $ember_connection_upgrade {
    default upgrade;
    ''      close;
}

# Emby 客户端可能把 AccessToken 放在 query。这里只记录 $uri，禁止使用
# 会展开 query value 的 $request、$request_uri 或 $args。
log_format ember_gateway_safe '$remote_addr - $host [$time_local] '
                              '"$request_method $uri $server_protocol" '
                              '$status $body_bytes_sent $request_time';

server {
    listen 443 ssl;
    server_name emby.example.com;

    # ssl_certificate /path/to/fullchain.pem;
    # ssl_certificate_key /path/to/privkey.pem;

    access_log /var/log/nginx/ember-gateway-access.log ember_gateway_safe;

    location / {
        proxy_pass http://127.0.0.1:8081;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $ember_connection_upgrade;
        proxy_buffering off;
        proxy_request_buffering off;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }
}
```

必须代理 `/` 下的完整 Emby 请求面，不能只代理 `/Videos/`；认证、public bootstrap、PlaybackInfo、字幕和播放事件都要经过 Gateway。外部代理不应自行改写 `Range`、`Location` 或 `X-Emby-*` Header。由于 `api_key`、`AccessToken`、`X-Emby-Token` 等 query carrier 属于兼容合同，Nginx/Caddy/CDN access log 都必须隐藏 query value；不能把示例中的安全格式改回默认完整 request URI。确认新公网入口可用后，再从公网防火墙或原反向代理中移除原始 Emby 入口。

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
   - `PLAYBACK_GATEWAY_PORT`：Gateway 宿主机回环端口，默认 `8081`
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

启用 Playback Gateway（要求新镜像且已配置内部 Emby）：

```bash
docker compose --profile gateway up -d
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
# 在设置中心配置内部 EMBY_URL / EMBY_API_KEY 后：curl http://localhost:8081/health
# 启用 Bot 时再加：curl http://localhost:8000/health
```

6. 打开浏览器访问 `http://localhost`，确认 Web 首页可用。

## 部署模式

### 模式 A：预构建镜像

这是默认模式，也是当前推荐路径。`docker-compose.yml` 中 `EMBER_API_IMAGE` / `EMBER_WEB_IMAGE` / `EMBER_BOT_IMAGE` 已钉版默认值（随每次发版同步更新），开箱可拉起。

启用 `gateway` profile 时，`EMBER_API_IMAGE` 同时作为 `ember-api` 和 `ember-gateway` 的镜像来源；不能为两个服务配置不同版本。

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

1. 保留 `ember-api` 的 `image:` 作为本地 Tag
2. 取消 `ember-api` 的 `build:` 注释；`ember-gateway` 会复用构建后的同名镜像
3. 执行本地构建

```bash
docker compose build
docker compose --profile gateway up -d
```

镜像构建细节见 [Docker 构建指南](./docker-build-guide.md)。

## 最小验收清单

- `postgres`、`ember-api`、`ember-web` 均为 `Up`
- 启用 `gateway` profile 且设置中心已有可用 `EMBY_URL/EMBY_API_KEY` 后，`ember-gateway` 为 `Up (healthy)`
- `GET http://localhost:8080/health` 返回 200
- Gateway 就绪后，`GET http://localhost:8081/health` 返回 200
- `http://localhost` 可打开前端页面
- API 日志中没有持续刷屏的数据库连接错误
- Gateway 日志中没有持续出现 `process_failed`；如有，优先核对内部 Emby 地址、版本和 API Key
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
