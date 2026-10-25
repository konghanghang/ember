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

   若 `embeddedTokenState=unparseable`，同时查找同一 `requestId` 的 `code=application_header_parse_failed`。新增诊断仅在 Debug 输出，使用现有身份解析器的失败分支，不改变请求或认证结果：

   - `reasonCode` 区分 `empty_header`、`header_too_long`、`invalid_utf8`、`header_line_break`、`unsupported_scheme`、`expected_field`、`invalid_field`、`duplicate_field`、`expected_equals`、`expected_value`、`expected_quote`、`unterminated_quote`、`invalid_escape`、`control_character`、`invalid_value` 和 `expected_comma`。
   - `field` 仅输出 `client/device/device_id/version/user_id/token/other/none`；未知字段名不输出原文。
   - `offset` 是原应用头中从零开始的字节偏移，头部级错误无法定位时为 `-1`；`characterClass` 只区分引号、分隔符、百分号、空白、控制字符、输入结束和 `other` 等固定类别，不输出字符原值。
   - 例如 `reasonCode=expected_quote field=token offset=11 characterClass=single_quote` 表示 Token 值在期望双引号的位置使用了单引号；这是诊断格式示例，不代表任何真实客户端已确认采用该语法。非 Token 元数据已允许 HTTP token 形式，`expected_value` 表示值开头既不是双引号也不是合法 tchar。结合 `route/statusCode` 判断影响：认证路由 `200` 可以与旁路解析失败并存，受保护请求出现 `token_header_invalid` 才表示本地拒绝。

   本地 GoLand 排查时，重启正确的 `cmd/ember gateway` 进程后，由操作者复现一次客户端请求；在 `services/api/logs/gateway-YYYY-MM-DD.log` 中按上述两种 code 和 requestId 取证，无需打印认证头或手动查看 Token。

   排查“同一客户端直连源站能登录、经过 Gateway 返回 `401`”时，同时按 `requestId` 收集 `authentication_request_shape`、`authentication_upstream_response` 和 `request_completed`。前者只观察代理已经读取的最多 `16 KiB`，记录 `contentType/bodyState/usernameState/pwState/passwordState`，没有用户名、密码、长度或摘要；`bodyState=valid_json` 时字段状态为 `missing/null/empty/present/invalid_type/duplicate`，其他 body 状态的字段均为 `unknown`。`pwState=missing` 与 `passwordState=present` 只能证明客户端采用了不同字段，不能据此自动改写登录 body；`present` 也不等于凭据验证通过。上游响应日志才直接证明代理实际取得的上游状态；若请求体尚未读完、超限、压缩或非 JSON，不能把 `unknown` 当成密码缺失。Info 不捕获登录 body，也不输出这组诊断。
3. 按下表判断失败阶段；仅凭时间相邻不能证明两条日志属于同一请求，缺少客户端/路径关联时标记“未证实”。

   | 证据 | 判断与下一步 |
   | --- | --- |
   | 旧版 `code=application_header_invalid route=authentication` | Gateway 在请求到达 Emby 前返回 `401`，不是 Emby 已判定密码错误。修复提交 `e611675` 已移除此登录门槛；新部署仍出现时先核对运行镜像和实例版本 |
   | `code=authentication_metadata_unavailable reasonCode=application_header_invalid` 等 Debug 记录 | 新版仅表示审计元数据无法采集，请求继续转发；不能把其中的 reasonCode 当成旧版本地拒绝 |
   | 已确认运行修复版本，`request_completed route=authentication statusCode=400/401/403/500` | 精确认证路由保留了上游返回的状态；结合 Emby 侧同一请求证据判断原因，不能把所有 `401/403` 都归为密码错误 |
   | `authentication_upstream_response statusCode=401` | 已直接观察到上游认证响应 `401`；结合相同 `requestId` 的请求字段状态、同一源站直连结果和服务端日志定位差异，不能仅凭最终状态判断字段值正确与否 |
   | `code=upstream_unavailable`，请求返回 `502` | Gateway 到上游的真实传输失败，先核对可达性与上游运行状态，不按凭据错误处理 |
   | `499`，视频为 `playback_canceled + proxyErrorCode=request_canceled`，普通代理仅有 Debug 取消诊断 | 代理请求已取消，不误判为 Emby/115 宕机；视频保留单条 Info 决策，不代表客户端收到回退视频 |
   | 登录 `200`，同时有 `authentication_response_invalid`、`authentication_response_decode_failed`、`authentication_response_read_failed`、`authentication_response_too_large` 或 `authentication_mapping_failed` | 上游成功响应已透传，但本地映射未建立；继续检查有界旁路解析、上游身份与用户绑定、存储错误。不能将客户端拿到 `200` 等同于整条登录链路通过 |
   | 登录后出现 `token_header_invalid` 或 `token_rejected` | 查看失败请求的 path、载体数量和固定原因。前者涉及 Token 缺失/无效/歧义；后者涉及映射、撤销或身份不匹配。需与本次登录关联，不能用别的客户端的映射成功日志排除问题 |

4. 复测结束后将 `LOG_LEVEL` 恢复为 `info`，按同一业务请求刷新机制生效。取证摘要只记录脱敏结果，不把整段生产日志写入仓库。

`Latest/Resume` 的旧 `item_container_snapshot_unusable` 是列表路由被误当详情后的旁路观察错误，不是登录拒绝证据；该误判已由 `c82064e` 修复。`direct_play_fallback ... statusCode=206` 属于播放回退结果，不能解释前面的登录失败。

部署认证修复后，按同一客户端依次复测：登录、媒体库与图片加载、打开详情、实际播放、字幕及进度/停止上报。分别记录“通过/失败/未执行”，并确认 Infuse、SenPlayer 的既有使用路径没有回归；`302/200/206/204` 日志只能证明对应 HTTP 步骤，不能代替播放器实际出画、声音或字幕结果。把部署提交、双方版本、平台、日期/业务时区及分项结论补入客户端兼容矩阵，未实测项不得写成已兼容。

#### Yamby 首页图片返回 404 或 503

现有合同允许的无 Token 精确 `/emby/Items/{Id}/Images/{Type}[/{Index}]` GET/HEAD 已从 Web Surface 分离，使用 `route=item_image` 原样请求 Emby。关闭网页或网页配置读取失败不再决定图片响应；携有效 Token 图片仍走普通身份门控。

- 图片仍出现 `route=emby_web`、`web_surface_disabled` 或 `web_surface_config_unavailable`：先核对部署提交和流量实际进入的实例；这是旧版图片与网页开关耦合的路径，不应通过开启整套 Web UI 修复新版本图片。
- `route=item_image` 的 `404/503`：核对同一请求的 Emby 响应或上游传输错误，不再归因于网页配置缓存；HTTP 成功仍需确认图片类型和实际显示。
- `token_header_invalid/token_rejected`：检查 Token 载体及实时用户资格；无效 Token 不能降级成匿名图片。root 无 Token `/Items/.../Images/...`、编码/尾斜杠/非法 Index 等未获合同许可的形态仍受保护。

历史版本因配置请求取消而短暂返回图片 `503` 的排查记录仍只适用于旧链路。真实 Web 页面仍使用原五秒缓存及取消隔离规则，不能仅凭 `errorType=*errors.errorString` 判断取消。部署后由操作者重新进入客户端首页确认图片显示，未执行时保留“未实机验收”。

#### SenPlayer 没有首次转存或续播转回 Emby

在设置中心“媒体集成 → 115 首次转存策略”核对 `P115_TRANSFER_TRIGGER_MODE`。默认严格模式仍要求 JSON 明确起播，SenPlayer 已记录的 query false/body 缺字段只允许复用已有目标。管理员显式选择兼容模式后，合格原文件视频 GET 可以首次转存；此模式也可能让详情预读消耗转存额度，HEAD 始终不新增转存。

最终视频日志的 `transferTrigger=explicit_playback|compatible_request|existing_target|policy_denied` 分别表示明确许可、兼容准入、既有目标复用和策略拒绝。策略拒绝仍使用 `playback_intent_required`；若伴随 Debug `transfer_policy_unavailable`，应检查配置读取或非法值，不能当作 115 账号失效。正常配置更新最多经过一个五秒缓存窗口收敛，已开始的转存不会因关闭模式回滚。

固定会话五分钟后重新请求 Range 时，只有先前成功 302 建立、并由成功租约事件续活的 session binding 才能补查。查看 `playback_info_revalidation_started/revalidated/revalidation_failed` 与最终视频决策；无活跃关联继续回退。补查使用当前用户 Token，保持原会话并核对媒体身份，不延长旧 proof 或重授严格起播许可。停止、关联过期、身份/媒体变化、所有等待者取消或较新客户端响应会阻止旧结果发布。

#### 慢起播、重复 GET 与播放名额异常

需要包端证据时使用 [Gateway 抓包与播放排查](./gateway-packet-capture.md)：脚本位于 `scripts/gateway-capture/`，支持参数化 SSH 跳板、已有容器 PID 发现与离线分析，原始包和分析产物放在仓库外。

preID/challenge 读取使用独立的源取链和 CDN Range 窗口，各最多 5 秒，名义窗口之和为 10 秒；当前 Service/Adapter 不再额外套竞争的 10 秒总定时器，仍受加速总 deadline 与客户端取消约束。查看 Debug `source_range_completed sourceDownloadMs/rangeReadMs/durationMs`，再对照各自的 `provider_operation_completed stage=download_url|source_range`：前者区分取链与实际读取，后者区分 DNS、建连、TLS、等待响应头等阶段。取链失败时不会继续请求 CDN；本地预算耗尽仍为 `source_read_timeout`，不应据此判定账号失效。旧版本的 `source_range` trace 包含内嵌取链回调，不能直接把其 `firstByteMs/connectMs` 当作单次 CDN 请求指标；先核对部署提交与合同文档。

按需 PlaybackInfo 只合并相同 mapping/item/mediaSource/device/Token 摘要；证明复用先筛选完整身份。proof、独立起播许可与播放关联各自最多 4096 条、每 mapping 64 条，满载优先淘汰本 mapping 最旧记录；不能仅凭一次缓存 miss 断言账号故障。停止事件被 Emby 成功接受后，租约更新有独立 2 秒预算，客户端此后断连不阻止释放；Emby 未接受或本次 Redis 更新失败仍按原 TTL 收口。解绑后的本地 UID 桶可能保留未回满余额/等待队列，随后自然回收；这与仍受原 TTL 约束的 Redis 播放占用是两类状态。

秒传初始化的 `provider_operation_completed stage=rapid_upload` 只描述上传 POST；获取上传信息仍是独立 `cookie_get` 观测。`rapidUploadMs` 已包含整个业务调用，不能再次叠加这两条 HTTP trace 的耗时。这里只读取已获授权的日志；真实复现仍须单独授权。

文件与账号故障先按 [运行期失败分类](../reference/p115-cookie-playback-contract.md#102-运行期失败分类) 区分：`file_unavailable`、秒传业务结果拒绝和本地文件校验不写账号故障；即使 reason 为 `provider_protocol`，也要核对具体操作是否存在健康回写。downurl/通用 GET 读取中断且 context 有效仍是 1 分钟临时冷却；HTTP `429` 保留限流冷却，裸 `403` 保留临时不可用冷却。未知业务 errno 不类推账号风控。

在设置中心临时开启 `LOG_LEVEL=debug`，等待至少 5 秒后分别复现“仅打开详情”和“点击播放直到出画，再停止”。记录客户端版本、平台、部署提交及操作时间，不收集原始鉴权 Header、事件 body 或 CDN URL。

1. 找到 `video_request_started`，按 `requestId` 串起该请求。`rangeKind` 和合法字节区间帮助比较重复读取形态；`purpose/secPurpose` 仅作辅助，不能凭缺失标记就判断不是预加载。
2. 查看 `playback_info_request_observed`：`source=client` 是客户端主动请求，`source=gateway` 是实际内部补查；`playback_info_reused_on_demand source=proof_cache` 表示没有新补查。按 `itemRef` 关联同一登录/设备/条目的请求、`playback_info_response_observed` 和 `video_context_resolved`，再按 `requestId/sessionRef` 对照视频及事件。比较“仅详情、点击播放、停止后再次播放”的 `bodyState/isPlayback/enableDirectPlay/enableDirectStream/enableTranscoding/autoOpenLiveStream/startTimeTicks/currentPlaySessionId/mediaSourceId/deviceProfile`。`missing` 不等于 false，body 无法解析时不能推断字段缺失；这些固定状态只供观察，不据此拦截。内部补全、PlaybackInfo 成功或 `IsPlayback=true` 均尚不能单独证明用户点击播放。
3. 按 `direct_play_step step=... phase=started|finished` 找等待位置。尚未看到 finished 时，只能说明日志窗口内没有返回记录；取消、进程中断或日志级别变化也要排除。mediaResolutionCache=hit 时不应出现源解析/查重/取链步骤；先看本次请求的最终决策再判断失败原因。
4. 回退时比较 `video_fallback_headers` 与 `video_fallback_completed` 的 durationMs：前者为等待上游响应头，后者包含代理后续处理；两者起点相同，不能相加。没有 headers 且 upstreamStatus=0 表示未观察到上游响应头，不是 Emby 返回了状态码 0。
5. 画面开始后按 `sessionRef` 对照视频请求与播放事件，查看 `playback_event_received` 的 snapshotState，以及 `playback_lease_updated/not_found/skipped/update_failed`。成功更新应能看到 active/paused 等状态和用量；不同进程的 sessionRef 不可直接比较。Stopped 成功转发并释放租约不意味着清除短期媒体缓存。
6. 复测后恢复 Info。Gateway 只观察自身及 Emby 上游，302 后客户端访问 CDN 的状态、字节和首帧时间仍需客户端侧证据，不能由这些日志推断。

#### 播放器正在播放，控制台用量仍为零

先刷新控制台页面，区分页面快照未刷新与后端读取到零。播放器出画不等于本次创建了新的 115 文件，也不证明 Redis 租约仍存在；已有文件复用不消耗转存额度，302 后视频字节直接来自 CDN。排查使用已经下载的 API/Gateway 日志，不需要启动服务或请求真实外部系统。

1. 按操作时间查看 Gateway 的最终 `decision=redirect|fallback`。`redirect + preexisting=true` 表示复用已有文件，小时/每日转存不增加是预期；fallback 不建立新的 115 播放租约。核对日志的 `userId` 是否为当前控制台用户。
2. 新文件以 `transfer_quota_committed` 的 `added` 和 `transferHourlyUsed/transferDailyUsed` 确认是否记账。`transfer task 成功` 出现在记账之后；`targetState=created`、任务成功和客户端播放成功是不同证据。出现 `transfer_quota_commit_failed usageAvailable=false` 时，不得把未确认结果解释成成功记账。按当前合同，这类故障保留文件，后续复用可能不再补计。
3. 在设置中心临时开启 `LOG_LEVEL=debug` 后，API 立即生效，Gateway 在后续请求边界最多约 5 秒刷新级别。刷新控制台，查看 API `p115_user_usage_read` 的用户、播放及转存计数、`businessTimezone` 和日窗口；同一用户的 Gateway 写入非零、API 相近时间读取为零时，私下核对双方实际运行配置是否使用同一 Redis 实例与数据库编号、相同身份及业务时区。API 能读到一个空 Redis 库时仍会合法返回 `usageAvailable=true` 和零，不能只凭该标记证明读写端一致。不要打印 DSN 或凭证。
4. 播放路数另按同一 Gateway 进程的 `sessionRef` 查看 `playback_event_received` 与 `playback_lease_updated/not_found/skipped/update_failed`。GET 首先只有 30 秒 reservation，成功 Playing/Progress 才续为 2 分钟 active；暂停为 15 分钟。事件缺失、被拒绝或身份不匹配会使租约到期，即使 CDN 播放仍在继续。`not_found` 不会复活过期租约，不能直接通过延长 TTL 或创建缺失租约掩盖原因。
5. 收集同一时间段的 API 与 Gateway 脱敏日志后恢复 Info。新日志随包含本改动的版本上线后才可见；旧日志缺少这些 code 不证明对应步骤没执行。未取得实际日志前，读写端不一致、事件间隔或身份错配都只能作为待核对原因。

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
