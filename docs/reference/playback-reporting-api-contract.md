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

## 6. Ember 排行榜实现约束

基于上述合同，后续实现至少应满足：

1. 启动统计前通过 `LIMIT 0` 无数据查询校验 `DateCreated`、`ItemId`、`ItemType`、`ItemName`、`PlayDuration`、`PauseDuration` 六个字段，而不是读取真实播放记录或只校验展示字段。
2. 解析插件响应时固定读取 `colums`，并把 `message` 中的 SQL 错误升级为业务错误。
3. 电影从 Playback Reporting 的 `ItemId` 回查；Episode 先按 Episode `ItemId` 回查 `SeriesId` / `SeriesName`，再以 Series 为剧集榜聚合对象。
4. 从 `/Users/{adminUserId}/Views` 获取媒体库后，后续成员关系查询优先使用同一管理员的 `/Users/{adminUserId}/Items`，不要在没有证据时切换到全局 `/Items`。
5. 找不到明确管理员用户时应失败并记录原因，不能回退到任意第一个普通用户，否则媒体库视图和可见条目会被静默缩小。
6. `ParentId + Ids` 返回空结果时必须保留可排查日志，包括 `userId`、`libraryId`、候选数量、HTTP 状态和插件/Emby错误；禁止记录 API Key 或完整外部响应体。
7. 在 `ParentId + Ids` 交集语义完成只读实机验证前，不应将该路径描述为已确认兼容。
8. 所有 Playback Reporting 时间解析和 SQL 边界必须使用全局 `CRON_TIMEZONE`，禁止转换为 UTC 后直接匹配插件本地时间字符串。

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
