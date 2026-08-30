# 115 源文件路径解析与 Emby Size 解耦实现方案

> 状态：已归档
> 负责人：Ember
> 更新时间：2026-08-30

## 背景

- Infuse `8.5.2` 的真实按需 PlaybackInfo 返回了完整媒体 `Path`、`SupportsDirectPlay=true`，但 `Size=0`；当前 Gateway 因 `size_invalid` 不生成 proof，尚未执行 source 账号路径映射就 fallback Emby 并得到上游 `404`。
- 当前 `PlaybackProof`、`MediaPathResolveRequest`、`FilePathQuery` 和 `CookieHTTPAdapter.ResolveFileByPath` 都把 Emby Size 当作必填匹配条件，导致 Emby 元数据缺失阻断本应由 115 Provider 完成的权威文件解析。
- 用户已明确确认：115 候选归属由 `Path` 是否命中 source 账号 `embyPathPrefix + "/"` 决定；源文件身份由 115 精确路径解析后返回的 Size/SHA1 决定。

## 目标

本方案要实现：

1. Emby `Size` 只保留为观察字段，不再影响 PlaybackProof、路径映射或 `ResolveFileByPath`。
2. `ResolveFileByPath` 只使用 `sourceRootId + 完整相对目录链 + 最终文件名` 唯一解析源文件。
3. 同一父目录没有同名文件时返回 not found，存在多个同名文件时返回 ambiguous，禁止任意选择。
4. 以 115 返回的正数 Size、合法 SHA1、fileId、pickCode 和 parentId 作为后续查重、秒传、Range、任务与 302 的权威文件身份。

## 非目标

本次明确不做：

- 不按全局文件名搜索 115，不跨 `sourceRootId` 或父目录猜测文件。
- 不因 Emby Size 与 115 Size 不一致拒绝路径解析；Emby Size 不再是校验提示。
- 不放宽目录边界、路径规范、分页稳定性、父目录、重名歧义、SHA1、Provider Size 或下载安全校验。
- 不修改 HTTP API、数据库模型、SQL migration、账号配置或前端。
- 实现与自动化阶段不启动 Gateway、不请求真实 Emby/115；部署后的真实 `302` 只接受用户提供的受控运行日志作为证据。

## 当前事实

- `PlaybackProof.Size` 当前必须为正数，否则返回 `size_invalid`。
- `directplay.Service.ResolveMediaPath` 当前在加载 source 账号和匹配 `embyPathPrefix` 前拒绝 `Size <= 0`。
- `p115.FilePathQuery` 当前携带 Size；Provider normalizer 要求其为正数，最终文件又按“名称 + Emby Size”筛选。
- Provider 的目录遍历已经逐级校验 root、parent、分页、目录名和最终文件名，并能对同名候选返回 `ErrSourceFileAmbiguous`。
- `resolveWithAccounts` 在路径解析后使用 Provider 返回的 `sourceFile.Size/SHA1` 构造目标查重、任务锁和秒传请求，具备接管权威文件身份的现成边界。

## 方案设计

### 1. 用户可见行为

- `Size=0` 或 PlaybackInfo 未提供 Size 时，只要 Path、会话、MediaSource 和 DirectPlay 能力合格，仍可生成 proof 并进入 115 候选判断。
- Path 命中 `embyPathPrefix` 且 115 唯一解析成功时，后续使用 Provider 文件 Size/SHA1；可能最终返回 `302`。
- Path 未映射、115 未找到、同名歧义或 Provider 文件身份无效时，保持现有 Emby fallback，不伪造成功。

### 2. 数据与模型

> 本次不涉及数据库模型变更。

- `PlaybackProof.Size` 可继续保存 Emby 观察值用于诊断，但 `0` 或缺失不再使 proof 失效。
- `MediaPathResolveRequest` 删除 Size；Gateway 只把经过授权观察的 Path 和客户端 User-Agent 交给 DirectPlay。
- `p115.FilePathQuery` 删除 Size，只保留 RootID 和 RelativePath。
- Provider 返回的 `File.Size` 必须为正数，并与 SHA1/fileId/pickCode/parentId 一起成为下游唯一文件身份。

### 3. 接口与边界

- 不修改外部 HTTP API。
- 修改的 Go 内部合同：`PlaybackProof` 校验、`MediaPathResolveRequest`、`ResolveRequest`、`FilePathQuery`、`ResolveFileByPath` 与源文件校验。
- `ResolveFileByPath` 仍逐级列目录；最终段只按 `entry.Name == segment && !entry.IsDirectory` 匹配，0/1/多候选分别映射为 not found/成功/ambiguous。

### 4. 关键流程

1. PlaybackInfo 校验身份、会话、Path、Container 和 `SupportsDirectPlay`；Size 只进入日志和可选内存观察。
2. 视频请求命中 proof 后加载活动 source/playback 账号。
3. 使用 `embyPathPrefix + "/"` 完整边界把 Emby Path 映射为相对路径。
4. Provider 从 sourceRootId 逐级解析目录和最终文件名，唯一命中后返回完整 115 File。
5. DirectPlay 校验 Provider File 的正数 Size、合法 SHA1、fileId、pickCode、parentId，再以这些值进入查重、锁、秒传、复核和直链。

### 5. 失败路径与边界条件

- Emby Size 为 `0` 或缺失：允许继续，不作为错误。
- Emby Size 为负数：作为异常观察值记录，但不参与 115 路由；Provider 文件身份仍必须合法。
- Path 不命中前缀：`path_not_mapped`。
- 最终父目录没有同名文件：`ErrSourceFileNotFound`。
- 最终父目录有多个同名文件：`ErrSourceFileAmbiguous`。
- Provider 唯一文件 Size 非正数、SHA1/fileId/pickCode/parentId 无效：Provider protocol failure，不进入 302。

## 影响范围

- API：有，涉及 Gateway、DirectPlay 和 P115 Provider 内部合同与测试。
- Web：无。
- Bot：无。
- 配置/部署：无。
- 数据库：无，无 migration。
- 文档：同步系统架构、Emby 播放代理合同、115 Cookie/端到端合同、客户端兼容证据和现有 115 计划。

## 验证方式

### 编译/测试

- `cd services/api && go test -count=1 ./internal/integrations/p115 ./internal/services/directplay ./internal/playbackgateway`
- `cd services/api && go test -race -count=1 ./internal/integrations/p115 ./internal/services/directplay ./internal/playbackgateway ./internal/services/embytoken`
- `cd services/api && go test ./...`
- `cd services/api && go vet ./...`
- `cd services/api && go build ./...`
- `git diff --check`

### 手工验证

- 部署后复现 `Size=0` 条目，确认 `proofAccepted=true` 且进入路径映射。
- 唯一源文件命中时确认最终决策显示 `embyPathPrefix/sourceRootId/mappedRelativePath`；115 完整成功时确认 `302`。
- 构造或选择同目录同名文件时确认失败关闭，不返回任意文件。

## 落地后文档处理

- 稳定语义已同步到现行 reference 和系统架构。
- `Size=0` 真实 115 结果已经回填；稳定事实由系统架构和现行 reference 接管，本计划仅保留历史追溯价值。

## 实施结果

- `PlaybackProof` 不再使用 Emby Size 判断有效性；Size 缺失、`0` 或负值均只保留为观察数据，Path/身份/会话/Container/DirectPlay 合同继续失败关闭。
- `MediaPathResolveRequest`、`ResolveRequest` 和 `p115.FilePathQuery` 已移除 Emby Size；Gateway 与 DirectPlay 只传递授权 Path 和客户端 User-Agent。
- `ResolveFileByPath` 已改为 `sourceRootId + 完整目录链 + 最终文件名` 唯一解析，不按 Size 过滤；同目录同名文件即使 Size 不同也返回 `ErrSourceFileAmbiguous`。
- 唯一命中后，DirectPlay 只接受 Provider 返回的正数 Size、合法 SHA1、fileId、pickCode 和 parentId，并以 Provider Size/SHA1 进入查重、锁、秒传、Range 与任务状态机。
- 一次性只读/保留式合同检查器仍保留终端显式 `P115_SOURCE_SIZE` 作为受控检查期望值，但已从 `FilePathQuery` 分离为 `ExpectedSourceSize`，不影响生产路径解析合同。
- 已通过目标包测试、目标包 race、API 全量 `go test ./...`、`go vet ./...`、`go build ./...` 和 `git diff --check`。
- AI 未启动服务或请求真实 Emby/115。用户提供的 2026-08-29 Infuse `8.5.2` 部署日志已确认同一 `Size=0` 条目得到 `proofAccepted=true/proofCount=1`，映射为 `sourceRootId=0 + video/.../龙门飞甲...mkv`，随后以 Provider Size `27932893135` 完成首次转存并返回 `302`；后续 `preexisting=true` 请求也多次返回 `302`。该证据证明 Gateway 直链响应，不证明客户端已从 115 CDN 读取完整媒体字节。
