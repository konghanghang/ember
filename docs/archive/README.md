# 历史归档

这里放已经完成、失效或只保留追溯价值的文档。它们可以帮助理解历史决策，但不能直接当成当前规范。

## 归档内容

- [历史实施计划](./plan/)
- [历史提案](./proposal/)
- [历史总结与测试报告](./report/)
- [旧版参考资料](./reference/)
- [MVP 历史资料](./mvp/README.md)

当前 `plan/` 目录按 Ember 现有职责边界分类，而不是按历史来源分类：

- `access-auth/`：注册、登录保护、邮箱验证、找回密码
- `billing-redemption/`：Stripe、套餐分组、兑换码、一人一码、兑换目录重构
- `media-subscription/`：追剧日历、媒体质量、播放历史、分季订阅、排行榜、最近入库
- `bot-telegram/`：Telegram 绑定、Bot 菜单、搜索订阅、通知、Polling
- `console-admin/`：活跃会话、统一控制台、设备管理、头像、权限模板、后台创建用户
- `architecture/`：设置中心、邮箱鉴权边界等结构性方案

`report/` 用于保存总结、复盘和历史测试报告，例如：

- [重大 Bug 修复总结](./report/bugfix-summary.md)
- [多 Agent 执行复盘：兑换码备注字段](./report/multi-agent-redemption-code-notes-case.md)
- [历史测试报告](./report/test/2025-12-07-mvp-core-testing.md)

`reference/` 用于收口旧版但仍需追溯的参考材料，例如：

- [旧版 API 参考](./reference/api-reference.md)

`proposal/` 用于保存已经退场的治理/重构提案，例如：

- [API 目录重构提案](./proposal/api-directory-refactor.md)
- [前端设计系统治理提案](./proposal/design-system-governance.md)

`docs/archive/mvp/` 当前用于收口 MVP 阶段的历史资料，例如：

- [MVP 初始设计](./mvp/design.md)
- [MVP 历史任务清单](./mvp/tasks.md)

## 使用规则

- 需要追溯历史时再看。
- 归档文档如果与当前实现冲突，以 [系统架构](../system-architecture.md) 和 `docs/reference/` 为准。
