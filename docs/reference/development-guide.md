# 开发指南

这不是另一个大杂烩文档。它只负责告诉你开发 Ember 时先看什么、按什么顺序看，以及哪些文档才算当前有效。

## 最短阅读路径

1. [系统架构](../system-architecture.md)
2. [API 开发与目录规范](./api-development-conventions.md)
3. [项目治理经验](./project-governance-guide.md)
4. [API 响应规范](./api-response-standard.md)
5. 按任务进入对应操作手册：
   - [部署指南](../runbooks/deployment.md)
   - [测试指南](../runbooks/testing.md)
   - [Cloudflared 本地联调](../runbooks/cloudflared-local-testing.md)
   - 前端构建体积、依赖注册和 chunk 问题：看 [前端构建优化规范](./web-build-optimization-guide.md)

## 文档判断规则

- 当前系统怎么实现：看 [系统架构](../system-architecture.md)。
- 长期有效的开发约束：看 `docs/reference/`。
- 遇到治理、重构、目录收口、文档归档判断：看 [项目治理经验](./project-governance-guide.md)。
- 前端打包体积、共享依赖注册和 chunk 切分：看 [前端构建优化规范](./web-build-optimization-guide.md)。
- 某个功能准备怎么做：优先看 `docs/plan/`；新稿直接从 `docs/plan/plan-template.md` 起步。`docs/specs/` 主要保留 workflow 产物和历史设计资料。
- 历史方案和旧实现：只去 `docs/archive/` 追溯，不拿它当现行依据。

## 文档维护规则

- 改动涉及模型、服务边界、API 路由、前端结构、配置来源时，同步更新 [系统架构](../system-architecture.md)。
- 新增稳定规范时，不要塞进 `README`，直接落到 `docs/reference/`。
- 设计稿落地后，把稳定结论提炼到 `docs/reference/` 或 `docs/runbooks/`，不要让方案文档永远挂着。
