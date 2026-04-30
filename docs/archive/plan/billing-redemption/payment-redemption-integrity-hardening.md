# 支付与兑换码完整性加固方案

> 状态：已归档（主干完成，转为历史追溯）
> 负责人：Ember
> 更新时间：2026-04-30

本文档已退出现行实施稿目录。当前支付 / 套餐分组 / 兑换码完整性事实以 `docs/system-architecture.md` 为准；本文件仅保留历史整改过程与决策追溯价值。

## 落地进度

批次 2 + 后续 review 修复已完成本方案绝大部分 P0 / P1 主干项：

- ✅ pending payment partial unique、事务内占位 + Stripe `Idempotency-Key` 已落地
- ✅ Stripe webhook `event.id` 去重、`checkout.session.expired` 收口和 pending 过期 cron 已落地
- ✅ 支付 / 兑换的 Emby 调权已移出事务；失败统一落到 `failed_emby_async_ops` 补偿队列，而不是原计划里的独立 `failed_emby_unbans` 表
- ✅ 模板用户 Policy 白名单收口、`expirePendingPayments*` 命名区分、兑换码状态语义复用已落地
- ✅ 多币种口径、支付索引去重与架构文档同步已落地
- ✅ `PlanGroup` 展示态已拆到 `PlanGroupView`，持久化模型不再靠 `gorm:"-"` 挂展示字段

## 交叉引用

- 当前系统事实：
  - [docs/system-architecture.md](</Users/konghang/data/me/github/ember/docs/system-architecture.md>) §5.3 / §5.4 / §5.15 已收录支付幂等、Stripe webhook 去重、事务外 Emby 补偿与兑换码语义
- 当前盘点入口：
  - [docs/proposals/plan-inventory.md](</Users/konghang/data/me/github/ember/docs/proposals/plan-inventory.md>) 已把本方案标为已归档

## 退场说明

- 本文档不再承担当前支付 / 兑换码完整性规则说明职责；现行事实以 `docs/system-architecture.md` 为准。
- 顶部状态、交叉引用与入口文档已完成归档收口，因此本文件只保留历史追溯价值。
