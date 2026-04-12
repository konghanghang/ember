# 追剧日历状态纠偏实现方案

> 状态：草稿
> 负责人：Ember
> 更新时间：2026-04-12

## 背景

这个问题为什么现在要解决：

- 当前 `TV Calendar` 存在较明显的状态显示错误，用户会把“今天更新”看成“待播”，或把已经入库的剧集继续看成“缺失”。
- 项目已经存在 `CRON_TIMEZONE` 配置和默认时区，但追剧日历状态判定链路没有真正接入该时区，导致“today / upcoming / missing”按 UTC 粗暴计算。
- 默认同步窗口只覆盖本周，而前端允许浏览更大的周范围；很多用户可见周视图天然没有被及时同步，状态错误和空数据会被放大。
- 追剧日历读链路过度依赖缓存状态，错过 webhook 或同步不及时后，状态缺少轻量校正机制，错误会长期残留。

## 目标

本方案要实现：

1. 让追剧日历所有用户可见状态统一基于配置时区计算，不再直接使用 UTC 日期。
2. 让默认同步窗口与前端默认浏览范围至少在“本周 + 下周”上对齐，减少天然空窗和滞后状态。
3. 为当前周可见条目增加轻量物理校验或校正机制，避免“已入库但仍显示缺失”的长期脏状态。
4. 保持现有 TV Calendar 页面、订阅入库 webhook、TMDB 同步链路继续工作，不破坏既有接口路径。

## 非目标

本次明确不做：

- 不重做 TV Calendar 产品定位，不把页面直接改造成全新“我的追剧日历”产品。
- 不引入新的消息中心、下载器联动或缺集补货流程。
- 不在本次方案里扩大到 TV Calendar 订阅模型重构。
- 不做跨时区的用户个性化时区设置；首版统一使用系统配置时区。

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：
  - `docs/system-architecture.md`
  - `docs/reference/configuration-reference.md`
  - `docs/reference/web-design-guide.md`
- 相关服务/页面/模型：
  - `services/api/internal/services/tvcalendar/service.go`
  - `services/api/internal/handlers/tv_calendar.go`
  - `services/api/internal/config/config.go`
  - `services/api/internal/app/cron.go`
  - `services/web/src/views/console/TVCalendarView.vue`
  - `services/web/src/api/console.ts`
  - `services/api/internal/models/tv_calendar.go`
- 当前行为：
  - `CRON_TIMEZONE` 已定义为运行期配置，默认值是 `Asia/Shanghai`。
  - cron 初始化会读取 `CRON_TIMEZONE`，并用它驱动排行榜、过期检查和追剧日历同步调度。
  - TV Calendar 的状态判定仍使用 `time.Now().UTC()` 和 `normalizeDateUTC()`。
  - 默认同步周偏移仅为 `0`，即本周。
  - 前端 TV Calendar 页面当前固定调用 `/api/v1/tv-calendar/global`，不是 `following` 视图。
- 现有限制：
  - 状态判定基线和系统配置时区脱节。
  - 可浏览周范围大于默认同步窗口，导致很多周天生数据不准或为空。
  - 读链路没有对当前可见条目做轻量 ready 校验，状态一旦错就容易长期错。
  - 页面语义偏“个人追剧”，但当前读的是 global 数据，容易放大用户对状态错误的感知。

## 方案设计

### 1. 用户可见行为

- 新增能力：
  - 追剧日历中的 `已入库 / 今日播出 / 缺失 / 待播` 在配置时区下稳定显示，不再因 UTC 边界跳变。
  - 当管理员完成同步后，用户默认能稳定看到“本周 + 下周”的正确状态。
  - 当前周内已实际入库的条目，能在可接受的延迟内被纠正为 `已入库`，不长期停留在 `缺失`。
- 修改现有行为：
  - TV Calendar 的“今日”定义从 UTC 自然日改为系统配置时区下的自然日。
  - 默认周历缓存窗口从仅本周扩大到至少本周和下周。
- 哪些现有行为必须保持不变：
  - 现有 `/api/v1/tv-calendar/global`、`/api/v1/tv-calendar/following`、`/api/v1/webhooks/emby` 路径不变。
  - 现有 `ready / missing / today / upcoming` 状态枚举不变。
  - webhook 仍然是最快速的入库点亮路径，不被读时校验取代。
- 前端约束：
  - 前端实现必须遵守 Ember 风格。
  - 设计与交互基线以 `docs/reference/web-design-guide.md` 为准。
  - 本次优先修正现有 `TVCalendarView` 的状态可靠性，不引入第二套日历页面。
  - 若增加视图说明、状态提示或范围提示，必须沿用现有卡片、筛选条和状态标签体系。

### 2. 数据与模型

> 本次不涉及数据模型变更。

- 继续使用现有：
  - `tv_calendar_sources`
  - `tv_calendar_items`
  - `tv_calendar_subscriptions`
- 调整内容集中在：
  - 状态计算函数
  - 同步默认周窗口
  - 读链路的轻量 ready 校验策略

### 3. 接口与边界

- 新增或修改哪些 API / Internal API / webhook / 命令：
  - 不新增公开 API 路径。
  - `GET /api/v1/tv-calendar/global`
    - 内部状态计算改为基于配置时区
    - 对当前周可见条目增加轻量 ready 校正
  - `GET /api/v1/tv-calendar/following`
    - 同步使用相同状态判定基线
  - `GET /api/v1/tv-calendar`
    - 同步使用相同状态判定基线
  - `POST /api/v1/admin/tv-calendar/sync`
    - 默认同步周偏移由 `[0]` 调整为 `[0, 1]`
- 请求参数与响应字段怎么变：
  - 接口路径和字段保持不变，优先做行为修正而不是协议扩张。
  - 若后续确有必要，可追加一个只读字段 `timezone`，帮助前端说明当前状态基线；首版不是必需项。
- 哪些调用方会受影响：
  - TV Calendar 前端页面
  - cron 和启动补偿同步
  - 依赖 TV Calendar 状态的后续链路

### 4. 关键流程

按顺序写清主链路，不需要贴代码：

1. API 启动时继续从设置中心读取 `CRON_TIMEZONE`，并提供统一的 TV Calendar 时区加载函数。
2. 追剧日历同步时，不再只默认同步本周，而是同步本周和下周。
3. TMDB 季集 air date 进入本地缓存后，状态基础值仍可落库，但读路径不直接盲信缓存状态。
4. 用户请求周历时：
   - 先按配置时区计算“今天”和目标周边界
   - 再基于该时区重算 `today / upcoming / missing`
5. 对当前周、且状态非 `ready` 的条目，按系列维度做一次轻量物理校验：
   - 若 Emby 中已存在对应季集的物理媒体，则纠正为 `ready`
   - 若没有，则保留按 air date 推导出的状态
6. webhook 到达时仍然优先直接点亮 `ready`，作为最快路径；读时校验只做兜底纠偏。

### 5. 失败路径与边界条件

- 配置时区为空或非法：回退默认 `Asia/Shanghai`，再异常时回退 UTC，但必须打明确日志。
- 当前周轻量校验失败：不能阻断周历接口；只记录日志并回退到缓存状态 + 日期推导状态。
- TMDB 数据为空或季详情拉取失败：保持当前降级行为，不让整周接口报错。
- webhook 未命中、同步延迟、缓存过期：读时校验负责纠偏，但只限当前可见窗口，避免全量重扫。
- 兼容性约束：
  - 不能破坏现有 TV Calendar webhook 点亮逻辑。
  - 不能因为纠偏逻辑把周历查询变成全库重扫。
  - 不能引入新的用户可见状态值。

## 影响范围

涉及的子系统：

- API：有
  - TV Calendar 服务状态计算、同步窗口、读时校验
  - 配置时区加载复用
- Web：低影响
  - `TVCalendarView` 可视说明和状态展示会受益，但主结构不必大改
- Bot：无
- 配置/部署：无新增环境变量
  - 继续依赖现有 `CRON_TIMEZONE`、`TV_CALENDAR_SYNC_SCHEDULE`、`TV_CALENDAR_STARTUP_SYNC_ENABLED`
- 文档：需要更新
  - `docs/system-architecture.md`
  - 如有必要，补充 `docs/reference/configuration-reference.md` 对 TV Calendar 时区语义的说明

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`
- `cd services/web && npm run build`

按改动补充针对性测试：

- API：
  - 配置时区下的 `today / upcoming / missing` 判定
  - 默认周偏移窗口
  - 当前周轻量 ready 校验逻辑
- Web：
  - 不同状态的渲染与筛选在修正后仍正常工作

### 手工验证

- 在 `CRON_TIMEZONE=Asia/Shanghai` 下，验证本地晚上和凌晨场景的“今日播出”判定不再错位
- 访问下周周历，确认默认同步后数据不再大面积为空
- 模拟 webhook 丢失但剧集已实际入库的场景，确认当前周内能通过读时校验纠正为 `已入库`
- 重复触发同步和查看周历，确认接口响应时间没有因纠偏逻辑明显失控
- TV Calendar 原有筛选、刷新、手动同步仍可用

## 落地后文档处理

落地后应同步处理：

- 将稳定结论同步到 `docs/system-architecture.md`
  - TV Calendar 状态判定基线
  - 默认同步窗口
  - webhook 与读时纠偏的职责边界
- 如最终确认需要对运维明确说明，可在 `docs/reference/configuration-reference.md` 中补充“CRON_TIMEZONE 同时作用于 TV Calendar 用户可见状态”
- 功能落地、编译验证和手工链路验证完成后，将本方案迁入 `docs/archive/plan/media-subscription/`
