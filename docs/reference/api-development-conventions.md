# API 开发与目录规范

> 本文档用于沉淀 `services/api` 在本轮目录重构中的经验、约束和后续开发规范。目的不是写漂亮话，而是减少后续继续开发时的目录漂移、循环依赖和职责污染。

---

## 1. 这轮重构得到的核心经验

### 1.1 目录问题本质上是边界问题

最开始 `internal/services` 之所以越长越乱，不是因为文件太多，而是因为不同职责混在一起：

- 业务服务
- 配置系统
- 第三方集成
- 启动装配
- 领域错误

如果边界不清，只是“把文件挪目录”不会让项目变好，反而会制造循环依赖。

### 1.2 能拆的模块，不是因为它大，而是因为它边界清晰

这次能顺利拆出去的有：

- `config/`
- `integrations/emby`
- `integrations/moviepilot`
- `integrations/notifier`
- `services/device`
- `services/media`
- `services/payment`
- `services/playback`
- `services/subscription`
- `services/telegram`
- `services/tvcalendar`

这些模块有一个共同点：

- 依赖方向明确
- 对外能力清晰
- 不需要反向引用根 `services` 包

而暂时没拆干净的：

- `user`
- `auth`
- `email`

不是因为不重要，而是因为职责还粘在一起。

### 1.3 compat 只能是过渡层，不能变常驻层

为了平滑迁移，我们引入了若干 `*_compat.go` 文件。它们的价值只有一个：

- 在目录重构阶段，保持旧调用面可用

但 compat 有明显副作用：

- 会掩盖旧依赖没有被真正清掉
- 会让“目录已经重构”产生假象
- 如果长期保留，`services` 根层会重新长回去

结论：

**compat 允许临时存在，但最终必须删除。**

### 1.4 错误定义不能继续做全局垃圾桶

最早的 `services/errors.go` 把所有错误都塞在一个文件里，这种结构短期方便，长期一定失控。

现在已经按领域拆分，这个方向必须坚持：

- 错误跟着业务走
- handler 通过 `errors.Is()` 判断
- 不要再新增新的全局错误桶

### 1.5 启动装配不应该留在 `cmd` 入口

`cmd` 的职责应该尽量薄，只保留进程选择、信号入口和退出码。

现在已经下沉到：

- `internal/app/server.go`
- `internal/app/routes.go`
- `internal/app/cron.go`
- `internal/app/handlers.go`

以后继续新增启动流程时，也应优先放到 `internal/app` 或对应运行时包，不要重新把 `main.go` 变回巨型入口。

Playback Gateway 部署入口的目标合同是单个 `cmd/ember`：`ember api` 分发到 API `RunProcess`，`ember gateway` 分发到 `internal/playbackgateway.RunProcess`。无参数默认 API 以兼容当前镜像行为，未知子命令必须 fail-fast；禁止通过环境变量隐式选择进程角色。

当前 `cmd/server` 与 `cmd/playback-gateway` 在统一入口落地后只允许作为薄兼容包装存在，不进入生产镜像。删除条件固定为 Docker、CI、Makefile、runbook 和仓库脚本全部迁移到 `cmd/ember`，不能让兼容层长期承载第二套装配逻辑。

---

## 2. 当前推荐目录结构

当前 `services/api/internal` 推荐理解为 6 层：

### 2.1 `app/`

职责：

- 服务启动装配
- handler 组装
- 路由注册
- cron 初始化

规则：

- 不放业务逻辑
- 不放数据库查询
- 不放第三方集成细节

### 2.2 `config/`

职责：

- 配置定义
- 配置解析
- 配置校验
- 配置导入
- 配置测试

规则：

- 这是运行期配置系统，不属于业务服务
- 以后新增配置项，一律先加在这里

### 2.3 `integrations/`

职责：

- 第三方系统接入

当前包括：

- `emby/`
- `moviepilot/`
- `notifier/`

规则：

- 这里只做外部系统协议适配
- 不做业务编排
- 不做 handler 层 HTTP 映射

### 2.4 `services/`

职责：

- 业务服务
- 业务编排

当前已分组的包括：

- `auth/`
- `device/`
- `email/`
- `media/`
- `payment/`
- `playback/`
- `redemption/`
- `subscription/`
- `system/`
- `telegram/`
- `tvcalendar/`
- `user/`

未完全分组的包括：

- 当前无明确需要继续目录化的核心业务文件

### 2.5 `handlers/`

职责：

- HTTP 参数绑定
- Service 调用
- HTTP 状态码映射

规则：

- 不要把业务逻辑塞进 handler
- handler 中允许做的事情只有：
  - 解析参数
  - 调 service
  - `errors.Is()` 映射状态码
  - 拼接 HTTP 响应

### 2.6 `models/`

职责：

- 数据模型
- GORM 映射

规则：

- 不要把业务逻辑堆到 model
- model 里只允许保留非常轻量的领域方法，例如：
  - `IsExpired()`
  - `IsAdmin()`
  - `SetPassword()`

---

## 3. 依赖方向规范

推荐依赖方向：

```text
app -> handlers -> services -> integrations/config/models/db/common
```

更具体地说：

- `app` 可以依赖 `handlers`
- `handlers` 可以依赖 `services`、`config`
- `services` 可以依赖 `integrations`、`config`、`models`、`db`
- `integrations` 可以依赖 `config`，但不应依赖 `services`
- `models` 不应依赖 `services`

### 明确禁止

以下依赖方向原则上禁止：

- `integrations -> services`
- `config -> services`
- `models -> handlers`
- `handlers -> integrations`（除非极少数只做纯代理且无业务语义）

如果出现这种依赖，说明边界设计有问题，应优先重构，而不是继续硬搬文件。

---

## 4. 新代码应该怎么放

### 4.1 新增第三方接入

放到：

```text
internal/integrations/<vendor>/
```

例如：

- `internal/integrations/stripe/`
- `internal/integrations/tmdb/`

### 4.2 新增运行期配置能力

放到：

```text
internal/config/
```

不要再把配置解析塞回 `services/`。

### 4.3 新增业务能力

优先放到已有领域目录：

- 设备管理相关 → `services/device/`
- 支付相关 → `services/payment/`
- 播放历史/排行 → `services/playback/`
- 追剧日历 → `services/tvcalendar/`
- Telegram Bot 绑定/自助 → `services/telegram/`

如果没有现成领域，再评估是否值得新建目录。

### 4.4 什么时候新建子目录

满足以下任一条件时，可以考虑建子目录：

1. 该领域已经有 2 个以上强相关文件
2. 该领域已经有独立错误定义
3. 该领域对外已经形成稳定 API
4. 继续平铺会明显恶化阅读体验

不满足这些条件时，先别建目录，避免过度碎片化。

---

## 5. compat 文件规范

当前 compat 已清理完成。

### 5.1 compat 使用规则

compat 只允许用于：

- 子目录迁移后的短期兼容导出

compat 不允许用于：

- 长期稳定 API
- 掩盖循环依赖
- 偷偷维持旧目录结构

### 5.2 删除 compat 的前提

删除某个 compat 前，必须满足：

1. 调用方已经直接依赖新子包
2. handler 中的错误判断已经改成新子包错误
3. `internal/app` 中的装配也已经直接依赖新子包
4. `go test ./...` 通过

### 5.3 明确要求

**所有 compat 文件最终都必须删除。**

这是当前目录重构的硬性收尾要求，不是可选项。

---

## 6. 还没完全收口的模块

### 6.1 `user`

问题：

- 职责过多
- 和 `email` / `emby` 耦合明显

要求：

- 不要直接硬搬目录
- 先切分职责，再拆目录

### 6.2 `auth`

问题：

- 目前仍是编排层

要求：

- 暂时不继续拆目录
- 等 `user`、`email` 边界清晰后再评估

### 6.3 `email`

问题：

- 既负责 SMTP，又负责验证码业务

要求：

- 后续如继续重构，应先拆职责，再评估目录

### 6.4 `redemption` / `redemption_code`

问题：

- 已完成目录分组，但还需要继续清理调用面与兼容层

要求：

- 视为已完成的领域目录化案例
- 后续重点转到 compat 清理，不再回退到根 `services`

---

## 7. 后续开发硬规则

后续开发请直接遵守下面这几条：

1. 不要再向 `internal/services` 根目录新增新的“杂项大文件”。
2. 新增第三方接入必须优先放 `internal/integrations/`。
3. 新增运行期配置逻辑必须优先放 `internal/config/`。
4. 不允许重新引入全局 `errors.go` 垃圾桶。
5. 不允许新增新的 `utils.go`，必须按职责命名。
6. 若使用 compat 过渡，必须同步写清楚“后续删除计划”。
7. 每次目录重构都必须跑 `go test ./...`。
8. 目录拆分失败时，优先回退未完成迁移，不要把半成品留在工作区。

---

## 8. 当前稳定实现模式

这些不是“建议风格”，而是当前主线代码已经收口出来的稳定模式。以后如果实现变了，应优先同步这里，而不是再造一个碎片速查页。

### 8.1 Handler / Service 分工

- Handler 模式：`ShouldBindJSON` / `ShouldBindQuery` 解析参数 → 调用 service → 返回 JSON
- Handler 不承载业务编排；允许做的事只有参数绑定、状态码映射、错误分类和响应拼接
- Service 模式：接收 request struct → 执行业务逻辑 → 返回 response / error

### 8.2 统一错误与上游脱敏

- 上游网络或 HTTP 错误不要直接把 `err.Error()` 回给客户端
- 统一使用 `internal/common/upstream.SafeUpstreamError(err, system)` 剥离 URL、Token 等敏感信息
- 仅需回客户端的上游失败统一走 `internal/common/httpx.InternalError(c, err)`，文案保持为 `上游服务暂不可用`

### 8.3 火忘式异步通知

- 不阻塞主流程的异步通知统一使用 `internal/async.SafeGo(name, fn)`
- 允许 fire-and-forget，但必须由统一封装接管 panic recover 和结构化日志
- 不允许在 handler / service 里直接裸起 goroutine

### 8.4 标识与密钥生成

- 主键 ID 当前使用 CUID 风格：`cl` + timestamp(hex) + random(hex)，总长 25 字符
- 兑换码 / 绑定码等短码统一走 `crypto/rand.Read(bytes)` + `hex.EncodeToString`
- 密码哈希统一使用 `bcrypt.GenerateFromPassword(DefaultCost)`

## 9. 当前推荐下一步

如果继续推进，最建议的顺序是：

1. 处理 `user/email/auth`

原因：

- compat 不清理，重构永远没完成
- `user/auth/email` 已经进入“边界设计问题”，不是简单的目录问题了
