-- 观看保号：分组规则、用户权益考核状态与结果审计。可重复执行，无历史失效回填。
-- 新规则默认关闭；存量当前组的起点在首次启用时由分组 reset_at 提供。
ALTER TABLE plan_groups ADD COLUMN IF NOT EXISTS watch_retention_enabled boolean NOT NULL DEFAULT false;
ALTER TABLE plan_groups ADD COLUMN IF NOT EXISTS watch_retention_days integer NOT NULL DEFAULT 30;
ALTER TABLE plan_groups ADD COLUMN IF NOT EXISTS watch_retention_min_minutes integer NOT NULL DEFAULT 0;
ALTER TABLE plan_groups ADD COLUMN IF NOT EXISTS watch_retention_reset_at timestamptz;
ALTER TABLE user_entitlements ADD COLUMN IF NOT EXISTS watch_retention_started_at timestamptz;
ALTER TABLE user_entitlements ADD COLUMN IF NOT EXISTS watch_retention_invalidated_at timestamptz;
DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='plan_groups_watch_retention_valid' AND conrelid='plan_groups'::regclass) THEN
  ALTER TABLE plan_groups ADD CONSTRAINT plan_groups_watch_retention_valid CHECK (
   watch_retention_days BETWEEN 1 AND 3650 AND watch_retention_min_minutes BETWEEN 0 AND 5256000
   AND (NOT watch_retention_enabled OR (watch_retention_min_minutes > 0 AND watch_retention_reset_at IS NOT NULL))
  );
 END IF;
END $$;
CREATE TABLE IF NOT EXISTS watch_retention_checks (
 user_id varchar(25) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 plan_group varchar(50) NOT NULL,
 checked_at timestamptz NOT NULL,
 window_start timestamptz NOT NULL,
 started_at timestamptz NOT NULL,
 period_days integer NOT NULL,
 min_minutes integer NOT NULL,
 watched_seconds bigint NOT NULL,
 invalidated boolean NOT NULL,
 PRIMARY KEY (user_id, plan_group, checked_at)
);
