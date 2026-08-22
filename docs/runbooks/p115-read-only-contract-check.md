# 115 Cookie Provider 一次性只读合同验证

本手册用于在用户明确授权后，以一次性 Go 命令验证真实 115 Cookie/Web API。命令不启动 Ember 服务、不连接 Ember 数据库、不进入 CI，也不包含上传初始化或删除能力。

## 安全边界

- 使用 `cmd/p115-contract-check`，只依赖 `ValidateCredential`、`GetUploadInfo`、`ResolveFileByPath`、`SearchBySHA1`、`GetDownloadURL` 和 `HashFileRange`。
- 禁止把真实 Cookie 放进命令参数、`.env`、脚本、文档、终端截图或聊天消息。
- 命令只从当前进程环境读取 Cookie；输出不包含 Provider UID、userkey、相对路径、pickCode、完整 SHA1 或完整签名 URL。
- Range 固定从源文件起点最多读取 `128 KiB`，不会读取完整文件。
- `CI` 非空或缺少精确确认值时命令直接退出，不发送 HTTP。
- 不使用 `tee`、shell 重定向或日志采集保存输出；需要留证时只人工记录脱敏结论。

## 前置条件

1. 两个不同的可用 Cookie：拥有测试文件的账号作为 `source`，另一个作为 `playback`。
2. 每个 Cookie 对应的原始客户端 User-Agent。浏览器 Cookie 应使用获取 Cookie 时的浏览器 User-Agent，不能随意编造。
3. 一个源账号已有文件的 `rootId`、slash 分隔相对路径和精确字节大小。
4. 文件大于 `128 KiB`，验证期间不移动或重命名。
5. 在未来网关相同的主机和出口网络运行，以便观察可能的 IP/UA 绑定。

如果没有真实 Infuse User-Agent，`P115_TEST_CLIENT_USER_AGENT` 可以暂时使用已知浏览器 User-Agent；此时结果只证明下载 URL 的 UA 绑定合同，不代表 Infuse 已验收。

## 自动化预检

以下命令只使用 fake Provider，不访问真实 115：

```bash
GOCACHE=/Users/konghang/.m2/ember-go-cache \
go -C services/api test -count=1 \
  ./cmd/p115-contract-check \
  ./internal/integrations/p115

GOCACHE=/Users/konghang/.m2/ember-go-cache \
go -C services/api build ./...
```

目标 Linux 主机可以省略 macOS 专用 `GOCACHE` 覆盖。

## 在 Linux 主机准备普通参数

以下值只是示例，必须替换为目标文件的实际数据：

```bash
export P115_SOURCE_ROOT_ID='0'
export P115_SOURCE_RELATIVE_PATH='relative/path/to/fixture.mkv'
export P115_SOURCE_SIZE='123456789'
```

如果文件来自挂载目录，先用 metadata-only 命令取得精确大小，禁止运行会读取完整视频的 `sha1sum`：

```bash
stat -c '%s' -- '/mounted/path/to/fixture.mkv'
```

## 在终端静默输入 Cookie

以下命令适用于 Bash。输入 Cookie 时终端不会回显：

```bash
IFS= read -r -s -p 'Source Cookie: ' P115_SOURCE_COOKIE
printf '\n'
IFS= read -r -s -p 'Playback Cookie: ' P115_PLAYBACK_COOKIE
printf '\n'

export P115_SOURCE_COOKIE
export P115_PLAYBACK_COOKIE
```

然后在本机终端设置两个 Cookie 对应的实际 User-Agent；不要把值发送到聊天或写入仓库：

```bash
IFS= read -r -p 'Source User-Agent: ' P115_SOURCE_USER_AGENT
IFS= read -r -p 'Playback User-Agent: ' P115_PLAYBACK_USER_AGENT
IFS= read -r -p 'Download test User-Agent: ' P115_TEST_CLIENT_USER_AGENT

export P115_SOURCE_USER_AGENT
export P115_PLAYBACK_USER_AGENT
export P115_TEST_CLIENT_USER_AGENT
```

最后设置必须完全匹配的真实调用确认值：

```bash
export P115_CONTRACT_CHECK_ACK='I_UNDERSTAND_READ_ONLY_REAL_115'
```

## 执行一次性检查

从仓库根目录执行，不启动任何服务：

```bash
go -C services/api run ./cmd/p115-contract-check
P115_CHECK_EXIT=$?
```

成功时 stdout 只包含脱敏 JSON。主要字段：

- `outcome=passed`：所有实际执行的只读操作完成。
- `accounts.distinct=true`：两个 Cookie 对应不同 Provider UID。
- `sourceFile.resolved=true`：路径、size 和源文件身份解析成功。
- `playback.found=false`：播放账号没有相同 SHA1 文件；这不是接口失败，但最终播放账号下载 URL 尚未验证。
- `playback.downloadValidated=true`：播放账号已有同一文件，并已验证其下载 URL 合同。
- `range.bytesRead=131072`：严格完成 128 KiB `206` Range；较小文件会读取完整文件大小，但仍受 Adapter 的 1 MiB 上限约束。

失败时 stderr 只输出固定格式：

```text
p115 contract check failed: stage=<stage> code=<code>
```

如果下载 URL 被安全策略拒绝，会额外输出一行不可复用的合同证据：

```text
p115 contract check evidence: reason=<reason> scheme=<scheme> host=<hostname-or-redacted>
```

该行只允许出现经过 ASCII/长度校验的 hostname；不会输出 URL path、query、签名、端口值、userinfo 或 IP literal。不能仅凭 hostname 自动放宽 allowlist，必须先确认域名所有权、固定源码证据和目标账号实测结果。

不要根据一次失败反复重试。记录 stage/code 后先检查合同、Cookie、UA、路径和目标账号状态。

截至 2026-08-22，真实 source downurl 已观察到 `cdnfhnfile.115cdn.net`。仓库只精确允许该 hostname；若再次出现其他 `115cdn.net` 子域，仍应停止并按同一证据流程核对，不能自行把配置扩展成通配后缀。

同日完成的本地只读验证结果：两个账号有效且不同，`uploadinfo` 身份匹配，10,747,391,752 字节 source 文件解析成功，playback 查重正常未命中；source URL 为 `same_user_agent`、并发上限 `2`，并以精确 `bytes=0-131071` 成功读取 `131072` 字节。该结果没有验证 playback 最终下载 URL，也不代表 `hz-sb` 出口或 Infuse 行为。

## 立即清理当前 shell

无论成功或失败，都先清理环境变量，再处理结果：

```bash
unset P115_SOURCE_COOKIE
unset P115_PLAYBACK_COOKIE
unset P115_SOURCE_USER_AGENT
unset P115_PLAYBACK_USER_AGENT
unset P115_TEST_CLIENT_USER_AGENT
unset P115_SOURCE_ROOT_ID
unset P115_SOURCE_RELATIVE_PATH
unset P115_SOURCE_SIZE
unset P115_CONTRACT_CHECK_ACK

printf 'contract check exit=%s\n' "$P115_CHECK_EXIT"
unset P115_CHECK_EXIT
```

如果使用临时 Cookie，验证后在 115 侧撤销或轮换。脱敏结果同步到实施计划时必须写明日期、代码提交、运行主机/出口、已验证项和仍未验证项，不能把一次成功描述为长期稳定性证明。
