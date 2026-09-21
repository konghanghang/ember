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
- gateway profile 的 `redis` 是否 healthy，API 与 Gateway 的 `REDIS_URL` 是否指向同一实例

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

Redis 不可用通常不会产生 `process_failed`：客户端构造不执行网络探测，实际请求会在固定短超时后记录 `reasonCode=redis_unavailable` 并走 Emby fallback，用户账号页显示“用量不可用”。先检查：

```bash
docker compose --profile gateway ps redis ember-gateway
docker compose --profile gateway logs --tail=200 redis ember-gateway
```

不要通过固定 Redis 版本、增加多 Gateway 补偿或从 `playback_transfer_tasks` 回放计数来绕过故障。当前合同接受 Redis 数据丢失后从零开始；如果使用外部 Redis，只核对私有 `REDIS_URL` 的网络、认证和通用 Lua/Sorted Set/TTL 能力，禁止把连接串或完整 Key 粘进日志。

日志不会输出原始错误文本、数据库 DSN、Emby URL、API Key 或响应体；不要为了排障把这些值手工打印出来。

#### 客户端无法登录或登录后立即失败

先记录部署镜像的版本/提交、Emby Server 版本、客户端版本与平台，以及失败操作的时间和时区。以下步骤由部署者在现有服务上执行；AI 不因本文存在而自动发起真实客户端验证。

1. 在 Ember 后台设置中心把 `LOG_LEVEL` 临时改为 `debug`。API/Gateway 不读取同名环境变量；等待至少 5 秒后发起一次客户端登录，Gateway 在业务请求边界刷新配置，无需重启。若没有 Debug 摘要，检查是否存在 `log_level_refresh_failed`，不能只凭保存设置就认定 Gateway 已生效。
2. 在实际部署使用的 Compose 目录读取该时间窗口的日志；同一时间尽量只复测一个客户端，避免把其他客户端的成功映射当成本次结果。

   ```bash
   docker compose logs --since=5m --tail=500 ember-gateway
   ```

   重点保留相邻的 `request_completed`、`authentication_*`、`token_*`、`upstream_unavailable` 和 `[EmbyToken]` 行。`request_completed` 包含 method、path、route、状态、认证载体数量和客户端 family/version；不包含 Token 值。排障共享前隐藏主机名、用户/设备/映射标识及媒体路径，禁止附上真实密码、Token、完整认证头、query value 或外部响应体。
3. 按下表判断失败阶段；仅凭时间相邻不能证明两条日志属于同一请求，缺少客户端/路径关联时标记“未证实”。

   | 证据 | 判断与下一步 |
   | --- | --- |
   | 旧版 `code=application_header_invalid route=authentication` | Gateway 在请求到达 Emby 前返回 `401`，不是 Emby 已判定密码错误。修复提交 `e611675` 已移除此登录门槛；新部署仍出现时先核对运行镜像和实例版本 |
   | `code=authentication_metadata_unavailable reasonCode=application_header_invalid` 等 Debug 记录 | 新版仅表示审计元数据无法采集，请求继续转发；不能把其中的 reasonCode 当成旧版本地拒绝 |
   | 已确认运行修复版本，`request_completed route=authentication statusCode=400/401/403/500` | 精确认证路由保留了上游返回的状态；结合 Emby 侧同一请求证据判断原因，不能把所有 `401/403` 都归为密码错误 |
   | `code=upstream_unavailable`，请求返回 `502` | Gateway 到上游的传输失败，先核对可达性与上游运行状态，不按凭据错误处理 |
   | 登录 `200`，同时有 `authentication_response_invalid`、`authentication_response_decode_failed`、`authentication_response_read_failed`、`authentication_response_too_large` 或 `authentication_mapping_failed` | 上游成功响应已透传，但本地映射未建立；继续检查有界旁路解析、上游身份与用户绑定、存储错误。不能将客户端拿到 `200` 等同于整条登录链路通过 |
   | 登录后出现 `token_header_invalid` 或 `token_rejected` | 查看失败请求的 path、载体数量和固定原因。前者涉及 Token 缺失/无效/歧义；后者涉及映射、撤销或身份不匹配。需与本次登录关联，不能用别的客户端的映射成功日志排除问题 |

4. 复测结束后将 `LOG_LEVEL` 恢复为 `info`，按同一业务请求刷新机制生效。取证摘要只记录脱敏结果，不把整段生产日志写入仓库。

`Latest/Resume` 的旧 `item_container_snapshot_unusable` 是列表路由被误当详情后的旁路观察错误，不是登录拒绝证据；该误判已由 `c82064e` 修复。`direct_play_fallback ... statusCode=206` 属于播放回退结果，不能解释前面的登录失败。

部署认证修复后，按同一客户端依次复测：登录、媒体库与图片加载、打开详情、实际播放、字幕及进度/停止上报。分别记录“通过/失败/未执行”，并确认 Infuse、SenPlayer 的既有使用路径没有回归；`302/200/206/204` 日志只能证明对应 HTTP 步骤，不能代替播放器实际出画、声音或字幕结果。把部署提交、双方版本、平台、日期/业务时区及分项结论补入客户端兼容矩阵，未实测项不得写成已兼容。

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

## 已付款但没有续期

1. 在受控运维环境按 Stripe eventId 检查 `stripe_webhook_events` 的 `event_type/status/error_message`，结合本地 paymentId 日志定位订单与用户；不要导出完整支付响应体或凭据。
2. 新实现中本地订单 `expired/failed` 不会阻止成功付款履约；若事件是 `failed`，先处理数据库故障或错误中的业务原因。`reasonCode=plan_group_mismatch paymentId=...` 表示当前用户与订单套餐分组不匹配，必须先按业务事实处理，不能直接将事件或订单改成成功。
3. 故障排除后，通过 Stripe 既有事件重投入口再次发送该事件。API 会重新分发 `failed/received`，订单锁和 completed 终态防止重复发放。自动重试窗口有限，持续失败不能只等待，需要人工跟进。
4. 升级前已被旧代码静默记成 `processed` 的事件，重投不会自动越过去重记录。历史订单需单独只读对账、确认未履约，再制定受控补偿；本次代码升级不自动改写历史事件或补发权益。

以上是部署者的操作路径。本轮修复仅完成 fake 回归，没有执行真实 Stripe 重投、线上对账、补发或退款。

## 相关文档

- [部署指南](./deployment.md)
- [部署环境与配置](./deployment-environment.md)
- [数据库 Migration Baseline](./database-migration-baseline.md)
- [Cloudflared 本地联调](./cloudflared-local-testing.md)
- [数据库迁移说明](../../infrastructure/database/README.md)
