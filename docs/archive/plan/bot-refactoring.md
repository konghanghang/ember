# Bot 服务重构方案（已落地）

> 状态：已归档（已落地）
> 负责人：Ember
> 更新时间：2026-03-28

## 背景

这个问题为什么现在要解决：

- `services/bot/app/clients/api_client.py` 中每次调用都新建 `httpx.AsyncClient`，连接池完全没有复用，内部 API 调用成本偏高。
- `/notify/*` 4 个端点重复同一段内部鉴权逻辑，`telegram_handler.py` 里也有多处重复的消息编辑降级和 API 调用模板。
- `api_client.py` 当前大量 `except Exception` 直接吞掉，关键失败路径缺少排障日志，不符合项目对关键路径日志的要求。
- `search_cache.py` 使用 `threading.Lock` 保护纯内存字典，复杂度高于实际需要，且与 Bot 其他模块的并发模型表达不一致。

## 目标

本方案要实现：

1. 复用 Bot 到 Ember API 的 HTTP 连接，减少重复建连。
2. 消除 Bot 服务内确定无价值的重复代码，但不改变现有用户可见行为。
3. 为内部 API 调用补足可排障日志，同时明确敏感信息脱敏边界。

## 非目标

本次明确不做：

- 不改 Go API，不新增批量 settings 等接口。
- 不改 Telegram Bot 功能逻辑、命令语义、消息文案和交互流程。
- 不引入新依赖，不顺手做 unrelated cleanup。

## 当前事实

以当前代码和现行文档为准，现状如下：

- 相关文档：`docs/system-architecture.md`
- 相关服务：`services/bot/app/clients/api_client.py`、`services/bot/app/server.py`、`services/bot/app/handlers/search_cache.py`、`services/bot/app/handlers/telegram_handler.py`
- 当前行为：
  - `api_client.py` 中 8 个 HTTP 调用函数都使用 `async with httpx.AsyncClient(...)`，请求结束即关闭连接。
  - `get_setting()` 使用 `5s` timeout，其他内部 API 与搜索 API 基本使用 `10s` timeout。
  - `/notify/subscription`、`/notify/registration`、`/notify/payment`、`/notify/ranking` 端点均手动校验 `X-Internal-Secret`，鉴权失败返回 `401` 和 `{"error": "unauthorized"}`。
  - 搜索结果相关消息的编辑逻辑存在“图片消息优先编辑 media，文本消息保持文本”的现有行为。
- 现有限制：
  - Bot 服务必须保持对现有 Go API 的调用契约不变。
  - 项目日志禁止记录密码、验证码、Token、内部鉴权 secret 和完整返回体。

## 方案设计

### 1. 用户可见行为

- 不新增任何 Bot 命令、按钮、接口或配置项。
- 以下现有行为必须保持不变：
  - `/notify/*` 鉴权失败时继续返回 `401` 和 `{"error": "unauthorized"}`。
  - `handle_bind`、`handle_info`、`handle_redeem`、`handle_resetpw`、`handle_search` 的成功/失败文案保持不变。
  - 搜索结果页、详情页、备注输入页的切换逻辑保持不变。
  - 文本消息不会因为本次重构被“升级”为海报消息；只有当前消息本身是 photo 时才允许优先走 `edit_message_media`。

### 2. 数据与模型

> 本次不涉及数据模型变更。

### 3. 接口与边界

- 不新增或修改任何 Go API、Internal API、Webhook 路径、请求字段和响应字段。
- `api_client.py` 的改造只影响客户端内部实现，不改变调用方拿到的返回结构：
  - 成功仍返回现有 JSON。
  - 业务失败仍返回带 `error` 的 dict。
  - 网络异常或解析异常仍按现有语义返回 `None` 或空字符串。
- `/notify/*` 端点的鉴权逻辑会抽为共享 helper，但不会改成会改变响应体格式的默认 `HTTPException` 路径。
- `search_cache.py` 继续提供同步的 `get_session()`、`set_session()`、`delete_session()` 接口，不引入异步调用方改造。

### 4. 关键流程

1. 在 `api_client.py` 中增加模块级共享 `httpx.AsyncClient`，通过 `init()` / `close()` 生命周期函数管理。
2. `server.py` 的 `lifespan` 在 Bot 初始化阶段调用 `api_client.init()`，在退出阶段调用 `api_client.close()`。
3. 各 HTTP 调用函数改为复用共享 client，但每次请求继续显式传入当前 timeout，保证 `get_setting=5s`、其他调用大多 `10s` 的现有边界不变。
4. `api_client.py` 为异常路径补日志，只记录接口名、关键标识、状态码、异常类型和耗时，不记录密码、验证码、兑换码、`X-Internal-Secret` 或完整响应体。
5. `server.py` 提取 `_verify_internal_secret(request) -> JSONResponse | None` 或等价 helper；各 `/notify/*` 端点先调用 helper，再决定是否继续处理请求。
6. `telegram_handler.py` 提取消息编辑辅助函数，但 helper 只封装重复降级链，不改变原有“是否允许 media 编辑”的判断条件。
7. `telegram_handler.py` 提取通用的“API 调用 -> 统一错误回复 -> 成功回调”模板，覆盖 `handle_bind`、`handle_info`、`handle_redeem`、`handle_resetpw`，保留 `handle_search` 的特殊错误分支。

### 5. 失败路径与边界条件

- HTTP client 初始化失败：Bot 服务启动应失败并暴露明确日志，不允许静默降级为每次重新建连。
- HTTP 请求异常：保持现有返回语义，但日志必须能看出失败的是哪个接口、针对哪个 `telegram_id` 或 `subscription_id`。
- 敏感参数：
  - 禁止记录 `newPassword`、绑定验证码、兑换码、`X-Internal-Secret`。
  - 禁止记录完整请求体和完整响应体。
- `/notify/*` 鉴权失败：
  - 必须继续返回 `401`。
  - 必须继续返回 `{"error": "unauthorized"}`。
  - 不允许因为使用 FastAPI dependency 而把响应体变成默认的 `{"detail":"unauthorized"}`。
- 搜索消息编辑：
  - 若原消息是 photo，允许 `edit_message_media -> edit_message_caption -> edit_message_text` 逐级降级。
  - 若原消息是 text，保持直接走 `edit_message_text`，不引入新的消息类型切换。
  - `handle_cancel_note` 抽公共逻辑后，也必须明确沿用当前详情页恢复策略，失败时继续提示用户重新 `/search`。
- `search_cache.py` 去掉锁后，不额外引入线程并发假设；该模块仍按当前单进程事件循环访问方式使用。

## 分步方案

### Step 1: `api_client.py` — 共享 `httpx.AsyncClient` + 可排障日志

问题：

- 当前每次请求都会新建并销毁 `AsyncClient`。
- 关键失败路径没有日志。

方案：

1. 增加模块级 `_client: httpx.AsyncClient | None = None`。
2. 增加 `_INTERNAL_HEADERS` 常量，统一内部鉴权 header。
3. 增加 `init()`、`close()`、`_get_client()`，统一管理共享 client 生命周期。
4. 各请求函数改为复用 `_get_client()`，但请求时继续显式传入各自 timeout。
5. 为异常路径补 `logger.warning` 或 `logger.exception`，只记录白名单字段：
   - 接口名
   - `telegram_id` / `subscription_id` / `key`
   - HTTP 方法
   - 状态码
   - 异常类型

兼容性约束：

- `get_setting()` 仍使用 `5s` timeout。
- 其他现有 `10s` timeout 的接口继续保持 `10s`。
- 返回值语义保持不变，不借机统一成新结构。

### Step 2: `search_cache.py` — 移除 `threading.Lock`

问题：

- 当前只是进程内内存缓存，锁的收益小于额外复杂度。

方案：

1. 删除 `from threading import Lock` 和 `_lock = Lock()`。
2. 去掉 `with _lock:` 包裹，保留现有 TTL 清理与覆盖写入逻辑。
3. 不改 `SearchSession` 结构，不改外部调用方式。

兼容性约束：

- `SESSION_TTL` 保持 `600` 秒。
- 过期即删除、`set` 时惰性清理的行为保持不变。

### Step 3: `server.py` — 提取 `/notify/*` 鉴权 helper

问题：

- 4 个 `/notify/*` 端点重复同一段 secret 校验逻辑。

方案：

1. 提取共享 helper，例如 `_verify_internal_secret(request: Request) -> JSONResponse | None`。
2. helper 内统一使用 `compare_digest()`，与 webhook 路径保持一致。
3. 每个 `/notify/*` 端点开头调用 helper；若返回非空 `JSONResponse`，立即返回，不再继续执行端点逻辑。

兼容性约束：

- 不使用会改写响应体格式的默认 `HTTPException` 路径。
- 仍返回 `{"error": "unauthorized"}`，而不是 `{"detail": "unauthorized"}`。

### Step 4: `telegram_handler.py` — 提取消息编辑降级 helper

问题：

- `_handle_pick`、`_handle_back`、`handle_cancel_note` 存在相似的“media -> caption -> text”降级逻辑。

方案：

1. 提取共享 helper，输入显式包含：
   - bot/query 上下文
   - `chat_id`
   - `message_id`
   - `prefer_media`
   - `poster_url`
   - `caption`
   - `reply_markup`
2. helper 只负责执行既定降级链，不负责决定“当前场景能不能尝试 media”。
3. `_handle_pick`、`_handle_back`、`handle_cancel_note` 继续在各自调用点判断 `prefer_media`，确保行为不被抽象偷改。

兼容性约束：

- 原消息是 text 时，不新增 `edit_message_media` 尝试。
- 无海报时继续沿用当前占位图 / 文本前缀策略。

### Step 5: `telegram_handler.py` — 提取 handler API 调用模板

问题：

- `handle_bind`、`handle_info`、`handle_redeem`、`handle_resetpw` 都有相同的“调用 API -> 判断 `None` -> 判断 `error` -> 回复消息”流程。

方案：

1. 提取 `_call_api_and_reply(message, coro, on_success)` 或等价 helper。
2. helper 负责：
   - 网络失败统一回复“服务暂不可用，请稍后重试”
   - 业务失败统一回复 `error`
   - 成功时调用传入的 success callback 生成回复
3. `handle_search` 保持独立实现，因为它的 `status == 400` 分支要提示先绑定 Telegram。

兼容性约束：

- 仅抽模板，不统一业务错误文案。
- 不改变 `handle_search` 的特殊分支。

## 执行顺序

建议顺序：

1. Step 1：先改 `api_client.py`
2. Step 1 联动：在 `server.py` 的 `lifespan` 中接入 `api_client.init()` / `api_client.close()`
3. Step 2：改 `search_cache.py`
4. Step 3：抽 `/notify/*` 鉴权 helper
5. Step 4：抽消息编辑 helper
6. Step 5：抽 handler API 调用模板

## 影响范围

涉及的子系统：

- API：无，不改 Go 服务接口和响应结构
- Web：无
- Bot：有，涉及 `clients`、`server`、`handlers`
- 配置/部署：无新增环境变量，无部署入口变更
- 文档：落地后同步更新 `docs/system-architecture.md` 中 Bot 目录职责描述

## 验证方式

### 编译/测试

- `cd services/bot && python -m py_compile app/clients/api_client.py`
- `cd services/bot && python -m py_compile app/server.py`
- `cd services/bot && python -m py_compile app/handlers/search_cache.py`
- `cd services/bot && python -m py_compile app/handlers/telegram_handler.py`
- `cd services/bot && python -m compileall .`

### 手工验证

- 鉴权失败时，请求任一 `/notify/*` 端点，确认仍返回 `401` 和 `{"error": "unauthorized"}`
- 执行 `/bind`、`/info`、`/redeem`、`/resetpw`，确认成功和失败文案与改造前一致
- 执行 `/search` -> 进入详情页 -> 添加备注 -> `/cancel`，确认搜索消息仍按当前图片/文本策略编辑，不出现新的消息类型切换
- 观察 Bot 日志，确认异常路径能定位接口和关键标识，但不包含密码、验证码、兑换码和内部 secret

## 落地后文档处理

落地后应同步处理：

- 将 Bot 目录中 `api_client.py` 使用共享 `AsyncClient`、`server.py` 在 `lifespan` 中联动初始化/关闭的稳定结论同步到 `docs/system-architecture.md`
- 若最终实现明确了日志字段白名单，可提炼到 Bot 相关参考文档或后续运维文档
- 实现并验证完成后，将本方案移入 `docs/archive/`
