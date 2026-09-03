# 部署排障

这份文档只保留部署期最常见的检查动作和恢复动作。

## 先做最小检查

```bash
cd infrastructure/docker
docker compose ps
docker compose logs --tail=100 ember-api
# 启用 gateway profile 时：docker compose logs --tail=100 ember-gateway
docker compose logs --tail=100 ember-bot
curl http://localhost:8080/health
# 启用 gateway profile 时：curl http://localhost:8081/health
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
- “已生成临时口令，请立即登录并修改密码”

结论很直接：

- `ADMIN_PASSWORD` 没配：仍会创建管理员，但会生成临时口令并要求首次改密
- 数据库里已有 admin：不会重复创建

### 4. Gateway 重启或健康检查失败

检查：

- 设置中心 `EMBY_URL` 是否指向容器可访问的原始 Emby，而不是 Gateway 公网地址
- `EMBY_API_KEY` 是否有效，目标 Server 是否仍为固定兼容版本
- `CONFIG_ENCRYPTION_KEY` 是否与 API 完全一致
- `PLAYBACK_GATEWAY_PORT` 是否正确映射到 Gateway 固定的容器内 `8081`

辅助命令：

```bash
docker compose logs --tail=200 ember-gateway
curl http://localhost:8081/health
```

新环境尚未配置 Emby 时 Gateway fail-fast/restart 是预期；先通过 Web/API 完成设置，不要通过放宽身份核对让进程假启动。

Gateway 在通用 `ember: command=gateway code=process_failed` 之前会打印一条脱敏原因日志：

```text
[PlaybackGateway] code=process_failed stage=<runtime_init|runtime_run> reasonCode=<fixed-code> errorType=<type>
```

常见 `reasonCode`：

| reasonCode | 含义 |
| --- | --- |
| `database_url_invalid` | `DATABASE_URL` 缺失、带首尾空白或换行 |
| `encryption_key_invalid` | `CONFIG_ENCRYPTION_KEY` 缺失、带首尾空白或换行 |
| `emby_url_unavailable` | 设置中心没有可用的 `EMBY_URL` |
| `emby_api_key_unavailable` | 设置中心没有可用的 `EMBY_API_KEY`，或现有密文无法用当前根密钥解密 |
| `upstream_identity_failed` | 无法从原始 Emby 取得合法 ServerIdentity；检查网络、API Key、HTTP 状态和响应合同 |
| `upstream_version_unsupported` | Emby 版本不在 Gateway 支持范围 |
| `runtime_dependency_missing` | Token/DirectPlay 等运行依赖构造失败 |
| `listen_failed` | 固定监听端口 `8081` 不可用，通常是端口已被占用 |
| `serve_failed` / `shutdown_failed` | HTTP Serve 或 graceful shutdown 失败 |

日志不会输出原始错误文本、数据库 DSN、Emby URL、API Key 或响应体；不要为了排障把这些值手工打印出来。

`PLAYBACK_LOCAL_MEDIA_ROOT` 配置错误不会触发 `process_failed`，而是关闭可选本地回退并继续启动 Gateway：

```text
[PlaybackGateway] level=warn code=local_media_disabled reasonCode=<local_media_root_invalid|local_media_root_unsafe|local_media_root_unavailable> errorType=<type>
```

- `local_media_root_invalid`：不是规范化绝对目录、使用 `/`、包含歧义段或非法字符。
- `local_media_root_unsafe`：配置根目录本身是符号链接。
- `local_media_root_unavailable`：容器内目录不存在、不是目录或不可访问；优先核对 Compose override 的 `:ro` mount 和容器内路径是否与变量一致。

日志不会显示真实本地根目录。排障时也不要把宿主机绝对路径写入仓库、工单或公开日志。

### 5. Web 能打开，但页面请求 API 失败

检查：

- `ember-api` 是否真的已启动
- 前端容器日志是否有静态资源或代理配置异常

辅助命令：

```bash
docker compose logs --tail=100 ember-web
curl http://localhost:8080/health
```

### 6. Bot 401 或收不到 Telegram 回调

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

### 7. 某些功能页面能打开，但能力不可用

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
docker compose restart ember-gateway
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
