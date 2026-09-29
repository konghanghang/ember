# Emby 4.9.3.0 与 Playback Reporting API 合同

本文档记录 Ember 播放排行榜依赖的 Emby 原生 API、Playback Reporting 插件 API、字段语义和版本证据。目标是避免根据字段名猜测协议，后续实现和排障应优先以本文档列出的固定版本证据为准。

## 1. 适用范围与证据等级

当前兼容基线：

| 组件 | 已确认版本 | 证据 | 结论 |
| --- | --- | --- | --- |
| Emby Server / SDK | `4.9.3.0 Release` | Emby.SDK 标签对应提交 `6ee0155063bc85578196489926359a8f37419502` | 本文所列 Emby 原生路由、参数和 DTO 均按该提交确认 |
| Playback Reporting 源码 | `2.1.0.7` | 插件 `develop` 分支提交 `30d39f9934051ccd7a0536eb7db3acf3434f125b` | 本文所列插件路由、数据库字段和响应行为按该提交确认 |
| 实际安装的 Playback Reporting | 未确认 | 需要调用 `GET /emby/Plugins` 查看 | 不能仅凭仓库源码断言实际部署行为完全一致 |

Playback Reporting `2.1.0.7` 的项目文件引用 Emby Core `4.8.0.27-beta`，没有声明以 `4.9.3.0` 为编译目标。因此固定源码可以证明接口实现方式，但不能单独证明该插件二进制与 Emby `4.9.3.0` 完全兼容；最终仍以目标服务器实际安装版本和只读接口响应为准。

证据等级：

- **版本源码确认**：由 Emby `4.9.3.0` SDK 或 Playback Reporting 固定提交直接证明。
- **官方文档确认**：由 Emby 官方 REST 文档证明，但在线页面可能随最新版更新。
- **未实机确认**：接口模型允许该用法，但尚未在目标 Emby 实例上做只读请求验证。

## 2. 版本与插件发现接口

### 2.1 查询 Emby Server 版本

```http
GET /emby/System/Info
X-Emby-Token: <api-key>
```

`4.9.3.0` OpenAPI 将响应定义为 `SystemInfo`。本功能至少使用以下字段：

| 字段 | 类型 | 用途 |
| --- | --- | --- |
| `Version` | `string` | 确认服务器版本，例如 `4.9.3.0` |
| `ServerName` | `string` | 辅助识别目标服务器 |
| `Id` | `string` | 服务器唯一标识 |
| `OperatingSystem` | `string` | 排查服务器本地时间和运行环境时使用 |

### 2.2 查询实际安装的插件版本

```http
GET /emby/Plugins
X-Emby-Token: <api-key>
```

响应是 `Plugins.PluginInfo[]`，识别 Playback Reporting 时关注：

| 字段 | 类型 | 用途 |
| --- | --- | --- |
| `Name` | `string` | 插件名称，应核对是否为 Playback Reporting |
| `Version` | `string` | 实际安装版本；这是兼容判断的最终依据 |
| `Id` | `string` | 插件唯一标识 |
| `Description` | `string` | 辅助确认插件身份 |
| `ConfigurationFileName` | `string` | 插件配置文件名 |

Playback Reporting 仓库没有可用于部署核对的 GitHub Release 或版本标签，因此不能用仓库当前版本替代 `/Plugins` 的实际结果。

## 3. Emby 媒体库与条目 API

### 3.1 获取指定用户可见视图

```http
GET /emby/Users/{UserId}/Views?IncludeExternalContent=false
X-Emby-Token: <api-key>
```

`UserId` 和 `IncludeExternalContent` 在 `4.9.3.0` OpenAPI 中均为必填参数。响应是 `QueryResult_BaseItemDto`：

```json
{
  "Items": [],
  "TotalRecordCount": 0
}
```

用于媒体库选择时至少读取 `Items[].Id`、`Items[].Name`、`Items[].Type` 和 `Items[].CollectionType`。

这里的结果是**指定用户的视图集合**，不是与用户无关的全局媒体库清单。后续如果使用这些 `Id` 查询条目，应保持同一个 `UserId` 上下文，避免把用户视图 ID 与全局查询结果混用。

### 3.2 全局条目查询

```http
GET /emby/Items
X-Emby-Token: <api-key>
```

### 3.3 用户范围条目查询

```http
GET /emby/Users/{UserId}/Items
X-Emby-Token: <api-key>
```

`4.9.3.0` OpenAPI 对以上两个接口都声明支持以下参数：

| 参数 | 类型 | 语义 |
| --- | --- | --- |
| `ParentId` | `string` | 将搜索限定到指定条目或文件夹；省略时使用根目录 |
| `Ids` | `string` | 逗号分隔的指定条目 ID 列表 |
| `Recursive` | `boolean` | 在文件夹下查询时是否递归 |
| `Fields` | `string` | 逗号分隔的附加返回字段 |
| `IncludeItemTypes` | `string` | 逗号分隔的条目类型过滤条件 |
| `StartIndex` | `integer` | 分页起始位置 |
| `Limit` | `integer` | 最大返回数量 |

两个接口的响应均为 `QueryResult_BaseItemDto`：

```json
{
  "Items": [
    {
      "Id": "item-id",
      "ParentId": "parent-id",
      "Type": "Episode",
      "SeriesId": "series-id",
      "SeriesName": "series-name",
      "IndexNumber": 1,
      "ParentIndexNumber": 1
    }
  ],
  "TotalRecordCount": 1
}
```

排行榜所需 `BaseItemDto` 字段合同：

| 字段 | OpenAPI 类型 | 语义 |
| --- | --- | --- |
| `Id` | `string` | 当前媒体条目 ID |
| `ParentId` | `string` | 直接父条目 ID，不应直接假设它必然等于媒体库 View ID |
| `Type` | `string` | 条目类型，例如 `Movie`、`Episode`、`Series` |
| `SeriesId` | `string` | Episode 所属 Series ID |
| `SeriesName` | `string` | Episode 所属剧集名称 |
| `IndexNumber` | `integer(int32)`, nullable | 集号等当前层级序号 |
| `ParentIndexNumber` | `integer(int32)`, nullable | 季号等父层级序号 |

`ItemFields` 枚举在同一 `4.9.3.0` SDK 中明确包含 `ParentId`、`SeriesId`、`SeriesName`。REST 页面中 `Fields` 参数的文字选项列表没有列出全部枚举值，因此字段能力应以同版本 SDK 的 `ItemFields` 和 `BaseItemDto` 元数据为准，不能只看在线页面的简短说明。

### 3.4 `ParentId + Ids` 的确认边界

`4.9.3.0` OpenAPI 证明 `ParentId`、`Ids` 和 `Recursive` 可以出现在同一个 `/Items` 或 `/Users/{UserId}/Items` 请求模型中，例如：

```http
GET /emby/Users/{UserId}/Items?ParentId={viewId}&Recursive=true&Ids={id1,id2}&Fields=ParentId,SeriesId,SeriesName&Limit=2
```

但 OpenAPI 没有明确说明多个过滤参数组合后一定采用“交集”语义。因此以下结论仍属于**未实机确认**：

- 响应只返回 `Ids` 中且位于 `ParentId` 视图下的条目。
- `/Items` 与 `/Users/{UserId}/Items` 对 View ID 的解释完全一致。
- `ParentId` 使用 `/Users/{UserId}/Views` 返回的 ID 时，在所有媒体库类型上都能递归命中电影和 Series。

在完成只读实机验证前，Ember 不应把“请求成功且返回空数组”直接解释为“候选条目都不属于所选媒体库”；它也可能是用户上下文、View ID 语义或参数组合行为不一致。

## 4. Playback Reporting 自定义查询 API

### 4.1 路由、权限与请求体

插件源码定义的路由：

```http
POST /emby/user_usage_stats/submit_custom_query
Content-Type: application/json
X-Emby-Token: <api-key>

{
  "CustomQueryString": "SELECT ...",
  "ReplaceUserId": false
}
```

| 字段 | 源码类型 | 语义 |
| --- | --- | --- |
| `CustomQueryString` | `string` | 直接交给插件 SQLite 连接执行的 SQL |
| `ReplaceUserId` | `bool` | 当结果列名包含 `UserId` 时，是否替换为 `UserName` |

插件 `2.1.0.7` 使用 `[Authenticated(Roles = "admin")]`，因此该接口要求管理员身份。Ember 当前通过 Emby API Key 调用；API Key 在目标服务器上是否满足插件的管理员角色检查，应以实际 HTTP 状态和响应为准。

### 4.2 响应合同

插件返回一个对象：

```json
{
  "colums": ["ItemId", "ItemName", "play_count", "total_duration"],
  "results": [
    ["123", "Example", "2", "3600"]
  ],
  "message": ""
}
```

| 字段 | 类型 | 重要约束 |
| --- | --- | --- |
| `colums` | `string[]` | 插件源码固定使用该错误拼写，不能改按 `columns` 解析 |
| `results` | `array[]` | 每行按 `colums` 的位置排列 |
| `message` | `string` | SQL 错误和“无数据”提示都可能通过此字段返回 |

插件使用 `row.GetString(x)` 读取每个结果单元格，所以 SQL 的整数、聚合值等在 JSON 中也通常是字符串。Ember 必须显式解析数值，不能依赖 JSON number。

SQL 执行异常会被插件捕获并写入：

```text
Error Running Query</br>...
```

插件方法随后仍返回普通响应对象，不能只按 HTTP `2xx` 判断查询成功。只要 `message` 表示 `Error Running Query`，Ember 就应返回错误并记录安全的上下文，不能把空 `results` 当作空榜。

### 4.3 自定义查询的执行边界

以下事实由固定提交的 `RunCustomQuery` 和 `Post(CustomQuery)` 确认：

- 请求模型只有 SQL 字符串和 `ReplaceUserId`，没有独立的绑定参数、分页游标、查询截止时间或取消参数。`LIMIT`、排序和时间条件均由 SQL 本身决定。
- `RunCustomQuery` 在同一 SQLite 连接的 `lock (connection)` 中执行一条 prepared statement，读取全部结果并在内存中构造响应；没有服务端默认行数上限。Ember 不应为了排行榜拉取整个播放明细表。
- 方法没有将语句限制为只读查询。Ember 应继续只发送内部生成的固定 `SELECT` 模板；不能把这个能力直接暴露为用户输入 SQL 的接口。
- 自定义查询不会自动套用插件报表的排除用户名单或 `IgnoreSmallerThan`。切换到内置报表不是等价替换。
- 没有查询级 cancellation token 或结果快照标识。HTTP 客户端取消不能证明插件已经停止 SQL；多个 HTTP 查询也不共享一个数据库读取快照。
- `ReplaceUserId=true` 只在列名精确包含 `UserId` 时替换该列，并枚举用户读取名称；排行榜不需要这个步骤，继续使用 `false`。

### 4.4 内置报表接口与排行榜的适配边界

2026-09-29 按同一固定源码补充核对。下表路径均位于 `/emby/user_usage_stats`，这些路由都要求管理员权限；没有访问真实服务器。

同日确认的业务口径：日榜默认每天 20:00 执行，查询当天日期范围内、执行时已经存在的播放数据。20:00 是执行计划，不是必须精确冻结的 SQL 截止点；因此传当天 `end_date`、查询到当天 `23:59:59` 可以满足这一需求，不能以接口只接受日期为由排除内置报表。Ember 自定义查询继续使用 `[当天 00:00,次日 00:00)` 也符合该口径。

| 接口 | 主要参数 / 响应 | 固定源码确认的行为 | 对 Ember 阶段榜的结论 |
| --- | --- | --- | --- |
| `GET /MoviesReport` | `user_id`、`days`、`end_date`；返回 `label/count/time` 数组 | 按 `ItemName` 分组，不返回 `ItemId`；同名不同条目会合并 | 不能代替基于稳定 ID 的电影榜 |
| `GET /TvShowsReport` | 同上；返回 `label/count/time` 数组 | 用 `substr(ItemName,0,instr(ItemName,' - '))` 截取剧名，再按名字分组；没有 `SeriesId` | 不能代替 Episode 回查 Series 后的聚合；名称带分隔符或同名剧集会碰撞 |
| `GET /UserPlaylist` | `user_id`、`aggregate_data`、`filter_name`、`days`、`end_date`、`filter` | 返回用户 / 日期 / 媒体条目及 `remote_address`；聚合粒度仍包括用户、日期、名称和条目。repository 接收 `types`，但此版本没有将它写入 SQL；`item_id` 按整数读取 | 可以查询当天，但数据比排行所需更宽，仍需二次汇总并核对过滤和 ID 类型 |
| `GET /{UserID}/{Date}/GetItems` | 指定用户、日期和 `Filter`；返回当天逐条活动 | 读取整天明细，并带客户端、设备、远端地址等字段 | 不适合全站排行逐用户遍历，也不必为了汇总传回这些明细 |
| `GET /PlayActivity`、`GET /HourlyReport`、`GET /{BreakdownType}/BreakdownReport` | 用户 / 日期、小时或类型等维度 | 服务于图表，不提供当前排行所需的完整实体 ID、Series 归并和媒体库过滤结果 | 可用于对应报表需求，不作为日榜数据源直接替换 |
| `GET /get_items`、`GET /get_item_stats`、`GET /get_item_path` | 单个父节点或整数条目 ID 等 | 读取 Emby 当前条目、用户累计观看状态或父路径，未按本次播放统计周期筛选；没有候选 ID 批量参数 | 不替代周期内聚合；逐条路径查询会增加请求数，不能据此推断 View ID 合同 |

内置电影榜、剧集榜的共同边界：

1. `end_date` 只接受 `yyyy-MM-dd`。即使省略时使用 `DateTime.Now`，repository 最终仍把上界格式化为当天 `23:59:59`，下界格式化为起始日期 `00:00:00`。这满足当前按当天日期、在计划时刻读取已有数据的要求，无需改造接口以支持精确 20:00 截止。
2. 这些方法先用 `end_date - days` 求开始日期，再包含首尾两天。因此 `days=1` 的 SQL 会覆盖昨天和今天；`days=0` 才是当天。不能凭参数名字直接当作 Ember 的“最近 N 个完整自然日”。
3. SQL 使用 `DateCreated <= end`，与 Ember 的半开区间不同；插件时间可含小数秒。最后一秒边界只在历史整日查询等场景需要注意，不构成默认 20:00 执行时选用内置报表的障碍。
4. 电影榜、剧集榜会排除 `UserList` 中的用户；`IgnoreSmallerThan` 按单条播放记录应用严格 `>` 过滤。Ember 当前自定义查询没有自动应用这两个条件，现有 60 秒门槛则在媒体聚合后判断，二者口径不同。

因此，日期参数能力不再是选择接口的限制。继续使用 `submit_custom_query` 的依据是保留稳定媒体 ID、现有媒体库筛选和明确的聚合口径；内置电影 / 剧集报表的名称分组、缺少条目 ID 及隐式过滤差异仍需处理，不能直接替换后声称行为完全相同。

## 5. `PlaybackActivity` 表合同

插件 `2.1.0.7` 创建或补齐以下字段：

| 字段 | SQLite 声明类型 | 排行榜用途 |
| --- | --- | --- |
| `DateCreated` | `DATETIME NOT NULL` | 播放开始时间及统计周期过滤 |
| `UserId` | `TEXT` | 用户维度 |
| `ItemId` | `TEXT` | 电影 ID 或 Episode ID，是回查 Emby 条目的主键 |
| `ItemType` | `TEXT` | 区分 `Movie`、`Episode` 等 |
| `ItemName` | `TEXT` | 播放条目展示名称 |
| `PlaybackMethod` | `TEXT` | DirectPlay、DirectStream、Transcode 等 |
| `ClientName` | `TEXT` | 客户端名称 |
| `DeviceName` | `TEXT` | 设备名称 |
| `PlayDuration` | `INT` | 从开始播放到当前的累计秒数 |
| `PauseDuration` | `INT` | 累计暂停秒数 |
| `RemoteAddress` | `TEXT` | 远端地址；排行榜当前不使用 |
| `TranscodeReasons` | `TEXT` | 转码原因；排行榜当前不使用 |

字段来源由插件事件监控代码直接确定：

- `ItemId = session.NowPlayingItem.Id`
- `ItemType = session.NowPlayingItem.Type`
- `DateCreated = DateTime.Now`
- `PlayDuration` 来自 `DateTime.Now - playback_info.Date` 的总秒数
- `PauseDuration` 来自暂停区间累计秒数

因此有效播放时长按插件自身报表习惯计算为：

```sql
COALESCE(PlayDuration, 0) - COALESCE(PauseDuration, 0)
```

### 5.1 时间语义

`DateCreated` 使用 Emby Server 进程的 `DateTime.Now`，不是 `DateTime.UtcNow`，保存到 SQLite 时没有独立时区字段。因此：

- `DateCreated` 表示 Emby Server 的本地墙上时间。
- Ember 不能无条件把排行榜周期转换为 UTC 后再查询。
- Ember 统一使用全局 `CRON_TIMEZONE` 解释 `DateCreated` 并生成 SQL 时间边界，不新增 Playback Reporting 专用时区配置。
- Emby Server 进程的本地时区必须与 `CRON_TIMEZONE` 对齐；不一致时属于部署配置错误，不能通过猜测 UTC 或静默换算掩盖。
- 统计周期应使用半开区间 `[start, end)`，即 `DateCreated >= start AND DateCreated < end`，避免相邻日榜或周榜重复统计边界时刻。

### 5.2 累计更新与执行时采集

`EventMonitorEntryPoint` 在开始 / 停止事件及周期性会话检查中写入数据。正常轮询间隔是 20 秒，发生异常时逐步退避，最长 300 秒；这不是精确 20 秒的数据新鲜度保证。

`UpdatePlaybackAction` 会原地更新既有行的 `PlayDuration` 和 `PauseDuration`。表中没有历史版本、最后更新时间、结束状态或逐段暂停区间。对当前执行时采集的需求：

- 按当天日期范围读取执行时已有的累计值即可。任务延迟或重新计算时读到更新后的值是正常行为，不要求还原精确 20:00，也不要求把跨 20:00 会话按秒拆分。
- 电影、剧集和总量分次查询时可能遇到累计更新。现有需求没有严格同一时刻快照要求，这属于可优化的一致性差异，不单独列为必修正确性缺陷。
- 单次自定义聚合在插件连接锁内完成，可以减少重复扫描，并降低多个查询之间的差异；外部直接操作数据库、Emby 元数据后续回查不属于这一保证。是否合并应结合复杂度、结果大小和实际耗时决定。
- 补发已生成的榜单与重新计算是两个动作：前者复用已保存的内容，后者读取当前数据。不能把重新计算描述为还原旧消息。

接口能力边界仍然存在：最终 `PlayDuration/PauseDuration` 不能说明暂停发生在某个时刻之前还是之后，插件也没有历史时点查询接口。但当前需求不依赖这种能力，不为此引入历史时长重建或严格时点冻结设计。

### 5.3 SQLite 存储与查询成本

固定源码创建独立的 `playback_reporting.db`，初始化 `PlaybackActivity` 与 `UserList`；本次检查的源码没有为 `PlaybackActivity` 创建查询索引，也没有把 Emby 媒体数据库附加进来。实际安装数据库是否另有人工索引，尚未实机确认。

- 在按源码初始化、没有额外索引的 SQLite 中，电影、Episode、总量三个独立查询各自扫描 `PlaybackActivity`。改变 `LIMIT` 不会把聚合变成只读取前若干条原始记录。
- 当前电影扩窗 `100 → 300 → 1000 → 3000` 会重复查询同一统计周期；它不是增量分页。达到窗口上限也不能证明已取得所选媒体库的完整总量。
- 自定义查询与播放记录写入共用连接锁。并发发起多个插件 SQL 不等于并行读取，较长聚合还会占用记录写入需要的锁；实际影响程度需要目标环境证据。
- 表里没有 `SeriesId`、`SeriesName` 或媒体库 ID，不能通过一个只读 SQL 直接按真实 Series 和媒体库汇总。Emby 条目回查仍有必要，不能用名称拆分替代。
- 不应由 Ember 自动修改插件管理的数据库、加索引或 `ATTACH` Emby 数据库。可以先减少重复查询；任何外部数据库结构调整都需要独立方案和授权。

### 5.4 本地 SQL 反例与验证边界

2026-09-29 使用固定源码的建表语句、SQLite `3.54.0` 内存库与合成数据验证，未启动插件、未连接 Emby，也没有使用真实播放数据：

| 用例 | 本地结果 | 能证明的结论 |
| --- | --- | --- |
| 同一电影 `ItemId=m1`，旧名 70 秒、新名 80 秒；另一电影 100 秒 | 当前 `GROUP BY ItemId, ItemName` 把 m1 拆成两项，100 秒电影排前；只按类型和 ID 聚合后 m1 为 150 秒 | 名称参与聚合会改变电影排名，名称只能作为展示信息 |
| 两个不同电影 ID 同名，分别 60、120 秒 | 内置 `MoviesReport` 的分组表达式合并为一个 180 秒条目 | 内置电影榜不保持实体身份 |
| 剧名分别为 `Alpha - East` 和 `Alpha - West`，各带单集后缀 | 内置剧集名称表达式均得到 `Alpha` | 按 ` - ` 截取剧名不能替代 Series ID |
| 昨日 21:00、今日 19:00、今日 21:00 各一条合成记录 | 内置电影 / 剧集报表 `days=1` 的日期条件纳入三条，`days=0` 纳入当天两条 | 查询当天应使用 `days=0`；这只是日期边界验证，不表示 20:00 执行能读到尚未发生的播放 |
| 先查询电影 600 秒，再将同一行累计时长更新为 780 秒，随后查询总量 | 电影与总量分别为 600 和 780 秒，查询日期范围完全相同 | 多次查询可能有采集时间差异；当前需求下作为优化依据，不认定为必须冻结时点的缺陷 |
| 按源码建表，12 万条合成 Movie / Episode / Audio 记录 | `EXPLAIN QUERY PLAN` 显示当前三个数据查询共三次表扫描；单条按类型和 ID 聚合一次扫描，应用侧求和保留所有类型总量 | 单次聚合可减少扫描次数；不是目标 Emby 的耗时或容量结论 |
| 45 秒电影、90 秒 Audio、无 ID 的 30 秒记录、有效时长 -10 秒记录和空时长记录 | 原总量 SQL 与完整聚合结果直接求和均为 155 秒 | 合并查询可保持现有总量语义；如果提前应用上榜门槛、丢弃无 ID 行或单独钳制负数，则会改变口径，需另行明确 |

单次聚合实验没有证明可无限量拉取候选，也没有确认目标 Emby 使用的 SQLite 版本。应用层仍需结果大小、整体超时和失败完整性边界；不能在超限时静默截断为成功榜单。

## 6. Ember 排行榜实现约束

基于上述合同，后续实现至少应满足：

1. 启动统计前通过 `LIMIT 0` 无数据查询校验 `DateCreated`、`ItemId`、`ItemType`、`ItemName`、`PlayDuration`、`PauseDuration` 六个字段，而不是读取真实播放记录或只校验展示字段。
2. 解析插件响应时固定读取 `colums`，并把 `message` 中的 SQL 错误升级为业务错误。
3. 电影从 Playback Reporting 的 `ItemId` 回查；Episode 完整读取周期内单集聚合，按 Episode `ItemId` 分批回查 `SeriesId` / `SeriesName`，再以 Series 为剧集榜聚合对象。白名单分支同样在完整汇总与媒体库过滤后取前十，不能按单集时长先截候选，也不能以已得到十部剧作为提前结束依据。
   - 详情请求继续每批最多 100 个唯一条目；完整读取的内存与请求批次数随周期内条目数增长，未执行生产容量验证。既有详情部分批次失败继续使用已解析结果、全失败降级空榜的边界保留，因此消除候选截断不等于保证上游故障时仍有完整结果。
4. 从 `/Users/{adminUserId}/Views` 获取媒体库后，后续成员关系查询优先使用同一管理员的 `/Users/{adminUserId}/Items`，不要在没有证据时切换到全局 `/Items`。
5. 找不到明确管理员用户时应失败并记录原因，不能回退到任意第一个普通用户，否则媒体库视图和可见条目会被静默缩小。
6. `ParentId + Ids` 返回空结果时必须保留可排查日志，包括 `userId`、`libraryId`、候选数量、HTTP 状态和插件/Emby错误；禁止记录 API Key 或完整外部响应体。
7. 在 `ParentId + Ids` 交集语义完成只读实机验证前，不应将该路径描述为已确认兼容。
8. 所有 Playback Reporting 时间解析和 SQL 边界必须使用全局 `CRON_TIMEZONE`，禁止转换为 UTC 后直接匹配插件本地时间字符串。
9. 聚合身份由媒体类型和稳定 `ItemId` 决定，展示名称不应参与分组；当前电影 SQL 仍按 ID 和名称共同分组，§5.4 的反例说明这一点尚待修复。若 SQL 用 `MAX(ItemName)` 选择确定的展示候选，不能将其解释成“最新名称”，需要最新名称时以 Emby 元数据为准。
10. 排行截断与总量计算是不同步骤。总量必须保留明确的统计类型和媒体库范围；当前全部媒体库总量 SQL 包含所有 `ItemType`，不能在合并查询时悄悄改成仅 Movie / Episode，或拿 Top 10 的时长代替总量。

### 6.1 Ember 快照存储与展示合同

2026-09-29 第一步实现将整期元数据保存到 `playback_ranking_batches`，与全部明细事务提交。周期重复时不追加明细或重复通知，空榜仍有批次；历史空批次 ID 由前向 migration 回填，缺失明细和总时长不推测恢复。

- 最新榜从已生成批次中按 `periodEnd / snapshotAt / createdAt` 排序，允许当前自然日 / 周尚未结束；历史读取继续包含完整周期上界。
- 数据库存储原始周期边界；API / Bot 的 `periodStart / periodEnd` 是供展示的覆盖日期，零点排他上界显示为前一天。例如 `[9 月 29 日 00:00,9 月 30 日 00:00)` 展示日期为 `9 月 29 日`。
- `snapshotAt` 在 Go 边界转成 `CRON_TIMEZONE` 下带 offset 的 RFC3339，Web / Bot 按该时区分量展示“生成于”，不再从周期结束时间生成“截至 00:00”。`cutoffAt` 兼容字段保留生成时分，不代表历史时点冻结。
- 新 Bot 在滚动升级时仍接受缺失 / 无效 `snapshotAt` 的旧载荷并保留原 `cutoffAt` 文案。第二步通知合同升级时复查此过渡分支；最低支持 API 均提供 `snapshotAt` 且旧版回滚窗口结束后移除。
- 第一阶段只改变存储、读取和展示，不改变本文件记录的聚合 SQL、媒体库过滤和 fire-and-forget 投递行为；后续问题见[分步修复计划](../plan/media-subscription/playback-summary-improvements.md)。

## 7. 推荐的只读验证清单

以下请求只用于确认协议，不触发播放、写库或修改 Emby 配置；执行前仍需用户明确授权：

1. `GET /emby/System/Info`：确认 `Version == 4.9.3.0`。
2. `GET /emby/Plugins`：确认 Playback Reporting 的实际 `Version`。
3. `GET /emby/Users`：确定明确的管理员 `UserId`，不使用“第一个用户”推断。
4. `GET /emby/Users/{adminUserId}/Views?IncludeExternalContent=false`：取一个已知非空媒体库 View ID。
5. 分别调用 `/emby/Items?Ids=...` 和 `/emby/Users/{adminUserId}/Items?Ids=...`：核对候选 `Id`、`ParentId`、`SeriesId`、`SeriesName`。
6. 调用 `/emby/Users/{adminUserId}/Items?ParentId=...&Recursive=true&Ids=...`：验证组合过滤是否确实返回交集。
7. 对插件执行只读 `SELECT`：确认 `colums`、字符串结果、`message` 和 `PlaybackActivity` 实际 schema。

## 8. 出处

### Emby `4.9.3.0` 固定版本资料

- [Emby.SDK `4.9.3.0` 版本文件](https://github.com/MediaBrowser/Emby.SDK/blob/6ee0155063bc85578196489926359a8f37419502/Version.txt#L1)
- [Emby `4.9.3.0` OpenAPI: `/Items`](https://github.com/MediaBrowser/Emby.SDK/blob/6ee0155063bc85578196489926359a8f37419502/Resources/OpenApi/openapi_v3.json#L8964)
- [Emby `4.9.3.0` OpenAPI: `/Plugins`](https://github.com/MediaBrowser/Emby.SDK/blob/6ee0155063bc85578196489926359a8f37419502/Resources/OpenApi/openapi_v3.json#L14790)
- [Emby `4.9.3.0` OpenAPI: `/System/Info`](https://github.com/MediaBrowser/Emby.SDK/blob/6ee0155063bc85578196489926359a8f37419502/Resources/OpenApi/openapi_v3.json#L44968)
- [Emby `4.9.3.0` OpenAPI: `/Users/{UserId}/Items`](https://github.com/MediaBrowser/Emby.SDK/blob/6ee0155063bc85578196489926359a8f37419502/Resources/OpenApi/openapi_v3.json#L80827)
- [Emby `4.9.3.0` OpenAPI: `/Users/{UserId}/Views`](https://github.com/MediaBrowser/Emby.SDK/blob/6ee0155063bc85578196489926359a8f37419502/Resources/OpenApi/openapi_v3.json#L81914)
- [Emby `4.9.3.0` DTO: `QueryResult_BaseItemDto`](https://github.com/MediaBrowser/Emby.SDK/blob/6ee0155063bc85578196489926359a8f37419502/Resources/OpenApi/openapi_v3.json#L103587)
- [Emby `4.9.3.0` DTO: `BaseItemDto`](https://github.com/MediaBrowser/Emby.SDK/blob/6ee0155063bc85578196489926359a8f37419502/Resources/OpenApi/openapi_v3.json#L103602)
- [Emby `4.9.3.0` `ItemFields` 枚举](https://github.com/MediaBrowser/Emby.SDK/blob/6ee0155063bc85578196489926359a8f37419502/Documentation/reference/pluginapi/MediaBrowser.Model.Querying.ItemFields.html#L165)

### Emby 在线 REST 文档

- [`GET /Items`](https://dev.emby.media/reference/RestAPI/ItemsService/getItems.html)
- [`GET /Users/{UserId}/Items`](https://dev.emby.media/reference/RestAPI/ItemsService/getUsersByUseridItems.html)
- [`GET /Users/{UserId}/Views`](https://dev.emby.media/reference/RestAPI/UserViewsService/getUsersByUseridViews.html)
- [`GET /System/Info`](https://dev.emby.media/reference/RestAPI/SystemService/getSystemInfo.html)
- [`GET /Plugins`](https://dev.emby.media/reference/RestAPI/PluginService/getPlugins.html)

### Playback Reporting 固定源码

- [插件版本 `2.1.0.7`](https://github.com/faush01/playback_reporting/blob/30d39f9934051ccd7a0536eb7db3acf3434f125b/playback_reporting/playback_reporting.csproj#L4-L6)
- [插件使用的 Emby Core 编译依赖 `4.8.0.27-beta`](https://github.com/faush01/playback_reporting/blob/30d39f9934051ccd7a0536eb7db3acf3434f125b/playback_reporting/playback_reporting.csproj#L56)
- [自定义查询路由、请求字段和管理员权限](https://github.com/faush01/playback_reporting/blob/30d39f9934051ccd7a0536eb7db3acf3434f125b/playback_reporting/Api/UserActivityAPI.cs#L93-L100)
- [自定义查询响应的 `colums`、`results`、`message`](https://github.com/faush01/playback_reporting/blob/30d39f9934051ccd7a0536eb7db3acf3434f125b/playback_reporting/Api/UserActivityAPI.cs#L1043-L1093)
- [`PlaybackActivity` schema](https://github.com/faush01/playback_reporting/blob/30d39f9934051ccd7a0536eb7db3acf3434f125b/playback_reporting/Data/ActivityRepository.cs#L200-L261)
- [自定义查询结果字符串化与错误消息](https://github.com/faush01/playback_reporting/blob/30d39f9934051ccd7a0536eb7db3acf3434f125b/playback_reporting/Data/ActivityRepository.cs#L350-L394)
- [`ItemId`、`ItemType`、`DateCreated` 等字段来源](https://github.com/faush01/playback_reporting/blob/30d39f9934051ccd7a0536eb7db3acf3434f125b/playback_reporting/EventMonitorEntryPoint.cs#L224-L272)
- [内置报表路由、请求参数与管理员权限](https://github.com/faush01/playback_reporting/blob/30d39f9934051ccd7a0536eb7db3acf3434f125b/playback_reporting/Api/UserActivityAPI.cs#L119-L249)
- [电影 / 剧集报表日期解析与配置读取](https://github.com/faush01/playback_reporting/blob/30d39f9934051ccd7a0536eb7db3acf3434f125b/playback_reporting/Api/UserActivityAPI.cs#L995-L1040)
- [内置剧集 / 电影 SQL、分组、名单与时长过滤](https://github.com/faush01/playback_reporting/blob/30d39f9934051ccd7a0536eb7db3acf3434f125b/playback_reporting/Data/ActivityRepository.cs#L918-L1026)
- [UserPlaylist 聚合粒度与字段读取](https://github.com/faush01/playback_reporting/blob/30d39f9934051ccd7a0536eb7db3acf3434f125b/playback_reporting/Data/ActivityRepository.cs#L1087-L1182)
- [插件 SQLite 连接与初始化](https://github.com/faush01/playback_reporting/blob/30d39f9934051ccd7a0536eb7db3acf3434f125b/playback_reporting/Data/ActivityRepository.cs#L86-L261)
- [播放累计时长原地更新](https://github.com/faush01/playback_reporting/blob/30d39f9934051ccd7a0536eb7db3acf3434f125b/playback_reporting/Data/ActivityRepository.cs#L643-L657)
- [20 秒会话轮询、异常退避及暂停累计](https://github.com/faush01/playback_reporting/blob/30d39f9934051ccd7a0536eb7db3acf3434f125b/playback_reporting/EventMonitorEntryPoint.cs#L113-L175)
- [Episode 展示名称拼接方式](https://github.com/faush01/playback_reporting/blob/30d39f9934051ccd7a0536eb7db3acf3434f125b/playback_reporting/EventMonitorEntryPoint.cs#L307-L324)
- [当前条目、用户累计观看状态与父路径查询](https://github.com/faush01/playback_reporting/blob/30d39f9934051ccd7a0536eb7db3acf3434f125b/playback_reporting/Api/UserActivityAPI.cs#L281-L540)
