-- media_gaps：增加 DISPATCH_FAILED 状态所需的 last_dispatch_error 列。
--
-- 业务背景：DispatchGap 在调用 MoviePilot 失败时原本只记录到日志，工单状态保持
-- REQUESTED / SEARCHED 不变；管理员看不到失败状态，也无法分辨"还在请求中"与
-- "请求失败但未重试"。本次新增 DISPATCH_FAILED 状态枚举（仅在应用层校验，不加 DB CHECK），
-- 并在工单上记录 last_dispatch_error，便于前端展示重试入口。
--
-- 字段说明：
--   last_dispatch_error  最近一次下发失败的脱敏错误（来自 upstream.SafeUpstreamError），最多 500 字符
--
-- 幂等：ADD COLUMN IF NOT EXISTS。

ALTER TABLE media_gaps
    ADD COLUMN IF NOT EXISTS "last_dispatch_error" varchar(500);
