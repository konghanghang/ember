# 测试指南

这份文档只保留测试入口和最短执行路径，不再把历史测试报告和整套旧栈说明塞进来。

## 何时看这份文档

- 你改了 API、Web、Bot，需要知道最低验证动作
- 你改了集成配置、鉴权、支付、Telegram，需要知道还要补哪些手工检查
- 你要决定是做“编译级验证”还是“完整手工回归”

## 最短验证路径

### API 改动

```bash
cd services/api
go vet ./...
go test ./...
go build ./...
```

Playback Gateway 相关改动额外运行：

```bash
cd services/api
go test -count=1 ./internal/integrations/emby ./internal/playbackgateway
go test -race -count=1 ./internal/integrations/emby ./internal/playbackgateway ./internal/services/embytoken
go test -count=1 ./internal/entrypoint ./internal/app
go build ./cmd/ember
```

115 personal/system 路由与 Redis 配额的默认测试使用进程内 fake Redis、`httptest` fake Emby/115，不需要真实服务。专项入口：

```bash
cd services/api
go test -count=1 ./internal/integrations/p115 ./internal/playbackgateway ./internal/services/directplay ./internal/services/p115account ./internal/services/p115quota
go test -race -count=1 ./internal/integrations/p115 ./internal/playbackgateway ./internal/services/directplay ./internal/services/p115account ./internal/services/p115quota
```

专项测试必须覆盖 personal/system 账号路由、Redis reservation/active/paused/Stopped、HEAD 不创建、小时/自然日转存额度、Redis 断连 fallback，以及成功 `302` 不访问 Emby、DirectPlay 失败无损回到 Emby。不得把 fake Redis 替换为真实 Redis、Emby、115 或 CloudDrive2 调用。

2026-10-17 二轮修复 A–C 已分别通过 API 全量 `go vet ./...`、`go test ./...`、`go build ./...`；B 的 Gateway 与 C 的 Provider/账号/DirectPlay/Gateway 定向 race 通过。关键回归包括 `cookie_failure_classification_test.go`、`cookie_download_context_test.go`、`source_range_budget_test.go`、`account_limiter_cleanup_test.go`、`rapid_upload_observation_test.go`，以及 Gateway 的取消/独立租约释放/设备隔离/容量测试和账号 `store_revoke_test.go`。源读取预算使用 `internal/testutil/faketime` 的手动时钟；新时钟方法通过这些上层行为用例覆盖，不靠真实 5 秒等待。tombstone 事务使用 SQL mock；这批验证未启用真实 PostgreSQL，也未执行真实外部或播放器验收，不能外推历史数据库结果。

上述命令只做 fake 上游、生命周期、统一子命令分发和构建验证，不启动 API/Gateway、不请求真实 Emby。默认构建验证不会在工作区生成二进制；需要单独产物时显式使用 `go build -o bin/ember ./cmd/ember`，且禁止提交 `bin/`。

若环境中已设置集成数据库变量，但本次只验收 fake 链路，必须显式排除集成用例，避免包级命令自动连接数据库：

```bash
go test ./internal/services/directplay ./internal/services/p115quota ./internal/playbackgateway -skip Integration -count=1 -timeout=90s
go test -race ./internal/services/directplay ./internal/services/p115quota ./internal/playbackgateway -skip Integration -count=1 -timeout=90s
```

Gateway 客户端兼容与首次转存策略专项在 `services/api` 执行：

```bash
go test ./internal/config ./internal/playbackgateway ./internal/services/directplay ./internal/services/p115quota ./internal/services/p115account -skip 'Integration|PostgreSQL' -count=1 -timeout=180s
go test -race ./internal/config ./internal/playbackgateway ./internal/services/directplay ./internal/services/p115quota ./internal/services/p115account -skip 'Integration|PostgreSQL' -count=1 -timeout=180s
```

重点覆盖 `client_surface_compatibility_test.go` 的图片与会话、`playback_revalidation_test.go` 的活跃关联/原会话/发布乱序/等待者取消、`transfer_trigger_test.go` 的模式准入和生产装配，以及配置目录的策略保存/五秒刷新/失败关闭。首次转存仍复用现有 DirectPlay 配额、锁内查重、HEAD 与失败释放回归；不使用真实 Emby、115、Redis 或数据库。

设置中心在 `services/web` 执行 `npm run test -- src/views/admin/SettingsView.spec.ts` 和 `npm run build`；采集日志在仓库根执行 `python3 -B -m unittest discover -s scripts/gateway-capture/tests -v`，必须覆盖归因枚举和新诊断 code 的脱敏边界。以上验证不启动服务，也不代替 Yamby/SenPlayer/Infuse 真实客户端验收。

后续协议兼容增量在 `services/api` 执行：

```bash
go test ./internal/playbackgateway -run '^TestGateway(ApplicationAuthorizationScheme|ClientProtocol|StreamFirst)' -count=1 -timeout=120s
go test -race ./internal/playbackgateway -run '^TestGateway(ApplicationAuthorizationScheme|ClientProtocol|StreamFirst)' -count=1 -timeout=120s
```

- `authorization_scheme_test.go` 覆盖三种应用头中的 Emby/MediaBrowser 标准、小写与混合大小写 scheme、Token 大小写保真、重复/冲突、非法 grammar 和日志脱敏；只改变 scheme 识别，不扩展 Token 或认证语法。
- `client_protocol_regression_test.go` 覆盖复杂多 MediaSources 按 ID 选择、音轨/字幕/空数组/未知字段原字节保持、实际视频 UA、Range/If-Range 的 200/206/416 与 HEAD，以及普通字幕/HLS/Audio/Download/Sync 的受保护透传和上游拒绝。Audio/Sync 只证明通用代理，Download 不证明新增套餐下载权益。
- `stream_first_test.go` 覆盖显式 Static、唯一媒体源且完全无会话的扩展名 stream / 固定 original；original 必须显式带同扩展名 Container，stream 的可选 Container 必须一致，DeviceId 唯一且匹配 Principal。检查未知/选流参数、缺 Static/源、空会话 key、固定会话冷 proof、重复身份参数、manifest、歧义/错配、补查和 DirectPlay 失败时原 URL/RawQuery/Range/Header 保持，以及 strict/compatible、HEAD 禁新增、Size=0、取消等待与发布隔离。

随后复用前述 Gateway/DirectPlay/配额专项覆盖固定会话 live binding、旧 plain stream 权威 fallback、停止、许可和租约回归，再执行 `go test ./... -skip 'Integration|PostgreSQL' -count=1`、`go vet ./...` 与 `go build ./...`。本增量不新增 Web UI、配置或 migration，也不实施 CORS；测试不得启动服务或请求真实 Emby、115、Redis、数据库。最终执行结果记录在[主计划第 7 节](../plan/architecture/gateway-client-compatibility.md#7-后续协议兼容改进2026-10-25)，不能用定向通过替代整体验证。

真实验收仍须经授权部署后按客户端与版本记录登录、选源、首播/复用、拖动/续播、字幕、停止及 CDN Range 的分项结果；fake 响应、Gateway `302/206`、上游会话 `204` 不证明首帧或完整播放。既有 Infuse/SenPlayer 样本只覆盖当时版本与请求形态。

采集、缓存原因与 preID 复用增量在 `services/api` 执行：

```bash
go test ./internal/services/directplay -run '^Test(CacheDiagnostics|PreID)' -skip 'Integration|PostgreSQL' -count=1 -timeout=180s
go test -race ./internal/services/directplay -skip 'Integration|PostgreSQL' -count=1 -timeout=180s
```

- `cache_diagnostics_test.go` 锁定媒体/直链全部固定原因、hit 无 reason、未触及缓存不输出容量快照、强制刷新失败仍有诊断，以及并发容量淘汰只输出进程累计快照；Provider 工作量、原 TTL/淘汰顺序和 Debug 事件保持。
- `preid_cache_test.go`、`preid_behavior_test.go` 与 `preid_reuse_test.go` 覆盖源账号/Provider/正版本/精确凭据/Provider 参数/完整文件身份/范围隔离，1024 条容量与固定 10 分钟 TTL、时钟回退、非法哈希/字节数和取消不发布、独立等待者取消、失败 owner 后重试，以及恢复探测/未知身份不读取、不合并、不发布。成功哈希无需等待目标上传，后续目标失败不删除；同范围 challenge 仍实时读取，HEAD、许可、配额、任务和健康边界不能被缓存跳过。
- 等工作量验证保留两个用户、两个个人目标和各自任务/配额，分别比较顺序及重叠请求；已取得 fake 基线到复用后的 preID 读取 `2→1`，source 解析 `2`、目标查重 `4`、上传/复核/最终取链各 `2`、独立任务 `2`、每用户额度 `1` 均保持。只减少前缀读取不能当作真实命中率或客户端延迟收益。

采集工具在仓库根运行 `python3 -B -m unittest discover -s scripts/gateway-capture/tests -v`，使用合成 PCAP、fake socket 和 mock SSH。覆盖固定路由模板与 UA、有界认证逐 code 过滤、恶意嵌套 JSON、requestId 歧义和明确会话绑定例外、上游 unknown、图片最终状态、独立阶段统计、最终决策缓存计数、累计容量快照及 preIDWait 新字段；不按时间跨 PCAP/日志 join。详细输出合同见[抓包手册](./gateway-packet-capture.md#4-离线分析与对照)。

最后复用全量非数据库 Go 测试、相关包 race、vet/build 和独立审查，执行结果单独记入[主计划第 8 节](../plan/architecture/gateway-client-compatibility.md#8-采集缓存诊断与-preid-复用2026-10-25)。本轮没有 Web UI、配置、migration、图片/字幕缓存、预热、负缓存或连接池调参；测试不启动项目服务、不访问真实外部系统或数据库。实际命中率和准备耗时仍须经授权部署后测量。

2026-10-25 在 `master@8799688` 加本轮增量上，`go test ./... -skip 'Integration|PostgreSQL' -count=1 -timeout=180s`、DirectPlay/p115account/p115quota/integrations/p115/playbackgateway 五包同范围 race、全量 `go vet ./...` 与 `go build ./...` 均通过，独立审查问题已收口。Python `3.9.6` 原目录和独立复制目录的 39 项离线测试、10 个源文件编译/AST 检查通过；另以 Go 实际 `TimingDiagnostics.LogFields` 串接 Python `filter_line/build_summary` 复验 miss/hit/bypass、新原因/快照/preID 字段及累计摘要语义。这些本地验证没有使用真实 Emby、115 或数据库。

2026-09-16 上述两种范围均通过。新增回归覆盖独立 locker 在真实 `database/sql` 小连接池上的 fake 锁竞争/取消、A 租约过期后 B 占满额度时 A 不得返回直链，以及 HEAD/Playing 不重建丢失租约。`TestIntegrationPostgresContentLockSmallPoolWaiterCancellation` 已保留为真实 PostgreSQL 专项，但本轮专用库不可达，用户确认采用 fake 验收，未执行该用例；不能将 fake driver 或 miniredis 结果写成数据库/真实 Redis 已验证。

2026-09-21 已在专用 `ember_integration_codex` PostgreSQL 数据库补跑 DirectPlay 专项：

```bash
go test ./internal/services/directplay -run '^TestIntegration' -count=1 -timeout=180s
```

13 个顶层测试及 2 个子测试全部通过，零跳过。覆盖配置版本与健康回写隔离、共享/个人目录保存竞争、配置版本 migration 幂等、并发转存、挑战/失败任务持久化、冷却探测、旧凭证结果防覆盖、小连接池锁等待取消及最近任务成功记录采样。首次运行发现测试 helper 在完整迁移后重放旧增量，重新引入已废弃的 `ck_p115_accounts_source_location`；已按顺序补齐后续个人账号与配置版本增量，再完整重跑通过。此修正只影响测试初始化，不改生产迁移或业务逻辑。

本次使用真实 PostgreSQL 和独立 `itest_*` schema（测试后清理），115 Provider 为 fake；未启动项目服务，未验证真实 Redis、Emby、115 或播放器。当前 CI 只单独运行 `internal/app` 数据库集成，不包含上述 DirectPlay 专项，发布复验需显式执行。

如果要跑本地 API 集成测试：

- 必须通过 `EMBER_INTEGRATION_DATABASE_URL` 指向专用测试数据库

截至 2026-09-05，已在专用 PostgreSQL 集成环境执行并通过：

```bash
go test ./internal/app -run 'Integration|PostgreSQL|P115' -count=1 -v
```

该测试会由 harness 创建和清理独立的 `itest_*` schema；不会启动 Ember 服务，也不会访问真实 Emby、115 或其他外部系统。若环境变量缺失，相关测试会跳过，不能将跳过写成通过。

`docker compose config --quiet` 属于部署者上线前的运行前检查，不是本地代码测试的必要条件；应在目标部署环境使用实际 `.env` 和 Compose override 单独执行。
- 不要直接连接共享开发库，尤其不要复用其他人正在使用的测试库
- 集成测试骨架会在目标数据库里创建并清理独立 schema，所以目标库本身应只用于集成测试
- 115 migration 用例会重复执行当前增量，并覆盖套餐默认值、账号 partial unique、owner `ON DELETE RESTRICT`、revoked tombstone 与 transfer provenance；未设置该变量时会明确跳过，不能据此声称 PostgreSQL migration 已实际执行

示例：

```bash
cd services/api
EMBER_INTEGRATION_DATABASE_URL='postgres://user:pass@127.0.0.1:5432/ember_integration?sslmode=disable' \
go test ./internal/app -run Integration -count=1
```

如果需要保留每个测试用例的执行结果：

```bash
make test-api-report
```

### Web 改动

```bash
cd services/web
npm ci
npm run test
npm run build
```

如果需要保留测试结果产物：

```bash
make test-web-report
```

### Bot 改动

```bash
cd services/bot
python3.11 -m venv .venv
source .venv/bin/activate
pip install -r requirements-dev.txt
python -m py_compile main.py
python -m pytest tests
```

约定：

- Bot 测试与本地运行默认使用 `services/bot/.venv`
- 当前仓库通过 `services/bot/.python-version` 固定 pyenv Python `3.11.15`；本机验证已执行 `.venv/bin/python -m py_compile main.py` 和 `.venv/bin/python -m pytest tests`，49 项测试全部通过
- 如果 `.venv` 不存在，`make setup`、`make test-bot`、`make test-bot-report` 会直接提示先创建虚拟环境

如果需要保留测试结果产物：

```bash
make test-bot-report
```

## 什么时候必须补手工测试

下列改动只跑编译不够：

- 登录、注册、兑换、账号状态流转
- 管理后台关键页面和设置保存
- Emby、TMDB、MoviePilot 集成
- Telegram 绑定、通知、Webhook
- 支付流程
- Docker Compose、环境变量、反向代理、部署脚本

手工测试项见 [手工测试清单](./manual-testing-checklist.md)。

## 历史测试报告怎么处理

现行指南不再内嵌历史报告。需要追溯旧测试结论时，去看：

- [历史测试报告：2025-12-07](../archive/report/test/2025-12-07-mvp-core-testing.md)

## 常见误区

- 文档改动不会触发 `.github/workflows/test.yml`，因为 CI 对 `docs/**` 和 `*.md` 做了 `paths-ignore`
- `go build ./...` 通过，不代表 Emby、Telegram、支付链路真的可用
- 手工测试不是“把所有页面点一遍”，而是按变更范围跑对应清单

## 继续阅读

- [测试策略](../reference/testing-strategy.md)
- [集成测试手册](./integration-testing.md)
- [手工测试清单](./manual-testing-checklist.md)
- [115 Cookie Provider 一次性只读合同验证](./p115-read-only-contract-check.md)
- [115 playback 保留式秒传合同验证](./p115-retained-transfer-contract-check.md)
- [Stripe 支付测试指南](./stripe-payment-testing.md)
- [测试排障](./testing-troubleshooting.md)
- [部署指南](./deployment.md)
- [Cloudflared 本地联调](./cloudflared-local-testing.md)

### 分组权益改版专项

在 `services/api` 工作目录执行 `go test ./internal/services/entitlement ./internal/services/payment ./internal/services/system ./internal/services/policy -skip Integration`，并按需补关键包 `-race`。专用 PostgreSQL 设置后执行 `go test ./internal/app -run 'TestIntegration(Entitlement|LegacyEntitlement|Billing)' -count=1 -v`，覆盖迁移重跑、保留人工限制/历史记录、旧码失效和并发续期。2026-10-04 已连接专用测试库执行上述 6 个用例并通过，临时 schema 已清理；升级步骤见 [权益升级说明](./entitlements-upgrade.md)。

2026-10-04 单组商品收敛补充：`go test ./internal/app -run 'TestIntegration(SingleGroupPlan|HistoricalCombinationSnapshot|Entitlement|LegacyEntitlement|Billing)' -count=1`，共 9 个顶层用例通过，覆盖新商品 DTO、永久/限时转换、旧商品 SQL 转换及组合拒绝回滚、历史组合订单在商品下架/改天数后按原快照履约与重放幂等。配合 Web 单组表单 4 项回归及权益工具测试；完整 Web 测试 314 项通过、3 项按现有配置跳过。

2026-10-04 CI 求片回归修复：完整 `go test ./internal/app -run Integration -count=1` 在隔离 schema 中 33 个顶层用例通过（包括新建/重新提交的有效权益、无权益、人工禁用边界）。此前只跑权益/支付专项未覆盖求片的真实用户查询，不能用专项通过替代完整集成验证。Web 集成夹具在迁移之后建用户，须同时初始化访问投影和永久权益；无服务单测保护该初始化，完整 Web→API→数据库流程仍须 CI 复验。


## 永久兑换码验证（2026-10-05）

本轮在 `293c5d1` 基础上的未提交兑换码改动完成以下验证，范围涵盖创建/批量/编辑、注册与已有用户兑换、历史快照、Telegram 展示及前后端合同：

- API：`go test ./...`、`go vet ./...`、`go build ./...` 通过；新增 mock 用例覆盖永久注册不按零天试用禁用、永久权益落库、消费失败回滚、重复兑换不消耗、零天不被 GORM 默认值覆盖和历史类型返回。
- Web：`npm run build` 通过；新增兑换码表单测试覆盖默认限时、永久请求归一化、批量及编辑。首次默认并发全量测试有一条既有登录跳转等待断言失败，单独复验通过；`npm test -- --maxWorkers=2` 最终 323 项通过、3 项既有集成用例跳过，未改动该登录用例或放宽断言，偶发失败根因未证实。
- Bot：使用 `services/bot/.venv/bin/python` 编译检查和 `pytest tests`，82 项通过；全局 `python3` 是旧版且没有 pytest，不能代替项目虚拟环境。
- PostgreSQL：加载仓库根私有 `.env.integration.local` 的 `EMBER_INTEGRATION_DATABASE_URL` 后，在专用库的独立临时 schema 中实际执行 `TestIntegrationRedemptionPermanentValidity` 与 `TestIntegrationRedemptionValidityUpgrade`，两项通过。覆盖 fresh-install、旧列形态回填、重复执行、永久发放、重复兑换保护、编辑后历史不变及数据库约束；schema 由测试清理。该 dotenv 不会被直接执行的 `go test` 自动加载，运行前须显式将变量传入测试进程，不打印或提交连接串。

数据库专项入口：在 `services/api` 工作目录并已加载上述测试变量后执行 `go test ./internal/app -run '^TestIntegrationRedemption(PermanentValidity|ValidityUpgrade)$' -count=1 -v`。测试不启动项目服务，也不调用真实 Emby、Telegram 或支付链路；真实 Emby、Telegram 或支付链路未验证；后续浏览器验收见下。


### 永久兑换码浏览器追加验收

2026-10-05 使用 Playwright MCP 验收用户已运行的 `http://localhost:3000/console/billing?tab=codes`，复用管理员登录；未启动项目服务。用户明确授权创建并清理临时兑换码，不兑换、不修改用户权益。

- 默认按天 30 天；切换永久隐藏天数，切回按天恢复；输入 0 天失焦后限制到最少 1 天。
- 单个永久码创建成功，页面真实 POST 返回 200，请求为 `validityType=permanent/defaultDays=0`，列表显示永久；编辑为 45 天再改回永久，两次 PUT 均返回 200，列表和重新打开表单的值一致。
- 批量生成 2 个永久码并独立设置兑换截止时间，POST 返回 200；两条记录同时正确显示永久权益和设定的截止时间。
- 创建/编辑弹窗在宽屏与 390px 窄屏下无页面或弹窗横向溢出。
- 发现同页两处备注输入框 `rows="2"` 类型警告，改为数值绑定 `:rows="2"`；刷新后重新操作，控制台 0 errors / 0 warnings，兑换码组件 3 项测试复验通过。
- 本轮 3 个临时码已通过页面逐一删除，列表恢复到原有 1 条记录，本轮测试备注匹配数为 0。MCP 浏览器已关闭，42 个本轮快照/日志已清理，原有产物保留。

浏览器范围限于管理员生成、批量、编辑及删除；实际用户注册/兑换未在该运行环境执行，对应权益发放证据来自前述 mock 和专用 PostgreSQL 测试。


## 用户中心权益编辑验证（2026-10-05）

同日补充用户列表分组筛选验证：`TestIntegrationUserEntitlementFilter` 使用 `.env.integration.local` 中的专用 PostgreSQL 配置，在自动清理的隔离 schema 内通过真实 HTTP handler 验证当前分组兼容、非当前组永久/未到期权益、排除过期/撤销权益、组合查询、非法 key 和分页计数；不调用真实外部服务。`UsersView.spec.ts` 覆盖筛选传参、页码重置、清空两个分组条件，以及仅选持有组不会成为历史同步目标。Web 全量 331 passed / 3 skipped，build 通过；Go 全量 test、vet、build 通过。无数据库 schema 变更。浏览器首轮验收：管理员登录后确认新参数正确发送；当前分组、日期筛选、重置及 390px/1440px 无页面横向溢出通过，控制台无错误或警告。首轮发现 GoLand API 仍为修改前的旧进程，用户重启后已完成复测：持有 XIANYU 返回两名双权益测试用户并排除过期用户，与当前 DEFAULT 取交集仍为两人，与当前 XIANYU 取交集为空；关键词 qaent1004b 分别结合 XIANYU / DEFAULT 持有组均只返回本人；重置恢复全部 8 人，20 条/页切换发送 page=1/pageSize=20，单页前后翻页按钮禁用正常。相关列表请求全部 200，控制台无错误或警告。现有数据不足一页，真实浏览器跨页未覆盖，跨页计数由上述 PostgreSQL 隔离集成测试覆盖。未写入用户权益，浏览器已关闭，本轮快照已清理。

基于 `9c27b4a` 的本轮未提交改动：权益面板直接展示三种操作并统一保存/取消按钮；编辑用户统一保持不变、延期、日期和永久四种模式，移除操作列独立延期按钮，换组与改期限分开保存；日期使用全局业务时区。

- 前端全量 `npm test -- --maxWorkers=2`：330 项通过、3 项既有集成测试跳过；`npm run build` 通过。覆盖三种操作的请求字段、缺日期拒绝、永久切换、失败重试、取消、仅提交变化字段、跨时区日期回填与换组保护。
- API：`go test ./...`、`go vet ./...`、`go build ./...` 通过；日期解析单测覆盖业务本地时间、UTC、显式偏移和非法日期。
- 通过私有 `.env.integration.local` 加载专用测试库，实际执行 `TestIntegrationAdminCurrentGroupExpiry`、`TestIntegrationAdminEditExtension`、`TestIntegrationAdminTransferGroup`、`TestIntegrationAdminTransferGroupRejectsConflict` 四项通过。覆盖日期/永久/兼容 RFC3339、其他组权益与人工禁用保留，以及合并延期与资料的原子保存、重试只延期一次、邮箱冲突回滚权益/审计、换组保持期限和冲突回滚；临时 schema 已清理，未调用真实外部系统。
- Playwright MCP 在用户已运行的本机用户中心只读验收：三种权益操作直接可见；保存按钮 12px 圆角、14px 字号、指针及右对齐生效；永久模式隐藏日期/天数，指定日期显示业务时区；编辑用户四种有效期操作切换正常，默认保持不变，延期显示默认 30 天，指定日期显示业务时区，永久不显示日期/天数。操作列独立延期按钮数量为 0。390px 窄屏下两个弹窗无横向溢出，控制台 0 errors / 0 warnings；只有读取请求，没有提交真实用户权益变化。浏览器已关闭，两轮本机验收的 12 个及 10 个快照均已清理。

本轮无模型或 schema 变化，无新增 migration。后端新增的无偏移日期解析需 API 加载本次版本后生效；未代为重启用户服务。

## 观看保号回归

在 `services/api` 执行 `go test ./internal/services/entitlement ./internal/services/payment ./internal/services/user ./internal/services/system ./internal/integrations/emby -skip Integration`，覆盖保号起算、失效/恢复、调度边界、SQL mock 回滚及插件响应合同。`TestIntegrationWatchRetentionLifecycle` 仅在专用 `EMBER_INTEGRATION_DATABASE_URL` 配置后执行，验证真实 PostgreSQL 迁移重放与生命周期，外部播放及 Policy 使用 fake。

2026-10-11 增加恢复与公平扫描专项：在同一专用数据库配置下执行 `go test ./internal/app -run '^TestIntegrationWatchRetention' -count=1 -v`。覆盖提交后中断的禁用/降组恢复、已有活跃任务替换及批次保留、即时同步成功/失败、成功或失败前缀后的跨轮推进、进度迁移重放。配置、首次日期和运行期注册统一拒绝 `@every`；旧配置暂停后权益展示仍可读取，由 `config`、`entitlement` 和 `user` 包单测覆盖。

后端修复已提交为 `00b24a8`，通过全量非集成 Go 测试、`go vet ./...`、`go build ./...`，以及 `entitlement/policy/user/config` 四包竞态检测；完整 `internal/app` PostgreSQL 集成回归通过，零跳过。外部依赖均为 mock/fake，临时 schema 自动清理；该轮未启动服务，未验证真实 Emby 或浏览器。

Web 使用 `npm run test` 覆盖 `watch-retention`、分组表单、概览和持有权益；Bot 格式化回归验证失效永久权益不再显示为永久。测试不请求真实 Emby 或 Telegram，不启动项目服务。

2026-10-08 补充 PostgreSQL 实测：从本机私有 `~/.config/zsh/local.zsh` 显式注入专用连接后，`TestIntegrationWatchRetentionLifecycle` 通过；另外 `TestIntegrationEntitlementMigrationAndFallback`、`TestIntegrationRedemptionPermanentValidity`、`TestIntegrationRedemptionValidityUpgrade`、`TestIntegrationAdminTransferGroup`、`TestIntegrationAdminTransferGroupRejectsConflict`、`TestIntegrationUserEntitlementFilter` 六项通过。均在独立 `itest_*` schema 中运行并自动清理，外部播放与权限同步为 fake；此次结果不扩展为真实 Emby 或浏览器验收。

### 观看保号管理员浏览器与表单回归

2026-10-11，基于 `00b24a8` 的 Web 表单与设置页修复工作区完成以下验证，无 API 合同或数据库 schema 变更：

- 真实 Element Plus 数字控件回归覆盖新建/编辑开启后保持空门槛、填写后保存、关闭时空值提交为 `0`、切换开关保留已有正数。设置页组件测试覆盖保存失败后保留草稿、恢复按钮、不重复提示或冒泡 Vue 异常，以及修正后重试成功。
- `npm run test -- --maxWorkers=2`：41 个文件通过、1 个既有集成文件跳过，348 项通过、3 项既有跳过；`npm run build` 类型检查与生产构建通过。
- Playwright MCP 在用户已启动的本机服务验证：新建及编辑原门槛为 `0` 的临时分组，开启后输入框保持空白，提交被拦截且无保存请求；关闭后创建请求以 `watchRetentionMinMinutes: 0` 返回 `200`；编辑为开启、14 天/90 分钟后保存返回 `200`。
- 设置中心提交 `@every 24h` 返回预期 `400`，正确展示拒绝原因，草稿保留、保存按钮恢复可用。控制台仅有该 HTTP `400` 记录，没有 Vue 警告或未捕获异常；随后重置回原 `0 2 * * *`，保存按钮禁用，没有修改有效调度配置。
- 临时分组已删除，列表恢复原有两组；没有修改既有分组或用户权益。MCP 浏览器已关闭，本轮 17 个快照/日志已清理，原有产物保留。

本轮浏览器范围为管理员配置，用户端观看要求、真实播放考核与 Emby 权限同步尚未实机验收；相关自动化仍使用 mock/fake。

### 观看保号日志回归

2026-10-11，基于 `48dd5244` 的日志补充工作区以 SQL mock 和现有 fake 验证：

- `TestWatchRetentionCronRegistrationLogs` 使用真实 cron 注册器核对注册成功/失败、业务时区及下次执行时间，并验证回调会记录 worker 失败；不启动调度器或项目服务。
- `TestRetentionRunSummary` 覆盖空候选、候选读取失败、未满周期、不再符合条件、达标、失效、状态变化、重复考核、统计失败、中断及即时同步失败的计数和结果。扩展 `TestRetentionInvalidationAndRollback`，确认数据库提交失败时不打印“考核完成”。
- `TestPolicyTaskTerminalLogRequiresPersistedResult` 覆盖 synced/failed 终态、旧任务被替换、更新及提交失败；`TestImmediatePolicySyncWithoutClaimDoesNotLogSuccess` 确认未领取任务不报同步成功。终态日志不输出原始上游错误内容。

Go 全量非集成测试、`go vet ./...`、`go build ./...` 及 `entitlement/policy` 两包竞态测试通过。本次不改 API 合同、数据库 schema 或 Web；未触发真实外部链路，既有专用 PostgreSQL 与浏览器证据不扩展为本轮线上日志已实测。线上观察入口与字段含义见[观看保号运行手册](./watch-retention.md#线上日志观察)。


### 观看保号起点规则回归（2026-10-18）

API 全量非数据库测试、vet/build 和 entitlement/payment/user/system 四包 race 通过。先用失败用例锁定“开启分组不能覆盖成熟权益起点”，再移除分组重开字段；缺失／零值起点跳过并记录固定原因，不查询真实播放或写失效。

专用 PostgreSQL 的隔离 schema 已运行 `go test ./internal/app -run '^TestIntegrationWatchRetention' -count=1 -timeout=180s -v`，6 项顶层用例全部通过、零跳过。新增 `TestIntegrationWatchRetentionRuleChangesKeepStart` 覆盖真实分组更新和权益 DTO；`TestIntegrationWatchRetentionResetRemoval` 覆盖 fresh/upgrade/replay、原起点/失效/审计保留、空值不回填、正数门槛约束及新增启用分组。既有恢复、公平扫描和生命周期用例同步通过，所有 Emby/播放/Policy 依赖为 fake。

Web 定向运行 `npm run test -- src/utils/watch-retention.spec.ts src/views/console/DashboardView.spec.ts src/components/billing/UserEntitlementsPanel.spec.ts --maxWorkers=2`，13 项通过；本轮未修改 Web 源码。未启动项目服务、未操作线上业务数据，真实播放器和浏览器验收未执行。
