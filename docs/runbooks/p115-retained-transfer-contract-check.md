# 115 playback 保留式秒传合同验证

本手册用于在用户单独明确授权后，把一个已验证的 source 文件秒传到 playback 专用目录，验证 target 复核、playback 下载 URL和 128 KiB Range，并保留目标文件供重复播放与后续 Infuse 验收。

## 安全边界

- 使用 `cmd/p115-transfer-contract-check`，接口只包含账号/上传信息读取、source 文件解析、playback 目录解析/查重、秒传、目标复核、下载 URL 和 Range Hash。
- 检查器的 Provider 接口不包含 `DeleteFile`；成功报告固定 `retained=true`、`cleanup.attempted=false`。
- 命令会在 playback 专用目录创建文件，并在成功或后续阶段失败时保留可能已创建的文件；不得指向根目录或正式媒体目录。
- 命令只从当前进程环境读取 Cookie、路径和 User-Agent，不接受包含凭证的命令行参数或 `.env`。
- 输出不包含 Cookie、UID、userkey、目录/文件 ID、路径、pickCode、完整 SHA1 或签名 URL。
- `CI` 非空或缺少精确确认值时，在任何 HTTP 前拒绝运行。
- 检查器是单进程合同验证：会在写入前执行两次 playback 查重，但不连接 PostgreSQL，不验证生产 `playback_transfer_tasks` 唯一约束或 advisory lock；报告固定 `databaseLockValidated=false`。

## 前置条件

1. 已通过 [115 Cookie Provider 一次性只读合同验证](./p115-read-only-contract-check.md)。
2. source 与 playback 是两个不同账号，Cookie 和对应 User-Agent 均可用。
3. source 文件有明确 `rootId + relativePath + size`，文件大于 `128 KiB`。
4. playback 已手工创建专用缓存目录，例如 `/EmberPlayback`；第一阶段文件会持续保留在该目录。
5. playback 空间足以容纳 source 文件对应的秒传记录。
6. 运行期间不在其他终端、服务或播放器中同时对同一 SHA1 发起秒传。

如果没有真实 Infuse User-Agent，可以暂时使用已知 UDown/浏览器 UA完成 Provider 合同；结果不能表述为 Infuse 已验收。

## 自动化预检

以下命令只使用 fake Provider，不访问真实 115：

```bash
GOCACHE=/Users/konghang/.m2/ember-go-cache \
go -C services/api test -count=1 \
  ./cmd/p115-transfer-contract-check \
  ./internal/integrations/p115

GOCACHE=/Users/konghang/.m2/ember-go-cache \
go -C services/api build ./...
```

目标 Linux 主机可以省略 macOS 专用 `GOCACHE` 覆盖。

## 准备普通参数

以下值只是结构示例：

```bash
export P115_SOURCE_ROOT_ID='0'
export P115_SOURCE_RELATIVE_PATH='relative/path/to/fixture.mkv'
export P115_SOURCE_SIZE='123456789'

export P115_PLAYBACK_ROOT_ID='0'
export P115_PLAYBACK_TARGET_PATH='/EmberPlayback'
```

目标路径由 `ResolveDirectoryByPath` 逐级解析为内部目录 ID；命令不会自动创建目录，也不会把路径或 ID写入报告。

## 静默输入凭证

Bash：

```bash
IFS= read -r -s -p 'Source Cookie: ' P115_SOURCE_COOKIE
printf '\n'
IFS= read -r -s -p 'Playback Cookie: ' P115_PLAYBACK_COOKIE
printf '\n'

export P115_SOURCE_COOKIE
export P115_PLAYBACK_COOKIE

IFS= read -r -p 'Source User-Agent: ' P115_SOURCE_USER_AGENT
IFS= read -r -p 'Playback User-Agent: ' P115_PLAYBACK_USER_AGENT
IFS= read -r -p 'Download test User-Agent: ' P115_TEST_CLIENT_USER_AGENT

export P115_SOURCE_USER_AGENT
export P115_PLAYBACK_USER_AGENT
export P115_TEST_CLIENT_USER_AGENT
```

设置必须完全匹配的保留式写入确认值：

```bash
export P115_TRANSFER_CONTRACT_CHECK_ACK='I_UNDERSTAND_PLAYBACK_FILE_WILL_BE_CREATED_AND_RETAINED'
```

## 执行一次

从仓库根目录执行，不启动任何服务：

```bash
go -C services/api run ./cmd/p115-transfer-contract-check
P115_TRANSFER_CHECK_EXIT=$?
```

成功报告关键字段：

- `writeCapable=true`
- `writePerformed=true/false`：只有本次实际秒传创建目标记录时为 true；preexisting 快速路径为 false。
- `targetDirectory.resolved=true`
- `transfer.preexisting=false/true`
- `transfer.created=true`：本次秒传进入 reused 并通过目标复核。
- `transfer.retained=true`
- `transfer.secondCheckPerformed=true`：写入前完成第二次查重。
- `transfer.databaseLockValidated=false`：本命令没有验证生产数据库锁。
- `transfer.challengeCount=0/1`
- `playbackRange.bytesRead=131072`
- `cleanup.attempted=false`

如果目标目录已存在相同文件，命令走 preexisting 快速路径，不调用上传初始化，仍验证 playback 下载 URL和 Range，也不删除该文件。

失败时只输出：

```text
p115 transfer contract check failed: stage=<stage> code=<code> fileMayExist=<true|false> cleanupAttempted=false
```

`fileMayExist=true` 表示 Provider 已返回 reused 或查重已命中，专用目录中可能保留目标文件。不要立即重跑；先在 playback 专用目录人工确认，并保留文件供后续快速路径验证。

下载 hostname 被策略拒绝时，会额外输出仅含 reason/scheme/hostname 的安全证据；不能据此自动开放整个域名后缀。

## 清理当前 shell，不清理网盘文件

无论成功或失败，都只清理环境变量：

```bash
unset P115_SOURCE_COOKIE
unset P115_PLAYBACK_COOKIE
unset P115_SOURCE_USER_AGENT
unset P115_PLAYBACK_USER_AGENT
unset P115_TEST_CLIENT_USER_AGENT
unset P115_SOURCE_ROOT_ID
unset P115_SOURCE_RELATIVE_PATH
unset P115_SOURCE_SIZE
unset P115_PLAYBACK_ROOT_ID
unset P115_PLAYBACK_TARGET_PATH
unset P115_TRANSFER_CONTRACT_CHECK_ACK

printf 'transfer contract check exit=%s\n' "$P115_TRANSFER_CHECK_EXIT"
unset P115_TRANSFER_CHECK_EXIT
```

不要为了检查器收尾删除 playback 文件。第一阶段以该文件继续验证再次查重命中、下载 URL刷新和 Infuse 重复播放；全部验证结束后如需删除，由管理员手工处理。
