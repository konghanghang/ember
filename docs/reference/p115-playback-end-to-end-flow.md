# 115 Cookie 直连播放端到端流程参考

本文档从当前代码出发，说明 Ember 的 115 Cookie 账号控制面、Emby 身份桥接、PlaybackInfo 短期证明、保留式秒传、302 直连、Emby fallback、安全 reject、持久化与日志边界。

它回答“现在实际怎么运行”，不替代协议合同和实施计划：

- Emby method/path/Header/DTO：看 [Emby 4.9 系列播放代理 API 合同](./emby-playback-proxy-contract.md)。
- 115 Cookie/Web API、上传加密和直链安全：看 [115 Cookie 播放兼容合同](./p115-cookie-playback-contract.md)。
- 当前系统内置链路的尚未完成项和阶段安排：看 [Emby 115 直连播放网关实施计划](../plan/architecture/emby-115-direct-play-gateway.md)。
- 用户自有账号、套餐来源、Redis 当前活跃数与转存配额的实现约束及未执行受控验证：看 [115 用户自有账号路由与 Redis 配额实现方案](../plan/architecture/p115-personal-account-routing-and-redis-quotas.md)。

## 1. 核心原则

当前实现只有三种视频请求决策：

| 决策 | 条件 | 客户端结果 |
| --- | --- | --- |
| `reject` | Token/身份/用户硬状态失败，或 DirectPlay 期间请求已取消/超时 | Gateway 固定终止，不访问后续 Emby/115 |
| `redirect` | Principal、PlaybackInfo 证明、请求资格和 115 全链路均成功 | Gateway 返回空体 `302`，客户端直连 115 CDN |
| `fallback` | Principal 合法，但请求不适合加速或任一 115 步骤失败 | 透明代理权威 Emby 请求 |

最重要的边界是：

> Emby 正常代理播放是基线，115 是可选加速。只有安全失败可以拒绝；合法用户不能因为没有 115 条件而失去正常播放能力。

### 1.1 用户账号路由与 Redis 配额（已实现，真实链路未验证）

当前运行时保留唯一管理员 source，并按用户有效套餐在个人 playback 与管理员共享 playback 之间路由。实现不建立数据库播放会话或套餐播放并发：

播放路由支持 `admin` 与 `user`，两者均读取本人显式分组（未指定时读取默认分组）的播放模式、转存额度和策略模板。管理员的 `system` 套餐选择共享 playback；`personal` 套餐仍要求本人个人账号，不隐式借用共享账号。个人账号控制面仍限制为普通用户；共享账号的合计并发、健康检查和 Redis 准入不因管理员身份而放宽。

- 套餐组增加 `personal|system` 账号来源，migration 将历史套餐组统一回填为 `personal`，新建套餐组也默认 `personal`；既有用户没有个人账号时按已接受的产品语义进入公共 fallback，不为历史共享直连保留隐式 `system` 或 feature flag。只有管理员主动设置的套餐组使用当前管理员共享 playback；`system` 只是套餐路由值，不是账号类型或 scope。
- `personal` 用户可在控制台绑定本人唯一 playback 账号，固定按“只提交 write-only Cookie 创建 `pending + disabled` → 显式验证为 `active + disabled` → 配置已有目录路径与最大播放路数 → 完整性和当前套餐复验后启用”流转。创建不请求 115 或套餐模板，页面和 API 不接受 `appType/UserAgent`；未绑定或账号不可用时回退 Emby。
- 后端从 Cookie 唯一合法 `UID` 的 `ssoent` 自动派生个人账号 `app_type`，未知编码保存 `unknown`，缺失、重复或非法 `UID` 直接拒绝；普通 Cookie/Web 请求固定使用 `Mozilla/5.0`，该默认值尚未经过目标个人 Cookie 的真实 115 验证。
- 最终下载直链继续使用 Gateway 收到的真实播放器 User-Agent，不能用固定 Provider User-Agent 替代；秒传初始化继续使用协议代码内的版本绑定上传 User-Agent。
- 最大播放路数属于具体 playback 账号。个人账号配置时读取当前有效套餐模板：`SimultaneousStreamLimit > 0` 要求 `1 <= maxConcurrentStreams <= SimultaneousStreamLimit`，值为 `0` 时按 Ember 内部合同视为没有有限套餐上限，但账号配置仍限制为 `1..100`。运行时使用 `effectiveMaxConcurrentStreams = min(configuredMaxConcurrentStreams, positive SimultaneousStreamLimit)`；套餐降低不自动改写数据库配置。管理员共享 playback 按所有 `system` 使用者合计，不与单个套餐上限比较。
- 管理员共享 playback 在显式验证为 `active` 后，通过 `PUT /api/v1/admin/p115-accounts/:id/playback-config` 原子提交已有目录路径和正整数 `maxConcurrentStreams`；两个字段都必填，目录解析失败时 path、ID 和并发配置全部不写，解析期间 Cookie、状态或账号版本变化则返回 `409`，禁止旧凭证解析结果覆盖新状态。已启用账号允许降低上限但不终止已有播放，新 reservation 等待合计占用降到新上限以下。现有管理员列表/详情直接返回该共享账号下由 `system` 路由建立的全部现存租约合计，不按用户当前套餐重新归类；Redis 查询成功且 Key 缺失时 `usageAvailable=true` 并返回零，Redis 不可用时 `usageAvailable=false` 且计数为 `null`，不新增独立用量端点。
- 个人与共享 playback 复用同一运行期健康状态机：先按套餐路由选择不解密 Cookie 的精确账号元数据，Redis 准入成功后再按 account ID/owner 加载凭证。冷却未到期时 fallback；到期后 PostgreSQL 行锁只发放一个 1 分钟半开探测，成功恢复 `active`，失败按类型重新冷却、进入 `expired` 或 `error`。Redis 失败不消耗半开机会，两次读取间的停用、解绑、Cookie 替换或控制面配置更新均失败关闭并只释放本次新 reservation。
- 个人账号 Cookie 替换回到 `pending + disabled`，清空旧 Provider UID 和目标目录 path/ID，保留待下次启用重新按套餐复验的并发配置；旧目录 ID 禁止跨 Provider 账号沿用。只有 `enabled` playback 必须同时具备 `active`、Cookie、Provider UID、成对目录 path/ID 和正整数并发配置；未配置完成的 pending/disabled 记录允许目录和并发为空。
- 个人账号解绑使用不可复活的 `revoked` tombstone：在同一事务清空 owner、Cookie、Provider、`appType/UserAgent`、source/playback 目录、并发和健康等运行期数据，但保留账号 ID、role、后端固定 alias、auth mode、revoked/disabled 状态和时间供 transfer provenance 引用；owner 外键使用 `ON DELETE RESTRICT`，确保未完成 tombstone 的活动个人账号会阻止直接删除用户，revoked 状态则保证已清空 owner 的 tombstone 不会被共享账号加载器选中。
- Redis 同时维护账号/用户的占用与真实活跃索引：合格 GET 在 302 前只建立 `30s reservation` 并参与账号并发准入，HEAD 无既有租约时直接 fallback；成功 Playing/Progress 才晋级为 `active` 并刷新 `2m` TTL，暂停继续占用并刷新 `15m` TTL，Stopped 成功转发后释放。成功事件的租约更新使用保留请求上下文值的 `WithoutCancel + 2s timeout`，客户端在 Emby 接受后断连不取消这次更新；未成功转发仍不更新。Sorted Set 的过期 member 每次脚本按 Gateway 可注入时钟清理并不再计数，account/user `leases + active` 另使用 `16m` Key TTL 回收无后续请求的空闲索引。三类业务 TTL 首期使用代码常量，不增加环境变量或后台配置；用户索引只用于展示、归因和后续治理，不参与第二套并发门控。
- Redis 账号索引键使用规范化 Provider UID 的服务端用途隔离 HMAC，不使用数据库账号 ID、owner 或 Ember 用户 ID，也不暴露原始 Provider UID；解绑只擦除持久凭证并停止新 `302`，不删除仍可能对应已签发 CDN URL 的 Redis 占用。同一真实 115 账号以新数据库 ID 重新绑定时仍命中旧租约，不同 Provider UID 不继承；现有会话由成功 `Stopped` 或 TTL 收口。
- Redis 可用且命令成功时，当前 Key 是占用和转存用量的唯一真相源；Key 不存在按零处理。不锁定或探测 Redis 版本，只使用 Lua/Sorted Set/TTL 通用能力；首期只支持单 Gateway，所有 score 和套餐窗口使用 Gateway 可注入时钟与全局 `CRON_TIMEZONE`，不承诺多 Gateway、Redis Cluster 或跨主机时钟兼容。Redis 重启或数据丢失后的计数重置是已接受行为，不增加 epoch、恢复等待、数据库重建或历史补偿。
- 套餐组提供用户小时/每日转存限额，默认每小时 `5`、每天 `10`；小时范围固定 `1..100`，每日范围固定 `1..1000`，`0` 非法，越界直接拒绝且不截断，两者不要求大小关系。只有目标缺失且秒传、目标复核均成功的新文件消耗一次额度，预存命中、重复请求和失败不消耗。并发防穿透使用固定 `5m` 且不续租的 pending reservation，pending/succeeded 复用同一 opaque `transferAttemptId` 并幂等完成。失败或预存命中立即删除 pending，进程崩溃后最多保留 5 分钟；pending 已过期但外部转存晚到成功时仍补记一次 succeeded，记录固定诊断码且不删除文件、不污染账号健康。succeeded 提交使用独立 `2s` 总预算有限重试，只有记账成功才继续 `302`；最终失败时保留文件和 pending、本次公共 fallback，不建立数据库补偿或从 transfer 历史重建 Redis。
- Redis 客户端启用 `ContextTimeoutEnabled`，底层读写同时受 500ms 超时和调用方剩余 deadline 约束，避免成功记账重试末尾仍等待完整读写超时。失败退款使用独立 context，释放错误仅记录 `transfer_quota_release_failed`、`userId` 和错误类型，不输出原始错误或 attempt ID；预占由原 TTL 回收，正常已不存在不告警。
- 控制台 115 页面优先复用账号摘要的可用统计，正常成功路径从五次 Redis 统计脚本降为三次；未绑定、摘要失败或统计不可用时仍查询独立个人用量接口。刷新时清除旧统计，可用零值正常展示，不可用值不伪装成零。
- Redis、账号并发或转存配额不可用时只停止新的 115 加速并 fallback Emby，不改变用户安全门控，不污染 115 账号健康状态。
- 不新增 Gateway 用户级总并发门控。115 `302` 使视频字节绕开 Emby 视频上游，当前没有证据证明 Emby `SimultaneousStreamLimit` 能限制这类分流播放；该效果保持“未证实”，不能写成当前保证。

### 1.2 Emby fallback 边界

- source 路径映射使用独立非敏感加载边界：只读取唯一启用的管理员全局 source 的 `embyPathPrefix/sourceRootId`，不检查 Provider 的 `active/error/cooling_down`、不读取 Provider UID、不解密 Cookie 或申请半开探测；管理员手工停用 source、没有唯一启用记录或位置非法时直接回退 Emby，真正访问 115 仍走严格凭证加载器并复核 source 位置。
- DirectPlay 返回取消/deadline 或 request context 已终止时返回 `499/504`，不发起 Emby fallback。已进入代理后，上游传输返回包装后的 `context.Canceled` 则返回空体 `499`，普通请求只记 Debug 取消信息，视频保留单条 Info `playback_canceled`，不误记上游故障。
- 其他 DirectPlay 不适用或失败场景透明回退 Emby。仅旧 plain stream 的缺会话补查分支生成权威地址；新增扩展名 stream / 固定 original 补查分支保留原客户端 URL、RawQuery、Range 和 Header。Gateway 不读取媒体文件，也不自行实现 Range、缓存或传输编码语义。

继承显式起播意图的内部媒体证明，在视频准入阶段使用其记录的原客户端会话建立 115 租约；后续客户端事件无需会话翻译，原样转发 Emby 后更新相同租约。媒体证明仍按补查会话验证，日志以 `playback_lease_session_bound` 关联原会话与 proof 的脱敏引用。此修复解决收到事件却找不到对应租约的问题，不证明客户端为何出现上报空档；超过 reservation/active/paused 原 TTL 的租约仍不复活。升级前已使用内部会话建立的旧租约不会被迁移，应停止并重新起播建立新租约。

媒体证明过期后，新的内部补查可以继续关联原播放会话：proof 缓存同一锁下保存全局最多 4096 条、每个 mapping 最多 64 条身份/媒体匹配记录，满载优先淘汰同 mapping 最旧关联并接纳新记录，只有成功准入的有效视频证明才能建立。初始窗口为 reservation 的 30 秒；真实事件转发成功且租约更新命中后，按 active 2 分钟或 paused 15 分钟更新窗口。视频读取和缓存命中不延长该活动窗口，Stopped、租约缺失、更新的客户端证明或过期会清除关联；晚到的旧候选不能恢复停止关联。新 proof 仍须独立满足媒体合同，只继承会话身份，绝不重新授予或延长 30 秒转存许可。多个候选会话或媒体身份不一致时不猜测匹配。关联不写数据库或 Redis，进程重启后需重新建立。

### 1.3 新增转存起播许可

详情浏览的视频 GET 不等于点击播放。Gateway 在现有 proof 缓存中单独保存 30 秒新增转存许可：只有客户端 POST PlaybackInfo 的唯一布尔 `IsPlayback=true` 与成功响应匹配，才授予选定媒体源；未指定媒体源时要求响应唯一。DirectPlay 能力不足的显式客户端响应可独立保留意图；后续内部 proof 按完整身份、路径、Container 和已知 Size 匹配后仅继承原窗口剩余许可，并绑定一个补查会话。Gateway GET 补查和缓存命中不自行签发或延长许可，意图解析歧义不改变普通 proof 和透明代理。严格字段与多源合同见 [Emby 播放合同](./emby-playback-proxy-contract.md#新增转存起播许可ember-内部策略)。

DirectPlay 先保留现有缓存/目标查重行为；内容锁内再次确认目标缺失后，在转存配额、任务和 preID 之前调用 Gateway 提供的实时许可检查。缺失、过期、已停止或 proof 代次被替换时以 `playback_intent_required` fallback，不创建任务、不占转存配额、不污染账号健康，并释放本次新建 reservation。HEAD 只能复用已有目标。该策略仍允许详情探测复用已有文件及短暂预留播放并发，不表示详情页完全不请求 115。

成功 Stopped 清对应身份/条目/session 的许可，缺少 MediaSourceId 时覆盖该 session 的全部 source；清理不依赖 Redis 租约是否存在。普通 proof、十分钟跨 session 媒体缓存和下载地址缓存保留。许可有效期不续期，TTL 内失败可重试；准入后已经开始的转存不因随后停止或过期回滚。默认严格模式下，未携明确起播字段的客户端首次目标缺失时回退 Emby；管理员兼容策略见下节，其他播放器仍须单独实机验收。

按需 PlaybackInfo 的内部 GET 与客户端响应观察共用 proof 缓存锁和有界请求期令牌。内部 owner 的缓存复查/登记及最终检查/发布均原子完成；内部发布只替换同 mapping/item/device 的整 item 快照并废弃同设备其他内部结果，其他设备可独立完成，不撤销正在读取 body 的客户端观察。客户端响应入口使同 item 的旧 proof/两类旧令牌失效并登记自身，终态在自身令牌仍有效时发布成功或空快照；旧内部 GET 不能覆盖新起播许可，旧客户端失败收尾也不能清掉较新快照。不影响无关 item，过时内部结果不借用新许可；实际 owner/响应观察结束后清理令牌，不增加数据库或持久缓存。

2026-09-22 用户确认当前部署只有 STRM 中的本地路径，没有对应媒体挂载，接受未明确起播的详情探测回退 404。STRM 存在不代表 Emby 可读其目标；这类部署中真正播放的 115 链路失败时也没有 Emby 视频源兜底。该部署取舍不改变 Gateway 透明返回真实状态的合同。同日用户随后反馈正常播放没有问题，确认当前客户端实际可播放；首次新转存、停止后重播等场景尚未逐项取证。

用户账号、套餐来源和 Redis 租约/配额均已有代码与自动化 fake/fixture 证据；本轮自动化验证没有启动项目服务，也没有请求真实 Redis、个人 115 Cookie、Emby/CloudDrive2。用户对正常播放的反馈单独记录，不扩展为上述各子系统均已逐项验收。

#### 首次转存策略

上面的明确起播限制属于默认 `strict` 模式。设置中心可显式选择 `P115_TRANSFER_TRIGGER_MODE=compatible`，让合格原文件视频 GET 在目标缺失时以兼容策略准入；包含可能的详情预读，适用于所有客户端，HEAD 始终不能新转存。锁内实时检查仍核对原 proof 和请求取消，然后才申请配额、创建任务；不改写客户端起播字段，也不生成严格许可。

已有严格许可优先处理；只有需额外兼容授权时才读取五秒配置缓存，单次读取最多 `500ms`。过期读取失败、非法值或缺失配置不新增兼容授权，已有目标复用不依赖该读取；配置正常时修改最多一个五秒窗口收敛，关闭不回滚已开始转存。

### 1.4 Infuse 详情页读取视频与缺失 thumb 的排查记录

2026-09-22，用户提出候选原因：单集缺少 `thumb` 图片，Infuse 打开详情页时可能读取视频以生成缩略图。该因果关系**未证实**；以下记录针对 Emby `4.9.3.0`、Infuse `8.5.3`，不推广为其他版本的固定行为。

- **已确认的现象**：用户未点击播放，日志仍出现视频 GET。门控上线前，19:55 的单集请求触发一次新增转存，随后复用直链读取视频不同位置；这些请求本身不能区分缩略图生成与媒体信息探测。
- **门控后的结果**：`gateway-2026-09-22.log` 中，20:26:37—20:26:39 以及 21:12:51—21:12:53 的单集 `itemId=85716` 请求均因 `playback_intent_required` 跳过新增转存，Emby fallback 返回 404；对应窗口未见 Playing/Progress/Stopped。详情页视频请求仍存在，不能把阻止转存写成消除了客户端探测。
- **缺少的证据**：尚未核对该单集的图片文件及 Emby 图片元数据，也未进行补齐 thumb 前后的受控对照；现有日志不足以建立“缺图 → 视频取图”的因果链。

当前保留新增转存起播许可，不因详情页可能需要截图而放开转存。用户已接受本部署未挂载真实媒体时的详情探测 404，并于同日反馈正常播放没有问题；若缺图假设成立，视频读取失败仍可能使自动生成缩略图失败。正常播放的用户实测结果不证明缺图假设，也不替代首次新转存、停止后重播等分项取证。

后续验证先核对单集 thumb 是否存在且能由 Emby 提供，再通过既有刮削流程补齐图片，对比相同客户端、相同浏览操作下的视频请求；记录 Infuse 缓存状态，并用此前未打开的单集作对照，避免把缓存命中误判为补图效果。本次只记录现象和验证方向，尚未执行补图或真实客户端对照。

## 2. 组件与职责

```mermaid
flowchart LR
    Admin[管理员] --> Web[管理 Web<br/>/console/p115-accounts]
    User[普通用户] --> UserWeb[用户 Web<br/>/console/p115]
    Web --> API[Ember API<br/>P115AccountHandler]
    UserWeb --> API
    API --> AccountService[P115AccountService]
    AccountService --> Secretbox[secretbox<br/>Cookie 加解密]
    AccountService --> PG[(PostgreSQL)]
    AccountService --> Validator[CookieCredentialValidator]
    Validator --> P115[115 Cookie/Web API]

    Client[Infuse / Emby Client] --> Gateway[ember-gateway]
    Gateway --> TokenService[EmbyTokenService]
    TokenService --> PG
    Gateway --> Emby[Emby 4.9.x]
    Gateway --> Proof[进程内 PlaybackInfo 证明缓存]
    Gateway --> DirectPlay[DirectPlayService]
    Gateway --> Redis[(Redis<br/>leases + transfer quota)]
    DirectPlay --> AccountService
    DirectPlay --> Redis
    DirectPlay --> TaskStore[TransferTaskStore + advisory lock]
    TaskStore --> PG
    DirectPlay --> Provider[CookieProvider]
    Provider --> P115
    Gateway -- 302 --> CDN[115 CDN]
    Client -- Range / 视频字节 --> CDN
    Gateway -- fallback --> Emby
```

| 组件 | 当前职责 | 明确不负责 |
| --- | --- | --- |
| 管理/用户 Web | 管理员维护全局 source/shared playback；普通用户按四步流转维护本人 playback，并查看 Redis 用量 | 不保存或回填 Cookie，不直接请求 115，不显示内部 Redis Key/目录 ID |
| P115AccountService | 加密凭证、owner/状态/唯一性、personal/system 路由元数据、准入后凭证加载、控制面用量与 revoked tombstone | 不做 Provider 秒传编排，不清理 Redis 旧租约 |
| CookieProvider | 固定 Cookie/Web 协议、路径解析、查重、秒传、目标复核、下载 URL、Range Hash | 不做用户资格和 Gateway 决策 |
| EmbyTokenService | Token HMAC 映射、实时用户资格、软撤销 | 不保存 Token 明文，不替代 Emby Token |
| Playback Gateway | Emby 透明代理、身份门控、证明观察、视频决策、成功事件后的 Redis 租约更新和单条最终日志 | 不持久化播放决策，不读取媒体文件，不把 Cookie/Provider UID/Redis Key 给客户端或日志 |
| DirectPlayService | 路由元数据、Redis reservation、source 解析、目标查重、转存配额、保留式秒传、任务/锁、直链候选 | 不注册 HTTP 路由，不删除保留文件，不把 Redis 故障写成账号健康错误 |
| Redis | account/user `leases + active`、反向 session、用户 transfer pending/succeeded；Lua 原子更新，缺失 Key 按零 | 不保存 Cookie、下载 URL、原始 Provider UID 或原始 PlaySessionId，不从 PostgreSQL 重建 |
| PostgreSQL | 账号密文、Token 摘要、transfer provenance、唯一约束与 advisory lock | 不保存完整 115 URL、Token 或播放证明 Path |

### 2.1 Gateway 启动边界

`ember gateway` 按 `InitDB → Migrate → VerifySchema → load ConfigService/CRON_TIMEZONE → GET /emby/System/Info → build Redis/EmbyToken/DirectPlay/Gateway → listen` 启动。只有目标 Emby 的 ServerId 非空，且四段数字 Version 满足 `>= 4.9.0.0 && < 4.10.0.0` 时才打开 Listener；`4.9.3.0` 是协议证据基线，不是唯一运行版本；`/health` 只在完整构造后可用。Redis 构造不做 Ping 或版本探测，实际命令失败只关闭本次 115 加速。

Compose 的 `gateway` profile 复用 `ember-api` 镜像，只把 command 改为 `gateway`，同时启动浮动 `redis:alpine`、AOF 和持久卷并等待 Redis healthy；API/Gateway 共用 `REDIS_URL`。Gateway 固定监听容器内 `8081`。当前 Redis 合同只支持单 Gateway，不支持 Cluster 或跨主机时钟协调；公网 HTTPS 和原始 Emby 网络隔离仍不由 Compose 自动完成。

## 3. 管理员账号控制面

### 3.1 两种账号角色

| 角色 | 用途 | 必填运行位置 |
| --- | --- | --- |
| `source` | 定位 Emby 原始文件并读取秒传所需的有界 Range | `embyPathPrefix + sourceRootId` |
| `playback` | 保存保留式秒传文件并签发最终客户端直链 | `targetParentPath + targetParentId + maxConcurrentStreams` |

运行期稳定态要求一个 `enabled + active` 管理员 source；playback 则由套餐路由选择用户本人的 enabled personal 账号或唯一 enabled shared 账号。数据库 partial unique 保证管理员每个角色最多启用一条、每个 owner 最多一个当前个人账号，并禁止 Provider UID 在任一非 revoked 账号间重复；已到期 `cooling_down` 仅允许 PostgreSQL 半开租约持有者临时加载并探测。

管理员 source/shared playback 是全局基础设施；普通用户可在 `/console/p115` 绑定自己的 playback。`system` 只是套餐路由值，不是账号 role/scope，管理员控制面始终只查询 `owner_user_id IS NULL AND status <> revoked`。

### 3.2 创建、验证、启用时序

```mermaid
sequenceDiagram
    actor Admin as 管理员
    participant Web as 管理 Web
    participant API as Ember API
    participant Account as P115AccountService
    participant Secret as secretbox
    participant DB as PostgreSQL
    participant P115 as 115

    Admin->>Web: 输入角色、Cookie、UA、目录信息
    Web->>API: POST /api/v1/admin/p115-accounts
    API->>Account: Create(input)
    Account->>Account: 从 UID ssoent 识别 appType
    Account->>Secret: Encrypt(Cookie)
    Secret-->>Account: cookieCiphertext
    Account->>DB: INSERT pending + disabled
    DB-->>Account: P115Account
    Account-->>API: 安全摘要（无 Cookie）
    API-->>Web: 201 AccountSummary

    Admin->>Web: 点击“验证”
    Web->>API: POST .../:id/validate
    API->>Account: Validate(id)
    Account->>DB: 读取密文与账号元数据
    Account->>Secret: Decrypt(ciphertext)
    Account->>P115: ValidateCredential(Cookie, UA)
    alt Cookie 有效
        P115-->>Account: Provider UID
        Account->>DB: status=active, lastValidatedAt, lastSucceededAt
        API-->>Web: valid=true
    else Cookie 明确失效
        Account->>DB: status=expired, enabled=false
        API-->>Web: valid=false
    else 网络/协议失败
        Account->>DB: status=error + 脱敏错误
        API-->>Web: 502
    end

    Admin->>Web: 点击“启用”
    Web->>API: PUT .../:id/enabled
    API->>Account: SetEnabled(id, true)
    Account->>DB: 行锁检查 active/UID/角色位置/唯一约束
    DB-->>Account: enabled=true
    Account-->>API: AccountSummary
    API-->>Web: 200
```

创建和 Cookie 替换都会优先从 `UID` 的 `ssoent` 自动识别客户端类型并写入现有 `app_type`；只有未知编码才接受请求中的 `appType` 人工兜底。Cookie 替换会在同一次更新中刷新 `app_type`，把账号重置为 `pending + disabled`，并清空 Provider UID、验证时间、成功时间、冷却和错误字段；必须重新验证后才能启用。识别全程在本地完成，不请求 115 设备接口。

个人账号创建和 Cookie 替换只提交 Cookie：未知 `ssoent` 保存 `unknown`，缺失、重复或非法 `UID` 拒绝，Provider User-Agent 由后端固定为 `Mozilla/5.0`；管理员现有 `appType` 人工兜底和显式 User-Agent 输入保持兼容。

### 3.3 账号状态

```mermaid
stateDiagram-v2
    [*] --> pending: 创建 / 替换 Cookie
    pending --> active: 显式验证成功
    active --> active: 启用 / 停用
    active --> expired: 显式验证确认 Cookie 失效
    pending --> expired: 显式验证确认 Cookie 失效
    active --> error: 显式验证遇到网络/协议失败
    pending --> error: 显式验证遇到网络/协议失败
    error --> active: 再次验证成功
    active --> cooling_down: 播放遇到临时 Provider 故障
    cooling_down --> cooling_down: 半开探测失败并续租
    cooling_down --> active: 显式验证 / 半开探测成功
    active --> expired: 播放确认 Cookie 失效
    cooling_down --> expired: 显式验证 / 半开探测确认 Cookie 失效
    active --> error: 播放确认 Provider 协议错误
    cooling_down --> error: 显式验证 / 半开探测遇到协议错误
    expired --> pending: 替换 Cookie
    error --> pending: 替换 Cookie
    cooling_down --> pending: 替换 Cookie
```

`enabled` 是独立布尔轴：`active` 不等于已启用。显式验证或播放调用确认 Cookie 失效时会进入 `expired + disabled`；显式验证的网络/协议错误进入 `error` 并保留 enabled。播放期间临时 Provider 故障进入默认 1 分钟 `cooling_down`，限流 / 风控分别默认 15 秒 / 30 秒；未到期时不读取 Cookie；到期后 PostgreSQL 行锁只发放一个 1 分钟半开探测租约，成功恢复 `active`，失败按错误类型重新冷却。播放中的账号级协议错误回写进入 `error`；已识别文件失败按 §7.2 隔离。路由和目录保存使用独立 `config_version`，健康成功不再使其他请求的配置快照失效；Cookie、显式验证、启停、目录/并发和解绑会推进配置版本。运行期回写同时匹配加载凭证时的 Cookie 密文、`config_version` 和 `updated_at`，旧请求不能覆盖 Cookie 替换、显式验证、手工启停或更新后的结果。

source 的路径解析及媒体身份校验成功，即构成真实源健康观察。后续目标查询、起播许可、配额或存储拒绝不应让 source 继续停在半开冷却；源健康在本次源调用结束后独立结算，若后续源 preID/challenge 明确凭证失效或 Provider 临时不可用则不报告成功；业务拒绝、Range 协议不匹配和无效 hash 按文件请求隔离，不把整个源账号置为 error，任务失败落库异常也不能掩盖源失败。继续使用原凭证快照 CAS，较新的故障、停用和 Cookie 替换不能被旧成功覆盖。playback 仍以真实下载接口成功作为恢复证据。

DirectPlay 对源路径查询设置 30 秒只读预算，与 Adapter 目录 flight / 分页上限对齐；每次 preID/challenge 的源取链（含本地数据面排队）和实际 Range 各有独立 5 秒窗口；名义预算之和为 10 秒，Service 和 Adapter 不再叠加会抢先覆盖阶段信息的 10 秒总定时器。Range 的窗口覆盖 DNS、TCP/TLS、响应头与完整有界 body 读取；取链阶段不再消耗它的窗口。各阶段仍受调用方更早 deadline 与整次加速 2 分钟总 deadline 约束。超预算主动取消并以 `source_read_timeout` 回退，Adapter 的局部超时保留可用的 `download_url/source_range` 失败阶段，大目录/范围读取超时不写账号故障，未开始的秒传不再执行；已开始的 retained 写入不受这一只读预算中断。客户端取消本身不写账号故障；没有有效源身份时也不伪造恢复成功，既有半开探测租约保留原到期规则。该预算不限制 Emby 回退耗时，客户端实际起播仍需线上验证。

持续健康账号复用凭证读取快照，对 1 分钟内的干净成功跳过健康 SQL；错误、恢复和配置修改后的首次成功仍立即尝试条件写入。采样观察不会推进健康版本或清除新错误。保留文件复用的访问时间使用单条条件 UPDATE，先确定最近 succeeded 任务，再按 1 分钟窗口更新；旧请求不回退时间，窗口内及外部预存文件均允许零行更新。这两个时间是观察采样，不是逐次播放审计或活跃会话真相。

源路径解析按请求范围处理：`/files/get_path_id` 或最终目录列表的业务拒绝、协议异常，以及返回的无效源文件身份，均不能单独证明源账号故障，因此不触发上述 `error` 状态。DirectPlay 停在源解析阶段，释放本次新建的播放 reservation；Gateway 保留 `providerOperation=resolve_source_path` 并回退 Emby。明确的凭证失效/临时不可用仍遵循原健康状态机；失败请求不会清除已有冷却状态或冒充成功探测。

### 3.4 个人账号控制面与解绑

个人账号固定按以下顺序推进：

1. `POST /api/v1/user/p115-account` 只提交 Cookie，创建 `pending + disabled`；不调用 115 或套餐模板。
2. `POST /api/v1/user/p115-account/validate` 使用本人密文凭证显式验证，成功进入 `active + disabled`。
3. 目录接口解析已有路径并成对保存 path/ID；并发接口在同一事务内锁定账号、用户、套餐组和模板，要求 `1..100` 且不超过正数 `SimultaneousStreamLimit`。
4. 启用接口在行锁内复验 active、凭证、Provider UID、目录、并发和当前套餐；套餐降低不改写已保存配置，运行时计算较小有效值。
5. `DELETE` 不物理删除账号，而是写入不可复活的 revoked tombstone，清空 owner/凭证/Provider/目录/并发/健康字段并保留账号 ID 供 transfer 外键引用。用户删除顺序为 Token 撤销 → tombstone → Emby 删除 → Ember 用户删除。

解绑及删除用户的 tombstone 提交成功后，才以事务锁定记录的旧 Provider UID 通知本进程数据面限流器；失败、回滚或重复解绑不误清理。无人等待且余额回满时回收桶，未回满或仍有等待者则保留原余额/FIFO，防止重绑刷新突发额度；旧回调不能修改新桶。API/Gateway 分进程时，各自依靠空闲回满回收，不新增跨进程通知。解绑不删除 Redis 租约：已签发 CDN URL 仍可能播放，现有 reservation/active/paused 由成功 Stopped 或 TTL 收口。同一 Provider UID 重新绑定时会生成相同 HMAC account key 并继续受旧占用限制。

## 4. Emby 登录与 Token 映射

```mermaid
sequenceDiagram
    participant Client as Emby Client
    participant Gateway as ember-gateway
    participant Emby as Emby 4.9.x
    participant Token as EmbyTokenService
    participant DB as PostgreSQL

    Client->>Gateway: GET /System/Info/Public<br/>无需登录或应用头
    Gateway->>Gateway: root API 规范化为 /emby/System/Info/Public
    Gateway->>Emby: 透明转发 bootstrap
    Emby-->>Gateway: PublicSystemInfo
    Gateway-->>Client: Emby 原状态/Header/Body
    Client->>Gateway: POST /Users/AuthenticateByName<br/>严格应用头或 Web query 元数据
    Gateway->>Gateway: root API 规范化为 /emby/Users/AuthenticateByName
    Gateway->>Gateway: 严格校验唯一 Header/query 元数据载体
    Gateway->>Emby: 原请求透明转发
    Emby-->>Gateway: deflate AuthenticationResult
    Gateway->>Gateway: 保留原压缩响应，按 identity/gzip/deflate/br 白名单有界解码旁路副本
    Gateway->>Gateway: 读取 User.Id / AccessToken / ServerId
    Gateway->>Token: RecordAuthenticationResult
    Token->>Token: purpose 隔离 HMAC-SHA256(AccessToken)
    Token->>DB: upsert serverId + tokenHash + user/device
    Gateway-->>Client: Emby 原状态/Header/Body
```

关键语义：

- Emby 登录响应始终是客户端真相；旁路映射失败不能改写 Emby 成功响应。
- 固定 OpenAPI API family 的 root path 与已有 `/emby/...` 共用同一门控和处理器；family/前缀大小写不敏感，重复 `/emby/emby/...`、尾斜杠、额外层级和 alternate escaping 失败关闭，根 `/web/...` 留给后续 Web Surface 合同。
- `System/Info/Public` 仍是原生客户端唯一不要求已映射 AccessToken 和应用头的登录前公开 API；Emby Web 仅额外允许精确 `/emby/Branding/Css.css`、单层语言 JSON、携严格 query 元数据的 Branding Configuration，并让 Public users 与 AuthenticateByName 使用同一严格 query 形态，其他 bootstrap 不随之放宽。
- 登录后的目标 Web Item Image 可按独立图片合同使用无 Token 的精确 `/emby/Items/{Id}/Images/{Type}` 与可选规范非负 int32 Index；它不参与 Principal、PlaybackInfo 证明或 115 决策，携 Token 图片仍走普通身份门控。
- 数据库只保存 32 字节 Token HMAC，不保存 AccessToken 明文。
- 后续受保护请求按 [客户端兼容矩阵](./emby-client-compatibility-matrix.md) 收集 `X-Emby/X-MediaBrowser` 直接 Token Header、严格应用头和固定 query aliases；所有非空来源同值才接受，空值、重复、冲突和非法格式失败关闭。任意 Bearer、Quick Connect/PIN 不进入 Gateway 身份来源。
- 三种应用头的 `Emby` / `MediaBrowser` scheme 按 [RFC 9110 §11.1](https://www.rfc-editor.org/rfc/rfc9110.html#section-11.1) 大小写不敏感识别，Token 原字节及字段 grammar、重复/冲突检查保持；日志只用相同规则输出固定 scheme 分类。
- Store 请求取消/deadline 分别返回 `499/504`，只有 driver 保证未发送到 PostgreSQL 的幂等读错误才重试一次；真正存储失败继续 `503` 并记录脱敏连接池统计。
- 用户停用、Emby 禁用、访问禁用、解绑/删除和设备强制退出会写本地软撤销；普通到期使用实时资格拒绝，不永久撤销映射。

### 4.1 用户条目 Container 快照

Infuse 可能不先调用 PlaybackInfo，而是在成功获取 `/Users/{UserId}/Items/{ItemId}` 后直接请求 `/Videos/{Id}/stream?MediaSourceId=...&Static=true`。Gateway 对用户和响应 Id 均匹配的条目 `200` 响应只缓存有界 Container：

```text
mappingId + itemId + mediaSourceId -> container
```

该缓存不包含 Token、Path、Size 或响应体，不是 115 授权证明。按需 PlaybackInfo 失败后，plain stream 仍完全缺少必填 Container 时，它才用于克隆正常 Emby fallback 并追加 Container；该降级分支不会进入 DirectPlay。

分类时显式排除 `/Users/{UserId}/Items/Latest`、`Resume`、`Root` 三种静态端点：它们继续按普通受保护请求透明代理，不使用尾段字面值建立 ItemId 快照。固定版本响应合同见 [用户与条目路由合同补充](./emby-user-policy-and-item-route-contract.md)。[GitHub Issue #8](https://github.com/konghanghang/ember/issues/8) 的误分类已由代码与 fake 回归覆盖，测试包含 root/`/emby`、大小写、HEAD、gzip 原始响应保持和快照日志缺席，不代表新增实机验收。

### 4.2 缺 PlaySessionId 的按需 PlaybackInfo

GET/HEAD 的 plain stream、`stream.{Container}` 和固定 `original.mp4/mkv` 在唯一非空 MediaSourceId、显式唯一 `Static=true` 且完全没有 PlaySessionId key 时，可以按[播放合同 4.5](./emby-playback-proxy-contract.md#45-缺失-playsessionid-的按需-playbackinfo)补查。新增扩展名 stream / 固定 original 只接受身份 Token、MediaSourceId、Static、Container，以及可选唯一且匹配 Principal.DeviceId 的 DeviceId；容器必须单一合法且不是 manifest，stream 的额外 Container 存在时必须一致，original 必须显式带同扩展名 Container。无 Static 的 original 缺会话、缺源、多源歧义、未知/选流参数及设备/容器错配继续回源。

1. 先筛选同一 mapping/item/source 且完整 Principal 身份匹配的未过期证明，再选本设备最新记录；其他设备的新证明不能遮蔽本设备仍有效的证明，身份变化时不得复用旧 PlaySessionId。
2. 未命中时使用当前用户 Token 请求内部 Emby `GET /Items/{Id}/PlaybackInfo?UserId=...`；不使用管理员 API Key，Token 不进入 URL。
3. singleflight 键为 mapping/item/mediaSource/device/Token-SHA256，键中不保留明文 Token，也不输出键；相同键才合并请求。调用固定 10 秒超时，等待方可独立取消。
4. 成功响应必须提供 PlaySessionId/Container，按明确 MediaSourceId 精确选择匹配 item/source 的唯一源，不能取多源中的第一项；合格 DirectPlay source 写入原证明缓存。显式 Static 的 Size=0 观察语义保持，不套用无 Static MKV 的正数 Size 例外。
5. 115 决策请求只追加缺失参数，Emby fallback 独立保存。新增扩展名 stream / 固定 original 在补查、证明判断或 DirectPlay 失败时都保留原 URL、RawQuery、Range 和 Header，不采用补查返回的 DirectStreamUrl。
6. 只有旧 plain stream 的正常 fallback 使用严格验证后的 `DirectStreamUrl`，缺失或无效时使用 Emby 官方 Web 的 `/Videos/{Id}/stream.{Container}`；只有扩展名也无法安全形成时才使用补齐后的 plain stream。
7. 旧 plain stream 的 DirectStreamUrl 中 URL Token 全部删除，改用当前用户 Token Header；原 method、Range、应用 Header 和非播放身份参数继续保留。resolver 失败时不伪造参数，仅该旧分支可进入 4.1 的 Container 降级 fallback。

补查不自行产生或续期严格起播许可，首次转存仍由 strict/compatible 独立判定；HEAD 不能新增转存或租约。取消、发布令牌、停止隔离和固定会话 live binding 条件保持。

### 4.3 固定会话续播重新核验

成功 302 为所有已准入视频建立有界 session binding，初始 30 秒，只有成功找到租约的播放事件续期，GET 不续期。固定原文件或 static stream 的 proof 过期后，可以从同身份、item/source/session 的活跃关联发起当前用户 Token 的重新查询；无关联、过期或停止后仍回退。

补查保持原客户端会话，重新验证路径、容器、已知 Size 和播放能力，只发布本会话的证明别名，不重授严格起播许可。固定 session 与 binding 代次参与请求合并，其他会话不共享；停止、新客户端观察、媒体变化和全部等待者取消阻止旧结果发布。失败保留原始视频请求，不能混用 4.2 的 plain stream 补参数及 fallback URL 改写。

## 5. PlaybackInfo 短期证明

```mermaid
sequenceDiagram
    participant Client as Emby Client
    participant Gateway as ember-gateway
    participant Token as EmbyTokenService
    participant Emby as Emby
    participant Proof as 进程内证明缓存

    Client->>Gateway: GET/POST Items/{Id}/PlaybackInfo<br/>root 或 /emby 形态
    Gateway->>Gateway: 提取唯一且一致的 Header/query Token 来源
    Gateway->>Token: ResolvePrincipal(AccessToken)
    Token-->>Gateway: Principal
    Gateway->>Emby: 原请求透明转发
    Emby-->>Gateway: 200 application/json<br/>identity / gzip / deflate / br
    Gateway->>Gateway: 保留原响应并有界解码旁路副本<br/>解析 PlaySessionId + MediaSources
    Gateway->>Proof: 记录 mapping/item/mediaSource/playSession
    Gateway-->>Client: Emby 原响应字节
```

证明键固定为：

```text
mappingId + itemId + mediaSourceId + playSessionId
```

缓存包含 Principal 身份、`Path/Container`、Emby Size 观察值和 Direct Play 能力，TTL 5 分钟、全局最多 4096 条、每个 mapping 最多 64 条，仅当前 Gateway 进程可见。证明、独立起播许可、播放关联分别使用相同容量规则：优先淘汰当前 mapping 最旧记录；新 mapping 在全局满载时才淘汰全局最旧记录，相同时间按写入顺序处理。许可/关联满载不直接拒绝新记录；在途发布令牌仍沿用请求期容量保护，不能为腾位删除仍运行的 owner。Emby Size 不参与 proof 或路径解析；补查后仍无有效证明的合法请求 fallback Emby。进程重启会丢失 proof/live binding，固定会话冷请求不补查；客户端 PlaybackInfo 或符合 4.2 的无会话首请求可以重建证明。

## 6. 视频请求决策

```mermaid
flowchart TD
    A[GET / HEAD 固定视频路径] --> B{唯一且一致的 Header/query Token 来源?}
    B -- 否 --> R[reject<br/>401]
    B -- 是 --> C{ResolvePrincipal 成功?}
    C -- Token/身份失败 --> R
    C -- 用户硬状态/到期 --> R2[reject<br/>403]
    C -- 身份存储失败 --> R3[reject<br/>503]
    C -- 请求取消 / deadline --> R4[reject<br/>499 / 504]
    C -- Principal 合法 --> OD{符合 4.2 的无会话首请求?<br/>显式 Static 和唯一媒体源}
    OD -- 是 --> PI[当前用户 Token<br/>补取 PlaybackInfo]
    PI -- 成功 --> PK{原请求是 plain stream?}
    PK -- 是 --> AF[准备独立 Emby fallback<br/>DirectStreamUrl / stream.Container]
    PK -- 否 --> OF[只补齐 115 决策参数<br/>保留原客户端 fallback]
    AF --> D{固定静态播放形态?}
    OF --> D
    PI -- 失败 --> CR
    OD -- 否 --> D
    D -- 否 --> FB{已有权威 fallback?}
    CR{plain stream 缺 Container<br/>且条目快照可用?}
    CR -- 否 --> F[fallback 客户端请求到 Emby]
    CR -- 是 --> FC[追加 Container<br/>fallback 到 Emby]
    D -- 是 --> E{近期 PlaybackInfo 证明匹配?}
    E -- 否 --> RV{证明缺失或过期<br/>且有可信活跃会话关联?}
    RV -- 否 --> FB
    RV -- 是 --> RP[当前用户 Token<br/>重新核验固定会话媒体]
    RP --> RS{媒体一致且发布成功?}
    RS -- 否 --> FB
    RS -- 是 --> G
    E -- 是 --> G{Container 匹配且 DirectPlay 可用?}
    G -- 否 --> FB
    G -- 是 --> H[ResolveMediaPath]
    H --> I{115 全链路成功?}
    I -- 否 --> FB
    I -- 是 --> J[redirect<br/>空体 302 到 playback 直链]
    FB -- 是 --> FA[fallback 权威流地址到 Emby]
    FB -- 否 --> F
    F --> K[Emby 状态/Header/视频体透明返回]
    FC --> K
    FA --> K
    J --> L[客户端向 115 CDN 发 Range]
```

尝试 115 的请求当前必须同时满足：

- `GET` 或 `HEAD`；
- `/emby/Videos/{Id}/stream`、`stream.{Container}` 或 `{StreamFileName}`；
- 唯一非空 `MediaSourceId`；
- 唯一非空 `PlaySessionId`，由客户端提供或符合 4.2 的补查取得；
- 精确 `Static=true`；或满足[播放代理合同 5.1 节](./emby-playback-proxy-contract.md#51-senplayer-缺失-static-的证据边界)及 [5.2 节](./emby-playback-proxy-contract.md#52-senplayer-originalmkv-抓包证据2026-10-14)的受限 `original.mp4/mkv` 无 Static 请求（只有身份参数、有效本地对应容器证明，MKV 额外要求正数 Size）；
- 请求容器与证明中的 Container 匹配；
- 证明中的 MediaSource 支持 Direct Play。

其中 GET 会在 Provider 前申请 Redis reservation；HEAD 只有同一 session 已存在 `reservation|active|paused` 时才继续，未命中不创建租约、不调用 115，直接进入公共 fallback。

媒体解析与最终直链使用两层进程内缓存。媒体键按同一 Server/用户/登录映射/设备/媒体路径/真实 UA、两类账号配置与凭证、源/目标目录隔离，不包含 PlaySessionId；退出重播与新会话可命中，Stopped 释放租约并撤销当时对应的起播许可，不清媒体缓存。每个请求仍实时执行身份/证明、账号/套餐、Redis 准入与最终确认。媒体命中跳过源解析和目标查重，引用既有直链条目，返回 preexisting=true，不复用任务 ID 或转存用量；访问记录继续采样，双方账号都不写虚构的健康成功。未命中才进入下文完整查询/转存流程。

媒体缓存最多 1024 条，满时淘汰最早到期项；跨会话新鲜度截止于文件解析开始后 10 分钟与原直链缓存截止时间的较早值；同一仍 active/paused 的原播放会话可继续复用至直链安全截止时间。直链缓存仍为最多 1024 条 LRU、每条 URL 最大 16 KiB，截止于 `ExpiresAt - min(5m, 剩余寿命 10%)`，不再附加本地 10 分钟上限，也不再用固定 10 秒窗口。两层命中都不续期，不落 Redis/数据库；URL 淘汰或到期令媒体引用失效，时钟回退丢弃条目。跨会话最多延迟该短窗口后发现同路径文件变动；持续播放允许保持已验证文件身份至链接安全期限；配置/凭证变更隔离旧条目，任一账号恢复探测绕过媒体缓存。跨 session 同键昂贵处理串行，获锁后加载账号快照，错误/取消/最终租约确认失败不填媒体缓存。单条决策日志包含 mediaResolutionCache 和 downloadURLCache 的 hit/miss/bypass；媒体命中时没有源解析/查重/取链调用计数。具体合同见 [Cookie 合同 §9.1](./p115-cookie-playback-contract.md#91-同设备短期直链缓存ember-内部合同)。

这两层按用户/设备隔离的媒体和直链缓存，与下节的 source preID 成功哈希复用相互独立。低层 `Resolve` 缺设备/登录上下文时不使用前两层，仍可在独立转存准入后按[Cookie 合同 §9.2](./p115-cookie-playback-contract.md#92-preid-成功结果复用ember-内部合同)复用已验证源内容结果；它不复用用户、playback、任务、配额或租约。

单 Gateway 按同 session 串行执行候选、续租与清理；排队及主处理共用 2 分钟预算。GET 新建/复用 reservation 时刷新 30 秒有效期，处理期间每 10 秒续租，返回前原子确认并续租；HEAD 准入通过一次原子查询同时读取租约和账号/用户用量，返回前再次确认，但不创建或延长租约。active/paused 的到期仍只由播放事件管理。租约丢失/过期/Stopped 时不重新创建，固定 `playback_lease_lost` 回退；内部主处理预算耗尽为 `playback_resolve_timeout` 回退。客户端自己的取消/超时仍为 `499/504`，内部预算和续租错误不污染账号健康。所有后台续租都在请求收尾时停止并等待退出，之后才清理本次新建预留并放行下一请求。

HLS/DASH manifest、转码分片、不完整参数、未映射路径、账号不可用、Provider 错误、秒传失败、目标复核失败、链接不兼容等都进入 fallback，不拒绝合法用户。

其中按需 PlaybackInfo 成功时会补齐真实 PlaySessionId/Container，并可能获得 115 `302`；旧 plain stream 的失败分支使用与决策请求分离的权威 Emby fallback，新增扩展名 stream / 固定 original 则保留原请求。resolver 失败后，只有 plain `/stream` 再命中近期用户条目快照时才记录 `fallback/route/container_recovered`，该降级分支不尝试 115。

## 7. DirectPlay 保留式秒传

源目录解析在 Cookie Adapter 内合并时间重叠的相同账号、精确凭证和父目录请求，多个文件共享一次路径解析与完整分页，再分别校验文件名唯一性。完成即移除，不缓存后续播放；等待者独立取消，全部离开取消上游，总预算 30 秒耗尽映射目录超时并以 `source_read_timeout` 回退，不回写账号健康。该优化位于用户/账号/Redis 准入之后，不共享目标查重、转存记账或最终直链；详细隔离、日志及中间目录重名未验证边界见 [115 Cookie 合同](./p115-cookie-playback-contract.md#521-源文件路径解析)。

以下序列图描述媒体缓存未命中时的完整路径；媒体命中直接复用已验证结果并采样访问记录，跳过 Provider/transfer 步骤，再执行最终租约确认。

内容锁在同进程先按内容键排队，再竞争 PostgreSQL advisory lock；未取得锁的轮询归还 SQL 连接，有限池的在途取锁/持锁任务不超过 `MaxOpenConns - 1`，避免所有连接都被锁请求占用而无法执行任务 SQL。成功持锁仍固定物理连接；取锁响应未知或解锁失败时丢弃连接。进程内排队不替代数据库锁，也不删除锁内第二次查重。

```mermaid
sequenceDiagram
    participant Gateway as ember-gateway
    participant DP as DirectPlayService
    participant Accounts as P115AccountService
    participant Redis as Redis
    participant DB as PostgreSQL
    participant Provider as CookieProvider
    participant P115 as 115

    Gateway->>DP: ResolveMediaPath(Path, Method, identity, ClientUA, CanCreateTransfer)
    DP->>Accounts: ResolvePlaybackRoute(userId)
    Accounts-->>DP: 套餐 + personal/shared 非敏感元数据
    DP->>Redis: GET reservation / HEAD existing session
    alt Redis 不可用、账号已满或 HEAD 无租约
        DP-->>Gateway: 类型化 fallback 原因
    end
    DP->>Accounts: LoadActiveCredentialByRole(source) + AcquirePlaybackRoute(exact account/version)
    Accounts-->>DP: 解密后的窄 Credential + 目录配置
    DP->>Provider: ResolveFileByPath(source root + relative path)
    Provider->>P115: 父目录路径一次解析 + 最终目录文件唯一校验
    P115-->>DP: source fileId/pickCode/SHA1/size

    DP->>Provider: SearchBySHA1(playback target, SHA1, size)
    alt 目标已存在
        Provider-->>DP: 唯一精确目标文件
        DP->>DP: 检查同设备短期直链缓存
        opt 新转存、缓存未命中或需要真实探测
            DP->>Provider: GetDownloadURL(playback Cookie, ClientUA)
        end
        DP->>DP: 保存 preexisting 候选
    else 目标不存在
        DP->>DB: 获取 playbackAccountId + SHA1 + size advisory lock
        DP->>Provider: 锁内第二次目标查重
        alt 其他请求已完成
            Provider-->>DP: 目标文件
        else 仍不存在
            DP->>Gateway: 实时转存准入：原 proof + 明确许可或兼容策略
            break HEAD 或转存准入不通过
                DP-->>Gateway: playback_intent_required，释放本次新 reservation
            end
            DP->>Redis: ReserveTransfer(user, hourly/day limits, attempt)
            alt 配额已满或 Redis 不可用
                DP-->>Gateway: fallback，不调用 InitRapidUpload
            end
            DP->>DB: 创建独立任务并推进到 Initializing
            alt source 恢复探测、版本或身份不完整
                DP->>DP: preIDCache=bypass，不读取或合并缓存
                DP->>Provider: HashFileRange(source, 前至多 128 KiB)
                DP->>DP: 校验 SHA1 与字节数，不填缓存
            else source 完整身份可缓存
                DP->>DP: 查 preID 缓存；miss 时 gate 等待并复查
                alt preIDCache=hit
                    DP->>DP: 复用已验证哈希，不增加 preIDCalls
                else preIDCache=miss
                    DP->>Provider: HashFileRange(source, 前至多 128 KiB)
                    DP->>DP: 哈希/字节数有效且请求未取消才发布
                end
                DP->>DP: 若持有 preID gate 则释放，不等待目标上传
            end
            DP->>Provider: InitRapidUpload(preID)
            alt status=7 range challenge
                DP->>Provider: HashFileRange(source, challenge Range)
                DP->>Provider: InitRapidUpload(signKey/signValue)
            else status=1 普通上传
                DP->>DB: task=failed
                DP-->>Gateway: ErrRapidUploadUnavailable
            end
            DP->>Provider: FindTargetFile(parent + SHA1 + size)
            DP->>Redis: CommitTransfer(attempt, 独立 2s 重试)
            DP->>DB: task=succeeded + target provenance
        end
        DP->>DB: 释放 advisory lock
        DP->>DP: 检查同设备短期直链缓存
        opt 新转存、缓存未命中或需要真实探测
            DP->>Provider: GetDownloadURL(playback Cookie, ClientUA)
        end
        DP->>DP: 保存转存候选
    end
    DP->>Accounts: source 实际成功回写；仅实际取链时回写 playback succeeded
    DP->>Redis: 原子确认有效租约；GET reservation 续租
    alt 租约有效且身份一致
        DP-->>Gateway: RedirectCandidate
    else 租约丢失或 Redis 失败
        DP-->>Gateway: 清除候选 URL 并 fallback
    end
```

任一 Provider 调用返回账号级类型化错误时，DirectPlay 只把实际调用账号和固定 `credential_rejected/provider_unavailable/provider_rate_limited/provider_risk_control/provider_protocol` 失败结果交给 `P115AccountService`；请求取消、目标文件不可见、下载 Header 不兼容和其他文件级失败不污染账号健康。健康回写使用独立 2 秒上限，失败或旧结果被拒绝都不替换原始 DirectPlay 错误、302 或 Gateway fallback。

preID 缓存仅保留验证过的前至多 128 KiB 哈希与字节数，最多 1024 条 LRU，成功读取后固定 10 分钟且命中不续期；过期/时钟回退丢弃，无后台刷新。key 绑定 source 账号、Provider UID、正 `DownloadCacheVersion`、精确 Cookie、AppType/source UA 等 Provider 参数、文件 ID/pickCode/完整 SHA1/Size 与范围，不包含用户、playback 目标、设备、会话或播放器 UA。每个请求仍执行源路径观察和各自的权限、租约、目标双查、任务、配额、上传及复核；challenge 始终真实读取。

同键 owner 使用本请求 context，失败或取消不发布，等待者取消不影响 owner；后续等待者可用自己的 context 重新读取。成功 preID 可在目标上传前发布，后续目标失败不撤销已验证源哈希。命中不能伪造 preID 网络/账号恢复成功，source 实际路径成功的健康回写保持。实施与等工作量验证记录见[主计划第 8 节](../plan/architecture/gateway-client-compatibility.md#8-采集缓存诊断与-preid-复用2026-10-25)，fake 减少读取不证明真实命中率、首帧或起播延迟改善。

### 7.1 Transfer task 状态

```mermaid
stateDiagram-v2
    [*] --> pending
    pending --> initializing
    initializing --> challenging: status=7
    challenging --> initializing: challenge Hash 完成
    initializing --> verifying: status=2 reused
    verifying --> succeeded: 目标文件精确复核
    pending --> failed
    initializing --> failed
    challenging --> failed
    verifying --> failed
```

如果第一次查重直接命中外部预存文件，可以不创建 task；`preexisting=true` 仍允许播放。第一阶段所有 playback 文件都保留，Playing/Stopped/TTL 不调用 `DeleteFile`。

转存配额只在 advisory lock 内第二次查重仍缺失、且实时新增转存许可通过后申请。pending 固定 `5m` 且不续租；Provider 或目标复核失败尽力立即释放。目标复核成功后，同一 opaque attempt 必须先幂等写入 succeeded 才能继续下载 URL/302；pending 已过期仍补记并记录 `transfer_pending_expired_before_commit`。独立 `2s` 预算耗尽时记录 `transfer_quota_commit_failed`，保留 Provider 文件和仍存在的 pending、标记 task 失败并 fallback，不污染账号健康。

### 7.2 文件级失败与账号健康

单一且 pickCode 匹配的 downurl 条目返回 `url=false/null/缺失/空`，以及已识别的 `70005/231011/50003/50015`，映射为 `ErrFileUnavailable`；过期直链与空/过期/负并发候选同样按 `file_unavailable` fallback，不回写账号故障。`99/990001` 才按已列出的兼容表判为凭证失效；这些 errno 依据公开实现，**未经真实账号证实**。未知 errno 保持 `ErrProviderRejected`，不推断限流或风控。

Service 的秒传业务结果拒绝、目标复核不符、无效/重复 challenge 或查重候选不符只失败当前文件/任务；后两类仍可能使用 `provider_protocol` 作为 fallback 原因，但没有账号 `provider_protocol` 回写。真正返回的 `ErrProviderRejected/ErrProviderProtocol` 则按操作分类：源路径与源 preID/challenge 隔离，其他 playback 操作保留既有账号 `error` 语义，不能写成所有协议错误都被忽略。

downurl 和通用 GET 的响应体读取中断在调用方 context 有效时是 `ErrProviderUnavailable`，沿用 1 分钟冷却；context 取消/deadline 保留原错误。HTTP `429` 保留默认 15 秒限流冷却，裸 HTTP `403` 保留 1 分钟临时不可用冷却，不把它们与未知专有业务码混为一谈。完整映射见 [Cookie 合同 §5.5.1](./p115-cookie-playback-contract.md#551-downurl-文件失败与-errno未经真实账号证实) 和 [运行期失败分类](./p115-cookie-playback-contract.md#102-运行期失败分类)。

## 8. 302、fallback 与日志

### 8.1 302

- 只允许 playback 账号的直链成为客户端 `Location`。
- source 下载 URL 只允许在 Provider 内执行 preID/challenge Range。
- 候选必须是 HTTPS、允许的 hostname、未过期、并发信息有效（`c=0` 表示 115 CDN 无限打开，不改 Ember Redis 路数），并且 HeaderMode 只允许 `none` 或 `same_user_agent`。
- `same_user_agent_and_cookie` 首期不兼容，不能向播放器泄露 playback Cookie。

### 8.2 fallback

115 fallback 按 4.2 区分请求形态：旧 plain stream 可使用补查后的权威 Emby 地址，新增扩展名 stream / 固定 original 保留原 URL/RawQuery；method、Range、If-Range、实际视频 User-Agent 和应用认证 Header 继续透传。上游 200/206/416、ETag、Last-Modified、Content-Range 与 HEAD 无正文语义保持，Gateway 不自行实现条件 Range。fallback 决策只表示请求已交给 Emby，不代表客户端已经完整播放。

### 8.3 单条决策日志

新增 `transferTrigger` 固定枚举：`explicit_playback` 为明确起播准入、`compatible_request` 为兼容策略准入、`existing_target` 为不经过新转存准入的目标复用、`policy_denied` 为策略拒绝。只有经过对应分支时输出，不把配置读取异常伪装成 115 账号故障。Debug `transfer_policy_unavailable` 定位模式读取失败；`playback_info_revalidation_started/revalidated/revalidation_failed` 关联可信固定会话的重新核验，不代表客户端已出画。

2026-09-08 起，实际进入 DirectPlay 的请求在同一条最终决策日志附带分阶段耗时，默认 Info 可见，失败和取消也保留已执行阶段；未执行阶段不输出，亚毫秒操作可显示 `0`，用 `Calls` 区分是否执行。字段全部为固定名称与数值，不增加第二条 Info 决策日志：

| 字段 | 统计范围 |
| --- | --- |
| `directPlayMs` | DirectPlay 入口到返回，包括错误返回；不包含 Gateway 身份验证、按需 PlaybackInfo 或 Emby fallback |
| `prepareMs` | DirectPlay 内路径映射、账号/路由加载及播放租约准入 |
| `sourceResolveMs` | 当前调用等待源文件解析的时间，包括共享目录等待与最终文件匹配 |
| `targetSearchMs` | playback 目标查重累计耗时；锁内外重复查询累计计数 |
| `lockWaitMs` | 获取内容锁的等待时间，不是持锁时长 |
| `preIDWaitMs` | 当前请求等待同源 preID gate 的时间，独立于实际源读取；可能等待后命中缓存 |
| `preIDMs` / `challengeMs` | 当前请求实际执行的源 preID / challenge Provider Range Hash，各自包含内部取链与读取范围；preID hit 不产生 `preIDCalls`，challenge 不缓存 |
| `rapidUploadMs` | 秒传初始化与 challenge 后重试累计，包含 Provider 内上传信息读取 |
| `targetVerifyMs` | 秒传后目标复核，包括 Provider 的可见性轮询等待 |
| `transferCommitMs` | Redis 转存成功记账及其有限重试累计 |
| `downloadURLMs` | 最终 playback 直链获取 |
| `otherMs` | DirectPlay 总耗时减上述互不嵌套阶段，包括任务持久化、健康回写、其余转存准入/清理及本地编排；不能直接认定为数据库耗时 |

各阶段同时输出同前缀 `Calls`，例如 `targetSearchCalls=2`、`rapidUploadCalls=2`；这些是业务操作调用次数，不能当成 HTTP 请求总数。耗时使用单调时钟，单位毫秒，分别取整可能使展示值相差少量毫秒。既有 `durationMs` 仍是 Gateway 请求处理时长，fallback 时可能包含媒体代理传输，不能当作首帧时间，也不能把它与 `directPlayMs` 相加。

最终决策保留 `mediaResolutionCache/downloadURLCache=hit|miss|bypass`，另有 `preIDCache=hit|miss|bypass`。前两层只有 miss/bypass 输出固定 `mediaResolutionCacheReason/downloadURLCacheReason`，hit 不带 reason；preID 没有独立 reason 合同。完整枚举见[Cookie 合同 9.1](./p115-cookie-playback-contract.md#91-同设备短期直链缓存ember-内部合同)，其中 missing 只证明当前条目不存在，不能推断配置变化或容量淘汰。

`mediaCacheEvictions/downloadCacheEvictions` 只在本请求实际触及对应 cache 时输出，表示收尾时的进程累计容量淘汰快照；过期、时钟回退和显式移除不计入。缺失不等于零，不同请求快照不能求和或关联到某次 miss 的原因；采集摘要只给 `maxObserved`，`intervalDelta=unknown`。preID 没有容量快照输出。上述诊断不改变媒体/直链缓存的 key、容量、TTL 或门控。

排查 sourceResolve 耗时时，开启 Debug 后查看 `code=source_directory_resolved` 的 `parentResolveMs/listMs/listPages` 及 `shared/callers/entries/durationMs/success`。该明细每个共享任务一条，`listPages` 为实际尝试的列表请求数（含失败页），根目录文件的 `parentResolveMs=0`；不可把共享任务耗时乘以等待者人数视为串行耗时。明细不输出路径、凭证、Provider ID 或摘要。

排查 preID/challenge 耗时时，Debug `source_range_completed` 的 `sourceDownloadMs` 是源取链阶段（含排队），`rangeReadMs` 是实际 CDN 请求与读取阶段，`durationMs` 是整次 Hash 读取。`provider_operation_completed stage=download_url` 与 `stage=source_range` 各自记录本次 HTTP trace，CDN 的 TLS/首字节字段不会继承源取链回调。取链失败时 `rangeReadMs=0` 且没有 CDN 请求；仅凭相邻日志不能建立并发请求的归属。上述时间已经包含在 Gateway 最终 `preIDMs/challengeMs` 内，不能与它们重复相加。

秒传初始化另有 `provider_operation_completed stage=rapid_upload`，只描述实际上传 POST 的 trace；上传信息 GET 仍是独立 `cookie_get`。失败阶段与脱敏协议证据沿原链路传递，不能把整次 `rapidUploadMs` 与这些 HTTP 明细重复相加。

上游代理取消时，视频最终记录使用 Info `code=playback_canceled result=canceled statusCode=499 proxyErrorCode=request_canceled`，保留原有 decision/stage/reasonCode，不把取消写成上游故障或回退成功。普通代理请求的取消只记 Debug `upstream_request_canceled`。这不改变 DirectPlay 在代理前因取消/deadline 直接终止的边界。

每个固定视频请求在默认 Info 只打印一条播放决策；设置中心数据库项 `LOG_LEVEL=debug` 时额外打印入口、过程与统一的 `request_completed` 请求摘要，API 保存后由 Gateway 最多在 5 秒内从进程缓存刷新生效：

- `requestId` 是 Gateway 进程随机前缀与原子序号生成的请求标识，不采信客户端 Header；贯穿视频入口、DirectPlay 阶段、回退、最终决策与请求完成。`sessionRef` 是带进程随机种子的 Server/用户/映射/设备/PlaySessionId 摘要，关联视频与播放事件；缺失身份为 `unavailable`，进程重启后不可关联，不使用原始会话或 Redis Key。
- `video_request_started` 在视频身份校验及上游调用前记录 method/path、规范化 `rangeKind=none|bounded|open|suffix|multiple|invalid` 与合法数值，`purpose/secPurpose=none|prefetch|other` 仅为观测，绝不作为拦截依据。非法、超长和多 Range 不输出原文，日志不读取未认证请求体。
- `playback_info_started/finished` 包含按需播放信息准备的时间；`video_context_resolved` 标记 `playbackContext=client|supplemented|unavailable`，播放证明不是用户点击播放的证据。
- `direct_play_step` 使用固定 step 与 phase：远端阶段复用既有 sourceResolve/targetSearch/lockWait/preID/challenge/rapidUpload/targetVerify/transferCommit/downloadURL；增加 sourceLocation/sessionWait/mediaWait/downloadWait/routing/leaseAdmission/accounts/leaseConfirm/accessTouch/transferAdmission/taskBegin/healthUpdate，并以 preIDWait 单独观察源哈希串行等待。started 在调用前输出，finished 只表示返回，不表示成功；总结果仍以最终决策为准。preIDWait 独立累计且不计入 preIDMs，其余等待与准备步骤不新增累计耗时列，避免与 prepareMs 重复计数。leaseAdmission 另记录 created/reused；mediaResolutionCache/downloadURLCache/preIDCache 记录 hit/miss/bypass，命中不会虚构 Provider 调用。
- `video_fallback_started`、`video_fallback_headers`、`video_fallback_completed` 分别记录开始、上游响应头到达和代理退出；后两者 durationMs 均从本次回退开始计算。未收到响应头时没有 headers 日志且 upstreamStatus=0；completed 不等于客户端成功播放。原最终 fallback 决策仍由响应头或错误 hook 单次输出。
- `playback_event_received` 记录通过身份校验后的事件类型与 snapshotState；`playback_lease_update_started/updated/not_found/skipped` 关联 requestId/sessionRef，成功包含当前状态与账号/用户 reserved/active/occupied。错误仍为 Warn，并补关联标识；不输出事件原始 body、原始 PlaySessionId、Token、Cookie、直链或未知错误内容。

```text
level=info code=direct_play_redirect message="115直链成功" result=success statusCode=302 target=p115 targetState=created|reused
level=info|warn code=direct_play_fallback message="115直链失败，Emby回退成功|失败" directPlayResult=failure fallbackResult=success|failure fallbackTarget=emby
level=info|warn code=direct_play_fallback message="无起播许可，跳过新增转存；Emby回退成功|失败" directPlayResult=skipped fallbackResult=success|failure reasonCode=playback_intent_required
level=warn code=playback_rejected message="播放请求已拒绝" result=rejected
```

上述结果及取消记录继续保留 `decision=redirect|fallback|reject`、固定 `stage/reasonCode` 和必要请求标识，便于机器检索。直链成功时 `targetState=created|reused` 明确区分首次转存与已有文件复用；DirectPlay 失败时仅允许记录固定 `providerOperation=resolve_source_path|hash_source_preid|rapid_upload|hash_source_challenge|rapid_upload_retry|verify_playback_target|search_playback_target|get_download_url`，账号加载失败只记录 `accountRole=source|playback`。这些字段来自内部类型化诊断，不接受 Provider 原始错误。

缺少起播许可仅标记新增转存 skipped，不表述为 Provider 故障；最终 `fallbackResult/statusCode/upstreamStatus` 和 info/warn 仍反映真实上游响应，不统一隐藏 404 或连接错误。

进入套餐路由后，最终同一条日志补充 `playbackMode`、`playbackAccountOwner`、账号配置/有效并发、账号和用户的 reserved/active/occupied；个人模式额外记录原始 `simultaneousStreamLimit`。只有实际进入转存准入且 Redis 返回有效快照时才记录小时/每日 used/limit，Redis 故障不伪造零。日志边界不接收原始 Provider UID、PlaySessionId、Redis Key 或 `transferAttemptId`。

新转存另在 Redis 提交返回后记录一条 Info 业务事件 `transfer_quota_committed`，包含 `userId/taskId/playbackAccountId`、`added`、`pendingExpiredBeforeCommit`、`transferPending/transferHourlyUsed/transferDailyUsed`。`added=true` 表示本次幂等 attempt 首次记入；`added=false` 表示已经记入，不应再次增加。该事件只证明 Redis 提交完成，不代表后续任务写库、取链或客户端播放成功；预存命中和缓存复用不产生新的提交事件。提交预算耗尽时 Warn `transfer_quota_commit_failed` 关联相同用户与任务，并固定 `usageAvailable=false`，不打印未确认的用量。

API 成功读取控制台用户用量时，Debug `p115_user_usage_read` 记录 `userId/playbackMode/usageAvailable`、用户 reserved/active/occupied、转存 pending/used/limit、`businessTimezone` 及 `sampledAtUnixMs/dayStartUnixMs/dayEndUnixMs`。时间来自本次读取时钟和全局 `CRON_TIMEZONE`；按相同用户及相近时间对照 Gateway 日志。读取失败仍返回 unavailable/null，并沿用读取失败日志，不生成成功零值快照。详情摘要与独立用量请求会分别读取，日志是各次读取的快照，不承诺跨请求原子性。

Debug 请求摘要记录有界 method/Host/原始 request path、query key、route、status/outcome/耗时和脱敏认证 Header 形态；Info 对响应级合同成立后的每个唯一 MediaSource 记录 `playback_info_media_source_observed`，完整显示合法 `mediaPath`、Size/播放能力和 proof 接受/拒绝原因。按需 PlaybackInfo 选中的路径即使没有形成 proof 也进入最终决策；真正进入 DirectPlay 后再记录 `embyPathPrefix/sourceRootId/mappedRelativePath`，以核对 Emby 原路径和 115 source 映射。Emby fallback 记录 `fallbackTarget/fallbackSource/upstreamStatus/proxyErrorCode`。Debug 不重复生成第二条决策。所有日志仍禁止 query value、Header 原值、Token、Cookie、完整 SHA1、115 URL、PlaybackInfo 原文或上游原始错误。当前明确不建日志表。

仓库外采集产物使用更窄的逐 code 过滤，新增 `diagnosticSummary` 将认证、图片、回退、耗时和缓存结果按已观察证据汇总。应用日志只在唯一且一致的 requestId 内关联，冲突标记歧义；图片 `upstreamStatus` 保持 unknown，登录上游 200 不证明本地 Token 映射成功。PCAP 与 Gateway 日志不能按时间相邻自动合并，包端响应头与应用阶段耗时分别保留，不相加推断首帧；字段和使用边界见[采集手册](../runbooks/gateway-packet-capture.md#4-离线分析与对照)。

## 9. 数据与秘密边界

| 数据 | 存放位置 | 禁止位置 |
| --- | --- | --- |
| 115 Cookie 明文 | 创建/替换请求与调用栈内存 | 数据库、响应、日志、前端持久状态 |
| Cookie 密文 | `p115_accounts.cookie_ciphertext` | JSON 响应 |
| Emby AccessToken 明文 | 客户端 Header、Gateway 调用栈 | 数据库、日志、管理页面 |
| Token HMAC | `emby_access_tokens.token_hash` | JSON、日志、客户端 |
| Media Path | PlaybackInfo 进程内证明、DirectPlay 调用栈、已授权的持久观察/决策日志 | 数据库、公开 API、完整外部响应体 |
| SHA1 | Provider/DirectPlay、`playback_transfer_tasks` | JSON、普通日志与 URL |
| pickCode/fileId | Provider、成功 task provenance | 客户端响应、普通日志 |
| 115 下载 URL | 当前请求内存与 302 `Location` | 数据库、日志、API JSON |
| source Range 字节 | Provider HTTP body 内部 | Service、数据库、日志 |
| Gateway 请求摘要与播放决策 | 脱敏应用日志 | 数据库表 |

## 10. 当前验证证据

| 层级 | 已证明 | 没有证明 |
| --- | --- | --- |
| Go 单元/fake HTTP/进程内 fake Redis | 管理员/个人账号生命周期、personal/system 路由、Cookie/会话用途隔离 HMAC、账号/用户 leases+active、`30s/2m/15m` 状态、HEAD 不创建、断连 fallback、小时/自然日配额、pending/succeeded、晚到成功和 2s 提交预算；source 父目录 `/files/get_path_id` 一次解析、最终目录分页与文件唯一性；Provider/Token/Gateway 既有合同 | 真实 Redis 服务、目标账号 `/files/get_path_id` 响应与非零 `parent_id` 行为、个人 Cookie 固定 UA、真实 personal/system 播放与配额边界 |
| PostgreSQL 集成 | 既有账号/transfer/Token migration、唯一约束、advisory lock、并发只秒传一次、共享冷却和旧 Cookie 结果丢弃已有历史证据；2026-09-05 已在专用 `EMBER_INTEGRATION_DATABASE_URL` 环境执行并通过 `go test ./internal/app -run 'Integration|PostgreSQL|P115' -count=1 -v`，本轮新增 personal migration 用例覆盖幂等、默认 personal、owner/Provider/共享角色 unique、RESTRICT、tombstone 与 transfer provenance | 不验证真实 Redis、真实 115 或多 Gateway 部署语义 |
| 2026-08-22 受控 115 检查 | source 只读、一次 challenge 秒传、目标复核、playback downurl/128 KiB Range、preexisting 复跑、文件保留 | Gateway/Infuse 端到端播放 |
| GitHub Actions 预览构建 | 单 `ember` 二进制 API 镜像可实际构建和推送 | 目标部署网络与原始 Emby 隔离 |
| Gateway/Infuse | 2026-08-23 已确认登录/普通资源 `200` 与按需 PlaybackInfo `proofCount=1`。2026-08-29 Infuse `8.5.2` 的 `Size=0` 条目确认 `proofAccepted=true`、source 路径映射、Provider Size 转存成功，并由 Gateway 首次及多次复用返回 `302`。2026-08-31 macOS Infuse `8.5.2` 进一步确认 `path_not_mapped` 条目经权威 Emby 扩展名 fallback 返回 `206` 并实际播放，115 首次/复用 `302` 可播放，外挂/内嵌字幕及 Playing/Progress/Stopped `204` 均正常 | 115 CDN 完整响应头/Range/全文件字节、UA/IP 绑定和长期 Provider 风控；生产未故障注入 `providerOperation/accountRole`，只保留 fake 测试证据 |

自动化测试不得请求真实 Emby/115。真实验证必须使用测试账号/文件并取得明确授权，不能把 fake、数据库或一次性 Provider 检查表述为 Infuse 已可用。

2026-10-25 首轮三批次基线为 `master@30fb9a4d`，后续协议增量已完成认证 scheme、复杂 PlaybackInfo/多源、实际视频 UA、条件 Range、非加速媒体代理与无会话首请求补查的定向回归，以及全量非数据库 Go 测试、Gateway/DirectPlay/EmbyToken race、vet/build 和独立代码审查；记录见[主计划第 7 节](../plan/architecture/gateway-client-compatibility.md#7-后续协议兼容改进2026-10-25)，入口见[测试指南](../runbooks/testing.md)。普通字幕/HLS/Audio/Download/Sync 只验证受保护透明代理，Audio/Sync 不增加专用能力，Download 不证明新增套餐权益；本轮不增加 Web UI、配置、migration 或 CORS，未部署，不把历史 Infuse/SenPlayer 实测扩展为这些新增形态已验收。

同日基于 `master@8799688` 的采集/缓存/preID 优化已完成身份/并发/失败回归、全量非数据库 Go 测试、DirectPlay/账号/配额/115 Adapter/Gateway 五包 race、vet/build 与独立审查。两个用户、两个个人目标的顺序和重叠 fake 均验证 preID 读取 `2→1`，source 解析、目标双查、两次独立上传/复核/取链、两份任务和每用户一次配额保持；完整计数见[Cookie 合同 9.2](./p115-cookie-playback-contract.md#92-preid-成功结果复用ember-内部合同)。Python `3.9.6` 在原目录及独立复制目录的 39 项离线测试、10 个源文件编译/AST，以及 Go 实际日志字段→Python 过滤/摘要串接复验通过；结果见[主计划第 8 节](../plan/architecture/gateway-client-compatibility.md#8-采集缓存诊断与-preid-复用2026-10-25)。未部署或请求真实 Emby/115/数据库，这些结果不证明真实命中率、首帧或起播延迟。

2026-10-17 二轮修复的自动化证据：`080be97`（A）、`da4d64a`（B）、`d3f6fed`（C）对应工作区均通过 API `go vet ./...`、`go test ./...`、`go build ./...`；B 的 Gateway、C 的 Provider/账号/DirectPlay/Gateway 另有 race 验证。新增 fake/SQL mock 覆盖文件错误与健康隔离、errno、断连后停止释放、设备与凭证合并键、每 mapping 配额及淘汰、假时钟阶段预算、取消日志、秒传观测、UID 桶与提交后清理。该批验证未启用真实 PostgreSQL 集成，也未请求真实 Emby/115/Redis 或播放器，不能并入历史真实验收结论。

## 11. 系统性审查结论

【品味评分】

🟡 凑合。用户自有账号、套餐来源、Redis 租约/转存配额、HEAD 收口和管理员目录路径配置已经完成代码、fake 测试、race、静态检查和构建；新增 PostgreSQL migration 已在专用集成库实际执行并通过，本轮未发现新的 P0/P1。个人 Cookie/Redis/客户端真实链路和 CDN 完整响应合同仍未验证。

【致命问题】

- `P1-1` 已关闭：2026-08-31 本地媒体扩展名 fallback 已取得上游 `206` 并由 Infuse 实际播放，字幕和 Playing/Progress/Stopped 同步通过。
- `P1-2` 按部署确认关闭：部署管理员确认原始 Emby 公网隔离已处理；该项属于运维确认，不表述为 AI 独立网络复验通过。

【改进方向】

- 后续只把未验证边界作为受控验收：真实个人 Cookie 默认 UA、单 Gateway Redis 断连/配额与播放器事件时序、115 CDN 完整响应头/Range/全文件字节。主动健康告警和容量治理仍是独立后续能力。
- 生产不为归档主动制造 Provider/账号故障；`providerOperation/accountRole` 继续由 fake 合同保护，出现自然故障时再补实机证据。

问题总表：

| 优先级 | 问题 |
| --- | --- |
| `P0` | 本轮未发现 P0 |
| `P1` | `P1-1` 权威 Emby fallback 已以 `206` 实机关闭；`P1-2` 原始 Emby 旁路风险按部署管理员确认关闭 |
| `P2` | `P2-1` 账号运行期健康与共享冷却已关闭；`P2-2` 用户账号路由与 Redis 活跃/配额已完成自动化实现；`P2-3` HEAD 探测副作用已关闭；`P2-4` 保留文件无容量治理；`P2-5` 两次历史 Store error 根因待新日志复验 |
| `P3` | `P3-1` 管理员 playback 目录路径配置已关闭 |

### P1

#### 【P1-1，已关闭】本地视频权威 Emby fallback 已返回 206

- 原触发条件：Infuse 直接请求没有 Container/PlaySessionId 的 plain stream，历史 plain fallback 由目标 Emby 返回 `404`。
- 关闭证据：2026-08-31 本地硬盘条目因 `path_not_mapped` 进入 `fallbackSource=playback_info_extension_stream`，连续 Range 请求由目标 Emby 返回 `206`，macOS Infuse `8.5.2` 实际播放成功。
- 定位：`services/api/internal/playbackgateway/playback_info_resolver.go`、`video.go`、`docs/reference/emby-playback-proxy-contract.md`。
- 当前边界：这里关闭的是“本地媒体条目经权威 Emby fallback 播放”。Gateway `302` 与用户实际播放观察不替代 115 CDN 完整响应头、Range 和全文件字节取证。

#### 【P1-2，按部署确认关闭】原始 Emby 公网入口未隔离时可以绕过 Ember 门控

- 触发条件：客户端仍能直接访问原始 Emby 地址。
- 实际后果：本地 Token 撤销、用户硬状态和 Gateway 日志全部可以被绕过。
- 定位：`infrastructure/docker/docker-compose.yml` 只提供 Gateway 回环端口；外部防火墙/反向代理隔离必须由部署者完成。
- 建议：Infuse 验收前先确认公网只有 Gateway 域名，原始 Emby 只允许内网或运维 allowlist。
- 关闭记录：2026-08-31 部署管理员明确确认原始 Emby 公网隔离已处理并批准相关 Gateway 计划归档；这是运维确认，未提升为 AI 独立网络复验证据。

### P2

#### 【P2-1，已关闭】播放运行期已回写账号失效、冷却和成功状态

- 关闭实现：`directplay.Service` 通过窄 `AccountHealthReporter` 只回传实际调用账号和六种固定结果；成功按 1 分钟采样更新 source/playback `lastSucceededAt`（首次及恢复立即写入），凭证失效进入 `expired + disabled`，临时不可用进入 1 分钟 `cooling_down`，HTTP `429` 默认 15 秒限流冷却，风控哨兵保留但裸 HTTP `403` 按 1 分钟临时不可用处理，账号级协议错误回写进入 `error`。本地 `MaxWait` 拒绝（入队估计或实际排队到期）不回写健康。
- 并发边界：冷却期间不读取 Cookie；到期后 `AcquireRuntimeByRole` 在 PostgreSQL 行锁内续租并只放行一个半开探测。回写同时匹配 Cookie 密文、`config_version` 和 `updated_at`，旧请求不能覆盖 Cookie 替换、显式验证、手工启停或更新后的健康结果。
- 失败边界：2026-10-17 已将识别到的文件不可用/过期下载候选、秒传业务结果拒绝、目标不符与文件级 challenge/候选校验从账号故障中隔离；详情及仍保留的协议错误语义见 §7.2。请求取消、本地预算和 MaxWait 超限不写故障；源路径及源 preID/challenge 的业务/协议失败也不把源账号置为 `error`。downurl/通用 GET 读取中断且 context 有效时仍进入 1 分钟临时冷却；明确凭证失效、HTTP `429` 与裸 `403` 按原合同处理。fallback 的 `provider_protocol` 原因不等于账号已被写为 `error`。健康回写使用独立 2 秒上限，失败不改写原始 302/fallback 结果。
- 验证证据：既有 Go/独立 PostgreSQL schema 证据覆盖历史四类映射、冷却阻断、半开单探测、成功恢复、凭证停用及旧 Cookie 结果丢弃。2026-10-17 新增分类、预算、租约、缓存与清理修复由上述 fake/race/SQL mock 验证，不代表这批改动已实测 PostgreSQL 或真实 115；生产自然故障的冷却时长仍待观察。

#### 【P2-2，自动化已关闭】用户自有账号路由与 Redis 活跃/转存配额

- 关闭实现：`p115account` 已提供 owner/tombstone/个人控制面与同事务路由元数据；`p115quota` 已提供 Lua/Sorted Set 租约和 pending/succeeded 配额；DirectPlay/Gateway 已接入准入、事件、成功记账、fallback 和固定日志；API/Web 已展示 null/zero 语义。
- 自动化证据：关键包单元和 race、API 全量 test/vet/build、Web 全量 test/build、Compose YAML 静态合同均通过；外部依赖只使用 fake/miniredis。
- 未关闭边界：真实 Redis、个人 115 Cookie 和播放器事件/配额验收未执行；新增 migration 已在专用 PostgreSQL 集成库实际执行并通过。

#### 【P2-3，自动化已关闭】HEAD 无租约不再进入完整 DirectPlay

- 当前实现：HEAD 先按用途隔离 fingerprint 原子查询已有反向 session 与账号/用户用量；未命中直接返回 `head_lease_missing` 并进入公共 fallback，不创建 reservation、不加载 Cookie、不调用 Provider。命中时才继续，返回候选前再次确认；HEAD 全程不续租。
- 证据边界：fake Gateway/DirectPlay 测试已覆盖；真实 Infuse HEAD/GET 顺序仍未重新取证。

#### 【P2-4】playback 文件无限保留但没有容量治理

- 触发条件：长期播放大量新内容。
- 实际后果：playback 专用目录持续增长；虽然不会误删正在播放的文件，但最终可能耗尽账号容量。
- 定位：DirectPlay 生产接口刻意不包含 `DeleteFile`，Stopped/TTL 没有清理调用方。
- 建议：保留首期“不自动删”不变；阶段 2 基于 `lastAccessedAt + 无活跃会话 + 容量水位` 做串行清理，默认 dry-run，并要求明确 provenance。

#### 【P2-5】两次历史 Token Store error 的底层原因仍需新日志复验

- 触发条件：Infuse 并发扫描 Items/Latest 时，旧日志分别在 `find_mapping` 和 `find_user_by_id` 记录 `*errors.errorString`。
- 实际后果：对应请求被误写为 `503 token_store_unavailable`；旧日志无法区分客户端取消、deadline、坏连接或未知存储错误。
- 定位：`services/api/internal/services/embytoken/store.go`、`services/api/internal/playbackgateway/gateway.go`。
- 建议：部署新分类后观察固定 `reasonCode` 与 pool 统计；`context_canceled/deadline_exceeded` 不再算 Store outage，只有最终真实存储错误才继续按 `503` 处理。没有新运行证据前，不把历史两次失败断言为连接池耗尽。

### P3

#### 【P3-1，已关闭】playback 目标目录不再要求管理员填写内部 ID

- 关闭实现：管理员创建 playback 不再强制内部 ID；验证后通过 `PUT .../:id/playback-config` 完整提交已有路径和最大路数，后端解析目录并以凭证/版本条件原子保存 path/ID/并发。Web 只展示路径和用量。

## 12. 建议的后续顺序

1. 需要数据库复验时，在专用 `EMBER_INTEGRATION_DATABASE_URL` 隔离库重跑既有 migration/约束及相关生命周期用例；A–C 没有新增 schema，tombstone 提交后 UID 清理的 SQL mock 不能代替真实 PostgreSQL 验证。
2. 用户明确授权后，再验证个人 Cookie 固定 UA、单 Gateway Redis 断连/恢复、HEAD/Playing/Progress/Stopped 与 `5/10` 配额边界；日志不得携带凭证、UID、session、Redis Key 或 attempt ID。
3. 独立推进 115 CDN 完整响应合同、保留文件容量治理和主动账号健康告警；不得把这些后续项塞回当前 Redis 配额状态机。
