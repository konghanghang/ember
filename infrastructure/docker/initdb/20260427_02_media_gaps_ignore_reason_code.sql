-- 20260427_02_media_gaps_ignore_reason_code
-- 用途：为 media_gaps 增加 ignoreReasonCode，显式区分人工忽略与系统忽略。
-- 变更：
--   - media_gaps 新增 ignoreReasonCode varchar(50) 可空列
--   - 回填当前由系统扫描收口产生的 "season not activated in library" 为 season_not_activated
--   - 其余历史 IGNORED 行保持空值，避免仅凭文本误判来源
-- 幂等：是，可重复执行。

ALTER TABLE media_gaps
  ADD COLUMN IF NOT EXISTS "ignoreReasonCode" varchar(50);

UPDATE media_gaps
SET "ignoreReasonCode" = 'season_not_activated'
WHERE status = 'IGNORED'
  AND "ignoreReasonCode" IS NULL
  AND "ignoreReason" = 'season not activated in library';
