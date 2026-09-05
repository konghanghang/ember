# 115 用户自有账号路由与 Redis 配额实现方案

> 状态：代码、自动化与 PostgreSQL 集成已完成，待受控真实验收
> 负责人：Ember
> 更新时间：2026-09-04

## 背景

当前 115 DirectPlay 已完成一个管理员 source 账号和一个管理员共享 playback 账号的系统内置链路：Gateway 根据 Emby 媒体路径定位 source 文件，将缺失文件秒传到共享 playback 账号，并在兼容条件成立时返回 115 CDN `302`。该实现适合不希望自行配置 115 的家人和朋友，但不适合作为所有套餐组的唯一账号来源：

- 普通用户后续需要绑定自己的 115 playback 账号，由本人承担目标文件和账号风控边界。
- 管理员共享 playback 账号会被多个用户使用，需要按该账号的总活跃播放数限制新直连，不能只按单个用户统计。
- 个人账号同样需要可配置最大播放路数，达到上限时应回退 Emby，而不是拒绝播放。
- 从管理员 source 向 playback 账号秒传新文件会消耗 115 侧操作额度，必须按用户限制小时和每日成功转存数。
- 实施前项目没有 Redis；原 DirectPlay 计划曾保留“套餐并发”和数据库 `direct_play_sessions` 设想，本计划已撤销旧方向并完成 Redis 数据面的实现。

## 目标

本方案要实现：

1. 在套餐组上明确选择 `personal` 或 `system` 115 playback 账号来源；所有套餐组默认 `personal`，`system` 必须由管理员主动设置。
2. 为普通用户增加自助绑定一个 115 playback 账号的页面和 API；凭证侧只要求 write-only Cookie，`appType` 由后端识别、Provider User-Agent 由后端固定，同时支持已有目标目录路径、显式验证、启停、解绑和最大播放路数配置。
3. 保留当前管理员维护的全局 source + 共享 playback 链路，让 `system` 套餐组无需用户配置即可继续使用共享 playback。
4. 使用 Redis 同时维护 playback 账号与 Ember 用户的短期 302 预留、当前活跃播放和暂停租约，不创建数据库播放会话表。
5. 使用 Redis 按用户执行小时和每日转存配额；套餐组默认固定为每小时 `5` 个、每天 `10` 个。
6. 任一 115 账号、并发、配额或 Redis 条件不成立时，对合法用户固定回退 Emby，不污染账号健康状态，也不绕过现有身份与硬状态门控。

## 非目标

本次明确不做：

- 不实现二维码登录、Cookie 自动抓取、Cookie 自动续期或 115 OpenAPI Token 生命周期。
- 不允许一个用户绑定多个个人 playback 账号，不实现个人账号池、粘性选路或主备切换。
- 不新增 `direct_play_sessions` 或其他数据库播放会话表，也不把当前播放数、暂停状态、小时用量和每日用量写入 PostgreSQL。
- 不用 Redis 替代 `p115_accounts` 的持久账号配置、Cookie 密文或现有 `playback_transfer_tasks` provenance。
- 不修改 Emby `SimultaneousStreamLimit` 的现有配置和同步机制；本计划只把有效套餐模板中的该值作为个人 115 账号配置上限，不新增 Gateway 用户级总并发门控。
- 不把 Emby 当作已证实的分流播放并发门控：115 `302` 后视频字节不经过 Emby，后续本地命中也不访问 Emby 视频上游，当前没有证据证明 `SimultaneousStreamLimit` 能拦截这两类播放。
- 不自动创建、移动或重命名用户的 115 目录；用户必须填写已经存在的目标目录路径。
- 不因 `Stopped`、Redis TTL、解绑或套餐切换自动删除 playback 文件。
- 不把 115 下载 URL 返回到 API、Web、数据库、日志或可长期读取的 Redis 值中。

## 当前事实

- 当前版本化协议和系统内置链路分别见 [115 Cookie 播放兼容合同](../../reference/p115-cookie-playback-contract.md)、[Emby 4.9 系列播放代理 API 合同](../../reference/emby-playback-proxy-contract.md) 与 [115 Cookie 直连播放端到端流程参考](../../reference/p115-playback-end-to-end-flow.md)。
- `p115_accounts` 已同时表示管理员全局 `source|playback` 与用户个人 playback；owner 外键、非 revoked Provider UID/owner unique、共享启用角色 unique、字段 CHECK 和 revoked tombstone 由 `20260903_01` migration 维护。
- `directplay.Service` 已按用户有效套餐选择 personal/shared playback，并在 Redis 准入后才按精确账号 ID、owner 和 `updated_at` 加载凭证。
- Cookie 已使用 `CONFIG_ENCRYPTION_KEY` 的用途隔离派生密钥加密；创建、替换和验证状态机已有 fake、race 和 PostgreSQL 测试保护。
- `DetectCookieAppType` 已能从 Cookie 唯一 `UID` 的第二段 `ssoent` 识别已知客户端类型；当前 Cookie Provider 的生产请求不读取 `Credential.AppType` 选择端点或改变协议，它只作为账号诊断元数据保存和展示。
- 当前管理员账号控制面要求手工配置 Provider User-Agent，但计划固定的 `p115client` 提交对普通 Cookie/Web 请求默认使用 `Mozilla/5.0`；这能作为个人账号后端默认值的公开源码依据，尚未经过目标个人 Cookie 的真实 115 验证。
- 获取最终下载直链时使用 Gateway 当前视频请求携带的真实播放器 User-Agent，不使用账号 Provider User-Agent；秒传初始化继续使用协议代码内的版本绑定上传 UA。
- `CookieProvider.ResolveDirectoryByPath` 已能把已存在的根相对目录路径解析为唯一目录 ID，但不会创建目录。
- Gateway 已在身份门控和 Emby 成功响应后把 `Playing/Progress/Stopped` 映射到既有 115 反向 session，完成 active/paused 续租与 Stopped 释放；普通 Emby/local 会话不会创建租约。
- `playback_transfer_tasks` 已按 `playbackAccountId + SHA1 + size` 保存秒传任务与 provenance，并用 PostgreSQL advisory lock 防止相同目标文件重复秒传；它不是播放会话或用户配额表。
- 当前仓库已使用 `go-redis/v9`、miniredis/进程内 fake、`REDIS_URL` 和 gateway profile 的浮动 `redis:alpine` + AOF + volume；客户端不做版本探测，当前只支持单 Gateway。
- 套餐组 Emby 策略模板持久化 `SimultaneousStreamLimit`；API/Web 接受 `0..100`。个人账号保存值限制 `1..100` 并受正数套餐值约束，运行时取配置值与套餐值的较小值；共享账号不与单个套餐比较。
- 固定 Emby SDK/OpenAPI 只声明 `SimultaneousStreamLimit` 为整数，没有定义 `0` 的业务语义。本计划把 `0` 固定为 Ember 内部的“没有有限套餐上限”，不能扩写成 Emby 官方语义。
- 现有实机证据确认 Infuse `8.5.2` 的 Playing/Progress/Stopped 可经 Gateway 转发并取得 `204`，但没有固定“暂停后是否持续发送 Progress、多久发送一次”的客户端时序证据。

## 已确认决策

| 主题 | 决策 |
| --- | --- |
| 套餐归属 | 字段放在 `plan_groups`，不放在单个可购买的 `plans` 商品 |
| 默认账号来源 | 所有已有和新套餐组默认 `personal`；这是显式产品决策，不为历史共享直连回填 `system` 或增加兼容开关，只有管理员主动选择后才使用 `system` |
| `system` 语义 | `system` 只表示套餐路由到当前管理员共享 playback，不是 `p115_accounts` 的账号类型或 scope |
| `system` 路由 | 使用当前管理员 source + 管理员共享 playback；用户无需绑定 115 |
| 个人模式 | 使用管理员 source + 当前用户唯一的个人 playback 账号；未绑定或不可用时回退 Emby |
| 运行期账号加载 | 先读取不解密 Cookie 的精确 playback 路由元数据，完成 Redis 准入后再按账号 ID 加载凭证；个人与共享账号复用同一冷却/半开状态机 |
| 个人账号 `appType` | 用户不填写；已知 `ssoent` 自动映射，未知编码保存固定诊断值 `unknown`，缺失/重复/非法 `UID` 直接拒绝 Cookie |
| 个人账号 Provider UA | 用户不填写；验证、目录和查重等普通 Cookie/Web 请求固定使用代码常量 `Mozilla/5.0`，不新增配置项 |
| 最终直链 UA | 始终使用 Gateway 收到的真实播放器 User-Agent；不得用固定 Provider UA 替代 |
| 并发归属 | 最大播放路数属于具体 playback 账号；个人账号配置值受当前有效套餐模板约束，管理员共享 playback 是所有 `system` 用户合计的账号总上限，不与任一用户套餐上限比较 |
| 个人账号校验 | `SimultaneousStreamLimit > 0` 时要求 `1 <= maxConcurrentStreams <= SimultaneousStreamLimit`；等于 `0` 时只要求 `1..100`，其“无有限套餐上限”语义仅属于 Ember 内部合同 |
| 运行时有效值 | 个人账号使用 `effectiveMaxConcurrentStreams = min(configuredMaxConcurrentStreams, positive SimultaneousStreamLimit)`；套餐上限为 `0` 时有效值等于配置值，套餐降低不自动改写数据库配置 |
| 租约状态 | `reservation → active ↔ paused → stopped/expired`；302 前短预留与真实活跃播放必须分开，不能把 `HEAD` 或预加载直接算成正在播放 |
| 租约 TTL | 首期固定为 `reservation=30s`、`active=2m`、`paused=15m`，由 Playing/Progress 按状态续期；使用代码常量，不增加环境变量或后台配置 |
| 两组计数 | Redis 同时维护账号/用户的占用数和真实活跃数；占用数包含 `reservation + active + paused` 并用于准入，活跃数只包含 `active + paused` 并用于展示、归因和后续治理 |
| Redis 账号键 | 对规范化 `providerUserId` 使用服务端用途隔离 HMAC；不使用数据库账号 ID 或 Ember 用户 ID，不在 Redis 暴露原始 Provider UID |
| Redis 真相源 | Redis 可用且命令成功时，只按当前 Key 判断；Key 不存在就是零占用、零用量，重启或数据丢失后的计数重置是接受的结果，不做 epoch、恢复等待或数据库重建 |
| Redis 兼容与部署范围 | 不锁定 Redis 版本、不做版本探测，只使用 Lua/Sorted Set/TTL 等通用能力；首期只支持单 Gateway 进程，不设计多 Gateway、Redis Cluster 或跨主机时钟协调 |
| Redis 业务时间 | 租约 score、滚动小时和自然日边界都使用 Gateway 可注入时钟；自然日复用全局 `CRON_TIMEZONE`，不调用 Redis `TIME` |
| 实际门控 | 只用账号占用数执行账号有效上限；Redis 用户索引只用于展示、归因和后续治理，不参与第二套并发门控 |
| Emby 证据边界 | 不新增 Gateway 用户级总并发门控；Emby `SimultaneousStreamLimit` 能否限制 115 `302` 或本地文件分流播放仍未证实，不能把该假设写成当前保证 |
| 转存配额 | 小时/每日限额属于套餐组，默认每小时 `5`、每天 `10`；小时只接受 `1..100`，每日只接受 `1..1000`，`0` 非法，越界直接拒绝且不截断。两者不要求大小关系；按发起播放的 Ember 用户统计，只有缺失目标的新文件在秒传和目标复核都成功后才消耗一次，预存命中、重复请求和失败不消耗 |
| 转存预留 TTL | `pending reservation` 首期固定为 `5m` 代码常量且不续租；成功后以同一 `transferAttemptId` 幂等写入 succeeded，失败或确认预存命中时立即删除。pending 已过期但外部转存最终成功时仍必须记账，进程崩溃且没有成功结果时最多保留 5 分钟 |
| 成功记账失败 | 目标复核成功后使用独立 `2s` 总超时，以同一 `transferAttemptId` 有限重试 succeeded 提交；只有记账成功才继续 `302`。最终失败时保留外部文件和仍存在的 pending，本次公共 fallback，不新增数据库补偿或历史重建 |
| 会话存储 | 不建数据库会话表；Redis 处理 302 reservation、Playing/Progress/Stopped 状态晋级、暂停和 TTL |
| 失败策略 | 合法用户在账号、Redis、并发或配额不满足时进入已实现的公共 fallback；先查本地精确路径，未命中才到 Emby；身份和硬状态失败仍拒绝 |

## 方案设计

本文后续沿用的“fallback Emby”表示最终权威 Emby 分支。当前 [STRM 本地媒体回退播放实现方案](./strm-local-media-fallback.md) 已完成代码与自动化验证：账号、Redis、并发或配额失败应进入同一个 Gateway fallback 选择器，本地精确路径命中则直接播放，本地未命中才代理 Emby/CloudDrive2。`personal|system` 套餐路由选择、Redis 计数和转存配额本身不负责本地文件判断。

### 1. 用户可见行为

#### 1.1 套餐组

管理员在现有套餐组页面配置：

- `p115PlaybackMode`：`personal|system`，默认 `personal`。
- `p115TransferHourlyLimit`：每个用户滚动 60 分钟内允许成功创建的新 playback 文件数，默认 `5`，允许范围 `1..100`。
- `p115TransferDailyLimit`：每个用户在 `CRON_TIMEZONE` 自然日内允许成功创建的新 playback 文件数，默认 `10`，允许范围 `1..1000`。

套餐组不再配置 115 播放并发。套餐模式或转存配额更新只影响后续新请求，不撤销已签发的 115 CDN URL。

#### 1.2 普通用户

控制台新增独立“115 网盘”菜单。普通用户可以：

- 查看本人账号的脱敏状态、目标目录路径、配置最大播放路数、当前有效最大播放路数、有效套餐的 `SimultaneousStreamLimit`、当前账号/本人真实活跃数；短 reservation 单独显示为“准备中”，不混入正在播放。
- 填写或替换 Cookie；Cookie 使用密码输入语义、禁止自动填充，提交后立即清空，查询接口永不返回。页面不展示或要求填写 `appType/UserAgent`。
- 手工填写一个已经存在的 115 目标目录路径；后端解析并保存唯一 `targetParentId`，页面不要求用户获取内部 ID。
- 显式验证、启用、停用或解绑自己的账号。
- 设置本人 playback 账号的 `maxConcurrentStreams`：有效套餐上限为正数时只能填写 `1..SimultaneousStreamLimit`，为 `0` 时只能填写 `1..100`。
- 查看当前小时/每日成功转存用量和套餐组限额，但不能自行修改套餐额度。

个人账号固定为四步流转：只提交 Cookie 创建 `pending + disabled` 账号 → 显式验证为 `active + disabled` → 配置已有目标目录和最大播放路数 → 完整性检查通过后启用。页面按“待验证 / 待配置 / 可启用 / 已启用”表达下一步，不把有前置关系的操作平铺成一组无序按钮。

用户处于 `system` 套餐组时仍可提前绑定个人账号，但新播放只使用管理员共享 playback；切换到 `personal` 后无需重新录入。

#### 1.3 管理员共享账号

现有管理员 115 账号页面继续管理唯一全局 source 和唯一共享 playback 账号：

- source 配置 Cookie、Emby Path 前缀和 source 根目录，不展示无意义的最大播放路数。
- 共享 playback 配置 Cookie、已有目标目录路径和 `maxConcurrentStreams`。
- 共享 playback 的目录和最大播放路数通过独立原子配置动作一起提交；目录解析失败时两项都不写入，不允许形成 path、ID 或并发配置的半完成状态。
- 共享 playback 的准备中、真实活跃和总占用按该账号下由 `system` 路由建立的全部现存租约合计，不按用户分别重置账号上限，也不因用户随后切换套餐而重新归类历史租约。Redis 不可用时页面必须显示用量不可用，不能把故障伪装成零占用。

### 2. 数据与模型

#### 2.1 `plan_groups`

在现有模型与幂等 SQL migration 中增加：

- `p115_playback_mode VARCHAR(...) NOT NULL DEFAULT 'personal'`
- `p115_transfer_hourly_limit INTEGER NOT NULL DEFAULT 5`
- `p115_transfer_daily_limit INTEGER NOT NULL DEFAULT 10`

要求：

- `p115_playback_mode` 只接受 `personal|system`。
- 小时配额 SQL CHECK 固定为 `1..100`，每日配额固定为 `1..1000`；`0` 不表示禁用或无限，越界值不自动截断。每日配额不要求大于等于小时配额，因为滚动小时窗口和 `CRON_TIMEZONE` 自然日窗口并不对齐。
- migration 将全部已有套餐组回填为 `personal`，新建套餐组同样使用数据库/API 默认值 `personal`。不保留历史共享 playback 的隐式兼容语义：既有用户未绑定个人账号时会进入公共 fallback；只有管理员显式把指定套餐组改成 `system`，该组才恢复使用管理员共享 playback。这是已接受的用户可见路由变化，不增加 legacy 枚举、feature flag 或按名称猜测回填。
- Go 字段使用 CamelCase、JSON 使用 camelCase、GORM 显式指定 snake_case 列名。

#### 2.2 `p115_accounts`

复用现有表并增加：

- `owner_user_id`：可空并引用 `users(id) ON DELETE RESTRICT`；管理员维护的全局 source/共享 playback 为空，未解绑的个人 playback 必须等于其 Ember 用户 ID。该字段为空本身不能决定账号是共享账号，查询还必须结合 `status`；RESTRICT 保证任何绕过业务 Service 的用户删除都会失败关闭，而不是把活动个人账号静默变成共享候选。
- `target_parent_path`：规范化目录路径快照，只用于输入回显和诊断；`target_parent_id` 仍是运行时真相源。个人账号在验证后单独配置，两字段必须同时为空或同时非空。
- `max_concurrent_streams`：个人 `pending` 或尚未完成配置的 disabled 账号可为空；任何 `enabled` playback 必须为正整数。source 与 revoked 账号必须为空。
- `status` 增加不可逆终态 `revoked`；`system` 不进入账号状态枚举。
- `cookie_ciphertext` 改为可空，但只允许 `status=revoked` 时为空；Go 模型同步改为可空类型，所有非 revoked 凭证加载仍要求非空密文。
- `app_type` 对个人账号只保存后端派生结果：已知 `ssoent` 使用固定映射，未知编码保存 `unknown`；它不参与当前 Provider 路由或权限判断。为允许 revoked tombstone 擦除派生凭证元数据，模型和列同步改为可空；非 revoked 仍要求非空。
- `user_agent` 对个人账号由后端写入固定 `Mozilla/5.0`；现有管理员全局账号继续保留已配置值，避免改变既有 userspace。该列同样为 revoked 改为可空，非 revoked 仍要求非空。

个人账号字段状态矩阵固定为：

| 状态 | Cookie | Provider UID | 目标目录 path + ID | 最大路数 | `enabled` |
| --- | --- | --- | --- | --- | --- |
| `pending` | 必须 | 空 | 空 | 可空 | `false` |
| `active` 未配置完成 | 必须 | 必须 | 可成对为空 | 可空 | `false` |
| `active` 已启用 | 必须 | 必须 | 必须成对非空 | 必须 | `true` |
| `error/cooling_down` | 必须 | 必须 | 保留 | 保留 | 保留当前启用意图 |
| `expired` | 保留 | 保留 | 保留 | 保留 | `false` |
| `revoked` | 空 | 空 | 空 | 空 | `false` |

需要重写现有 partial unique/约束：

- 管理员共享范围只能启用一个 source 和一个 playback；两者均要求 `owner_user_id IS NULL AND status <> 'revoked'`。
- 每个 `owner_user_id` 只能拥有一个 `status <> 'revoked'` 的个人 playback；历史 revoked tombstone 不阻止同一用户重新绑定新账号。
- 同一非空 `provider_user_id` 不能被任一非 revoked 的管理员共享账号或个人账号重复绑定。
- 普通用户只能创建 `playback`，不能创建 source、指定其他 owner 或修改管理员共享账号。
- 数据库 CHECK 要求 `owner_user_id IS NOT NULL` 的账号只能是非 revoked playback；`status=revoked` 时必须同时满足 `enabled=false`，且 owner、Cookie、Provider、`app_type/user_agent`、source/playback 目录、最大路数、验证/成功时间、冷却和错误字段全部为空。
- 用户个人 playback 与管理员 source 的 Provider UID 必须不同，禁止借 source 账号签发最终直链。
- 所有 owner、状态、凭证、目录与并发约束都必须在 SQL migration 中重写，并由 GORM 字段显式映射；不能只在 Service 层约定。
- SQL CHECK 固定 source/revoked 的 `max_concurrent_streams` 必须为空，个人目录 path/ID 必须同空或同时非空，任何 `enabled` playback 必须具备完整凭证、Provider UID、目标目录和正整数并发配置。个人账号的 `1..100` 和有效套餐跨表上限由 Service 在并发更新、启用和运行时解析，不能用无法表达跨表语义的 CHECK 或触发器伪装完成。

##### 解绑与用户删除

`DELETE /api/v1/user/p115-account` 的语义是“撤销并擦除凭证”，不是物理删除账号行：

1. 事务内按 `owner_user_id` 锁定当前非 revoked 个人 playback；不存在时按幂等成功返回。
2. 原子写入 `status=revoked + enabled=false`，并清空 `owner_user_id/cookie_ciphertext/provider_user_id/app_type/user_agent/emby_path_prefix/source_root_id/target_parent_id/target_parent_path/max_concurrent_streams`、验证/成功时间、冷却和错误字段。解绑不再派生 Redis cleanup handle；它只停止新的 DirectPlay 凭证加载和 `302` 签发，不伪装已签发 CDN URL 已被撤销。
3. 保留账号 `id/role/alias/auth_mode/status/enabled/created_at/updated_at`，让既有 `playback_transfer_tasks` 继续通过 `ON DELETE RESTRICT` 引用同一个账号 ID；不删除 transfer provenance，也不调用 `DeleteFile`。个人账号 alias 使用后端固定值，不保留用户自由输入。
4. revoked 是不可复活终态：验证、启停、Cookie/目录/并发更新和 DirectPlay 加载全部拒绝。用户重新绑定必须创建新的账号 ID，不能让新凭证继承旧任务历史。
5. 已存在的 Redis `reservation|active|paused`、account/user 索引和反向 session 不因解绑删除；已签发 CDN URL 仍可能播放，所以它们必须继续表示真实占用，由成功 `Stopped` 或各自 TTL 收口。同一 Provider UID 重绑后生成同一 `playbackAccountKey`，新请求必须继续受旧占用限制；不同 Provider UID 不继承旧账号占用。

管理员删除 Ember 用户时，顺序固定为：本地 Gateway Token 撤销 → 个人 115 账号 tombstone（同一事务清空 owner）→ 删除 Emby 用户 → 删除 Ember 用户。Token 撤销或 tombstone 失败时不得继续外部/本地用户删除；Token 撤销后旧播放事件不能再通过身份门控刷新租约，已有 Redis 占用等待 TTL 自然收口。`owner_user_id ON DELETE RESTRICT` 作为数据库兜底，保证仍有关联的非 revoked 个人账号时用户行不能被直接删除；完成 tombstone 后用户删除不再触碰历史账号行。Emby 删除失败时保留已经完成的本地 Token 撤销和账号凭证擦除，不回滚安全结果。

#### 2.3 保持不变的数据

- `playback_transfer_tasks` 继续以 `ON DELETE RESTRICT` 引用 source/playback 账号并记录目标文件和秒传 provenance；个人账号解绑后引用指向 revoked tombstone，不新增用户当前播放数字段。
- `emby_access_tokens` 继续只保存 purpose 隔离的 HMAC 摘要。
- 不创建 `direct_play_sessions`、用户转存计数表或每日统计表。

### 3. Redis 数据边界

#### 3.1 运行要求

- 新增 Redis 客户端边界和 `REDIS_URL`；Cookie、Emby Token、完整 SHA1、下载 URL 和 Provider 原始响应不得进入连接串示例、Key 或 Value。
- Compose 的 `gateway` profile 增加不固定版本标签的 Redis 服务、healthcheck、持久卷和 AOF；允许部署者用外部 `REDIS_URL` 覆盖。代码不执行 `INFO` 版本门控，持久化只用于尽量保留计数，不引入额外恢复协议。
- Redis 可用且命令成功时，当前数据就是唯一真相源；不存在的 Key 按零占用、零用量处理。Redis 不可用、超时或命令失败时 Gateway 仍可代理 Emby，但禁止签发新的 115 `302`，固定 fallback Emby。
- 首期部署合同只允许一个 Gateway 进程消费这些 Redis Key；不声明多 Gateway 准入、跨主机时钟或 Redis Cluster 兼容。租约和配额脚本所有 `nowMs/dayStartMs/dayEndMs` 由 Gateway 同一可注入时钟计算，日边界使用全局 `CRON_TIMEZONE`。
- Redis 脚本使用固定参数化实现，所有 Key 通过 `KEYS` 传入；客户端必须处理脚本缓存丢失并重新加载。
- 账号/用户活跃索引和反向会话映射必须在一个 Redis 原子操作中更新，禁止分别 `INCR/DECR` 造成漂移。

Redis 官方合同依据：Lua 脚本在服务端原子执行，并允许跨多个 Key 进行条件更新；脚本缓存是易失的，客户端必须处理重启后的重新加载。Sorted Set 的唯一 member、score 和范围删除语义用于实现带到期时间的当前播放索引与滑动窗口限流。不锁定服务端版本；实际部署不支持脚本使用的通用命令时按 `redis_unavailable` fallback，不再另建版本分支。参考 [Redis Lua scripting](https://redis.io/docs/latest/develop/programmability/eval-intro/)、[Redis sorted sets](https://redis.io/docs/latest/develop/data-types/sorted-sets/) 和 [`INCR` rate limiter pattern](https://redis.io/docs/latest/commands/incr/)。

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
- `playbackAccountKey = lowerHex(HMAC-SHA256(K_p115_playback_account, canonicalProviderUserId))`。`K_p115_playback_account` 使用现有 `CONFIG_ENCRYPTION_KEY` 经 `tokenhash` 用途隔离机制派生，固定 purpose 为 `p115-playback-account-key`，不新增密钥配置；`canonicalProviderUserId` 使用账号验证后保存的规范十进制 Provider UID。
- 该 Key 只标识真实 115 playback 账号，不混入 `p115_accounts.id`、owner 或 Ember `userId`。同一 115 账号解绑后以新数据库 ID 重新绑定，必须命中旧租约；不同 115 账号必须生成不同 Key。Redis Key、日志和 API 都不得出现原始 Provider UID 或 HMAC 输入。
- Sorted Set 的 score 只表示业务到期，member 不会因 score 过期自动消失。因此每次租约脚本都必须先以 Gateway `nowMs` 对本次涉及的 account/user `leases + active` 执行 `ZREMRANGEBYSCORE`，统计时只计算 `score > nowMs` 的 member，禁止直接把未清理的 `ZCARD` 当成有效占用。
- account/user `leases + active` 每次写入或续租后都将整个 Key 的物理 TTL 重置为 `16m`（最长 paused `15m` + `1m` 回收余量）；reverse session 使用与当前状态相同的 `30s/2m/15m` TTL。这些物理 TTL 只防止无后续请求的空闲 Key 永久残留，有效占用仍以 member score 为准。

`GET` 原子预留流程：

1. 只有身份、PlaybackInfo 证明和静态原文件条件完整的 `GET` 才能申请新预留；首次实际播放、带 Range 的 GET 或预加载 GET 在网关侧无法可靠区分，因此都最多形成同 session 的一个短 reservation，不能直接形成 active。`HEAD` 和会话事件不能创建新预留。
2. 使用同一 Gateway `nowMs` 清理 account/user `leases` 与 `active` 索引中 `score <= nowMs` 的 member，后续占用统计只计算 `score > nowMs`。
3. 如果同一 session 已存在，只复用既有 `reservation|active|paused`，不重复计数，也不因视频重试把 `active|paused` 降级成 reservation。
4. 取得本次选中账号的运行时上限：个人账号使用当前有效套餐模板计算 `effectiveMaxConcurrentStreams`，管理员共享 playback 直接使用自身配置上限；账号 `leases` 占用数已满则返回 `account_concurrency_exceeded`。套餐组或模板无法解析时不得猜默认值，也不得申请 reservation。
5. 未满时同时写入 account/user `leases` 和反向 session，状态为 `reservation`，TTL 固定 `30s`；此时不写 `active` 索引。
6. DirectPlay 未能生成安全候选时立即原子释放本次 `reservation`；既有 `active|paused` 不能因一次重新签发失败被释放。成功返回 `302` 后只保留 `reservation`，等待成功的播放事件晋级。

`HEAD` 处理固定为：已有同 session 的 `reservation|active|paused` 时可以复用并继续执行直链候选；没有既有租约时不创建新租约、不触发新的 115 DirectPlay，直接进入公共 fallback。

事件语义：

- 只有会话事件成功转发给 Emby，且反向 session 已证明该会话取得过 115 `reservation|active|paused`，才允许更新 Redis；普通 Emby/local fallback 会话不能借事件创建 115 租约。
- `Playing`：把已有 `reservation|paused` 晋级/恢复为 `active`，或续租已有 `active`；同时写入 account/user `active` 索引并使用固定 `2m` active TTL。
- `Progress + IsPaused=false`：把已有 `reservation|paused` 晋级/恢复为 `active`，或续租已有 `active`，并刷新固定 `2m` active TTL。
- `Progress + IsPaused=true`：把已有 `reservation|active` 晋级/切换为 `paused`，继续计入 `leases` 与 `active`，并刷新固定 `15m` paused TTL；暂停不能按 `Stopped` 删除。
- `Stopped`：只有请求成功转发给 Emby 后，才同时删除 account/user `leases`、`active` 和反向 session。
- 找不到反向 session 的 Playing/Progress/Stopped 不创建或猜测 playback 账号租约，只记录固定观察结果；无后续事件时，member 先按 score 不再计入有效占用，索引 Key 最迟按 `16m` 物理 TTL 删除，不依赖数据库 cron。

首期业务 TTL 固定为 `reservation=30s`、`active=2m`、`paused=15m`，并作为单 Gateway 进程中的同一套代码常量；不增加环境变量或后台配置。这些是保守初值，不扩写为已由真实客户端证明的最优值；后续只有取得目标客户端的请求与事件间隔证据后才能修改，并同步测试和稳定文档。reservation 只承担 302 签发前后的并发防穿透，不展示为正在播放；暂停期间继续占用一路，避免恢复旧连接时突破账号上限。

#### 3.3 用户转存配额

至少维护：

```text
{p115}:transfer:pending:{userId}
{p115}:transfer:succeeded:{userId}
```

- pending 与 succeeded 的 member 使用同一个服务端生成、不含用户输入和 Provider 标识的 opaque `transferAttemptId`；同一尝试的完成重试必须复用该 ID，禁止因重试重复计数。该 ID 只用于 Redis 原子幂等，不返回 API，也不写日志。
- 小时窗口使用滚动 60 分钟；每日窗口使用 `CRON_TIMEZONE` 的自然日边界。
- pending/succeeded 同样使用 Gateway 时钟的毫秒时间作为 score，每次脚本先删除不再影响当前 pending、滚动小时或当日窗口的 member。pending Key 每次写入后设置 `6m` 物理 TTL；succeeded Key 设置为 `max(距下一个 CRON_TIMEZONE 自然日边界, 60m) + 1m` 物理 TTL，使无后续请求的用户 Key 也能回收。
- 配额适用于 `personal` 和 `system` 两种模式，按发起请求的 Ember 用户统计。
- 只有目标 playback 账号原本缺少文件、秒传成功且目标目录复核通过，才消耗一次成功额度。
- 目标预存命中、重复 HEAD/GET/Range、重新签发下载 URL和失败尝试不消耗成功额度。
- 管理员共享 playback 中由其他用户先前转存的文件如果已命中，本次用户不消耗额度。

为防止并发穿透，配额必须在现有 transfer lock 内的第二次目标查重仍未命中后、调用 `InitRapidUpload` 前原子预留：

1. 同时清理过期 pending/succeeded 事件并计算小时、每日已用量。
2. 任一窗口达到套餐组上限时返回 `transfer_quota_exceeded`，不调用 115。
3. 未达到上限时写入固定 `5m` TTL 的 pending reservation；TTL 使用代码常量且执行期间不续租。
4. 目标复核成功后，用同一 `transferAttemptId` 原子删除 pending 并以 NX 语义写入 succeeded；重复完成只能保留一条 succeeded。pending 已因 `5m` TTL 过期时仍写入 succeeded，并返回内部诊断结果 `transfer_pending_expired_before_commit`，不能让已经成功创建的新文件永久漏计。
5. 进程异常退出时 pending 最多保留 `5m` 后自动过期，不能永久占用额度；正常链路应远短于该时间，超过 `5m` 视为异常并记录固定诊断结果。

晚到成功可能在 pending 过期到 succeeded 写入之间短暂放出一个配额名额，这是外部 115 副作用与 Redis 无法形成单事务时接受的异常窗口；succeeded 落地后，后续请求立即按新用量判断。不得为消除该窗口而删除已创建文件、伪造账号健康错误或恢复数据库配额真相源。

目标复核成功后的 succeeded 提交使用独立于客户端请求取消的 `2s` 总超时，并在该总预算内以同一 `transferAttemptId` 有限重试瞬时 Redis 错误；幂等 NX 语义保证重试不会重复计数。只有 succeeded 确认落地后才能继续签发本次 `302`。总预算耗尽后固定返回 `transfer_quota_commit_failed`：不删除已经创建的 115 文件、不主动删除仍存在的 pending、不污染账号健康，本次进入公共 fallback；pending 在 Redis 可用且仍存在时按原 `5m` TTL 自然释放。

该失败不创建 PostgreSQL 补偿任务，不从 `playback_transfer_tasks` 回放或重建 Redis。后续请求若将文件识别为预存命中，可能不再补计本次成功转存；这是“Redis 当前数据是唯一真相、数据丢失允许计数重置”合同在外部副作用边界上的已接受结果，必须通过固定日志暴露，不能伪装成成功记账。

`transfer_quota_exceeded`、Redis 不可用和账号并发已满都是 Gateway 加速资格结果，不是 Provider 健康错误：不能把账号写成 `expired/error/cooling_down`。

### 4. API 与权限边界

列表接口统一返回 `data`，字段使用 camelCase。Cookie 只出现在创建或替换请求。

#### 4.1 套餐组

复用现有管理员套餐组接口：

- `POST /api/v1/admin/plan-groups`
- `PUT /api/v1/admin/plan-groups/:key`
- `GET /api/v1/admin/plan-groups`

请求/响应增加 `p115PlaybackMode`、`p115TransferHourlyLimit`、`p115TransferDailyLimit`。创建默认值和 migration 默认值必须一致；API 对小时 `1..100`、每日 `1..1000` 强制校验，任一越界时整次创建/更新失败且不写数据库，不允许静默截断或只更新另一字段。

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

`POST /api/v1/user/p115-account` 的请求 DTO 精确为 `{cookie}`，不接受 alias、role、owner、目录、并发、`appType` 或 `userAgent`。后端将 owner 固定为当前 Principal.User.ID、role 固定为 `playback`、alias 固定为服务端值，再从 Cookie 解析唯一合法 `UID`：已知 `ssoent` 写入映射后的 `app_type`，未知 `ssoent` 写入 `unknown`，缺失、重复或非法 UID 返回参数错误。`user_agent` 固定写入 `Mozilla/5.0`，Cookie 加密后以 `pending + disabled` 创建，不请求 115，也不读取套餐模板。已有非 revoked 个人账号时返回 `409`，不覆盖原账号。

`PUT /api/v1/user/p115-account/cookie` 的请求同样只有 `{cookie}`。替换成功后回到 `pending + disabled`，清空 Provider UID、`targetParentPath/targetParentId`、验证/成功时间、冷却和错误字段，并重新派生 `app_type` 与固定 `user_agent`。`maxConcurrentStreams` 是 Ember 账号配置，可保留，但下次启用时必须重新按当前套餐校验。旧目录 ID 属于旧 Provider 账号，禁止跨 Cookie 沿用。

`POST /api/v1/user/p115-account/validate` 只使用已保存的凭证；成功写入规范 Provider UID 并进入 `active + disabled`，不自动配置目录、并发或启用。Provider UID 的跨管理员/个人账号唯一约束在验证回写时生效，冲突返回 `409`。

账号摘要只返回脱敏 Provider 标识、状态、启用状态、目录路径快照、配置值 `maxConcurrentStreams`、运行时值 `effectiveMaxConcurrentStreams`、有效套餐值 `simultaneousStreamLimit`、账号/用户的 `reservedStreams`、`activeStreams`、`occupiedStreams` 和小时/每日配额用量；其中 `occupiedStreams = reservedStreams + activeStreams`，`activeStreams` 已包含 paused。`SimultaneousStreamLimit=0` 时 `effectiveMaxConcurrentStreams` 等于配置值，不能返回 `0` 伪装为账号无限制。摘要显式返回 `usageAvailable`：Redis 可用且查询成功但 Key 不存在时为 `true` 且计数返回零；Redis 不可用或查询失败时为 `false` 且所有 Redis 用量计数字段返回 `null`，不能伪装成成功读取到的零。

`PUT /api/v1/user/p115-account/concurrency` 只在当前账号非 revoked 时允许调用，并必须通过现有有效套餐解析器读取当前用户的套餐组及其 Emby 策略模板，再按上述区间强制校验。套餐组缺失、默认套餐组缺失或模板读取失败时，接口失败关闭且不得写入账号配置；禁止回退到模型默认值、当前 Emby 用户字段或硬编码值。摘要读取同样不得在模板不可用时猜测 `effectiveMaxConcurrentStreams`。

目录接口只允许当前账号已显式验证为 `active`，并接收用户输入的已有目录路径；使用本人当前加密凭证调用 `ResolveDirectoryByPath`，成功后在同一次更新中保存规范化 `targetParentPath` 快照和唯一 `targetParentId`。禁止只保存 path、使用根目录兜底、自动创建目录或返回 Provider 原始目录响应。

`PUT /api/v1/user/p115-account/enabled` 将 `enabled=true` 时必须在行锁内重新检查：账号仍属于当前用户、`status=active`、Cookie 密文与 Provider UID 存在、`targetParentPath/targetParentId` 成对非空、`maxConcurrentStreams` 已配置，且当前有效套餐校验仍通过。缺少或越界返回 `409` 并保持 disabled，不自动补默认目录或并发值；设置 `enabled=false` 不受当前验证状态限制。

#### 4.3 管理员共享账号

现有 `/api/v1/admin/p115-accounts` 合同继续只管理 `owner_user_id IS NULL AND status <> revoked` 的管理员全局 source/共享 playback。source 请求必须拒绝并发字段，playback 请求必须拒绝 source 位置字段；解绑时已清空 owner 的 revoked tombstone 不进入该控制面。

新增 `PUT /api/v1/admin/p115-accounts/:id/playback-config`，请求 DTO 精确为：

```json
{
  "targetParentPath": "/existing/path",
  "maxConcurrentStreams": 3
}
```

两个字段都必填并执行完整替换，不支持部分更新。接口只接受 `owner_user_id IS NULL + role=playback + status=active` 的非 revoked 管理员共享账号；`maxConcurrentStreams` 必须为正整数，但不与任一用户套餐上限比较。后端先读取账号版本及当前凭证并调用 `ResolveDirectoryByPath`，解析失败时整次请求失败且不得写入任何字段；成功后以账号版本、凭证和状态未变化为条件，在同一数据库更新中保存规范化 `targetParentPath`、唯一 `targetParentId` 和 `maxConcurrentStreams`。解析期间若发生 Cookie 替换、状态变化或其他账号更新，条件更新失败并返回 `409`，禁止把旧凭证解析出的目录 ID 写给新账号状态。

已启用的共享 playback 允许调用该接口。新上限低于 Redis 当前 `occupiedStreams` 时仍保存配置，不中断既有播放、不删除租约；后续 reservation 按新上限失败关闭，直到占用降到新上限以下。该更新只影响新请求，不能宣称撤销已经签发的 CDN URL。

现有管理员列表和详情响应直接增加共享 playback 用量，不新增独立用量端点。playback 摘要固定返回 `usageAvailable`、`reservedStreams`、`activeStreams`、`occupiedStreams`，统计该共享账号下由 `system` 路由建立的全部现存租约，不按用户当前套餐重新归类，且 `occupiedStreams = reservedStreams + activeStreams`、`activeStreams` 已包含 paused。Redis 查询成功但 Key 不存在时返回 `usageAvailable=true` 和三个零；Redis 不可用或查询失败时返回 `usageAvailable=false`，三个计数字段均为 `null`。source 摘要不返回这些 playback 专属字段。

管理员控制面现有 `appType` 未识别时人工兜底和显式 `userAgent` 输入保持兼容；本计划只简化普通用户个人账号，不借机改变已落地的管理员请求合同。

### 5. DirectPlay 账号选择

每次视频候选在调用 Provider 前：

1. 使用已经解析的 Principal 实时读取用户有效套餐组；`users.plan_group IS NULL` 时继续遵守默认套餐组语义。
2. `p115PlaybackMode=system`：选中唯一的管理员共享 playback，路由元数据查询要求 `owner_user_id IS NULL + enabled=true + status<>revoked`；`system` 只是本次路由模式。
3. `p115PlaybackMode=personal`：按 Principal.User.ID 选中本人唯一、非 revoked 且已启用的 personal playback。路由元数据只包含账号 ID、owner、规范 Provider UID、目标目录、并发配置、状态、冷却时间和用于竞态校验的版本字段；不读取或解密 Cookie。
4. 未绑定、未验证、停用、`pending/error/expired/revoked` 直接返回类型化不适用原因并 fallback。`cooling_down` 未到期时同样 fallback；已到期时只标记为可申请半开探测，此时仍不解密 Cookie。
5. source 始终使用管理员维护的全局 source；source 与选中 playback 的 Provider UID 相同则拒绝加速。
6. `GET` 使用选中 playback 元数据申请 Redis `reservation`；个人账号先按当前有效套餐模板计算 `effectiveMaxConcurrentStreams`，共享账号直接使用自身配置上限。账号占用已满、套餐模板不可用或 Redis 不可用时 fallback，不申请半开探测。`HEAD` 只有命中同 session 既有租约才继续，否则直接 fallback。
7. Redis 准入后按步骤 2/3 选定的精确 account ID 和 owner 加载运行期凭证。`active` 直接可用；已到期 `cooling_down` 在 PostgreSQL 行锁内将 `cooldown_until` 续租 1 分钟并只放行一个半开探测，其他并发请求 fallback。账号在两次读取之间被停用、解绑、替换 Cookie 或更新时失败关闭，并只释放本次新建的 reservation，不释放同 session 已有 `active|paused`。
8. 目标查重命中则直接签发新下载 URL，不消费转存配额；缺失时进入 transfer lock 和 Redis 配额预留。
9. 只有全部合同成立才返回空体 `302`；任一步失败均保持现有公共 fallback。Provider 结果继续通过现有 Cookie 密文 + `updated_at` 乐观并发保护回写账号健康：半开成功恢复 `active`，失败按类型重新冷却、进入 `expired` 或 `error`。

套餐模式改变、个人账号停用、账号配置值调低或有效套餐的 `SimultaneousStreamLimit` 调低时不撤销已签发链接，也不删除文件；只影响新租约。套餐降低不会自动改写数据库中的 `max_concurrent_streams`，运行时通过 `effectiveMaxConcurrentStreams` 立即收紧新准入；当前占用数不低于有效值时，新 115 播放持续 fallback，直到 `reservation + active + paused` 占用数低于有效上限。套餐上限随后调高或改为 `0` 时，原配置值继续生效。

### 6. 日志与可观察性

视频最终决策日志继续保持每请求一条，并补充固定字段：

- `playbackMode=personal|system`
- `playbackAccountOwner=shared|current_user`
- `accountActiveStreams`、`accountConfiguredStreamLimit`、`accountEffectiveStreamLimit`
- `simultaneousStreamLimit`（只在个人账号路由中记录当前有效套餐值；`0` 保留原值）
- `userActiveStreams`
- `accountReservedStreams`、`accountOccupiedStreams`
- `userReservedStreams`、`userOccupiedStreams`
- `transferHourlyUsed/Limit`、`transferDailyUsed/Limit`（只有进入转存配额判断时记录）
- `reasonCode=personal_account_missing|account_concurrency_exceeded|transfer_quota_exceeded|transfer_quota_commit_failed|redis_unavailable`
- 晚到成功额外记录固定诊断码 `transfer_pending_expired_before_commit`，但最终播放决策仍由 succeeded 记账和后续直链结果决定，不能把它映射为 Provider/账号健康错误。

禁止记录 Cookie、Token、完整 SHA1、下载 URL、Redis 连接串、原始 PlaySessionId、Lua 参数原文或 Provider 原始错误。Redis Key 日志只允许固定模板名，不打印完整实例 Key。

## 失败路径与边界条件

- 用户是 `personal` 但未绑定账号：fallback Emby，不自动借用管理员共享 playback。
- 升级 migration 将历史套餐组统一设为 `personal`：既有用户未绑定个人账号时同样 fallback，不因升级前曾使用共享 playback 而隐式保留 `system`。
- 用户是 `system`：忽略个人账号，使用管理员共享 playback；共享账号不可用时 fallback Emby。
- 个人或共享 playback 处于未到期 `cooling_down`：fallback；冷却到期后只有拿到 PostgreSQL 半开探测租约的一个请求可以读取 Cookie 和调用 Provider，其他请求继续 fallback。
- Redis 不可用、超时或脚本失败：fallback Emby；不能伪装成成功读取到的零活跃数，也不能污染账号健康。
- 个人账号 `reservation + active + paused` 占用数达到 `effectiveMaxConcurrentStreams`，或共享账号占用数达到自身 `maxConcurrentStreams`：fallback Emby；不拒绝用户，不撤销 Token。
- 个人账号创建只写入 Cookie 派生的 `pending + disabled` 记录，不读取套餐。并发配置或启用时不满足当前有效套餐区间：返回参数/状态错误且不写数据库；套餐组或策略模板读取失败：失败关闭且不猜默认值。
- 个人账号尚未验证就配置目录，或尚缺目录/并发就请求启用：返回 `409` 并保持原状态，不调用无关 Provider 方法。
- 个人账号替换 Cookie：清空旧 Provider UID 和目标目录 path/ID，回到 `pending + disabled`；保留的 `maxConcurrentStreams` 只是待重新启用时复验的 Ember 配置，不代表新 Cookie 已可运行。
- 套餐模板在账号保存后降低：保留原 `maxConcurrentStreams` 配置值，运行时按较小的 `effectiveMaxConcurrentStreams` 准入；管理员共享 playback 不参与该比较。
- 任一 account/user `leases` 或 `active` 索引写入失败：整个原子操作失败并 fallback，禁止只更新部分索引。
- `HEAD` 没有同 session 既有租约：不创建 reservation，不触发新的 115 DirectPlay，进入公共 fallback。
- 302 后没有成功 Playing/Progress：reservation 最多保留 `30s` 后自然过期，不能长期显示为真实活跃播放。
- `IsPaused=true`：继续占用并刷新 `15m` paused TTL；不能删除索引。
- Stopped 上游失败：不假装停止成功，等待后续事件或 TTL 收口。
- 转存配额达到小时或每日上限：不调用 `InitRapidUpload`，fallback Emby。
- 套餐组提交的小时配额不在 `1..100` 或每日配额不在 `1..1000`：返回参数错误且整次写入失败；不截断、不赋予 `0` 特殊语义，也不强制两个字段的大小关系。
- 转存失败：释放 pending；Provider 类型化错误仍按现有账号健康合同处理。
- pending 已过期后目标复核成功：使用原 `transferAttemptId` 幂等写入 succeeded，记录 `transfer_pending_expired_before_commit`，不删除 115 文件、不污染账号健康；记账成功后继续当前直链流程。
- 目标复核成功但 succeeded 在独立 `2s` 总预算内仍无法写入：保留已创建文件和仍存在的 pending，记录 `transfer_quota_commit_failed`，本次进入公共 fallback；不签发 `302`、不污染账号健康、不创建数据库补偿或历史重建。
- 目标文件已经存在：不消耗转存额度，只申请播放租约并获取新直链。
- Redis 重启或数据丢失：恢复连接后只按 Redis 当前数据继续判断；缺失 Key 按零占用、零用量处理，允许会话和转存计数随 Redis 数据一起重置。不增加 epoch、恢复等待、历史重建或数据库补偿。
- 用户解绑：只把个人账号原子写成 revoked tombstone 并擦除凭证，不删除现有 Redis 租约或反向 session；旧 CDN URL 仍可能播放，现有占用必须继续由 `Stopped` 或 TTL 如实收口。
- 用户解绑后重新绑定：必须创建新账号 ID；旧 transfer 继续引用 revoked tombstone，不能改挂到新凭证。同一 Provider UID 生成同一 `playbackAccountKey` 并继续命中旧占用，不同 Provider UID 不继承。
- 管理员删除用户：Token 撤销或个人账号 tombstone 失败时不调用 Emby 删除；不主动删除 Redis 租约，Token 撤销使旧事件无法继续刷新，现有占用按 TTL 收口。

## 影响范围

- API：扩展套餐组和 115 账号模型，新增用户账号/用量接口、管理员共享 playback 原子配置与用量字段、账号选择器、Redis 租约和配额 Service。
- Gateway：在 DirectPlay 前后申请/释放 Redis 租约，旁路消费成功的 Playing/Progress/Stopped。
- Web：套餐组页面增加账号来源和转存配额；新增普通用户“115 网盘”页面；管理员共享 playback 增加路径和最大路数。
- Bot：本次无改动。
- 数据库：只修改长期配置模型与 SQL migration，不新增播放会话或配额使用表。
- 配置/部署：新增 Redis 客户端、`REDIS_URL`、Compose Redis、healthcheck、AOF、数据卷和故障说明。
- 文档：实现时同步系统架构、配置参考、数据模型、API 目录、Web 信息架构、部署和测试 runbook。

前端实现必须遵守 Ember 风格；设计与交互基线以 [Web 设计规范](../../reference/web-design-guide.md) 为准。当前没有偏离规范的特例。页面应通过字段、状态和动作表达能力，不堆叠解释性文案。

## 验证方式

### 自动化验证

- Go TDD：套餐模式默认/更新、转存配额默认 `5/10`、小时 `1..100`、每日 `1..1000`、边界内任意大小关系、任一越界时整次拒绝且不写入、个人账号所有权、只含 Cookie 的创建/替换 DTO、创建不请求 115 或套餐模板、`pending → active → 配置 → enabled` 前置状态、已知 `ssoent` 自动识别、未知编码写 `unknown`、缺失/重复/非法 UID 拒绝、固定 Provider UA、真实播放器 UA 隔离、未验证目录调用拒绝、目录 path/ID 原子成对更新、Cookie 替换清理旧目录但保留待复验并发配置、启用前完整性/套餐复验、revoked 擦除字段矩阵、唯一约束、账号选择、两段式路由元数据/凭证加载、个人与共享账号的未到期冷却阻断、到期冷却并发单探测、半开成功/失败回写、Redis 准入失败不消耗半开租约、账号竞态变化后释放本次新 reservation 和所有 fallback 原因；覆盖管理员共享 playback 配置 DTO 拒绝缺字段/source/非 active/revoked 账号，目录解析失败零写入，成功时 path/ID/并发原子更新，解析期间 Cookie/状态/版本变化返回 `409` 且旧目录不落库，降低上限不清理既有租约；覆盖 `SimultaneousStreamLimit` 为 `0/1/100`、正数上下界、越界拒绝、默认套餐解析、套餐/模板缺失时不写入，以及响应同时返回配置值、有效值和套餐值。
- 账号键与解绑测试：同一规范 Provider UID 在不同数据库账号 ID、owner 和解绑重绑前后生成相同 `playbackAccountKey`，不同 Provider UID 生成不同 Key；解绑不删除已有 account/user `leases + active` 或反向 session，同 UID 重绑继续受旧占用限制，不同 UID 不继承；Redis Key、日志和响应不包含原始 Provider UID，purpose 变化时摘要必须不同。
- Redis adapter/fake 测试：单 Gateway 内 GET 原子 reservation、leases/active 两组索引一致性、缺失 Key 按零、重复 session、配置/有效并发上限、固定 `30s/2m/15m` 业务 TTL、状态晋级、Stopped、脚本缓存丢失、连接失败和取消；用可注入 Gateway 时钟覆盖过期 member 不再计入、统计不依赖残留 `ZCARD`、reverse session 状态 TTL、leases/active 空闲 Key `16m` 物理回收；个人账号按有效值准入，共享账号按自身配置值和所有 `system` 用户合计准入，用户索引不形成第二套门控。覆盖管理员/用户摘要在查询成功、Key 缺失和 Redis 故障时分别返回正确的 `usageAvailable`、零值或 `null`，不得把故障伪装成零。连接或通用脚本命令错误必须 fallback，重新连接后的空数据必须按零重新开始，不执行版本分支、恢复等待或历史重建，不声明多 Gateway/Cluster 兼容。默认验证不得启动项目服务或访问真实 Redis 云服务。
- 转存配额测试：基于同一可注入 Gateway 时钟的滚动小时窗口、`CRON_TIMEZONE` 自然日、并发预留、成功提交、失败退款、固定 `5m` pending TTL、不续租、进程崩溃后到期释放、pending Key `6m` 物理回收、succeeded Key 跨滚动小时/自然日的最长需要时间后回收、预存不计数、system/personal 一致口径；覆盖 pending 存在时正常转换、pending 过期后的晚到成功仍以同一 `transferAttemptId` 写入一次 succeeded、重复完成不重复计数、后续请求看到新用量且账号健康不变。另覆盖独立 `2s` 总预算、客户端取消后仍可完成、瞬时失败重试成功、预算耗尽后不签发 `302`、文件和 pending 不被主动删除、固定诊断码以及无数据库补偿调用。
- Gateway fake Emby/115 测试：HEAD 无租约不创建且直接 fallback、HEAD 命中既有租约可复用、重复 GET 不重复计数、预加载只形成短 reservation；Playing/Progress/Stopped 请求与响应透明，只有成功事件且命中反向 session 才更新 Redis；套餐降低后不改数据库配置但立即使用更小有效值，套餐上限为 `0` 时恢复配置值，模板解析失败以及 Redis/配额/账号失败均回退 fake Emby。
- PostgreSQL 集成测试：migration 幂等、全部历史套餐组回填 `personal`、新建套餐组默认 `personal`、重复执行不覆盖管理员已显式修改的 `system`、管理员共享/个人账号 partial unique、所有权隔离、个人 pending/未配置 active/已启用 active/revoked 字段矩阵、目录 path/ID 同空或同非空约束、启用 playback 的完整性约束、带 transfer 历史的解绑、非 revoked owner 对直接用户删除的 RESTRICT、tombstone 清空 owner 后可删除用户、tombstone 不会进入共享账号查询，以及既有管理员账号兼容。
- 用户删除状态流转测试：Token 撤销失败或 tombstone 失败时不得调用 Emby 删除；删除链路不主动清理 Redis 租约，Token 撤销后旧事件不能续租并由 TTL 收口；Emby 删除失败不复活 Token 或个人账号；重新绑定生成新账号 ID 且旧 transfer 归属不变。
- Web Vitest：个人账号只显示 Cookie 而不出现 appType/UserAgent 输入、Cookie 提交后清空、未知客户端显示“未识别”、目录路径、配置/有效最大路数和套餐上限、动态输入边界 `1..100`、reservation“准备中”与真实活跃区分、状态门控、用量 unavailable 和角色隔离；套餐上限降低时不得把有效值误写回配置值。管理员共享 playback 覆盖路径/并发整体提交、不能部分更新、所有 `system` 用户合计用量、Redis 故障显示不可用而非零，以及降低上限不显示为已终止既有播放。套餐组表单同时覆盖小时 `1..100`、每日 `1..1000`、越界阻止提交、无静默截断以及每日小于小时仍允许提交。
- `services/api` 下执行 `go test ./...`、关键包 `go test -race`、`go vet ./...`、`go build ./...`。
- `services/web` 下执行 `npm run test`、`npm run build`。
- Compose 静态校验 Redis profile、不固定版本标签、依赖、healthcheck、volume 和外部 `REDIS_URL` 覆盖。

所有 Emby/115 测试必须使用 fake/fixture；测试不得启动项目服务或真实请求 Emby、115、Redis 云服务或其他外网。

### 受控验证

真实验证必须由用户另行明确授权，并逐项执行：

1. 先确认套餐组默认 personal、system 显式切换和无个人账号时 Emby fallback。
2. 使用测试用户/测试 115 账号验证只提交 Cookie 时，固定 `Mozilla/5.0` 能完成账号验证、目录解析和目标查重，再验证已有目录、最大路数和个人目标秒传；记录该默认 UA 的真实结果，不能用固定源码或 fake 测试代替。
3. 分别验证 personal 账号配置上限、正数套餐上限、套餐值 `0`、套餐降低后的运行时有效值与 system 共享账号跨用户总上限；确认个人账号越界配置被拒绝、降级不改数据库配置、共享账号不与单一套餐比较，超限只进入公共 fallback。
4. 验证 HEAD、首次 GET、Playing、暂停 Progress、恢复 Progress、Stopped、预加载未播放和无 Stopped TTL；记录目标客户端精确版本、事件顺序以及 reservation 是否按期晋级或释放。
5. 验证第 5/6 个小时请求、第 10/11 个每日请求、预存文件和失败退款边界。
6. 验证 Redis 短暂不可用时 Gateway 仍可通过 Emby 播放，且没有未计数的 115 新直链。

真实验证不得输出 Cookie、Token、完整 URL、完整 SHA1、Redis DSN 或 Provider 原始响应。

## 分阶段落地

### 阶段 0：计划、合同与 Redis 原型

- 已修订现有 115 总计划和稳定参考中的旧套餐并发/数据库会话设想。
- 固定 Redis Key、`reservation → active ↔ paused → stopped/expired` 原子结果、HEAD 禁止创建、`30s/2m/15m` 三类代码常量 TTL、配额窗口和故障行为。
- 为普通用户仅 Cookie 写入、appType 自动识别/unknown、固定 Provider UA、真实播放器 UA 隔离、目录解析和所有权补充合同测试。

完成条件：计划、数据合同、失败语义和 fake Redis 测试边界明确，不依赖真实 115 开始业务实现。

### 阶段 1：持久配置与用户控制面

- migration、模型、套餐组字段和现有管理员共享账号扩展；补齐共享 playback 的原子配置端点和列表/详情用量合同，`system` 只保留为套餐路由枚举。
- 用户个人账号 API/Web：只用 Cookie 创建 pending，显式验证后分别配置目录/并发，完整性与有效套餐复验后才启用；后端独占 appType/Provider UA，并完成 revoked tombstone 和用户删除联动。
- 暂不切换 Gateway 数据面。

完成条件：管理员共享账号 userspace 保持，目录和并发只能通过完整请求原子更新，目录解析失败零写入；个人账号按 Cookie 创建、显式验证、目录/并发配置、启用四步流转，且不能覆盖后端 appType/Provider UA；个人账号只能由 owner 管理，Cookie 替换必须清理旧 Provider/目录且不跨账号复用 ID，启用前重新检查完整性与有效套餐；解绑后 Cookie 被擦除、transfer provenance 保留、tombstone 不会被共享加载器选中，且不伪装清理已签发 CDN URL 对应的 Redis 占用；个人并发配置按有效套餐模板强制校验并返回配置值/有效值/套餐值，模板不可用时不写入；套餐默认 personal 有 migration 和前端测试保护。

### 阶段 2：Redis 播放租约与账号路由

- Redis 部署/客户端、account/user `leases + active` 两组索引、反向 session、固定 `30s/2m/15m` TTL 和事件旁路。
- Gateway 按 `personal|system` 套餐模式选择个人 playback 或管理员共享 playback；先用不解密 Cookie 的精确账号元数据完成 Redis 准入，再按账号 ID 获取运行期凭证并复用现有冷却/半开状态机。个人账号按 `effectiveMaxConcurrentStreams`、共享账号按自身总上限执行准入，账号满、套餐模板不可用或 Redis 失败时 fallback，不消耗半开探测机会。Redis 用户索引不新增第二套门控。

完成条件：并发和三类 TTL 测试覆盖单 Gateway 内的竞争，过期 member 不参与统计且无后续请求的索引 Key 会按物理 TTL 回收；HEAD/预加载不会形成真实活跃会话，GET reservation 不能穿透账号上限，个人/共享账号冷却到期后均只有一个半开探测，Redis 准入失败不延长账号冷却，普通 Emby 代理在 Redis 故障时保持可用；管理员共享 playback 摘要按所有 `system` 用户合计，并能区分零占用与 Redis 不可用。本阶段不验收多 Gateway 或 Redis Cluster。

### 阶段 3：转存配额

- 小时/每日配额、固定 `5m` 且不续租的 pending reservation、基于 `transferAttemptId` 的幂等成功提交、晚到成功补记、独立 `2s` succeeded 提交预算和失败退款。
- 用量 API/Web 与固定诊断日志。

完成条件：并发请求不能穿透 `5/10` 默认值，小时 `1..100` 与每日 `1..1000` 在 API/Web/SQL CHECK 中一致，预存文件和失败不误扣，日界线复用 `CRON_TIMEZONE`。

## 已完成项、剩余项与归档条件

已完成：

- 需求方向和核心语义已确认。
- 阶段 0：固定 TTL、Key/HMAC、Cookie `ssoent` 和 fake Redis 合同。
- 阶段 1：套餐字段、migration、管理员共享 playback 原子配置/用量、个人账号 API/Web 四步流转、revoked tombstone 与用户删除顺序。
- 阶段 2：Redis leases/active/reverse session、personal/system 两段式路由、账号准入、HEAD 复用和成功播放事件更新。
- 阶段 3：小时/自然日转存配额、pending/succeeded、晚到成功、独立 2s 提交预算、用户用量和最终决策日志。
- 稳定架构、数据模型、配置、API、Web 信息架构、115 端到端流程、数据库入口和部署/测试 runbook 已同步。
- `go test ./...`、`go vet ./...`、`go build ./...` 通过；`p115quota/p115account/directplay/playbackgateway` race 通过；Web 30 个文件通过、1 个跳过（224 项通过、3 项跳过），生产构建通过；Compose Redis 合同通过 YAML 静态检查。

剩余：

- 2026-09-05 已使用专用 `EMBER_INTEGRATION_DATABASE_URL` 执行 `go test ./internal/app -run 'Integration|PostgreSQL|P115' -count=1 -v`；新增 migration 幂等、约束、个人账号生命周期、tombstone、用户删除顺序等 PostgreSQL 集成用例全部通过。测试 harness 使用独立 `itest_*` schema，未启动项目服务或访问真实 115/Emby。
- 当前环境虽已安装 Docker CLI，但 Docker daemon 未运行，未执行 `docker compose config`；仅完成 YAML 解析与 Redis profile/image/AOF/healthcheck/volume/依赖/URL 静态断言。
- 未启动项目服务，未访问真实 Redis、Emby 或 115；个人 Cookie 固定 `Mozilla/5.0`、真实 personal/system 路由、客户端事件间隔、配额边界和 Redis 故障回退均待用户另行授权后受控验证。
- Emby `SimultaneousStreamLimit` 能否限制 115/local 分流仍未证实，本计划不新增 Gateway 用户级门控。

归档条件：

- 四个阶段全部落地并通过自动化验证。
- 在专用 PostgreSQL 集成库实际执行新增 migration 用例（已于 2026-09-05 完成）。
- 真实验证按用户授权范围记录证据；未授权的外部 E2E 必须明确标为未验证，不能伪写通过。
- 当前实现事实提炼到 `docs/system-architecture.md` 和对应 `docs/reference/`。
- `docs/plan/README.md`、计划盘点和交叉引用同步完成后，移入 `docs/archive/plan/architecture/`。

## 落地后文档处理

实现后同步：

- `docs/system-architecture.md`：账号归属、套餐路由、Redis 边界和 fallback。
- `docs/reference/data-model-reference.md`：套餐组和账号长期配置字段。
- `docs/reference/configuration-reference.md`：Redis 地址、可用性和生效方式；播放租约与转存 pending TTL 都是代码常量，不增加运行时配置。
- `docs/reference/api-endpoint-catalog.md`：用户账号、用量、套餐组字段和管理员共享 playback 配置端点。
- `docs/reference/web-information-architecture.md`：用户 115 菜单、管理员共享账号和套餐组页面职责。
- `docs/reference/p115-playback-end-to-end-flow.md`：从当前全局账号链路更新为已实现的 system/personal 路由。
- `docs/runbooks/deployment*.md`：Redis 部署、AOF、备份、故障回退和敏感配置。
