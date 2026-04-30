# 功能方案

这里放具体功能或模块的实现方案。默认要求是：写到足够让另一个工程师直接开做，但不要把文档写成源码替代品。

## 什么时候放这里

- 新功能设计
- 重要功能重构方案
- 需要明确接口、数据结构、行为边界的实施稿

## 目录规则

`docs/plan/` 根目录只保留入口和模板：

- `README.md`
- `plan-template.md`

新增计划文档默认按职责边界落位，不再平铺在根目录。

当前标准目录为：

- `docs/plan/access-auth/`
- `docs/plan/billing-redemption/`
- `docs/plan/media-subscription/`
- `docs/plan/bot-telegram/`
- `docs/plan/console-admin/`
- `docs/plan/architecture/`

具体归类规则见 [docs/reference/plan-directory-governance.md](../reference/plan-directory-governance.md)。

## 不该放这里的内容

- 文档治理、盘点、重构策略：放 `docs/proposals/`
- 已完成或废弃的旧方案：移到 `docs/archive/`
- 稳定规则或现行事实：提炼到 `docs/reference/` 或 `docs/system-architecture.md`

当前 `docs/plan/` 中仍在推进中的实施稿包括：

- `access-auth/registration-email-domain-allowlist.md`
- `bot-telegram/notification-mute-rules.md`
- `bot-telegram/subscription-admin-message-sync.md`
- `console-admin/device-risk-automation.md`
- `console-admin/in-app-notification-center.md`
- `console-admin/playback-and-device-observation-hardening.md`
- `media-subscription/media-dedupe-and-quality-governance.md`
- `media-subscription/subscription-resubmission-after-rejection.md`

当前已完成主干、但仍保留明确尾项的实施稿：

- `bot-telegram/bot-notification-and-info-leak-hardening.md`

当前已进入“归档准备”但尚未迁出的实施稿：

- `architecture/schema-deployment-and-baseline-cleanup.md`

## 模板

- [功能方案模板](./plan-template.md)

## 编写标准

- 讲清目标、非目标、影响面和验收方式
- 写清接口、数据结构、用户可见行为
- 不贴大段实现代码
- 不写“先这样后面再说”的空话
- 新文档创建前，先判断主链路属于哪个职责目录；不要默认直接放在 `docs/plan/` 根目录
