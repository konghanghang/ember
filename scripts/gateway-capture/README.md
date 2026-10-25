# Gateway 抓包工具

此目录可以整体复制到其他 Mac / Linux 机器。客户端只需要 Python 3.9+ 和 OpenSSH，不依赖 Ember 服务、数据库或第三方 Python 包。

```bash
python3 -B gateway_capture.py --help
python3 -B -m unittest discover -s tests -v
```

- `capture`：通过显式 SSH 路径，进入已有 Docker Gateway 的网络命名空间，限时、限容量采集 IPv4 TCP。
- `logs`：在远端过滤指定时间段的 Gateway 日志，再写入本机 JSON。
- `analyze`：离线解析本机 PCAP，输出 HTTP、TLS 握手、视频请求响应耗时及有界诊断摘要。
- `capture --dry-run`：只打印配置，不连接 SSH、不创建文件。

跨机器配置、完整命令、权限要求、证据边界和历史样本见 [Gateway 抓包与播放排查](../../docs/runbooks/gateway-packet-capture.md)。若独立复制本目录，请同时带走该手册。

采集和分析产物必须放在仓库外的私有目录。原始 PCAP 可能包含鉴权信息，不随工具分发。工具不会上传文件、安装软件、启动或重启 Ember 服务。

分析保留 `formatVersion=1` 和原有字段，新增 `diagnosticSummary`。HTTP 使用固定客户端 family、路径类别和状态分布；新识别的登录、图片和字幕路径只输出固定模板，动态段不透出。`clientFamily/clientVersion` 与 Gateway 的已知 UA 前缀和有界版本规则一致，原 `client` 字段继续保留。路径类别只描述观察到的请求形状，不代表 Gateway 准入判断。

日志新增四类请求/认证诊断的逐 code 白名单，仅保留固定枚举、有界计数和引用；Host、原始路径、query 值、Token、body 和未知枚举不会通过这些入口。Gateway 摘要只按明确的 `requestId` 关联，重复或冲突记录计入 `ambiguousRequestGroups`；两个会话引用只有明确的 `verified_client_session + proofSessionRef` 绑定可以解释。PCAP 和日志不会按时间自动合并。

缺字段保持 `unknown` 或空集合。登录上游 `200` 不证明本地 Token 映射完成；图片只报告 `finalStatus`，`upstreamStatus` 保持 `unknown`；`302` 和响应头耗时不证明实际播放或首帧。阶段耗时按 `code + stage + field` 独立展示样本数及最近秩 `p50/p95`，Debug step 只统计 `finished`，无 `requestId` 的 Provider 日志不归入请求阶段，也不把嵌套阶段相加。

缓存摘要仅统计最终决策日志中的 `hit/miss/bypass` 和固定原因。`mediaCacheEvictions/downloadCacheEvictions` 是进程累计容量淘汰快照，摘要只取 `maxObserved`，不求和、不当作采集区间增量，也不据此推断某次 miss 的原因；`preIDCache` 与 `preIDWaitMs/preIDWaitCalls` 保留独立观测。
