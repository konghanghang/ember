# 前端工程质量收口方案

> 状态：已完成并归档（未执行真实浏览器验收）
> 负责人：Ember
> 更新时间：2026-08-31

## 背景

2026-07-26 对 `services/web`（约 2.3 万行、94 个源文件）做了一轮系统性评审（构建链路、状态/API 层、7 个大型视图），结论：

- 路由懒加载、Element Plus 按需异步注册、vendor 分包、跨 tab 登录同步、401 竞态锁等主线是健康的。
- 真正的短板是工程纪律：构建链路零类型检查、两处对外契约违反 AGENTS.md 硬约束 #2、工具函数复制粘贴成规模、一批无人收尾的死资源。
- 评审发现一个真实功能 bug（扫描按钮永久卡死）和一个响应竞态。

如果不收口：类型错误可一路进生产镜像；契约违规随接口增多持续放大；复制粘贴的工具函数改一处必漏其他处。

## 目标

1. 把 `vue-tsc` 类型检查接入构建与 CI，并修通 tsconfig 引用。
2. 修复 P1 功能 bug 与 P2 契约违规（含后端 handler 映射）。
3. 消灭已成规模的工具函数复制粘贴，沉淀到 `src/utils/`。
4. 清理死资源与死依赖。

## 非目标

- 不做框架级升级（Vite 5→7、Tailwind 3→4 另行排期）。
- 不一次性拆分所有巨型视图；拆分边界已在本文档记录，随后续功能迭代顺手做。
- 不重构 Element Plus 注册机制（现状正确，仅补防回归测试）。
- 前端实现必须遵守 Ember 风格；设计与交互基线以 [docs/reference/web-design-guide.md](../../../reference/web-design-guide.md) 为准，本方案不允许偏离。

## 立项时事实（2026-07-26）

以下内容保留为实施前基线；完成态以文末进度和归档结论为准。

- 相关服务：`services/web`；契约违规涉及后端 `services/api/internal/handlers/media.go`、`tmdb.go`。
- 构建现状：`package.json` 的 `build` 仅 `vite build`（转译不做类型检查）；`vue-tsc@1.8.27` 躺在 devDependencies 从未被脚本/CI 调用；`tsconfig.app.json` extends 未安装的 `@vue/tsconfig`，手动跑 `vue-tsc` 直接报错；根 `tsconfig.json` 自包含，两套配置并存。
- 契约现状：`MediaStats` 透传 Emby PascalCase 字段（`MovieCount` 等，全项目唯一例外）；`/tmdb/search` 返回 `{results,total}` 而非 `{data,total}`。
- 组件基线现状：`src/components/ember/` 基础组件复用情况整体良好；列表接口除 `/tmdb/search` 外均走 `data` 字段。

## 问题清单（评审实证）

### P1

- **P1-1 构建零类型检查**：`build` 无 `vue-tsc`；`tsconfig.app.json:2` extends 缺失依赖；CI `scripts/test/web.sh:35` 只跑 `npm run build`。
- **P1-2 扫描按钮永久卡死**：`MediaGapsView.vue:737-752`，`handleScan` 抛错时 `scanning` 无复位路径，按钮永久 disabled，须刷新页面。

### P2

- **P2-1 PascalCase 泄漏**：`types/api.ts:1112-1116` ← 后端 `integrations/emby/emby.go:413-417` 经 `handlers/media.go:101` 透传；`DashboardView.vue:39,315` 已被迫写 `stats.MovieCount`。
- **P2-2 `/tmdb/search` 未用 `data` 字段**：`types/api.ts:1161-1164` ← 后端 `handlers/tmdb.go:248-251`（上游 TMDB 的 `total_results` 不动）。
- **P2-3 request 层类型签名失真**：`api/request.ts:70` 拦截器运行时解包 `response.data`，类型仍声称 `AxiosResponse<T>`，靠 `as unknown as` 强转维持（`api/console.ts:204`、`api/admin.ts:292,713`）。
- **P2-4 错误 toast 双弹**：拦截器（`api/request.ts:90-92`）与 12 个视图 catch 内 `ElMessage.error`（如 `RankingsView.vue:179,197,220,246`）重复提示。
- **P2-5 `role`/`passwordResetRequired` 双份存储**：`store/auth.ts:53,103` 与 `store/user.ts:11` 手工同步，`updatePassword` 同字段写两遍。
- **P2-6 模块级循环依赖**：`api/request.ts:5` → `store/auth.ts:3` → `api/auth.ts:1` → `api/request.ts`，靠拦截器回调延迟调用侥幸可用。
- **P2-7 `fetchData` 无竞态防护**：`MediaGapsView.vue:638-688`，grouped/table 响应乱序可渲染空页；同项目 `SubscriptionsView.vue:64,103-125` 已有 `fetchRequestToken` 解法未复用。
- **P2-8 工具函数复制粘贴成规模**：
  - `policySyncStatus` 状态映射 3 份（`UsersView.vue:722-747`、`PlanGroupsView.vue:141-159`、`AccountCenterView.vue:153-157,852-873`）
  - `formatCandidateSize` 2 份且规则不同（`MediaGapsView.vue:473-502` vs `SubscriptionsView.vue:525-534`）
  - `isConflictError` 2 份（`AccountCenterView.vue:166-171`、`PlanGroupsView.vue:128-133`）
  - `copyToClipboard` 3 份（`AccountCenterView.vue:545-552`、`DashboardView.vue:120-127`、`SettingsView.vue:211-220`）
  - `isMessageBoxCancel` 2 份（`MediaGapsView.vue:210`、`UsersView.vue:122`）
  - 日期格式化 4 套，与 `utils/date.ts:93-99` 冲突（`MediaGapsView.vue:270-292`、`SubscriptionsView.vue:505-519`、`TVCalendarView.vue:99-126`）
- **P2-9 分页 size-change 不重置页码**：`UsersView.vue:1033`、`SubscriptionsView.vue:867`，会请求越界空页；`MediaGapsView.vue:706-710` 行为正确。
- **P2-10 声明字体从未加载**：`Plus Jakarta Sans`/`JetBrains Mono`（`base.css:36`、`tailwind.config.js:29-30`）全项目无 `@font-face`/link，静默回退 system-ui。
- **P2-11 死资源进产物**：`public/favicon.ico` 364KB（首屏加载）、`favicon.png` 524KB 无引用、`vite.svg` 无引用，public 整目录进 dist 白增约 0.9MB。

### P3（记录备查，随迭代顺手收口）

- 巨型组件拆分边界：MediaGapsView 拆搜索弹窗（模板 1380-1526 + 逻辑 755-835）、聚合剧卡片（1084-1251 + 313-438）、候选归一化纯函数群（463-595）；UsersView 三个弹窗；SubscriptionsView 两个弹窗 + 海报卡；PlanGroupsView 两个几乎逐字相同的表单弹窗合并为一个；SettingsView 配置行渲染（674-872）。
- 模板渲染期重复计算：`UsersView.vue:879-940`（每行 11 次状态函数）、`MediaGapsView.vue:1160-1243`（每卡 10+ 次全量 filter）、`SubscriptionsView.vue:823-830`。
- 死代码：`LibraryView.vue`（路由已 redirect）、`AccountCenterView.vue:49` 永不生效的 loading、`PlanGroupsView.vue:44-48` 未使用的 embedded prop、sass 依赖（全项目零 scss）、`test:unit`/`test:component` 脚本名不副实。
- 错误分支小坑：`UsersView.vue:314` 非法日期 `toISOString()` 抛 RangeError；`PlanGroupsView.vue:238-240` 轮询遇瞬时错误永久停摆；`SubscriptionsView.vue:449-456` 二次提交失败逃出 catch；`AccountCenterView.vue:408-424` 双空串密码放行。
- auth 细节：跨 tab 同步把 `event.key === null` 误判为登出（`store/auth.ts:189-194`）；`restoreAuth` 内存态残留分支致"幽灵登录"（`store/auth.ts:144-146`）；"清空所有 store"逻辑重复 5 处；`loadProtectionConfig` 永久缓存无失效路径（`store/auth.ts:64-71`）。
- 类型零散：约 15 处内联 `{ data: T[] }` 匿名返回类型，缺 `ListResponse<T>` 泛型；`MediaGapItem.searchSnapshot` 的 `T | string | null` 联合类型需先核对后端序列化行为再收口。
- Element Plus 手工 async 注册清单无防回归测试，新增组件忘注册时生产静默渲染失败。
- `utils/date.ts` 手写相对时间无测试，`formatRelative` 在 59.5 分钟边界显示跳变。

## 方案设计

### 批次划分

按"安全网先行、契约其次、杂物一次清、拆分随迭代"分四批：

**批次 1：类型检查安全网（P1-1、P2-3 前置）**
- 安装 `@vue/tsconfig`（或 `tsconfig.app.json` 改为 extends `./tsconfig.json`，二选一，优先对齐 Vue 官方 preset）。
- `vue-tsc` 升级到与 TS 5.9 配套的 2.x/3.x；`build` 改为 `vue-tsc --noEmit && vite build`；`scripts/test/web.sh` 同步。
- 给 request 层包 `request<T>(config): Promise<T>` 具名函数，把解包事实编码进类型，替换现有强转。
- 预期会暴露一批存量类型错误，本批次一并修平，不允许用 `any` 压。

**批次 2：功能 bug 与契约（P1-2、P2-1、P2-2、P2-4、P2-7、P2-9）**
- P1-2：catch 中复位 `scanning`，或改为以 `scanStatus.running` 单一事实源驱动按钮态。
- P2-1：后端 `handlers/media.go` 增加 camelCase DTO 映射（同文件 `LatestMediaItem` 已有先例），前端类型与 DashboardView 同步；属对外字段变更，需核对所有调用方。
- P2-2：后端 `tmdb.go` 改返 `{data,total}`，前端 `TmdbSearchResponse` 同步。
- P2-4：统一为"视图侧 `silent: true` + 自定义文案"或"只走拦截器"，二选一后全站收口。
- P2-7：照搬 `SubscriptionsView` 的请求令牌守卫。
- P2-9：size-change 统一重置到第 1 页。

**批次 3：工具沉淀与死资源清理（P2-8、P2-10、P2-11、P3 死代码）**
- 抽 `src/utils/policy-sync.ts`（状态映射）、`clipboard.ts`、`api-error.ts`（isConflictError/isMessageBoxCancel）、`format.ts`（formatCandidateSize）；MediaGapsView 日期函数换用 `@/utils/date`，日历周界函数沉淀进 utils；均补 `go test` 对应的 Vitest。
- 字体二选一：删声明承认 system-ui 基线，或自托管 woff2 + preload。默认倾向删声明（符合实用主义，且设计规范未要求必须自定义字体时以系统字体为准）；若保留字体需同步 web-design-guide。
- favicon.ico 压缩到 <50KB 或改用已有 favicon.svg；删除 favicon.png、vite.svg、vue.svg、LibraryView.vue、sass 依赖；`test:unit`/`test:component` 改名或删除。

**批次 4：状态层收口（P2-5、P2-6、P3 auth 项）**
- request.ts 改为注入 token getter 与 unauthorized 回调（main.ts 装配），斩断循环依赖。
- auth store 的 `role`/`passwordResetRequired` 改为从 user store 派生（computed），消除双份存储。
- 修跨 tab `key === null` 误判、`restoreAuth` 幽灵登录残留分支；提取 `resetAllStores()`。
- 该批次涉及登录态与用户状态流转，按硬约束 #3 必须先补特征测试锁住当前行为，再做变更。

**不在本方案内、随后续迭代顺手做**：P3 巨型视图拆分（边界已记录）、模板渲染期重复计算优化、Element Plus 注册防回归测试、`ListResponse<T>` 泛型统一。

### 关键约束

- P2-1、P2-2 是对外 API 字段变更，前端类型、视图、测试快照与后端 handler 必须同一提交内同步，禁止只改一侧。
- 所有改动不破坏未提及的用户可见行为（Never break userspace）。
- 新抽的 utils 必须补 Vitest；批次 4 涉及状态流转必须先特征测试。

## 影响范围

- API：有。`handlers/media.go`、`handlers/tmdb.go` 响应字段映射。
- Web：有。构建脚本、request 层、store、7 个大型视图、utils、public 资源。
- Bot：有。Internal MediaStats 与 TMDB 搜索复用同一 handler，Bot `/count`、`/search` 已同步消费 camelCase 和 `data` 合同。
- 配置/部署：无（nginx 缓存策略已健康，不动）。
- 文档：批次 2 契约变更后同步 `docs/system-architecture.md` 与 `docs/reference/api-response-standard.md`（若该标准文档已有 MediaStats 例外记录则需移除例外）；字体决策若保留需同步 `docs/reference/web-design-guide.md`。

## 验证方式

### 编译/测试

- `cd services/web && npm run build`（批次 1 后内含 `vue-tsc --noEmit`）
- `cd services/web && npm run test`
- `cd services/api && go test ./... && go build ./... && go vet ./...`（批次 2）

### 手工验证

- P1-2：触发扫描失败后按钮恢复可点（以测试或本地复现为准，不真实调用 Emby）。
- 批次 3：构建产物中不再包含 favicon.png/vite.svg，dist 体积下降约 0.9MB。

## 落地后文档处理

- 全部批次与 follow-up 代码已完成，稳定结论已经提炼；按用户明确决定，本文档已移入 `docs/archive/plan/architecture/`。
- 批次 1 的“build 必过 vue-tsc”结论已沉淀到 `docs/runbooks/testing.md`。
- 批次 2 的 MediaStats camelCase 与 TMDB `data` 合同已同步到 `docs/system-architecture.md` 和 API 端点目录。

## 进度记录

- 2026-07-26：完成三路并行评审（构建/状态层/大型视图），问题清单定稿，等待排期修复。
- 2026-07-26：布局与设计维度的问题另立 [前端页面布局与设计收口方案](../console-admin/web-layout-design-improvement.md) 收口；其中日期/时间格式统一（该文档 S11）与本方案 P2-8 同源，执行时合并处理，避免两次触碰同一批文件。该方案已于 2026-08-30 归档。
- 2026-07-26：前端部分全部实施完成，`vue-tsc --noEmit` 全项目 0 错误、`npm run build` 通过、`npm run test` 155 passed / 3 skipped（跳过项为 EMBER_WEB_RUN_INTEGRATION 门控集成 spec）。
  - 批次 1（类型安全网）：完成。安装 `@vue/tsconfig@0.9.1`、`vue-tsc@3.3.8`；`build` 接入 `vue-tsc --noEmit`，`scripts/test/web.sh` 加独立 typecheck 步骤；tsconfig 双套并存收口；首次暴露的 103 个存量类型错误零 `any` 修平；新增 `request<T>(config): Promise<T>` 具名函数，约 80 个调用点从类型谎言变真实类型。
  - 批次 2（功能 bug 与契约）：前端项完成，**两处前后端联动项跳过待排期**——P2-1（MediaStats PascalCase）、P2-2（`/tmdb/search` 改 `data` 字段）均需后端 handler 同改，未实施。其余前端项完成：P1-2 扫描按钮卡死（`scanStatus.running` 单一事实源，补 spec）、P2-4 toast 双弹（全站 catch 删 `ElMessage.error` 走拦截器，差异化文案改 `silent`）、P2-7 fetchData 竞态（请求令牌守卫，补 spec）、P2-9 分页 size-change 重置第 1 页。
  - 批次 3（工具沉淀 + 死资源）：完成。新增 `utils/policy-sync.ts`（三份语义差异实证后沉淀为具名变体）、`clipboard.ts`、`api-error.ts`、`format.ts`（两份规则不同沉淀为两个具名函数）；S11 日期函数全部收口进 `utils/date.ts` 并用测试锁定等价性；字体声明删（规范已改 system-ui 基线）；favicon 364KB→6.6KB；删 favicon.png/vite.svg/vue.svg/LibraryView.vue/sass 依赖/`test:unit`/`test:component` 别名/AccountCenter 死 loading/PlanGroups 死 prop；新 utils 补 48 个测试。
  - 批次 4（状态层收口）：完成。先补 31 个特征测试（auth/user/request 三 spec）做安全网；P2-6 循环依赖（request.ts 改 DI `setupRequestInterceptors`，main.ts 装配，斩 `request→store/auth→api/auth→request` 环）；P2-5 单一事实源（`role`/`passwordResetRequired` 改从 `userStore.profile` computed 派生）；P3 auth 四项（key===null 误判、restoreAuth 幽灵登录、提取 `store/reset.ts` 的 `resetAllStores()`、protectionConfig TTL）；对外 API 字段名/方法签名/localStorage key 全部不变。整合阶段顺带清除 G 留下的 `setSessionFromProfile` no-op 死壳（定义+导出+router 单处调用）。
  - 延后项（随迭代做）：P3 巨型组件拆分、模板渲染期重复计算优化、Element Plus 注册防回归测试、`ListResponse<T>` 泛型统一；EmberMetricCard 图标 slot（需扩组件契约）；媒体库 409 toast 双弹（需 api 层 `silent`）。
  - 已知 follow-up（Codex review 发现，待修，不阻塞本次提交）：
    - DevicesView 删除了唯一的全量刷新按钮（S4「有查询就删刷新」决策对多数据区页面不适用）：查询只刷新设备列表，stats/黑名单/操作日志仅在挂载时加载，多管理员并发改动后会显示旧数据。修法：保留全量刷新入口，或让查询同时调用 `refreshAll`。
    - register 切号跨 tab 广播登出：`resetAllStores()` 先删旧 token（广播 `signed-out`）再写新 token（广播 `updated`），其他 tab 会先弹登出跳登录页、再弹切号跳 dashboard，两次竞争。修法：register 成功后只清 user/console 数据并原子替换 token，不走 `resetAllStores`。
- 2026-08-30：进度复核确认 P2-1 与 P2-2 仍未实施：`MediaStats` 仍使用 `MovieCount/SeriesCount/EpisodeCount`，`/tmdb/search` 仍返回 `results`。这两项属于 API/Web 同步合同变更，未完成前本计划继续保留在 `docs/plan/architecture/`。
- 2026-08-31：完成四项剩余代码与自动化验证，等待受控手工验收后归档。
  - P2-1：API handler 使用 DTO 将 Emby `MovieCount/SeriesCount/EpisodeCount` 映射为 Ember 对外 `movieCount/seriesCount/episodeCount`；Web Dashboard 与 Bot `/count` 同步新合同，Emby integration 原协议保持不变。
  - P2-2：公开与 Internal `/tmdb/search` 统一返回 `{data,total}`；Web 新建订阅与 Bot `/search` 同步调用方，Bot 原有 movie/tv 过滤与序号语义保持不变。
  - follow-up：注册成功切号不再执行删除 token 的登出式清场，只清 user/console 派生态后写入新会话；设备页在页头恢复独立全量刷新，查询/重置仍只负责筛选设备列表，并补 loading/disabled 反馈。
  - 构建收口：`vue-tsc` 暴露的 4 处 Element Plus `prompt` 联合返回类型注解已用成功分支运行时合同收窄，不改变交互行为。
  - 自动化验证：API `go test ./...`、`go vet ./...`、`go build ./...` 通过；Web `npm run test` 为 181 passed / 3 skipped，`npm run build` 通过；Bot `pytest tests` 为 49 passed，`py_compile main.py` 通过；`git diff --check` 通过。所有上游均为 fake 或未触发，没有启动服务或真实请求 Emby、TMDB、Telegram。
  - 未执行项：真实浏览器双标签页注册切号与设备页全量刷新仍需受控手工验收；当前自动化结果不表述为 E2E 或真实 Emby 验证。
- 2026-08-31：用户明确决定直接归档；未执行的真实浏览器验收作为历史限制保留，不再作为归档阻塞项，也不补写为“已通过”。

## 归档结论

- 四个实施批次、两处 API 合同项与两项 follow-up 均已完成，现行代码、测试、系统架构、API 端点目录和 Web 设计规范已接管稳定事实，本文只保留历史追溯价值。
- 真实浏览器双标签页注册切号与设备页全量刷新未执行；本归档不声称 E2E、真实 Emby 或人工交互验收通过。
- 用户已明确接受上述验证限制并要求归档；如后续复现交互问题，应新建缺陷或计划，不再恢复本历史方案为现行实施稿。
- P3 巨型组件拆分、模板计算优化、Element Plus 注册防回归和 `ListResponse<T>` 统一仍按原约定随后续功能迭代处理，不属于本轮验收或归档阻塞项。
