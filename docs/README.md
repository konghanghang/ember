# Ember 文档中心

这里是 Ember 的唯一文档导航入口。规则很简单：先判断文档属于“稳定事实”还是“阶段性方案”，再决定放哪。

## 阅读顺序

1. [系统架构](./system-architecture.md)：先建立系统全局认知。
2. [开发指南](./reference/development-guide.md)：再看开发时真正要遵守的入口规则。
3. 按用途进入对应分区：`reference`、`runbooks`、`proposals`、`archive`。

## 文档分层

- [system-architecture.md](./system-architecture.md)：当前系统的核心真相来源
- [reference/](./reference/README.md)：稳定参考文档
- [runbooks/](./runbooks/README.md)：部署、测试、构建、联调、排障
- [proposals/](./proposals/README.md)：提案、盘点与仍在推进的设计
- [archive/](./archive/README.md)：历史归档

## 当前入口

### 核心

- [系统架构](./system-architecture.md)
- [开发指南](./reference/development-guide.md)

### 稳定参考

- [配置参考](./reference/configuration-reference.md)
- [项目治理经验](./reference/project-governance-guide.md)
- [多 Agent 协作指南](./reference/multi-agent-collaboration-guide.md)
- [API 开发与目录规范](./reference/api-development-conventions.md)
- [API 响应规范](./reference/api-response-standard.md)
- [Emby API 参考](./reference/emby-api-guide.md)
- [Web 设计规范](./reference/web-design-guide.md)

### 操作手册

- [部署指南](./runbooks/deployment.md)
- [部署环境与配置](./runbooks/deployment-environment.md)
- [部署排障](./runbooks/deployment-troubleshooting.md)
- [测试指南](./runbooks/testing.md)
- [手工测试清单](./runbooks/manual-testing-checklist.md)
- [测试排障](./runbooks/testing-troubleshooting.md)
- [Stripe 支付测试指南](./runbooks/stripe-payment-testing.md)
- [Cloudflared 本地联调](./runbooks/cloudflared-local-testing.md)
- [Docker 构建指南](./runbooks/docker-build-guide.md)
- [发布流程](./runbooks/release-process.md)

### 方案与设计

- [提案总览](./proposals/README.md)
- [`docs/plan/`](./plan/README.md)：进行中的功能规划，当前主要保留未落地或持续治理项

### 归档

- [归档总览](./archive/README.md)
- [API 目录重构提案（已完成）](./archive/api-directory-refactor.md)
- [MVP 初始设计（历史资料）](./archive/mvp/design.md)

## 维护规则

- 新增稳定规则：放进 `reference/`。
- 新增操作流程：放进 `runbooks/`。
- 新增需求或设计讨论：优先放 `plan/` 或 `proposals/`。
- 已经失效但需要保留追溯：放进 `archive/`。
