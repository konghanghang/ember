# 订阅状态机与 webhook 命中精度加固方案

> 状态：已归档（主干完成，转为历史追溯）
> 负责人：Ember
> 更新时间：2026-04-30

本文档已退出现行实施稿目录。当前订阅 / 缺集状态机事实以 `docs/system-architecture.md` 为准；本文件仅保留历史决策与整改过程追溯价值。

## 落地进度

批次 2 + 后续 review 修复已完成本方案的大部分主干项：

- ✅ 订阅审批 / 拒绝 / 管理员手动收口改为原子状态转移，避免旧快照覆盖新状态
- ✅ 创建 / 重提交改为 advisory lock + 活跃唯一索引幂等收口
- ✅ `RedispatchSubscription`、`DISPATCH_FAILED`、`lastDispatchError`、`media_gap_scans` 已落地
- ✅ `ignoreReasonCode` 已落地：人工忽略写 `manual`，系统收口写 `season_not_activated`
- ✅ `MarkSubscriptionIngestedAsAdmin` 与 webhook 路径用户通知对齐
- ✅ handler 内部错误收口到统一错误响应，不再裸透 SQL 错误
- ✅ 缺集扫描跨副本互斥已通过 PostgreSQL advisory lock + `media_gap_scans` 表落地
- ✅ review 补丁已收口：缺集扫描不再整行 `Save` 回滚并发状态；命中 `IGNORED` 时不再自动复活；系统忽略不再计入整剧人工排除分母

## 稳定结论

以下结论已经提炼为当前事实：

- 订阅审批 / 拒绝 / 管理员手动收口必须走原子状态转移，命中并发冲突返回明确 409，而不是用整行覆盖写回旧快照。
- 整剧订阅的自动收口语义已经固定：先写 `ingestProgress`，只有 `Y >= X` 时才自动进入 `INGESTED`；否则保留 `APPROVED`，管理员仍可显式确认。
- 缺集工单的 `IGNORED` 决策不能被 webhook 或扫描自动撤销；`DISPATCH_FAILED` 是可观察、可重试的稳定状态，而不是临时 toast。
- 缺集扫描跨副本互斥已经固定为 PostgreSQL advisory lock + `media_gap_scans` 审计记录，不再依赖进程内单实例心智。

## 交叉引用

- 当前系统事实：
  - [docs/system-architecture.md](</Users/konghang/data/me/github/ember/docs/system-architecture.md>) §5.9 已收录订阅状态转移、`RedispatchSubscription`、整剧 `ingestProgress` 与 webhook 收口语义
  - [docs/system-architecture.md](</Users/konghang/data/me/github/ember/docs/system-architecture.md>) §5.11 已收录 `media_gap_scans`、`DISPATCH_FAILED`、`IGNORED` 不复活和缺集 webhook 命中规则
- 当前盘点入口：
  - [docs/proposals/plan-inventory.md](</Users/konghang/data/me/github/ember/docs/proposals/plan-inventory.md>) 已把本方案标为已归档

## 退场说明

- 本文档不再承担当前状态机规则说明职责；现行事实以 `docs/system-architecture.md` 为准。
- 顶部状态、交叉引用与入口文档已完成归档收口，因此本文件只保留历史追溯价值。

## 背景

2026-04-25 系统性 review 在求片订阅 / 缺集（mediagap）链路暴露多类状态机硬伤，整体品味评分 🔴：

- 审批 / 拒绝 / 校验入库链路用 `Save(&subscription)` 全字段覆盖。当并发 Emby webhook 已经把同一条订阅写入 `INGESTED + ingestedAt`，审批路径 Save 会用旧的 status / mpError 把它打回 APPROVED，丢 `ingestedAt`，丢审计。
- `MarkSubscriptionsIngestedByWebhook` 用 `season = 0 OR season = ?` 匹配，整剧订阅（season=0）只要任意一集入库就被错误收口为 INGESTED；用户和管理员都以为整剧已完成。
- 缺集 webhook `MarkIngestedByWebhook` 无条件复活 `IGNORED` 工单：管理员的人工忽略决定被静默撤销，且 `ignoreReason / ignoredAt` 被清空，审计丢失。
- MoviePilot 调用失败仅记 `mpError`，无重试入口、无后台再下发、无告警，APPROVED 长期挂死，需要管理员手动 reject + 用户 resubmit 才能恢复。
- `hasActiveSubscription` 检查与 `Create` 之间无事务、无锁，并发"重新提交"或快速重试可能造出多条同 retryFromId 的边界状态；唯一索引兜底但错误信息不再是业务语义。
- 缺集异步扫描使用进程内 `mediaGapScanManager`，多副本部署时并发跑全库扫描，对 Emby / TMDB 流量做 N 倍放大。
- `MarkIngestedByWebhook` 对每条 gap 各自 `Save`，N 集批量入库触发 N 次 round-trip。
- `DispatchGapCandidate` 失败后 gap 状态不更新为可重试，前端只能依赖 toast；状态机里没有 `DISPATCH_FAILED`。
- handler 直接 `c.JSON(StatusInternalServerError, gin.H{"error": err.Error()})`，把内部 SQL 错误透传给客户端。
- `cleanupInactiveSeasonGaps` 把"季未激活"的工单一律改为 IGNORED 且 `ignoreReason` 是固定英文字符串，前端无法区分系统 IGNORE 与人工 IGNORE。
- `MarkSubscriptionIngestedAsAdmin` 路径不发 `notifyIngested`，与 webhook 路径用户体验不一致。
- 通知协程裸 `go`，无超时、无 panic recover；该问题与 `bot-telegram` 计划共用 `safeFireAndForget` 收口。

> 注：TMDB 密钥泄漏、追剧日历相关的链路问题集中在 `docs/archive/plan/media-subscription/tv-calendar-and-tmdb-key-protection.md` 中处理，本计划只覆盖订阅与缺集状态机。
