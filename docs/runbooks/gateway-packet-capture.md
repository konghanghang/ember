# Gateway 抓包与播放排查

本手册用于在其他机器复用 Gateway 排查工具。入口为 `scripts/gateway-capture/gateway_capture.py`，代码和手册随仓库同步；每次采集的原始包、日志 JSON 和分析产物独立保存到仓库外。

## 1. 准备工具和 SSH

获取包含本工具的仓库版本，或整体复制 `scripts/gateway-capture/` 及本手册。必须保留目录内全部 Python 文件；不能只复制 CLI 入口。

| 位置 | 要求 |
| --- | --- |
| 本机（Mac / Linux） | Python 3.9+、OpenSSH；不需要本机 sudo、Infuse 自动控制或 Python 第三方包 |
| 最终 Gateway 主机 | Linux、Python 3、Docker CLI、`nsenter`，Gateway 容器已运行 |
| 最终主机权限 | 能读取 Docker 容器 PID/日志，能进入网络命名空间并打开 AF_PACKET 原始 socket；通常使用 root，或对相应命令配置非交互 sudo |
| SSH 各跳 | 已有公钥/主机密钥配置，`BatchMode=yes` 下可登录；工具不保存密码、不接受交互密码、不自动接受未知主机密钥 |

`--host` 是最终主机的 SSH 别名，`--via` 按“本机到远端”顺序重复填写。例如本机先到 `dmit`，再由 `dmit` 登录 `hz`：

```bash
python3 -B scripts/gateway-capture/gateway_capture.py capture \
  --host hz --via dmit \
  --container ember-gateway \
  --output "$HOME/ember-captures/example" --dry-run
```

此模式执行嵌套 SSH：本机需要 `dmit` 的配置，`dmit` 上需要 `hz` 的配置和登录能力；密钥可以留在各自那一跳。它不要求把 `hz` 的私钥复制回本机，也不自动启用 agent forwarding。多跳可写成 `--via jump-one --via jump-two --host gateway`；直连省略 `--via`。

`--dry-run` 只核对参数并打印配置，不检验真实 SSH、远端权限、接口或端口。新机器先由操作者完成正常 SSH 登录和主机密钥确认，再运行采集。

## 2. 采集并复现播放

以下命令从仓库根目录执行。输出目录每次使用新名称，位于仓库外；工具新建目录为 `0700`、文件为 `0600`，已有目录必须属于当前用户且无组/其他用户访问权限。

```bash
CAPTURE_DIR="$HOME/ember-captures/$(date +%Y%m%d-%H%M%S)"
python3 -B scripts/gateway-capture/gateway_capture.py capture \
  --host hz --via dmit \
  --container ember-gateway \
  --duration 240 --max-mib 128 \
  --output "$CAPTURE_DIR"
```

需要 sudo 时增加 `--sudo`，最终主机的 Docker inspect 和 nsenter 使用 `sudo -n`；权限不足会失败，不提示输入密码。工具在每次采集前读取已有容器的运行状态与 PID，避免复制旧 PID。PID 检查与采集之间若容器重启，应结束该次采集后重新运行，不自行重启服务。

终端输出 `captureReady=true` 后，由操作者在 Infuse 或其他播放器复现：

1. 选择硬盘条目，播放、拖动、暂停/继续、停止，核对 Emby 回退与 Range。
2. 选择尚未完成转存的网盘条目，首次播放约 20 秒、停止，再重播约 10 秒、停止。
3. 记录条目数字 ID 或片名、操作时刻、是否出画/卡住及大致等待时间。自动分析的 HTTP 响应头耗时不能代替首帧观察。

工具按时长或容量上限自动结束，输出 `captureFinished=true`、PCAP 路径和 metadata 路径。metadata 包含采集配置、源码 SHA256、起止 epoch、收包/丢包统计、结束原因及 SSH 退出码。达到容量上限会提前结束，需检查 `stopReason`，不能假定后续操作已经录入。

Ctrl+C 会终止本机 SSH 并保留已有材料；远端 helper 有时长上限，收到信号或输出管道断开时退出。断线后不能仅凭本机退出推断远端已立即结束；最长仍受所配置采集时长约束。工具不向 Gateway PID 发停止信号。

默认采集 Gateway 命名空间 `eth0` 上 TCP 8081、18096 和 443。部署不同，应按命名空间内实际地址/端口调整：

```bash
python3 -B scripts/gateway-capture/gateway_capture.py capture \
  --host gateway --interface eth0 \
  --gateway-port 8081 --emby-port 8096 --tls-port 443 \
  --duration 240 --max-mib 128 --snaplen 8192 \
  --output "$CAPTURE_DIR"
```

`--gateway-port` 是 Nginx→Gateway 这一段的容器内监听端口；`--emby-port` 是 Gateway 连接 Emby 时实际使用的端口，不一定等于宿主机发布端口。默认不记录 PostgreSQL/Redis 协议载荷。请求方向保留有界前缀以供播放事件取证，响应优先保留 HTTP 头，其他载荷仅采样；TLS 不解密。

如果需要 Debug 分段日志，由部署操作者在设置中心临时切换 `LOG_LEVEL`，结束后恢复 Info。工具不修改设置或自动刷新配置。

## 3. 提取同一时间窗口的日志

采集完成后，在同一终端中运行：

```bash
python3 -B scripts/gateway-capture/gateway_capture.py logs \
  --host hz --via dmit --container ember-gateway \
  --metadata "$CAPTURE_DIR"/*.metadata.json \
  --output "$CAPTURE_DIR"
```

需要远端 sudo 时，同样增加 `--sudo`。每个输出目录只放一轮采集，上述 glob 应只匹配一个 metadata；多轮混放时应显式指定文件。

也可省略 metadata，明确填写带时区的 `--since` 与 `--until`；二者不可与 metadata 同时使用。元数据 epoch 转为 UTC 是 PCAP/Docker 外部时间合同，不改变 Ember 的 `CRON_TIMEZONE`。日志默认 `--tail 10000`，最大 20000；窗口内日志超过 tail 时较早记录会缺失，不能据此声称某一步没有发生。

远端先按 code/字段白名单过滤，原始 Docker 日志不传回本机。输出包括白名单内的加速决策、源目录/取链/Range/秒传 POST 计时、PlaybackInfo 意图状态、租约及停止事件；密码、Cookie、Token、原始 path、媒体路径、完整响应和任意 message 不输出。未知 code 或字段会被忽略；日志定义增加后需同步过滤规则。

首次转存归因仅保留 `transferTrigger=explicit_playback|compatible_request|existing_target|policy_denied` 四种固定值，未知值丢弃。白名单同时接纳 `transfer_policy_unavailable`、`playback_info_revalidation_started`、`playback_info_revalidated` 和 `playback_info_revalidation_failed`，用于区分策略拒绝、已有目标复用和固定会话重新核验；继续丢弃原始错误类型/消息、凭据和 URL。

原文件匹配诊断保留固定 `video_original_mp4_accepted` / `video_original_mkv_accepted` 与 `evidence=local_mp4_proof|local_mkv_proof`；未知 evidence 丢弃。匹配诊断只证明受限路径通过媒体证明检查，不证明随后一定返回 302 或已经完成播放。当前日志工具的 `_CODES` 尚未包含 `playback_canceled`，这类新 Info 取消决策会被过滤；筛选结果缺行不能证明未取消。需要判断取消时，使用已授权取得的 Gateway 脱敏日志核对 `499`/取消 code，不放开未知 code 或原始日志透传。

## 4. 离线分析与对照

```bash
python3 -B scripts/gateway-capture/gateway_capture.py analyze \
  --pcap "$CAPTURE_DIR"/*.pcap \
  --logs "$CAPTURE_DIR"/gateway-logs-*.json \
  --output "$CAPTURE_DIR"
```

`--logs` 可省略。采集修改过端口时，分析也必须传入相同的 `--gateway-port/--emby-port/--tls-port`。`analyze` 只读取本机文件，无 SSH 或真实 HTTP 调用，可以在没有服务器访问权限的另一台机器执行。

生成 `<PCAP文件名>.analysis.json`，已有同名结果会拒绝覆盖；重跑请改用另一个私有输出目录。结果中：

| 字段 | 用途与边界 |
| --- | --- |
| `httpEvidence` | 连续 TCP 片段的 HTTP 元数据、Range、状态、过滤后的路由和一次分析内的会话引用；含不完整/非连续消息计数 |
| `packetVideoPairs` | 已观察到的 Gateway 视频请求→最终响应头配对，`packetHeadersMs` 是包端耗时，不是首帧 |
| `tlsConnections` | 新连接是否被观察到、TCP SYN 重复、SYN→SYN-ACK 耗时和完整 ClientHello 前缀中的 SNI；未解密应用数据 |
| `gatewayDiagnostics` | 可选的再次过滤后的应用诊断日志；不按时间相邻自动归属并发请求 |

按部署提交与客户端版本解释结果，结合 `requestId/itemId/sessionRef` 和 Range 交叉核对。PCAP 内的会话引用只在本次分析进程内一致，不等于 Gateway 自身的 `sessionRef`，也不能跨次重跑直接比较。`sourceDownloadMs/rangeReadMs` 已含在 `preIDMs/challengeMs` 中，不能重复相加；旧部署的混合 HTTP trace 边界见 [源读取诊断合同](../reference/p115-cookie-playback-contract.md#71-源读取故障诊断与预算2026-10-12)。

HTTP 路径白名单已包含固定 `original.mp4`、`original.mkv`，可与 stream 系列一起形成视频请求响应配对；其他任意文件名继续显示 `[other]`。已知客户端识别包括有界 Infuse/SenPlayer 版本，未知 UA 不输出原文。2026-10-14 修复前，SenPlayer 原文件请求曾因白名单缺失被报告为 0 个视频配对；复核这类材料应使用当前工具，不能从旧结果推断没有视频请求。

## 5. 支持范围、保存与离线验证

- 支持 classic PCAP、Ethernet、IPv4 TCP；不支持 pcapng、IPv6、VLAN、IP 分片、HTTP/2/3 或任意乱序 TCP 重组。畸形包跳过，截断的 PCAP 报受控错误，不把不完整记录解释成正常请求。
- 只解析连续片段与有界的 HTTP/1 头/小型 JSON 请求；大请求、chunked body、抓包丢包、snaplen 截断、采集前已建立的连接均可能使证据缺失。未知路由显示 `[other]`，未知 query key 显示 `other`。
- metadata 的 socket drops 是观测完整性的限制，不用于定位丢包发生在哪一跳。没有观察到 SYN 或 SNI，不代表连接没有发生。
- 捕获的是 Gateway 的命名空间；302 后的“播放器→115 CDN”视频字节通常不经过这里。实际首帧、CDN 全文件 Range/HEAD 合同与网络具体故障点仍需另行取证。
- PCAP 可能包含真实鉴权信息，留在本机受限的非临时目录，不能提交 Git。分析 JSON 已过滤鉴权字段，但仍含 IP、SNI 和数字条目 ID；共享结论时摘录必要证据。
- 其他机器复用工具时不需要复制本次原始包、SSH 私钥或 session。需要离线复核历史材料时，另行通过已授权的私有方式传递原始包。

仓库根目录执行以下离线检查；测试使用合成 PCAP、fake socket 和 mock SSH，不启动服务、不请求真实 Emby/115：

```bash
python3 -B -m unittest discover -s scripts/gateway-capture/tests -v
```

2026-10-13，在修复提交 `248dedb6` 基础上的工具工作区，本机 Python `3.9.6` 通过 15 项离线测试和 8 个 Python 源文件编译检查；整体复制工具目录后，再次通过相同测试与 dry-run，验证其不依赖仓库工作目录。用已有抓包离线回放提取到 452 个 HTTP 事件、5 个视频请求响应配对和 9 个 TLS 连接，5 个视频结果与原报告一致。本次参数化后的新远端采集入口及其他操作系统尚未实机验收；此前真实采集只证明原始 Linux AF_PACKET 方法在目标部署可用。

## 6. 2026-10-12 样本基线

当时运行 `preview-f18c84e`（源码 `f18c84e10f4e47010500900a777d8d8c18afa8f8`）、Emby `4.9.3.0`、Infuse `8.5.6`。下列为历史观测，不能直接视为其他机器或当前部署的状态。

| 条目与阶段 | HTTP 结果 | 应用耗时 | 包端响应头耗时 |
| --- | --- | --- | --- |
| 511291 首次转存 | 302 | 11591 ms | 11591.749 ms |
| 511291 停止后重播 | 302，缓存命中 | 19 ms | 19.256 ms |
| 511191 首个视频请求 | Emby 206，preID 超时回退 | 16905 ms | 16905.535 ms |
| 511191 同一播放会话内下一 GET | 302，首次成功转存 | 7875 ms | 7875.712 ms |
| 511191 停止后人工重播 | 302，缓存命中 | 9 ms | 10.124 ms |

该轮存储 36891 个包，socket drops 48。四次停止后日志中的账号/用户占用均归零；重播命中缓存，没有重复转存编排。硬盘条目另轮采集表现为 `path_not_mapped` 后透传 Emby `206`，播放事件 `204`。

旧共享 5 秒 preID 预算分别在 TLS 和等待响应头阶段耗尽。第二例源取链约 2.37 秒、CDN TCP 建连约 183 ms，未观察到 SYN 重传，因此不能只归因于 TCP 重传。旧 `source_range.firstByteMs` 还混入取链响应，不能当作 CDN 首字节。

历史修复提交 `248dedb6` 将取链/Range 各限 5 秒、另设每次源片段总限 10 秒并分离 trace。后续 `d3f6fed` 已移除 Service/Adapter 竞争的外层 10 秒定时器；当前仍各保留 5 秒阶段窗口和调用方更早 deadline，以保留实际失败阶段。单元测试不证明部署后的起播效果；持续失败时回退可能更晚。按同样的首播/停止/重播步骤采集修复部署，才能判断实际效果。更完整的当前行为定义见 [115 合同](../reference/p115-cookie-playback-contract.md#71-源读取故障诊断与预算2026-10-12) 与 [端到端流程](../reference/p115-playback-end-to-end-flow.md)。
