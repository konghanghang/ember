# Codex Agent 与 Claude Code Agent 对比说明

> 说明：
> 1. 本文中的 “Codex agent” 指当前这类 Codex 会话环境中的 agent / subagent 使用方式。
> 2. 本文中的 “Claude Code agent” 指 Claude Code 官方文档中的 subagents / agent teams 机制。
> 3. 不同产品版本会持续演进，本文重点说明使用模型和协作方式上的差异，不把某一版 UI 或细节当成永恒规则。

## 1. 一句话结论

两者都支持把复杂任务拆给多个 AI 单元处理，但默认工作方式不同：

- Claude Code 更偏向“先定义长期可复用的角色，再按角色委派”
- Codex 更偏向“先拆当前任务，再临时派出 agent 并行执行”

所以 Claude Code 更像“项目级 agent 配置体系”，Codex 更像“任务级 agent 调度体系”。

## 2. 共同点

无论是 Codex 还是 Claude Code，agent 的核心价值都差不多：

- 把大任务拆成多个更小的子任务
- 让不同 agent 在各自上下文里工作，减少主线程污染
- 支持并行探索、并行实现、并行 review
- 通过角色边界减少“一个 agent 什么都做”的混乱

在工程实践里，它们都适合做这些事情：

- 前后端并行开发
- 代码盘点与链路梳理
- 大改动 code review
- 文档与实现一致性检查

## 3. 关键区别

### 3.1 是否强调“预定义角色”

Claude Code 更强调预定义角色。

官方文档里，subagent 可以作为文件存放在项目或用户目录下，例如：

- `.claude/agents/`
- `~/.claude/agents/`

每个 subagent 都可以有自己的：

- 名称
- 描述
- system prompt
- 工具权限
- 模型选择
- 记忆、hooks、MCP 范围等配置

这意味着 Claude Code 的思路是：

1. 先定义一批长期可复用 agent
2. 这些 agent 带着稳定角色长期存在
3. 后续任务按需要复用，甚至允许系统自动委派

Codex 更强调按任务即时拆分。

在当前这类 Codex 会话环境里，subagent 通常是临时生成的执行单元：

- 这次任务需要就创建
- 任务完成后就结束
- 不依赖项目里预先维护一整套 agent 配置目录

因此 Codex 的思路更像：

1. 先分析当前任务
2. 决定哪些子任务能并行
3. 临时创建合适的 agent
4. 收回结果，由主 agent 整合

### 3.2 自动委派 vs 人工拆分

Claude Code 官方 subagents 更强调“描述驱动委派”。

官方文档明确写到：

- Claude 会根据 subagent 的 description 判断何时委派
- Claude Code 也有内建 subagents，例如 Explore、Plan、General-purpose

这意味着 Claude Code 本身就更偏向“内置角色 + 自动判断是否调用”。

Codex 在当前环境里更强调“先拆分，再委派”。

这里的合理流程通常是：

1. 主 agent 先做任务分析
2. 明确哪些工作可以并行
3. 明确每个 subagent 的职责、文件边界和写入范围
4. 再启动 subagent

也就是说，Codex 更依赖主 agent 的主动调度，而不是把大量委派逻辑预先沉淀在角色系统里。

### 3.3 角色是“长期资产”还是“临时施工队”

Claude Code 的 subagent 更容易成为长期资产。

例如你可以在一个项目里长期维护这些角色：

- `backend-reviewer`
- `migration-checker`
- `vue-admin-ui-worker`
- `debugger`
- `code-reviewer`

这些角色会和项目一起演进。

Codex 里的 subagent 更像临时施工队。

例如这次做“兑换码备注字段”时可以临时拆成：

- worker A：后端 + migration
- worker B：前端管理页
- 主 agent：整合和验收

下次做“求片分季支持”时，又可能换成另一套拆法。

也就是说，Codex 的强项不是“长期角色库”，而是“按当前任务快速建立合适分工”。

### 3.4 主 agent 的控制强度

Codex 模式下，主 agent 的控制责任通常更重。

主 agent 需要负责：

- 定边界
- 定共享契约
- 控兼容性
- 控写集冲突
- 控收尾和验收

subagent 更像明确边界内的执行单元。

Claude Code 也需要主线程控制，但由于它更强调预定义 agent、描述驱动和自动委派，所以它在使用体验上更像“先搭体系，再调用体系”。

Codex 在实践上更像“主 agent 临场指挥多个临时小组”。

## 4. 如果放到同一个工程任务里看，区别是什么

以一个典型任务为例：

“给系统增加兑换码备注字段，涉及后端模型、SQL migration、管理端表单和列表展示。”

### 在 Claude Code 里更自然的做法

你可以长期准备这些项目级 subagents：

- `go-backend-worker`
- `sql-migration-checker`
- `vue-admin-worker`
- `docs-sync-agent`

然后遇到任务时，让 Claude Code 基于这些角色去委派。

它更像：

- 先定义角色库
- 再让这些角色处理任务

### 在 Codex 里更自然的做法

你通常不会先维护一整套长期角色库。

更自然的流程是：

1. 主 agent 先分析任务
2. 拆出本次最合理的并行单元
3. 临时创建：
   - backend worker
   - web worker
4. 主 agent 最后整合

它更像：

- 先拆这次任务
- 再临时派工

## 5. 哪种方式更适合什么团队

### Claude Code 风格更适合

- 团队长期高频使用 agent
- 希望把角色沉淀成项目资产
- 希望某些角色长期复用
- 希望通过角色配置控制工具、模型、权限、记忆

如果你们已经有稳定分工习惯，例如长期存在：

- review agent
- debugger agent
- migration validator
- frontend implementation agent

那 Claude Code 的模式会更顺手。

### Codex 风格更适合

- 任务变化大，拆分方式经常变
- 更看重主 agent 的现场判断
- 不想先维护一整套项目级 agent 配置
- 希望用一套稳定的“拆分规则”和“提问模板”替代角色库

如果你的工作方式更像：

- 先讨论这次任务怎么拆
- 再临时派几个 agent 去做
- 每次任务结束就收队

那 Codex 的模式更自然。

## 6. 对使用者来说，操作成本有什么不同

### Claude Code 的成本

前期成本更高，后期复用更强。

你通常需要：

- 先定义 subagent
- 写清 description
- 写 system prompt
- 控制工具权限
- 管理 agent 文件

但定义好之后，后续复用会比较顺。

### Codex 的成本

前期配置更轻，单次拆分要求更高。

你不一定要先维护 agent 文件，但通常需要：

- 说明这次是否要用多 agent
- 让主 agent 先做拆分方案
- 明确每个 subagent 的任务边界

也就是说，Codex 把复杂度从“预先配角色”转移到了“任务当场拆分”。

## 7. 两者在工程协作上的本质差异

把话说透一点：

- Claude Code 的 agent 体系更像“组织架构”
- Codex 的 agent 体系更像“项目经理临时派工”

前者强调：

- 长期角色
- 角色复用
- 配置沉淀
- 自动委派

后者强调：

- 任务拆分
- 当前边界
- 临时并行
- 主 agent 集中整合

## 8. 在 Ember 这种项目里，该怎么理解这个区别

如果是 Ember 这种 monorepo 项目：

- Go API
- Vue Web
- Python Bot
- SQL migration
- docs 同步

那么 Claude Code 风格更适合沉淀这种长期角色：

- Go backend agent
- Vue admin UI agent
- Bot integration agent
- SQL migration validator
- Docs sync reviewer

而 Codex 风格更适合沉淀这种长期规则：

- 哪些任务适合多 agent
- 怎么按写集拆分
- 主 agent 负责什么
- subagent 不该碰什么
- 什么时候必须先出拆分方案

也就是说：

- Claude Code 更适合沉淀“角色库”
- Codex 更适合沉淀“拆分规则”

## 9. 最实用的判断方式

如果你想快速判断一件事该怎么做，可以直接用这个标准：

### 如果你在用 Claude Code

先问：

- 这个任务是否已经有现成角色可以接？
- 要不要把这类工作沉淀成长期 subagent？

### 如果你在用 Codex

先问：

- 这次任务能不能拆成几个互不冲突的写集？
- 主 agent 需不需要先出拆分方案？
- subagent 是否会改到同一文件？

## 10. 最后结论

两者不是谁高谁低，而是默认工作流不同。

Claude Code 更像：

- “先定义 agent，再反复调用”

Codex 更像：

- “先拆当前任务，再临时派 agent”

如果你希望长期维护一套项目级角色体系，Claude Code 的思路更顺。

如果你希望先把工程拆分规则跑顺，再按任务灵活调度，Codex 的思路更顺。

在实际工程里，最成熟的做法通常不是二选一，而是把两种思路结合：

- 长期沉淀少量稳定角色
- 同时保留按任务即时拆分的能力

## 11. 补充说明

本文写作时参考了 Claude Code 官方关于 subagents 的说明，包括：

- custom subagents
- built-in subagents
- project/user scope
- automatic delegation

官方文档入口：

- https://code.claude.com/docs/en/sub-agents
- https://code.claude.com/docs/en/settings
