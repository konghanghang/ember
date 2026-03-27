# 历史归档

这里放已经完成、失效或只保留追溯价值的文档。它们可以帮助理解历史决策，但不能直接当成当前规范。

## 归档内容

- [API 目录重构提案](./api-directory-refactor.md)
- [旧版 API 参考](./api-reference.md)
- [重大 Bug 修复总结](./bugfix-summary.md)
- [多 Agent 执行复盘：兑换码备注字段](./multi-agent-redemption-code-notes-case.md)
- [历史实施计划](./plan/)
- [历史测试报告](./test-reports/)
- [MVP 历史资料](./mvp/README.md)

当前 `plan/` 目录下既有旧实现计划，也开始承接已落地方案的退场归档，例如：

- 兑换码批量生成
- 兑换码一人一码一次
- 活跃会话
- 邮箱验证码注册
- 找回密码
- 最近入库
- 播放排行榜
- 设置中心
- Stripe 支付
- Telegram 绑定 / 搜索 / 通知
- Telegram Bot 菜单
- 统一控制台
- 欢迎消息
- 部分 `embypulse-features` 已完成条目

`docs/archive/mvp/` 当前用于收口 MVP 阶段的历史资料，例如：

- [MVP 初始设计](./mvp/design.md)
- [MVP 历史任务清单](./mvp/tasks.md)

## 使用规则

- 需要追溯历史时再看。
- 归档文档如果与当前实现冲突，以 [系统架构](../system-architecture.md) 和 `docs/reference/` 为准。
