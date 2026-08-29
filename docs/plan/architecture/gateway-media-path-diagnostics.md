# Gateway 媒体路径诊断日志实现方案

> 状态：已完成（待部署验收）
> 负责人：Ember
> 更新时间：2026-08-29

## 背景

- `item_container_snapshot_unusable reasonCode=response_invalid` 同时覆盖 JSON 解析失败、响应条目 ID 缺失和响应条目 ID 错配，现有持久日志无法定位实际原因。
- Playback Gateway 从 Emby `PlaybackInfoResponse.MediaSources[].Path` 取得媒体路径，并由 DirectPlay 将其从 `embyPathPrefix` 映射为 `sourceRootId + relativePath`，但当前日志只保留固定决策枚举。
- 真实排障无法确认 Emby 原始路径、目录前缀和映射后路径，尤其是最终返回 `302` 或 `path_not_mapped` 时缺少直接证据。

## 目标

本方案要实现：

1. 将条目 Container 快照的 `response_invalid` 拆分为稳定、可判断的失败原因。
2. 按用户明确授权，在持久日志中记录从 Emby 选中的完整媒体路径。
3. 在最终视频决策日志中记录原始媒体路径、配置前缀、source root 和映射后相对路径，使 `302`、fallback 与映射失败都可关联排查。
4. 保持 Token、Cookie、Authorization、115 下载 URL、完整外部响应体和 Provider 原始错误不进入日志。

## 非目标

本次明确不做：

- 不记录完整 `PlaybackInfoResponse` 或用户条目响应体。
- 不新增日志表、配置项、数据库模型或 SQL migration。
- 不改变 DirectPlay 的路径匹配、115 查重/秒传、302 或 Emby fallback 业务语义。
- 不启动 Gateway，不请求真实 Emby 或 115；真实运行验证由部署后受控复现完成。

## 当前事实

- 相关文档：`docs/system-architecture.md`、`docs/reference/emby-playback-proxy-contract.md`、`docs/reference/p115-playback-end-to-end-flow.md`。
- 相关代码：`internal/playbackgateway/item_snapshot.go`、`playback_info.go`、`playback_info_resolver.go`、`video.go` 与 `internal/services/directplay/service.go`。
- `PlaybackProof.Path` 保存通过合同校验的 `MediaSources[].Path`，视频请求命中证明后才会进入 `ResolveMediaPath`。
- `mapMediaPath` 以完整目录边界移除 `embyPathPrefix + "/"`，产生 `sourceRootId + relativePath`。
- 当前决策日志明确禁止媒体 Path；本次按用户授权修改该合同，但继续保留其他秘密和原始响应的禁止边界。

## 方案设计

### 1. 用户可见行为

- `item_container_snapshot_unusable` 使用独立 `reasonCode` 区分 JSON 非法、响应 ID 缺失和响应 ID 错配。
- 成功形成 PlaybackInfo 证明时，每个被接受的 MediaSource 记录 `mappingId/itemId/mediaSourceId/mediaPath`。
- 每条视频最终决策继续只记录一次；进入 DirectPlay 后增加 `mediaPath/embyPathPrefix/sourceRootId/mappedRelativePath`。
- 路径字段使用 quoted 日志格式，避免换行或控制字符破坏单行日志。

### 2. 数据与模型

> 本次不涉及数据模型变更。

DirectPlay 的 Gateway 返回结构增加仅用于进程内诊断的路径映射结果，不写数据库、不进入 JSON：

- Emby 原始媒体路径。
- 当前 source 账号 `embyPathPrefix`。
- 当前 source 账号 `sourceRootId`。
- 映射成功后的 `relativePath`。

即使 Provider 后续失败，也保留已经完成的映射结果供最终 fallback 决策日志使用。

### 3. 接口与边界

- 不修改任何 HTTP API、DTO、数据库合同或配置项。
- `playbackgateway.DirectPlayService` 的内部 Go 返回值调整为“重定向候选 + 路径映射诊断”，调用面只限 Gateway 与 fake 测试。
- 完整媒体 Path 成为明确允许的持久日志字段；Token、Cookie、Authorization、query value、115 URL、完整 SHA1、完整响应体和原始错误仍禁止记录。

### 4. 关键流程

1. Gateway 解析 Emby PlaybackInfo，校验 item/source/session/Path/Size/DirectPlay 能力。
2. 响应级合同成立后，每个唯一有效 MediaSource ID 都记录完整 `mediaPath`、字段存在性、播放能力和 proof 接受/拒绝原因，不依赖证明缓存写入成功。
3. 按需 PlaybackInfo 选中的路径进入最终视频决策；命中证明后再把 `PlaybackProof.Path` 交给 DirectPlay。
4. DirectPlay 返回原始路径、配置前缀、source root 和映射后相对路径；Provider 后续失败不抹掉已经形成的映射诊断。
5. Gateway 在唯一的最终视频决策日志中记录这些字段；成功 `302` 与 fallback 使用相同字段合同。

### 5. 失败路径与边界条件

- 条目响应不是合法 JSON：记录 `response_json_invalid`，仍原样返回上游响应。
- 条目响应缺少 `Id`：记录 `response_item_id_missing`，不写 Container 快照。
- 条目响应 `Id` 与 path 不一致：记录 `response_item_id_mismatch`，不写 Container 快照。
- Emby 路径非法或不匹配前缀：最终决策保持 `path_not_mapped`，同时记录已知的原始路径和配置映射字段。
- source/playback 账号或 Provider 后续失败：保留已经形成的路径映射诊断，正常 fallback 行为不变。
- 日志注入：所有路径使用 quoted 输出；测试覆盖换行和敏感载体不会进入日志。

## 影响范围

- API：有，仅修改 Gateway 与 DirectPlay 内部代码和测试。
- Web：无。
- Bot：无。
- 配置/部署：无；继续使用现有 `LOG_LEVEL`，本次路径字段随现有 Info 业务日志持久化。
- 文档：同步系统架构、Emby 播放代理合同和 115 端到端流程。

## 验证方式

### 编译/测试

- `cd services/api && go test -count=1 ./internal/playbackgateway ./internal/services/directplay`
- `cd services/api && go test -race -count=1 ./internal/playbackgateway ./internal/services/directplay ./internal/services/embytoken`
- `cd services/api && go test ./...`
- `cd services/api && go vet ./...`
- `cd services/api && go build ./...`
- `git diff --check`

### 手工验证

- 部署后打开一个媒体详情并播放，确认 PlaybackInfo 日志包含完整 `mediaPath`。
- 直连成功时确认 `decision=redirect statusCode=302` 同时包含原始路径和映射后路径。
- 路径前缀错误时确认 `decision=fallback reasonCode=path_not_mapped` 包含原始路径和已知配置字段。
- 手工日志验证必须由用户明确执行；本次实现不启动服务、不调用真实 Emby/115。

## 落地后文档处理

- 稳定日志合同已同步到 `docs/reference/emby-playback-proxy-contract.md` 与 `docs/reference/p115-playback-end-to-end-flow.md`。
- 架构事实已同步到 `docs/system-architecture.md`，相关进行中架构计划的旧 Path 禁止边界也已同步校正。
- 真实部署验收结果回填后，将本计划移入 `docs/archive/plan/architecture/`。

## 实施结果

- 条目快照已用 `response_json_invalid`、`response_item_id_missing`、`response_item_id_mismatch` 区分三类失败，并保持 Emby 响应透明。
- 普通与按需 PlaybackInfo 都会记录 `playback_info_media_source_observed`；`proofCount=0` 时仍输出选中 MediaSource 的 quoted `mediaPath`、Size/能力和固定 `proofRejectReason`。
- DirectPlay 候选携带不序列化、不持久化的 `PathMapping`；映射成功后即使 Provider 后续失败，也会把原始路径、配置前缀、source root 和相对路径交给最终视频决策日志。
- `decision=redirect|fallback|reject` 保持每请求一条；按需解析得到的 `mediaPath` 在 `playback_proof_missing` 时也保留，进入 DirectPlay 后再增加 `embyPathPrefix/sourceRootId/mappedRelativePath`。Token、Cookie、115 URL、完整响应体和原始错误继续由测试保护。
- 已通过目标包测试、目标包 race、API 全量 `go test ./...`、`go vet ./...`、`go build ./...` 与 `git diff --check`。
- 用户提供的 2026-08-29 Infuse `8.5.2` 日志已确认旧落点在 `proofCount=0` 时丢失路径，本轮已用对应 fake 回归测试修复。未由 AI 启动服务或请求真实 Emby/115；新 `proofRejectReason`、真实 `302` 和 `path_not_mapped` 日志形态仍待部署后受控复现确认。
