-- 20260426_14_airdate_to_date
-- 用途：将 tv_calendar_items.air_date 和 media_gaps.air_date 从 timestamptz 改为 date 类型，
--       避免 DST 漂移导致日期偏移一天的问题。
--
-- 改了哪些表、字段、索引、约束：
--   - tv_calendar_items.air_date: timestamptz → date
--   - media_gaps.air_date: timestamptz → date
--
-- 是否需要回填：转换时使用 AT TIME ZONE 'UTC'，仅当时间部分为 00:00:00 UTC 时才安全执行
-- 是否可重复执行：否（二次执行 ALTER COLUMN TYPE 对已是 date 的列仍会执行，但 USING 中
--                 的 AT TIME ZONE 对 date 类型无效；生产执行前请先用预检查 SQL 验证）
--
-- 执行前预检（两条都返回 0 才能安全执行）：
-- SELECT count(*) FROM tv_calendar_items WHERE EXTRACT(HOUR FROM "air_date" AT TIME ZONE 'UTC') != 0;
-- SELECT count(*) FROM media_gaps WHERE EXTRACT(HOUR FROM "air_date" AT TIME ZONE 'UTC') != 0;

ALTER TABLE tv_calendar_items
  ALTER COLUMN "air_date" TYPE date USING ("air_date" AT TIME ZONE 'UTC')::date;

ALTER TABLE media_gaps
  ALTER COLUMN "air_date" TYPE date USING ("air_date" AT TIME ZONE 'UTC')::date;
