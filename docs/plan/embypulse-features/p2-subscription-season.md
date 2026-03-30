# P2-1: 求片分季支持（Subscription Season Support）

> 状态：草稿
> 负责人：Ember
> 更新时间：2026-03-30

## 功能描述

支持电视剧按季求片，例如“权力的游戏 第 3 季”。

**优先级**：P2

---

## Ember 对齐要点

1. 复用现有 `subscriptions` 表，不新建 `media_subscriptions`
2. 与现有字段命名保持一致：`type/name/tmdbId/posterPath/note/status/mpError`
3. 新增 `season` 字段时保持向后兼容（默认 `0` 表示整剧）
4. 去重语义将从“全局去重(type+tmdbId)”调整为“按季去重(type+tmdbId+season)”（属于可见行为变更，需发布说明）
5. 当前实现覆盖 `Web + API + SQL migration + Telegram Bot`，并透传 MoviePilot 季参数

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

- Telegram Bot 已支持电视剧先选季、再确认订阅
- Bot 搜索流不再支持备注输入，统一使用纯按钮交互

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
4. 审批推送 MoviePilot 时：
- `season=0` 不传季参数
- `season>0` 透传季参数，精确订阅指定季

5. 扩展 `MoviePilotClient` 请求结构
- `SubscribeRequest` 支持 `season`
- Ember 审批通过时，`season>0` 透传给 MoviePilot，`season=0` 则省略该字段
- `mpError` 仅用于记录真实的 MoviePilot 调用失败，不再记录人为降级说明

6. Telegram/Bot 已支持分季输入
- Telegram `SubscribeByTelegram` 接收并透传 `season`
- Bot 搜索后：电影直接确认订阅，电视剧先选季再确认
- Bot 不再支持备注输入，避免按钮流和文本输入混用

---

## 影响范围

- API：
  - `Subscription` 模型
  - `CreateSubscriptionRequest`
  - 去重逻辑
  - 审批时的 MoviePilot 季参数透传
- Web：
  - `NewSubscriptionView`
  - `SubscriptionsView`
  - 如有管理端订阅列表，也需同步展示季号
- Bot：
  - 支持电视剧按季选择
  - 电影保持直接订阅
  - 备注输入从 Bot 主流程移除
- 数据库：
  - 必须新增 migration
- 文档：
  - `docs/system-architecture.md`
  - 如需对用户说明行为变化，可补发布说明

---

## 非目标

本次明确不做：

- 不新增独立的“按季订阅”表
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
- [ ] `season>0` 审批后会透传 MoviePilot 季参数
- [ ] SQL migration 可重复执行且不破坏历史数据

**预计工作量**：1-2 天
