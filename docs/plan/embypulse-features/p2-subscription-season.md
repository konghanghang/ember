# P2-1: 求片分季支持（Subscription Season Support）

> 状态：草稿
> 负责人：Ember
> 更新时间：2026-03-29

## 功能描述

支持电视剧按季求片，例如“权力的游戏 第 3 季”。

**优先级**：P2

---

## Ember 对齐要点

1. 复用现有 `subscriptions` 表，不新建 `media_subscriptions`
2. 与现有字段命名保持一致：`type/name/tmdbId/posterPath/note/status/mpError`
3. 新增 `season` 字段时保持向后兼容（默认 `0` 表示整剧）
4. 去重语义将从“全局去重(type+tmdbId)”调整为“按季去重(type+tmdbId+season)”（属于可见行为变更，需发布说明）
5. 第一版只做 `Web + API + SQL migration`，不扩 Telegram/Bot 分季输入，不透传 MoviePilot 季参数

---

## 数据模型设计

在现有 `Subscription` 模型基础上新增字段：

```go
Season int `json:"season" gorm:"column:season;not null;default:0;uniqueIndex:uk_subscription_media,priority:3"`
```

说明：

- `season=0`：整剧订阅
- `season>0`：指定季订阅
- 现有字段 `posterPath/status/note/mpError/updatedAt` 保持不变，不重写整模型

唯一索引建议：
- 由现有 `(type, tmdbId)` 升级为 `(type, tmdbId, season)`

数据库迁移交付要求：

- 新增 `season` 列，默认值 `0`
- 回填历史数据为 `0`
- 显式删除旧唯一索引 `(type, tmdbId)`
- 显式创建新唯一索引 `(type, tmdbId, season)`
- migration 文件必须幂等

说明：

- 仅靠 AutoMigrate 不可靠，本功能完成条件必须包含 SQL migration
- 迁移文件应放在 `infrastructure/database/`

---

## API 端点设计

沿用现有创建接口：`POST /api/v1/subscriptions`  
第一版同时扩展内部结构，但不新增独立 endpoint。

```json
{
  "type": "TV",
  "name": "Game of Thrones",
  "tmdbId": "1399",
  "season": 3
}
```

- 不传 `season` 时，后端默认 `0`（整剧）

输入约束：

- `TV`：允许 `season >= 0`
- `MOVIE`：后端强制视为 `season = 0`
- `season` 仅允许整数
- 负数、非数字、小数都应拒绝

说明：

- 第一版不扩 Telegram Bot 分季输入，所以 Telegram internal subscribe 请求保持现状
- 若后续要支持 Bot 分季，需单独补计划或在第二版明确扩展

---

## 前端改动范围

第一版前端必须改两处：

1. 新建订阅页 `NewSubscriptionView`
- 仅当 `type=TV` 时展示“季数”输入
- 默认值为 `0`（整剧）
- 电影不展示该输入，提交时固定 `season=0`

2. 订阅列表页 `SubscriptionsView`
- 当 `season>0` 时，名称或元信息里必须显示“第 N 季”
- 同剧不同季必须可区分、可单独删除

说明：

- 如果列表不展示季号，这个功能在用户侧是半成品
- 管理端订阅列表若已承接同一数据，也应同步展示季号

---

## 核心实现建议

1. 扩展 `CreateSubscriptionRequest`，新增 `season` 可选字段
2. 在 service 层统一规范化 `season`
- `MOVIE` 强制置为 `0`
- `TV` 未传时置为 `0`
3. 去重逻辑改为 `type + tmdbId + season`
4. 审批推送 MoviePilot 时（第一版确定策略）：
- `season=0` 不传季参数
- `season>0` 先降级为整剧订阅

5. 第一版不扩展 `MoviePilotClient` 请求结构
- 当前 `SubscribeRequest` 仅支持 `type/name/tmdbid`
- 第一版仅在 Ember 侧记录 `season`
- 当 `season>0` 且审批通过时，在 `mpError` 标注“季参数未透传（已降级整剧）”
- 后续若上游 MoviePilot API 明确支持季参数，再单独升级为透传

6. 第一版不扩 Telegram/Bot 分季输入
- Telegram `SubscribeByTelegram` 维持现状
- Bot 搜索后订阅仍然只能提交整剧
- 避免同时改 Web、Bot、Internal API 三条输入链路，扩大回归面

---

## 影响范围

- API：
  - `Subscription` 模型
  - `CreateSubscriptionRequest`
  - 去重逻辑
  - 审批时的 MoviePilot 降级说明
- Web：
  - `NewSubscriptionView`
  - `SubscriptionsView`
  - 如有管理端订阅列表，也需同步展示季号
- Bot：
  - 第一版不改输入能力
  - 仅确认不被现有后端改动误伤
- 数据库：
  - 必须新增 migration
- 文档：
  - `docs/system-architecture.md`
  - 如需对用户说明行为变化，可补发布说明

---

## 非目标

本次明确不做：

- 不新增独立的“按季订阅”表
- 不扩 MoviePilot 季参数透传
- 不扩 Telegram/Bot 分季输入
- 不做“自动解析第几季”的自然语言识别
- 不修改现有审批按钮、拒绝按钮与 Bot 通知协议

---

## 验证清单

- [ ] 同剧不同季可分别提交
- [ ] 同剧同季重复提交被拦截
- [ ] 未传季数时行为与当前一致
- [ ] `MOVIE` 提交时始终落为 `season=0`
- [ ] 用户订阅列表能明确展示季号
- [ ] 管理端订阅列表能明确展示季号（若复用同一数据）
- [ ] 审批/拒绝流程不受影响
- [ ] `season>0` 审批后不会阻塞 MoviePilot 调用，但会记录降级说明
- [ ] SQL migration 可重复执行且不破坏历史数据

**预计工作量**：1-2 天
