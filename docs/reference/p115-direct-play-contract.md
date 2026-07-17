# 115 OpenAPI 直连、查重与秒传合同

本文档记录 Ember 115 直连播放依赖的授权、文件身份、目标账号查重、跨账号秒传、下载直链和 Token 生命周期边界。目标是明确区分官方 OpenAPI、公开客户端实现语义和仍需真实账号验证的行为，避免把非官方 Web API 或经验值当成稳定合同。

## 1. 适用范围与证据等级

当前证据：

| 能力 | 证据 | 结论 |
| --- | --- | --- |
| OpenAPI 应用授权 | 115 官方开放平台文档 | 可使用 AppID 通过 PKCE/设备码或授权码获得 Token |
| OpenAPI 文件搜索 | 115 官方 `/open/ufile/search` 文档 | 可作为目标账号文件查询入口 |
| OpenAPI 上传初始化 | 115 官方 `/open/upload/init` 文档 | 支持 SHA1、大小、目标目录和二次校验参数 |
| OpenAPI 下载地址 | 115 官方 `/open/ufile/downurl` 文档 | 使用 pickCode 获取下载地址 |
| SHA1 搜索和秒传状态解析 | `p115client` 提交 `dc13e14132006f877183dd6347fa6da3aca6b48e` | 当前公开实现用 `status=2` 表示复用、`status=7` 表示二次校验 |
| 目标真实账号行为 | 未确认 | 尚未调用任何真实 115 接口，不能断言配额、风控和返回字段完全一致 |

证据等级：

- **官方合同**：由 115 开放平台文档直接确认。
- **公开实现确认**：由当前 `p115client` 固定提交证明其实现方式，但不是 115 官方稳定承诺。
- **未实机确认**：需要受控真实账号验证，目前不能作为生产结论。

主要证据：

- [115 开放平台](https://open.115.com/)
- [设备码与 PKCE 授权](https://www.yuque.com/115yun/open/shtpzfhewv5nag11)
- [授权码模式](https://www.yuque.com/115yun/open/okr2cq0wywelscpe)
- [文件搜索](https://www.yuque.com/115yun/open/ft2yelxzopusus38)
- [上传初始化](https://www.yuque.com/115yun/open/ul4mrauo5i2uza0q)
- [下载地址](https://www.yuque.com/115yun/open/um8whr91bxb5997o)
- [`p115client` 固定提交](https://github.com/ChenyangGao/p115client/tree/dc13e14132006f877183dd6347fa6da3aca6b48e)

## 2. OpenAPI 应用与用户授权

### 2.1 正式部署要求

正式 Ember 部署应使用部署方自己创建的 115 开放平台应用：

```text
Ember 部署方
  -> 在 115 开放平台创建应用
       -> 获得 AppID/client_id
       -> 授权码模式额外获得 AppSecret 并配置 Redirect URI

Ember 用户
  -> 授权同一个 Ember 应用
       -> 每个用户获得自己的 accessToken 和 refreshToken
```

每个用户不需要单独申请开放平台应用。系统级 AppID 由 Ember 部署方维护，用户只完成授权。

### 2.2 PKCE/设备码模式

推荐用于 Ember 账号中心绑定：

```http
POST https://qrcodeapi.115.com/open/authDeviceCode
```

关键参数：

- `client_id`：开放平台 AppID。
- `code_challenge`：PKCE challenge。
- `code_challenge_method`：challenge 算法。

该流程不要求把 AppSecret 下发到浏览器。二维码创建、状态轮询、`code_verifier` 和 Token 获取必须由 Ember 后端控制，Web 只接收授权会话 ID、二维码内容和脱敏状态。

### 2.3 授权码模式

授权码模式需要：

- `client_id`
- `client_secret`
- `redirect_uri`
- `authorization_code`

`redirect_uri` 必须在 115 开放平台应用管理中配置。`client_secret` 属于部署信任根，不应存入前端构建产物或通过普通设置接口返回。

### 2.4 PoC 与兼容模式

- 技术 PoC 可以临时使用当前有效的第三方 AppID，通过 PKCE 验证搜索、秒传和直链链路。
- 正式 Ember 版本不能写死不受控制的第三方 AppID；应用可能失效、撤销或改变策略。
- Cookie/Web API 可以作为实验性兼容模式，但默认关闭，不作为稳定合同。
- 仅凭现有 NextEmby 日志无法判断其使用 OpenAPI 还是 Cookie API。

## 3. Token 生命周期合同

每个用户账号至少保存：

- `p115UserId`
- `appId`
- `accessTokenCiphertext`
- `refreshTokenCiphertext`
- `tokenExpiresAt`
- `authorizedAt`
- `lastValidatedAt`
- `status`

安全要求：

- access token 和 refresh token 必须使用 `CONFIG_ENCRYPTION_KEY` 派生的专用加密组件加密存储。
- API 只返回绑定状态、授权时间、最后验证时间和脱敏错误，不返回 Token 明文。
- 日志不得记录 Authorization Header、二维码授权 Token、完整 provider 响应或下载地址。

当前公开客户端确认 refresh token 采用轮换语义：刷新成功后返回新的 access token 和 refresh token，旧 refresh token 不能继续作为稳定凭证使用。因此 Ember 必须：

1. 对同一 115 账号的 Token 刷新加数据库锁或等价分布式互斥。
2. 只允许一个刷新请求执行。
3. 在同一事务中写入新的 access token、refresh token 和过期时间。
4. 请求结果不确定时停止自动并发重试，转入待重新授权状态。

公开实现证据：[`login_refresh_token_open`](https://github.com/ChenyangGao/p115client/blob/dc13e14132006f877183dd6347fa6da3aca6b48e/p115client/client.py#L1633-L1684)。

## 4. 文件身份合同

Ember 使用以下组合确认一个 115 文件：

```text
SHA1 + size
```

同时保留：

- `fileId`
- `parentId`
- `pickCode`
- `name`
- `SHA1`
- `size`

约束：

- SHA1 进入数据库和比较前统一转成大写。
- size 使用 `int64`，单位为字节。
- 不能使用文件名作为内容唯一标识。
- 同一内容可以在账号中有不同文件名和不同目录位置。
- `fileId` 和 `pickCode` 属于账号内定位信息，不能跨账号复用。

## 5. 三种“存在”语义

实现时必须区分：

### 5.1 源账号是否存在文件

来源通常是 Emby 路径解析和源账号元数据缓存。需要得到源账号自己的：

```text
fileId + pickCode + name + SHA1 + size
```

源账号文件不存在时不能继续秒传，也不能仅凭历史 SHA1 缓存假设文件仍然可用。

### 5.2 目标用户账号是否已经有文件

这是播放前的只读查重。必须使用目标用户自己的凭证搜索，并严格比较结果的 SHA1 和 size。

### 5.3 115 全局是否具备秒传内容

上传初始化可以判断 115 是否能够复用相同内容，但这是实际上传/创建尝试，不是目标账号的只读存在性查询。

不能把“115 全局可秒传”解释成“目标用户账号已经有这个文件”。

## 6. 目标账号 SHA1 查重

官方 OpenAPI 文件搜索入口：

```http
GET https://proapi.115.com/open/ufile/search
Authorization: Bearer <target-account-access-token>
```

当前公开实现使用类似参数：

```json
{
  "search_value": "3458F5C0700D8B6A2CD31ABCA236784E49711C4F",
  "fc": 2,
  "show_dir": 0,
  "type": 99,
  "limit": 16
}
```

响应处理必须遍历候选结果并再次校验：

```text
candidate.sha1 == expectedSHA1
&& candidate.size == expectedSize
&& candidate.isDirectory == false
```

不能只取第一条，不能只比较文件名。公开实现参考：[`sha1_to_id`](https://github.com/ChenyangGao/p115client/blob/dc13e14132006f877183dd6347fa6da3aca6b48e/modules/p115tiny302/p115tiny302/app.py#L85-L118)。

旧 Web API `files/shasearch` 最多返回一条记录、不支持目录约束，而且未命中时使用错误响应表达。它可以作为兼容模式快速查询，但最终仍需校验 size；正式 OpenAPI 模式优先使用文件搜索接口。

查重缓存建议：

```text
targetAccountId + SHA1 + size
  -> targetFileId + targetPickCode + verifiedAt
```

缓存命中后如果生成直链失败，应清除缓存并重新查一次目标账号，处理用户手工删除或移动文件的情况。

## 7. 跨账号秒传合同

### 7.1 上传初始化

目标账号调用：

```http
POST https://proapi.115.com/open/upload/init
Authorization: Bearer <target-account-access-token>
Content-Type: application/x-www-form-urlencoded
```

请求至少包含：

```json
{
  "file_name": "example.mkv",
  "fileid": "FULL_FILE_SHA1",
  "file_size": 31707512832,
  "target": "U_1_TARGET_DIRECTORY_ID",
  "topupload": 1
}
```

当前公开实现的语义：

- `status == 2`：内容复用成功，`reuse=true`。
- `status == 7`：需要二次范围校验，响应包含 `sign_key` 和 `sign_check`。
- 其他状态：不能直接认定秒传成功。

上述状态值来自固定公开实现，不是本文能够证明的官方长期稳定枚举。适配层必须把原始值映射成 Ember 内部语义枚举，并通过合同 fixture 锁定。

### 7.2 二次范围校验

当 115 返回 `sign_check`：

1. 校验范围格式和边界，不能越过源文件大小。
2. 使用源账号 pickCode 调用下载地址接口。
3. 保留源下载地址要求的 UA、Cookie 或其他 Header。
4. 对源文件发出 `Range: bytes=<sign_check>` 请求，只读取指定字节段。
5. 计算该字节段 SHA1，转成大写作为 `sign_val`。
6. 携带 `sign_key` 和 `sign_val` 再次调用目标账号上传初始化。

这条链路只读取 115 指定的范围数据，不下载完整视频。当前跨账号实现证据：[`transferfile`](https://github.com/ChenyangGao/p115client/blob/dc13e14132006f877183dd6347fa6da3aca6b48e/p115client/tool/edit.py#L2158-L2238)。

### 7.3 秒传完成判断

不能只按上传初始化 HTTP `2xx` 判断成功。最终成功条件必须是：

1. 响应明确表示内容复用成功；并且
2. 目标账号按返回 fileId/pickCode 或 `SHA1 + size` 重新查询到文件。

如果响应进入普通上传准备状态，但没有明确复用成功，Ember 不得继续把完整文件下载到服务器再上传。应返回 `rapidTransferUnavailable`，或者进入显式配置的备用策略。

### 7.4 幂等与并发

秒传任务使用以下幂等键：

```text
targetAccountId + SHA1 + size
```

执行顺序：

1. 首次目标账号查重。
2. 获取数据库唯一任务或 PostgreSQL advisory lock。
3. 拿锁后再次查重，避免其他请求已经完成。
4. 执行上传初始化和必要的 Range 校验。
5. 验证目标文件。
6. 写入成功缓存并释放锁。

Infuse 的 `HEAD`、预加载和重复 Range 请求必须复用同一任务，不能重复秒传。

## 8. 分享接收备用策略

Cookie/Web API 的 `share/receive` 可以实现：

1. 源账号创建单文件分享。
2. 目标账号接收到指定 `cid`。
3. 目标账号重新按 SHA1 和 size 验证。
4. 撤销临时分享。

公开实现证据：[`share_receive`](https://github.com/ChenyangGao/p115client/blob/dc13e14132006f877183dd6347fa6da3aca6b48e/p115client/client.py#L24765-L24816)。

该能力不属于本文已确认的 OpenAPI 主链路，只能作为显式启用的兼容策略，原因包括：

- 分享码和接收码增加泄露面。
- 需要创建、接收、撤销和失败清理。
- 分享数量、频率和风控边界未确认。
- 依赖非官方 Web API，兼容性风险更高。

## 9. 下载直链合同

官方 OpenAPI 下载地址入口：

```http
POST https://proapi.115.com/open/ufile/downurl
Authorization: Bearer <account-access-token>
```

请求使用目标账号自己的 `pick_code`。当前公开实现指出部分直链可能要求：

- 与获取直链时一致的 User-Agent。
- 响应返回的 Cookie。
- 在 URL 参数 `t` 指定时间前使用。

公开实现证据：[`download_url`](https://github.com/ChenyangGao/p115client/blob/dc13e14132006f877183dd6347fa6da3aca6b48e/p115client/client.py#L8296-L8330)。

因此：

- 生成用户播放直链时必须使用真实客户端 UA。
- 直链缓存至少按 `accountId + pickCode + UA` 隔离；如果实测存在 IP 绑定，还必须包含客户端 IP。
- 缓存有效期不得超过直链自身过期时间。
- 302 `Location` 必须校验为配置允许的 115/CDN 域名，禁止任意开放重定向。
- 完整直链和附带 Cookie 不得写日志或持久化到普通事件表。
- 源账号用于秒传 Range 校验的直链不能返回给最终用户。

## 10. 冷却、限流与错误分类

当前未实机确认 115 的完整错误码和限流合同。实现时至少内部区分：

- `credentialInvalid`
- `authorizationRevoked`
- `tokenRefreshRequired`
- `fileNotFound`
- `targetFileAlreadyExists`
- `rapidTransferChallenge`
- `rapidTransferUnavailable`
- `providerRateLimited`
- `providerCooldown`
- `providerUnavailable`

要求：

- 同一账号的限流和冷却必须共享，不能让每个播放请求独立重试。
- 负缓存只能短期存在，防止临时搜索失败被长期当作文件不存在。
- Provider 原始错误响应必须脱敏，禁止保存完整响应体。
- 未确认的错误码不能写成稳定业务语义，必须保留原始错误码的脱敏映射能力。

## 11. 测试与真实验证边界

自动化测试必须使用 fake 115 Provider 或固定 fixture，至少覆盖：

1. 目标账号 SHA1 + size 命中。
2. SHA1 命中但 size 不一致。
3. 目标未命中、上传初始化直接复用成功。
4. `status=7` 范围校验后复用成功。
5. sign_check 越界、格式错误和 Range 获取失败。
6. 初始化 HTTP 成功但未复用，不能误判成功。
7. 并发请求只创建一个秒传任务。
8. refresh token 并发刷新互斥和轮换写入。
9. 直链 UA 隔离、过期和域名 allowlist。
10. 日志、API 和数据库快照不包含明文 Token、Cookie 和完整直链。

真实账号验证属于外部链路，必须由用户明确授权并在受控测试账号上执行。至少确认：

- 申请 AppID 的当前账号资质和审核要求。
- PKCE 扫码、Token 有效期和 refresh token 轮换行为。
- OpenAPI 搜索 SHA1 的实际响应字段。
- 上传初始化状态和 Range challenge 的实际格式。
- 秒传后的目标文件可见延迟。
- 下载直链的 UA、Cookie、IP 和过期约束。
- API 频率、共享冷却和账号风控边界。

在这些行为完成实测前，应明确标记为“未实机确认”。
