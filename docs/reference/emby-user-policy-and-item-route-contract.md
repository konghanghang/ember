# Emby 4.9.3.0 用户与条目路由合同补充

> 核对日期：2026-09-16
> 证据等级：固定版本 SDK OpenAPI；本轮没有调用真实 Emby

## 适用范围

本文件补充 GitHub #8、#12、#19 所涉及的原生接口。协议基线为 Emby Server `4.9.3.0`，SDK 固定提交为 `6ee0155063bc85578196489926359a8f37419502`，不依赖插件。

出处：[固定版本 OpenAPI](https://github.com/MediaBrowser/Emby.SDK/blob/6ee0155063bc85578196489926359a8f37419502/Resources/OpenApi/openapi_v3.json)。Gateway 的运行版本窗口与历史实机证据继续以 [播放代理 API 合同](./emby-playback-proxy-contract.md) 为准。本轮未重新查询部署实例，也未对本补充中的所有接口执行完整 4.9 系列兼容比对。

## 用户与 Policy

下列 path 是 OpenAPI operation path；server base 为 `/emby`。

| Method / path | 请求 | 成功响应 |
| --- | --- | --- |
| `GET /Users/{Id}` | path 用户 ID | `200`，`UserDto` |
| `POST /Users/{Id}/Policy` | JSON `UserPolicy` | `200`，空响应 |
| `POST /Users/{Id}/Password` | JSON `UpdateUserPassword` | `200`，空响应 |

关键约束：

- 上述接口声明 `apikeyauth` 或 `embyauth`；API Key 可用 `X-Emby-Token` header 传递，禁止在日志或文档实例中保存真实凭证。OpenAPI 的认证声明不能单独证明任意普通用户 token 都具有修改其他用户的权限；Ember 管理链路继续使用配置的管理凭据。
- `UserPolicy.IsDisabled`、`IsAdministrator` 为 boolean，`SimultaneousStreamLimit` 为 int32。
- `UpdateUserPassword` 定义 `Id`、`NewPw`（string）与 `ResetPassword`（boolean）。本地管理员是否调用该接口属于 Ember 认证策略，不能从这些字段推导。
- Policy 接口是 POST 完整策略 DTO，没有在本次核对的 operation 中声明 revision/CAS 条件写。Ember 的 `PatchUserPolicyFields` 是客户端读后合并再写，不是远端原子 PATCH；串行化与旧结果纠正必须由本地编排保证。
- 当前客户端还存在 `GET /Users/{Id}/Policy` 兼容读取分支；本次固定 OpenAPI 仅在该 path 声明 POST，因此该 GET 分支不能写成已有 SDK 合同保证。优先使用有合同依据的 `GET /Users/{Id}` 读取 `UserDto.Policy`；兼容分支的目标环境行为保持未证实。
- 上述接口列出 `400/401/403/404/500` 错误响应；超时、连接断开和丢失响应不能证明远端没有写入，须按未确认结果处理。
- 这些写入请求不携带业务时区参数。本地到期判断复用 `CRON_TIMEZONE`，不可把服务端未声明的时间或条件写语义当作已验证能力。

## 用户条目静态路由与动态详情

| Method / path | `200` JSON 合同 | 快照边界 |
| --- | --- | --- |
| `GET /Users/{UserId}/Items/{Id}` | `BaseItemDto` | 正常动态条目详情 |
| `GET /Users/{UserId}/Items/Latest` | `BaseItemDto[]` | 静态集合端点，不将 Latest 当 ItemId |
| `GET /Users/{UserId}/Items/Resume` | `QueryResult_BaseItemDto` | 静态分页端点，不将 Resume 当 ItemId |
| `GET /Users/{UserId}/Items/Root` | `BaseItemDto` | 静态根条目端点，不将 Root 当 ItemId |

以上均声明 API Key/Emby 认证，并列出 `400/401/403/404/500`。固定版本没有给这些 path 单独声明 HEAD；Gateway 现有 HEAD 代理行为应由本地透明代理测试锁定，不据此声称 SDK 明确保证 HEAD。

`Root` 虽返回单个 DTO，仍不能按请求尾段字面值建立动态 ItemId 快照。修复路由分类时，应先识别这些静态路径，再保留动态详情现有 ID 兼容性；不据字段名猜测 ID 只能是数字。

## 回归要求与证据边界

- 用固定 fixture / fake transport 验证请求 method、path、JSON 与错误映射；不请求真实 Emby。
- Policy 测试保留非托管字段，并用屏障证明本地并发执行顺序；不能把 mock 的条件写能力添加到真实合同中。
- 条目测试覆盖三种静态端点与真实动态详情，确认原始响应透明、静态路径不产生错误快照诊断。
- 本合同补充不代表上述 issue 已修复，也不证明真实服务器权限、超时后写入结果或全部 4.9 版本行为。
