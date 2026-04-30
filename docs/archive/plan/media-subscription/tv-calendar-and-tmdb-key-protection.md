# 追剧日历同步与 TMDB 密钥保护方案

> 状态：已归档（主干完成，转为历史追溯）
> 负责人：Ember
> 更新时间：2026-04-30

本文档已退出现行实施稿目录。当前追剧日历 / TMDB 集成事实以 `docs/system-architecture.md` 为准；本文件仅保留历史整改过程与决策追溯价值。

## 落地进度

批次 1（commit `564ea20`）+ 后续 review 修复已完成本方案的 P0 / P1 主干项：

- ✅ 新增 `internal/common/upstream.SafeUpstreamError` / `SafeUpstreamHTTPError`，剥离 `*url.Error` 中含 `api_key` 的请求 URL 与上游响应体；同时新增 `internal/common/httpx.InternalError` 收口 handler 内部错误
- ✅ TMDB / MoviePilot 调用链路（`tvcalendar/service.go` `fetchTMDBJSON`、`mediagap/service.go` `fetchTMDBJSON`、`integrations/moviepilot/client.go` `doRequest` / `TestConnection` / `searchTitle` / `DispatchGapCandidate` / `CreateSubscription`）全部替换为脱敏 helper
- ✅ `tv_calendar.go` / `tmdb.go` / `media_gap.go` handler 中所有裸透 `err.Error()` 的 500 响应改走 `httpx.InternalError`；客户端只看到 `上游服务暂不可用` 统一文案，完整 err 落服务端日志
- ✅ 配置中心媒体测试链路（`config.go` `testEmbyConnection` / `testMoviePilotConnection`）也统一走 `SafeUpstreamError` / `SafeUpstreamHTTPError`；Emby 测试改用 `X-Emby-Token` 头携带密钥，避免 `*url.Error` 把含 `api_key` 的 URL 写进错误文本
- ✅ `MarkEpisodeReadyByWebhook` 缺主剧 `tmdbId` 时不再直接按 `seriesId` 宽匹配改库；必须先解析唯一追剧源 `tmdbId`
- ✅ 当前周 `ready` 纠偏已通过 debouncer 批量回写 `tv_calendar_items`，不再只改返回值
- ✅ `tmdb_cache` GC、`lastFullSyncAt` / `lastCorrectionAt` sync marker、cron 字符串布尔解析容错已落地
- ✅ `pickTargetSeasonNumbers` 已改为覆盖最近 2 季 + last/next episode 相关季，老剧补集不再只盯最近一季
- ✅ 同一 `cacheKey` 的 TMDB 并发击穿已通过 in-flight 去重收口，`tvcalendar` 与 `mediagap` 不再在同批请求中重复打上游

## 稳定结论

以下结论已经提炼为当前事实：

- TMDB / MoviePilot 的上游错误必须先经 `SafeUpstreamError` / `SafeUpstreamHTTPError` 脱敏，再由 `httpx.InternalError` 统一响应；客户端不再直接看到含 `api_key` 的 URL 或上游响应体。
- 追剧日历 webhook 命中精度已经固定为 `tmdbId + season + episode` 四元组；缺 `tmdbId` 时必须先解析唯一主条目，不能再用 `seriesId` 宽匹配改库。
- 当前周 `ready` 纠偏与回写节流已经固定为 `correctionDebouncer` 模式，读路径不再直接同步写库。
- `tmdb_cache` GC、`lastFullSyncAt` / `lastCorrectionAt` marker、cron 布尔解析容错已经成为当前同步基线。
- 默认季选择策略已经固定为“最近 2 季 + last/next episode 相关季”，避免老剧补集漏季。
- `resolveSeriesTMDBIDBySeriesID` 已增加 5 分钟进程内 TTL 缓存，重复 webhook / 同批匹配不再每次都打 Emby。
- 同一 `cacheKey` 的 TMDB 请求已经增加 in-flight 去重，避免 memory cache / DB cache 同时 miss 时的瞬时并发击穿。
- Stripe / SMTP 的上游网络与 HTTP 错误已经纳入 `SafeUpstreamError` / `SafeUpstreamHTTPError` 脱敏范围，不再把原始错误细节直接透传给日志回写或配置测试结果。

## 交叉引用

- 当前系统事实：
  - [docs/system-architecture.md](</Users/konghang/data/me/github/ember/docs/system-architecture.md>) §5.18 已收录追剧日历同步链路、纠偏节流、`tmdb_cache` GC 与同步 marker
  - [docs/system-architecture.md](</Users/konghang/data/me/github/ember/docs/system-architecture.md>) §4.16 已收录上游错误脱敏与 `httpx.InternalError` 的统一约束
- 当前盘点入口：
  - [docs/proposals/plan-inventory.md](</Users/konghang/data/me/github/ember/docs/proposals/plan-inventory.md>) 已把本方案标为已归档

## 退场说明

- 本文档不再承担当前追剧日历 / TMDB 集成事实说明职责；现行事实以 `docs/system-architecture.md` 为准。
- 顶部状态、交叉引用与入口文档已完成归档收口，因此本文件只保留历史追溯价值。

## 背景

2026-04-25 系统性 review 在追剧日历 / TMDB 集成 / 缺集 TMDB 调用链路暴露多类硬伤，整体品味评分 🔴：

- TMDB 错误 wrap 把含 `api_key` 的完整 URL 一路写进容器日志和 API 响应：`tvcalendar/service.go:1218` 与 `mediagap/service.go:1015`。任何能触发 TMDB 调用的渠道都能拿到密钥。
- `MarkEpisodeReadyByWebhook` 在 webhook 缺 tmdbId 时降级用 `seriesId + season + episode` 走 `Updates`，没有 tmdbId 维度限制，可能跨剧污染状态。
- `tmdb_cache` 表只写不清（`expiresAt` 字段建了索引但没人调度清理）；fetch 路径只在查询时跳过过期，无 GC 任务。
- `correctCurrentWeekReadyItems` 在 GET 接口里同步 `Updates` 写库 + 顺带刷新 `lastEpisodeIngestedAt`，是只读路径写放大热点。
- `loadReadyEpisodesBySeries` 为每个 series 触发完整 Emby 分页扫描，单次读请求可能放大成几十次 Emby 调用。
- `loadSourcesForSync`"无 activity marker 时回退全量源"叠加启动补偿同步，等价于服务每次重启都全库 force 同步一次。
- `cron.go` 把 `tvCalendarStartupSyncEnabled == "true"` 与字符串字面量比较，`True` / `TRUE` / 空格变体直接走 false 分支。
- 同步链路单剧 Emby 抖动只 log 跳过、`continue`，没有 metric / 告警可观察。
- TMDB 调用错误信息原样透传到 handler 的 `c.JSON(... err.Error() ...)`，存在内部错误外泄。

> 注：订阅与缺集状态机相关问题在 `docs/archive/plan/media-subscription/subscription-state-machine-hardening.md` 中处理；本计划聚焦追剧日历 / TMDB 集成 / 跨剧污染。
