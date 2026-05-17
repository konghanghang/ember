# 多 Agent 协作指南

本指南只回答 4 件事：

1. 什么时候值得用多 agent
2. 主 agent 和 subagent 各负责什么
3. Ember 里不同任务默认该怎么拆
4. 最短怎么提需求

它不是某次任务的执行稿，也不是另一份 `AGENTS.md`。

## 1. 什么时候适合用多 agent

只有同时满足下面条件时，才值得拆：

1. 任务能切成边界清楚的子任务
2. 子任务能并行推进
3. 不需要多个 agent 同时改同一个核心文件
4. 主 agent 能控制兼容性、验收和收尾

适合：

- 新功能跨 `services/api`、`services/web`、`infrastructure/database`
- 大一点的 code review
- 代码盘点、链路梳理、文档一致性检查
- 治理类任务，需要并行盘点代码与文档

不适合：

- 单文件小改
- 根因未明的阻塞性排查
- 需求边界还没收敛
- 多个 agent 会同时改同一文件

## 2. 角色边界

### 主 agent

主 agent 负责：

- 理解需求
- 确认边界与兼容性
- 决定拆分方式
- 先定共享契约
- 最后整合、验证、同步文档

主 agent 不应该把关键决策外包给 subagent。

### subagent

subagent 只负责局部施工或定向探索：

- 在指定文件范围内实现
- 回答一个明确问题
- 不越界拍板主方案
- 不修改其他 agent 负责的文件
- 写代码时必须遵守根协作文件中的“注释与测试”规则，新增 / 重写方法要补有价值注释和必要测试

## 3. 拆分硬规则

### 3.1 先看写集，不要先看技术名词

拆分不是按“听起来像后端 / 前端”分，而是按“会改哪些文件”分。

优先选择：

- 写集分离
- 共享契约少
- 整合成本低

避免：

- 多个 agent 同时改 `docs/system-architecture.md`
- 多个 agent 同时改 `services/web/src/types/api.ts`
- 多个 agent 同时改同一个页面或 service

### 3.2 主 agent 先定共享契约

subagent 开工前，主 agent 至少先定：

- 字段名
- API 行为
- 兼容性要求
- migration 策略
- 验收口径

共享契约没定，最后基本都会返工。

### 3.3 文档和收尾不要外包

这些动作优先由主 agent 控制：

- 文档同步
- 编译验证
- 最终验收
- 归档判断

因为它们跨模块、跨目录，最容易和多个 subagent 结果打架。

## 4. Agent 使用矩阵

这一节只给 Ember 日常开发里默认最省事、最不容易撞文件的组合。

### 4.0 速查表

| 任务类型 | 默认组合 | 常见追加 agent |
|---|---|---|
| 单后端功能或修补 | 主 agent + `backend-implementer` | `integration-chain-reviewer` / `docs-sync-auditor` |
| 单前端功能或修补 | 主 agent + `web-implementer` | `api-web-contract-checker` / `docs-sync-auditor` |
| 单 Bot 功能或修补 | 主 agent + `bot-implementer` | `integration-chain-reviewer` / `docs-sync-auditor` |
| 标准前后端联动 | 主 agent + `backend-implementer` + `web-implementer` | `api-web-contract-checker` / `docs-sync-auditor` |
| schema / migration 联动 | 主 agent + `backend-implementer` + `web-implementer` + `docs-sync-auditor` | `api-web-contract-checker` |
| 外部集成链路改动 | 主 agent + `backend-implementer` + `integration-chain-reviewer` | `web-implementer` / `bot-implementer` / `docs-sync-auditor` |
| 正式 Code Review | 主 agent + `system-reviewer` | `api-web-contract-checker` / `integration-chain-reviewer` / `docs-sync-auditor` |
| 联调前契约检查 | 主 agent + `api-web-contract-checker` | `web-implementer` / `system-reviewer` |
| 文档收尾 | 主 agent + `docs-sync-auditor` | 通常不再追加 |
| 非 trivial 改动开工前起草计划 | 主 agent + `plan-drafter` | 由施工 agent 后续承接实现 |
| 根因未明的排查 | 主 agent 单独先查 | 默认不要并行开实现 agent |

### 4.1 单后端功能或修补

- 默认组合：主 agent + `backend-implementer`
- 追加 agent：触达外部集成链路时加 `integration-chain-reviewer`；触达模型、路由、配置边界时加 `docs-sync-auditor`
- 适用：`services/api` 修补、`infrastructure/database` migration、cron / webhook / Internal API / 异步通知改动

### 4.2 单前端功能或修补

- 默认组合：主 agent + `web-implementer`
- 追加 agent：接口契约有变化或可疑时加 `api-web-contract-checker`；涉及通用视觉基线或页面结构时加 `docs-sync-auditor`
- 适用：`services/web` 页面、组件、store、路由、交互修补

### 4.3 单 Bot 功能或修补

- 默认组合：主 agent + `bot-implementer`
- 追加 agent：触达 Telegram / Internal API / fire-and-forget / 租约锁等链路时加 `integration-chain-reviewer`；改了 Internal API 契约或 `services/bot/README.md` 范围时加 `docs-sync-auditor`
- 适用：`services/bot` Telegram 命令 handler、`/notify/*` 入口、Internal API 客户端、webhook / polling 模式、菜单同步、运行期设置读取

### 4.4 标准前后端联动功能

- 默认组合：主 agent + `backend-implementer` + `web-implementer`
- 追加 agent：契约复杂或历史包袱重时加 `api-web-contract-checker`；模型、路由、配置或页面职责变了时加 `docs-sync-auditor`
- 适用：新字段、新接口、表单加列表联动、用户状态流转功能

### 4.5 涉及 schema / migration 的联动功能

- 默认组合：主 agent + `backend-implementer` + `web-implementer` + `docs-sync-auditor`
- 追加 agent：接口结构同时变化时加 `api-web-contract-checker`
- 适用：GORM 模型字段、索引、约束、表结构调整，以及依赖新字段的页面联动

### 4.6 外部集成链路改动

- 默认组合：主 agent + `backend-implementer` + `integration-chain-reviewer`
- 追加 agent：前端也要改入口或状态展示时加 `web-implementer`；Bot 推送或命令链路也要改时加 `bot-implementer`；文档、部署或测试清单要同步时加 `docs-sync-auditor`
- 适用：Emby、TMDB、MoviePilot、Stripe、Telegram、webhook、polling、租约锁、notify、fire-and-forget

### 4.7 正式 Code Review

- 默认组合：主 agent + `system-reviewer`
- 追加 agent：前后端接口对齐可疑时加 `api-web-contract-checker`；外部集成风险高时加 `integration-chain-reviewer`；文档同步风险高时加 `docs-sync-auditor`
- 适用：跨模块改动、用户可见状态流转、schema / migration / init script、异步 / webhook / 第三方集成 / 配置部署改动

补充：

- 正式 review 默认先用 `system-reviewer`
- 不要一上来就把 review 拆成一堆实现 agent；review 的目标是找问题，不是抢着修
- `system-reviewer` 与 `integration-chain-reviewer` 的边界：跨多链路 / 跨子系统的端到端一致性用 `system-reviewer`；单条集成链路的深度审查用 `integration-chain-reviewer`

### 4.8 联调前契约检查

- 默认组合：主 agent + `api-web-contract-checker`
- 追加 agent：怀疑页面行为回归时加 `web-implementer`；系统级大变更时仍优先加 `system-reviewer`
- 适用：后端刚改完准备接前端，或前端接线完准备联调

### 4.9 实现完成后的文档收尾

- 默认组合：主 agent + `docs-sync-auditor`
- 适用：模型、服务逻辑、API 路由、前端结构、配置入口变更后；文档归档、README、runbook、计划文档收口

补充：

- 文档收尾不是让文档 agent 接管实现，只是查漏和指出必须同步项

### 4.10 非 trivial 改动开工前起草计划

- 默认组合：主 agent + `plan-drafter`
- 适用：跨模块改动、schema / migration / init script、用户可见状态流转、外部集成链路重设计、配置 / 部署入口变更、长链路异步处理改造，以及任何"开工前需要书面方案对齐"的场景
- 不适用：单文件修补、纯样式 / 文案 / 重命名、已经有清晰共识的小功能（直接由对应施工 agent 执行）

补充：

- `plan-drafter` 只负责起草 `docs/plan/` 下的计划文档，不施工；后续实现仍由 `backend-implementer` / `web-implementer` / `bot-implementer` 承接
- 计划落地后，主 agent 应推动用户 review，形成共识后再启动实施
- 计划文档治理细则（结构、命名、归档条件）见 `AGENTS.md` / `CLAUDE.md` "计划文档"段落与 `docs/plan/plan-template.md`

### 4.11 根因未明的排查

- 默认组合：主 agent 单独先查

不要默认并行开多个实现 agent。根因没收敛时，多个 agent 往往会同时猜同一件事，写集和问题边界都不稳定，极容易冲突。

## 5. 最短提问模板

### 模板 A：先拆分，再执行

```text
这个需求请用多 agent 处理。

需求：
[把要做的事情写清楚]

要求：
1. 先不要直接开工
2. 先结合 Ember 当前代码结构给我拆分方案
3. 说明每个 agent 的职责、文件边界、拆分依据
4. 明确哪些任务可以并行，哪些必须由主 agent 控制
5. 我确认后你再执行
```

### 模板 B：直接按模块执行

```text
这个需求请直接用多 agent 执行。

需求：
[具体功能]

拆分方式：
- 主 agent：负责方案、整合、验收、文档
- subagent A：负责 [文件或模块范围]
- subagent B：负责 [文件或模块范围]

要求：
1. 各 agent 不要改同一文件
2. 主 agent 不要把关键决策外包
3. 如果拆分不合理，先指出再调整
4. 完成后汇总每个 agent 的产出和最终整合结果
```

### 模板 C：Review 或盘点

```text
这次请用多 agent 做 review / 代码盘点。

范围：
[分支 / 功能 / 文件范围 / 要查的问题]

要求：
1. 主 agent 负责最后汇总
2. subagent 只做指定范围的 review 或探索
3. findings 优先，按严重度排序
4. 先告诉我怎么拆，再执行
```

## 6. 使用约定

正式任务可在模板后追加：

```text
补充要求：
- 遵守 Ember 项目的 AGENTS.md 和文档规则
- 涉及模型变更必须检查 SQL migration
- 涉及前端视觉改动先对照 web-design-guide
- 涉及文档边界时同步考虑 system-architecture、reference、plan、proposals、archive
- 不要让多个 agent 同时修改 docs/system-architecture.md
- 提交前先完成编译验证，再由主 agent 汇总
```

模型使用说明：

- subagent 不一定和主 agent 使用同一模型，这是正常现象
- 边界清楚、局部实现型任务，可以优先用更轻模型
- 高风险、高耦合、强推理任务，可以优先用更强模型

最短使用方式：

```text
这个需求请用多 agent 处理。
先结合 Ember 当前代码结构给我拆分方案，我确认后你再执行。
```
