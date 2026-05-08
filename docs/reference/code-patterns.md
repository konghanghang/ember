# Ember 代码模式速查

> 本文档承接 Ember 当前稳定可复用的代码模式，用于协作时快速对齐常见实现约定。
> 这些模式是当前事实，不是建议草案；若实现已发生变化，应优先同步本文件。

| 模式 | 说明 |
|------|------|
| ID 生成 | CUID 格式：`cl` + timestamp(hex) + random(hex)，25 字符 |
| 分页响应 | `{data:[], total, page, pageSize, totalPages}` |
| 错误响应 | `{error: "中文错误消息"}`，400/401/404/500 |
| Handler 模式 | `ShouldBindJSON/ShouldBindQuery` → 调用 Service → 返回 JSON |
| Service 模式 | 接收 Request struct → 业务逻辑 → 返回 Response/error |
| 码生成 | `crypto/rand.Read(bytes)` → `hex.EncodeToString` → 16 字符 |
| 密码哈希 | `bcrypt.GenerateFromPassword(DefaultCost)` |
| Emby 认证 | `X-Emby-Token: {apiKey}` 头 |
| 内部通信 | `X-Internal-Secret: {secret}` 头（Bot ↔ API） |
| 前端请求 | Axios 拦截器自动加 Bearer token，401 自动清除登录态 |
| 火忘通知 | `internal/async.SafeGo(name, fn)` 启 goroutine，统一 recover panic 并记结构化日志；业务主流程不阻塞 |
| 上游错误脱敏 | `internal/common/upstream.SafeUpstreamError(err, system)` 剥离 `*url.Error` 中的请求 URL（含 `api_key`）；`SafeUpstreamHTTPError(system, statusCode)` 仅保留 system + 状态码，不回显响应体。当前已收口 TMDB / MoviePilot 调用链路、配置中心媒体测试接口（Emby / MoviePilot / SMTP）以及 Stripe / SMTP 上游网络与 HTTP 错误路径 |
| 内部错误响应 | `internal/common/httpx.InternalError(c, err)` 客户端只看到 `上游服务暂不可用` 统一文案，完整 err（含 requestId）落服务端日志；handler 不再裸透 `err.Error()` |
