# 115 用户自有账号路由与 Redis 配额实现方案

> 状态：草稿（需求边界已确认，尚未实现）
> 负责人：Ember
> 更新时间：2026-09-02

## 背景

当前 115 DirectPlay 已完成一个管理员 source 账号和一个管理员共享 playback 账号的系统内置链路：Gateway 根据 Emby 媒体路径定位 source 文件，将缺失文件秒传到共享 playback 账号，并在兼容条件成立时返回 115 CDN `302`。该实现适合不希望自行配置 115 的家人和朋友，但不适合作为所有套餐组的唯一账号来源：

- 普通用户后续需要绑定自己的 115 playback 账号，由本人承担目标文件和账号风控边界。
- 管理员共享 playback 账号会被多个用户使用，需要按该账号的总活跃播放数限制新直连，不能只按单个用户统计。
- 个人账号同样需要可配置最大播放路数，达到上限时应回退 Emby，而不是拒绝播放。
- 从管理员 source 向 playback 账号秒传新文件会消耗 115 侧操作额度，必须按用户限制小时和每日成功转存数。
- 当前项目没有 Redis；原 DirectPlay 计划曾保留“套餐并发”和数据库 `direct_play_sessions` 设想，本轮文档已撤销并由本计划接管。

## 目标

本方案要实现：

1. 在套餐组上明确选择 `personal` 或 `system` 115 playback 账号来源；所有套餐组默认 `personal`，`system` 必须由管理员主动设置。
2. 为普通用户增加自助绑定一个 115 playback 账号的页面和 API，支持 write-only Cookie、已有目标目录路径、显式验证、启停、解绑和最大播放路数配置。
3. 保留当前管理员维护的全局 source + 共享 playback 链路，让 `system` 套餐组无需用户配置即可继续使用共享 playback。
4. 使用 Redis 同时维护 playback 账号与 Ember 用户的短期 302 预留、当前活跃播放和暂停租约，不创建数据库播放会话表。
5. 使用 Redis 按用户执行小时和每日转存配额；默认值由套餐组提供，首次建议为每小时 `5` 个、每天 `10` 个。
6. 任一 115 账号、并发、配额或 Redis 条件不成立时，对合法用户固定回退 Emby，不污染账号健康状态，也不绕过现有身份与硬状态门控。

## 非目标

本次明确不做：

- 不实现二维码登录、Cookie 自动抓取、Cookie 自动续期或 115 OpenAPI Token 生命周期。
- 不允许一个用户绑定多个个人 playback 账号，不实现个人账号池、粘性选路或主备切换。
- 不新增 `direct_play_sessions` 或其他数据库播放会话表，也不把当前播放数、暂停状态、小时用量和每日用量写入 PostgreSQL。
- 不用 Redis 替代 `p115_accounts` 的持久账号配置、Cookie 密文或现有 `playback_transfer_tasks` provenance。
- 不改变 Emby `SimultaneousStreamLimit`；Emby 继续负责用户整体播放并发，Redis 只决定是否允许新的 115 加速。
- 不自动创建、移动或重命名用户的 115 目录；用户必须填写已经存在的目标目录路径。
- 不因 `Stopped`、Redis TTL、解绑或套餐切换自动删除 playback 文件。
- 不把 115 下载 URL 返回到 API、Web、数据库、日志或可长期读取的 Redis 值中。

## 当前事实

- 当前版本化协议和系统内置链路分别见 [115 Cookie 播放兼容合同](../../reference/p115-cookie-playback-contract.md)、[Emby 4.9 系列播放代理 API 合同](../../reference/emby-playback-proxy-contract.md) 与 [115 Cookie 直连播放端到端流程参考](../../reference/p115-playback-end-to-end-flow.md)。
- `p115_accounts` 当前只表示管理员维护的全局 `source|playback` 账号；`uq_p115_accounts_enabled_role` 限制每个角色全局只能启用一个账号。
- `directplay.Service` 当前通过角色加载唯一 source/playback 账号，还不能按用户和套餐组选择 playback 账号。
- Cookie 已使用 `CONFIG_ENCRYPTION_KEY` 的用途隔离派生密钥加密；创建、替换和验证状态机已有 fake、race 和 PostgreSQL 测试保护。
- `CookieProvider.ResolveDirectoryByPath` 已能把已存在的根相对目录路径解析为唯一目录 ID，但不会创建目录。
- Gateway 已在身份门控成功后旁路解析 `Playing/Progress/Stopped` 的 `ItemId/MediaSourceId/PlaySessionId/PositionTicks/IsPaused`，当前只用于日志，不维护业务计数。
- `playback_transfer_tasks` 已按 `playbackAccountId + SHA1 + size` 保存秒传任务与 provenance，并用 PostgreSQL advisory lock 防止相同目标文件重复秒传；它不是播放会话或用户配额表。
- 当前仓库没有 Redis 客户端、`REDIS_URL`、Compose Redis 服务或 Redis 测试夹具。
- 现有实机证据确认 Infuse `8.5.2` 的 Playing/Progress/Stopped 可经 Gateway 转发并取得 `204`，但没有固定“暂停后是否持续发送 Progress、多久发送一次”的客户端时序证据。

## 已确认决策

| 主题 | 决策 |
| --- | --- |
| 套餐归属 | 字段放在 `plan_groups`，不放在单个可购买的 `plans` 商品 |
| 默认账号来源 | 所有已有和新套餐组默认 `personal`；只有管理员显式选择后才使用 `system` |
| `system` 语义 | `system` 只表示套餐路由到当前管理员共享 playback，不是 `p115_accounts` 的账号类型或 scope |
| `system` 路由 | 使用当前管理员 source + 管理员共享 playback；用户无需绑定 115 |
| 个人模式 | 使用管理员 source + 当前用户唯一的个人 playback 账号；未绑定或不可用时回退 Emby |
| 并发归属 | 最大播放路数属于具体 playback 账号，不属于套餐组 |
| 租约状态 | `reservation → active ↔ paused → stopped/expired`；302 前短预留与真实活跃播放必须分开，不能把 `HEAD` 或预加载直接算成正在播放 |
| 两组计数 | Redis 同时维护账号/用户的占用数和真实活跃数；占用数包含 `reservation + active + paused` 并用于准入，活跃数只包含 `active + paused` 并用于展示、归因和后续治理 |
| 实际门控 | 当前只用账号占用数执行 `maxConcurrentStreams`；用户占用数不参与第二套并发门控，Emby 继续负责用户整体播放并发 |
| 转存配额 | 小时/每日限额属于套餐组，由管理员配置，按发起播放的 Ember 用户统计 |
| 会话存储 | 不建数据库会话表；Redis 处理 302 reservation、Playing/Progress/Stopped 状态晋级、暂停和 TTL |
| 失败策略 | 合法用户在账号、Redis、并发或配额不满足时进入公共 fallback；本地回退方案落地后先查本地精确路径，未命中才到 Emby；身份和硬状态失败仍拒绝 |

## 方案设计

本文后续沿用的“fallback Emby”表示当前已实现行为。与 [STRM 本地媒体回退播放实现方案](./strm-local-media-fallback.md) 组合后，账号、Redis、并发或配额失败先进入同一个 Gateway fallback 选择器：本地精确路径命中则直接播放，本地未命中才代理 Emby/CloudDrive2。`personal|system` 套餐路由选择、Redis 计数和转存配额本身不负责本地文件判断。

### 1. 用户可见行为

#### 1.1 套餐组

管理员在现有套餐组页面配置：

- `p115PlaybackMode`：`personal|system`，默认 `personal`。
- `p115TransferHourlyLimit`：每个用户滚动 60 分钟内允许成功创建的新 playback 文件数，默认 `5`。
- `p115TransferDailyLimit`：每个用户在 `CRON_TIMEZONE` 自然日内允许成功创建的新 playback 文件数，默认 `10`。

套餐组不再配置 115 播放并发。套餐模式或转存配额更新只影响后续新请求，不撤销已签发的 115 CDN URL。

#### 1.2 普通用户

控制台新增独立“115 网盘”菜单。普通用户可以：

- 查看本人账号的脱敏状态、目标目录路径、最大播放路数、当前账号/本人真实活跃数；短 reservation 单独显示为“准备中”，不混入正在播放。
- 填写或替换 Cookie；Cookie 使用密码输入语义、禁止自动填充，提交后立即清空，查询接口永不返回。
- 手工填写一个已经存在的 115 目标目录路径；后端解析并保存唯一 `targetParentId`，页面不要求用户获取内部 ID。
- 显式验证、启用、停用或解绑自己的账号。
- 设置本人 playback 账号的 `maxConcurrentStreams`。
- 查看当前小时/每日成功转存用量和套餐组限额，但不能自行修改套餐额度。

用户处于 `system` 套餐组时仍可提前绑定个人账号，但新播放只使用管理员共享 playback；切换到 `personal` 后无需重新录入。

#### 1.3 管理员共享账号

现有管理员 115 账号页面继续管理唯一全局 source 和唯一共享 playback 账号：

- source 配置 Cookie、Emby Path 前缀和 source 根目录，不展示无意义的最大播放路数。
- 共享 playback 配置 Cookie、已有目标目录路径和 `maxConcurrentStreams`。
- 共享 playback 的当前活跃数按所有 `system` 套餐用户合计，不按用户分别重置账号上限。

### 2. 数据与模型

#### 2.1 `plan_groups`

在现有模型与幂等 SQL migration 中增加：

- `p115_playback_mode VARCHAR(...) NOT NULL DEFAULT 'personal'`
- `p115_transfer_hourly_limit INTEGER NOT NULL DEFAULT 5`
- `p115_transfer_daily_limit INTEGER NOT NULL DEFAULT 10`

要求：

- `p115_playback_mode` 只接受 `personal|system`。
- 两个配额必须是正整数；本次不使用含义不清的 `0` 表示禁用或无限。
- migration 将全部已有套餐组回填为 `personal`；部署者需要在启用新路由前显式把家人/朋友套餐组改成 `system`。
- Go 字段使用 CamelCase、JSON 使用 camelCase、GORM 显式指定 snake_case 列名。

#### 2.2 `p115_accounts`

复用现有表并增加：

- `owner_user_id`：可空并引用 `users(id) ON DELETE RESTRICT`；管理员维护的全局 source/共享 playback 为空，未解绑的个人 playback 必须等于其 Ember 用户 ID。该字段为空本身不能决定账号是共享账号，查询还必须结合 `status`；RESTRICT 保证任何绕过业务 Service 的用户删除都会失败关闭，而不是把活动个人账号静默变成共享候选。
- `target_parent_path`：规范化目录路径快照，只用于输入回显和诊断；`target_parent_id` 仍是运行时真相源。
- `max_concurrent_streams`：非 revoked playback 账号必填正整数；source 与 revoked 账号必须为空。
- `status` 增加不可逆终态 `revoked`；`system` 不进入账号状态枚举。
- `cookie_ciphertext` 改为可空，但只允许 `status=revoked` 时为空；Go 模型同步改为可空类型，所有非 revoked 凭证加载仍要求非空密文。

需要重写现有 partial unique/约束：

- 管理员共享范围只能启用一个 source 和一个 playback；两者均要求 `owner_user_id IS NULL AND status <> 'revoked'`。
- 每个 `owner_user_id` 只能拥有一个 `status <> 'revoked'` 的个人 playback；历史 revoked tombstone 不阻止同一用户重新绑定新账号。
- 同一非空 `provider_user_id` 不能被任一非 revoked 的管理员共享账号或个人账号重复绑定。
- 普通用户只能创建 `playback`，不能创建 source、指定其他 owner 或修改管理员共享账号。
- 数据库 CHECK 要求 `owner_user_id IS NOT NULL` 的账号只能是非 revoked playback；`status=revoked` 时必须同时满足 `enabled=false`，且 owner、Cookie、Provider、目标目录、最大路数、验证/成功时间、冷却和错误字段全部为空。
- 用户个人 playback 与管理员 source 的 Provider UID 必须不同，禁止借 source 账号签发最终直链。
- 所有 owner、状态、凭证、目录与并发约束都必须在 SQL migration 中重写，并由 GORM 字段显式映射；不能只在 Service 层约定。

##### 解绑与用户删除

`DELETE /api/v1/user/p115-account` 的语义是“撤销并擦除凭证”，不是物理删除账号行：

1. 事务内按 `owner_user_id` 锁定当前非 revoked 个人 playback；不存在时按幂等成功返回。
2. 在清空字段前取得只存在于当前调用内存中的非敏感 Redis cleanup handle；随后原子写入 `status=revoked + enabled=false`，并清空 `owner_user_id/cookie_ciphertext/provider_user_id/target_parent_id/target_parent_path/max_concurrent_streams`、验证/成功时间、冷却和错误字段。cleanup handle 不落库、不进入日志或响应。
3. 保留账号 `id/role/alias/created_at/updated_at`，让既有 `playback_transfer_tasks` 继续通过 `ON DELETE RESTRICT` 引用同一个账号 ID；不删除 transfer provenance，也不调用 `DeleteFile`。
4. revoked 是不可复活终态：验证、启停、Cookie/目录/并发更新和 DirectPlay 加载全部拒绝。用户重新绑定必须创建新的账号 ID，不能让新凭证继承旧任务历史。
5. 数据库撤销成功后尽力清理该账号的 Redis reservation/active/paused；Redis 清理失败只记录固定脱敏日志并等待 TTL，不回滚凭证擦除，也不承诺撤销已签发 CDN URL。

管理员删除 Ember 用户时，顺序固定为：本地 Gateway Token 撤销 → 个人 115 账号 tombstone（同一事务清空 owner）→ 尽力清理 Redis → 删除 Emby 用户 → 删除 Ember 用户。Token 撤销或 tombstone 失败时不得继续外部/本地用户删除；Redis 清理失败不阻止后续删除。`owner_user_id ON DELETE RESTRICT` 作为数据库兜底，保证仍有关联的非 revoked 个人账号时用户行不能被直接删除；完成 tombstone 后用户删除不再触碰历史账号行。Emby 删除失败时保留已经完成的本地 Token 撤销和账号凭证擦除，不回滚安全结果。

#### 2.3 保持不变的数据

- `playback_transfer_tasks` 继续以 `ON DELETE RESTRICT` 引用 source/playback 账号并记录目标文件和秒传 provenance；个人账号解绑后引用指向 revoked tombstone，不新增用户当前播放数字段。
- `emby_access_tokens` 继续只保存 purpose 隔离的 HMAC 摘要。
- 不创建 `direct_play_sessions`、用户转存计数表或每日统计表。

### 3. Redis 数据边界

#### 3.1 运行要求

- 新增 Redis 客户端边界和 `REDIS_URL`；Cookie、Emby Token、完整 SHA1、下载 URL 和 Provider 原始响应不得进入连接串示例、Key 或 Value。
- Compose 的 `gateway` profile 增加 Redis 服务、healthcheck、持久卷和 AOF；允许部署者用外部 `REDIS_URL` 覆盖。
- Redis 不可用时 Gateway 仍可代理 Emby，但禁止签发新的 115 `302`，固定 fallback Emby。
- Redis 脚本使用固定参数化实现，所有 Key 通过 `KEYS` 传入；客户端必须处理脚本缓存丢失并重新加载。
- 账号/用户活跃索引和反向会话映射必须在一个 Redis 原子操作中更新，禁止分别 `INCR/DECR` 造成漂移。

Redis 官方合同依据：Lua 脚本在服务端原子执行，并允许跨多个 Key 进行条件更新；脚本缓存是易失的，客户端必须处理重启或 failover 后的重新加载。Sorted Set 的唯一 member、score 和范围删除语义用于实现带到期时间的当前播放索引与滑动窗口限流。参考 [Redis Lua scripting](https://redis.io/docs/latest/develop/programmability/eval-intro/)、[Redis sorted sets](https://redis.io/docs/latest/develop/data-types/sorted-sets/) 和 [`INCR` rate limiter pattern](https://redis.io/docs/latest/commands/incr/)。

#### 3.2 302 预留与当前活跃播放

至少维护：

```text
{p115}:leases:account:{playbackAccountKey}
{p115}:leases:user:{userId}
{p115}:active:account:{playbackAccountKey}
{p115}:active:user:{userId}
{p115}:session:{sessionFingerprint}
```

- account/user `leases` 索引使用 Sorted Set 保存所有仍占账号名额的 `reservation|active|paused`；`active` 索引只保存 `active|paused`，用于 API 展示真实活跃播放。两类索引的 member 都是同一 `sessionFingerprint`，score 为各自状态的租约到期时间。
- `sessionFingerprint` 使用服务端用途隔离 HMAC 关联 `serverId + userId + mappingId + deviceId + playSessionId`，不在 Redis 保存原始 Token。
- 反向 session 值只保存继续更新两组索引所需的内部账号键、用户 ID、`reservation|active|paused` 状态和有界时间；不根据用户当前套餐重新猜测历史播放使用的账号。
- `playbackAccountKey` 必须稳定标识实际 115 playback 账号，账号解绑/重绑不能通过生成新数据库 ID 绕过仍有效的旧租约；具体派生不得暴露 Cookie 或 Provider 凭证。

`GET` 原子预留流程：

1. 只有身份、PlaybackInfo 证明和静态原文件条件完整的 `GET` 才能申请新预留；首次实际播放、带 Range 的 GET 或预加载 GET 在网关侧无法可靠区分，因此都最多形成同 session 的一个短 reservation，不能直接形成 active。`HEAD` 和会话事件不能创建新预留。
2. 清理 account/user `leases` 与 `active` 索引中 score 已过期的 member。
3. 如果同一 session 已存在，只复用既有 `reservation|active|paused`，不重复计数，也不因视频重试把 `active|paused` 降级成 reservation。
4. 读取 playback 账号 `maxConcurrentStreams`，账号 `leases` 占用数已满则返回 `account_concurrency_exceeded`。
5. 未满时同时写入 account/user `leases` 和反向 session，状态为 `reservation`，TTL 暂定 `30s`；此时不写 `active` 索引。
6. DirectPlay 未能生成安全候选时立即原子释放本次 `reservation`；既有 `active|paused` 不能因一次重新签发失败被释放。成功返回 `302` 后只保留 `reservation`，等待成功的播放事件晋级。

`HEAD` 处理固定为：已有同 session 的 `reservation|active|paused` 时可以复用并继续执行直链候选；没有既有租约时不创建新租约、不触发新的 115 DirectPlay，直接进入公共 fallback。

事件语义：

- 只有会话事件成功转发给 Emby，且反向 session 已证明该会话取得过 115 `reservation|active|paused`，才允许更新 Redis；普通 Emby/local fallback 会话不能借事件创建 115 租约。
- `Playing`：把已有 `reservation|paused` 晋级/恢复为 `active`，或续租已有 `active`；同时写入 account/user `active` 索引并使用 active TTL。
- `Progress + IsPaused=false`：把已有 `reservation|paused` 晋级/恢复为 `active`，或续租已有 `active`。
- `Progress + IsPaused=true`：把已有 `reservation|active` 晋级/切换为 `paused`，继续计入 `leases` 与 `active`，并使用更长的 paused TTL；暂停不能按 `Stopped` 删除。
- `Stopped`：只有请求成功转发给 Emby 后，才同时删除 account/user `leases`、`active` 和反向 session。
- 找不到反向 session 的 Playing/Progress/Stopped 不创建或猜测 playback 账号租约，只记录固定观察结果；无后续事件时由 score/Key TTL 自然过期，不依赖数据库 cron 才能释放名额。

首版建议 reservation TTL 为 `30s`、active TTL 为 `2m`、paused TTL 为 `15m`。这些是保守初值，不是实机最优值；真实 Infuse 的视频请求与 Playing 顺序、暂停/恢复时序有证据后再调整。reservation 只承担 302 签发前后的并发防穿透，不展示为正在播放；暂停期间继续占用一路，避免恢复旧连接时突破账号上限。

#### 3.3 用户转存配额

至少维护：

```text
{p115}:transfer:pending:{userId}
{p115}:transfer:succeeded:{userId}
```

- 小时窗口使用滚动 60 分钟；每日窗口使用 `CRON_TIMEZONE` 的自然日边界。
- 配额适用于 `personal` 和 `system` 两种模式，按发起请求的 Ember 用户统计。
- 只有目标 playback 账号原本缺少文件、秒传成功且目标目录复核通过，才消耗一次成功额度。
- 目标预存命中、重复 HEAD/GET/Range、重新签发下载 URL和失败尝试不消耗成功额度。
- 管理员共享 playback 中由其他用户先前转存的文件如果已命中，本次用户不消耗额度。

为防止并发穿透，配额必须在现有 transfer lock 内的第二次目标查重仍未命中后、调用 `InitRapidUpload` 前原子预留：

1. 同时清理过期 pending/succeeded 事件并计算小时、每日已用量。
2. 任一窗口达到套餐组上限时返回 `transfer_quota_exceeded`，不调用 115。
3. 未达到上限时写入短 TTL pending reservation。
4. 目标复核成功后原子转为 succeeded；失败或确认预存命中时删除 pending。
5. 进程异常退出时 pending 自动过期，不能永久占用额度。

`transfer_quota_exceeded`、Redis 不可用和账号并发已满都是 Gateway 加速资格结果，不是 Provider 健康错误：不能把账号写成 `expired/error/cooling_down`。

### 4. API 与权限边界

列表接口统一返回 `data`，字段使用 camelCase。Cookie 只出现在创建或替换请求。

#### 4.1 套餐组

复用现有管理员套餐组接口：

- `POST /api/v1/admin/plan-groups`
- `PUT /api/v1/admin/plan-groups/:key`
- `GET /api/v1/admin/plan-groups`

请求/响应增加 `p115PlaybackMode`、`p115TransferHourlyLimit`、`p115TransferDailyLimit`。创建默认值和 migration 默认值必须一致。

#### 4.2 用户个人 115 账号

使用统一认证用户路由，普通用户只能访问自己的账号：

- `GET /api/v1/user/p115-account`
- `POST /api/v1/user/p115-account`
- `PUT /api/v1/user/p115-account/cookie`
- `PUT /api/v1/user/p115-account/directory`
- `PUT /api/v1/user/p115-account/concurrency`
- `POST /api/v1/user/p115-account/validate`
- `PUT /api/v1/user/p115-account/enabled`
- `DELETE /api/v1/user/p115-account`
- `GET /api/v1/user/p115-usage`

账号摘要只返回脱敏 Provider 标识、状态、启用状态、目录路径快照、最大播放路数、账号/用户的 `reservedStreams`、`activeStreams`、`occupiedStreams` 和小时/每日配额用量；其中 `occupiedStreams = reservedStreams + activeStreams`，`activeStreams` 已包含 paused。Redis 不可用时这些用量字段必须明确为 unavailable，不能伪装成零。

目录接口接收用户输入的已有目录路径，使用本人当前 write-only Cookie/加密凭证调用 `ResolveDirectoryByPath`；成功后保存规范化路径快照和唯一 ID。禁止使用根目录兜底、自动创建目录或返回 Provider 原始目录响应。

#### 4.3 管理员共享账号

现有 `/api/v1/admin/p115-accounts` 合同继续只管理 `owner_user_id IS NULL AND status <> revoked` 的管理员全局 source/共享 playback，并为 playback 摘要、创建和更新增加 `targetParentPath` 与 `maxConcurrentStreams`。source 请求必须拒绝并发字段，playback 请求必须拒绝 source 位置字段；解绑时已清空 owner 的 revoked tombstone 不进入该控制面。

### 5. DirectPlay 账号选择

每次视频候选在调用 Provider 前：

1. 使用已经解析的 Principal 实时读取用户有效套餐组；`users.plan_group IS NULL` 时继续遵守默认套餐组语义。
2. `p115PlaybackMode=system`：加载唯一可运行的管理员共享 playback，查询必须同时要求 `owner_user_id IS NULL + status=active + enabled=true`；`system` 只是本次路由模式。
3. `p115PlaybackMode=personal`：按 Principal.User.ID 加载本人唯一、已验证且启用的 personal playback 账号。
4. 个人账号未绑定、未验证、停用、过期或冷却时，返回类型化不适用原因并 fallback Emby。
5. source 始终使用管理员维护的全局 source；source 与选中 playback 的 Provider UID 相同则拒绝加速。
6. `GET` 使用选中 playback 账号申请 Redis `reservation`；账号占用已满或 Redis 不可用时 fallback Emby。`HEAD` 只有命中同 session 既有租约才继续，否则直接 fallback。
7. 目标查重命中则直接签发新下载 URL，不消费转存配额；缺失时进入 transfer lock 和 Redis 配额预留。
8. 只有全部合同成立才返回空体 `302`；任一步失败均保持现有 Emby fallback。

套餐模式改变、个人账号停用或最大播放路数调低时不撤销已签发链接，也不删除文件；只影响新租约。最大值调低到当前占用数以下时，新 115 播放持续 fallback，直到 `reservation + active + paused` 占用数低于新上限。

### 6. 日志与可观察性

视频最终决策日志继续保持每请求一条，并补充固定字段：

- `playbackMode=personal|system`
- `playbackAccountOwner=shared|current_user`
- `accountActiveStreams`、`accountStreamLimit`
- `userActiveStreams`
- `accountReservedStreams`、`accountOccupiedStreams`
- `userReservedStreams`、`userOccupiedStreams`
- `transferHourlyUsed/Limit`、`transferDailyUsed/Limit`（只有进入转存配额判断时记录）
- `reasonCode=personal_account_missing|account_concurrency_exceeded|transfer_quota_exceeded|redis_unavailable`

禁止记录 Cookie、Token、完整 SHA1、下载 URL、Redis 连接串、原始 PlaySessionId、Lua 参数原文或 Provider 原始错误。Redis Key 日志只允许固定模板名，不打印完整实例 Key。

## 失败路径与边界条件

- 用户是 `personal` 但未绑定账号：fallback Emby，不自动借用管理员共享 playback。
- 用户是 `system`：忽略个人账号，使用管理员共享 playback；共享账号不可用时 fallback Emby。
- Redis 不可用、超时或脚本失败：fallback Emby；不能按零活跃数放行，也不能污染账号健康。
- 账号 `reservation + active + paused` 占用数达到 `maxConcurrentStreams`：fallback Emby；不拒绝用户，不撤销 Token。
- 任一 account/user `leases` 或 `active` 索引写入失败：整个原子操作失败并 fallback，禁止只更新部分索引。
- `HEAD` 没有同 session 既有租约：不创建 reservation，不触发新的 115 DirectPlay，进入公共 fallback。
- 302 后没有成功 Playing/Progress：reservation 最多保留 `30s` 后自然过期，不能长期显示为真实活跃播放。
- `IsPaused=true`：继续占用，使用 paused TTL；不能删除索引。
- Stopped 上游失败：不假装停止成功，等待后续事件或 TTL 收口。
- 转存配额达到小时或每日上限：不调用 `InitRapidUpload`，fallback Emby。
- 转存失败：释放 pending；Provider 类型化错误仍按现有账号健康合同处理。
- 目标文件已经存在：不消耗转存额度，只申请播放租约并获取新直链。
- Redis 重启或数据丢失：Gateway 不能把缺失 Key 当作已确认的零占用；部署和恢复策略必须在实现前固定，AOF 只降低风险，不替代故障时 fail-safe。
- 用户解绑：先把个人账号原子写成 revoked tombstone 并擦除凭证，再尽力删除 Redis 租约；旧 CDN URL 无法保证立即失效，Redis 失败时旧租约按 TTL 收口。
- 用户解绑后重新绑定：必须创建新账号 ID；旧 transfer 继续引用 revoked tombstone，不能改挂到新凭证。
- 管理员删除用户：Token 撤销或个人账号 tombstone 失败时不调用 Emby 删除；Redis 清理失败只降级为 TTL 收口，不能恢复已擦除凭证。

## 影响范围

- API：扩展套餐组和 115 账号模型，新增用户账号/用量接口、账号选择器、Redis 租约和配额 Service。
- Gateway：在 DirectPlay 前后申请/释放 Redis 租约，旁路消费成功的 Playing/Progress/Stopped。
- Web：套餐组页面增加账号来源和转存配额；新增普通用户“115 网盘”页面；管理员共享 playback 增加路径和最大路数。
- Bot：本次无改动。
- 数据库：只修改长期配置模型与 SQL migration，不新增播放会话或配额使用表。
- 配置/部署：新增 Redis 客户端、`REDIS_URL`、Compose Redis、healthcheck、AOF、数据卷和故障说明。
- 文档：实现时同步系统架构、配置参考、数据模型、API 目录、Web 信息架构、部署和测试 runbook。

前端实现必须遵守 Ember 风格；设计与交互基线以 [Web 设计规范](../../reference/web-design-guide.md) 为准。当前没有偏离规范的特例。页面应通过字段、状态和动作表达能力，不堆叠解释性文案。

## 验证方式

### 自动化验证

- Go TDD：套餐模式默认/更新、个人账号所有权、Cookie write-only、目录解析、唯一约束、账号选择、revoked 终态和所有 fallback 原因。
- Redis adapter/fake 测试：GET 原子 reservation、leases/active 两组索引一致性、重复 session、并发上限、reservation/active/paused TTL、状态晋级、Stopped、脚本缓存丢失、连接失败和取消；默认验证不得启动项目服务或访问真实 Redis 云服务。
- 转存配额测试：滚动小时窗口、`CRON_TIMEZONE` 自然日、并发预留、成功提交、失败退款、pending crash TTL、预存不计数、system/personal 一致口径。
- Gateway fake Emby/115 测试：HEAD 无租约不创建且直接 fallback、HEAD 命中既有租约可复用、重复 GET 不重复计数、预加载只形成短 reservation；Playing/Progress/Stopped 请求与响应透明，只有成功事件且命中反向 session 才更新 Redis；Redis/配额/账号失败均回退 fake Emby。
- PostgreSQL 集成测试：migration 幂等、套餐默认 personal、管理员共享/个人账号 partial unique、所有权隔离、revoked 条件约束、带 transfer 历史的解绑、非 revoked owner 对直接用户删除的 RESTRICT、tombstone 清空 owner 后可删除用户、tombstone 不会进入共享账号查询，以及既有管理员账号兼容。
- 用户删除状态流转测试：Token 撤销失败或 tombstone 失败时不得调用 Emby 删除；Redis 清理失败仍保留凭证擦除并继续删除；Emby 删除失败不复活 Token 或个人账号；重新绑定生成新账号 ID 且旧 transfer 归属不变。
- Web Vitest：套餐字段、用户 Cookie 清空、目录路径、最大路数、reservation“准备中”与真实活跃区分、状态门控、用量 unavailable 和角色隔离。
- `services/api` 下执行 `go test ./...`、关键包 `go test -race`、`go vet ./...`、`go build ./...`。
- `services/web` 下执行 `npm run test`、`npm run build`。
- Compose 静态校验 Redis profile、依赖、healthcheck、volume 和外部 `REDIS_URL` 覆盖。

所有 Emby/115 测试必须使用 fake/fixture；测试不得启动项目服务或真实请求 Emby、115、Redis 云服务或其他外网。

### 受控验证

真实验证必须由用户另行明确授权，并逐项执行：

1. 先确认套餐组默认 personal、system 显式切换和无个人账号时 Emby fallback。
2. 使用测试用户/测试 115 账号验证 Cookie、已有目录、最大路数和个人目标秒传。
3. 分别验证 personal 账号上限与 system 共享账号总上限，确认超限只 fallback Emby。
4. 验证 HEAD、首次 GET、Playing、暂停 Progress、恢复 Progress、Stopped、预加载未播放和无 Stopped TTL；记录目标客户端精确版本、事件顺序以及 reservation 是否按期晋级或释放。
5. 验证第 5/6 个小时请求、第 10/11 个每日请求、预存文件和失败退款边界。
6. 验证 Redis 短暂不可用时 Gateway 仍可通过 Emby 播放，且没有未计数的 115 新直链。

真实验证不得输出 Cookie、Token、完整 URL、完整 SHA1、Redis DSN 或 Provider 原始响应。

## 分阶段落地

### 阶段 0：计划、合同与 Redis 原型

- 已修订现有 115 总计划和稳定参考中的旧套餐并发/数据库会话设想。
- 固定 Redis Key、`reservation → active ↔ paused → stopped/expired` 原子结果、HEAD 禁止创建、三类 TTL、配额窗口和故障行为。
- 为普通用户 Cookie 写入、目录解析和所有权补充合同测试。

完成条件：计划、数据合同、失败语义和 fake Redis 测试边界明确，不依赖真实 115 开始业务实现。

### 阶段 1：持久配置与用户控制面

- migration、模型、套餐组字段和现有管理员共享账号扩展；`system` 只保留为套餐路由枚举。
- 用户个人账号 API/Web、Cookie/目录/最大路数、revoked tombstone 和用户删除联动。
- 暂不切换 Gateway 数据面。

完成条件：管理员共享账号 userspace 保持，个人账号只能由 owner 管理；解绑后 Cookie 被擦除、transfer provenance 保留、tombstone 不会被共享加载器选中，套餐默认 personal 有 migration 和前端测试保护。

### 阶段 2：Redis 播放租约与账号路由

- Redis 部署/客户端、account/user `leases + active` 两组索引、反向 session、reservation/active/paused TTL 和事件旁路。
- Gateway 按 `personal|system` 套餐模式选择个人 playback 或管理员共享 playback，账号满或 Redis 失败时 fallback。

完成条件：并发和三类 TTL 测试覆盖多 Gateway 竞争；HEAD/预加载不会形成真实活跃会话，GET reservation 不能穿透账号上限，普通 Emby 代理在 Redis 故障时保持可用。

### 阶段 3：转存配额

- 小时/每日配额、pending reservation、成功提交和失败退款。
- 用量 API/Web 与固定诊断日志。

完成条件：并发请求不能穿透 `5/10` 默认值，预存文件和失败不误扣，日界线复用 `CRON_TIMEZONE`。

## 已完成项、剩余项与归档条件

已完成：

- 需求方向和核心语义已确认。
- 现有 115 总计划、计划索引、盘点清单和稳定参考已同步新的文档边界。
- 管理员全局 source + 共享 playback、Cookie Provider、秒传/复核/302 和 Emby fallback 已由现有计划落地。
- Gateway 已有三类播放事件的透明旁路解析基础。

剩余：

- 本计划阶段 0 至阶段 3 的全部代码、migration、Redis、Web、验证和稳定文档同步。

归档条件：

- 四个阶段全部落地并通过自动化验证。
- 真实验证按用户授权范围记录证据；未授权的外部 E2E 必须明确标为未验证，不能伪写通过。
- 当前实现事实提炼到 `docs/system-architecture.md` 和对应 `docs/reference/`。
- `docs/plan/README.md`、计划盘点和交叉引用同步完成后，移入 `docs/archive/plan/architecture/`。

## 落地后文档处理

实现后同步：

- `docs/system-architecture.md`：账号归属、套餐路由、Redis 边界和 fallback。
- `docs/reference/data-model-reference.md`：套餐组和账号长期配置字段。
- `docs/reference/configuration-reference.md`：Redis 地址、可用性、TTL 和生效方式。
- `docs/reference/api-endpoint-catalog.md`：用户账号、用量和套餐组字段。
- `docs/reference/web-information-architecture.md`：用户 115 菜单、管理员共享账号和套餐组页面职责。
- `docs/reference/p115-playback-end-to-end-flow.md`：从当前全局账号链路更新为已实现的 system/personal 路由。
- `docs/runbooks/deployment*.md`：Redis 部署、AOF、备份、故障回退和敏感配置。
