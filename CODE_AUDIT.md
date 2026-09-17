# Ember 代码审计记录

## 当前检查点：第二轮复审

- 日期：2026-09-17。
- 仓库：`konghanghang/ember`；固定基线：`master@930b1cc9b653a1ae62c1fdc4dd0bc6dce8804c1f`。
- 上轮基线：`bb1cbc6ae955221ab9b99531721ecb3c7fc331d9`。
- 修复入口：[PR #20](https://github.com/konghanghang/ember/pull/20) 已合并；#8、#10–#19 已关闭。
- [上轮完整审计报告](https://github.com/konghanghang/ember/blob/5db0ef797e4b6b2e1571339e450840fc107ed297/CODE_AUDIT.md) 仍保留在 [PR #9](https://github.com/konghanghang/ember/pull/9) 的审计分支中，本轮核对时尚未合并到 master。
- 本轮审计文档通过独立分支 `docs/code-audit-round-2` 提交，合并目标为 master；不自动合并。
- 本轮只审查与验证，没有修改业务代码、启动服务、访问共享数据库或调用真实第三方业务接口；已按用户授权登记 AUD-011–015 对应的新 Issue，未重新打开旧 Issue。
- 复核范围：上轮 10 项修复及相关代码增量，继续检查付款创建、订阅生命周期、媒体缺集、排行榜和日历。不是全仓穷尽审计或生产环境验收。

## 结论

上轮 10 项问题的原触发路径均已在代码中修正，本轮未发现足够证据重开这些 Issue。Bot 修复进行了离线测试；Go/Web 的本轮判断以代码、已有测试审阅及独立查询的 CI 状态为依据，不冒充本地完整测试。

继续扫描发现 **2 项 P1、3 项 P2**，均为本轮新发现的既有问题，没有证据表明是 PR #20 引入。付款快照一致性和历史排行榜边界应优先处理。没有在本轮范围内确认 P0。

| 编号 | 等级 | 问题 | 证据 | 状态 |
|---|---|---|---|---|
| AUD-011 / P1-1 | P1 | Checkout 重试混用当前套餐收费与旧订单权益 | 静态调用链和跨请求状态推演 | [#21](https://github.com/konghanghang/ember/issues/21)，待修复 |
| AUD-012 / P1-2 | P1 | 历史查询排除完整日榜、周榜快照 | 原 SQL 条件离线反例 | [#22](https://github.com/konghanghang/ember/issues/22)，待修复 |
| AUD-013 / P2-1 | P2 | 白名单剧榜先截单集再聚合，漏掉高总时长剧集 | 提取窗口常量后的算法反例 | [#23](https://github.com/konghanghang/ember/issues/23)，待修复 |
| AUD-014 / P2-2 | P2 | 普通用户取消与审批并发，可删除已审核订阅 | 静态并发交错 | [#24](https://github.com/konghanghang/ember/issues/24)，待修复 |
| AUD-015 / P2-3 | P2 | 缺集搜索/下发的旧结果覆盖入库或忽略状态 | 静态并发交错 | [#25](https://github.com/konghanghang/ember/issues/25)，待修复 |

## 上轮问题逐项复核

“代码修复”表示原最小触发路径被消除，不等于所有部署方式、并发和外部失败场景都已实测。

| 上轮编号 / Issue | 复核判断 | 具体证据与验证限制 |
|---|---|---|
| AUD-001 / #10 | 代码已修复 | 成功付款只对 completed 跳过；expired/failed 可进入履约，分组不符保留 failed 事件供重投。新增 SQL mock 与 billing 集成用例；本轮未本地运行 Go。 |
| AUD-002 / #11 | 代码已修复 | 兑换、管理员续期和支付统一锁用户后读取并累加有效期。已有兑换与人工续期集成用例；两个不同兑换码、兑换与支付的独立并发用例仍建议补充。 |
| AUD-003 / #12 | 代码已修复 | 完整同步按用户取得 PostgreSQL advisory lock 后重读状态，覆盖 worker 取消与释放。已读 fake/SQL mock 回归；本轮未运行真实 advisory lock 双实例验证。 |
| AUD-004 / #13 | 代码已修复 | 暂时资料故障进入稳定恢复态，401 才清理身份；重试后仍执行角色与强制改密守卫。新增 memory-router/组件测试；本轮未本地运行 Web。 |
| AUD-005 / #14 | 原风险路径已有修复 | 同内容先本地排队；失败 try-lock 归还连接；有限池为任务 SQL 留出连接，未知锁结果丢弃连接。已读小池 fake driver 测试；真实 PostgreSQL 专项本轮未跑。 |
| AUD-006 / #15 | 原风险路径已有修复 | GET reservation 定时续租，返回前原子确认；丢失租约不返回成功候选；同会话串行且整个准备阶段限时。已读慢解析、失效租约、Stopped/HEAD 回归；本轮未本地运行 Go/Redis。 |
| AUD-007 / #16 | 代码修复，Bot 离线测试通过 | 明确空群 ID 清除缓存并回退管理员；字段缺失/失败保留最近缓存。 |
| AUD-008 / #17 | 代码修复，Bot 离线测试通过 | 改为 peek + 固定 pendingRequestId 的 complete，事务锁上下文和订阅；终态幂等回放，失败可重试。Go 事务测试本轮仅阅读。 |
| AUD-009 / #18 | 代码修复，Bot 离线测试通过 | 按可见文本预算裁剪、保留实体/闭合标签，审批结果预留终态空间，caption/text 分别限长。 |
| AUD-010 / #19 | 代码已修复 | 已绑定及未绑定 Emby 的管理员均走本地重置分支；普通用户保持同步语义。新增本地重置及会话签名测试；本轮未本地运行 Go。 |

已知 #8 的 Latest/Resume/Root 静态路由排除也已阅读对应修改和测试，本轮没有依据重新打开。

历史上已被错误记录为 processed 的支付事件不在 PR #20 的自动补偿范围。该限制在 PR 中已明确说明，属于另行对账工作，不重复报为原修复失败。API/Bot 滚动升级和 Policy 混合版本互斥也应遵循 PR #20 的部署说明。

## AUD-011 / P1-1：付款重试的价格与权益快照不一致

**定位**

- [services/api/internal/services/payment/service.go:700](https://github.com/konghanghang/ember/blob/930b1cc9b653a1ae62c1fdc4dd0bc6dce8804c1f/services/api/internal/services/payment/service.go#L700)：读取当前 Plan，拿到预留 Payment 后仍传当前 Plan 创建 Checkout。
- 同文件 840–850 行保存初始 Amount/Currency/Days；886–898 行复用旧 pending；912–921 行构造上游金额/币种/days；1323 行按 Payment.Days 履约。
- 同文件 288–300、341–355 行：改价/天数/币种不会使 pending 失效，只有改分组才执行失效处理。

**触发与影响**

1. 套餐为 10 美元/10 天，首次创建 Checkout 已写本地 pending，但上游请求尚未成功发出便失败，checkoutUrl 为空。
2. 管理员把同一套餐改为 100 美元/100 天。
3. 用户在 30 分钟内重试，复用旧 Payment ID 和 10 天快照，却以当前 Plan 请求收费 100 美元。
4. 成功付款后只发放 10 天，本地账单金额也仍为旧金额。反向改价同样可能以新低价获得旧高权益。

此序列只要求首次上游会话尚未建立，不依赖对 Stripe 默认有效期或幂等行为的猜测。没有执行真实付款或 Go 生命周期复现。

**建议与验收**

同一 Payment ID 的上游收费参数和本地履约必须来自同一不可变快照；若需要采用新价格，应创建新的订单身份。补“占位成功 → 上游创建前失败 → 改价/天数/币种 → 重试 → webhook”的测试，断言收费、本地账单和权益一致；继续验证重复回调只履约一次。

## AUD-012 / P1-2：完整周期排行榜在历史接口中查不到

**定位**

- [services/api/internal/services/playback/ranking.go:932](https://github.com/konghanghang/ember/blob/930b1cc9b653a1ae62c1fdc4dd0bc6dce8804c1f/services/api/internal/services/playback/ranking.go#L932)；旧批次分支同文件 952 行。
- 默认 dayRange/weekRange 在同文件 330–350 行，computeRanking 在 370–375 行。
- [services/api/internal/handlers/ranking.go:198](https://github.com/konghanghang/ember/blob/930b1cc9b653a1ae62c1fdc4dd0bc6dce8804c1f/services/api/internal/handlers/ranking.go#L198)：历史接口生成相同周期上界。

**触发与影响**

默认日榜记录的 period_end 为次日零点，周榜为下周一零点；历史请求 rangeEnd 也是这个时间。查询却要求 `period_end < rangeEnd`，恰好排除了完整周期快照。于是默认定时/手动生成的记录已保存，按日期查询却返回空榜；若有较早截点快照，也可能只能返回不完整记录。

**已执行反例**

从原 Go 文件提取完整 WHERE 条件，在内存 SQLite 插入日榜和周榜边界记录：

```text
daily:  stored=1, current returned=0, inclusive returned=1
weekly: stored=1, current returned=0, inclusive returned=1
```

这是原 SQL 条件验证，不是 PostgreSQL/GORM 或 HTTP 集成测试。

**建议与验收**

快照元数据选择允许 `period_end <= rangeEnd`，覆盖完整日/周、周期内截点及旧批次。播放明细本身仍保留 `DateCreated >= start AND DateCreated < end` 的半开区间，不要连带改错。

## AUD-013 / P2-1：白名单剧集榜的候选截断使排名错误

**定位**

[services/api/internal/services/playback/ranking.go:190](https://github.com/konghanghang/ember/blob/930b1cc9b653a1ae62c1fdc4dd0bc6dce8804c1f/services/api/internal/services/playback/ranking.go#L190)，203 行以“已聚合到 10 剧”为提前终止条件；225–244 行先按单个 Episode 聚合时长排序并 LIMIT；随后才汇总 Series。

**触发与影响**

开启媒体库白名单，单集候选多于首窗口 300，首窗口已经涵盖 10 部剧。若 A0…A9 各有 30 集、每集累计 100 秒，它们占据前 300 条，每剧 3000 秒；B 有 40 集、每集 99 秒，实际总时长 3960 秒，本应第一，却完全不在候选榜中。已入榜的剧也可能丢失被截掉的单集时长。

离线反例从源码读取窗口常量，得到 `window=300, reported top=3000, omitted B=3960`。这是算法反例，不是运行原 Go 函数或请求 Emby。

**建议与验收**

先完整聚合到 Series 再截前十，或采用能够证明结果正确的候选终止条件。“已有 10 部剧”不能证明剩余单集汇总后不会超过它们。补上述反例，以及已入榜剧的部分集落在窗口外的用例。

## AUD-014 / P2-2：取消请求可删除刚审核完成的订阅

**定位**

[services/api/internal/services/subscription/service.go:739](https://github.com/konghanghang/ember/blob/930b1cc9b653a1ae62c1fdc4dd0bc6dce8804c1f/services/api/internal/services/subscription/service.go#L739) 的 DeleteSubscription；批准逻辑 821–867 行；拒绝事务在 `services/api/internal/services/subscription/pending_reject.go`。

**触发与影响**

用户取消先读取 PENDING 并通过检查；另一请求随后批准并安排 MoviePilot 下发/通知；取消继续按主键 Delete，没有把 `user_id + status=PENDING` 放入删除条件，于是硬删除已批准记录。用户和管理员都可能收到成功，但异步任务继续执行，订阅记录已经无法承接错误或入库回写。并发拒绝也存在同类窗口。

这是静态可达交错，未运行数据库并发测试。管理员本来允许删除任意状态，不将该独立权限报为问题。

**建议与验收**

普通用户取消采用包含 id、user_id、PENDING 的原子条件删除，检查 RowsAffected；审批已完成时返回状态冲突。以屏障测试取消与批准/拒绝交错，确认记录、返回值及后续副作用一致。

## AUD-015 / P2-3：缺集工单旧结果覆盖最新终态

**定位**

[services/api/internal/services/mediagap/service.go:430](https://github.com/konghanghang/ember/blob/930b1cc9b653a1ae62c1fdc4dd0bc6dce8804c1f/services/api/internal/services/mediagap/service.go#L430)：SearchGap 读取后等待外部搜索，454–465 行仅按 id 回写状态；DispatchGap 的 510–515、534–545 行也无状态条件；Webhook 的 602–617 行只在内存快照检查 IGNORED。

**触发与影响**

搜索读取 MISSING → 等待响应 → Webhook 已置 INGESTED 或管理员已置 IGNORED → 搜索返回把旧快照写成 SEARCHED。已完成工单重新可下发，人工忽略失效。下发成功/失败回写及 Webhook 与忽略竞争也有相同模式。

证据为原代码的读写谓词和可达时序，未动态运行 Go 并发场景。此结论不需要推断 MoviePilot 的特殊协议。

**建议与验收**

状态更新加允许的来源状态/CAS 条件并检查 RowsAffected；晚到的搜索快照与状态分开处理，不覆盖更晚的终态。避免为等待外部请求持有长事务。用阻塞 fake MoviePilot，交错 Ignore/Webhook，并断言终态不回退。

## 待确认项：不计入上述 5 项

1. **日历“每日全量”语义**：`tvcalendar/service.go:903-919` 的全量分支仍按 30 天活跃标记筛选，lastFullSyncAt 过期本身不能恢复停更源。文档同时存在活跃优先约定，需先确认产品希望每天覆盖全部 continuing 还是仅活跃源；暂不据注释单独判 Bug。
2. **过长密码的外部副作用**：普通用户修改/重置密码先写 Emby，再做本地 bcrypt；超过 72 字节会在本地失败。仓内有长密码失败测试，但没有外部写入顺序用例。真实 Emby 是否接受这类密码本轮未验证，因此“两端已实际不一致”未证实。建议至少在外部调用前完成本地校验和 hash，并以 fake client 检查无效输入不产生远端调用。

## 验证记录与限制

本轮实际运行 61 项 Bot 离线测试：

| 模块 | 数量 | 执行方式 |
|---|---:|---|
| test_message_formatter | 14 | 直接 unittest |
| test_telegram_handler | 29 | 直接 unittest，使用仓库既有 stub |
| test_runtime_settings | 3 | 预载既有 handler 测试 stub 后运行原测试 |
| test_runtime_settings_notifications | 1 | 同上 |
| test_search_cache | 3 | 同上 |
| test_api_client | 11 | 同上 |

后四组预载 `test_telegram_handler` 提供的 httpx/dotenv/telegram 内存替身；没有安装依赖或请求外网，不能称为真实 transport 集成测试。

另执行完整日/周历史 SQL 边界及剧集候选截断的离线反例。Go 不在当前环境 PATH，Web 项目缺少 node_modules，因此未本地运行 Go、Vitest、完整构建、PostgreSQL/Redis 集成或真实第三方验证。

独立读取 [当前基线 CI](https://github.com/konghanghang/ember/actions/runs/35098742621)，Test Go API、Test Vue Web、Test Python Bot 与汇总 job 均 success；[修复 PR 的 CI](https://github.com/konghanghang/ember/actions/runs/35097682422) 同样成功。这些是已有 CI 结果，不是本轮触发；成功不能证明新增反例已有覆盖，也不能扩展为所有专项数据库并发测试通过。

## 覆盖与续扫清单

- [x] 固定最新基线，核对修复 PR、Issue 状态及 101 个变更文件的范围。
- [x] 逐项复核原 10 项问题及相关测试、跨模块调用面。
- [x] 继续检查付款创建/改价/履约、订阅创建/重提/审批/删除/异步回写。
- [x] 检查媒体缺集搜索/下发/忽略/入库、排行榜查询和候选聚合、日历增量筛选及同步入口。
- [x] 阅读播放锁、租约确认/续租、账号配置版本、成功采样及迁移相关变更；本轮未确认新增缺陷。
- [x] 为 AUD-011–015 建立 Issue #21–#25，并关联到本报告。
- [ ] 确认修复范围后，为 AUD-011–015 建立正式回归及独立修复 PR。
- [ ] 补专项 PostgreSQL advisory lock、兑换与支付交错、Redis/多实例验证。
- [ ] 确认日历全量语义及密码边界；没有足够证据前不将待确认项计入确定缺陷。
- [ ] 更广的安全审计、全部 API/DTO 和部署故障恢复仍未穷尽覆盖。

下一轮先比较目标 commit 与本基线，复核变化模块，再更新每项状态和 Issue/PR。保留本轮静态证据与运行验证的区别，不以 Issue 已关闭或 CI 绿色替代逐项复核。
