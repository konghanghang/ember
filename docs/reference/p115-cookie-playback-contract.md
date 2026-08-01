# 115 Cookie 播放兼容合同

本文档记录 Ember 首期 115 播放 Provider 在没有 OpenAPI AppID 时，使用 Cookie/Web API 完成文件查重、秒传和下载直链所依赖的兼容合同。该链路不是 115 官方开放平台合同，必须把公开实现证据、Ember 内部语义和仍需实机确认的行为分开。

OpenAPI 获批后的正式授权、Token 生命周期和官方端点合同见 [115 OpenAPI 直连、查重与秒传合同](./p115-direct-play-contract.md)。

## 1. 状态与证据等级

当前选择 Cookie Provider 的唯一原因是 Ember 尚未取得 115 OpenAPI AppID。它是可替换的首期兼容实现，不是长期绑定的基础设施。

| 能力 | 证据 | 当前结论 |
| --- | --- | --- |
| Cookie 客户端行为 | [`p115client` 提交 `608a44396fea08d36131a68beb245be1fe17aa6d`](https://github.com/ChenyangGao/p115client/tree/608a44396fea08d36131a68beb245be1fe17aa6d) | 可作为协议调查和测试向量来源，不作为 Ember 运行时依赖 |
| Cookie 登录状态检查 | 同提交内 `login_status` 与 `user_id` | 公开实现确认固定 GET 端点、`state` 字段和 Cookie `UID` 取值方式；真实账号兼容性仍未实机确认 |
| Cookie 上传初始化加解密 | 同提交内 `p115cipher` `0.0.5.4` | 可据此独立实现 Go 适配器，必须用固定向量锁定行为 |
| `emby-toolkit` 小号播放行为 | `emby-toolkit` `v10.8.63`、提交 `7e64564884c9949390e5894b4be71038808e4e2a` | 只用于理解账号选择与失败语义，不复制 AGPL 代码 |
| 上游许可证 | `p115client` 固定提交的 `LICENSE` / `pyproject.toml` 与 `LICENSE_zh` 表述不一致 | 未澄清前只作协议研究，不逐行翻译或复制源码 |
| 115 Cookie/Web API 稳定性 | 非官方接口 | 随时可能变化，必须通过 Provider 边界隔离 |
| Ember 真实账号行为 | 未实机确认 | 尚未调用真实 115 接口，不能断言风控、配额和响应字段稳定 |

证据等级：

- **公开实现确认**：固定源码提交能证明该版本如何发请求或解析响应，不代表 115 官方承诺。
- **Ember 内部合同**：Ember 自己定义并通过 fake、fixture 和状态机测试锁定的行为。
- **未实机确认**：必须在用户明确授权后，用受控账号和文件验证；验证前不能写成生产事实。

许可证边界：Ember 不复制 `emby-toolkit` 的 AGPL 实现；对 `p115client` / `p115cipher` 只使用可观察协议行为和无敏感信息测试向量。在复用任何源代码前必须先澄清上游许可证不一致并完成单独审查。

## 2. 首期范围

首期只支持：

- 一个管理员配置的源账号，用于读取源文件信息和 Range challenge。
- 一个管理员配置的播放小号，用于接收秒传文件和生成最终下载直链。
- 管理员通过 Ember 管理端手工录入、替换和验证 Cookie。
- Ember 原生 Go `CookieProvider`，不调用 Python 进程，不依赖 `p115client` 包。
- SHA1 与文件大小查重、秒传初始化、必要的范围校验、目标文件复核和兼容条件成立时的 HTTP 302。

首期不支持：

- 普通用户绑定自己的 115 账号。
- 二维码登录、自动抓取 Cookie 或 Cookie 自动续期。
- 多播放账号池、自动选路、主备切换或账号借用。
- OpenAPI 与 Cookie 的隐式回退。
- Ember 下载完整视频后再上传。
- 需要最终播放器额外注入 Cookie、Authorization 等请求头的直链。

## 3. Provider 边界

业务层只依赖内部 Provider 语义，不感知 Cookie 端点、加密格式或原始状态码。首期接口至少覆盖：

| 操作 | 内部语义 |
| --- | --- |
| `ValidateCredential` | 验证 Cookie 是否能识别账号，并返回脱敏账号标识 |
| `GetUploadInfo` | 获取上传初始化所需 `userId`、`userKey` 等账号数据 |
| `SearchBySHA1` | 按 SHA1 查询候选文件，业务层再次校验大小和文件类型 |
| `InitRapidUpload` | 发起秒传，映射为复用成功、范围校验、普通上传或失败 |
| `GetDownloadURL` | 获取下载地址及其 UA、Cookie、过期时间等使用约束 |
| `FindTargetFile` | 秒传后在目标目录复核文件并返回 fileId、pickCode |
| `DeleteFile` | 清理临时目标文件；同一账号内串行执行 |

后续 OpenAPI Provider 必须实现同一组业务语义。Provider 切换要显式配置，不能在某次请求失败后静默改用另一种认证模式。

## 4. 账号与凭证合同

### 4.1 账号角色

账号角色固定为：

- `source`：系统源账号，只用于定位源文件、获取源下载地址和读取指定 Range。
- `playback`：播放小号，只用于目标查重、秒传落盘和签发最终直链。

数据库对每个角色至多允许一条启用记录；网关进入就绪状态时，必须同时存在一个启用的 `source` 和一个启用的 `playback`。两者不能是同一条账号记录；Provider 返回的账号标识相同时应拒绝启用，避免把源账号误当播放小号。

### 4.2 Cookie 安全

- Cookie 只能通过 HTTPS 管理接口写入，使用 `CONFIG_ENCRYPTION_KEY` 加密后保存在专用账号表。
- Cookie 是只写字段：创建或替换后，任何查询接口都不得返回明文或可复原片段。
- 管理端只展示账号别名、角色、脱敏 Provider 用户标识、状态和最后验证时间。
- 不通过环境变量、命令行参数、前端构建变量或普通 settings 表保存 Cookie。
- Cookie、完整 `Set-Cookie`、上传加密材料、完整下载地址和 Provider 完整响应体不得进入日志、审计详情或测试快照。
- Provider 使用的 `appType` 和 User-Agent 必须作为账号兼容参数显式保存；不能在不同请求里随机切换客户端身份。

Cookie 失效后没有首期自动续期流程。账号状态应转为 `expired` 或 `error`，停止新秒传和新直链，由管理员替换 Cookie 并重新验证。

## 5. Cookie/Web API 合同

### 5.1 账号验证

公开实现确认的只读入口：

```http
GET https://my.115.com/?ct=guide&ac=status
Cookie: <account-cookie>
User-Agent: <account-user-agent>
```

固定源码把 JSON 顶层布尔字段 `state` 作为登录状态，并从 Cookie 中 `UID` 值的第一个 `_` 之前解析数字用户 ID。Ember 的内部合同更严格：

- Cookie 必须恰好包含一个合法、非零的 `UID`；用户 ID 以十进制规范化后保存，不保存 Cookie 片段。
- 仅接受 `2xx` 且 `state` 为 JSON 布尔值的响应；`state=true` 才验证成功。
- `state=false` 视为凭证失效，账号转为 `expired + disabled`。
- 网络错误和非 `2xx` 视为 Provider 暂不可用；缺失或非法 `state` 视为协议错误。两者都转为 `error`，保留管理员的 `enabled` 意图，但 `LoadActiveCredential` 会因状态不是 `active` 而拒绝运行时读取。
- 验证结果按发起请求时的精确 Cookie 密文做条件更新；如果管理员在请求期间替换 Cookie，旧结果不得覆盖新凭证的 `pending` 状态。
- 验证成功只把账号转为 `active` 并记录 Provider 用户 ID，不自动启用。启用操作必须另行执行。

验证操作只能读取账号概要，不得修改账号文件或安全设置。成功后只保存 Provider 用户 ID 等脱敏标识。

证据：[`login_status`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/p115client/client.py#L20301-L20339)、[`user_id`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/p115client/client.py#L530-L537)。以上只证明固定公开源码的请求与解析行为，不代表 115 官方承诺；目标账号实际响应、风控和稳定性仍属未实机确认。

### 5.2 上传信息

公开实现确认的入口：

```http
GET https://proapi.115.com/app/uploadinfo
Cookie: <account-cookie>
```

响应用于取得上传初始化需要的 `user_id` 和 `userkey`。字段缺失、账号不一致或响应业务状态失败时，账号验证失败。

证据：[`upload_info_app`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/p115client/client.py#L26750-L26783)。

### 5.3 SHA1 查重

公开实现确认的入口：

```http
GET https://webapi.115.com/files/shasearch
Cookie: <playback-account-cookie>
```

旧 Web API 最多返回一个候选，不能约束目标目录，并通过业务错误表达未命中。Ember 不能只信任第一条或文件名，命中后必须同时校验：

```text
candidate.sha1 == expectedSHA1
&& candidate.size == expectedSize
&& candidate.isDirectory == false
```

如果无法拿到 size 或文件类型，结果只能作为候选，不能直接签发 302。秒传完成后还必须在目标目录复核。

证据：[`fs_shasearch`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/p115client/client.py#L17349-L17392)。

### 5.4 Cookie 秒传初始化

公开实现确认的入口：

```http
POST https://uplb.115.com/4.0/initupload.php
Cookie: <playback-account-cookie>
Content-Type: application/x-www-form-urlencoded
```

逻辑请求至少包含：

- `fileid`：完整文件 SHA1。
- `filename`：目标文件名。
- `filesize`：源文件字节数。
- `target`：`U_1_<target-parent-id>`。
- `userid`、`userkey`：来自播放小号上传信息。
- `appversion`、`topupload`。
- 二次校验时的 `sign_key`、`sign_val`。

该请求不是把上述表单直接明文发送。公开实现会生成签名与 `k_ec`，加密业务负载，并解密和解压响应。Ember 的 Go Provider 必须独立实现协议并用固定测试向量证明兼容，不能在业务 Service 中拼接这套加密逻辑。

证据：[`upload_init`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/p115client/client.py#L26785-L26961)。

### 5.5 下载地址

公开实现的 `download_url` 支持多个 app/Web 入口。Ember 首期究竟固定哪个 app 类型和端点，必须由受控验证决定，当前不得猜测。

公开实现指出下载链接包含以下约束信号：

- `t`：链接过期时间。
- `u`：账号标识。
- `c`：并发打开限制。
- `f=0`：未声明额外 Header 要求。
- `f=1`：要求使用获取直链时相同的 User-Agent。
- `f=3`：除相同 User-Agent 外，还要求携带直链响应给出的 Cookie。

首期只允许对已通过合同测试和受控验证、且最终客户端能够满足 Header 条件的链接返回 302。若链接需要播放器无法注入的 Cookie 或其他 Header，必须明确失败；不能把 Cookie 拼进 URL，也不能泄露播放小号凭证。

证据：[`download_url`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/p115client/client.py#L8300-L8457)。

### 5.6 删除

公开实现确认的入口：

```http
POST https://webapi.115.com/rb/delete
Cookie: <playback-account-cookie>
```

公开实现明确提示删除操作不要并发执行。Ember 必须按播放账号串行清理，并在删除前再次校验目标账号、文件 ID 和 Ember 任务归属，不能把搜索候选直接当作可删除对象。

证据：[`fs_delete`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/p115client/client.py#L10457-L10513)。

## 6. 上传加密合同

当前固定 `p115cipher` 实现确认：

- 请求负载使用 AES-128-CBC 处理。
- 响应在解密后进行 LZ4 解压。
- `k_ec` token 包含时间戳、公钥材料和 CRC 校验。
- `make_upload_payload` 根据 `userkey`、`userid`、文件 SHA1、大小、目标目录、二次校验参数、时间戳和 app 版本生成签名与 token。

实现要求：

1. 加密适配器与 Provider HTTP 逻辑分层，业务层不接触密钥材料。
2. 从固定提交生成不含真实账号信息的请求和响应测试向量。
3. Go 测试必须覆盖加密、解密、LZ4、签名、token 和单字节变更导致结果变化。
4. 不把公开实现中的协议常量重复写入文档、日志或业务配置。
5. 公开实现升级时先对比向量和端点合同，不能直接追随最新版。

证据：[`p115cipher`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/modules/p115cipher/p115cipher/__init__.py)。

## 7. 文件身份与范围数据

Ember 只用以下组合确认源文件和目标文件：

```text
SHA1 + size + isDirectory=false
```

源文件还应记录 `fileId`、`pickCode` 和路径映射结果。文件名、路径或 Emby ItemId 不能替代内容身份。

部分公开实现会把源文件前 128 KiB 的 SHA1 作为 `preid`。这一行为不是官方合同，目前标记为**公开实现确认、未实机确认**。如果初始化需要 `preid` 且缓存中没有，Ember 只能从源账号读取有界 Range 计算，不能下载完整文件。

二次校验流程：

1. 校验 `sign_check` 格式、起止位置和文件大小边界。
2. 使用源账号 pickCode 获取源下载地址。
3. 保留源下载请求所需的 UA 和 Cookie；这些 Header 只在服务端内部使用。
4. 只读取指定 Range，计算大写 SHA1 作为 `sign_val`。
5. 携带原 `sign_key` 和 `sign_val` 再次调用目标账号上传初始化。

## 8. 秒传状态机

Provider 必须把外部响应映射为内部枚举，业务层不得直接散落判断原始 `status`：

| 外部状态 | 内部语义 | Ember 行为 |
| --- | --- | --- |
| `status=2` | `reused` | 继续在目标目录复核文件 |
| `status=7` | `range_challenge` | 校验范围并读取源文件指定字节段，再次初始化 |
| `status=1` | `ordinary_upload_required` | 明确失败，禁止全文件上传 |
| 其他或缺失 | `provider_rejected` | 保留脱敏原始错误码并失败 |

HTTP `2xx` 不等于秒传成功。最终成功必须同时满足：

1. Provider 明确返回 `reused`；并且
2. 播放小号在目标目录查询到 SHA1 和 size 一致的非目录文件。

秒传任务幂等键为：

```text
playbackAccountId + SHA1 + size
```

首次查重后获取数据库唯一任务或 advisory lock，拿锁后再次查重。Infuse 的 `HEAD`、预加载、重复 Range 和重连请求必须复用同一任务。

## 9. 302 与直链安全

- 获取最终直链时使用真实客户端 User-Agent；直链缓存至少按 `accountId + pickCode + UA` 隔离。
- 只有实测证明存在 IP 绑定后，才把客户端 IP 纳入缓存键；不得凭经验增加。
- 缓存有效期不得超过链接参数 `t`，并预留安全窗口。
- `Location` 必须匹配配置的 115/CDN 域名 allowlist，禁止开放重定向。
- 源账号用于 Range challenge 的直链永远不能返回给客户端。
- `f=3` 或其他需要额外 Cookie 的链接，在证明 Infuse 能自然满足前按不兼容处理。
- 不兼容、凭证失效或 Provider 不可用时返回明确 `503` 和内部错误码，不静默让源账号播放，也不回退 Emby 视频中转。

## 10. 清理、冷却与错误

- 临时目标文件只能由成功任务记录归属；播放停止且超过 TTL 后才进入清理候选。
- 清理任务按账号串行执行，删除前复核任务、文件 ID、SHA1 和 size。
- Provider 限流按账号共享冷却，不让每个播放请求独立重试。
- Cookie 失效、账号风控、下载地址不兼容、普通上传要求和 Provider 限流必须使用不同内部错误码。
- 负缓存只能短期存在；一次搜索失败不能长期证明文件不存在。
- 已签发的 CDN 链接无法由 Ember 保证立即失效，封禁和账号失效只能阻止新直链。

## 11. 自动化测试合同

所有测试必须 mock 115，禁止真实外部请求。至少覆盖：

1. Cookie 加密落库、替换和 API 永不回显明文。
2. 登录状态端点 method、query、Cookie/User-Agent Header、`state` 正常/失效/非法响应和 UID 规范化。
3. 验证结果与 Cookie 版本绑定，过期、协议错误、网络错误和成功状态流转正确。
4. 源账号与播放账号角色唯一性，以及相同 Provider 账号拒绝启用。
5. SHA1 命中但 size 或文件类型不符时拒绝。
6. `status=2` 复用后目标文件复核。
7. `status=7` 的正常范围、越界、格式错误和 Range 获取失败。
8. `status=1` 明确拒绝，且不触发完整文件上传。
9. 加密请求与解密响应固定向量。
10. 并发 `HEAD`、预加载和 Range 只创建一个秒传任务。
11. 下载链接 UA 隔离、过期、域名 allowlist 和 `f=3` 拒绝。
12. Cookie、完整直链、完整 SHA1 和 Provider 响应不进入日志或普通数据库字段。

## 12. 受控真实验证

真实验证必须由用户明确授权，使用测试账号和测试文件，并以一次性命令执行；不能启动 Ember 服务或后台进程作为验证手段。每次记录日期、平台、Infuse 当前稳定版本、脱敏请求字段和结果。

首轮至少确认：

- 源账号和播放小号 Cookie 的有效性、账号标识与客户端类型约束。
- `uploadinfo`、`shasearch` 和上传初始化的实际响应字段。
- `status=2`、`status=7`、`sign_check` 和目标文件可见延迟。
- 最终下载地址使用的实际端点，以及 `t`、`c`、`f`、UA、Cookie 和 IP 约束。
- 当前稳定 Infuse 在目标平台对 `HEAD`、Range、302 和下载 Header 的实际行为。
- 删除串行要求、频率限制、风控和冷却边界。

验证完成后清理测试文件并替换或撤销临时 Cookie。在这些行为完成实测前，文档和实现结论必须保持“未实机确认”。

## 13. 演进边界

- Cookie Provider 与 OpenAPI Provider 可以并存，但每个账号必须显式选择认证模式。
- OpenAPI AppID 获批后，优先新增 OpenAPI Provider，不在 Cookie Provider 内混入 Token 分支。
- 是否迁移既有播放小号由管理员显式决定，不自动转换凭证或静默切换。
- 多账号池、普通用户自有账号和二维码 Cookie 登录必须分别重新设计并补计划，不能从首期单账号模型自然外推。
