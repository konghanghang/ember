# 参考文档

这里放长期有效、应该被反复引用的稳定文档。它们描述的是规则、边界和事实，不是阶段性方案。

## 文档列表

- [开发指南](./development-guide.md) - 开发时的最短阅读路径
- [项目治理经验](./project-governance-guide.md) - 结构治理、重构推进和文档收尾的稳定经验
- [归档治理规范](./archive-governance.md) - `docs/archive/` 的分类、收尾和索引同步规则
- [`docs/plan/` 目录治理规范](./plan-directory-governance.md) - 新计划文档的职责分类与落位规则
- [多 Agent 协作指南](./multi-agent-collaboration-guide.md) - 多 agent 任务拆分、职责边界与提问模板
- [配置参考](./configuration-reference.md) - 配置来源、优先级和边界
- [数据模型参考](./data-model-reference.md) - 系统表结构、字段语义与关系说明
- [API 端点目录](./api-endpoint-catalog.md) - HTTP / Internal API 路由分组与用途总表
- [Web 信息架构参考](./web-information-architecture.md) - 共享组件层、页面职责与路由归属
- [Bot 架构参考](./bot-architecture-reference.md) - Bot 端点、命令处理器、运行模式与环境变量
- [代码模式速查](./code-patterns.md) - 常见 handler/service/错误/通知等实现约定
- [API 开发与目录规范](./api-development-conventions.md) - `services/api` 的分层和目录约束
- [API 响应规范](./api-response-standard.md) - 接口返回、字段命名和模型映射约定
- [Emby API 参考](./emby-api-guide.md) - Emby 集成接口与调试说明
- [Web 设计规范](./web-design-guide.md) - 前端设计系统与实现约束
- [前端构建优化规范](./web-build-optimization-guide.md) - Web 体积、依赖注册和 chunk 切分约束

## 维护规则

- 只有“当前仍然有效”的内容才能放这里。
- 如果文档主要描述“准备怎么做”，那不是参考文档，应转去 `docs/proposals/` 或 `docs/plan/`。
