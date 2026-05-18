# 参考文档入口

这里放长期有效、应该被反复引用的稳定文档。它们描述的是规则、边界和事实，不是阶段性方案。

## 最短阅读路径

1. [系统架构](../system-architecture.md)
2. [API 开发与目录规范](./api-development-conventions.md)
3. [项目治理经验](./project-governance-guide.md)
4. [Git 协作规范](./git-workflow-guide.md)
5. [归档治理规范](./archive-governance.md)
6. [`docs/plan/` 目录治理规范](./plan-directory-governance.md)
7. [API 响应规范](./api-response-standard.md)
8. 按任务进入对应参考或操作手册：
   - [配置参考](./configuration-reference.md)
   - [数据模型参考](./data-model-reference.md)
   - [API 端点目录](./api-endpoint-catalog.md)
   - [Web 信息架构参考](./web-information-architecture.md)
   - [Bot 架构参考](./bot-architecture-reference.md)
   - [Web 设计规范](./web-design-guide.md)
   - [前端构建优化规范](./web-build-optimization-guide.md)
   - [部署指南](../runbooks/deployment.md)
   - [测试指南](../runbooks/testing.md)

## 文档判断规则

- 当前系统怎么实现：看 [系统架构](../system-architecture.md)。
- 长期有效的开发约束：看 `docs/reference/`。
- 遇到治理、重构、目录收口、文档归档判断：看 [项目治理经验](./project-governance-guide.md)。
- 遇到 archive 分类、计划归档和索引同步问题：看 [归档治理规范](./archive-governance.md)。
- 遇到新计划文档该放在哪个目录的问题：看 [`docs/plan/` 目录治理规范](./plan-directory-governance.md)。
- 需要在 Ember 项目里使用多 agent 协作：看 [多 Agent 协作指南](./multi-agent-collaboration-guide.md)。
- 前端打包体积、共享依赖注册和 chunk 切分：看 [前端构建优化规范](./web-build-optimization-guide.md)。
- 某个功能准备怎么做：优先看 `docs/plan/`；新稿直接从 `docs/plan/plan-template.md` 起步。
- 历史方案和旧实现：只去 `docs/archive/` 追溯，不拿它当现行依据。

## 文档地图

- [项目治理经验](./project-governance-guide.md) - 结构治理、重构推进和文档收尾的稳定经验
- [Git 协作规范](./git-workflow-guide.md) - 分支命名、PR 合并和发布分支约束
- [归档治理规范](./archive-governance.md) - `docs/archive/` 的分类、收尾和索引同步规则
- [`docs/plan/` 目录治理规范](./plan-directory-governance.md) - 新计划文档的职责分类与落位规则
- [多 Agent 协作指南](./multi-agent-collaboration-guide.md) - 多 agent 任务拆分、职责边界与提问模板
- [配置参考](./configuration-reference.md) - 配置来源、优先级和边界
- [数据模型参考](./data-model-reference.md) - 系统表结构、字段语义与关系说明
- [API 端点目录](./api-endpoint-catalog.md) - HTTP / Internal API 路由分组与用途总表
- [API 开发与目录规范](./api-development-conventions.md) - `services/api` 的分层、依赖方向与稳定实现模式
- [API 响应规范](./api-response-standard.md) - 接口返回、字段命名和模型映射约定
- [Web 信息架构参考](./web-information-architecture.md) - 共享组件层、页面职责与路由归属
- [Bot 架构参考](./bot-architecture-reference.md) - Bot 端点、命令处理器、运行模式与通信边界
- [Web 设计规范](./web-design-guide.md) - 前端设计系统与实现约束
- [前端构建优化规范](./web-build-optimization-guide.md) - Web 体积、依赖注册和 chunk 切分约束

## 维护规则

- 改动涉及模型、服务边界、API 路由、前端结构、配置来源时，同步更新 [系统架构](../system-architecture.md)。
- 新增稳定规范时，不要塞进入口文档，直接落到对应 `docs/reference/` 文件。
- 只有“当前仍然有效”的内容才能放这里。
- 如果文档主要描述“准备怎么做”，那不是参考文档，应转去 `docs/proposals/` 或 `docs/plan/`。
