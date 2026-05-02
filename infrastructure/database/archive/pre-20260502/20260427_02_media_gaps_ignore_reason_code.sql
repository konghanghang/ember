-- 20260427_02_media_gaps_ignore_reason_code
-- 用途：为 media_gaps 增加 ignore_reason_code，显式区分人工忽略与系统忽略。
-- 变更：
--   - media_gaps 新增 ignore_reason_code varchar(50) 可空列
--   - 回填当前由系统扫描收口产生的 "season not activated in library" 为 season_not_activated
--   - 其余历史 IGNORED 行保持空值，避免仅凭文本误判来源
-- 幂等：是，可重复执行。

ALTER TABLE media_gaps
  ADD COLUMN IF NOT EXISTS "ignore_reason_code" varchar(50);

UPDATE media_gaps
SET "ignore_reason_code" = 'season_not_activated'
WHERE status = 'IGNORED'
  AND "ignore_reason_code" IS NULL
  AND "ignore_reason" = 'season not activated in library';
