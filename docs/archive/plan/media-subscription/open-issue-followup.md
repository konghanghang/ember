# 支付与媒体状态问题修复计划

> 状态：已归档（本地修复与逐 issue 提交完成）
> 负责人：Ember
> 更新时间：2026-09-17

## 背景与目标

修复 GitHub #21、#22、#24、#25、#23 的收费快照、日期边界与并发状态问题。每个 issue 独立提交代码、测试和对应文档。用户已授权实施和提交；推送、合并及真实服务验证不在本轮范围。

## 修复前事实

基线为 b8b816f。订单已存金额/币种/天数，但重试仍读取当前套餐收费；历史快照选择排除完整周期上界；剧集白名单候选提前截断；订阅取消及缺集多个更新入口缺少原子来源状态约束。

## 方案设计

- #21：同一 Payment 的金额、币种、天数以订单快照为准，保持既有订单身份和回调幂等。检查 Stripe 重试参数的其他可变来源。
- #22：历史快照元数据上界允许相等；播放明细继续采用半开区间和 CRON_TIMEZONE。
- #24：本人 PENDING 订阅原子条件删除，冲突映射 409，不存在/他人记录维持 404；前端冲突后刷新状态。
- #25：搜索、下发、Webhook 和扫描不得用旧状态覆盖终态；快照与状态分别约束，检查 RowsAffected 并返回权威状态。人工忽略保留既有优先级，不宣称能撤回已发出的外部请求。
- #23：完整单集聚合后解析 Series、白名单过滤并取前十；复用现有批量条目查询，不猜测插件具有 SeriesId 字段。

复用现有字段与状态，本次未修改 schema，无需 migration。前端实现遵守 Ember 风格，设计与交互基线以 [设计规范](../../../reference/web-design-guide.md) 为准，无视觉规范特例。列表响应继续使用 data，字段保持 camelCase。

## 影响范围

API payment/playback/subscription/mediagap 及相关 handler；Web 订阅取消、缺集状态冲突恢复；Bot 保持既有审批调用合同。Emby 按 [Playback Reporting 合同](../../../reference/playback-reporting-api-contract.md) 固定版本基线实现，实际部署插件版本未验证。不改变配置、调度或时区入口。

## 验证方式

先写失败回归，再最小实现。收费和回调使用 fake Stripe；并发通过屏障与 SQL mock/专用测试库验证；排行榜用完整候选 fixture。所有第三方依赖 mock，不启动项目服务。

在 services/api 执行 go test ./... -skip Integration -count=1、相关包 race、go vet ./...、go build ./...；前端行为改动执行相关 Vitest、npm run test、npm run build。未运行的真实 PostgreSQL/第三方验证必须明确记录，不将 SQL mock 视为真实并发数据库验收。

## 进度与落地后文档处理

- [x] #21 订单快照：失败重试/实际收费表单/履约与重复回调 SQL mock，payment 单测、race、vet、build 通过。名称与配置请求参数变化的幂等拒绝边界保留。
- [x] #22 历史快照边界：日/周、新/旧批次、完整/部分周期 SQL mock 通过，明细半开区间回归保留；新增 HTTP/PostgreSQL 集成用例因未配置专用数据库而跳过，未作为已通过验收。
- [x] #24 原子取消：SQL mock/GORM 屏障覆盖取消与批准/拒绝交错、404/409/数据库故障及管理员权限；subscription/handler 单测和 race、Web 取消组件测试与构建通过。真实 PostgreSQL 并发未执行。
- [x] #25 缺集状态竞争：阻塞 fake MoviePilot 与 Ignore/Webhook 交错、扫描 SQL、权威 DTO、409 前端恢复回归通过；缺集包 race 与 Web 构建通过。没有真实外部或 PostgreSQL 并发验证。
- [x] #23 剧集榜完整聚合：真实 Go 聚合函数配合 fake Emby 重现 B=3960 秒被遗漏、入榜剧尾集少算及白名单过滤；修复后全部通过。沿用详情缺失/部分失败降级，不宣称故障时结果仍完整，也未执行生产容量验证。

稳定结论已同步 system-architecture、API endpoint catalog 与 Playback Reporting 合同；本计划只保留实施追溯，已迁入 docs/archive/plan/media-subscription/ 并更新索引和盘点。

## 最终验证与交付边界

- API 全量非集成测试通过；payment/playback/subscription/mediagap/handlers 五包 race 通过；go vet ./...、go build ./... 通过。
- Web 262 项通过、3 项跳过；生产构建通过。
- 五个 issue 均有修复前失败、修复后通过的定向回归，独立系统复核未发现本次范围内阻塞。
- 新增历史接口 PostgreSQL 集成用例已编译，但因未配置 EMBER_INTEGRATION_DATABASE_URL 跳过；SQL mock 和 GORM 屏障不是实际 PostgreSQL 并发验收。
- 第三方全部 fake/mock，未启动项目服务、未进行真实外部或浏览器验收。
- 按 issue 分别创建 SSH 签名提交，保留 Fixes 引用；尚未推送、合并或更新远端 issue 状态。
