# Bot 通知与信息泄露加固方案

> 状态：已归档（主干已落地，稳定结论已下沉）
> 负责人：Ember
> 更新时间：2026-04-30

## 落地进度

批次 0（commit `71a61f3`）已完成 `SafeGo` 收口；批次 1（commit `61cd9b2`）+ 后续 review 修复已完成本方案的 P0 / P1 主干项：

- ✅ `VerifyBind` 命中 `len > 1` 时记 ERROR 日志（`code / count`）+ 改返回 `ErrTelegramBindCodeInvalid`，反 DoS：避免攻击者借"绑定码碰撞"造成全量用户绑不上
- ✅ `GenerateBindCode` 改用 `Clauses(clause.OnConflict{Columns: userId, DoUpdates: code/expiresAt/createdAt}).Create` 原地刷新；新增 `infrastructure/database/20260425_02_telegram_bind_codes_user_unique.sql` 创建 `uq_telegram_bind_codes_user` 唯一索引；migration 内置 CTE 在建索引前去重旧绑定码（每个 userId 仅保留最新一条），避免 CREATE UNIQUE INDEX 因历史脏数据失败连带 API 启动期 VerifySchema 停摆；schemaFingerprintIndexes 已追加
- ✅ Handler 层 `GetAccountInfo` / `RedeemByTelegram` / `ResetPassword` / `SubscribeByTelegram` 在 `ErrTelegramNotBound` 命中时统一返回 400 + `请求参数错误`，反 Telegram→Ember 绑定枚举
- ✅ `PaymentSuccessNotification` 删除 `Email` / `StripeSessionID` / `StripePaymentIntentID` 三个字段；`payment service` 同步去赋值；Bot Python `format_payment_message` 去渲染 + 单测同步
- ✅ `runtime_settings_service` 失败保留旧值，不再把有效配置覆盖为空值
- ✅ `pending_reject_requests` 已补服务端持久化主链路；`subscriptionId / messageId / hasPhoto / originalText` 可覆盖 Bot 重启或滚动发布后的待输入状态恢复
- ✅ `polling` 模式已补数据库租约锁：启动前申请、运行中续租、失败时主动停止 polling
- ✅ Bot API `httpx` 客户端已补 `limits`、重试和更完整的失败日志
- ✅ `BotNotifier` 已补进程内配置缓存与刷新节流，不再在每次 `IsConfigured` / `post` 时重建 `ConfigService` 查库
- ✅ Bot 通知 formatter 已补 Telegram 文本 / caption 长度治理；长标题、长备注、长拒绝原因统一截断，避免通知因超长载荷失败
- ✅ `BotNotifier.post` 已补 `event / payloadSize / requestId / latency` 结构化日志，便于追踪通知失败链路

归档时保留的边界说明：

- 搜索会话 `message_id` 仍是进程内 TTL 缓存；当前已明确这是私聊交互态边界，而不是需要继续持久化的缺陷
- 外部 metric/告警平台接入仍可继续补，但当前主链路已通过 `/health` 暴露 webhook 未注册状态，这类增强不再阻塞主计划退场

## 归档判断

- 当前已完成归档迁移。
- 原因：主干实现已经稳定，稳定结论已下沉到 `docs/system-architecture.md`，入口索引也已同步；本文只保留历史追溯价值，不再承担现行规则说明。

## 稳定结论

以下结论已经稳定，可视为当前基线，而不是临时整改步骤：

- API 侧所有关键 fire-and-forget 调用都已统一改走 `internal/async.SafeGo(name, fn)`，panic 不再打死主进程。
- `BotNotifier` 已收口到进程内共享单例，发送日志包含 `endpoint / event / payloadSize / requestId / latency`。
- `pending_reject_requests` 已服务端持久化到 `bot_pending_reject_requests`，审批消息 `messageId` 也已持久化，用于保护 Bot 重启或滚动发布期间的拒绝原因待输入状态；搜索交互 `message_id` 则明确保留为 10 分钟 TTL 的私聊会话态边界。
- Polling 模式已通过数据库租约锁强制单实例；续租失败时实例主动停止 polling。
- webhook 注册当前采用有限重试策略：最多重试 `6` 次，失败后记 `ERROR` 并停止继续重试；`/health` 在 `webhook` 模式下会暴露 `degraded` 状态和最后错误。

## 背景

2026-04-25 系统性 review 在 Telegram Bot ↔ Go API 双向链路集中暴露多类硬伤，整体品味评分 🟡：

- 火忘式通知 goroutine 内任意 panic（`os.Stdout` 阻塞、JSON 越界、`refreshConfig` DB panic）会顺着所有 `go s.notify*(...)` 直接 crash 整个 API 进程。
- `VerifyBind` 当 DB 里同一 `code` 命中 ≥2 行时直接返回"绑定失败"，把所有持有该码的用户全部锁死，构成自暴 DoS 面。
- `BotNotifier.refreshConfig` 在每个 `IsConfigured` / `post` 都重建 `ConfigService` 并查 DB，高并发下把 settings 表打成热点。
- `/redeem` `/resetpw` 在未绑定 Telegram 时直接返回明文 `ErrTelegramNotBound`，可被用来枚举"该 Telegram ID 是否已绑 Ember"。
- Bot 端 `httpx.AsyncClient()` 无连接池调优 / 无重试，对 API 短抖动直接失败；`lifespan` 在初始化失败路径下不会清理资源。
- Polling 模式无单实例约束保护，多副本会重复消费 update；`/resetpw` 不做服务端去重。
- `BotNotifier.post` 用 `fmt.Printf` 输出错误，无 endpoint / payload 上下文，关键链路失败无可观测性。
- `runtime_settings_service` 30s TTL 缓存击穿与多实例不一致；缓存失败回退仅用 env，运行期改动会丢。
- `pending_reject_requests` 只放进程内 dict 时，Bot 重启或滚动发布会丢失待输入状态。
- `generateTelegramBindCode` 取模导致 `0~777215` 命中概率略偏；并发同一 userID 旧码清理不在唯一索引行锁内。
- `subscribeByTelegram` 输入字段（`tmdbId / posterPath / name`）无白名单。
- `_call_subscription_action` 把任何非 200 当失败，吞掉 4xx 业务提示。
- `PaymentSuccessNotification` 推给 admin chat 的内容包含 `email / stripeSessionId / paymentIntentId`，敏感度过高。
- Webhook 注册失败仅指数回退，不上报告警；polling 启动失败 cleanup 不完整。

如果不收口，会出现"火忘 panic 打死整个 API 进程"、"绑定码冲突让用户永远绑不上"、"攻击者枚举 Telegram→Ember 绑定关系"、"Polling 多副本重复扣 token"等真实可触发的可用性 / 安全事故。

## 目标

本方案要实现：

1. 所有 `go s.notifier.notify*(...)` 调用统一通过 `safeFireAndForget(ctx, name, fn)` 封装，包 `defer recover()` + 结构化日志 + 超时
2. `VerifyBind` 命中 `len(bindCodes) > 1` 时记 ERROR 但返回与正常错误一致的文案；`GenerateBindCode` 改用 `INSERT ... ON CONFLICT (userId) DO UPDATE`
3. `BotNotifier` 改成单例长连接 + 缓存 `botURL` + ConfigService 解析结果，每次发送不再查 DB
4. `/redeem` `/resetpw` 在未绑定时统一返回模糊文案；与 `/info` 入口一致
5. Bot httpx 客户端引入 `limits` + 简单重试 + 总超时；`lifespan` 初始化失败路径补 cleanup
6. Polling 模式启动时 WARN 单实例约束，并在文档明文写死"仅允许单实例部署"
7. `BotNotifier.post` 错误统一过 `logger`，含 `endpoint / event / payload size / status / latency`
8. `runtime_settings_service` 缓存失败回退保留旧值，禁止覆盖为空；多实例不一致风险写入文档
9. `pending_reject_requests` 持久化到 DB（或 Redis）替代进程内 dict，保护 Bot 重启或滚动发布期间的两步拒绝流程
10. `generateTelegramBindCode` 改为 `crypto/rand` 拒绝采样保证均匀分布；旧码清理 + 写入在 `userId` 唯一索引行锁内
11. `subscribeByTelegram` 输入字段加白名单校验（tmdbId 必须正整数 / posterPath 路径白名单 / name 长度上限）
12. `_call_subscription_action` 把 4xx 业务文案透传给 admin chat
13. 推给 admin 的支付成功通知去除高敏字段（仅保留 username / planName / amount / currency）
14. Webhook 注册长期失败 → metric + 告警；polling 启动失败 cleanup 路径完整

## 非目标

本次明确不做：

- 不替换 python-telegram-bot 库
- 不替换 FastAPI
- 不引入 Redis（如确实需要持久化 pending_reject_requests，先用 DB 表）
- 不重写订阅 / 兑换 / 重置密码业务逻辑
- 不调整 Internal API 端点路径（仅修业务行为）
- 不动 Bot 端命令交互流程（`/bind` / `/info` / `/redeem` / `/resetpw` / `/search`）

## 当前事实

以当前代码和现行文档为准，现状如下：

- 相关文档：`docs/system-architecture.md` §5.14 / §5.17 / §9
- 相关服务：
  - `services/api/internal/integrations/notifier/notifier.go`
  - `services/api/internal/services/telegram/service.go`、`wiring.go`、`errors.go`
  - `services/api/internal/services/subscription/service.go`（火忘点）
  - `services/api/internal/services/auth/register_notify.go`
  - `services/api/internal/services/payment/service.go`（火忘点）
  - `services/api/internal/services/playback/ranking.go`（火忘点）
- Bot 端：
  - `services/bot/main.py`、`server.py`、`runtime_settings.py`
  - `services/bot/app/clients/api_client.py`
  - `services/bot/app/handlers/telegram_handler.py`、`search_cache.py`
  - `services/bot/app/formatters/message_formatter.py`
- 中间件：
  - `services/api/internal/middleware/internal_auth.go`（与 access-auth 计划共同收口）
- handler：
  - `services/api/internal/handlers/telegram.go`
- 模型：
  - `services/api/internal/models/telegram_bind_code.go`
- 涉及表：`telegram_bind_codes`
- 当前行为：
  - API 侧关键 fire-and-forget 已统一改走 `internal/async.SafeGo(name, fn)`，不再裸 `go s.notifier.notify*(...)`
  - `BotNotifier.post` 已统一走结构化日志，包含 `endpoint / event / payloadSize / requestId / latency`，`BotNotifier` 本身是进程内共享单例并带配置缓存
  - Polling 模式已通过 API Internal 路由申请 / 续租 / 释放数据库租约锁，拿不到锁的实例拒绝启动
  - `pending_reject_requests` 已落到 `bot_pending_reject_requests` 表，避免 Bot 重启或滚动发布丢失待输入状态
  - 审批消息 `messageId` 已服务端持久化到 `bot_pending_reject_requests`，搜索交互 `message_id` 仍只保留在 `SearchSession` 10 分钟 TTL 缓存，用于校验“用户是否在操作最新一条搜索结果消息”
  - webhook 注册当前采用有限重试策略：最多重试 `6` 次，失败后记 `ERROR` 并停止继续重试，不再无限指数退避悬空
  - `/health` 在 `webhook` 模式下会暴露 webhook 注册状态；若达到最大重试次数仍未注册成功，健康状态返回 `degraded` 并附最后错误与重试次数
- 现有限制：
  - 线上 `AUTO_MIGRATE=false`
  - Bot 仅 webhook 模式可多实例（lifespan 持有 PTB Application 单例时不严谨）

## 方案设计

### 1. 用户可见行为

- API 进程不再因为 Bot 通知 panic 而 crash
- Telegram 用户被绑定 DoS 攻击时仍能成功绑定（旧码清理 + 唯一行锁）
- 攻击者枚举 Telegram→Ember 绑定关系：所有响应模糊一致，无信号
- Bot 在 API 短抖动时不立刻失败，自动重试 3 次以内
- Polling 模式启动时日志 WARN "Polling 模式仅允许单实例部署"
- admin 接收的支付成功消息不含邮箱 / Stripe ID
- 拒绝订阅功能：Bot 重启或滚动发布后，admin 仍可在有效期内提交拒绝原因（pending_reject 持久化）
- `/redeem` 返回业务错误时 admin 能看到具体原因（兑换码无效 / 已用尽等）

### 2. 数据与模型

#### `telegram_bind_codes` 表

- 新增唯一索引：`(userId)` 唯一索引（保证同一用户只有一条有效绑定码）
- 已有 `code` 唯一索引保留

#### 新增表 `bot_pending_reject_requests`

| 字段 | 类型 | 说明 |
|---|---|---|
| id | string(25) | CUID |
| chatId | int64 | admin chat ID |
| adminUserId | string(25) | 操作 admin 的本地 user ID |
| subscriptionId | string(25) | 待拒绝订阅 ID |
| createdAt | time.Time | 自动 |
| expiresAt | time.Time | 默认 5 分钟 |

#### Migration 命名

- `YYYYMMDD_NN_telegram_bind_codes_userid_unique.sql`
- `YYYYMMDD_NN_bot_pending_reject_requests.sql`

所有 migration 幂等。

#### 配置项

- `BOT_NOTIFIER_TIMEOUT_SECONDS` 默认 `5`
- `BOT_NOTIFIER_RETRY_MAX` 默认 `3`
- `BOT_HTTP_TOTAL_TIMEOUT_SECONDS` 默认 `10`
- `BOT_HTTP_RETRY_MAX` 默认 `2`
- `RUNTIME_SETTINGS_TTL_SECONDS` 默认 `30`
- `RUNTIME_SETTINGS_FAIL_KEEP_OLD` 默认 `true`
- `TELEGRAM_UPDATE_MODE_SINGLE_INSTANCE_REQUIRED` 文档明确说明（Polling）

### 3. 接口与边界

#### Internal API

- `POST /api/v1/internal/telegram/redeem`、`POST /api/v1/internal/telegram/reset-password`
  - 入参不变
  - 返回错误统一为业务码：`bind_required` / `code_invalid` / `code_expired` / `password_too_short` 等
  - 未绑定路径：返回 `bind_required` 与"账号 / 资源不可用"统一文案，避免枚举
- `POST /api/v1/internal/telegram/subscribe`
  - 入参字段约束：
    - `tmdbId`：必须 `^\d+$` 且非负
    - `posterPath`：必须 `^/[\w./-]+$` 或为空
    - `name`：长度 ≤ 200
  - 不通过：返回 422
- `POST /api/v1/internal/telegram/info`
  - 已绑定 / 未绑定均返回结构化结果，未绑定时仍带 `bound: false`，前端文案保持模糊一致

#### BotNotifier 内部契约

- `internal/async.SafeGo(name, fn)` 作为统一 fire-and-forget 契约：
  - 启 goroutine
  - `defer func() { recover(); 记 ERROR + name + panic value + stack }`
  - `fn()` 闭包内部自行记录失败上下文
- `post(endpoint, payload)` 错误日志含：endpoint / payload 大小 / 状态码 / latency / event 类型

#### Bot httpx

- `httpx.AsyncClient(limits=httpx.Limits(max_keepalive_connections=10, max_connections=20), timeout=httpx.Timeout(10.0, connect=5.0))`
- 失败重试：最多 2 次，指数退避

### 4. 关键流程

#### 4.1 fire-and-forget 收口

1. 所有关键 fire-and-forget 调用点改为 `async.SafeGo(name, fn)`，例如注册通知、支付解封、订阅下发、缺集扫描启动
2. `async.SafeGo` 统一负责：
   - 启 goroutine
   - `defer recover()`：记 panic + name + stack
   - 保证 panic 不再打死 API 进程
3. 业务闭包内部自行记录 error 上下文；`notifier.post` 失败统一走结构化日志

#### 4.2 GenerateBindCode 行锁化

1. service 接收 userID
2. 事务内：
   - `INSERT INTO telegram_bind_codes (userId, code, expiresAt) VALUES (?, ?, ?) ON CONFLICT (userId) DO UPDATE SET code=excluded.code, expiresAt=excluded.expiresAt, createdAt=now()`
3. code 生成：`crypto/rand` + 拒绝采样保证 0-999999 均匀分布

#### 4.3 VerifyBind 修复

1. service 查询 `WHERE code=? AND expiresAt > now() AND telegramId IS NULL`
2. `len(bindCodes) > 1`：记 ERROR + telegramId + code + 多个 userId，但仍返回 `ErrTelegramBindCodeInvalid`
3. 正常一条：事务内更新 `users.telegramId` + 删 bind code

#### 4.4 反向链路错误模糊化

1. handler 接收 `ErrTelegramNotBound`：返回业务码 `bind_required` + 文案"请先在控制台绑定 Telegram"
2. `/redeem` `/resetpw` 在未绑定时通过同一文案返回，不显式区分"未绑定"vs"用户不存在"

#### 4.5 BotNotifier 缓存

1. 启动期一次性创建 `*BotNotifier`：
   - 解析 `BOT_NOTIFY_URL` 缓存到结构体
   - 持有单 `*http.Client`（含 `Transport.MaxIdleConns`）
2. 提供 `Reload()` 方法响应配置中心修改（可选：通过 ConfigService 事件订阅）
3. 不在每次 post 时 `NewConfigService()`

#### 4.6 Polling 单实例

1. Bot 启动时检查 `TELEGRAM_UPDATE_MODE`：
   - `polling`：日志 WARN "Polling 模式仅允许单实例部署，多副本会重复消费 update"
   - 同时尝试从 DB 读 `bot_polling_lock` 表（可选）抢锁；失败立即退出
2. 文档明确写"Polling 仅单实例"

#### 4.7 runtime_settings 失败回退

1. `runtime_settings_service.refresh()` 失败时：
   - 保留 `_cached`（不覆盖为空）
   - 记 WARN 日志
2. 缓存 TTL 与失败回退策略写入文档

#### 4.8 pending_reject_requests 持久化

1. 拒绝流程：
   - admin 点"拒绝"按钮 → Bot 调 API 写 `bot_pending_reject_requests`
   - admin 在 5 分钟内输入 reason → Bot 读 DB 取 pending → 调 reject API
2. Bot 重启或滚动发布：待确认记录仍由 DB 保存，行为一致
3. cron 每 30 分钟清理过期 pending

#### 4.9 subscribeByTelegram 输入校验

1. service 入参校验：tmdbId 正整数、posterPath 白名单、name 长度
2. 不通过：返回业务码 `invalid_input`
3. Bot 端拼通知文本前对外部数据再做 escape

#### 4.10 _call_subscription_action 透传

1. Bot 调用审批 / 拒绝 API
2. 4xx：取响应 body 中的 `error` 字段透传给 admin chat
3. 5xx：fallback "操作失败，请重试"

#### 4.11 admin 通知脱敏

1. `PaymentSuccessNotification` 仅含：username / planName / amount / currency / paidAt
2. 移除 email / stripeSessionId / paymentIntentId（保留在 Go 端日志即可）

### 5. 失败路径与边界条件

- **fire-and-forget panic 频发**：metric 计数 + ERROR 日志，但主进程不退出
- **GenerateBindCode 唯一冲突**：ON CONFLICT DO UPDATE 自动覆盖，无错误
- **VerifyBind 出现 >1 行**：日志 ERROR 后用户看到"绑定失败"统一文案；运维需手工排查
- **BotNotifier 配置变更不生效**：管理员触发 `Reload()` 或重启 API（文档说明）
- **Polling 多实例启动**：第二个实例直接退出 + ERROR 日志
- **runtime_settings 长时间失败**：缓存保留旧值，运行期改动暂时不生效；恢复后下一次 TTL 自然刷新
- **pending_reject 5 分钟超时**：admin 重新点拒绝按钮即可
- **subscribeByTelegram 输入非法**：返回 422，Bot 显示"输入数据不合法"
- **_call_subscription_action 4xx**：透传业务文案；前端 admin 看到具体原因
- **admin chat 通知误删**：脱敏后通知信息可被任意管理员查看，避免敏感字段长期落 Telegram 服务器

## 影响范围

- API：
  - 修改：`integrations/notifier/notifier.go`、`services/telegram/service.go`、`services/subscription/service.go`、`services/payment/service.go`、`services/playback/ranking.go`、`services/auth/register_notify.go`、`handlers/telegram.go`
  - 新增：`internal/async/safe.go`、`services/telegram/pending_reject.go`
- Web：
  - 不影响（admin 拒绝订阅在 Bot 端进行）
- Bot：
  - 修改：`server.py`（lifespan / Polling 单实例 WARN）、`clients/api_client.py`（httpx limits / retry）、`runtime_settings.py`（失败保留旧值）、`handlers/telegram_handler.py`（pending_reject 走 API）、`formatters/message_formatter.py`（admin 通知脱敏）
- 配置 / 部署：
  - 新增 2 份 SQL migration
  - 新增 6 个 ConfigDefinition
  - 文档明确"Polling 仅单实例"
- 文档：
  - `docs/system-architecture.md` §5.14 / §5.17 / §9 改写
  - 新增"Bot fire-and-forget 契约（`async.SafeGo`）"
  - 新增"runtime_settings 失败回退策略"
  - 新增"Polling 单实例约束"

## 验证方式

### 编译 / 测试

- `cd services/api && go build ./...`
- `cd services/api && go test ./internal/integrations/notifier/... ./internal/services/telegram/...`
- `cd services/bot && python -m py_compile main.py`
- `cd services/bot && python -m pytest tests/`（如有）

### 手工验证

#### fire-and-forget recover
- 在 BotNotifier mock 一个 panic（强制 nil 解引用）→ 触发审批 → API 不 crash，日志出现结构化 ERROR

#### GenerateBindCode 行锁
- 并发同 userID 触发 GenerateBindCode：DB 始终唯一一条记录
- 老用户已有 code 时再次触发：code 被覆盖

#### VerifyBind 自暴 DoS 修复
- 人为造 2 条同 code 数据 → 触发 VerifyBind：日志 ERROR + 用户看到一致错误，不影响其他用户绑定

#### 反向链路模糊化
- 未绑定 Telegram 用户调 `/redeem`：返回与 `/info` 一致的"请先绑定"
- 攻击者无法区分"未绑定"vs"码无效"

#### BotNotifier 缓存
- 高并发触发 100 条通知：DB `settings` 表只查 1 次（启动期）
- 修改 `BOT_NOTIFY_URL` 配置 → 触发 Reload 后生效

#### Bot httpx
- mock API 短抖动：Bot 重试 2 次后成功
- mock API 长时间不可用：Bot 显示"服务暂不可用"

#### Polling 单实例
- 启 2 个 polling Bot：第二个启动失败 + ERROR 日志

#### runtime_settings 失败回退
- mock API 5xx → runtime_settings 缓存保留旧值，未覆盖为空

#### pending_reject 持久化
- 启 2 个 webhook Bot 副本：admin 拒绝订阅在不同副本上完成（输 reason）也能正常工作

#### subscribeByTelegram 校验
- 提交 `tmdbId=abc`：返回 422
- 提交超长 name：返回 422

#### _call_subscription_action 透传
- mock API 返回 `409 {"error":"订阅已被处理"}`：admin 看到"订阅已被处理"

#### admin 通知脱敏
- 完成支付：admin chat 不出现 email / stripeSessionId

### 历史验证口径

- API 侧验证口径：`go build ./...` 与 `go test ./internal/integrations/notifier/...`、`./internal/services/telegram/...`
- Bot 侧验证口径：`python -m py_compile services/bot/main.py`
- 数据侧验证口径：2 份 SQL migration 可在临时库重灌
- 行为侧验证口径：`async.SafeGo` panic 不打死 API 进程、双副本 polling 第二个实例拒绝启动、runtime settings API 5xx 时缓存保留旧值
- 观察性口径：关键日志包含 `endpoint / event / payloadSize / status / latency / requestId`
- 本次归档不追加代码改动；这里只保留历史验收口径，不再作为当前待办清单

## 批次 5 第一阶段已落地（2026-04-28）

- ✅ `services/telegram/errors.go` / `service.go` 补 `ErrTelegramUserNotFound`，`GenerateBindCode` / `Unbind` 不再靠 `"用户不存在"` 字符串
- ✅ `handlers/telegram.go` 对应路径改为 `errors.Is` 分支；部分 `500` 裸透改走 `httpx.InternalError`
- ✅ `services/telegram/service.go` 去掉 `Save(&user)` 全字段写入：绑定 Telegram / Telegram 重置密码改为按字段更新
- ✅ `handlers/telegram_test.go` 补齐 `GenerateBindCode` / `Unbind` / `VerifyBind` / `SubscribeByTelegram` 的错误映射测试，锁死 `400/404/409/500` 语义，避免后续回归

不阻塞归档的后续治理项：

- 搜索会话 `message_id` 若后续要跨实例追踪，可再评估是否值得持久化；当前继续视为私聊交互态边界
- 外部 metric / 告警平台接入仍可继续补，但当前主链路已通过 `GET /health` 暴露 webhook 未注册状态
- 更细颗粒度观察性补强可继续追加，但不再作为本主计划的退场阻塞项

### 后续治理观察点

- sweep 所有 `go ` 启动 goroutine 位置：确认都过 `async.SafeGo` 或明确说明无需包裹
- sweep 所有 `fmt.Printf` / `fmt.Println` 在生产路径上的使用，统一改 logger
- sweep 所有"未绑定 / 不存在"错误路径，确认对外文案模糊一致
- sweep 所有 `*http.Client` 实例化位置，确认 BotNotifier / EmbyService / TMDBService / MoviePilotClient / Stripe Client 都复用单例
- sweep 所有 admin / group 通知文案，确认无敏感字段（email / sessionId / token / hash）
- sweep 所有 `os.Getenv` 在中间件 / handler 中的一次性 capture，确认与 ConfigService 边界对齐（与 access-auth 计划协同）
- 复核 `runtime_settings_service` 在滚动发布与实例切换时的一致性窗口
- 复核 `_synced_chat_versions` 缓存在 Bot 重启后的影响

## 落地后文档处理

- 已把 `async.SafeGo` 契约、Polling 单实例约束、runtime settings 失败回退、admin 通知脱敏等稳定结论提炼到 `docs/system-architecture.md`
- 本文已移入 `docs/archive/plan/bot-telegram/`
- P2 / P3 中未顺手收口的项继续按后续治理项跟踪，不再阻塞本主计划退场

## 附录：问题清单与本方案条目映射

| review 编号 | 问题 | 本方案条目 |
|---|---|---|
| P0-1 (Bot) | 火忘 panic 打死进程 | §4.1 + `async.SafeGo` |
| P0-2 (Bot) | VerifyBind 自暴 DoS | §4.3 + GenerateBindCode 行锁 |
| P1-1 (Bot) | BotNotifier 每次查 DB | §4.5 |
| P1-2 (Bot) | /redeem /resetpw 信息枚举 | §4.4 |
| P1-3 (Bot) | httpx 无 limits / retry / lifespan 漏 cleanup | §4 + Bot httpx |
| P1-4 (Bot) | Polling 无单实例约束 | §4.6 |
| P1-5 (Bot) | BotNotifier fmt.Printf 无可观测性 | §4.1 + logger |
| P2-1 (Bot) | runtime_settings 失败覆盖空 | §4.7 |
| P2-2 (Bot) | pending_reject_requests 随 Bot 重启或滚动发布丢失 | §4.8 + 表 `bot_pending_reject_requests` |
| P2-3 (Bot) | bind code 取模偏置 + 旧码并发 | §4.2 |
| P2-4 (Bot) | subscribeByTelegram 输入校验 | §4.9 |
| P2-5 (Bot) | _call_subscription_action 吞 4xx | §4.10 |
| P2-6 (Bot) | webhook 注册失败无告警 / polling cleanup | §4.6 + 文档 |
| P3-2 (Bot) | admin 通知泄漏 email / Stripe ID | §4.11 |
| P3-1 / P3-3 / P3-4 (Bot) | binding 边界 / compare_digest / batch settings | 二次暴露清单 |
