# 115 Cookie 播放兼容合同

本文档记录 Ember 首期 115 播放 Provider 在没有 OpenAPI AppID 时，使用 Cookie/Web API 完成源文件解析、文件查重、秒传、受限 Range 校验和下载直链所依赖的兼容合同。该链路不是 115 官方开放平台合同，必须把公开实现证据、Ember 内部语义和仍需实机确认的行为分开。

OpenAPI 获批后的正式授权、Token 生命周期和官方端点合同见 [115 OpenAPI 直连、查重与秒传合同](./p115-direct-play-contract.md)。

从管理员配置到 Gateway/Emby/115 CDN 的整体调用关系、状态流转和当前缺口，见 [115 Cookie 直连播放端到端流程参考](./p115-playback-end-to-end-flow.md)。

普通用户已复用同一 Cookie Provider 绑定本人 playback 账号、按套餐路由到个人或管理员共享 playback，并使用 Redis 执行播放租约与转存配额。现行边界及[个人账号与 Redis 待验收清单](./p115-playback-end-to-end-flow.md#121-个人账号与-redis-待验收清单)由端到端参考维护；[归档方案](../archive/plan/architecture/p115-personal-account-routing-and-redis-quotas.md)仅保留实施和历史自动化证据。个人 Cookie、Redis 与客户端真实验收仍未完成，管理员与个人输入合同分别见下文。

## 1. 状态与证据等级

当前选择 Cookie Provider 的唯一原因是 Ember 尚未取得 115 OpenAPI AppID。它是可替换的首期兼容实现，不是长期绑定的基础设施。

| 能力 | 证据 | 当前结论 |
| --- | --- | --- |
| Cookie 客户端行为 | [`p115client` `0.0.9.6.4`，提交 `608a44396fea08d36131a68beb245be1fe17aa6d`](https://github.com/ChenyangGao/p115client/tree/608a44396fea08d36131a68beb245be1fe17aa6d) | 可作为协议调查和测试向量来源，不作为 Ember 运行时依赖 |
| Cookie 登录状态检查 | 同提交内 `login_status` 与 `user_id`；2026-08-22 两个真实 Cookie | 固定 GET 端点、`state` 和 Cookie `UID` 取值已在两个不同账号上通过；长期稳定性、风控和其他客户端仍未证实 |
| Cookie 上传初始化加解密 | 同提交内 `p115cipher` `0.0.5.4` 黑盒输出；2026-08-22 受控写入验证 | Ember 已用无敏感信息固定向量锁定 token、AES-CBC、LZ4、签名和完整上传表单；真实端点曾返回 220/236 字节二进制响应，均由完整 AES blocks 加 12 字节短尾部组成。对齐固定实现的完整 block 解密与 LZ4 终止语义后，真实 `range_challenge → reused` 已通过 |
| 源路径解析与 Range 校验 | 同提交内 `fs_dir_getid2`、`fs_files`、`read_range` 和秒传 Range callback；父目录快速解析已有 fake HTTP 合同；2026-08-22 本地真实只读检查仍是旧逐级 `/files` 路径 | 父目录路径一次解析、最终目录完整分页与文件唯一性已由自动化锁定；旧真实源文件曾按 root-relative path/size 成功解析并完成精确 128 KiB Range，但尚未真实验证 `/files/get_path_id` 的响应和非零 `parent_id` 行为 |
| 真实下载 CDN hostname | 2026-08-22 本地一次性只读检查；UDown/38.2.0 UA；真实 `proapi.115.com` 加密响应、DNS 与 TLS 证书 | source 下载 URL 返回 `cdnfhnfile.115cdn.net`；TLS 证书组织为广东一一五科技股份有限公司且 SAN 覆盖 `*.115cdn.net` / `115cdn.net`，Ember 仅把本次精确 hostname 加入 allowlist |
| `emby-toolkit` 小号播放行为 | `emby-toolkit` `v10.8.63`、提交 `7e64564884c9949390e5894b4be71038808e4e2a` | 只用于理解账号选择与失败语义，不复制 AGPL 代码 |
| 上游许可证 | 固定提交根 `LICENSE` / `pyproject.toml` 和模块 `pyproject.toml` 写 MIT，但模块 `LICENSE` / `LICENSE_zh` 与源码 `__license__` 写 GPLv3 | 按 GPLv3 保守边界处理：不复制、翻译或运行时依赖上游源码，只使用临时黑盒执行得到的兼容向量；这不是对上游最终许可的法律认定 |
| 115 Cookie/Web API 稳定性 | 非官方接口 | 随时可能变化，必须通过 Provider 边界隔离 |
| Ember 真实账号行为 | 部分实机确认 | 两个账号的登录/uploadinfo、旧逐级源解析、双重查重、preID、一次 Range challenge、秒传复用、目标复核、source/playback downurl、128 KiB Range 和保留文件的 preexisting 快速路径已通过；重复运行未再次上传且未调用删除。新的父目录路径接口、风控和长期稳定性仍未证实 |

证据等级：

- **公开实现确认**：固定源码提交能证明该版本如何发请求或解析响应，不代表 115 官方承诺。
- **Ember 内部合同**：Ember 自己定义并通过 fake、fixture 和状态机测试锁定的行为。
- **未实机确认**：必须在用户明确授权后，用受控账号和文件验证；验证前不能写成生产事实。

许可证边界：Ember 不复制 `emby-toolkit` 的 AGPL 实现。`p115client` / `p115cipher` 的固定提交已完成一次保守审查：上游许可声明互相冲突，因此 Ember 不复制或逐行翻译其实现，也不把 Python 包作为构建或运行时依赖；只把固定提交临时执行产生的无敏感信息输入/输出当作兼容性事实，并使用 Go 标准算法和独立依赖实现。若后续希望直接复用上游源码，仍必须先取得明确许可结论。

### 1.1 上游参考基线与已核对范围

本节是 Cookie Provider 后续跟进 `p115client` / `p115cipher` 的版本入口。Ember 运行时使用原生 Go `CookieProvider`，没有这两个 Python 包的安装或升级步骤；“已采用基线”和“已核对上游”分别记录，不能因为核对了新版本就改写既有实现和测试向量的来源。

| 用途 | 固定版本与提交 | 当前处理 |
| --- | --- | --- |
| 已采用的 Cookie 协议参考 | `p115client 0.0.9.6.4`；`608a44396fea08d36131a68beb245be1fe17aa6d` | 保留为本文各端点合同的原始依据 |
| 已采用的加密向量来源 | `p115cipher 0.0.5.4`；同一 `608a44396fea08d36131a68beb245be1fe17aa6d` 提交 | 保留 `p115cipher-0.0.5.4.json` / `p115rsa-0.0.5.4.json` 及其 `sourceCommit`、`moduleVersion` |
| 最近已核对的客户端 | [`p115client 0.0.9.7.2`](https://pypi.org/project/p115client/0.0.9.7.2/)；[`856e4f8f09ce034534bd458f06d3dee4474a1698`](https://github.com/ChenyangGao/p115client/tree/856e4f8f09ce034534bd458f06d3dee4474a1698) | 2026-10-25 完成与已采用基线的源码差异核对，未发现当前 Cookie 链路必须跟进的协议改动 |
| 最近已核对的加密模块 | [`p115cipher 0.0.6`](https://pypi.org/project/p115cipher/0.0.6/)；上述 `856e4f8f09ce034534bd458f06d3dee4474a1698` 提交内的模块 | 现有默认 RSA、AES/LZ4、上传签名行为没有需要移植的变化；未重新生成该版本向量 |

已核对范围是 [`608a443…856e4f8` 的源码差异](https://github.com/ChenyangGao/p115client/compare/608a44396fea08d36131a68beb245be1fe17aa6d...856e4f8f09ce034534bd458f06d3dee4474a1698)，不是对区间内每个发布版本逐个验收，也不是 Python 依赖的兼容版本声明。后续提交和版本不自动纳入结论。

OpenAPI 合同仍单独引用 `dc13e14132006f877183dd6347fa6da3aca6b48e`，它属于尚未启用的 OpenAPI Provider 调查依据，不作为当前 Cookie Provider 基线；本次没有复验 OpenAPI 授权、Token 或官方端点。

### 1.2 2026-10-25 上游重构核对结果

上游在 [`0.0.9.7` 重构提示](https://github.com/ChenyangGao/p115client/blob/856e4f8f09ce034534bd458f06d3dee4474a1698/readme.md#L8-L13) 中明确说明工具函数大幅调整并删除部分函数。核对结论限定于 Ember 实际使用的协议：

| 检查面 | 基线至核对提交的差异 | Ember 结论 |
| --- | --- | --- |
| Python 工具层 | 目录遍历、下载等工具调整，删除 `tool/pool.py`，新增 `tool/iter_nodes.py` | 不调用这些 Python 工具，无需跟随目录、函数或依赖重构 |
| 登录、上传信息、父目录查询 | `login_status`、`upload_info_app`、`fs_dir_getid2` 实现未变 | 保留现有请求与响应合同 |
| 目录列表、SHA1 搜索与文件字段 | `fs_files` 将排序参数 `custom_order` 的覆盖改为默认填充；`fs_search` 默认 `limit` 从 `32` 改为 `1150` 并增加整数入参；属性工具调整未改变 Ember 使用的文件身份字段映射 | Ember 自行构造请求，目标搜索显式传 `limit=100` 和 SHA1；未发现必须迁移的端点或身份字段 |
| 秒传初始化 | `upload_init`、上传版本 `36.2.28`、版本绑定 UA、签名构造未变 | 保留 `/4.0/initupload.php`、AES/LZ4 与 `status=1/2/7` 处理 |
| 下载直链 | `download_url_web` 新增 RSA 加密 POST 并作为默认方式；`download_url_app` 的 Chrome 分支没有行为变化 | Ember 使用 `/app/chrome/downurl`，不迁移到 Web 下载分支；真实播放器 UA 规则保持 |
| 加密模块 | `p115cipher 0.0.6` 新增公钥解析和可配置 RSA 块大小、公钥参数，默认仍为原 128 字节块和原公钥；AES/LZ4、token 和上传签名实现未变 | 保留独立 Go 实现和原固定向量；不引入本链路未使用的通用 RSA 能力 |
| Range 与删除 | `read_range`、`fs_delete` 实现未变 | 保留有界 Range；删除仍只是容量回收预留能力 |

关键上游定位：[`download_url_app` / `download_url_web`](https://github.com/ChenyangGao/p115client/blob/856e4f8f09ce034534bd458f06d3dee4474a1698/p115client/client.py#L8974-L9144)、[`fs_search`](https://github.com/ChenyangGao/p115client/blob/856e4f8f09ce034534bd458f06d3dee4474a1698/p115client/client.py#L17474-L17573)、[`upload_init`](https://github.com/ChenyangGao/p115client/blob/856e4f8f09ce034534bd458f06d3dee4474a1698/p115client/client.py#L27538-L27594)、[`p115cipher` RSA 默认参数](https://github.com/ChenyangGao/p115client/blob/856e4f8f09ce034534bd458f06d3dee4474a1698/modules/p115cipher/p115cipher/util.py#L343-L382)。

验证记录：

- Ember 基线为 `master` 提交 `7667ec47b251d0a8cb952e1a9f3fd3ad1a5ae30a`，核对时工作区干净。
- 校验了上述两个 PyPI 版本 wheel 的 SHA-256；`p115client` 的 `client.py` / `tool/attr.py` / `const.py` 和 `p115cipher` 的 `__init__.py` / `util.py` / `aes.py` 与核对提交内容一致，未安装这些包。
- 在 `services/api` 执行 `go test -count=1 -timeout=90s ./internal/integrations/p115/...`，适配器和加密包均通过；使用现有 fake HTTP 与 `0.0.5.4` 固定向量，没有执行 `0.0.6` 包的黑盒向量验证。
- 本次未启动服务、未请求真实 115、未执行播放器验收；公开源码比对与离线测试不能证明当前真实端点、账号或 CDN 行为。此前尚未实机确认的父目录路径接口、个人 Cookie 与客户端边界继续保留。

本轮未发现因上游重构而需要修复的问题，保持已采用基线。

### 1.3 后续跟进方式

1. 从 [PyPI 客户端发布历史](https://pypi.org/project/p115client/#history)、[加密模块发布历史](https://pypi.org/project/p115cipher/#history) 和上游 README 确认新版本，再固定待核对提交；版本号与 Git 提交分别记录。当前增量入口为 [`856e4f8…main`](https://github.com/ChenyangGao/p115client/compare/856e4f8f09ce034534bd458f06d3dee4474a1698...main)，该链接会变化，不能代替固定提交证据。
2. 优先检查 `p115client/client.py`、`p115client/const.py`、`p115client/tool/attr.py`、`p115client/fs/fs_base.py`、`modules/p115cipher/`，沿本次改变的 helper 和依赖继续追踪；只影响未使用的 Python 工具时记录结论即可。
3. 对照本文 §5–§7 核对 method/path、请求字段、响应身份字段、上传版本与 UA、加解密、状态码、错误分类、下载 URL 的 `t/c/f` 约束及 Range。涉及已采用协议时，同时与 `608a443` 基线比较，不能只看两个最新版本之间的差异。
4. 必须适配时，先补无敏感信息的 fixture/合同测试，再调整 Go Adapter 与本文；影响账号状态或播放回退时，继续覆盖 `p115account`、`directplay` 和 Gateway 的相关 fake 测试。真实外部验证仍须单独获得授权。
5. 更新本节“最近已核对”版本，并保留每次核对的日期、Ember SHA、固定上游提交、结论、测试和未验证边界。现有向量来源不随核对版本改名；只有按新固定版本实际生成并验证向量后，才记录新来源。若正式调整已采用基线，保留旧来源及迁移记录，同步受影响的合同、实现、测试和计划引用。

## 2. 首期范围

首期只支持：

- 一个管理员配置的源账号，用于读取源文件信息和 Range challenge。
- 一个管理员配置的播放小号，用于接收秒传文件和生成最终下载直链。
- 管理员通过 Ember 管理端手工录入、替换和验证 Cookie。
- Ember 原生 Go `CookieProvider`，不调用 Python 进程，不依赖 `p115client` 包。
- SHA1 与文件大小查重、秒传初始化、必要的范围校验、目标文件复核和兼容条件成立时的 HTTP 302。

首期不支持：

- 二维码登录、自动抓取 Cookie 或 Cookie 自动续期。
- 多播放账号池、自动选路、主备切换或账号借用。
- OpenAPI 与 Cookie 的隐式回退。
- Ember 下载完整视频后再上传。
- 需要最终播放器额外注入 Cookie、Authorization 等请求头的直链。

普通用户绑定本人 playback 账号不属于首期设计，但现已由后续个人账号计划实现，复用本合同；见 §4.2 与本文开头的个人账号路由计划。它不新增 source 角色或账号池。

## 3. Provider 边界

业务层只依赖内部 Provider 语义，不感知 Cookie 端点、加密格式或原始状态码。首期接口至少覆盖：

| 操作 | 内部语义 |
| --- | --- |
| `ValidateCredential` | 验证 Cookie 是否能识别账号，并返回脱敏账号标识 |
| `GetUploadInfo` | 获取上传初始化所需 `userId`、`userKey` 等账号数据 |
| `SearchBySHA1` | 按 SHA1 查询候选文件，业务层再次校验大小和文件类型 |
| `ResolveFileByPath` | 在显式根目录下按父目录路径一次解析目录 ID，再在最终目录唯一取得完整源文件身份 |
| `ResolveDirectoryByPath` | 把用户友好的 playback 根目录相对路径解析为唯一目录 ID；不创建目录 |
| `InitRapidUpload` | 发起秒传，映射为复用成功、范围校验、普通上传或失败 |
| `GetDownloadURL` | 获取下载地址及其 UA、Cookie、过期时间等使用约束 |
| `FindTargetFile` | 秒传后在目标目录复核文件并返回 fileId、pickCode |
| `HashFileRange` | 在 Provider 内部读取一个有界 Range，只返回大写 SHA1 和读取字节数 |
| `DeleteFile` | 第二阶段容量回收预留；同一账号内串行执行，第一阶段业务链路不调用 |

`CookieProvider` 组合 `CookieCredentialValidator` 与 `CookieHTTPAdapter`，并通过 Go 编译期断言完整实现上述接口。后续 OpenAPI Provider 必须实现同一组业务语义。Provider 切换要显式配置，不能在某次请求失败后静默改用另一种认证模式。

## 4. 账号与凭证合同

### 4.1 账号角色

账号角色固定为：

- `source`：管理员维护的全局源账号，只用于定位源文件、获取源下载地址和读取指定 Range。
- `playback`：播放小号，只用于目标查重、秒传落盘和签发最终直链。

管理员全局 source/shared playback 按角色各至多启用一条；个人 playback 按 owner 各至多保留一条非 revoked 记录。尝试 115 加速时必须取得唯一可运行的管理员 source 和本次套餐路由选中的 playback；账号缺失或不可用只使加速 fallback，不改变 Gateway 的普通 Emby 代理能力。两者不能是同一条账号记录或相同 Provider UID；非 revoked 账号之间也不能重复绑定同一 Provider UID。

### 4.2 Cookie 安全

- Cookie 只能通过 HTTPS 管理接口写入，使用 `CONFIG_ENCRYPTION_KEY` 加密后保存在专用账号表。
- Cookie 是只写字段：创建或替换后，任何查询接口都不得返回明文或可复原片段。
- 管理端只展示账号别名、角色、脱敏 Provider 用户标识、状态和最后验证时间。
- 不通过环境变量、命令行参数、前端构建变量或普通 settings 表保存 Cookie。
- Cookie、完整 `Set-Cookie`、上传加密材料、完整下载地址和 Provider 完整响应体不得进入日志、审计详情或测试快照。
- 当前管理员账号的 `appType` 和 User-Agent 必须作为兼容参数显式保存；`appType` 优先从 Cookie `UID` 的 `ssoent` 自动识别，User-Agent 仍由管理员显式配置，不能在不同请求里随机切换客户端身份。

个人账号不沿用管理员输入合同：创建和替换凭证时只提交 Cookie，不接收 `appType` 或 `userAgent`。后端从唯一合法 `UID` 的第二段 `ssoent` 派生 `app_type`；已知编码使用下表映射，未知编码保存固定诊断值 `unknown`，缺失、重复或非法 `UID` 直接拒绝。普通 Cookie/Web 请求的 Provider User-Agent 固定为 `Mozilla/5.0`，最终下载直链仍使用 Gateway 收到的真实播放器 User-Agent，秒传初始化继续使用协议代码内的版本绑定上传 User-Agent。该个人账号方案已实现，固定 Provider User-Agent 的外部适配仍只有公开源码依据，尚未经过目标个人 Cookie 的真实 115 验证。

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

#### 5.1.1 Cookie 客户端类型识别

固定公开源码把 Cookie `UID` 解析为 `<userId>_<ssoent>_<timestamp>`，并使用第二段 `ssoent` 映射登录客户端。Ember 在创建和替换 Cookie 时只做同样的本地解析，不调用设备列表或 SSO 检查接口：

| `ssoent` | Ember `appType` | `ssoent` | Ember `appType` |
| --- | --- | --- | --- |
| `A1` | `web` | `D1` | `ios` |
| `D2` | `bios` | `D3` | `115ios` |
| `F1` | `android` | `F2` | `bandroid` |
| `F3` | `115android` | `H1` | `ipad` |
| `H2` | `bipad` | `H3` | `115ipad` |
| `I1` | `tv` | `I2` | `apple_tv` |
| `M1` | `qandroid` | `N1` | `qios` |
| `O1` | `qipad` | `P1` | `os_windows` |
| `P2` | `os_mac` | `P3` | `os_linux` |
| `R1` | `wechatmini` | `R2` | `alipaymini` |
| `S1` | `harmony` |  |  |

当前管理员控制面的内部合同：

- 已知 `ssoent` 的识别结果是权威值，覆盖请求中可能携带的 `appType`，并写入现有 `p115_accounts.app_type`。
- `A1` 同时被公开实现用于 `web` 和 `desktop`；Ember 统一归一为 `web`，不声称能进一步区分浏览器类型。
- 未知、缺失或重复 `UID` 不猜测客户端；创建或替换请求必须显式提供合法 `appType` 作为兼容兜底。
- `appType` 只描述 Cookie 登录客户端，不能推导完整原始 User-Agent；User-Agent 继续单独保存。
- Cookie 替换必须在同一次持久化更新中写入新 `app_type`，并把账号重置为 `pending + disabled`；旧客户端类型不能残留。
- 识别不调用 `login_devices`；更不能调用可能影响同设备旧 Cookie 的 `check/sso`，客户端识别失败不扩大为外部副作用。

后续个人账号控制面复用同一映射表，但不接受人工兜底：未知编码保存 `unknown`，缺失、重复或非法 `UID` 拒绝创建或替换。`appType` 只是诊断元数据，当前 Provider 不读取它来选择端点或改变协议。

证据：固定提交的 [`P115UID`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/p115client/type.py#L39-L46)、[`APP_TO_SSOENT`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/p115client/const.py#L48-L78) 与[客户端默认 Header](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/p115client/client.py#L518-L528)。固定源码中的普通请求默认 User-Agent 为 `Mozilla/5.0`。这些都是公开实现兼容证据，不是 115 官方稳定承诺；新增编码必须先核对并更新本表和合同测试，个人账号固定 UA 仍需受控真实验证。

### 5.2 上传信息

公开实现确认的入口：

```http
GET https://proapi.115.com/app/uploadinfo
Cookie: <account-cookie>
```

响应用于取得上传初始化需要的 `user_id` 和 `userkey`。字段缺失、账号不一致或响应业务状态失败时，账号验证失败。

Ember 的 `CookieHTTPAdapter.GetUploadInfo` 当前固定：

- 只发送 `GET /app/uploadinfo`，不带 query 和请求体；Header 仅承载固定 `Accept: */*`、账号 Cookie 和账号 User-Agent，不发送 `Authorization`。
- 只接受 HTTP `2xx`、顶层布尔 `state=true`、顶层十进制 `user_id` 和非空 `userkey`；`user_id` 可为 JSON number 或十进制 string，进入内部合同前规范化为十进制 string。
- 响应 `user_id` 必须与请求 Cookie 的规范化 `UID` 一致；不一致按凭证拒绝处理，禁止把其他账号的 `userkey` 带入上传初始化。
- `state=false` 映射为 `ErrProviderRejected`；网络、响应体读取中断及一般非 `2xx` 映射为 `ErrProviderUnavailable`，其中 HTTP `429` 单独映射为 `ErrProviderRateLimited`；调用方取消/deadline 保留 context 错误。字段缺失、非法 JSON 或超限响应映射为 `ErrProviderProtocol`。这些 sentinel 是否回写账号健康还取决于运行期操作，见 §10.2。
- 错误不携带请求 URL、Cookie、`userkey`、响应正文或 Provider 原始错误文本。

证据：[`upload_info_app`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/p115client/client.py#L26750-L26783) 固定 method/path；[`py115 UploadInfoApi`](https://github.com/deadblue/py115/blob/7f96f039be7ec62b937ee41290ba9469aca6921e/src/py115/lowlevel/api/upload.py) 固定顶层 `user_id` / `userkey` 映射。二者都属于公开实现证据，不代替目标账号实测。

#### 5.2.1 源文件路径解析

Emby `PlaybackInfo` 提供媒体源 `Path` 和可能缺失或不可靠的 `Size`，不能提供可直接信任的 115 文件身份。首期唯一启用的 source 账号必须同时配置 `embyPathPrefix + sourceRootId`；DirectPlay 先按完整目录边界移除该账号的 Emby 前缀，再把剩余部分作为 slash 分隔的相对路径交给 `CookieHTTPAdapter.ResolveFileByPath`。Emby Size 不进入该查询。当前是一对一账号配置，不建立独立路径映射表：

- `embyPathPrefix` 必须是非 `/` 的绝对 Unix 路径，禁止尾随 `/`、反斜杠、空段、`.`、`..`、NUL 或换行；`/mnt/source2` 不能命中 `/mnt/source`。
- `rootId` 必须是显式十进制目录 ID；相对路径不允许绝对路径、反斜杠、空段、`.`、`..`、NUL 或换行，不执行会改变文件名语义的 path clean。
- 相对路径总长上限为 `4 KiB`，单段上限为 `1024` 字节；这些是 Ember 首期安全边界，不是 115 官方限制。
- 相对路径只有最终文件名时，父目录直接使用显式 `rootId`；存在父目录时固定发送 `GET /files/get_path_id`，query 只包含 `parent_id=rootId`、slash 分隔的相对父目录 `path` 和 `is_create=0`。播放解析禁止创建目录。
- 路径响应必须有顶层布尔 `state=true`，目录 ID 只接受顶层 `id` 或 `data.file_id` 的正十进制值；两者同时存在时必须相等。缺字段、非法值或冲突按协议错误，零 ID 按未找到处理，业务拒绝不回显原始响应。
- 取得父目录 ID 后固定发送 `GET /files`，query 为 `aid=1`、该 `cid`、`cur=1`、`show_dir=1`、`fc_mix=1`、`count_folders=1`、`o=file_name`、`asc=1`、`limit=200` 和当前 `offset`。
- `/files` 响应必须有顶层布尔 `state=true`，并严格映射 `cid/count/offset/data`；响应 `cid` 与请求不一致时视为未找到，防止无效目录被 Provider 静默回退到根目录。
- 固定映射 Web 列表短字段：目录使用 `cid/pid/n`，文件使用 `fid/cid/n/pc/sha/s`。每一项的 parent 必须等于请求 `cid`，返回文件必须包含合法 pickCode、SHA1 和 size。
- 最终文件在已经确定的父目录内使用“精确文件名 + 非目录”匹配。零候选返回 `ErrSourceFileNotFound`，多个同名候选即使 Size 不同也返回 `ErrSourceFileAmbiguous`，禁止任意选择第一条。唯一命中后必须由 115 响应提供合法 fileId、pickCode、SHA1、正数 Size 和正确 parentId；这些 Provider 字段才是后续文件身份。
- 为了检测最终文件同名项，最终目录必须读取完整分页快照；快照 count 变化或分页不连续按协议错误处理。首期最多检查 `10,000` 项，超过返回 `ErrSourceDirectoryTooLarge`。中间目录由 Provider 路径接口选择，Ember 不再自行枚举并判断同名目录歧义。
- 最终返回的 `fileId/pickCode/SHA1/size/parentId` 才是源文件身份；文件名和路径本身不能替代内容身份。
- 源解析的业务拒绝、响应协议异常和无效源文件身份只使本次 DirectPlay 失败，不据此回写源账号 `error`；目录/文件未找到、同名文件歧义和目录超限同样属于请求级错误。Gateway 继续按 `providerOperation=resolve_source_path` 记录失败并回退 Emby，DirectPlay 释放本次新建的播放 reservation。明确的 `ErrCredentialRejected` 和 `ErrProviderUnavailable` 仍分别触发凭证停用与临时冷却；源 preID/challenge 的文件级业务和协议错误同样不代表整个账号故障；播放账号操作按 §10.2 区分已识别文件失败与账号级错误。

2026-09-08 源目录并发读取合同：

- 同一个 Cookie Adapter 内，相同账号 ID、精确 Cookie、AppType、Provider UA、规范化 root ID 和相对父目录共享正在进行的“父目录解析 + 完整分页”；不同文件名各自做唯一性匹配，根目录文件也共享目录读取。合并键使用长度分隔字段的 SHA256 摘要，不保存明文 Cookie，也不输出摘要。
- 只合并时间重叠的调用；成功或失败完成后立即移除，不缓存完整文件身份或目录快照，不跨进程共享。顺序播放、下一集或后续重试仍重新访问 Provider；每个调用者返回独立 File 值。
- 每个等待者独立响应取消或 deadline；最后一个等待者离开时取消上游请求并移除任务。旧任务稍后结束不能删除同键的新任务。共享读取从创建起固定最多 `30s`，包含父目录解析与全部分页；DirectPlay 必须给 `ResolveFileByPath` 同样的 30s 预算。该预算耗尽映射 `ErrSourceDirectoryTimeout`，Gateway 按 `source_read_timeout` 回退 Emby，不回写账号健康，也不冒充客户端 deadline 或 Provider 宕机。每个 HTTP 请求仍使用既有 `10s` 上限。
- 用户资格、账号健康、租约/转存配额和最终下载 URL 不在共享范围内；仍由各自调用链检查。Debug 每次共享读取结束只记录一条 `source_directory_resolved`，包含 shared、callers、entries、parentResolveMs、listMs、listPages、durationMs、success；listPages 统计实际尝试页数（含失败页），不记录凭证、路径、Provider ID、摘要或响应体。DirectPlay 各调用者另在 Gateway 最终决策日志携带自身等待源解析及后续各阶段的耗时，不把共享工作时间重复累加。
- **中间目录重名仍未收口**：路径接口只返回一个 ID，现有证据不能证明其会拒绝同名中间目录；最终文件唯一性不等于完整路径唯一性。并发合并不恢复旧逐层歧义检查。后续必须用固定协议证据或明确授权的只读验证确认；若不能保证唯一性，再单独评估逐层校验的请求成本。目标账号的非零 `parent_id`、不存在目录、重名目录和真实耗时仍未证实。

证据：固定提交的 [`fs_dir_getid2`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/p115client/client.py#L10837-L10885) 固定 `GET /files/get_path_id`、`path/parent_id/is_create` 和仅返回目录 ID 的边界；[`fs_files`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/p115client/client.py#L11481-L11640) 固定最终目录 method/path/query 与分页字段；[`normalize_attr_web`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/p115client/tool/attr.py#L80-L180) 固定 Web 短字段语义。父目录路径接口已有 fake HTTP 合同，尚未使用目标账号执行真实只读验证。

`fs_dir_getid2` 的这段公开实现只转发响应，没有定义成功、目录不存在或业务拒绝的完整响应结构。顶层 `id` / `data.file_id` 的接受规则属于 Ember 当前的内部适配合同；目录不存在是否使用 `state=false`、非零 `parent_id` 与同名目录选择仍未证实，不猜测错误码，也不把未知业务拒绝当作账号失效证据。

#### 5.2.2 playback 目录路径解析

`CookieHTTPAdapter.ResolveDirectoryByPath` 复用 `/files` 分页合同，把管理员输入的 `/EmberPlayback` 或 `EmberPlayback` 解析为唯一目录：

- `rootId` 必须是显式十进制 ID；相对路径允许一个前导 `/` 作为 UI 表达，但禁止根目录 `/`、尾随 `/`、反斜杠、空段、`.`、`..`、NUL 和换行。
- 每一级读取完整目录分页，只接受 `isDirectory=true` 且名称完全一致的候选；文件与目录同名时忽略文件，多个同名目录返回 `ErrDirectoryAmbiguous`。
- 响应 cid 回退、目录不存在、路径中间节点不是目录和最终节点是文件都返回 `ErrDirectoryNotFound` 或协议错误，禁止默认写入根目录。
- 返回 `Directory{ID, ParentID, Name, Path}`，其中 `Path` 是带单个前导 `/` 的规范化展示路径；秒传、复核和未来清理只使用 `ID`。
- 该方法只读，不创建、移动或重命名目录；Cookie、原始路径和目录响应不进入普通日志或检查报告。

当前 Provider 方法和 fake HTTP 合同已实现；管理员 API/Web 的路径解析体验仍按现行计划 TODO 后续落地。

### 5.3 SHA1 查重

公开实现确认的入口：

```http
# 不带目标目录的全局探测
GET https://webapi.115.com/files/shasearch

# 带目标目录的 playback 查重
GET https://webapi.115.com/files/search?cid=<target>&search_value=<SHA1>&...
Cookie: <playback-account-cookie>
```

旧 `shasearch` 最多返回一个全局候选，不能约束目标目录，并通过业务错误表达未命中，因此不得用于 playback 专用目录的 preexisting 判断。带 `parentId` 的查询必须改用目录作用域 `/files/search`；Ember 不能只信任第一条或文件名，命中后必须同时校验：

```text
candidate.sha1 == expectedSHA1
&& candidate.size == expectedSize
&& candidate.isDirectory == false
```

如果无法拿到 size 或文件类型，结果只能作为候选，不能直接签发 302。秒传完成后还必须在目标目录复核。

Ember 的 `CookieHTTPAdapter.SearchBySHA1` 当前固定：

- 输入 SHA1 必须是 40 位十六进制并规范化为大写；size 必须非负，可选 `parentId` 必须是十进制 ID。
- `parentId` 为空时发送 `GET /files/shasearch?sha1=<UPPER_SHA1>`；成功响应为单个 `data` 对象，同时接受固定 normalizer 覆盖的 Web 短字段 `fid/cid/n|fn|file_name/pc/sha|sha1/s|fs`，以及旧 app2 长字段 `file_id`、`parent_id | category_id`、`file_name`、`pick_code`、`sha1 | file_sha1`、`file_size`。
- `parentId` 非空时不调用全局 `shasearch`，而复用一次目录作用域 `/files/search`：固定发送目标 `cid`、SHA1、`fc=2`、`show_dir=0`、`type=99` 和首 100 条，只接受 Web 短字段数组中的唯一精确候选；多个精确候选失败关闭。
- 两条路径最终都映射为同一份完整文件身份；只有 SHA1、size、`isDirectory=false` 以及作用域查询的父目录全部匹配时才返回一个候选，不匹配统一返回空列表，调用方仍需在业务层重复校验。
- 固定公开源码确认的 `state=false + error="文件错误"` 映射为空列表；其他业务拒绝映射为 `ErrProviderRejected`，不向上暴露 Provider 原文。

证据：[`fs_shasearch`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/p115client/client.py#L17349-L17392)、[`normalize_attr_web`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/p115client/tool/attr.py#L59-L135)。

#### 5.3.1 目标目录复核与可见性

`status=2` 只表示上传初始化接受了秒传复用，不能证明目标目录已经可查询。调用方随后必须执行 `CookieHTTPAdapter.FindTargetFile`：

- 固定请求 `GET /files/search`，query 为 `aid=1`、目标 `cid`、`fc=2`、`limit=100`、`offset=0`、`search_value=<UPPER_SHA1>`、`show_dir=0`、`type=99`。
- 固定映射 Web 搜索短字段 `fid/cid/n/pc/sha/s`，字段缺失、非法数字、非法 SHA1 或 Header/JSON/HTTP 错误立即失败，不进入重试。
- 只有 `sha == expectedSHA1`、`s == expectedSize`、`sha` 非空所表达的 `isDirectory=false`、`cid == expectedParentId` 全部成立时才接受候选；文件名不能替代内容身份。
- 同一次查询返回多条精确候选时返回 `ErrTargetFileAmbiguous`，禁止任意选择第一条。
- 首次查询立即执行；只有正常空列表或没有精确候选时继续轮询，默认每 `500ms` 查询一次，整体可见性窗口固定为 `10s`。
- 最后剩余时间不足一个轮询间隔时只等待剩余时间，并在截止点执行最后一次查询；仍不可见返回 `ErrTargetFileNotVisible`。
- context 已取消时不发送请求；等待期间取消则原样返回 `context.Canceled` / `context.DeadlineExceeded`。

以上间隔与超时是 Ember 的内部首期策略，不是 115 官方时序保证。真实账号验证必须记录实际可见延迟；调整默认值时需要同步更新 fake clock 测试和本合同。

证据：[`fs_search`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/p115client/client.py#L17012-L17131) 固定 method、path、默认 query 与短字段响应；[`p115tiny302 sha1_to_id`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/modules/p115tiny302/p115tiny302/app.py#L85-L112) 固定按 SHA1/size 复核候选的公开实现语义。

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

Ember 的 `CookieHTTPAdapter.InitRapidUpload` 当前固定：

- 调用前先校验 filename、完整 SHA1、正数 size、十进制目标目录、可选 preID，以及成对出现的 `sign_key/sign_val`；非法输入在任何 HTTP 请求前返回 `ErrInvalidRequest`。
- 先通过同一账号的 `GetUploadInfo` 获取并核对 `userid/userkey`，再构造 `target=U_1_<parentId>`；固定提交的 `_app_version` 和版本绑定上传 User-Agent 只保留在协议代码中，不进入业务配置或日志。
- 只发送 `POST /4.0/initupload.php?k_ec=<token>`；请求体是 `BuildUploadRequest` 生成的 AES-CBC 密文，应用层 Header 固定包含 Cookie、`Accept: */*`、`Content-Type: application/x-www-form-urlencoded` 和上传专用 User-Agent，不发送 `Authorization`。
- 固定向量覆盖 `fileid`、`filename`、`filesize`、`target`、可选 `preid`、`sign_key/sign_val`、`topupload=true`、`userid/userkey`、appVersion、`sig/token` 和密文；测试数据不含真实账号或文件。
- HTTP `2xx` 响应先执行 AES-CBC 解密和 LZ4 block 解压，再解析顶层 JSON；网络、非 `2xx`、超限密文、解密失败、非法 JSON 和字段错误均返回脱敏错误，不保存原始响应。
- 不自动重试固定源码注释中的偶发 `401`；在真实账号验证明确重试条件、次数和幂等边界前，非 `2xx` 都失败：HTTP `429` 使用既有 `ErrProviderRateLimited`，其余沿用 `ErrProviderUnavailable`，不按未知业务码添加重试或风控推断。

状态映射固定为：

| 外部响应 | 内部状态 | 约束 |
| --- | --- | --- |
| `status=1` | `ordinary_upload_required` | 明确结束，不触发普通上传 |
| `status=2` | `reused` | 不信任响应中的占位文件字段，继续目标目录复核 |
| `status=7` | `range_challenge` | 必须有非空 `sign_key` 和合法的包含端点 Range，且 `end < sourceSize` |
| 未知 `status` 或 `state=false` | `provider_rejected` | 仅保留短 Provider code，不暴露 message/error/正文 |

证据：[`upload_init`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/p115client/client.py#L26785-L26961)。

### 5.5 下载地址

公开实现的 `download_url` 支持多个 app/Web 入口。Ember 首期离线合同固定 Cookie 模式的 Chrome App 入口：

```http
POST https://proapi.115.com/app/chrome/downurl
Cookie: <playback-account-cookie>
User-Agent: <actual-playback-client-user-agent>
Content-Type: application/x-www-form-urlencoded

data=<RSA encrypted {pickcode,user_id}>
```

真实播放客户端 User-Agent 必须原样参与本次直链签发，不能使用账号配置的 Provider User-Agent 代替。请求明文只包含单个合法文件 pickCode 和从 Cookie `UID` 规范化的 user ID；RSA 请求与响应变换由 `p115cipher` 边界负责，业务层不接触协议常量。

成功响应固定为顶层布尔 `state=true` 和 RSA 加密的 `data` string；`state=false` 先按 §5.5.1 读取 errno，不要求失败响应仍携带 string data。解密结果必须只有一个文件条目，且条目的 `pick_code` 与请求完全一致；`url=false/null/缺失/空` 按文件不可用拒绝，非法 URL 字段类型、多条记录和 pickCode 错配仍属于协议错误。RSA 请求向量和任意密文解码向量来自固定 `p115cipher 0.0.5.4` 黑盒输出；HTTP 测试通过 cipher seam 返回合成 JSON，不伪造服务端私钥。

公开实现指出下载链接包含以下约束信号：

- `t`：链接过期时间。
- `u`：账号标识。
- `c`：并发打开限制。
- `f=0`：未声明额外 Header 要求。
- `f=1`：要求使用获取直链时相同的 User-Agent。
- `f=3`：除相同 User-Agent 外，还要求携带直链响应给出的 Cookie。

`CookieHTTPAdapter.GetDownloadURL` 当前固定：

- 只接受绝对 HTTPS URL；禁止 userinfo、显式端口、fragment、IP literal 和非 allowlist 主机。
- 首期 allowlist 允许 `115.com` 及其子域，并根据 2026-08-22 受控真实响应额外精确允许 `cdnfhnfile.115cdn.net`；不因此开放整个 `*.115cdn.net`。所有匹配都使用完整 hostname 边界，其他 CDN hostname 必须取得同等级真实证据后再逐项加入。
- `t`、`c`、`f` 必须各出现一次：`t` 是严格晚于当前时间的 Unix 秒并原样映射为 UTC `ExpiresAt`；`c` 是非负并发打开上限，`0` 表示无限；`f="" | 0 | 1 | 3` 分别映射为 none、none、same_user_agent、same_user_agent_and_cookie。
- 未知 `f` 返回 `ErrDownloadURLIncompatible`；已过期返回 `ErrDownloadURLExpired`；负数 `c` 返回文件级 `ErrFileUnavailable`；域名/协议不允许返回 `ErrDownloadURLNotAllowed`。
- URL 安全策略拒绝使用类型化 `DownloadURLPolicyError` 保留固定 reason、受限 scheme 和仅含 ASCII hostname；普通 `Error()`、API 和日志仍只看到通用 sentinel。一次性检查器可以显式读取这三个安全字段定位合同漂移，但不能输出 path、query、userinfo、端口值、IP 或完整 URL。
- Adapter 返回 Provider 原始过期边界，不在这里扣除缓存安全窗口；后续缓存层必须以 `ExpiresAt` 为上限并预留安全窗口。
- URL、Cookie、RSA 数据、Provider response 和原始错误文本不进入 JSON、日志或错误消息。

`f=3` 会被准确映射，但首期播放网关仍必须按不兼容拒绝 302，因为最终播放器未被证明能够自然携带响应 Cookie。不能把 Cookie 拼进 URL，也不能泄露播放小号凭证。

证据：[`download_url`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/p115client/client.py#L8300-L8457)、[`download_url_app`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/p115client/client.py#L8565-L8657) 和固定源码的 [`p115tiny302 get_downurl`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/modules/p115tiny302/p115tiny302/app.py#L273-L299)。source 账号的 `f=1`、并发上限 `2`、精确 CDN hostname 和同 UA Range 已实测；playback 账号最终直链、完整 CDN 集合和真实 Infuse Header 行为尚未确认。

#### 5.5.1 downurl 文件失败与 errno（未经真实账号证实）

本节依据社区逆向实现定义 Ember 的兼容映射，**未经真实账号证实**，不是 115 官方稳定错误码合同。仅当 HTTP 成功且顶层 `state=false` 时读取 `errno`；接受 JSON 整数或整数 string，不按 `error/message` 文本猜测账号状态。

| downurl 响应 | 公开实现语义 | Ember 映射与账号影响 |
| --- | --- | --- |
| `errno=99`、`990001` | 未登录 `ErrNotLogin` | `ErrCredentialRejected`；实际调用账号进入 `expired + disabled` |
| `errno=70005`、`231011` | 文件不存在或已删除 | `ErrFileUnavailable`；当前文件不可用，fallback，不回写账号健康 |
| `errno=50003`、`50015` | pickCode 不存在，或不存在/已删除 | `ErrFileUnavailable`；与文件不存在同样隔离 |
| 缺失、无法识别或其他 `errno` | 未确认 | 保持 `ErrProviderRejected`，不增加凭证失效或风控推断 |
| 单一且 pickCode 匹配的解密条目中 `url=false/null`、缺失 URL 或空 URL | 无有效文件下载地址 | `ErrFileUnavailable`；不作为账号协议错误 |

证据：社区 Go 实现 [`SheltonZhu/115driver` 固定提交 `542720cb0034954750454e89f32b4852add6e2a9` 的 `pkg/driver/error.go`](https://github.com/SheltonZhu/115driver/blob/542720cb0034954750454e89f32b4852add6e2a9/pkg/driver/error.go) 给出上述 errno 表；同提交 [`pkg/driver/download.go` 的 `FileDownloadUrl.UnmarshalJSON` 与 `Valid`](https://github.com/SheltonZhu/115driver/blob/542720cb0034954750454e89f32b4852add6e2a9/pkg/driver/download.go#L14-L36) 明确容纳 `false/null`。这里只引用公开协议事实，不引入上游运行时依赖。真实端点是否在所有下载场景使用同一错误码仍未证实，未知码不类推。

响应体读取遇到连接中断或 `http.Client.Timeout`，且调用方 context 仍有效时，downurl 与通用 Cookie GET 返回 `ErrProviderUnavailable`，沿用 1 分钟共享冷却；调用方 context 已取消或到期时保留原 context 错误，不写账号故障。成功读取后的超限、解密失败和非法 JSON 仍属于协议错误。

DirectPlay 对已识别的文件不可用、失效/空下载候选、负并发限制、秒传业务拒绝、目标文件复核不符及文件级 challenge 失败只结束当前请求/任务，不据此回写账号 `provider_protocol`。HTTP `429` 沿用 `ErrProviderRateLimited` 与短期限流冷却；裸 HTTP `403` 沿用 `ErrProviderUnavailable` 与默认 1 分钟不可用冷却。未经确认的 115 专有业务码保持既有拒绝分类，不据其猜测限流或账号风控；显式凭证错误与传输故障继续使用原健康状态机。

### 5.6 删除

公开实现确认的入口：

```http
POST https://webapi.115.com/rb/delete
Cookie: <playback-account-cookie>
```

公开实现明确提示删除操作不要并发执行。Ember 必须按播放账号串行清理，并在删除前再次校验目标账号、文件 ID 和 Ember 任务归属，不能把搜索候选直接当作可删除对象。

本节固定的是 Provider 删除协议，不代表第一阶段启用自动删除。第一阶段 playback 专用目录作为持久播放缓存，direct play Service、会话收口和受控写入检查器都不得调用 `DeleteFile`；该方法只为第二阶段经过独立设计和验证的容量回收保留。

`CookieHTTPAdapter.DeleteFile` 当前固定：

- 只接受一个正十进制 file ID；进入 HTTP 前规范化 ID，禁止逗号批量值、目录猜测或其他自由表单字段。
- 固定发送 `POST /rb/delete`，body 仅为 `fid=<canonical-id>`；Header 使用账号 Cookie、账号 User-Agent、`Accept: */*` 和 `Content-Type: application/x-www-form-urlencoded`，不发送 `Authorization`。
- 只接受 HTTP `2xx` 和顶层布尔 `state=true`；`state=false` 映射为 `ErrProviderRejected`，网络/HTTP/JSON/超限错误沿用脱敏 Provider 错误边界。
- 串行键使用 Cookie 规范化后的 Provider UID，不使用 Ember 账号记录 ID；同一真实 115 账号即使被不同记录引用，也不会在同一进程内并发删除。
- 同一 UID 的等待支持 context 取消，取消的 waiter 不发送 HTTP；不同 Provider UID 不互相阻塞。
- 真正发出删除 HTTP 前走同一账号 Cookie/Web 数据面 FIFO 限流；本地 `MaxWait` 拒绝不发送删除请求。CDN Range 读取不取令牌。
- 锁注册表由进程内所有 `CookieHTTPAdapter` 共享并在无持有者/等待者时清理。未来播放网关多实例部署时，业务层必须再使用数据库任务所有权和 advisory lock 或等价分布式互斥，不能把进程内锁误当成全局锁。

Adapter 不判断一个文件是否应当删除。第二阶段若启用容量回收，业务层必须重新核对成功任务归属、播放账号、目标 parent、file ID、SHA1、size、最后访问时间、保留策略和活跃会话；任一不一致都不得调用删除端点。

证据：[`fs_delete`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/p115client/client.py#L10457-L10513)。

## 6. 上传加密合同

当前固定 `p115cipher` 实现确认：

- 请求负载使用 AES-128-CBC 处理。
- 响应 AES-CBC 解密只处理 `len(ciphertext) & -16` 个字节，即所有完整 AES blocks；真实响应可能携带不足 16 字节的 opaque suffix，该 suffix 不参与解密。没有任何完整 block 时仍按非法密文拒绝。
- 响应在解密后进行 LZ4 解压；固定实现只在剩余数据多于 2 字节且 block 长度非零时继续，剩余 1–2 字节或零长度终止头结束解压，正数 block 长度超出剩余数据仍按坏帧拒绝。
- `k_ec` token 包含时间戳、公钥材料和 CRC 校验。
- `make_upload_payload` 根据 `userkey`、`userid`、文件 SHA1、大小、目标目录、二次校验参数、时间戳和 app 版本生成签名与 token。

实现要求：

1. 加密适配器与 Provider HTTP 逻辑分层，业务层不接触密钥材料。
2. 从固定提交生成不含真实账号信息的请求和响应测试向量。
3. Go 测试必须覆盖加密、解密、LZ4、签名、token 和单字节变更导致结果变化。
4. 不把公开实现中的协议常量重复写入文档、日志或业务配置。
5. 公开实现升级时先对比向量和端点合同，不能直接追随最新版。

当前 Ember 离线 PoC 位于 `internal/integrations/p115/p115cipher/`：

- 固定向量记录来源仓库、提交和模块版本，不包含真实账号、Cookie、文件或目录信息。
- `EncodeToken` / `DecodeToken` 覆盖 `k_ec` 时间戳、公钥材料和 CRC；解码拒绝被篡改的 CRC。
- `EncryptRequest` 与 `DecryptResponse` 覆盖协议 AES-CBC 填充语义、响应短尾部兼容、长度前缀 LZ4 block 解压及其固定终止语义，解压结果设置上限；失败只向检查器暴露 `aes` / `lz4` 子阶段，不暴露密文或明文。
- `BuildUploadRequest` 覆盖 filename、preID、topupload、`sig`、`token`、参数排序和请求密文；单字节输入变化必须改变派生结果。
- `RSAEncrypt` 覆盖 Chrome downurl 请求包装；`RSADecrypt` 使用固定任意密文黑盒向量锁定服务端响应变换，不把测试 seam 当作真实服务端密文证据。
- `GetUploadInfo`、`ResolveFileByPath`、`ResolveDirectoryByPath`、`InitRapidUpload`、`FindTargetFile`、`GetDownloadURL` 和 `HashFileRange` 均已通过 fake HTTP 合同接入。2026-08-22 真实只读运行已覆盖账号验证、当时逐级 `/files` 的源路径解析、SHA1 查重、source downurl 和 128 KiB Range；这份旧证据不证明 2026-09-07 新增的 `/files/get_path_id` 父目录快速解析。受控写入在补齐 AES 短尾部与 LZ4 终止语义后返回 `outcome=passed`，覆盖双重查重、preID、一次 Range challenge、`reused`、目标复核、playback 最终直链和 128 KiB Range。随后用保留文件完成 preexisting 快速路径，确认不再上传、重新签发 playback 直链并再次通过 Range。

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

`CookieHTTPAdapter.HashFileRange` 当前固定：

- 输入必须是非目录文件、合法 pickCode、正文件大小，以及完全位于文件内部的包含端点 Range；单次最多读取 `1 MiB`，超出在任何 HTTP 前返回 `ErrInvalidRequest`。
- 使用源账号的配置 User-Agent 获取下载 URL，并使用同一 User-Agent 发起 Range GET；`f=3` 只允许在这个服务端内部读取路径携带源账号 Cookie，Cookie 不返回业务层。最终播放器的 `f=3` 仍按不兼容处理。
- 请求固定包含 `Accept-Encoding: identity` 和 `Range: bytes=<start>-<end>`，禁止透明压缩改变字节语义。默认使用账号 User-Agent；受控 playback Range 可以显式传入本次测试/播放器 User-Agent，签发下载 URL 和读取 Range 必须使用同一个值。
- 只接受 `206 Partial Content`；`Content-Range` 的 start/end/总文件大小和 `Content-Length` 必须与请求完全一致。`200`、压缩响应、短读、长读和错误范围全部失败关闭。
- 最多读取“期望长度 + 1”字节用于检测上游越界，读取完成后只返回大写 SHA1 与 `BytesRead`；源字节、签名 URL 和 Cookie 不离开 Provider 边界。

证据：固定提交的 [`read_range`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/p115client/fs/fs_base.py#L3827-L3884) 固定下载 URL Header 与 HTTP Range 用法；[`upload_file_init`](https://github.com/ChenyangGao/p115client/blob/608a44396fea08d36131a68beb245be1fe17aa6d/p115client/client.py#L4090-L4180) 固定 `status=7` Range callback 计算 SHA1 的公开实现语义。`1 MiB`、精确响应校验和只返回 Hash 是 Ember 的内部 fail-closed 合同。

### 7.1 源读取故障诊断与预算（2026-10-12）

- 生产日志先出现父目录解析约 10 秒失败，后又出现目录解析成功、起播许可继承通过，但 `hash_source_preid` 在约 1.21 秒失败。现有样本无法确定后者是直链响应解析、解密还是 Range 校验失败，不把任一假设当作协议事实。
- Debug `provider_operation_completed` 区分 `source_parent/source_list/download_url/source_range/cookie_get/rapid_upload`，记录固定 reason、数值 HTTP 状态、networkPhase、DNS/连接/TLS/首字节与总耗时；复用连接的前几项可以为零。`download_url` 描述源取链；`source_range` 的 trace 只在实际 CDN 请求前创建，其网络与首字节耗时不包含先前的取链请求。Debug `source_range_completed` 汇总 `sourceDownloadMs/rangeReadMs/durationMs/success`；前两项分别是取链与 Range 阶段耗时，整次 Hash 总耗时仍计入 DirectPlay `preIDMs/challengeMs`，不可再次相加。HTTP trace 字段不携带域名、IP、URL、Cookie 或原始响应。秒传的上传信息 GET 与实际上传 POST 分别观测；`rapid_upload` 保留脱敏阶段、HTTP 状态与取消语义，协议错误仍可携带既有有界检查器证据。
- 失败元数据沿 Provider → DirectPlay → Gateway 传递，最终决策增加 `providerStage/providerReason/providerHTTPStatus`；嵌套直链失败保留 download_url 阶段，不误报为 CDN Range 失败。无精确细分的外部错误使用固定兜底原因，不输出原始错误。
- DirectPlay 源路径读取（`ResolveFileByPath`）使用与共享目录 flight 对齐的 30 秒预算。每次 preID/challenge 的获取源直链与实际 CDN Range 各有独立 5 秒窗口，不再由 Service 或 Adapter 额外套 10 秒总定时器；两阶段串行且名义预算之和仍为 10 秒，后者包含 DNS、TCP/TLS、响应头和完整 body 读取。取链窗口包括本地数据面排队及响应解析；取链失败后不发起 Range。各阶段受调用方更早 deadline 和整次加速 2 分钟总 deadline 约束，不新增部署配置。
- Adapter 的本地阶段超时使用 `ErrSourceRangeTimeout`，DirectPlay 统一映射为 `source_read_timeout` 并保留可用的失败 HTTP 阶段；客户端取消/自身 deadline 保持原有语义。本地预算耗尽与真实网络错误分开，不污染账号健康。共享目录的 30 秒总预算、单 HTTP 10 秒底层上限仍保留；最后一个等待者退出会取消共享请求。
- 2026-10-12 在部署 `preview-f18c84e`、Emby `4.9.3.0`、Infuse `8.5.6` 上的受控抓包，观察到旧的 5 秒共享 preID 预算分别在 TLS 和等待响应头阶段耗尽；后一例 TCP 建连约 183 ms、源取链约 2.37 s，不能只归因为 SYN 重传。另一个首次转存样本成功，停止后重播命中缓存。该证据指导预算拆分，不证明本次修复已经部署或首帧延迟已经改善；原始抓包和完整响应不进入仓库。
- 不放宽 Range 的状态、范围、编码、长度校验，不自动重试 retained 写入，不把未经验证的外部响应当成功。真实 115 故障根因仍需要部署后细分日志确认；本次验证依赖 mock 和 PostgreSQL 隔离集成。

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

### 8.1 完整秒传到 playback 并播放流程

以下是 source 文件在 playback 缺失时的唯一首期闭环。各步骤不能交换顺序，也不能把 source 下载地址直接交给播放器：

1. 运行时加载唯一可运行的 source 和 playback 账号，确认两个 Provider UID 不同；稳定态必须为 `active + enabled`，已到期 `cooling_down` 仅允许数据库半开租约持有者继续。playback 必须配置明确的 `targetParentId`，禁止默认写入根目录。
2. 使用 source 账号的 `embyPathPrefix + sourceRootId` 把 Emby `Path` 转换为 `rootId + relativePath`。相对路径存在父目录时，`ResolveFileByPath` 先以 `parent_id=rootId + path=相对父目录 + is_create=0` 一次取得父目录 ID；随后在最终目录完整分页并按最终文件名唯一取得 source `fileId/pickCode/SHA1/size/parentId`。Emby Size 只作为观察值，不参与解析或一致性判断，后续统一使用 Provider 返回的正数 Size。
3. 使用 playback Cookie 按 source `SHA1 + size` 执行 `SearchBySHA1`：
   - 已命中：复核非目录和 size 后直接进入第 9 步；这是 playback 预存文件，不属于 Ember 秒传任务，禁止后续自动删除。
   - 未命中：继续创建或复用传输任务。
4. 以 `playbackAccountId + SHA1 + size` 获取数据库唯一任务和 advisory lock；拿锁后必须再次查询 playback，避免 `HEAD`、预加载、Range 或重连重复秒传。
5. 锁内仍未命中时，先独立通过转存许可与配额预留，并将本请求任务推进到 `Initializing`；随后按 9.2 取得源前缀 preID，再使用 playback Cookie 调用 `InitRapidUpload`。请求的 `TargetParentID` 必须等于该 playback 账号配置的 `targetParentId`：
   - `status=1`：明确失败，禁止下载并上传完整视频。
   - 初始化结果状态为 `provider_rejected`：结束当前任务，以 `rapid_upload_unavailable` fallback，不据此写账号 `provider_protocol`。这与调用直接返回 `ErrProviderRejected` 是两种输入；后者仍按 §10.2 的操作分类处理。凭证、传输与协议错误也按该表判断账号影响，禁止改用 source 账号播放。
   - `status=7`：校验 challenge Range，使用 source Cookie 调用 `HashFileRange`；把返回 SHA1 作为 `sign_val`，连同原 `sign_key` 再次调用 playback `InitRapidUpload`。首期只接受一次 challenge；第二次仍非 `status=2` 时失败关闭，除非后续真实合同证明可以安全继续。
   - `status=2`：只表示初始化接受复用，不能直接视为目标文件存在。
6. 收到 `status=2` 后，使用 playback Cookie 和精确 `targetParentId` 调用 `FindTargetFile`；只有唯一候选同时满足 parent、SHA1、size 和非目录才算成功。
7. 只有“锁内再次查重未命中 → 本任务初始化返回 `status=2` → 目标目录复核成功”的文件，才能记录为该任务创建的 playback 文件；任务保存 playback account、target parent、fileId、pickCode、SHA1、size、完成时间和 `lastAccessedAt`。这些 provenance 第一阶段用于复用、审计和外部删除恢复，第二阶段才能作为容量回收依据。
8. 目标复核失败、超时或出现多个精确候选时，任务失败且不得签发任何下载地址；不确定归属的文件不得自动删除。
9. 按第 9 节同设备缓存规则复用地址，未命中时使用 playback Cookie、目标文件 pickCode 和本次真实播放器 User-Agent 调用 `GetDownloadURL`，校验 HTTPS hostname allowlist、`ExpiresAt`、并发上限和 HeaderMode；`f=3` 首期拒绝。
10. 播放网关只把第 9 步得到的 playback 下载 URL 作为 `Location` 返回 `302`。source 下载 URL 仅允许在服务端执行 preID/challenge Range，永远不能进入客户端、API、日志、数据库或缓存序列化。
11. `Playing/Progress/Stopped` 继续透明转发给 Emby；成功事件才更新已有 Redis sessionFingerprint 租约。GET 创建或复用 reservation，HEAD 只读既有租约；Playing/Progress 晋级或续租，暂停继续占用，Stopped 成功转发后释放。Stopped 不清除同设备直链缓存，新 session 必须重新准入。每次成功复用 playback 文件时仍按既有 1 分钟采样规则更新任务的 `lastAccessedAt`。
12. 第一阶段不自动删除 playback 文件：会话停止、过期或用户短期重复打开都只影响会话状态，不触发 `DeleteFile`。文件持续保留在专用目录并作为后续播放缓存；如果管理员在 115 中手工删除，下一次播放必须以实时查重未命中为准重新秒传，不能只信任历史成功任务。

首期不允许以下捷径：使用 source 账号直链播放、把 source Cookie 注入播放器、完整文件中转上传、未复核目标文件就返回 302、播放停止/会话过期后自动删除 playback 文件。Provider/DirectPlay Service 只返回类型化结果，不自行代理 Emby；Gateway 在 Principal 合法且 115 非成功时记录固定原因并显式 fallback 原始 Emby 请求。

## 9. 302 与直链安全

- 获取最终直链时使用真实客户端 User-Agent；直链缓存至少按 `accountId + pickCode + UA` 隔离。
- 只有实测证明存在 IP 绑定后，才把客户端 IP 纳入缓存键；不得凭经验增加。
- 缓存有效期不得超过链接参数 `t`，并预留安全窗口。
- `Location` 必须匹配配置的 115/CDN 域名 allowlist，禁止开放重定向。Gateway 最终 302 与直链缓存命中复用 `IsAllowedDownloadHost` 同一边界，不放宽 `*.115cdn.net`。
- 源账号用于 Range challenge 的直链永远不能返回给客户端。
- `f=3` 或其他需要额外 Cookie 的链接，在证明 Infuse 能自然满足前按不兼容处理。
- 不兼容、凭证失效或 Provider 不可用时向 Gateway 返回类型化内部错误，禁止改用 source 账号；Gateway 不签发 302，记录脱敏 fallback 原因并透明转发原始 Emby 视频请求。

### 9.1 同设备短期直链缓存（Ember 内部合同）

- Gateway 使用两层同设备进程内缓存：跨播放会话的媒体解析结果缓存位于源路径解析与目标查重之前，命中后跳过这两类 Provider 调用并引用仍有效的最终直链缓存；未命中时执行完整内容校验。账号、套餐及 Redis 租约检查仍实时执行。没有设备/登录上下文的低层 `Resolve` 不使用本节媒体/直链两层缓存；9.2 的源 preID 成功内容哈希是独立复用层，不包含用户或 playback 授权。
- 媒体键对 Server、用户、登录映射、设备、原始媒体路径、真实播放器 UA、源和 playback 账号 ID/Provider 身份/干净 active 配置版本/精确凭证/Provider 参数、源挂载前缀/root 和目标目录做长度编码摘要；不含 PlaySessionId，也不按会话过期或 Stopped 清除。媒体条目仅保存源内容 SHA1/size、原播放会话身份和直链缓存引用，最多 1024 条，容量满时淘汰最早到期项。
- 跨会话媒体新鲜度截止于 `min(本次文件解析开始时间 + 10m, 引用直链缓存的原始截止时间)`；条目最多保留至引用直链原期限。超过新鲜度后，仅实时租约准入确认 active/paused、且 PlaySessionId 与该条目原会话一致时允许继续复用；其他会话、reservation、过期或停止会话必须重新验证文件身份。直链过期或被淘汰同样视为未命中；时钟回退丢弃条目。持续播放允许保留当前已验证文件身份至链接安全截止时间，跨会话仍最多延迟 10 分钟发现外部同路径替换/删除。不得用媒体重新填充延长原直链期限；媒体内容身份只在进程内使用，不输出日志或落库。
- 缓存键对 Server、用户、登录映射、设备、playback 账号 ID/干净 active 配置版本、凭证及 Provider 参数、目标目录/文件 ID/pickCode/SHA1/size、真实播放器 UA 做长度编码后摘要；不保存原始 Cookie，不含 `PlaySessionId`，不跨设备或登录复用。摘要只在进程内使用，不输出日志。
- 进程内 LRU 最多 1024 条，每条 URL 最多 16 KiB；缓存截至 `ExpiresAt - min(5m, 剩余寿命 10%)`，不再附加获取后 10 分钟上限，也不再用固定 10s 窗口把短寿命链接整段禁用。命中不续期，过期惰性删除并按容量淘汰，无后台刷新。写入与命中都复用既有 115 hostname allowlist，不放宽。时钟回退到写入之前时丢弃条目。URL 不进入 Redis、数据库、API 或日志。
- 同一键的取链串行，等待者可独立取消；持有者取消或调用失败不填缓存，后续请求可重新获取。不缓存错误、额外 Cookie 链接、无效结果或已进入安全窗口的地址，不以过期地址兜底。
- 每个请求独立执行 Redis 准入和最终确认；HEAD 无租约仍 fallback，缓存不能绕过停用、冷却、解绑或配额状态。相同用户/登录/设备/媒体路径/UA 的昂贵处理跨 session 串行，获锁后加载账号快照，等待者独立取消。只有完整成功且最终租约确认通过后才发布媒体条目；新转存强制清除旧直链并重新获取。媒体命中返回 `preexisting=true`，不复制旧任务 ID、首次创建标记或转存用量；仍调用既有成功访问采样，不再次消耗转存额度。
- 账号凭证加载暴露非持久化 `DownloadCacheVersion`：仅干净 active 状态为正数；任一账号冷却探测/错误恢复或未知版本为零时绕过媒体缓存，执行真实文件查询。playback 探测同时绕过直链缓存并实际取链。媒体命中不回写任一账号健康成功；只命中直链缓存时，真实源解析成功仍按原规则采样。
- 决策日志增加固定 `mediaResolutionCache=hit|miss|bypass`，保留 `downloadURLCache=hit|miss|bypass`。媒体命中不产生 `sourceResolveCalls`、`targetSearchCalls` 或 `downloadURLCalls`；不打印 URL、SHA1 或缓存键。
- miss/bypass 只输出本次查询可证实的固定原因，hit 不输出原因；诊断不改变缓存 key、容量、TTL、淘汰顺序或准入。原因枚举见下表，`entry_missing` 与 `dependent_url_missing` 不证明是配置变化、容量淘汰或外部文件变化。
- `mediaCacheEvictions` / `downloadCacheEvictions` 仅在本请求实际触及对应 cache 时输出，是请求收尾时在缓存锁内取得的进程累计容量淘汰快照。过期删除、时钟回退删除、显式移除和强制刷新不计为容量淘汰；数值缺失不等于零。不同请求的快照不得求和，也不能据缺失条目反推淘汰因果；离线摘要只给 `maxObserved`，`intervalDelta` 保持 `unknown`。
- 自动化只证明 Ember 复用、隔离与门控。Gateway 无法观察 302 后 CDN 提前失效，跨播放 session 的真实客户端复用、CDN 并发/网络切换兼容性和实际延迟收益尚未实测。

| 字段 | 固定原因 |
| --- | --- |
| `mediaResolutionCacheReason` | `entry_missing`、`expired`、`clock_rollback`、`freshness_expired`、`dependent_url_missing`、`dependent_url_expired`、`dependent_url_clock_rollback`、`dependent_url_host_rejected`、`key_unavailable`、`cache_unavailable` |
| `downloadURLCacheReason` | `entry_missing`、`expired`、`clock_rollback`、`host_rejected`、`key_unavailable`、`cache_unavailable`、`forced_refresh` |

`freshness_expired` 指其他会话已越过媒体新鲜度窗口；`dependent_url_*` 仅描述媒体引用的直链在本次读取时的失效。`forced_refresh` 是新转存走既有强制重新取链分支，`key_unavailable/cache_unavailable` 表示无法使用缓存，不猜测更细的配置或身份根因。

### 9.2 preID 成功结果复用（Ember 内部合同）

2026-10-25 用户授权在采集与缓存诊断之后优化重复 preID 读取；实现与本地验证记录见[主计划第 8 节](../plan/architecture/gateway-client-compatibility.md#8-采集缓存诊断与-preid-复用2026-10-25)。本层不改变 115 请求/响应协议，也不据上游持久缓存实现扩大凭据或转存权限。

- 缓存位于 DirectPlay，仅在目标第二次查重未命中、转存许可、配额预留及任务 `Initializing` 之后使用；保存经过 `validateRangeHash` 校验的前至多 128 KiB SHA1 和字节数。challenge 仍实际读取，不命中本缓存。
- key 使用长度编码摘要绑定 source 账号 ID、Provider UID、正 `DownloadCacheVersion`、精确凭据及 Provider 参数、文件 ID/pickCode/完整 SHA1/Size 和范围；不绑定 playback、用户、设备、会话或播放器 UA，source UA 仍属于 key。当前调用必须先取得有效源文件身份。未知配置版本、恢复探测或不完整身份直接 bypass，不读取、合并或填充缓存。
- 进程内 LRU 最多 1024 条，成功读取后固定 10 分钟非滑动 TTL；命中只改变容量淘汰顺序，不延长期限。过期惰性删除、时钟回退丢弃，无后台刷新和持久化。值不包含视频字节、URL、明文凭据、任务、配额或健康结果；摘要和哈希不进入日志。9.1 中低层 `Resolve` 的缓存限制只指媒体/直链两层，不用于取消本层独立的已验证内容结果复用。
- 同键使用现有 requestGate 串行并在获锁后复查。owner 使用原请求上下文，失败或取消不发布，下一个等待者可重试；成功 hash/字节数校验且调用未取消后即可发布并释放 gate，不等待无关目标上传完成。目标后续失败不撤销已证实的内容哈希。
- 账号、源路径、许可、租约、任务、配额、目标上传及复核仍由每个请求独立执行。缓存不伪造 preID 网络/账号恢复成功；原来真实源路径观察对应的健康回写保持。HEAD 和无新增许可请求不能借缓存跳过门控。
- `measureStage(preID)` 只包围实际 HashFileRange；hit 不增加 `preIDCalls`，gate 等待单独记 `preIDWaitMs/preIDWaitCalls` 与 Debug `step=preIDWait`。缓存只输出固定 `preIDCache=hit|miss|bypass`，不增加 preID 原因/容量计数字段，也不复制另一请求的 Provider 事件。
- 验收先对两个用户、两个不同 playback 账号及同 source 文件测基线，再确认 preID 读取从两次降到一次，目标写入、独立任务与配额工作量不变。fake 计数不等于真实命中率或客户端延迟收益。

`preid_reuse_test.go` 已在两个用户、不同个人 playback 目标和同一 source 文件的顺序、重叠请求中取得以下等工作量 fake 对照；只证明该固定场景减少一次实际前缀读取：

| 观测 | 无 preID 复用基线 | 启用复用 |
| --- | --- | --- |
| source 解析 / 目标查重 | 2 / 4 | 2 / 4 |
| preID `HashFileRange` | 2 | 1 |
| 上传 / 目标复核 / 最终取链 | 2 / 2 / 2 | 2 / 2 / 2 |
| 独立任务 / 每位用户转存额度 | 2 / 各 1 | 2 / 各 1 |

身份、并发、失败/取消与恢复探测回归，以及全量非数据库 Go 测试、DirectPlay/账号/配额/115 Adapter/Gateway 五包 race、vet/build 和独立审查均已完成。采集工具 39 项离线测试和 Go 日志→Python 摘要串接复验同样通过，具体命令见[测试指南](../runbooks/testing.md)。未部署，未访问真实 Emby、115 或数据库；上述计数不能推导真实缓存命中率、首帧或起播时延收益。本轮不增加图片/字幕缓存、预热、负缓存、连接池调参、Web UI、配置或 migration。

## 10. 保留、冷却与未来清理

- 第一阶段 playback 专用目录是持久缓存：秒传文件默认保留，direct play Service、Redis 租约 TTL 和受控写入检查器均不得调用 `DeleteFile`。
- 重复播放先在精确 `targetParentId` 下按 SHA1 + size 查重；命中后直接刷新下载 URL并更新 `lastAccessedAt`，不创建新传输任务。
- 成功任务必须保留完整 provenance；管理员手工删除 playback 文件后，短期媒体缓存到期后的实际查重未命中应允许重新秒传，历史 `succeeded` 不能永久阻止恢复。
- 第一阶段不承诺自动控制 playback 容量，管理员通过专用目录观察占用并手工处理；手工处理不属于 Ember 自动状态流转。
- 第二阶段才设计自动回收。候选至少同时满足：无活跃会话、超过基于 `lastAccessedAt` 的最短保留期、容量策略要求回收、任务 provenance 完整、删除前 parent/fileId/SHA1/size 全量复核一致。
- 第二阶段清理任务按账号串行执行，并补跨副本任务所有权/互斥；不能把 Adapter 进程内锁当成全局锁。
- DirectPlay 当前将 `succeeded/credential_rejected/provider_unavailable/provider_rate_limited/provider_risk_control/provider_protocol` 六种固定账号级结果回写到实际调用的 source/playback 账号；Provider 原始错误和响应体不进入账号 Service。
- `provider_unavailable` 当前按账号进入默认 1 分钟共享冷却，不让每个播放请求独立重试；冷却未到期时不读取 Cookie，到期后用 PostgreSQL 行锁另延长 1 分钟半开探测租约并只放行一个探测，成功立即恢复 `active`，失败按错误类型重新冷却。
- `provider_rate_limited` 默认冷却 15 秒，`provider_risk_control` 默认冷却 30 秒，均保留启用意图，不进入 `expired + disabled`。冷却时长可通过 `P115_RATE_LIMIT_COOLDOWN` / `P115_RISK_CONTROL_COOLDOWN` 覆盖；非正值保留默认。风控哨兵与冷却配置已保留，但真实 115 结构化风控错误码尚未实机确认，当前没有 HTTP 触发源，待真实证据接入。
- Cookie/Web 数据面按账号做本地 FIFO 发牌限流，覆盖源列表、SHA1 查重、downurl、秒传和删除；`Burst`/`MaxWait` 必须为正，`MaxWait=0` 不表示零等待或无限排队。业务上限为 `RPS<=100`、`Burst<=1000`、`MaxWait<=1m`。`RPS=0` 关闭等待；`Inf`/`NaN`/小于 `0.01`、大于 `100`、负数与负零（`-0`/`-0.0`）以及超量 `Burst`/`MaxWait` 保留默认并写日志。115 API `429` 使用 `ErrProviderRateLimited` 并短暂冷却：HTTP 429 是 RFC 6585 / RFC 9110 的标准 Too Many Requests 语义，不是未确认的 115 专有业务码。本地等待超过 `MaxWait` 使用 `ErrDataPlaneQueueLimited`：入队时比较理论等待，入队后还监听真实截止时间，令牌计时器因暂停或锁竞争晚触发时到期安全出队；与发牌同时到期则使用已授令牌，未授牌则归还。不回写账号健康、不进入共享冷却，该请求仍回退 Emby。裸 API `403` 使用 `ErrProviderUnavailable`，沿用默认 1 分钟不可用冷却，不据此推断账号风控。`ErrProviderRiskControl` 只在已验证的结构化风控错误码出现时映射，当前无触发源。取消是从等待队列移除，不预分配或回拨未来时隙。CDN Range 读取不取令牌，也不套用该 HTTP 映射。对外仍按 Provider 不可用回退 Emby，不改变播放器可见行为。
- `credential_rejected` 进入 `expired + disabled`，账号级 `provider_protocol` 回写进入 `error`；请求取消、大目录解析超时、本地数据面排队拒绝、目标文件不可见、下载 Header 不兼容等文件/请求级失败不污染账号健康。
- 运行期回写同时匹配请求加载时的 Cookie 密文、`config_version` 和 `updated_at`；旧请求不能覆盖 Cookie 替换、显式验证、手工启停或较新的运行期结果。健康回写失败不改变原始 302/fallback 结果。
- Cookie 失效、账号风控、下载地址不兼容、普通上传要求、本地数据面排队拒绝和 Provider 限流必须使用不同内部错误码。
- 负缓存只能短期存在；一次搜索失败不能长期证明文件不存在。
- 已签发的 CDN 链接无法由 Ember 保证立即失效，封禁和账号失效只能阻止新直链。

### 10.1 数据面限流桶生命周期

生产 Cookie Adapter 共用进程内 Provider UID 限流器，环境配置在首次构造时读取；FIFO、令牌发放、`MaxWait` 和本地排队拒绝语义保持。无等待者且令牌自然回满时，回调回收 UID 桶，不以提前删除桶来刷新突发额度。计时回调同时校验桶身份和代次，旧回调不能触及重绑后的新桶。

个人账号解绑及删除用户都走 revoked tombstone：事务返回被锁定记录的旧 Provider UID，只有提交成功后才通知本进程限流器。事务失败或账号已不存在不清理；未回满或仍有等待者时保留原余额与队列，随后自然回收。API 与 Gateway 仍是独立进程，Gateway 依靠相同空闲回收处理残留，不新增跨进程通知、共享限流存储或 Redis 租约删除。

### 10.2 运行期失败分类

以下只描述 DirectPlay 运行期；显式账号验证继续使用 §5.1 的状态机。Gateway 的 `reasonCode` 与账号健康回写是不同结果，日志出现 `provider_protocol` 并不证明账号已进入 `error`。

| 输入或失败边界 | DirectPlay / Gateway 结果 | 账号健康影响 |
| --- | --- | --- |
| `ErrCredentialRejected`，包括 downurl 已识别的 `99/990001` | `account_unavailable` | `expired + disabled`；errno 的公开证据仍未经真实账号证实 |
| `ErrProviderUnavailable`，包括裸 HTTP `403`、downurl/通用 GET 的非 context 响应体读取失败 | `provider_unavailable` | 默认 1 分钟 `cooling_down` |
| `ErrProviderRateLimited`，包括 HTTP `429` | `provider_unavailable` | 默认 15 秒限流冷却，可配置 |
| `ErrProviderRiskControl` | `provider_unavailable` | 默认 30 秒风控冷却，可配置；当前无已验证的 HTTP 触发源 |
| `ErrFileUnavailable`、`ErrDownloadURLExpired`，或候选 URL 空/过期/并发限制为负 | `file_unavailable` | 不回写账号健康 |
| 秒传返回 `RapidUploadProviderRejected` / 普通上传要求；目标复核返回不匹配文件 | `rapid_upload_unavailable` / `target_unavailable` | 仅失败当前任务，不写账号 `provider_protocol` |
| Service 发现 challenge 无效/重复，或目标查重候选重复/身份不符 | `provider_protocol` | 仅当前文件校验失败，不写账号 `provider_protocol` |
| Provider 返回 `ErrProviderRejected` 或 `ErrProviderProtocol` | `provider_protocol` | 源路径解析、源 preID/challenge 不回写；其他 playback 操作保留既有 `error` 分类 |
| 本地数据面排队超限、源目录/片段阶段预算耗尽、目标不可见/歧义、下载 Header 或 URL 安全策略拒绝 | 对应本地 fallback 分类 | 不回写账号健康，不因当前请求失败推断账号故障 |
| 调用方取消或 deadline | 保留 context 错误，Gateway 按适用边界终止为 `499/504` | 不回写故障 |

未知 downurl errno 继续返回 `ErrProviderRejected`，按上表的具体操作分类处理；不把它扩展为凭证失效、限流或风控。已完成的真实 source 路径观察可独立报告成功；playback 只有实际下载接口成功才构成恢复证据，缓存命中不伪造健康成功。半开租约、协议错误恢复条件与阈值保持既有合同。

## 11. 自动化测试合同

所有测试必须 mock 115，禁止真实外部请求。至少覆盖：

1. Cookie 加密落库、替换和 API 永不回显明文。
2. 登录状态端点 method、query、Cookie/User-Agent Header、`state` 正常/失效/非法响应和 UID 规范化；Cookie 客户端识别覆盖完整已知 `ssoent` 映射、`A1 → web`、未知编码人工兜底，以及创建/替换同步刷新 `app_type`。
3. 上传信息端点 method、无 query、Cookie/User-Agent Header、UID 一致性、必需字段和业务拒绝映射。
4. 源文件解析覆盖固定 `/files/get_path_id` method/query、`is_create=0`、顶层/嵌套目录 ID、缺失/零值/冲突 ID、根目录文件跳过路径接口，以及最终 `/files` 完整分页、无效 cid 回退、同名文件歧义、目录规模上限和非法相对路径。DirectPlay 另覆盖源解析失败不改变账号健康、释放新播放 reservation、随后正常路径可继续解析，以及 Gateway 保留源解析诊断并回退 Emby。
5. playback 目录解析覆盖可选前导 `/`、多层目录、文件同名过滤、最终文件拒绝、同名目录歧义、分页、cid 回退和非法路径。
6. SHA1 查重覆盖无 parent 的全局 `shasearch` 与有 parent 的目录作用域 `/files/search`，并覆盖 Web 短字段/app2 长字段命中、固定未命中、size/目录/parent 不匹配、多个精确候选和非法字段。
7. 目标目录复核覆盖立即可见、延迟可见、最终截止查询、超时、取消、多精确候选和 Provider 错误不重试。
8. 验证结果与 Cookie 版本绑定，过期、协议错误、网络错误和成功状态流转正确。
9. 源账号与播放账号角色唯一性，以及相同 Provider 账号拒绝启用。
10. `status=7` 的正常范围、越界、格式错误和 Range 获取失败。
11. Range Hash 覆盖默认账号 UA、显式 playback 测试 UA、`f=0/1/3` Header、精确 `206/Content-Range/Content-Length`、压缩、短读、长读、传输失败和 `1 MiB` 上限。
12. `status=1` 明确拒绝，且不触发完整文件上传。
13. 加密请求与解密响应固定向量；有效完整 AES blocks 后的 1 至 15 字节短尾部被忽略，不足一个完整 block 的响应仍拒绝；LZ4 剩余 1–2 字节和零长度终止头按固定实现结束，正数截断 block 仍拒绝。
14. 并发 `HEAD`、预加载和 Range 只创建一个秒传任务；PostgreSQL 独立 schema 集成测试必须证明相同内容的两个并发 Resolve 只调用一次 fake `InitRapidUpload`。
15. 下载链接覆盖真实客户端 UA、RSA request/response seam、单记录/pickCode 校验、HTTPS allowlist、唯一 `t/c/f`、过期、负数 `c` 和未知 Header 模式；覆盖 `url=false/null/空`、errno 数字/string/未知值、响应体读取中断与 context 取消、文件级健康隔离和 `file_unavailable` fallback；播放网关另测 `f=3` 拒绝。
16. 删除 Adapter 覆盖单文件表单、同 Provider UID 串行、跨 UID 并行、锁等待取消和错误不重试；第一阶段业务测试必须反向确认 Stopped、会话过期和重复播放都不会调用 `DeleteFile`。
17. 保留式秒传检查器覆盖双重查重、preID、零/一次 challenge、重复 challenge 拒绝、目标复核、preexisting 快速路径、playback UA Range、`retained=true`、`cleanup.attempted=false` 和 `databaseLockValidated=false`。
18. 重复播放命中同一 playback 文件时跳过秒传、按既有采样规则刷新 `lastAccessedAt`，并复用合格缓存或签发新临时直链；外部手工删除后查重未命中可以重新创建活动任务。
19. Cookie、完整直链、source 完整路径和 Provider 原始响应不进入日志或数据库；完整 SHA1、目标目录/fileId/pickCode 只进入 `playback_transfer_tasks` provenance，且不进入普通 JSON 或日志。
20. `playback_transfer_tasks` migration 可重复执行，活动内容 partial unique、终态 provenance、challenge `attemptCount=2`、失败脱敏和 `lastAccessedAt` 均由 PostgreSQL 集成测试锁定。

## 12. 受控真实验证

真实验证必须由用户明确授权，使用测试账号和测试文件，并以一次性命令执行；不能启动 Ember 服务或后台进程作为验证手段。每次记录日期、平台、Infuse 当前稳定版本、脱敏请求字段和结果。

仓库提供 `go -C services/api run ./cmd/p115-contract-check` 作为只读入口，具体环境变量、安全确认值、执行和清理步骤见 [115 Cookie Provider 一次性只读合同验证](../runbooks/p115-read-only-contract-check.md)。该命令使用不含 `InitRapidUpload` / `DeleteFile` 的窄接口，不连接 Ember 数据库；自动化测试只注入 fake Provider，禁止在测试中调用真实 115。失败 `code` 与写入检查器共用 `providerContractErrorCode`：HTTP 429 是 `provider_rate_limited`，本地排队是 `data_plane_queue_limited`，目录预算耗尽是 `source_directory_timeout`，保留风控哨兵是 `provider_risk_control`，不得落成 `unknown`。URL 安全策略失败时允许额外输出类型化的 reason、scheme 和 hostname，用于根据真实证据修订 allowlist，仍禁止输出任何可复用 URL 部分。

受控写入验证使用独立命令 `go -C services/api run ./cmd/p115-transfer-contract-check` 和更强的显式确认值，按本合同 §8.1 执行“playback 单进程二次查重 → 秒传/单次 challenge → 目标复核 → playback downurl → playback 128 KiB Range → retained=true”。命令明确声明文件会保留在 playback 专用目录，报告 `cleanup.attempted=false`，且接口层不持有 `DeleteFile` 能力；具体步骤见 [115 playback 保留式秒传合同验证](../runbooks/p115-retained-transfer-contract-check.md)。该命令不连接 PostgreSQL，不能把二次查重结果表述为生产唯一约束/advisory lock 已验证。

首轮至少确认：

- 源账号和播放小号 Cookie 的有效性、账号标识与客户端类型约束。
- `uploadinfo`、`/files`、`shasearch` 和上传初始化的实际响应字段，以及大目录分页和无效 `cid` 行为。
- `status=2`、`status=7`、`sign_check` 和目标文件可见延迟。
- 源账号 Range 的实际 `206`、`Content-Range`、UA、Cookie 和单次验证字节数。
- 最终下载地址使用的实际端点，以及 `t`、`c`、`f`、UA、Cookie 和 IP 约束。
- 当前稳定 Infuse 在目标平台对 `HEAD`、Range、302 和下载 Header 的实际行为。
- playback 文件保留后的重复查重/复用行为，以及频率限制、风控和冷却边界。删除只保留为第二阶段独立验证项。

2026-08-22 本地只读验证最终结果为 `outcome=passed`：两个 Cookie 均可验证且 UID 不同，两个账号 `uploadinfo` UID 一致，source 相对路径与 10,747,391,752 字节 size 解析成功，playback SHA1 查询正常未命中；source downurl 为 `cdnfhnfile.115cdn.net`、`same_user_agent`、并发上限 `2`，过期时间正常；`bytes=0-131071` 精确读取 `131072` 字节并完成 SHA1。首次运行因该 hostname 未列入 allowlist 而失败关闭；DNS/TLS/证书组织核对后只加入精确 hostname，第二次运行通过。由于 playback 未命中，播放账号最终下载 URL 仍未验证；整个流程没有上传、移动、重命名或删除文件。

2026-08-22 本地保留式写入验证最终结果为 `outcome=passed`：两个账号再次验证且 UID 不同，playback 目录解析和写入前双重查重完成；源文件 preID 后首次初始化返回 `range_challenge`，读取一次有界 challenge 后重试为 `reused`，目标文件约 1,179ms 可见并通过 SHA1/size/parent 复核。playback downurl 为 `cdnfhnfile.115cdn.net`、`same_user_agent`、并发上限 `2`，`bytes=0-131071` 精确读取 `131072` 字节并完成与 source 相同的 SHA1 前缀校验。报告为 `writePerformed=true`、`created=true`、`retained=true`、`cleanup.attempted=false`；文件按合同保留。

同日用该保留文件完成 preexisting 复跑：目录作用域搜索直接命中，报告为 `outcome=passed`、`writePerformed=false`、`preexisting=true`、`created=false`、`retained=true`、`secondCheckPerformed=false`、`challengeCount=0` 和 `cleanup.attempted=false`；步骤中没有 preID、上传初始化、challenge、重试或目标轮询，只重新签发 playback downurl 并精确读取 128 KiB Range。两次运行均未连接 PostgreSQL，`databaseLockValidated=false`；播放网关和 Infuse 仍未实机确认。

生产编排的数据库合同由独立自动化验证补齐，不改变上述一次性命令的报告语义：`directplay.Service` 使用 `playbackAccountId + SHA1 + size` 的 PostgreSQL session advisory lock，拿锁后再次查重，并在 `playback_transfer_tasks` 记录终态。2026-08-22 专用集成数据库的三个独立 schema 已验证 migration/`VerifySchema` 与 migration 重入、两个并发 Resolve 只执行一次 fake 秒传、challenge 后 `attemptCount=2`，以及普通上传要求落为脱敏 `failed`。这些测试不访问真实 115；播放网关和 Infuse 仍未验证。

如果为验证专门创建了 source 测试文件，可在确认无其他用途后手工清理；使用既有只读 source 文件时不得为了收尾删除。第一阶段受控写入验证创建的 playback 文件按合同保留，用于重复播放、已有文件快速路径和后续 Infuse 验收；如需手工删除，由管理员在完成全部验证后自行处理，不属于检查器自动动作。临时 Cookie 应在验证后替换或撤销。尚未覆盖的行为必须继续标记“未实机确认”。

## 13. 演进边界

- Cookie Provider 与 OpenAPI Provider 可以并存，但每个账号必须显式选择认证模式。
- OpenAPI AppID 获批后，优先新增 OpenAPI Provider，不在 Cookie Provider 内混入 Token 分支。
- 是否迁移既有播放小号由管理员显式决定，不自动转换凭证或静默切换。
- 多账号池、普通用户自有账号和二维码 Cookie 登录必须分别重新设计并补计划，不能从首期单账号模型自然外推。
