# 多 Agent 协作指南

本指南说明在 Ember 项目中，什么时候适合使用多 agent，怎么提需求，以及主 agent 和 subagent 应该怎样分工。

它不是某次任务的执行稿，而是长期可复用的协作模板。

## 1. 什么时候适合用多 agent

只有同时满足下面条件时，才值得用多 agent：

1. 任务可以拆成边界清楚的子任务
2. 各子任务可以并行推进
3. 不需要多个 agent 同时修改同一个核心文件
4. 主 agent 能明确控制兼容性、验收和收尾

典型适用场景：

- 新功能跨 `services/api`、`services/web`、`infrastructure/database`
- 大一点的 code review，需要分模块检查
- 代码盘点、链路梳理、文档一致性检查
- 治理类任务，需要并行盘点代码与文档

不适用场景：

- 单文件小改
- 根因未明的阻塞性排查
- 需求边界尚未收敛
- 多个 agent 会同时改同一文件

## 2. 主 agent 和 subagent 的职责

### 主 agent

主 agent 负责：

- 理解需求
- 确认边界与兼容性
- 决定拆分方式
- 控制共享契约
- 最后整合、验证、同步文档

主 agent 不应该把关键决策外包给 subagent。

### subagent

subagent 只负责局部施工或定向探索：

- 在指定文件范围内实现
- 回答一个明确问题
- 不越界拍板主方案
- 不修改其他 agent 负责的文件

## 3. Ember 项目的推荐拆分方式

### 功能开发

推荐拆法：

- 主 agent：方案、兼容性、整合、验收、文档
- backend worker：`services/api` + `infrastructure/database`
- web worker：`services/web`

适合：

- 新字段
- 新接口
- 表单 + 列表 +管理页联动
- 涉及 SQL migration 的功能

### Code Review

推荐拆法：

- 主 agent：汇总 review 结论
- explorer A：API / service / migration 风险
- explorer B：Web 页面与交互回归
- explorer C：文档同步、架构文档、README 漏项

### 代码盘点 / 链路梳理

推荐拆法：

- 主 agent：定义问题与汇总结论
- explorer A：路由入口与 handler
- explorer B：service 链路
- explorer C：前端调用链与文档状态

## 4. 拆分时必须遵守的原则

### 4.1 先看写集，不要先看技术名词

拆分不是按“听起来像后端 / 前端”分，而是按“会改哪些文件”分。

优先选择：

- 写集分离
- 共享契约少
- 整合成本低

避免：

- 多个 agent 同时改 `docs/system-architecture.md`
- 多个 agent 同时改 `services/web/src/types/api.ts`
- 多个 agent 同时改同一个页面或 service

### 4.2 主 agent 先定共享契约

在 subagent 开工前，主 agent 至少要先定：

- 字段名
- API 行为
- 兼容性要求
- migration 策略
- 验收口径

共享契约没定，subagent 只能各写各的，最后必然返工。

### 4.3 文档和收尾不要外包

文档同步、最终验收、编译验证、归档判断应优先由主 agent 控制。

因为这些动作跨模块、跨目录，容易和多个 subagent 结果发生冲突。

## 5. Ember 项目标准提问模板

### 模板 A：先拆分，再执行

适合还不确定怎么分工的时候。

```text
这个需求请用多 agent 来处理。

需求：
[把要做的事情写清楚]

要求：
1. 先不要直接开工
2. 先结合 Ember 当前代码结构，给我拆分主 agent 和 subagent 的方案
3. 说明每个 agent 的职责、文件边界、拆分依据
4. 明确哪些任务可以并行，哪些必须由主 agent 控制
5. 我确认后你再执行
```

### 模板 B：前后端并行开发

适合有明确 `api + web` 边界的功能。

```text
这个需求请用多 agent 处理，按前后端拆。

需求：
[功能名称 / 目标]

要求：
1. 主 agent 先确认行为边界、兼容性、文档同步点
2. 一个 subagent 负责 backend
3. 一个 subagent 负责 web
4. 不要让多个 agent 修改同一个核心文件
5. 主 agent 最后负责整合、验证和文档更新
6. 先给我拆分方案，我确认后再执行
```

### 模板 C：后端 + SQL migration + 前端

适合涉及 schema 变更的任务。

```text
这个需求请用多 agent 处理，并把 SQL migration 单独纳入考虑。

需求：
[具体功能]

要求：
1. 主 agent 先确认数据模型变更、兼容性和 migration 策略
2. backend subagent 负责 services/api 和 infrastructure/database
3. web subagent 负责 services/web
4. 改模型必须补 SQL migration
5. 主 agent 最后检查 API、前端、migration、文档是否一致
6. 先输出拆分方案和文件边界，再执行
```

### 模板 D：多 agent 做 code review

适合大改动 review。

```text
这次请用多 agent 做 review。

review 范围：
[分支 / 功能 / 文件范围]

要求：
1. 主 agent 负责最后汇总
2. 按模块拆多个 subagent 分别 review
3. review 重点放在：
   - 行为回归
   - 兼容性风险
   - 数据模型 / migration 漏项
   - 文档同步遗漏
4. findings 优先，按严重度排序
5. 先告诉我打算怎么拆 review，再执行
```

### 模板 E：多 agent 做代码盘点

适合排查前的探索。

```text
这个问题先用多 agent 做代码盘点，不要直接改代码。

问题：
[你想搞清楚什么]

要求：
1. 主 agent 先定义要查的问题
2. subagent 只做定向代码探索
3. 分别回答：
   - 相关入口在哪里
   - 核心服务链路在哪里
   - 前端调用链在哪里
   - 文档是否已经同步
4. 最后由主 agent 汇总成结论和下一步建议
```

### 模板 F：你已经想好怎么拆了

适合已经有明确分工的时候。

```text
这个需求请直接用多 agent 执行。

需求：
[具体功能]

拆分方式：
- 主 agent：负责 [方案 / 整合 / 验收 / 文档]
- subagent A：负责 [文件或模块范围]
- subagent B：负责 [文件或模块范围]
- subagent C：负责 [如果有]

要求：
1. 各 agent 不要改同一文件
2. 主 agent 不要把关键决策外包
3. 执行前如果发现拆分不合理，先指出再调整
4. 完成后汇总每个 agent 的产出和最终整合结果
```

### 模板 G：最省事版本

适合只想先看看是否值得拆。

```text
这个需求如果适合多 agent，就按 Ember 当前代码结构帮我拆。
先给我主 agent 和 subagent 的分工方案，我确认后再执行。
需求是：
[一句话描述需求]
```

## 6. Ember 项目专用补充要求

如果任务比较正式，建议在模板后面追加：

```text
补充要求：
- 遵守 Ember 项目的 AGENTS.md 和文档规则
- 涉及模型变更必须检查 SQL migration
- 涉及前端视觉改动先对照 web-design-guide
- 涉及文档边界时同步考虑 system-architecture、reference、plan、proposals、archive
- 不要让多个 agent 同时修改 docs/system-architecture.md
- 提交前先完成编译验证，再由主 agent 汇总
```

## 7. 推荐使用方式

如果只是想开始用，不用每次自己手写完整任务书。

最实用的做法是：

1. 你明确说“这次请用多 agent”
2. 你给出目标和大致拆分方向
3. 由主 agent 先产出拆分方案
4. 你确认后，再正式执行

最常用的两句就够了：

```text
这个需求请用多 agent 处理。
先结合 Ember 当前代码结构给我拆分方案，我确认后你再执行。
```

或者：

```text
这个需求如果适合多 agent，就按 Ember 当前代码结构帮我拆。
```
