# SenPlayer 原文件播放兼容方案

> 状态：实现与本地验证完成，待部署后实机验收
> 负责人：Ember
> 更新时间：2026-10-25（补充后续兼容策略决策，现行实现不变）

2026-10-25 后续决策：用户确认在保留严格默认行为的基础上，新增管理员可开启的全局转存兼容模式，并接受开启后合格视频预读也可能产生首次转存。该扩展及图片、会话、续播问题统一由 [Gateway 客户端兼容与转存策略](./gateway-client-compatibility.md) 承接，当前尚未实施。本文件继续记录 2026-10-14 的受限原文件复用实现和当时确认的策略，不把新方案写成已生效行为。

## 背景与目标

SenPlayer `6.0.4` 实际播放 MKV 时使用缺失 `Static` 的 `original.mkv`。抓包时部署的 Gateway 只为同类 `original.mp4` 请求提供受限兼容，导致本轮 14 个视频请求全部在路由准入阶段回退 Emby，已有 115 目标也无法进入复用判断。本次工作区已完成 MKV 受限匹配，目标部署尚未更新。

目标是在当前身份、媒体证明和并发规则内兼容这类 MKV 请求，使已有目标和有效缓存能够用于 115 加速。用户已于 2026-10-14 明确选择保留“预读不新增转存”：缺少明确起播许可且目标不存在时继续回退 Emby。

非目标：新增视频 GET 起播许可、改写客户端的 PlaybackInfo 或 Emby 回退请求、支持任意 `original.*` / 转码参数、新增配置或数据库模型、承诺首帧改善。

## 当前事实与协议依据

- 目标版本沿用项目已确认的 Emby `4.9.3.0`，固定 SDK 为 `6ee0155063bc85578196489926359a8f37419502`。原生协议和本轮证据见 [播放代理合同 5.2](../../reference/emby-playback-proxy-contract.md#52-senplayer-originalmkv-抓包证据2026-10-14)。
- 固定 OpenAPI 仅对显式 `Static=true` 承诺无编码原文件播放，不声明省略该参数的任意流文件名都等价；POST PlaybackInfo 的起播字段属于 JSON `PlaybackInfoRequest`。
- 抓包时 Gateway 运行 revision 为 `f18c84e10f4e47010500900a777d8d8c18afa8f8`。本轮 3 次 PlaybackInfo 均得到合格 MKV 媒体证明，大小为 `27117028268` 字节，支持 DirectPlay 与 DirectStream；视频响应均为 `video/x-matroska`，支持 byte Range，ETag 在本轮内一致，响应总长度与媒体源 Size 一致。本轮实现与本地验证在 master 完成，不归属该旧部署 revision。
- 14 个视频 GET 均缺失 Static，身份 query 只有 MediaSourceId、PlaySessionId、DeviceId 和既有 Token aliases。Gateway 统一以 `route / request_not_eligible` 回退；200/206 和媒体字段支持本样本的原文件范围响应形态，不证明所有 MKV 或全文件字节相同。
- 3 次 PlaybackInfo query 均为 `IsPlayback=false`，JSON 无 IsPlayback，当前实现不授予新增转存许可。已有目标查重先于 `CanCreateTransfer` 门控；该许可只在内容锁内确认目标仍缺失后检查。

## 方案设计

### 1. 将格式作为受限原文件匹配的数据

将现有 MP4 专用识别与媒体证明匹配收敛为受限原文件匹配，首期格式集合仅为 `mp4`、`mkv`，不按 User-Agent 分流。

缺失 Static 的候选须同时满足：

1. GET/HEAD，固定视频路径，文件名精确为 `original.mp4` 或 `original.mkv`（大小写不敏感）；Static key 完全不存在。
2. MediaSourceId、PlaySessionId 唯一、非空且符合现有有界校验。query 只允许两者、唯一 DeviceId 和已通过身份门控的 Token aliases。
3. 当前 server/user/mapping/device/client/item/source/session 的未过期证明匹配；本地媒体，支持 DirectPlay 与 DirectStream，证明 Container、源路径扩展名与请求扩展名相同。DeviceId 出现时与证明相同。
4. 新增 MKV 例外首期要求媒体证明 Size 为正，限定在本轮已取得可对照媒体大小的证据范围。MP4 与显式 Static=true 的现行 Size=0 路径保持原规则；Size=0 MKV 缺失 Static 的扩展留待额外协议证据。

显式 Static=false、空值、重复逻辑 key，未知参数、Container、编码/码率/选流/起始时间参数，以及其他文件名继续走现行 Emby 语义，不进入这个例外。非法 Token 或用户状态仍按本地门控拒绝，不能以 fallback 绕过。

### 2. 原文件资格与新增转存许可保持分开

原文件匹配只允许进入既有 DirectPlay 决策，不授予或续期新增转存许可，不把 IsPlayback=false 改成 true，不使用 Range、Playing、客户端名称或详情访问作为许可。

| 场景 | 目标行为 |
| --- | --- |
| 合格 GET，当前选定播放账号存在匹配目标或有效缓存 | 通过既有账号、租约与并发门控后可返回 302 |
| 合格 GET，无匹配目标且无明确起播许可 | `playback_intent_required`，回退 Emby；不申请转存配额、不建转存任务、不读取 preID/challenge、不调用秒传 |
| HEAD，无同会话既有租约 | 沿既有 HEAD 门控回退；不创建 reservation 或转存 |
| HEAD，有同会话既有租约和目标 | 可复用候选；仍禁止新增转存 |
| proof 不匹配、媒体或 query 不合格 | 不调用 DirectPlay，保持原请求 Emby 回退 |
| 115 解析或复用失败 | 保持客户端原始 path/query/method/Range/Header 回退，不注入 Static |

用户策略及新增转存门控继续以 [起播许可方案](./p115-transfer-playback-intent.md) 为准。某次独立请求已经取得合法显式许可时仍按既有规则处理；本方案不增加第二种授予来源。

### 3. 实施范围与日志

- Gateway：受限原文件请求识别、proof 匹配、固定诊断和 mock 回归。使用请求容器数据，避免为每种扩展名复制编排分支。
- DirectPlay / proof / Redis：复用现有实现，新增组合回归锁定已有目标复用和无许可无写入，不改变状态与 TTL。
- 日志：受限匹配记录固定容器类别和 evidence，保留 requestId/sessionRef；MP4 已有诊断兼容保留，MKV 新增固定诊断。不得输出 Token、直链、原始响应体。
- 抓包工具：补充固定 original.mp4/original.mkv 路径和有界 SenPlayer UA 识别，保留固定原文件匹配诊断；未知路径/evidence 和鉴权字段仍不输出。离线复测不再把本轮视频请求误报为 0 个配对。
- 不涉及 Web、Bot、对外 API、SQL migration、配置或部署入口。系统架构、播放合同、客户端矩阵、抓包手册和本计划状态已同步。

## 验证方式与进度

- [x] 用户确认仅兼容已有目标，保留预读不新增转存。
- [x] 核对固定 4.9.3.0 SDK、当前代码及本轮脱敏抓包/媒体证明。
- [x] 现行 MP4 和起播许可专项 mock 回归通过：在 `services/api` 执行 `go test ./internal/playbackgateway -run 'TestGatewayOriginalMP4|TestGatewaySenPlayerOriginalMP4|Test.*TransferIntent' -count=1`。
- [x] TDD：新增 SenPlayer POST PlaybackInfo → MKV GET 回归，先复现旧逻辑在 route/request_not_eligible 回退；最小实现后已有目标调用复用，许可仍为 false，原 POST/响应字节及 fallback 保持。
- [x] MKV 文件名/容器/源路径/正数 Size、证明缺失/过期/错配、大小写/Token aliases、歧义/转码参数均有回归；新增 MP4 Size=0 与显式 Static=true 的 MKV Size=0 特征保护。
- [x] 分层合同覆盖：Gateway 经真实 proof 观察后以 fake DirectPlay 验证精确身份和许可回调；DirectPlay 实现用 fake Provider/Store 与内存 Redis 验证已有/锁内出现目标、跨会话缓存、无目标拒绝及 HEAD。转存配额未检查、任务/preID/challenge/秒传为零，拒绝释放新 reservation；既有停止/过期/并发用例一并通过。没有为测试导出业务内部构造器或引入真实数据库。
- [x] `services/api`：`go test ./... -skip Integration -count=1`、`go test -race ./internal/playbackgateway ./internal/services/directplay ./internal/services/p115quota -skip Integration -count=1 -timeout=90s`、`go vet ./...`、`go build ./...` 全部通过，外部依赖均为 mock/fake。
- [x] 抓包工具先复现原文件路径丢失与固定诊断被丢弃，再完成修复；`python3 -B -m unittest discover -s scripts/gateway-capture/tests -v` 17 项通过，含未知路径/evidence 和凭证脱敏保护。
- [x] 当前工具离线回放本轮原始 PCAP，得到 330 个 HTTP 事件、14 个视频配对、0 个新 TLS 连接；条目/Range/状态/响应头耗时与之前受限补充解析一致。该回放对应旧部署的 Emby 回退样本，不是新 Gateway 的实际 302 验收。
- [x] 自查触达链路：身份/证明 → 原文件匹配 → 账号/缓存/租约 → 锁内许可/配额/任务 → 代理回退及采集输出。P0/P1/P2/P3 本轮未发现需要修复的问题；现行文档已同步，链接与 diff 一致性检查完成。
- [ ] 经用户授权部署后，由操作者分别复验已有目标/缓存、缺失目标、详情浏览、拖动、暂停、停止和重播；确认 SenPlayer 接受 302，CDN Range 与字幕实际可用。当前抓包只证明 Emby 回退样本，真实 115 重定向与播放器效果未验收。

## 落地后文档处理

实现和自动化完成后，将正式适用条件提炼到播放代理合同、客户端矩阵和系统架构，并更新计划状态。真实客户端验收完成，或用户明确接受未验证边界后，归档到 `docs/archive/plan/architecture/`，同步计划、归档和盘点索引。原始 PCAP、源码下载和私有分析产物始终保留在仓库外，不作为 fixture 提交。
