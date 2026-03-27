# 多 Agent 执行复盘：兑换码备注字段

> 历史案例：本文档记录一次真实的多 agent 协作演示。
> 它的价值在于复盘拆分方式与收尾方法，不代表新的现行规范。

## 背景

本次选择的演示任务是：

- 为 `redemption_codes` 增加 `notes` 字段
- 后端支持创建、批量创建、更新、查询返回 `notes`
- 管理端兑换码页面支持创建、编辑和列表展示备注
- 补齐 SQL migration

对应计划文档：

- `docs/plan/embypulse-features/p2-code-notes.md`

## 为什么选这个任务做多 Agent 演示

这个任务适合用多 agent，不是因为它“够大”，而是因为它满足了适合并行的几个条件：

1. 写集天然分离
   - 后端修改集中在 `services/api` 和 `infrastructure/database`
   - 前端修改集中在 `services/web`

2. 共享契约很少
   - 只需要统一一个核心字段：`notes`
   - 字段长度、是否可选、是否支持筛选都能提前定死

3. 主线程容易收尾
   - 文档同步点少
   - 验证路径清楚
   - 不需要多个 agent 同时修改同一个核心文件

因此它非常适合作为 Ember 项目第一次多 agent 协作案例。

## 共享契约

在 subagent 开工前，主 agent 先锁定了这次的共享契约：

- 字段名固定为 `notes`
- 本次不做“按备注筛选”
- `notes` 允许为空，不填时保持旧行为
- 最大长度 `500`
- 必须补 SQL migration

这一步是整个多 agent 协作里最关键的动作。  
如果没有先定共享契约，前后端就会各自猜测，最后很容易返工。

## 拆分方案

### 主 agent

负责：

- 明确功能边界
- 决定兼容性策略
- 锁定共享契约
- 控制文档同步
- 最后整合、验证和收尾

### backend worker

负责写入范围：

- `services/api/internal/models/redemption_code.go`
- `services/api/internal/services/redemption/types.go`
- `services/api/internal/services/redemption/code_service.go`
- `services/api/internal/handlers/redemption_code.go`
- `infrastructure/database/20260327_01_add_redemption_code_notes.sql`

目标：

- 模型增加 `notes`
- 请求结构支持 `notes`
- 创建 / 批量创建 / 更新 / 查询返回支持 `notes`
- 补齐幂等 migration

### web worker

负责写入范围：

- `services/web/src/types/api.ts`
- `services/web/src/api/admin.ts`
- `services/web/src/views/admin/RedemptionCodesView.vue`

目标：

- 前端类型支持 `notes`
- 管理页创建弹窗支持备注输入
- 编辑弹窗支持备注修改
- 列表展示备注

## 实际执行过程

### 第一步：主线程先锁边界

主 agent 没有立刻改代码，而是先做了三件事：

1. 确认本次只做备注字段，不扩展筛选
2. 确认 migration 是必须项
3. 确认前后端共享契约只有 `notes`

### 第二步：两个 worker 并行施工

backend worker 完成了：

- `RedemptionCode` 模型新增 `Notes`
- 创建 / 更新请求结构新增 `notes`
- 服务层在创建和更新时写入 `notes`
- 补 `20260327_01_add_redemption_code_notes.sql`

web worker 完成了：

- 前端类型新增 `notes`
- 创建表单增加备注输入
- 编辑表单增加备注输入
- 列表增加备注列展示

### 第三步：主线程做整合收口

主线程没有重复做 worker 已负责的工作，而是只做整合动作：

1. 检查前后端契约是否一致
2. 修正交叉细节
3. 更新 `docs/system-architecture.md`
4. 运行测试与构建验证

## 主线程整合时发现的问题

这次最典型的一个问题是：

- 前端最初把 `notes` 类型写成了 `string | null`
- 后端实际按 `string` 处理，没有把 `null` 当正式契约的一部分

如果主线程不做最后整合，这种错位很容易留在代码里。

最终收口方式是：

- 前端类型统一收紧为 `notes?: string`
- 页面层仍允许“留空不填”，但不再把 `null` 当成核心契约

另一个收口点是编辑弹窗文案：

- 原本容易让人误解成“留空保持原备注”
- 但当前真实行为是“清空后保存会移除原备注”
- 该文案最终由主线程修正为与实际行为一致

这个例子说明：

- 多 agent 并行并不意味着主线程可以缺席
- 真正的价值在于并行施工 + 主线程收口，而不是“把任务分出去就结束”

## 验证结果

本次收尾阶段运行了这些验证：

- `cd services/api && go test ./internal/services/redemption ./internal/handlers`
- `cd services/api && go build ./...`
- `cd services/web && npm run build`

结果：

- Go 测试通过
- Go 构建通过
- Web 构建通过

## 文档同步

本次同步更新了：

- `docs/system-architecture.md`

同步内容包括：

- `RedemptionCode` 模型新增 `notes`
- `RedemptionCodeService` 创建/批量创建/查询返回能力补充 `notes`
- 管理端兑换码数据源说明补充 `notes`

## 这次拆分为什么有效

核心原因不是“开了两个 agent”，而是拆分方式正确：

1. 写集分离
2. 共享契约少
3. 主线程不抢子任务
4. 主线程最后负责整合和收尾

如果其中任意一项不成立，这次并行就不会顺。

## 哪些任务可以复用这套拆法

这套拆法特别适合：

- 新增字段
- 涉及模型 + SQL migration + 管理页展示
- 前后端契约单一且明确
- 文档同步点不多的功能

例如：

- 订阅扩展字段
- 兑换码扩展字段
- 管理端列表加新属性

## 哪些任务不适合照搬

不适合直接照搬这套拆法的任务：

- 根因未明的复杂 bug 排查
- 多个 agent 必须同时改同一个核心文件
- 需求边界尚未确定
- 文档与代码都需要大量同步、且共享契约复杂的重构

这类任务应先由主 agent 自己收敛边界，再决定是否值得并行。

## 最终结论

这次案例证明了一件事：

在 Ember 项目里，多 agent 最有效的使用方式不是“多开几个 agent”，而是：

1. 主 agent 先锁定共享契约
2. subagent 在清晰写集里并行施工
3. 主 agent 最后统一收口、验证、同步文档

如果少了第 3 步，多 agent 只会把错误并行放大。  
如果少了第 1 步，多 agent 只会各写各的。  
真正可复用的不是“开了两个 worker”，而是这套拆分和收尾方法。
